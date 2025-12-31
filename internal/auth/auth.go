package auth

import (
	"context"
	"fmt"

	"github.com/argoproj/argo-cd/v3/pkg/apiclient"
	"github.com/argoproj/argo-cd/v3/pkg/apiclient/account"
	"github.com/charmbracelet/lipgloss"
	"github.com/sirupsen/logrus"
)

// GetAuthToken retrieves an auth token using username/password
func GetAuthToken(ctx context.Context, logger *logrus.Logger, server, username, password, authURL string, insecure bool) (string, error) {
	opts := &apiclient.ClientOptions{
		ServerAddr: server,
		AuthToken:  username + ":" + password,
		Insecure:   insecure,
	}

	if authURL != "" {
		opts.ServerAddr = authURL
	}

	// Create a temporary client to get the token
	client, err := apiclient.NewClient(opts)
	if err != nil {
		return "", fmt.Errorf("failed to create client: %w", err)
	}

	// Get account to verify credentials
	closer, accountClient, err := client.NewAccountClient()
	if err != nil {
		return "", fmt.Errorf("failed to create account client: %w", err)
	}
	defer closer.Close()

	resp, err := accountClient.GetAccount(ctx, &account.GetAccountRequest{Name: username})
	if err != nil {
		return "", fmt.Errorf("failed to get account: %w", err)
	}

	if len(resp.Tokens) == 0 {
		return "", fmt.Errorf("no tokens found for user %s", username)
	}

	// Return the most recent token (using the Id as token since Token field doesn't exist)
	token := resp.Tokens[0].Id
	logger.Debug("Retrieved auth token from ArgoCD server")
	return token, nil
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
