package tools

import (
	"context"
	"fmt"
	"testing"

	"github.com/argoproj/argo-cd/v3/pkg/apiclient/application"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandleGetApplicationRevisionDiff(t *testing.T) {
	t.Run("success with modified resources", func(t *testing.T) {
		mock := &MockArgoClient{
			GetApplicationManifestsFn: func(_ context.Context, query *application.ApplicationManifestQuery) ([]string, error) {
				if query.Revision != nil && *query.Revision == "rev-a" {
					return []string{
						`{"apiVersion":"v1","kind":"ConfigMap","metadata":{"name":"cm1","namespace":"default"},"data":{"key":"old-value"}}`,
					}, nil
				}
				if query.Revision != nil && *query.Revision == "rev-b" {
					return []string{
						`{"apiVersion":"v1","kind":"ConfigMap","metadata":{"name":"cm1","namespace":"default"},"data":{"key":"new-value"}}`,
					}, nil
				}
				return nil, fmt.Errorf("unexpected revision")
			},
		}
		tm := testToolManager(mock, false, false)
		result, err := tm.CallTool(context.Background(), "get_application_revision_diff", map[string]interface{}{
			"name":        "myapp",
			"revision_a":  "rev-a",
			"revision_b":  "rev-b",
		})
		require.NoError(t, err)
		assert.False(t, result.IsError)
		data := parseResultYAML(t, result)
		assert.Equal(t, "myapp", data["application"])
		assert.Equal(t, "rev-a", data["revision_a"])
		assert.Equal(t, "rev-b", data["revision_b"])

		summary, ok := data["summary"].(map[string]interface{})
		require.True(t, ok)
		assert.Equal(t, float64(1), summary["modified"])
		assert.Equal(t, float64(0), summary["added"])
		assert.Equal(t, float64(0), summary["removed"])
	})

	t.Run("success with added resources", func(t *testing.T) {
		mock := &MockArgoClient{
			GetApplicationManifestsFn: func(_ context.Context, query *application.ApplicationManifestQuery) ([]string, error) {
				if query.Revision != nil && *query.Revision == "rev-a" {
					return []string{}, nil
				}
				if query.Revision != nil && *query.Revision == "rev-b" {
					return []string{
						`{"apiVersion":"v1","kind":"ConfigMap","metadata":{"name":"cm1","namespace":"default"},"data":{"key":"value"}}`,
					}, nil
				}
				return nil, fmt.Errorf("unexpected revision")
			},
		}
		tm := testToolManager(mock, false, false)
		result, err := tm.CallTool(context.Background(), "get_application_revision_diff", map[string]interface{}{
			"name":        "myapp",
			"revision_a":  "rev-a",
			"revision_b":  "rev-b",
		})
		require.NoError(t, err)
		assert.False(t, result.IsError)
		data := parseResultYAML(t, result)
		summary, ok := data["summary"].(map[string]interface{})
		require.True(t, ok)
		assert.Equal(t, float64(1), summary["added"])
	})

	t.Run("success with removed resources", func(t *testing.T) {
		mock := &MockArgoClient{
			GetApplicationManifestsFn: func(_ context.Context, query *application.ApplicationManifestQuery) ([]string, error) {
				if query.Revision != nil && *query.Revision == "rev-a" {
					return []string{
						`{"apiVersion":"v1","kind":"ConfigMap","metadata":{"name":"cm1","namespace":"default"},"data":{"key":"value"}}`,
					}, nil
				}
				if query.Revision != nil && *query.Revision == "rev-b" {
					return []string{}, nil
				}
				return nil, fmt.Errorf("unexpected revision")
			},
		}
		tm := testToolManager(mock, false, false)
		result, err := tm.CallTool(context.Background(), "get_application_revision_diff", map[string]interface{}{
			"name":        "myapp",
			"revision_a":  "rev-a",
			"revision_b":  "rev-b",
		})
		require.NoError(t, err)
		assert.False(t, result.IsError)
		data := parseResultYAML(t, result)
		summary, ok := data["summary"].(map[string]interface{})
		require.True(t, ok)
		assert.Equal(t, float64(1), summary["removed"])
	})

	t.Run("success with unchanged resources", func(t *testing.T) {
		manifest := `{"apiVersion":"v1","kind":"ConfigMap","metadata":{"name":"cm1","namespace":"default"},"data":{"key":"value"}}`
		mock := &MockArgoClient{
			GetApplicationManifestsFn: func(_ context.Context, _ *application.ApplicationManifestQuery) ([]string, error) {
				return []string{manifest}, nil
			},
		}
		tm := testToolManager(mock, false, false)
		result, err := tm.CallTool(context.Background(), "get_application_revision_diff", map[string]interface{}{
			"name":        "myapp",
			"revision_a":  "rev-a",
			"revision_b":  "rev-b",
		})
		require.NoError(t, err)
		assert.False(t, result.IsError)
		data := parseResultYAML(t, result)
		summary, ok := data["summary"].(map[string]interface{})
		require.True(t, ok)
		assert.Equal(t, float64(1), summary["unchanged"])
		assert.Equal(t, float64(0), summary["modified"])
	})

	t.Run("error fetching revision_a", func(t *testing.T) {
		mock := &MockArgoClient{
			GetApplicationManifestsFn: func(_ context.Context, _ *application.ApplicationManifestQuery) ([]string, error) {
				return nil, fmt.Errorf("not found")
			},
		}
		tm := testToolManager(mock, false, false)
		result, err := tm.CallTool(context.Background(), "get_application_revision_diff", map[string]interface{}{
			"name":        "myapp",
			"revision_a":  "rev-a",
			"revision_b":  "rev-b",
		})
		require.NoError(t, err)
		assert.True(t, result.IsError)
	})

	t.Run("error fetching revision_b", func(t *testing.T) {
		mock := &MockArgoClient{
			GetApplicationManifestsFn: func(_ context.Context, query *application.ApplicationManifestQuery) ([]string, error) {
				if query.Revision != nil && *query.Revision == "rev-a" {
					return []string{}, nil
				}
				return nil, fmt.Errorf("not found")
			},
		}
		tm := testToolManager(mock, false, false)
		result, err := tm.CallTool(context.Background(), "get_application_revision_diff", map[string]interface{}{
			"name":        "myapp",
			"revision_a":  "rev-a",
			"revision_b":  "rev-b",
		})
		require.NoError(t, err)
		assert.True(t, result.IsError)
	})

	t.Run("missing required parameter name", func(t *testing.T) {
		mock := &MockArgoClient{}
		tm := testToolManager(mock, false, false)
		result, err := tm.CallTool(context.Background(), "get_application_revision_diff", map[string]interface{}{
			"revision_a": "rev-a",
			"revision_b": "rev-b",
		})
		require.NoError(t, err)
		assert.True(t, result.IsError)
	})

	t.Run("missing required parameter revision_a", func(t *testing.T) {
		mock := &MockArgoClient{}
		tm := testToolManager(mock, false, false)
		result, err := tm.CallTool(context.Background(), "get_application_revision_diff", map[string]interface{}{
			"name":       "myapp",
			"revision_b": "rev-b",
		})
		require.NoError(t, err)
		assert.True(t, result.IsError)
	})

	t.Run("missing required parameter revision_b", func(t *testing.T) {
		mock := &MockArgoClient{}
		tm := testToolManager(mock, false, false)
		result, err := tm.CallTool(context.Background(), "get_application_revision_diff", map[string]interface{}{
			"name":       "myapp",
			"revision_a": "rev-a",
		})
		require.NoError(t, err)
		assert.True(t, result.IsError)
	})

	t.Run("with source_index", func(t *testing.T) {
		mock := &MockArgoClient{
			GetApplicationManifestsFn: func(_ context.Context, query *application.ApplicationManifestQuery) ([]string, error) {
				assert.NotNil(t, query.SourcePositions)
				if query.SourcePositions != nil {
					assert.Equal(t, int64(2), query.SourcePositions[0])
				}
				return []string{}, nil
			},
		}
		tm := testToolManager(mock, false, false)
		result, err := tm.CallTool(context.Background(), "get_application_revision_diff", map[string]interface{}{
			"name":         "myapp",
			"revision_a":   "rev-a",
			"revision_b":   "rev-b",
			"source_index": 2,
		})
		require.NoError(t, err)
		assert.False(t, result.IsError)
	})

	t.Run("strips managed fields", func(t *testing.T) {
		mock := &MockArgoClient{
			GetApplicationManifestsFn: func(_ context.Context, query *application.ApplicationManifestQuery) ([]string, error) {
				if query.Revision != nil && *query.Revision == "rev-a" {
					return []string{
						`{"apiVersion":"v1","kind":"ConfigMap","metadata":{"name":"cm1","namespace":"default","managedFields":[{"manager":"test"}],"creationTimestamp":"2024-01-01T00:00:00Z","uid":"abc-123","resourceVersion":"999","generation":1},"data":{"key":"old"}}`,
					}, nil
				}
				return []string{
					`{"apiVersion":"v1","kind":"ConfigMap","metadata":{"name":"cm1","namespace":"default","managedFields":[{"manager":"test"}],"creationTimestamp":"2024-01-01T00:00:00Z","uid":"abc-123","resourceVersion":"999","generation":1},"data":{"key":"new"}}`,
				}, nil
			},
		}
		tm := testToolManager(mock, false, false)
		result, err := tm.CallTool(context.Background(), "get_application_revision_diff", map[string]interface{}{
			"name":        "myapp",
			"revision_a":  "rev-a",
			"revision_b":  "rev-b",
		})
		require.NoError(t, err)
		assert.False(t, result.IsError)
		data := parseResultYAML(t, result)
		summary, ok := data["summary"].(map[string]interface{})
		require.True(t, ok)
		assert.Equal(t, float64(1), summary["modified"])
	})
}

func TestBuildResourceMap(t *testing.T) {
	t.Run("valid manifests", func(t *testing.T) {
		manifests := []string{
			`{"apiVersion":"v1","kind":"ConfigMap","metadata":{"name":"cm1","namespace":"default"},"data":{"key":"value"}}`,
			`{"apiVersion":"apps/v1","kind":"Deployment","metadata":{"name":"deploy1","namespace":"default"}}`,
		}
		result := buildResourceMap(manifests)
		assert.Len(t, result, 2)

		_, hasCM := result[resourceKey{Group: "", Kind: "ConfigMap", Namespace: "default", Name: "cm1"}]
		assert.True(t, hasCM)

		_, hasDeploy := result[resourceKey{Group: "apps", Kind: "Deployment", Namespace: "default", Name: "deploy1"}]
		assert.True(t, hasDeploy)
	})

	t.Run("invalid json skipped", func(t *testing.T) {
		manifests := []string{
			`invalid json`,
			`{"apiVersion":"v1","kind":"ConfigMap","metadata":{"name":"cm1"},"data":{"key":"value"}}`,
		}
		result := buildResourceMap(manifests)
		assert.Len(t, result, 1)
	})

	t.Run("strips metadata fields", func(t *testing.T) {
		manifests := []string{
			`{"apiVersion":"v1","kind":"ConfigMap","metadata":{"name":"cm1","managedFields":[],"creationTimestamp":"2024-01-01T00:00:00Z","uid":"abc","resourceVersion":"999","generation":1},"data":{"key":"value"}}`,
		}
		result := buildResourceMap(manifests)
		for _, v := range result {
			assert.NotContains(t, v, "managedFields")
			assert.NotContains(t, v, "creationTimestamp")
			assert.NotContains(t, v, "uid")
			assert.NotContains(t, v, "resourceVersion")
			assert.NotContains(t, v, "generation")
		}
	})

	t.Run("empty name skipped", func(t *testing.T) {
		manifests := []string{
			`{"apiVersion":"v1","kind":"ConfigMap","metadata":{}}`,
		}
		result := buildResourceMap(manifests)
		assert.Len(t, result, 0)
	})
}

func TestExtractResourceKey(t *testing.T) {
	t.Run("core api group", func(t *testing.T) {
		obj := map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata": map[string]interface{}{
				"name":      "cm1",
				"namespace": "default",
			},
		}
		key := extractResourceKey(obj)
		assert.Equal(t, "", key.Group)
		assert.Equal(t, "ConfigMap", key.Kind)
		assert.Equal(t, "default", key.Namespace)
		assert.Equal(t, "cm1", key.Name)
	})

	t.Run("grouped api", func(t *testing.T) {
		obj := map[string]interface{}{
			"apiVersion": "apps/v1",
			"kind":       "Deployment",
			"metadata": map[string]interface{}{
				"name":      "deploy1",
				"namespace": "production",
			},
		}
		key := extractResourceKey(obj)
		assert.Equal(t, "apps", key.Group)
		assert.Equal(t, "Deployment", key.Kind)
	})

	t.Run("cluster-scoped resource", func(t *testing.T) {
		obj := map[string]interface{}{
			"apiVersion": "rbac.authorization.k8s.io/v1",
			"kind":       "ClusterRole",
			"metadata": map[string]interface{}{
				"name": "admin",
			},
		}
		key := extractResourceKey(obj)
		assert.Equal(t, "rbac.authorization.k8s.io", key.Group)
		assert.Equal(t, "", key.Namespace)
		assert.Equal(t, "admin", key.Name)
	})
}

func TestComputeRevisionDiff(t *testing.T) {
	t.Run("mixed changes", func(t *testing.T) {
		resourcesA := map[resourceKey]string{
			{Group: "", Kind: "ConfigMap", Namespace: "default", Name: "cm1"}:      "data:\n  key: old\n",
			{Group: "", Kind: "ConfigMap", Namespace: "default", Name: "cm2"}:      "data:\n  key: unchanged\n",
			{Group: "apps", Kind: "Deployment", Namespace: "default", Name: "old"}: "spec:\n  replicas: 3\n",
		}
		resourcesB := map[resourceKey]string{
			{Group: "", Kind: "ConfigMap", Namespace: "default", Name: "cm1"}: "data:\n  key: new\n",
			{Group: "", Kind: "ConfigMap", Namespace: "default", Name: "cm2"}: "data:\n  key: unchanged\n",
			{Group: "batch", Kind: "Job", Namespace: "default", Name: "new"}:  "spec:\n  backoffLimit: 3\n",
		}

		result := computeRevisionDiff(resourcesA, resourcesB, "rev-a", "rev-b", "myapp")

		assert.Equal(t, "myapp", result.Application)
		assert.Equal(t, "rev-a", result.RevisionA)
		assert.Equal(t, "rev-b", result.RevisionB)
		assert.Equal(t, 1, result.Summary["modified"])
		assert.Equal(t, 1, result.Summary["added"])
		assert.Equal(t, 1, result.Summary["removed"])
		assert.Equal(t, 1, result.Summary["unchanged"])
		assert.Len(t, result.Resources, 3)
		assert.False(t, result.Limited)
	})

	t.Run("empty maps", func(t *testing.T) {
		result := computeRevisionDiff(map[resourceKey]string{}, map[resourceKey]string{}, "a", "b", "app")
		assert.Equal(t, 0, result.Summary["modified"])
		assert.Empty(t, result.Resources)
	})
}
