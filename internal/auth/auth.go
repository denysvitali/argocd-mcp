package auth

import (
	"context"
	"fmt"

	"github.com/argoproj/argo-cd/v3/pkg/apiclient"
	"github.com/argoproj/argo-cd/v3/pkg/apiclient/account"
	"github.com/argoproj/argo-cd/v3/pkg/apiclient/session"
	"github.com/charmbracelet/lipgloss"
	"github.com/sirupsen/logrus"
)

// GetAuthToken retrieves an auth token using username/password
func GetAuthToken(ctx context.Context, logger *logrus.Logger, server, username, password, authURL string, insecure, plaintext, grpcWeb bool, grpcWebRootPath string) (string, error) {
	// Determine which server address to use for auth
	authServerAddr := server
	if authURL != "" {
		authServerAddr = authURL
	}

	logger.Debugf("Creating auth client with server: %s, authURL: %s", server, authURL)
	logger.Debugf("Connection options - Insecure: %v, PlainText: %v, GRPCWeb: %v, GRPCWebRootPath: %s", insecure, plaintext, grpcWeb, grpcWebRootPath)

	// First, create a session client without auth to get a JWT token
	opts := &apiclient.ClientOptions{
		ServerAddr:      authServerAddr,
		Insecure:        insecure,
		PlainText:       plaintext,
		GRPCWeb:         grpcWeb,
		GRPCWebRootPath: grpcWebRootPath,
	}

	logger.Debug("Creating temporary client for session...")

	// Create a temporary client to get the token
	client, err := apiclient.NewClient(opts)
	if err != nil {
		logger.Debugf("Failed to create client: %v", err)
		return "", fmt.Errorf("failed to create client: %w", err)
	}

	logger.Debug("Creating session client...")

	// Create a session to get JWT token
	closer, sessionClient, err := client.NewSessionClient()
	if err != nil {
		logger.Debugf("Failed to create session client: %v", err)
		return "", fmt.Errorf("failed to create session client: %w", err)
	}
	defer closer.Close()

	logger.Debugf("Creating session for user: %s", username)

	// Create session with username/password
	sessionResp, err := sessionClient.Create(ctx, &session.SessionCreateRequest{
		Username: username,
		Password: password,
	})
	if err != nil {
		logger.Debugf("Failed to create session: %v", err)
		return "", fmt.Errorf("failed to create session: %w", err)
	}

	if sessionResp.Token == "" {
		return "", fmt.Errorf("received empty token from ArgoCD server")
	}

	logger.Debugf("Session created successfully for user: %s", username)

	// Now use the JWT token to get account info and verify it works
	logger.Debug("Creating authenticated client for token verification...")

	authOpts := &apiclient.ClientOptions{
		ServerAddr:      server,
		AuthToken:       sessionResp.Token,
		Insecure:        insecure,
		PlainText:       plaintext,
		GRPCWeb:         grpcWeb,
		GRPCWebRootPath: grpcWebRootPath,
	}

	authClient, err := apiclient.NewClient(authOpts)
	if err != nil {
		logger.Debugf("Failed to create authenticated client: %v", err)
		return "", fmt.Errorf("failed to create authenticated client: %w", err)
	}

	closer, accountClient, err := authClient.NewAccountClient()
	if err != nil {
		logger.Debugf("Could not create account client for verification: %v", err)
		return sessionResp.Token, nil // Return token anyway if we can't verify
	}
	defer closer.Close()

	// Verify the token works by getting account info
	logger.Debug("Verifying token by getting account info...")
	_, err = accountClient.GetAccount(ctx, &account.GetAccountRequest{Name: username})
	if err != nil {
		logger.Warnf("Token received but account verification failed: %v", err)
		// Return the token anyway - it might still work for other operations
	} else {
		logger.Debug("Token verified successfully")
	}

	logger.Debug("Retrieved auth token from ArgoCD server")
	return sessionResp.Token, nil
}

// MaskToken masks the auth token for display
func MaskToken(token string) string {
	if len(token) < 8 {
		return "****"
	}
	return token[:4] + "****" + token[len(token)-4:]
}

// Print success message
func PrintSuccess(msg string) {
	fmt.Println(lipgloss.NewStyle().Foreground(lipgloss.Color("10")).Render("✓ ") + msg)
}

// Print error message
func PrintError(msg string) {
	fmt.Println(lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Render("✗ ") + msg)
}

// Print info message
func PrintInfo(msg string) {
	fmt.Println(lipgloss.NewStyle().Foreground(lipgloss.Color("12")).Render("→ ") + msg)
}

// Print warning message
func PrintWarn(msg string) {
	fmt.Println(lipgloss.NewStyle().Foreground(lipgloss.Color("11")).Render("⚠ ") + msg)
}
