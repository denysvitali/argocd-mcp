//go:build integration
// +build integration

package tools

import (
	"context"
	"encoding/json"
	"math"
	"testing"
	"time"

	"github.com/argocd-mcp/argocd-mcp/internal/auth"
	"github.com/argocd-mcp/argocd-mcp/internal/client"
	"github.com/argocd-mcp/argocd-mcp/internal/config"
	"github.com/argoproj/argo-cd/v3/pkg/apiclient/application"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ToolTestResult represents the structure of tool results for testing
type ToolTestResult struct {
	Items json.RawMessage `json:"items"`
	Count int             `json:"count"`
}

// ToolTestItem represents a generic item structure
type ToolTestItem struct {
	Name string `json:"name"`
}

// ApplicationDetail represents application details
type ApplicationDetail struct {
	Name       string `json:"name"`
	Project    string `json:"project"`
	RepoURL    string `json:"repo_url"`
	Path       string `json:"path"`
	Server     string `json:"server"`
	NS         string `json:"namespace"`
	Status     string `json:"status"`
	Health     string `json:"health"`
	SyncStatus struct {
		Revision string `json:"revision"`
	} `json:"sync_status"`
}

// ProjectDetail represents project details
type ProjectDetail struct {
	Name         string `json:"name"`
	Description  string `json:"description"`
	Destinations []struct {
		Server    string `json:"server"`
		Namespace string `json:"namespace"`
	} `json:"destinations"`
}

// ClusterDetail represents cluster details
type ClusterDetail struct {
	Server    string         `json:"server"`
	Name      string         `json:"name"`
	ConnState map[string]any `json:"connection_state"`
}

// ManifestsResult represents manifests result
type ManifestsResult struct {
	Manifests []json.RawMessage `json:"manifests"`
	Count     int               `json:"count"`
}

// EventsResult represents events result
type EventsResult struct {
	Items []map[string]string `json:"items"`
	Total int                 `json:"total"`
}

// getTextContent extracts text content from a result
func getTextContent(result *mcp.CallToolResult) string {
	if len(result.Content) == 0 {
		return ""
	}
	if tc, ok := result.Content[0].(mcp.TextContent); ok {
		return tc.Text
	}
	return ""
}

// TestReadOnlyToolsIntegration tests all read-only tools against a real ArgoCD instance
func TestReadOnlyToolsIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	cfg, err := config.LoadConfig(logger)
	require.NoError(t, err, "Failed to load config")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Get auth token
	token := cfg.ArgoCD.Token
	if token == "" && cfg.ArgoCD.Username != "" && cfg.ArgoCD.Password != "" {
		token, err = auth.GetAuthToken(ctx, logger, cfg.ArgoCD.Server, cfg.ArgoCD.Username, cfg.ArgoCD.Password, cfg.ArgoCD.AuthURL, cfg.ArgoCD.Insecure, cfg.ArgoCD.PlainText)
		require.NoError(t, err, "Failed to get auth token")
	}

	if token == "" {
		t.Skip("No auth token available, skipping integration test")
	}

	argocdClient, err := client.NewClient(logger, cfg.ArgoCD.Server, token, cfg.ArgoCD.Insecure, cfg.ArgoCD.PlainText, cfg.ArgoCD.CertFile)
	require.NoError(t, err, "Failed to create ArgoCD client")

	tm := NewToolManager(argocdClient, logger, true)

	t.Run("list_applications", func(t *testing.T) {
		result, err := tm.handleListApplications(ctx, make(map[string]any))
		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.False(t, result.IsError, "Result should not be an error: %s", getTextContent(result))
	})

	// First get an application name to use in subsequent tests
	var appName string
	t.Run("get_application_name_for_tests", func(t *testing.T) {
		result, err := tm.handleListApplications(ctx, make(map[string]any))
		require.NoError(t, err)
		require.False(t, result.IsError, "Failed to list applications: %s", getTextContent(result))

		var listResult ToolTestResult
		err = json.Unmarshal([]byte(getTextContent(result)), &listResult)
		require.NoError(t, err)

		var items []ToolTestItem
		err = json.Unmarshal(listResult.Items, &items)
		require.NoError(t, err)

		if len(items) > 0 {
			appName = items[0].Name
		}
	})

	if appName == "" {
		t.Skip("No applications found in ArgoCD, skipping application-specific tests")
	}

	t.Run("get_application", func(t *testing.T) {
		args := make(map[string]any)
		args["name"] = appName
		result, err := tm.handleGetApplication(ctx, args)
		require.NoError(t, err)
		assert.False(t, result.IsError, "Result should not be an error: %s", getTextContent(result))

		var appDetail ApplicationDetail
		err = json.Unmarshal([]byte(getTextContent(result)), &appDetail)
		require.NoError(t, err)
		assert.Equal(t, appName, appDetail.Name)
	})

	t.Run("get_application_manifests", func(t *testing.T) {
		args := make(map[string]any)
		args["name"] = appName
		result, err := tm.handleGetApplicationManifests(ctx, args)
		require.NoError(t, err)
		assert.False(t, result.IsError, "Result should not be an error: %s", getTextContent(result))

		var manifests ManifestsResult
		err = json.Unmarshal([]byte(getTextContent(result)), &manifests)
		require.NoError(t, err)
		assert.Greater(t, manifests.Count, 0, "Should have at least one manifest")
	})

	t.Run("get_application_events", func(t *testing.T) {
		args := make(map[string]any)
		args["name"] = appName
		result, err := tm.handleGetApplicationEvents(ctx, args)
		require.NoError(t, err)
		assert.False(t, result.IsError, "Result should not be an error: %s", getTextContent(result))

		var events EventsResult
		err = json.Unmarshal([]byte(getTextContent(result)), &events)
		require.NoError(t, err)
		assert.NotNil(t, events.Items)
	})

	t.Run("get_application_diff", func(t *testing.T) {
		args := make(map[string]any)
		args["name"] = appName
		result, err := tm.handleGetApplicationDiff(ctx, args)
		require.NoError(t, err)
		assert.False(t, result.IsError, "Result should not be an error: %s", getTextContent(result))

		var diffResult map[string]any
		err = json.Unmarshal([]byte(getTextContent(result)), &diffResult)
		require.NoError(t, err)

		t.Logf("Diff result: %+v", diffResult)

		// Check that we have the expected structure
		assert.Contains(t, diffResult, "application")
		assert.Contains(t, diffResult, "out_of_sync")
		assert.Contains(t, diffResult, "synced")
		assert.Contains(t, diffResult, "total")

		outOfSync := diffResult["out_of_sync"].([]any)
		t.Logf("Out of sync count: %d", len(outOfSync))
		for i, r := range outOfSync {
			resource := r.(map[string]any)
			t.Logf("Resource %d: %s/%s/%s", i, resource["group"], resource["kind"], resource["name"])
			t.Logf("  status: %s", resource["status"])
			if target, ok := resource["target"].(string); ok {
				t.Logf("  target (first 500 chars): %s", target[:int(math.Min(500, float64(len(target))))])
			}
			if live, ok := resource["live"].(string); ok {
				t.Logf("  live (first 500 chars): %s", live[:int(math.Min(500, float64(len(live))))])
			}
			if diff, ok := resource["diff"].(string); ok {
				t.Logf("  diff:\n%s", diff)
			}
		}
	})

	t.Run("list_projects", func(t *testing.T) {
		result, err := tm.handleListProjects(ctx, make(map[string]any))
		require.NoError(t, err)
		assert.False(t, result.IsError, "Result should not be an error: %s", getTextContent(result))

		var listResult ToolTestResult
		err = json.Unmarshal([]byte(getTextContent(result)), &listResult)
		require.NoError(t, err)

		var items []ToolTestItem
		err = json.Unmarshal(listResult.Items, &items)
		require.NoError(t, err)
		assert.Greater(t, len(items), 0, "Should have at least one project")
	})

	// Get a project name for subsequent tests
	var projectName string
	t.Run("get_project_name_for_tests", func(t *testing.T) {
		result, err := tm.handleListProjects(ctx, make(map[string]any))
		require.NoError(t, err)

		var listResult ToolTestResult
		err = json.Unmarshal([]byte(getTextContent(result)), &listResult)
		require.NoError(t, err)

		var items []ToolTestItem
		err = json.Unmarshal(listResult.Items, &items)
		require.NoError(t, err)

		if len(items) > 0 {
			projectName = items[0].Name
		}
	})

	if projectName != "" {
		t.Run("get_project", func(t *testing.T) {
			args := make(map[string]any)
			args["name"] = projectName
			result, err := tm.handleGetProject(ctx, args)
			require.NoError(t, err)
			assert.False(t, result.IsError, "Result should not be an error: %s", getTextContent(result))

			var projectDetail ProjectDetail
			err = json.Unmarshal([]byte(getTextContent(result)), &projectDetail)
			require.NoError(t, err)
			assert.Equal(t, projectName, projectDetail.Name)
		})

		t.Run("get_project_events", func(t *testing.T) {
			args := make(map[string]any)
			args["name"] = projectName
			result, err := tm.handleGetProjectEvents(ctx, args)
			require.NoError(t, err)
			assert.False(t, result.IsError, "Result should not be an error: %s", getTextContent(result))

			var events map[string]any
			err = json.Unmarshal([]byte(getTextContent(result)), &events)
			require.NoError(t, err)
			assert.NotNil(t, events)
		})
	}

	t.Run("list_repositories", func(t *testing.T) {
		result, err := tm.handleListRepositories(ctx, make(map[string]any))
		require.NoError(t, err)
		assert.False(t, result.IsError, "Result should not be an error: %s", getTextContent(result))

		var listResult ToolTestResult
		err = json.Unmarshal([]byte(getTextContent(result)), &listResult)
		require.NoError(t, err)
		assert.NotNil(t, listResult.Items)
	})

	t.Run("list_clusters", func(t *testing.T) {
		result, err := tm.handleListClusters(ctx, make(map[string]any))
		require.NoError(t, err)
		assert.False(t, result.IsError, "Result should not be an error: %s", getTextContent(result))

		var listResult ToolTestResult
		err = json.Unmarshal([]byte(getTextContent(result)), &listResult)
		require.NoError(t, err)

		var items []ToolTestItem
		err = json.Unmarshal(listResult.Items, &items)
		require.NoError(t, err)
		assert.Greater(t, len(items), 0, "Should have at least one cluster")
	})

	// Get cluster server for subsequent tests
	var clusterServer string
	t.Run("get_cluster_name_for_tests", func(t *testing.T) {
		result, err := tm.handleListClusters(ctx, make(map[string]any))
		require.NoError(t, err)

		var listResult ToolTestResult
		err = json.Unmarshal([]byte(getTextContent(result)), &listResult)
		require.NoError(t, err)

		type clusterItem struct {
			Server string `json:"server"`
		}
		var items []clusterItem
		err = json.Unmarshal(listResult.Items, &items)
		require.NoError(t, err)

		if len(items) > 0 {
			clusterServer = items[0].Server
		}
	})

	if clusterServer != "" {
		t.Run("get_cluster", func(t *testing.T) {
			args := make(map[string]any)
			args["server"] = clusterServer
			result, err := tm.handleGetCluster(ctx, args)
			require.NoError(t, err)
			assert.False(t, result.IsError, "Result should not be an error: %s", getTextContent(result))

			var clusterDetail ClusterDetail
			err = json.Unmarshal([]byte(getTextContent(result)), &clusterDetail)
			require.NoError(t, err)
			assert.Equal(t, clusterServer, clusterDetail.Server)
		})
	}
}

// TestParseEventsIntegration tests the parseEvents function with real data
func TestParseEventsIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	cfg, err := config.LoadConfig(logger)
	require.NoError(t, err, "Failed to load config")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Get auth token
	token := cfg.ArgoCD.Token
	if token == "" && cfg.ArgoCD.Username != "" && cfg.ArgoCD.Password != "" {
		token, err = auth.GetAuthToken(ctx, logger, cfg.ArgoCD.Server, cfg.ArgoCD.Username, cfg.ArgoCD.Password, cfg.ArgoCD.AuthURL, cfg.ArgoCD.Insecure, cfg.ArgoCD.PlainText)
		require.NoError(t, err, "Failed to get auth token")
	}

	if token == "" {
		t.Skip("No auth token available, skipping integration test")
	}

	argocdClient, err := client.NewClient(logger, cfg.ArgoCD.Server, token, cfg.ArgoCD.Insecure, cfg.ArgoCD.PlainText, cfg.ArgoCD.CertFile)
	require.NoError(t, err, "Failed to create ArgoCD client")

	// First get an application name
	query := &application.ApplicationQuery{}
	apps, err := argocdClient.ListApplications(ctx, query)
	require.NoError(t, err)

	if len(apps.Items) == 0 {
		t.Skip("No applications found, skipping parseEvents test")
	}

	appName := apps.Items[0].Name

	// Get events to test parseEvents
	eventsQuery := &application.ApplicationResourceEventsQuery{
		Name: &appName,
	}

	eventsRaw, err := argocdClient.GetApplicationEvents(ctx, eventsQuery)
	require.NoError(t, err, "GetApplicationEvents should not error")

	// Test parseEvents with the real data
	parsedEvents, err := parseEvents(eventsRaw)
	require.NoError(t, err, "parseEvents should not error with real EventList data")
	assert.NotNil(t, parsedEvents, "parsedEvents should not be nil")
}
