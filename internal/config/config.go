package config

import (
	"fmt"
	"strings"

	"github.com/argoproj/argo-cd/v3/util/localconfig"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
)

type Config struct {
	ArgoCD  ArgoCDConfig  `mapstructure:"argocd"`
	Server  ServerConfig  `mapstructure:"server"`
	Logging LoggingConfig `mapstructure:"logging"`
}

type ArgoCDConfig struct {
	Server          string `mapstructure:"server"`
	AuthURL         string `mapstructure:"auth_url"`
	Username        string `mapstructure:"username"`
	Password        string `mapstructure:"password"`
	Token           string `mapstructure:"token"`
	Insecure        bool   `mapstructure:"insecure"`
	PlainText       bool   `mapstructure:"plaintext"`
	CertFile        string `mapstructure:"cert_file"`
	GRPCWeb         bool   `mapstructure:"grpc_web"`
	GRPCWebRootPath string `mapstructure:"grpc_web_root_path"`
	SSOSkipVerify   bool   `mapstructure:"sso_skip_verify"`
}

type ServerConfig struct {
	MCPEndpoint  string `mapstructure:"mcp_endpoint"`
	SafeMode     bool   `mapstructure:"safe_mode"`
	AllowDeletes bool   `mapstructure:"allow_deletes"`
}

type LoggingConfig struct {
	Level  string `mapstructure:"level"`
	Format string `mapstructure:"format"`
}

func LoadConfig(logger *logrus.Logger) (*Config, error) {
	v := viper.New()

	// Set defaults
	v.SetDefault("argocd.server", "localhost:8080")
	v.SetDefault("argocd.insecure", false)
	v.SetDefault("server.mcp_endpoint", "stdio")
	v.SetDefault("server.safe_mode", true)
	v.SetDefault("server.allow_deletes", false)
	v.SetDefault("logging.level", "info")
	v.SetDefault("logging.format", "json")

	// Environment variable prefix
	v.SetEnvPrefix("ARGOCD_MCP")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	// Config file support
	v.AddConfigPath("$HOME/.config/argocd-mcp")
	v.AddConfigPath(".")
	v.SetConfigName("config")
	v.SetConfigType("yaml")

	// Try to read config file
	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			logger.Warnf("Error reading config file: %v", err)
		}
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	// CLI flags override config file
	if server := v.GetString("server"); server != "" {
		cfg.ArgoCD.Server = server
	}
	if token := v.GetString("token"); token != "" {
		cfg.ArgoCD.Token = token
	}
	if grpcWeb := v.GetBool("grpc-web"); grpcWeb {
		cfg.ArgoCD.GRPCWeb = grpcWeb
	}
	if grpcWebRootPath := v.GetString("grpc-web-root-path"); grpcWebRootPath != "" {
		cfg.ArgoCD.GRPCWebRootPath = grpcWebRootPath
	}

	// Fallback: read token (and server) from native argocd CLI config (~/.config/argocd/config)
	if cfg.ArgoCD.Token == "" {
		if err := applyNativeArgocdConfig(logger, &cfg); err != nil {
			logger.Debugf("Could not read native argocd config: %v", err)
		}
	}

	return &cfg, nil
}

// applyNativeArgocdConfig reads the native argocd CLI config and applies the
// token (and optionally server/insecure) to cfg if they are not already set.
func applyNativeArgocdConfig(logger *logrus.Logger, cfg *Config) error {
	path, err := localconfig.DefaultLocalConfigPath()
	if err != nil {
		return fmt.Errorf("get native argocd config path: %w", err)
	}

	lc, err := localconfig.ReadLocalConfig(path)
	if err != nil {
		return fmt.Errorf("read native argocd config: %w", err)
	}
	if lc == nil {
		return nil
	}

	ctx, err := lc.ResolveContext(lc.CurrentContext)
	if err != nil {
		return fmt.Errorf("resolve argocd context: %w", err)
	}

	if ctx.User.AuthToken == "" {
		return nil
	}

	logger.Debugf("Using token from native argocd config (context: %s)", lc.CurrentContext)
	cfg.ArgoCD.Token = ctx.User.AuthToken

	if cfg.ArgoCD.Server == "" || cfg.ArgoCD.Server == "localhost:8080" {
		cfg.ArgoCD.Server = ctx.Server.Server
		cfg.ArgoCD.Insecure = ctx.Server.Insecure
		cfg.ArgoCD.GRPCWeb = ctx.Server.GRPCWeb
	}

	return nil
}
