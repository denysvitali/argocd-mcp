package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/argocd-mcp/argocd-mcp/internal/auth"
	"github.com/argocd-mcp/argocd-mcp/internal/client"
	"github.com/argocd-mcp/argocd-mcp/internal/config"
	"github.com/argocd-mcp/argocd-mcp/tools"
	"github.com/argoproj/argo-cd/v3/pkg/apiclient/application"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
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
				logger.Warnf("Invalid log level '%s', using default 'info': %v", cfg.Logging.Level, err)
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
				token, err = auth.GetAuthToken(ctx, logger, cfg.ArgoCD.Server, cfg.ArgoCD.Username, cfg.ArgoCD.Password, cfg.ArgoCD.AuthURL, cfg.ArgoCD.Insecure, cfg.ArgoCD.PlainText, cfg.ArgoCD.GRPCWeb, cfg.ArgoCD.GRPCWebRootPath)
				if err != nil {
					return fmt.Errorf("failed to get auth token: %w", err)
				}
			}

			if token == "" {
				return fmt.Errorf("authentication required: set token or username/password in config")
			}

			// Create client
			argoClient, err := client.NewClient(logger, cfg.ArgoCD.Server, token, cfg.ArgoCD.Insecure, cfg.ArgoCD.PlainText, cfg.ArgoCD.CertFile, cfg.ArgoCD.GRPCWeb, cfg.ArgoCD.GRPCWebRootPath)
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
					Server:    server,
					Username:  username,
					Password:  password,
					Token:     token,
					Insecure:  insecure,
					PlainText: plaintext,
					CertFile:  certFile,
				},
				Server: config.ServerConfig{
					MCPEndpoint: "stdio",
					SafeMode:    false,
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
	configCmd.Flags().StringP("server", "s", "", "ArgoCD server address (e.g., argocd.example.com:443)")
	configCmd.Flags().StringP("username", "u", "", "Username for authentication")
	configCmd.Flags().StringP("password", "p", "", "Password for authentication")
	configCmd.Flags().StringP("token", "t", "", "Authentication token (alternative to username/password)")
	configCmd.Flags().BoolP("insecure", "k", false, "Skip TLS certificate verification")
	configCmd.Flags().BoolP("plaintext", "", false, "Use HTTP without TLS (for testing only)")
	configCmd.Flags().StringP("cert-file", "c", "", "Path to CA certificate file")

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

			// Set log level
			logLevel, err := logrus.ParseLevel(cfg.Logging.Level)
			if err != nil {
				logger.Warnf("Invalid log level '%s', using default 'info': %v", cfg.Logging.Level, err)
				logLevel = logrus.InfoLevel
			}
			logger.SetLevel(logLevel)

			auth.PrintInfo(fmt.Sprintf("Connecting to ArgoCD at %s...", cfg.ArgoCD.Server))

			token := cfg.ArgoCD.Token
			if token == "" && cfg.ArgoCD.Username != "" && cfg.ArgoCD.Password != "" {
				ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				defer cancel()

				var err error
				token, err = auth.GetAuthToken(ctx, logger, cfg.ArgoCD.Server, cfg.ArgoCD.Username, cfg.ArgoCD.Password, cfg.ArgoCD.AuthURL, cfg.ArgoCD.Insecure, cfg.ArgoCD.PlainText, cfg.ArgoCD.GRPCWeb, cfg.ArgoCD.GRPCWebRootPath)
				if err != nil {
					return fmt.Errorf("failed to get auth token: %w", err)
				}
			}

			if token == "" {
				return fmt.Errorf("authentication required")
			}

			argoClient, err := client.NewClient(logger, cfg.ArgoCD.Server, token, cfg.ArgoCD.Insecure, cfg.ArgoCD.PlainText, cfg.ArgoCD.CertFile, cfg.ArgoCD.GRPCWeb, cfg.ArgoCD.GRPCWebRootPath)
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
			cfg, err := config.LoadConfig(logger)
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}

			// Set log level
			logLevel, err := logrus.ParseLevel(cfg.Logging.Level)
			if err != nil {
				logger.Warnf("Invalid log level '%s', using default 'info': %v", cfg.Logging.Level, err)
				logLevel = logrus.InfoLevel
			}
			logger.SetLevel(logLevel)

			token := cfg.ArgoCD.Token
			if token == "" && cfg.ArgoCD.Username != "" && cfg.ArgoCD.Password != "" {
				ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				defer cancel()

				token, err = auth.GetAuthToken(ctx, logger, cfg.ArgoCD.Server, cfg.ArgoCD.Username, cfg.ArgoCD.Password, cfg.ArgoCD.AuthURL, cfg.ArgoCD.Insecure, cfg.ArgoCD.PlainText, cfg.ArgoCD.GRPCWeb, cfg.ArgoCD.GRPCWebRootPath)
				if err != nil {
					return fmt.Errorf("failed to get auth token: %w", err)
				}
			}

			if token == "" {
				return fmt.Errorf("authentication required")
			}

			argoClient, err := client.NewClient(logger, cfg.ArgoCD.Server, token, cfg.ArgoCD.Insecure, cfg.ArgoCD.PlainText, cfg.ArgoCD.CertFile, cfg.ArgoCD.GRPCWeb, cfg.ArgoCD.GRPCWebRootPath)
			if err != nil {
				return fmt.Errorf("failed to create client: %w", err)
			}

			toolManager := tools.NewToolManager(argoClient, logger, cfg.Server.SafeMode)

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
			var arguments map[string]interface{}
			if len(args) > 1 {
				// Parse remaining args as key=value pairs
				arguments = make(map[string]interface{})
				for _, arg := range args[1:] {
					parts := splitOnce(arg, "=")
					if len(parts) == 2 {
						arguments[parts[0]] = parts[1]
					}
				}
			} else if len(args) == 1 && args[0] != "-" {
				// No arguments provided
				arguments = make(map[string]interface{})
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

	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(serveCmd)
	rootCmd.AddCommand(configCmd)
	rootCmd.AddCommand(configShowCmd)
	rootCmd.AddCommand(authCmd)
	rootCmd.AddCommand(testCmd)
	rootCmd.AddCommand(callCmd)

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

// extractResultContent extracts the content from an MCP tool result
func extractResultContent(result *mcp.CallToolResult) interface{} {
	if result == nil {
		return nil
	}

	// Check if there's an error
	if result.IsError {
		return map[string]interface{}{
			"error": true,
			"text":  result.Content,
		}
	}

	// Return the content as-is (it should already be parsed as interface{})
	return result.Content
}
