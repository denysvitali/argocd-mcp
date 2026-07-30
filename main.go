package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/argoproj/argo-cd/v3/pkg/apiclient/application"
	"github.com/denysvitali/argocd-mcp/internal/auth"
	"github.com/denysvitali/argocd-mcp/internal/client"
	"github.com/denysvitali/argocd-mcp/internal/config"
	"github.com/denysvitali/argocd-mcp/tools"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"sigs.k8s.io/yaml"
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
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			// Return nil to continue
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.LoadConfig(logger, "")
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}

			// Override from CLI flags if set
			if grpcWeb, _ := cmd.Flags().GetBool("grpc-web"); grpcWeb {
				cfg.ArgoCD.GRPCWeb = grpcWeb
			}
			if grpcWebRootPath, _ := cmd.Flags().GetString("grpc-web-root-path"); grpcWebRootPath != "" {
				cfg.ArgoCD.GRPCWebRootPath = grpcWebRootPath
			}
			if readWrite, _ := cmd.Flags().GetBool("read-write"); readWrite {
				cfg.Server.SafeMode = false
			}
			if allowDeletes, _ := cmd.Flags().GetBool("allow-deletes"); allowDeletes {
				cfg.Server.AllowDeletes = true
			}

			// Set log level
			logLevel, err := logrus.ParseLevel(cfg.Logging.Level)
			if err != nil {
				logger.Warnf("Invalid log level '%s', using default 'info': %v", cfg.Logging.Level, err)
				logLevel = logrus.InfoLevel
			}
			logger.SetLevel(logLevel)

			switch {
			case cfg.Server.SafeMode:
				logger.Info("Running in read-only mode (all writes disabled). Use --read-write to enable writes.")
			case cfg.Server.AllowDeletes:
				logger.Warn("Running in read-write mode with deletes enabled")
			default:
				logger.Warn("Running in read-write mode (deletes still disabled). Use --allow-deletes to enable deletes.")
			}

			logger.WithField("server", cfg.ArgoCD.Server).Info("Connecting to ArgoCD")

			// Get auth token
			token := cfg.ArgoCD.Token
			var refreshFn func(context.Context) (string, error)
			if cfg.ArgoCD.Username != "" && cfg.ArgoCD.Password != "" {
				// Capture config values for use in the refresh closure.
				argoCDServer := cfg.ArgoCD.Server
				argoCDUsername := cfg.ArgoCD.Username
				argoCDPassword := cfg.ArgoCD.Password
				argoCDAuthURL := cfg.ArgoCD.AuthURL
				argoCDInsecure := cfg.ArgoCD.Insecure
				argoCDPlainText := cfg.ArgoCD.PlainText
				argoCDGRPCWeb := cfg.ArgoCD.GRPCWeb
				argoCDGRPCWebRootPath := cfg.ArgoCD.GRPCWebRootPath
				refreshFn = func(ctx context.Context) (string, error) {
					return auth.GetAuthToken(ctx, logger, argoCDServer, argoCDUsername, argoCDPassword, argoCDAuthURL, argoCDInsecure, argoCDPlainText, argoCDGRPCWeb, argoCDGRPCWebRootPath)
				}
				if token == "" {
					ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
					defer cancel()

					var err error
					token, err = refreshFn(ctx)
					if err != nil {
						return fmt.Errorf("failed to get auth token: %w", err)
					}
				}
			}

			if token == "" {
				return fmt.Errorf("authentication required: set token or username/password in config")
			}

			// Create client
			argoClient, err := client.NewClientWithRefresh(logger, cfg.ArgoCD.Server, token, cfg.ArgoCD.Insecure, cfg.ArgoCD.PlainText, cfg.ArgoCD.CertFile, cfg.ArgoCD.GRPCWeb, cfg.ArgoCD.GRPCWebRootPath, refreshFn)
			if err != nil {
				return fmt.Errorf("failed to create client: %w", err)
			}

			// Ping: verify connectivity and auth before starting MCP loop.
			pingCtx, pingCancel := context.WithTimeout(context.Background(), 10*time.Second)
			if err := argoClient.Ping(pingCtx); err != nil {
				logger.Warnf("Startup connectivity check failed: %v", err)
			}
			pingCancel()

			// Create tool manager
			toolManager := tools.NewToolManager(argoClient, logger, cfg.Server.SafeMode, cfg.Server.AllowDeletes)
			structuredOutput := cfg.Server.StructuredOutput
			if cmd.Flags().Changed("structured-output") {
				structuredOutput, _ = cmd.Flags().GetBool("structured-output")
			}
			toolManager.SetStructuredOutput(structuredOutput)
			if structuredOutput {
				logger.Info("Structured output enabled: results also carry JSON structuredContent")
			}
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
			mcpSrv := mcp.NewServer(&mcp.Implementation{
				Name:    "argocd-mcp",
				Version: version,
			}, nil)
			listen, _ := cmd.Flags().GetString("listen")
			endpoint := cfg.Server.MCPEndpoint
			if flagEndpoint, _ := cmd.Flags().GetString("mcp-endpoint"); flagEndpoint != "" {
				endpoint = flagEndpoint
			}
			return startServer(ctx, mcpSrv, serverTools, endpoint, listen, logger)
		},
	}

	// Add flags to serveCmd
	serveCmd.Flags().Bool("grpc-web", false, "Enable gRPC-Web mode (use when ArgoCD is behind a reverse proxy that doesn't support native gRPC)")
	serveCmd.Flags().String("grpc-web-root-path", "", "Root path for gRPC-Web requests (e.g., /argo-cd)")
	serveCmd.Flags().Bool("read-write", false, "Enable write operations (overrides read-only default and config file)")
	serveCmd.Flags().Bool("allow-deletes", false, "Enable delete operations (requires --read-write; deletes are always gated separately)")
	serveCmd.Flags().String("mcp-endpoint", "", "Transport to serve on: stdio (default) or http")
	serveCmd.Flags().String("listen", "127.0.0.1:8000", "Address to listen on when --mcp-endpoint=http")
	serveCmd.Flags().Bool("structured-output", false, "Also return each tool result as JSON in structuredContent (duplicates the payload; only useful for clients that read it)")

	// Config command group. Cobra derives a command's name from the first
	// word of Use, so registering "config init" and "config show" as
	// siblings on rootCmd created two commands both named "config" and
	// "config show" silently ran init instead.
	configCmd := &cobra.Command{
		Use:   "config",
		Short: "Manage configuration",
	}

	// Config init command
	configInitCmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize configuration",
		Long: `Initialize ArgoCD MCP configuration.

Use flags for non-interactive configuration:
  argocd-mcp config init --server argocd.example.com:443 --username admin --password secret

Or run interactively without flags:
  argocd-mcp config init`,
		Run: func(cmd *cobra.Command, args []string) {
			// Get flags
			server, _ := cmd.Flags().GetString("server")
			username, _ := cmd.Flags().GetString("username")
			password, _ := cmd.Flags().GetString("password")
			token, _ := cmd.Flags().GetString("token")
			insecure, _ := cmd.Flags().GetBool("insecure")
			plaintext, _ := cmd.Flags().GetBool("plaintext")
			certFile, _ := cmd.Flags().GetString("cert-file")
			grpcWeb, _ := cmd.Flags().GetBool("grpc-web")
			grpcWebRootPath, _ := cmd.Flags().GetString("grpc-web-root-path")

			// Interactive mode if no flags provided
			interactive := server == "" && username == "" && password == "" && token == ""
			if interactive {
				fmt.Println("ArgoCD MCP Configuration")
				fmt.Println("========================")
				fmt.Println()

				auth.PrintInfo("Enter your ArgoCD server details")
				fmt.Print("Server address (default: localhost:8080): ")
				var srv string
				fmt.Scanln(&srv)
				if srv == "" {
					srv = "localhost:8080"
				}
				server = srv

				fmt.Print("Username: ")
				var user string
				fmt.Scanln(&user)
				username = user

				fmt.Print("Password: ")
				var pass string
				fmt.Scanln(&pass)
				password = pass
			}

			// Create config structure
			cfg := config.Config{
				ArgoCD: config.ArgoCDConfig{
					Server:          server,
					Username:        username,
					Password:        password,
					Token:           token,
					Insecure:        insecure,
					PlainText:       plaintext,
					CertFile:        certFile,
					GRPCWeb:         grpcWeb,
					GRPCWebRootPath: grpcWebRootPath,
				},
				Server: config.ServerConfig{
					MCPEndpoint:  "stdio",
					SafeMode:     true,
					AllowDeletes: false,
				},
				Logging: config.LoggingConfig{
					Level:  "info",
					Format: "json",
				},
			}

			// Create config directory
			configDir := filepath.Join(os.Getenv("HOME"), ".config", "argocd-mcp")
			if err := os.MkdirAll(configDir, 0755); err != nil {
				auth.PrintError(fmt.Sprintf("Failed to create config directory: %v", err))
				return
			}

			// Save config file
			configPath := filepath.Join(configDir, "config.yaml")
			data, err := yaml.Marshal(cfg)
			if err != nil {
				auth.PrintError(fmt.Sprintf("Failed to marshal config: %v", err))
				return
			}

			if err := os.WriteFile(configPath, data, 0600); err != nil {
				auth.PrintError(fmt.Sprintf("Failed to write config file: %v", err))
				return
			}

			auth.PrintSuccess("Configuration saved to " + configPath)
			auth.PrintInfo(fmt.Sprintf("Server: %s", server))
			if username != "" {
				auth.PrintInfo(fmt.Sprintf("Username: %s", username))
			}
			if plaintext {
				auth.PrintWarn("Plaintext mode enabled (HTTP without TLS)")
			}
			if insecure {
				auth.PrintWarn("Insecure mode enabled (skipping TLS verification)")
			}
		},
	}

	// Add flags for non-interactive configuration
	configInitCmd.Flags().StringP("server", "s", "", "ArgoCD server address (e.g., argocd.example.com:443)")
	configInitCmd.Flags().StringP("username", "u", "", "Username for authentication")
	configInitCmd.Flags().StringP("password", "p", "", "Password for authentication")
	configInitCmd.Flags().StringP("token", "t", "", "Authentication token (alternative to username/password)")
	configInitCmd.Flags().BoolP("insecure", "k", false, "Skip TLS certificate verification")
	configInitCmd.Flags().BoolP("plaintext", "", false, "Use HTTP without TLS (for testing only)")
	configInitCmd.Flags().StringP("cert-file", "c", "", "Path to CA certificate file")
	configInitCmd.Flags().Bool("grpc-web", false, "Enable gRPC-Web mode (use when ArgoCD is behind a reverse proxy that doesn't support native gRPC)")
	configInitCmd.Flags().String("grpc-web-root-path", "", "Root path for gRPC-Web requests (e.g., /argo-cd)")

	// Config show command
	configShowCmd := &cobra.Command{
		Use:   "show",
		Short: "Show current configuration",
		Run: func(cmd *cobra.Command, args []string) {
			cfg, err := config.LoadConfig(logger, "")
			if err != nil {
				fmt.Printf("Error loading config: %v\n", err)
				return
			}

			fmt.Println("Current Configuration")
			fmt.Println("=====================")
			fmt.Printf("Server: %s\n", cfg.ArgoCD.Server)
			fmt.Printf("Insecure: %t\n", cfg.ArgoCD.Insecure)
			fmt.Printf("gRPC-Web: %t\n", cfg.ArgoCD.GRPCWeb)
			if cfg.ArgoCD.GRPCWebRootPath != "" {
				fmt.Printf("gRPC-Web Root Path: %s\n", cfg.ArgoCD.GRPCWebRootPath)
			}
			fmt.Printf("MCP Endpoint: %s\n", cfg.Server.MCPEndpoint)
			fmt.Printf("Structured Output: %t\n", cfg.Server.StructuredOutput)
			switch {
			case cfg.Server.SafeMode:
				fmt.Printf("Mode: read-only (all writes disabled)\n")
			case cfg.Server.AllowDeletes:
				fmt.Printf("Mode: read-write + deletes enabled\n")
			default:
				fmt.Printf("Mode: read-write (deletes disabled)\n")
			}
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
		Long: `Update authentication token for ArgoCD.

Use --sso for Single Sign-On (OIDC) authentication:
  argocd-mcp auth login --sso

Use username/password for basic authentication:
  argocd-mcp auth login -u admin -p password`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.LoadConfig(logger, "")
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}

			// Get flags
			server, _ := cmd.Flags().GetString("server")
			username, _ := cmd.Flags().GetString("username")
			password, _ := cmd.Flags().GetString("password")
			sso, _ := cmd.Flags().GetBool("sso")
			insecure, _ := cmd.Flags().GetBool("insecure")
			plaintext, _ := cmd.Flags().GetBool("plaintext")
			grpcWeb, _ := cmd.Flags().GetBool("grpc-web")
			grpcWebRootPath, _ := cmd.Flags().GetString("grpc-web-root-path")

			// Override config with CLI flags
			if server != "" {
				cfg.ArgoCD.Server = server
			}
			if insecure {
				cfg.ArgoCD.Insecure = insecure
			}
			if plaintext {
				cfg.ArgoCD.PlainText = plaintext
			}
			if grpcWeb {
				cfg.ArgoCD.GRPCWeb = grpcWeb
			}
			if grpcWebRootPath != "" {
				cfg.ArgoCD.GRPCWebRootPath = grpcWebRootPath
			}

			var authToken string
			var authUser string

			if sso {
				// SSO login
				auth.PrintInfo("Starting SSO login to " + cfg.ArgoCD.Server + "...")

				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
				defer cancel()

				result, err := auth.SSOLogin(ctx, logger, auth.SSOLoginRequest{
					Server:          cfg.ArgoCD.Server,
					AuthURL:         cfg.ArgoCD.AuthURL,
					Insecure:        cfg.ArgoCD.Insecure,
					PlainText:       cfg.ArgoCD.PlainText,
					GRPCWeb:         cfg.ArgoCD.GRPCWeb,
					GRPCWebRootPath: cfg.ArgoCD.GRPCWebRootPath,
					SkipVerify:      cfg.ArgoCD.SSOSkipVerify,
				})
				if err != nil {
					return fmt.Errorf("SSO login failed: %w", err)
				}
				authToken = result.Token
				authUser = result.User
			} else {
				// Username/password login
				if username == "" {
					username, _ = cmd.Flags().GetString("username")
				}
				if password == "" {
					password, _ = cmd.Flags().GetString("password")
				}

				// Interactive if no credentials provided
				if username == "" || password == "" {
					fmt.Println("ArgoCD Authentication")
					fmt.Println("======================")

					if username == "" {
						fmt.Print("Username: ")
						fmt.Scanln(&username)
					}
					if password == "" {
						fmt.Print("Password: ")
						fmt.Scanln(&password)
					}
				}

				ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				defer cancel()

				var err error
				authToken, err = auth.GetAuthToken(ctx, logger, cfg.ArgoCD.Server, username, password, cfg.ArgoCD.AuthURL, cfg.ArgoCD.Insecure, cfg.ArgoCD.PlainText, cfg.ArgoCD.GRPCWeb, cfg.ArgoCD.GRPCWebRootPath)
				if err != nil {
					return fmt.Errorf("login failed: %w", err)
				}
				authUser = username
			}

			// Update config with new token
			cfg.ArgoCD.Token = authToken
			if username != "" {
				cfg.ArgoCD.Username = username
			}
			if password != "" {
				cfg.ArgoCD.Password = password
			}

			// Save config
			configDir := filepath.Join(os.Getenv("HOME"), ".config", "argocd-mcp")
			if err := os.MkdirAll(configDir, 0755); err != nil {
				return fmt.Errorf("failed to create config directory: %w", err)
			}

			configPath := filepath.Join(configDir, "config.yaml")
			data, err := yaml.Marshal(cfg)
			if err != nil {
				return fmt.Errorf("failed to marshal config: %w", err)
			}

			if err := os.WriteFile(configPath, data, 0600); err != nil {
				return fmt.Errorf("failed to write config file: %w", err)
			}

			auth.PrintSuccess("Authentication saved to " + configPath)
			auth.PrintInfo("User: " + authUser)
			return nil
		},
	}

	// Add flags to auth login command
	authCmd.Flags().StringP("server", "s", "", "ArgoCD server address")
	authCmd.Flags().StringP("username", "u", "", "Username for authentication")
	authCmd.Flags().StringP("password", "p", "", "Password for authentication")
	authCmd.Flags().StringP("token", "t", "", "Authentication token (alternative to username/password)")
	authCmd.Flags().Bool("sso", false, "Use Single Sign-On (SSO) authentication")
	authCmd.Flags().BoolP("insecure", "k", false, "Skip TLS certificate verification")
	authCmd.Flags().Bool("plaintext", false, "Use HTTP without TLS")
	authCmd.Flags().Bool("grpc-web", false, "Enable gRPC-Web mode")
	authCmd.Flags().String("grpc-web-root-path", "", "Root path for gRPC-Web requests")

	// Test connection command
	testCmd := &cobra.Command{
		Use:   "test",
		Short: "Test connection to ArgoCD",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.LoadConfig(logger, "")
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}

			// Override from CLI flags if set
			if grpcWeb, _ := cmd.Flags().GetBool("grpc-web"); grpcWeb {
				cfg.ArgoCD.GRPCWeb = grpcWeb
			}
			if grpcWebRootPath, _ := cmd.Flags().GetString("grpc-web-root-path"); grpcWebRootPath != "" {
				cfg.ArgoCD.GRPCWebRootPath = grpcWebRootPath
			}

			// Set log level
			logLevel, err := logrus.ParseLevel(cfg.Logging.Level)
			if err != nil {
				logger.Warnf("Invalid log level '%s', using default 'info': %v", cfg.Logging.Level, err)
				logLevel = logrus.InfoLevel
			}
			logger.SetLevel(logLevel)

			auth.PrintInfo(fmt.Sprintf("Connecting to ArgoCD at %s...", cfg.ArgoCD.Server))

			token := cfg.ArgoCD.Token
			var refreshFn func(context.Context) (string, error)
			if cfg.ArgoCD.Username != "" && cfg.ArgoCD.Password != "" {
				argoCDServer := cfg.ArgoCD.Server
				argoCDUsername := cfg.ArgoCD.Username
				argoCDPassword := cfg.ArgoCD.Password
				argoCDAuthURL := cfg.ArgoCD.AuthURL
				argoCDInsecure := cfg.ArgoCD.Insecure
				argoCDPlainText := cfg.ArgoCD.PlainText
				argoCDGRPCWeb := cfg.ArgoCD.GRPCWeb
				argoCDGRPCWebRootPath := cfg.ArgoCD.GRPCWebRootPath
				refreshFn = func(ctx context.Context) (string, error) {
					return auth.GetAuthToken(ctx, logger, argoCDServer, argoCDUsername, argoCDPassword, argoCDAuthURL, argoCDInsecure, argoCDPlainText, argoCDGRPCWeb, argoCDGRPCWebRootPath)
				}
				if token == "" {
					ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
					defer cancel()

					var err error
					token, err = refreshFn(ctx)
					if err != nil {
						return fmt.Errorf("failed to get auth token: %w", err)
					}
				}
			}

			if token == "" {
				return fmt.Errorf("authentication required")
			}

			argoClient, err := client.NewClientWithRefresh(logger, cfg.ArgoCD.Server, token, cfg.ArgoCD.Insecure, cfg.ArgoCD.PlainText, cfg.ArgoCD.CertFile, cfg.ArgoCD.GRPCWeb, cfg.ArgoCD.GRPCWebRootPath, refreshFn)
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

	// Add gRPC-Web flags to testCmd
	testCmd.Flags().Bool("grpc-web", false, "Enable gRPC-Web mode (use when ArgoCD is behind a reverse proxy that doesn't support native gRPC)")
	testCmd.Flags().String("grpc-web-root-path", "", "Root path for gRPC-Web requests (e.g., /argo-cd)")

	// Call command - invoke tools directly from CLI
	callCmd := &cobra.Command{
		Use:   "call <tool-name> [arguments]",
		Short: "Call an MCP tool directly from the command line",
		Long: `Call an MCP tool directly from the command line.

Arguments can be provided as JSON or as key=value pairs.

Examples:
  # Call with JSON argument
  argocd-mcp call get_application '{"name": "searxng"}'

  # Call with key=value pairs
  argocd-mcp call list_applications project=workloads

  # Call with stdin input
  echo '{"name": "searxng"}' | argocd-mcp call get_application -

  # List available tools
  argocd-mcp call --list`,
		Aliases: []string{"exec", "invoke"},
		RunE: func(cmd *cobra.Command, args []string) error {
			listOnly, _ := cmd.Flags().GetBool("list")
			pretty, _ := cmd.Flags().GetBool("pretty")
			output, _ := cmd.Flags().GetString("output")

			// Load config and create client
			cfg, err := config.LoadConfig(logger, "")
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}

			// Override from CLI flags if set
			if grpcWeb, _ := cmd.Flags().GetBool("grpc-web"); grpcWeb {
				cfg.ArgoCD.GRPCWeb = grpcWeb
			}
			if grpcWebRootPath, _ := cmd.Flags().GetString("grpc-web-root-path"); grpcWebRootPath != "" {
				cfg.ArgoCD.GRPCWebRootPath = grpcWebRootPath
			}

			// Set log level
			logLevel, err := logrus.ParseLevel(cfg.Logging.Level)
			if err != nil {
				logger.Warnf("Invalid log level '%s', using default 'info': %v", cfg.Logging.Level, err)
				logLevel = logrus.InfoLevel
			}
			logger.SetLevel(logLevel)

			token := cfg.ArgoCD.Token
			var refreshFn func(context.Context) (string, error)
			if cfg.ArgoCD.Username != "" && cfg.ArgoCD.Password != "" {
				argoCDServer := cfg.ArgoCD.Server
				argoCDUsername := cfg.ArgoCD.Username
				argoCDPassword := cfg.ArgoCD.Password
				argoCDAuthURL := cfg.ArgoCD.AuthURL
				argoCDInsecure := cfg.ArgoCD.Insecure
				argoCDPlainText := cfg.ArgoCD.PlainText
				argoCDGRPCWeb := cfg.ArgoCD.GRPCWeb
				argoCDGRPCWebRootPath := cfg.ArgoCD.GRPCWebRootPath
				refreshFn = func(ctx context.Context) (string, error) {
					return auth.GetAuthToken(ctx, logger, argoCDServer, argoCDUsername, argoCDPassword, argoCDAuthURL, argoCDInsecure, argoCDPlainText, argoCDGRPCWeb, argoCDGRPCWebRootPath)
				}
				if token == "" {
					ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
					defer cancel()

					token, err = refreshFn(ctx)
					if err != nil {
						return fmt.Errorf("failed to get auth token: %w", err)
					}
				}
			}

			if token == "" {
				return fmt.Errorf("authentication required")
			}

			argoClient, err := client.NewClientWithRefresh(logger, cfg.ArgoCD.Server, token, cfg.ArgoCD.Insecure, cfg.ArgoCD.PlainText, cfg.ArgoCD.CertFile, cfg.ArgoCD.GRPCWeb, cfg.ArgoCD.GRPCWebRootPath, refreshFn)
			if err != nil {
				return fmt.Errorf("failed to create client: %w", err)
			}

			toolManager := tools.NewToolManager(argoClient, logger, cfg.Server.SafeMode, cfg.Server.AllowDeletes)

			if listOnly {
				// List all available tools
				serverTools := toolManager.GetServerTools()
				fmt.Println("Available tools:")
				for _, tool := range serverTools {
					fmt.Printf("  %s\n", tool.Tool.Name)
					if tool.Tool.Description != "" {
						fmt.Printf("    %s\n", tool.Tool.Description)
					}
				}
				return nil
			}

			if len(args) < 1 {
				return fmt.Errorf("tool name required. Use --list to see available tools")
			}

			toolName := args[0]

			// Parse arguments
			var arguments map[string]any
			if len(args) > 1 && strings.HasPrefix(args[1], "{") {
				// JSON argument
				if err := json.Unmarshal([]byte(args[1]), &arguments); err != nil {
					return fmt.Errorf("failed to parse JSON argument: %w", err)
				}
			} else if len(args) > 1 {
				// Parse remaining args as key=value pairs
				arguments = make(map[string]any)
				for _, arg := range args[1:] {
					parts := splitOnce(arg, "=")
					if len(parts) == 2 {
						arguments[parts[0]] = parts[1]
					}
				}
			} else if len(args) == 1 && args[0] != "-" {
				// No arguments provided
				arguments = make(map[string]any)
			}

			// Check if reading from stdin
			if len(args) >= 1 && args[0] == "-" {
				decoder := json.NewDecoder(os.Stdin)
				if err := decoder.Decode(&arguments); err != nil {
					return fmt.Errorf("failed to parse JSON from stdin: %w", err)
				}
			}

			// Execute the tool
			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer cancel()

			result, err := toolManager.CallTool(ctx, toolName, arguments)
			if err != nil {
				return fmt.Errorf("tool call failed: %w", err)
			}

			// Output result
			return outputResult(result, output, pretty)
		},
	}

	callCmd.Flags().BoolP("list", "l", false, "List all available tools")
	callCmd.Flags().BoolP("pretty", "p", true, "Pretty-print JSON output")
	callCmd.Flags().StringP("output", "o", "json", "Output format: json or yaml")
	callCmd.Flags().Bool("grpc-web", false, "Enable gRPC-Web mode (use when ArgoCD is behind a reverse proxy that doesn't support native gRPC)")
	callCmd.Flags().String("grpc-web-root-path", "", "Root path for gRPC-Web requests (e.g., /argo-cd)")

	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(serveCmd)
	configCmd.AddCommand(configInitCmd)
	configCmd.AddCommand(configShowCmd)
	rootCmd.AddCommand(configCmd)
	rootCmd.AddCommand(authCmd)
	rootCmd.AddCommand(testCmd)
	rootCmd.AddCommand(callCmd)

	if err := rootCmd.Execute(); err != nil {
		logger.Fatal(err)
	}
}

// startServer starts the MCP server with the given tools
func startServer(ctx context.Context, srv *mcp.Server, serverTools []tools.ServerTool, endpoint, listen string, logger *logrus.Logger) error {
	for _, st := range serverTools {
		srv.AddTool(st.Tool, st.Handler)
	}

	logger.Infof("Starting MCP server with %d tools", len(serverTools))

	kind, warning := resolveEndpoint(endpoint)
	if warning != "" {
		logger.Warn(warning)
	}
	if kind == endpointHTTP {
		return serveHTTP(ctx, srv, listen, logger)
	}

	if err := srv.Run(ctx, &mcp.StdioTransport{}); err != nil {
		return fmt.Errorf("server error: %w", err)
	}

	return nil
}

// Transport kinds returned by resolveEndpoint.
const (
	endpointStdio = "stdio"
	endpointHTTP  = "http"
)

// resolveEndpoint maps a configured endpoint name to a transport kind,
// along with a warning to log if the name was not an exact match.
// Anything unrecognised falls back to stdio, which is the only transport
// an MCP client launching us as a subprocess can use.
func resolveEndpoint(endpoint string) (kind, warning string) {
	switch endpoint {
	case "", endpointStdio:
		return endpointStdio, ""
	case endpointHTTP, "streamable-http":
		return endpointHTTP, ""
	case "sse":
		// The standalone SSE transport is deprecated by the MCP spec in
		// favour of streamable HTTP, which serves SSE responses itself.
		return endpointHTTP, fmt.Sprintf("mcp_endpoint %q is deprecated, serving streamable HTTP instead", endpoint)
	default:
		return endpointStdio, fmt.Sprintf("Unknown endpoint %q, using stdio", endpoint)
	}
}

// serveHTTP serves the MCP server over the streamable HTTP transport,
// shutting the listener down when ctx is cancelled.
func serveHTTP(ctx context.Context, srv *mcp.Server, listen string, logger *logrus.Logger) error {
	handler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return srv }, nil)
	httpSrv := &http.Server{
		Addr:              listen,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := httpSrv.Shutdown(shutdownCtx); err != nil {
			logger.Warnf("HTTP shutdown: %v", err)
		}
	}()

	logger.Infof("Listening for streamable HTTP MCP connections on %s", listen)
	if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("server error: %w", err)
	}

	return nil
}

// splitOnce splits a string at the first occurrence of sep
func splitOnce(s, sep string) []string {
	if idx := findIndex(s, sep); idx >= 0 {
		return []string{s[:idx], s[idx+len(sep):]}
	}
	return []string{s}
}

// findIndex returns the index of the first occurrence of sep in s
func findIndex(s, sep string) int {
	for i := 0; i <= len(s)-len(sep); i++ {
		if s[i:i+len(sep)] == sep {
			return i
		}
	}
	return -1
}

// outputResult prints the tool result in the specified format
func outputResult(result *mcp.CallToolResult, format string, pretty bool) error {
	var data []byte
	var err error

	// Extract content from the result
	output := extractResultContent(result)

	if format == "yaml" {
		data, err = yaml.Marshal(output)
	} else {
		if pretty {
			data, err = json.MarshalIndent(output, "", "  ")
		} else {
			data, err = json.Marshal(output)
		}
	}

	if err != nil {
		return fmt.Errorf("failed to marshal output: %w", err)
	}

	fmt.Println(string(data))
	return nil
}

type toolErrorOutput struct {
	Error bool `json:"error"`
	Text  any  `json:"text"`
}

// extractResultContent extracts the content from an MCP tool result
func extractResultContent(result *mcp.CallToolResult) any {
	if result == nil {
		return nil
	}
	if result.IsError {
		return toolErrorOutput{Error: true, Text: result.Content}
	}
	return result.Content
}
