package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
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

// argoCDSettings holds the relevant fields from GET /api/v1/settings.
type argoCDSettings struct {
	OIDCConfig *argoCDOIDCConfig `json:"oidcConfig"`
}

// argoCDOIDCConfig mirrors the OIDC fields exposed by the ArgoCD settings API.
type argoCDOIDCConfig struct {
	Issuer      string   `json:"issuer"`
	ClientID    string   `json:"clientID"`
	CLIClientID string   `json:"cliClientID"` // preferred for CLI flows
	Scopes      []string `json:"scopes"`
}

// oidcDiscovery holds the endpoints from /.well-known/openid-configuration.
type oidcDiscovery struct {
	AuthorizationEndpoint string `json:"authorization_endpoint"`
	TokenEndpoint         string `json:"token_endpoint"`
}

// oidcTokenResponse is the token endpoint response.
type oidcTokenResponse struct {
	IDToken     string `json:"id_token"`
	AccessToken string `json:"access_token"`
}

// SSOLogin performs SSO login, mirroring the argocd CLI --sso flow:
//  1. Fetch ArgoCD settings → issuer, clientID
//  2. Fetch OIDC discovery → auth & token endpoints
//  3. Start local HTTP server on :8085 (registered redirect URI)
//  4. Open browser to authorization URL (with PKCE)
//  5. Receive code at http://localhost:8085/auth/callback
//  6. Exchange code for id_token directly at the OIDC token endpoint
//  7. Exchange id_token for an ArgoCD session JWT via gRPC
func SSOLogin(ctx context.Context, logger *logrus.Logger, req SSOLoginRequest) (*SSOLoginResult, error) {
	scheme := "https"
	if req.PlainText {
		scheme = "http"
	}

	serverAddr := req.Server
	if req.AuthURL != "" {
		serverAddr = req.AuthURL
	}
	baseURL := fmt.Sprintf("%s://%s", scheme, serverAddr)

	httpClient := &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: req.Insecure || req.SkipVerify, //nolint:gosec
			},
		},
	}

	// Step 1: ArgoCD settings → issuer + client IDs
	settings, err := fetchArgoCDSettings(ctx, httpClient, baseURL)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch ArgoCD settings: %w", err)
	}

	var issuer, clientID string
	scopes := []string{"openid", "profile", "email", "groups"}

	if settings.OIDCConfig != nil && settings.OIDCConfig.Issuer != "" {
		issuer = settings.OIDCConfig.Issuer
		// Prefer the CLI-specific client ID (has localhost redirect URIs registered)
		clientID = settings.OIDCConfig.CLIClientID
		if clientID == "" {
			clientID = settings.OIDCConfig.ClientID
		}
		if len(settings.OIDCConfig.Scopes) > 0 {
			scopes = settings.OIDCConfig.Scopes
		}
		logger.Debugf("SSO: external OIDC issuer=%s clientID=%s", issuer, clientID)
	} else {
		// Dex (built-in)
		issuer = baseURL + "/api/dex"
		clientID = "argo-cd-cli"
		logger.Debugf("SSO: Dex at %s", issuer)
	}

	// Step 2: OIDC discovery
	disc, err := fetchOIDCDiscovery(ctx, httpClient, issuer)
	if err != nil {
		return nil, fmt.Errorf("OIDC discovery failed for %s: %w", issuer, err)
	}

	// Step 3: PKCE verifier + challenge
	verifier, challenge, err := pkceChallenge()
	if err != nil {
		return nil, fmt.Errorf("failed to generate PKCE: %w", err)
	}

	state, err := randomString(32)
	if err != nil {
		return nil, fmt.Errorf("failed to generate state: %w", err)
	}

	// Port 8085 matches the redirect URI registered in the IdP for argocd-cli.
	// Path must be /auth/callback — that's what ArgoCD CLI registers.
	const callbackPort = 8085
	redirectURI := fmt.Sprintf("http://localhost:%d/auth/callback", callbackPort)

	authURL := buildAuthURL(disc.AuthorizationEndpoint, clientID, redirectURI, state, challenge, scopes)
	logger.Debugf("SSO: auth URL: %s", authURL)

	// Steps 4–5: open browser, receive code
	code, err := runCallbackServer(ctx, callbackPort, state, authURL)
	if err != nil {
		return nil, fmt.Errorf("authorization failed: %w", err)
	}

	// Step 6: exchange code → id_token
	tokens, err := exchangeCode(ctx, httpClient, disc.TokenEndpoint, clientID, code, verifier, redirectURI)
	if err != nil {
		return nil, fmt.Errorf("token exchange failed: %w", err)
	}
	if tokens.IDToken == "" {
		return nil, fmt.Errorf("OIDC token response contained no id_token")
	}

	// Step 7: exchange id_token → ArgoCD session JWT
	argoCDToken, err := createArgoCDSession(ctx, req, tokens.IDToken)
	if err != nil {
		logger.Debugf("SSO: ArgoCD session exchange failed (%v), using id_token directly", err)
		PrintSuccess("SSO authentication successful!")
		return &SSOLoginResult{Token: tokens.IDToken, User: "sso-user"}, nil
	}

	PrintSuccess("SSO authentication successful!")
	return &SSOLoginResult{Token: argoCDToken, User: "sso-user"}, nil
}

func fetchArgoCDSettings(ctx context.Context, client *http.Client, baseURL string) (*argoCDSettings, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/api/v1/settings", nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("settings returned HTTP %d", resp.StatusCode)
	}
	var s argoCDSettings
	return &s, json.NewDecoder(resp.Body).Decode(&s)
}

func fetchOIDCDiscovery(ctx context.Context, client *http.Client, issuer string) (*oidcDiscovery, error) {
	u := strings.TrimSuffix(issuer, "/") + "/.well-known/openid-configuration"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("discovery returned HTTP %d", resp.StatusCode)
	}
	var d oidcDiscovery
	return &d, json.NewDecoder(resp.Body).Decode(&d)
}

func buildAuthURL(endpoint, clientID, redirectURI, state, pkceChallenge string, scopes []string) string {
	p := url.Values{}
	p.Set("client_id", clientID)
	p.Set("redirect_uri", redirectURI)
	p.Set("response_type", "code")
	p.Set("scope", strings.Join(scopes, " "))
	p.Set("state", state)
	p.Set("code_challenge", pkceChallenge)
	p.Set("code_challenge_method", "S256")
	return endpoint + "?" + p.Encode()
}

// runCallbackServer starts a local HTTP callback server, opens the browser,
// and returns the authorization code from the OIDC provider redirect.
func runCallbackServer(ctx context.Context, port int, expectedState, authURL string) (string, error) {
	codeChan := make(chan string, 1)
	errChan := make(chan error, 1)

	mux := http.NewServeMux()
	srv := &http.Server{Addr: fmt.Sprintf(":%d", port), Handler: mux}

	mux.HandleFunc("/auth/callback", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if e := q.Get("error"); e != "" {
			msg := fmt.Sprintf("SSO error: %s — %s", e, q.Get("error_description"))
			http.Error(w, msg, http.StatusBadRequest)
			errChan <- fmt.Errorf("%s", msg)
			return
		}
		if q.Get("state") != expectedState {
			http.Error(w, "state mismatch", http.StatusBadRequest)
			errChan <- fmt.Errorf("state mismatch in callback")
			return
		}
		code := q.Get("code")
		if code == "" {
			http.Error(w, "no authorization code", http.StatusBadRequest)
			errChan <- fmt.Errorf("no code in callback")
			return
		}
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, "<html><body><h2>Login successful!</h2><p>You can close this tab.</p></body></html>")
		codeChan <- code
	})

	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errChan <- fmt.Errorf("callback server: %w", err)
		}
	}()
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()

	PrintInfo("Opening browser for SSO login...")
	if err := openBrowser(authURL); err != nil {
		PrintWarn(fmt.Sprintf("Could not open browser. Please visit:\n%s", authURL))
	}
	PrintInfo("Waiting for authorization callback...")

	select {
	case code := <-codeChan:
		return code, nil
	case err := <-errChan:
		return "", err
	case <-ctx.Done():
		return "", fmt.Errorf("timeout waiting for authorization")
	}
}

func exchangeCode(ctx context.Context, client *http.Client, tokenEndpoint, clientID, code, verifier, redirectURI string) (*oidcTokenResponse, error) {
	data := url.Values{}
	data.Set("grant_type", "authorization_code")
	data.Set("client_id", clientID)
	data.Set("code", code)
	data.Set("redirect_uri", redirectURI)
	data.Set("code_verifier", verifier)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenEndpoint, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("token endpoint HTTP %d: %s", resp.StatusCode, body)
	}

	var t oidcTokenResponse
	return &t, json.NewDecoder(resp.Body).Decode(&t)
}

// createArgoCDSession exchanges an OIDC id_token for an ArgoCD session JWT via gRPC.
func createArgoCDSession(ctx context.Context, req SSOLoginRequest, idToken string) (string, error) {
	client, err := apiclient.NewClient(&apiclient.ClientOptions{
		ServerAddr:      req.Server,
		Insecure:        req.Insecure,
		PlainText:       req.PlainText,
		GRPCWeb:         req.GRPCWeb,
		GRPCWebRootPath: req.GRPCWebRootPath,
	})
	if err != nil {
		return "", err
	}

	closer, sessionClient, err := client.NewSessionClient()
	if err != nil {
		return "", err
	}
	defer closer.Close()

	resp, err := sessionClient.Create(ctx, &session.SessionCreateRequest{Token: idToken})
	if err != nil {
		return "", err
	}
	return resp.Token, nil
}

// pkceChallenge generates a PKCE code_verifier and its S256 code_challenge.
func pkceChallenge() (verifier, challenge string, err error) {
	b := make([]byte, 32)
	if _, err = rand.Read(b); err != nil {
		return
	}
	verifier = base64.RawURLEncoding.EncodeToString(b)
	sum := sha256.Sum256([]byte(verifier))
	challenge = base64.RawURLEncoding.EncodeToString(sum[:])
	return
}

// randomString returns a random URL-safe base64 string of n random bytes.
func randomString(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// openBrowser opens the system default browser to rawURL.
func openBrowser(rawURL string) error {
	var lastErr error
	for _, browser := range []string{"xdg-open", "open", "gio"} {
		cmd := exec.Command(browser, rawURL)
		if err := cmd.Start(); err == nil {
			return nil
		}
		lastErr = fmt.Errorf("failed to open %s: %w", browser, cmd.Err)
	}
	return lastErr
}
