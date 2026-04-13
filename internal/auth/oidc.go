package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os/exec"
	"strings"
	"time"

	"github.com/argoproj/argo-cd/v3/pkg/apiclient"
	"github.com/argoproj/argo-cd/v3/pkg/apiclient/session"
	"github.com/sirupsen/logrus"
)

// SSOLoginRequest contains the parameters for SSO login
type SSOLoginRequest struct {
	Server          string
	AuthURL         string
	Insecure        bool
	PlainText       bool
	GRPCWeb         bool
	GRPCWebRootPath string
	SkipVerify      bool
}

// SSOLoginResult contains the result of SSO login
type SSOLoginResult struct {
	Token string
	User  string
}

// DeviceCodeResponse contains the device flow response from ArgoCD
type DeviceCodeResponse struct {
	DeviceCode      string `json:"device_code"`
	UserCode        string `json:"user_code"`
	VerificationURI string `json:"verification_uri"`
	Interval        int    `json:"interval"`
	ExpiresIn       int    `json:"expires_in"`
}

// TokenResponse contains the token response from ArgoCD
type TokenResponse struct {
	Token     string `json:"token"`
	ExpiresIn int    `json:"expires_in"`
}

// SSOLogin performs SSO login using the ArgoCD OIDC device flow
// This mimics the behavior of `argocd login --sso`
func SSOLogin(ctx context.Context, logger *logrus.Logger, req SSOLoginRequest) (*SSOLoginResult, error) {
	// Determine which server address to use for auth
	authServerAddr := req.Server
	if req.AuthURL != "" {
		authServerAddr = req.AuthURL
	}

	logger.Debugf("Starting SSO login with server: %s", authServerAddr)

	// Create client for SSO initiation
	opts := &apiclient.ClientOptions{
		ServerAddr:      authServerAddr,
		Insecure:        req.Insecure,
		PlainText:       req.PlainText,
		GRPCWeb:         req.GRPCWeb,
		GRPCWebRootPath: req.GRPCWebRootPath,
	}

	client, err := apiclient.NewClient(opts)
	if err != nil {
		return nil, fmt.Errorf("failed to create client: %w", err)
	}

	// Create session client - same as password auth, but ArgoCD handles SSO internally
	// when the user is configured for OIDC
	closer, sessionClient, err := client.NewSessionClient()
	if err != nil {
		return nil, fmt.Errorf("failed to create session client: %w", err)
	}
	defer closer.Close()

	// Get the session - for SSO users, ArgoCD will return an authorization URL
	// when OIDC is configured. The ArgoCD CLI then opens this URL in the browser.
	sessionResp, err := sessionClient.Create(ctx, &session.SessionCreateRequest{})
	if err != nil {
		return nil, fmt.Errorf("SSO session creation failed: %w", err)
	}

	// Check if we got a token directly (OIDC may return token immediately in some flows)
	if sessionResp.Token != "" && !strings.Contains(sessionResp.Token, "http") {
		PrintSuccess("SSO authentication successful!")
		return &SSOLoginResult{
			Token: sessionResp.Token,
			User:  "sso-user",
		}, nil
	}

	// If the token is a URL, we need to go through the browser flow
	if sessionResp.Token != "" {
		authURL := sessionResp.Token
		logger.Debugf("Authorization URL received: %s", authURL)

		// Check if it's a device flow URL or browser flow URL
		if strings.Contains(authURL, "client_id") {
			return runDeviceFlow(ctx, logger, authURL)
		}

		// For browser-based flow, open the browser
		return runBrowserFlow(ctx, logger, authURL, req.SkipVerify)
	}

	return nil, fmt.Errorf("no token or authorization URL received from SSO session")
}

// runDeviceFlow handles the OIDC device authorization flow
func runDeviceFlow(ctx context.Context, logger *logrus.Logger, authURL string) (*SSOLoginResult, error) {
	PrintInfo("Starting SSO device authorization flow...")

	// Parse the device code URL
	parsedURL, err := url.Parse(authURL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse device URL: %w", err)
	}

	// Extract base URL and parameters
	baseURL := parsedURL.Scheme + "://" + parsedURL.Host
	if parsedURL.Path != "" {
		baseURL += parsedURL.Path
	}

	params := parsedURL.Query()
	clientID := params.Get("client_id")
	scope := params.Get("scope")

	if clientID == "" {
		return nil, fmt.Errorf("client_id not found in device authorization URL")
	}

	// Construct the token endpoint
	tokenURL := strings.TrimSuffix(baseURL, "/authorize") + "/token"

	// Step 1: Request device code
	deviceCodeData := url.Values{}
	deviceCodeData.Set("client_id", clientID)
	deviceCodeData.Set("scope", scope)

	httpClient := &http.Client{Timeout: 30 * time.Second}
	resp, err := httpClient.PostForm(tokenURL, deviceCodeData)
	if err != nil {
		// Try alternative endpoint format
		tokenURL = baseURL + "/device/code"
		resp, err = httpClient.PostForm(tokenURL, deviceCodeData)
		if err != nil {
			return nil, fmt.Errorf("failed to request device code: %w", err)
		}
	}
	defer resp.Body.Close()

	var deviceResp DeviceCodeResponse
	if err := json.NewDecoder(resp.Body).Decode(&deviceResp); err != nil {
		return nil, fmt.Errorf("failed to parse device code response: %w", err)
	}

	PrintSuccess(fmt.Sprintf("Please visit: %s", deviceResp.VerificationURI))
	PrintInfo(fmt.Sprintf("Enter code: %s", deviceResp.UserCode))
	PrintInfo("Waiting for authorization...")

	// Step 2: Poll for token
	tokenData := url.Values{}
	tokenData.Set("client_id", clientID)
	tokenData.Set("grant_type", "urn:ietf:params:oauth:grant-type:device_code")
	tokenData.Set("device_code", deviceResp.DeviceCode)

	interval := time.Duration(deviceResp.Interval) * time.Second
	if interval == 0 {
		interval = 5 * time.Second
	}

	expires := time.Now().Add(time.Duration(deviceResp.ExpiresIn) * time.Second)

	pollCtx, cancel := context.WithTimeout(ctx, time.Duration(deviceResp.ExpiresIn)*time.Second)
	defer cancel()

	for {
		select {
		case <-pollCtx.Done():
			return nil, fmt.Errorf("timeout waiting for authorization")
		case <-time.After(interval):
			resp2, err := httpClient.PostForm(tokenURL, tokenData)
			if err != nil {
				logger.Debugf("Token poll error: %v", err)
				continue
			}
			defer resp2.Body.Close()

			var tokenResp TokenResponse
			if err := json.NewDecoder(resp2.Body).Decode(&tokenResp); err != nil {
				logger.Debugf("Token parse error: %v", err)
				continue
			}

			// Check for success or error
			if tokenResp.Token != "" {
				PrintSuccess("Authentication successful!")
				return &SSOLoginResult{
					Token: tokenResp.Token,
					User:  "sso-user",
				}, nil
			}

			if time.Now().After(expires) {
				return nil, fmt.Errorf("device code expired")
			}
		}
	}
}

// runBrowserFlow opens the browser for authorization and waits for callback
func runBrowserFlow(ctx context.Context, logger *logrus.Logger, authURL string, skipVerify bool) (*SSOLoginResult, error) {
	PrintInfo(fmt.Sprintf("Please authorize at: %s", authURL))

	// Try to open the browser
	if err := openBrowser(authURL); err != nil {
		PrintWarn("Could not open browser automatically. Please visit the URL above.")
	} else {
		PrintInfo("Browser opened automatically")
	}

	PrintInfo("Waiting for authorization...")

	// For browser-based OIDC flow, we need to start a local callback server
	// This mimics what the ArgoCD CLI does
	return startCallbackServer(ctx, logger, authURL, skipVerify)
}

// startCallbackServer starts a local HTTP server to receive the OIDC callback
func startCallbackServer(ctx context.Context, logger *logrus.Logger, authURL string, skipVerify bool) (*SSOLoginResult, error) {
	// Parse the auth URL to determine callback port
	parsedURL, err := url.Parse(authURL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse auth URL: %w", err)
	}

	// Use a random port for the callback - ArgoCD allows any port
	callbackPort := 8085

	// Determine the scheme (prefer https for callback if original is https)
	scheme := "http"
	if parsedURL.Scheme == "https" {
		scheme = "https"
	}

	callbackURL := fmt.Sprintf("%s://localhost:%d/callback", scheme, callbackPort)

	// Construct the actual login URL with callback
	loginURL := authURL
	if strings.Contains(authURL, "?") {
		loginURL += "&redirect_uri=" + url.QueryEscape(callbackURL)
	} else {
		loginURL += "?redirect_uri=" + url.QueryEscape(callbackURL)
	}

	// Start the callback server in a goroutine
	resultChan := make(chan *SSOLoginResult, 1)
	errorChan := make(chan error, 1)

	go func() {
		// Simple callback server
		http.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
			code := r.URL.Query().Get("code")
			if code == "" {
				w.WriteHeader(http.StatusBadRequest)
				w.Write([]byte("No authorization code received"))
				return
			}

			// Write success response
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("Authorization successful! You can close this tab."))

			// Exchange the code for a token via ArgoCD's token endpoint
			token, err := exchangeCodeForToken(ctx, authURL, code, skipVerify)
			if err != nil {
				errorChan <- fmt.Errorf("failed to exchange code for token: %w", err)
				return
			}

			resultChan <- &SSOLoginResult{
				Token: token,
				User:  "sso-user",
			}
		})

		// Also handle the root path gracefully
		http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/callback" {
				return // Already handled above
			}
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("Waiting for authorization callback..."))
		})

		server := &http.Server{
			Addr: fmt.Sprintf(":%d", callbackPort),
		}
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errorChan <- err
		}
	}()

	// Open the browser with the callback URL
	if err := openBrowser(loginURL); err != nil {
		logger.Debugf("Could not open browser: %v", err)
	}

	// Wait for either the result or an error
	select {
	case result := <-resultChan:
		PrintSuccess("Authentication successful!")
		return result, nil
	case err := <-errorChan:
		return nil, err
	case <-ctx.Done():
		return nil, fmt.Errorf("context cancelled while waiting for callback")
	}
}

// exchangeCodeForToken exchanges an authorization code for a token
func exchangeCodeForToken(ctx context.Context, authURL, code string, skipVerify bool) (string, error) {
	// Parse the original auth URL to get the token endpoint
	parsedURL, err := url.Parse(authURL)
	if err != nil {
		return "", fmt.Errorf("failed to parse auth URL: %w", err)
	}

	tokenURL := parsedURL.Scheme + "://" + parsedURL.Host
	if !strings.HasSuffix(tokenURL, "/token") {
		tokenURL = strings.TrimSuffix(tokenURL, "/authorize") + "/token"
	}

	// Get client_id from original URL for token exchange
	clientID := parsedURL.Query().Get("client_id")
	if clientID == "" {
		clientID = "argocd"
	}

	// Exchange code for token
	tokenData := url.Values{}
	tokenData.Set("client_id", clientID)
	tokenData.Set("grant_type", "authorization_code")
	tokenData.Set("code", code)

	httpClient := &http.Client{Timeout: 30 * time.Second}
	resp, err := httpClient.PostForm(tokenURL, tokenData)
	if err != nil {
		return "", fmt.Errorf("failed to exchange code: %w", err)
	}
	defer resp.Body.Close()

	var tokenResp TokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return "", fmt.Errorf("failed to parse token response: %w", err)
	}

	return tokenResp.Token, nil
}

// openBrowser opens the default browser with the given URL
func openBrowser(url string) error {
	var lastErr error
	for _, browser := range []string{"xdg-open", "open", "gio"} {
		cmd := exec.Command(browser, url)
		if err := cmd.Start(); err == nil {
			return nil
		}
		lastErr = fmt.Errorf("failed to open browser %s: %w", browser, cmd.Err)
	}
	return lastErr
}
