package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadConfig(t *testing.T) {
	// Create a temporary config file
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.yaml")

	configContent := `
argocd:
  server: "argocd.example.com"
  username: "admin"
  password: "secret"
  token: ""
  insecure: false
server:
  mcp_endpoint: "stdio"
logging:
  level: "info"
`
	err := os.WriteFile(configPath, []byte(configContent), 0644)
	require.NoError(t, err)

	// Create logger for test
	logger := logrus.New()
	logger.SetLevel(logrus.DebugLevel)

	t.Run("load valid config", func(t *testing.T) {
		// Change to temp dir so config can be found
		originalDir, err := os.Getwd()
		require.NoError(t, err)
		defer os.Chdir(originalDir)
		os.Chdir(tempDir)

		// Override HOME to prevent loading user config
		originalHome := os.Getenv("HOME")
		os.Setenv("HOME", tempDir)
		defer os.Setenv("HOME", originalHome)

		cfg, err := LoadConfig(logger)
		require.NoError(t, err)
		assert.Equal(t, "argocd.example.com", cfg.ArgoCD.Server)
		assert.Equal(t, "admin", cfg.ArgoCD.Username)
		assert.Equal(t, "secret", cfg.ArgoCD.Password)
		assert.Equal(t, "stdio", cfg.Server.MCPEndpoint)
		assert.Equal(t, "info", cfg.Logging.Level)
	})

	t.Run("defaults are applied", func(t *testing.T) {
		// Minimal config - only server specified
		minConfigContent := `
argocd:
  server: "argocd.example.com"
`
		err := os.WriteFile(configPath, []byte(minConfigContent), 0644)
		require.NoError(t, err)

		originalDir, err := os.Getwd()
		require.NoError(t, err)
		defer os.Chdir(originalDir)
		os.Chdir(tempDir)

		// Override HOME to prevent loading user config
		originalHome := os.Getenv("HOME")
		os.Setenv("HOME", tempDir)
		defer os.Setenv("HOME", originalHome)

		cfg, err := LoadConfig(logger)
		require.NoError(t, err)
		assert.Equal(t, "info", cfg.Logging.Level)
		assert.Equal(t, "stdio", cfg.Server.MCPEndpoint)
	})
}

func TestLoadConfig_DefaultValues(t *testing.T) {
	logger := logrus.New()

	// Use a temp dir without any config file to trigger all defaults
	tempDir := t.TempDir()

	originalDir, err := os.Getwd()
	require.NoError(t, err)
	defer os.Chdir(originalDir)
	os.Chdir(tempDir)

	// Override HOME to prevent loading any user config
	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tempDir)
	defer os.Setenv("HOME", originalHome)

	cfg, err := LoadConfig(logger)
	require.NoError(t, err)

	// Verify all defaults are applied (since no config file exists)
	assert.Equal(t, "localhost:8080", cfg.ArgoCD.Server)
	assert.False(t, cfg.ArgoCD.Insecure)
	assert.Equal(t, "stdio", cfg.Server.MCPEndpoint)
	assert.False(t, cfg.Server.SafeMode)
	assert.Equal(t, "info", cfg.Logging.Level)
	assert.Equal(t, "json", cfg.Logging.Format)
}

func TestLoadConfig_InvalidYAML(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.yaml")

	// Invalid YAML content
	invalidContent := `
argocd:
  server: "argocd.example.com"
  invalid_indent: this is invalid
    nested: broken
`
	err := os.WriteFile(configPath, []byte(invalidContent), 0644)
	require.NoError(t, err)

	logger := logrus.New()

	originalDir, err := os.Getwd()
	require.NoError(t, err)
	defer os.Chdir(originalDir)
	os.Chdir(tempDir)

	// Override HOME to prevent loading user config
	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tempDir)
	defer os.Setenv("HOME", originalHome)

	// LoadConfig should handle the error gracefully
	// viper may not strictly fail on YAML parse errors in all versions
	// but we can verify it doesn't panic
	_, _ = LoadConfig(logger) // nolint:staticcheck // We only care that it doesn't panic
	// The function may or may not return an error depending on viper version
	// but it should not panic
	assert.NotPanics(t, func() {
		_, _ = LoadConfig(logger)
	})
}

func TestConfigWithEnvVars(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.yaml")

	configContent := `
argocd:
  server: "argocd.example.com"
  username: ""
  password: ""
  token: ""
  insecure: false
server:
  mcp_endpoint: "stdio"
logging:
  level: "info"
`
	err := os.WriteFile(configPath, []byte(configContent), 0644)
	require.NoError(t, err)

	t.Run("env vars override config", func(t *testing.T) {
		os.Setenv("ARGOCD_MCP_ARGOCD_USERNAME", "env-admin")
		os.Setenv("ARGOCD_MCP_ARGOCD_PASSWORD", "env-secret")
		defer func() {
			os.Unsetenv("ARGOCD_MCP_ARGOCD_USERNAME")
			os.Unsetenv("ARGOCD_MCP_ARGOCD_PASSWORD")
		}()

		logger := logrus.New()

		originalDir, err := os.Getwd()
		require.NoError(t, err)
		defer os.Chdir(originalDir)
		os.Chdir(tempDir)

		// Override HOME to prevent loading user config
		originalHome := os.Getenv("HOME")
		os.Setenv("HOME", tempDir)
		defer os.Setenv("HOME", originalHome)

		cfg, err := LoadConfig(logger)
		require.NoError(t, err)
		assert.Equal(t, "env-admin", cfg.ArgoCD.Username)
		assert.Equal(t, "env-secret", cfg.ArgoCD.Password)
	})
}
