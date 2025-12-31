package config

import (
	"fmt"
	"strings"

	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
)

type Config struct {
	ArgoCD  ArgoCDConfig  `mapstructure:"argocd"`
	Server  ServerConfig  `mapstructure:"server"`
	Logging LoggingConfig `mapstructure:"logging"`
}

type ArgoCDConfig struct {
	Server   string `mapstructure:"server"`
	AuthURL  string `mapstructure:"auth_url"`
	Username string `mapstructure:"username"`
	Password string `mapstructure:"password"`
	Token    string `mapstructure:"token"`
	Insecure bool   `mapstructure:"insecure"`
	CertFile string `mapstructure:"cert_file"`
}

type ServerConfig struct {
	MCPEndpoint string `mapstructure:"mcp_endpoint"`
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

	return &cfg, nil
}
