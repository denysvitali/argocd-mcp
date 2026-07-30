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
	err := os.WriteFile(configPath, []byte(configContent), 0o644)
	require.NoError(t, err)

	logger := logrus.New()
	logger.SetLevel(logrus.DebugLevel)

	t.Run("load valid config", func(t *testing.T) {
		// Override HOME to prevent loading the user's real global config.
		t.Setenv("HOME", t.TempDir())

		cfg, err := LoadConfig(logger, configPath)
		require.NoError(t, err)
		assert.Equal(t, "argocd.example.com", cfg.ArgoCD.Server)
		assert.Equal(t, "admin", cfg.ArgoCD.Username)
		assert.Equal(t, "secret", cfg.ArgoCD.Password)
		assert.Equal(t, "stdio", cfg.Server.MCPEndpoint)
		assert.Equal(t, "info", cfg.Logging.Level)
	})

	t.Run("defaults are applied", func(t *testing.T) {
		minConfigContent := `
argocd:
  server: "argocd.example.com"
`
		require.NoError(t, os.WriteFile(configPath, []byte(minConfigContent), 0o644))

		// Override HOME to prevent loading the user's real global config.
		t.Setenv("HOME", t.TempDir())

		cfg, err := LoadConfig(logger, configPath)
		require.NoError(t, err)
		assert.Equal(t, "info", cfg.Logging.Level)
		assert.Equal(t, "stdio", cfg.Server.MCPEndpoint)
	})
}

func TestLoadConfig_DefaultValues(t *testing.T) {
	logger := logrus.New()

	// Override HOME to a fresh temp dir with no config files so all
	// defaults are exercised and the user's real global config does not
	// leak in.
	t.Setenv("HOME", t.TempDir())

	cfg, err := LoadConfig(logger, "")
	require.NoError(t, err)

	assert.Equal(t, "localhost:8080", cfg.ArgoCD.Server)
	assert.False(t, cfg.ArgoCD.Insecure)
	assert.Equal(t, "stdio", cfg.Server.MCPEndpoint)
	assert.True(t, cfg.Server.SafeMode)
	assert.Equal(t, "info", cfg.Logging.Level)
	assert.Equal(t, "json", cfg.Logging.Format)
}

func TestLoadConfig_InvalidYAML(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.yaml")

	invalidContent := `
argocd:
  server: "argocd.example.com"
  invalid_indent: this is invalid
    nested: broken
`
	err := os.WriteFile(configPath, []byte(invalidContent), 0o644)
	require.NoError(t, err)

	logger := logrus.New()
	t.Setenv("HOME", t.TempDir())

	// viper may or may not return an error for partially-invalid YAML, but
	// LoadConfig must never panic regardless.
	assert.NotPanics(t, func() {
		_, _ = LoadConfig(logger, configPath)
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
	err := os.WriteFile(configPath, []byte(configContent), 0o644)
	require.NoError(t, err)

	t.Run("env vars override config", func(t *testing.T) {
		t.Setenv("ARGOCD_MCP_ARGOCD_USERNAME", "env-admin")
		t.Setenv("ARGOCD_MCP_ARGOCD_PASSWORD", "env-secret")
		t.Setenv("HOME", t.TempDir())

		logger := logrus.New()

		cfg, err := LoadConfig(logger, configPath)
		require.NoError(t, err)
		assert.Equal(t, "env-admin", cfg.ArgoCD.Username)
		assert.Equal(t, "env-secret", cfg.ArgoCD.Password)
	})

	// Regression: with no config file at all, viper's AutomaticEnv only
	// resolves keys it already knows about, so ARGOCD_MCP_ARGOCD_TOKEN was
	// silently dropped and the server refused to start with
	// "authentication required" despite the documented env var being set.
	t.Run("env vars work without a config file", func(t *testing.T) {
		t.Setenv("HOME", t.TempDir())
		t.Setenv("ARGOCD_MCP_ARGOCD_SERVER", "env.example.com:443")
		t.Setenv("ARGOCD_MCP_ARGOCD_TOKEN", "env-token")

		cfg, err := LoadConfig(logrus.New(), "")
		require.NoError(t, err)
		assert.Equal(t, "env.example.com:443", cfg.ArgoCD.Server)
		assert.Equal(t, "env-token", cfg.ArgoCD.Token)
	})
}

// TestLoadConfig_IgnoresCwdConfig verifies that a config.yaml in the
// current working directory is NOT picked up by the default search path.
// This prevents running argocd-mcp from inside another project (e.g. one
// with its own config.yaml) from silently shadowing the global config in
// ~/.config/argocd-mcp/config.yaml.
func TestLoadConfig_IgnoresCwdConfig(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	origDir, err := os.Getwd()
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.Chdir(origDir) })

	// A foreign config.yaml in cwd — must NOT be loaded.
	cwd := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(cwd, "config.yaml"), []byte("argocd:\n  server: \"from-cwd\"\n"), 0o644))
	require.NoError(t, os.Chdir(cwd))

	// The global config that must win.
	globalDir := filepath.Join(tmpHome, ".config", "argocd-mcp")
	require.NoError(t, os.MkdirAll(globalDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(globalDir, "config.yaml"), []byte("argocd:\n  server: \"from-global\"\n"), 0o644))

	logger := logrus.New()
	cfg, err := LoadConfig(logger, "")
	require.NoError(t, err)
	assert.Equal(t, "from-global", cfg.ArgoCD.Server, "server should come from the global config, not from cwd")
}
