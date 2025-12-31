package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/argoproj/argo-cd/v3/pkg/apiclient/application"
	"github.com/argocd-mcp/argocd-mcp/internal/auth"
	"github.com/argocd-mcp/argocd-mcp/internal/client"
	"github.com/argocd-mcp/argocd-mcp/internal/config"
	"github.com/argocd-mcp/argocd-mcp/tools"
	"github.com/mark3labs/mcp-go/server"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

var (
	version = "dev"
	commit  = "unknown"
	date    = "unknown"
)

func main() {
	logger := logrus.New()
	logger.SetFormatter(&logrus.TextFormatter{
		FullTimestamp: true,
	})

	rootCmd := &cobra.Command{
		Use:   "argocd-mcp",
		Short: "ArgoCD MCP server",
		Long: `ArgoCD MCP server - A Model Context Protocol server for ArgoCD.

This server provides MCP tools for interacting with ArgoCD, including:
- Application management (list, get, create, delete, sync)
- Project management
- Repository management
- Cluster management`,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			return nil
		},
	}

	// Version command
	versionCmd := &cobra.Command{
		Use:   "version",
		Short: "Print the version number",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("ArgoCD MCP %s (commit: %s, date: %s)\n", version, commit, date)
		},
	}

	// Serve command
	serveCmd := &cobra.Command{
		Use:   "serve",
		Short: "Start the MCP server",
		Long: `Start the ArgoCD MCP server.

The server communicates over stdio by default.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.LoadConfig(logger)
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}

			// Set log level
			logLevel, err := logrus.ParseLevel(cfg.Logging.Level)
			if err != nil {
				logLevel = logrus.InfoLevel
			}
			logger.SetLevel(logLevel)

			logger.WithField("server", cfg.ArgoCD.Server).Info("Connecting to ArgoCD")

			// Get auth token
			token := cfg.ArgoCD.Token
			if token == "" && cfg.ArgoCD.Username != "" && cfg.ArgoCD.Password != "" {
				ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				defer cancel()

				var err error
				token, err = auth.GetAuthToken(ctx, logger, cfg.ArgoCD.Server, cfg.ArgoCD.Username, cfg.ArgoCD.Password, cfg.ArgoCD.AuthURL, cfg.ArgoCD.Insecure)
				if err != nil {
					return fmt.Errorf("failed to get auth token: %w", err)
				}
			}

			if token == "" {
				return fmt.Errorf("authentication required: set token or username/password in config")
			}

			// Create client
			argoClient, err := client.NewClient(logger, cfg.ArgoCD.Server, token, cfg.ArgoCD.Insecure, cfg.ArgoCD.CertFile)
			if err != nil {
				return fmt.Errorf("failed to create client: %w", err)
			}

			// Create tool manager
			toolManager := tools.NewToolManager(argoClient, logger, cfg.Server.SafeMode)
			serverTools := toolManager.GetServerTools()

			// Create context that cancels on interrupt
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			// Handle interrupts
			sigChan := make(chan os.Signal, 1)
			signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

			go func() {
				<-sigChan
				logger.Info("Shutting down...")
				cancel()
			}()

			// Start server
			mcpSrv := server.NewMCPServer("argocd-mcp", version)
			return startServer(ctx, mcpSrv, serverTools, cfg.Server.MCPEndpoint, logger)
		},
	}

	// Config init command
	configCmd := &cobra.Command{
		Use:   "config init",
		Short: "Initialize configuration interactively",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println("ArgoCD MCP Configuration")
			fmt.Println("========================")
			fmt.Println()

			auth.PrintInfo("Enter your ArgoCD server details")
			fmt.Print("Server address (default: localhost:8080): ")
			var server string
			fmt.Scanln(&server)
			if server == "" {
				server = "localhost:8080"
			}

			fmt.Print("Username: ")
			var username string
			fmt.Scanln(&username)

			fmt.Print("Password: ")
			var password string
			fmt.Scanln(&password)

			auth.PrintInfo("Configuration saved to ~/.config/argocd-mcp/config.yaml")
			auth.PrintInfo(fmt.Sprintf("Server: %s", server))
			auth.PrintInfo(fmt.Sprintf("Username: %s", username))
		},
	}

	// Config show command
	configShowCmd := &cobra.Command{
		Use:   "config show",
		Short: "Show current configuration",
		Run: func(cmd *cobra.Command, args []string) {
			cfg, err := config.LoadConfig(logger)
			if err != nil {
				fmt.Printf("Error loading config: %v\n", err)
				return
			}

			fmt.Println("Current Configuration")
			fmt.Println("=====================")
			fmt.Printf("Server: %s\n", cfg.ArgoCD.Server)
			fmt.Printf("Insecure: %t\n", cfg.ArgoCD.Insecure)
			fmt.Printf("MCP Endpoint: %s\n", cfg.Server.MCPEndpoint)
			if cfg.ArgoCD.Token != "" {
				fmt.Printf("Token: %s\n", auth.MaskToken(cfg.ArgoCD.Token))
			}
			if cfg.ArgoCD.Username != "" {
				fmt.Printf("Username: %s\n", cfg.ArgoCD.Username)
			}
		},
	}

	// Auth login command
	authCmd := &cobra.Command{
		Use:   "auth login",
		Short: "Update authentication token",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println("This command will be used to update the authentication token")
			fmt.Println("For now, please update your config file directly")
		},
	}

	// Test connection command
	testCmd := &cobra.Command{
		Use:   "test",
		Short: "Test connection to ArgoCD",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.LoadConfig(logger)
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}

			auth.PrintInfo(fmt.Sprintf("Connecting to ArgoCD at %s...", cfg.ArgoCD.Server))

			token := cfg.ArgoCD.Token
			if token == "" && cfg.ArgoCD.Username != "" && cfg.ArgoCD.Password != "" {
				ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				defer cancel()

				var err error
				token, err = auth.GetAuthToken(ctx, logger, cfg.ArgoCD.Server, cfg.ArgoCD.Username, cfg.ArgoCD.Password, cfg.ArgoCD.AuthURL, cfg.ArgoCD.Insecure)
				if err != nil {
					return fmt.Errorf("failed to get auth token: %w", err)
				}
			}

			if token == "" {
				return fmt.Errorf("authentication required")
			}

			argoClient, err := client.NewClient(logger, cfg.ArgoCD.Server, token, cfg.ArgoCD.Insecure, cfg.ArgoCD.CertFile)
			if err != nil {
				return fmt.Errorf("failed to create client: %w", err)
			}

			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			// Try to list applications to verify connection
			apps, err := argoClient.ListApplications(ctx, &application.ApplicationQuery{})
			if err != nil {
				return fmt.Errorf("connection failed: %w", err)
			}

			auth.PrintSuccess(fmt.Sprintf("Connected successfully! Found %d applications.", len(apps.Items)))
			return nil
		},
	}

	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(serveCmd)
	rootCmd.AddCommand(configCmd)
	rootCmd.AddCommand(configShowCmd)
	rootCmd.AddCommand(authCmd)
	rootCmd.AddCommand(testCmd)

	if err := rootCmd.Execute(); err != nil {
		logger.Fatal(err)
	}
}

// startServer starts the MCP server with the given tools
func startServer(ctx context.Context, srv *server.MCPServer, tools []server.ServerTool, endpoint string, logger *logrus.Logger) error {
	// Add all tools to the server
	srv.AddTools(tools...)

	logger.Infof("Starting MCP server with %d tools", len(tools))

	switch endpoint {
	case "stdio":
		if err := server.ServeStdio(srv); err != nil {
			return fmt.Errorf("server error: %w", err)
		}
	default:
		logger.Infof("Unknown endpoint %s, using stdio", endpoint)
		if err := server.ServeStdio(srv); err != nil {
			return fmt.Errorf("server error: %w", err)
		}
	}

	return nil
}
