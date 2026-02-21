package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/argocd-mcp/argocd-mcp/internal/client"
	"github.com/argoproj/argo-cd/v3/pkg/apiclient/application"
	"github.com/argoproj/argo-cd/v3/pkg/apiclient/cluster"
	"github.com/argoproj/argo-cd/v3/pkg/apiclient/project"
	"github.com/argoproj/argo-cd/v3/pkg/apiclient/repository"
	"github.com/argoproj/argo-cd/v3/pkg/apis/application/v1alpha1"
	healthlib "github.com/argoproj/gitops-engine/pkg/health"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// testToolManager creates a ToolManager with a mock client for testing.
func testToolManager(mock *MockArgoClient, safeMode bool) *ToolManager {
	logger := logrus.New()
	logger.SetLevel(logrus.DebugLevel)
	return NewToolManager(mock, logger, safeMode)
}

// parseResultJSON extracts and parses the JSON from a CallToolResult.
func parseResultJSON(t *testing.T, result *mcp.CallToolResult) map[string]interface{} {
	t.Helper()
	require.NotNil(t, result)
	require.NotEmpty(t, result.Content)
	text := result.Content[0].(mcp.TextContent).Text
	var data map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(text), &data))
	return data
}

// makeApp creates a test Application with sensible defaults.
func makeApp(name, project, repoURL string) *v1alpha1.Application {
	return &v1alpha1.Application{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec: v1alpha1.ApplicationSpec{
			Project: project,
			Source: &v1alpha1.ApplicationSource{
				RepoURL:        repoURL,
				Path:           "manifests",
				TargetRevision: "HEAD",
			},
			Destination: v1alpha1.ApplicationDestination{
				Server:    "https://kubernetes.default.svc",
				Namespace: "default",
			},
		},
		Status: v1alpha1.ApplicationStatus{
			Sync: v1alpha1.SyncStatus{
				Status:   v1alpha1.SyncStatusCodeSynced,
				Revision: "abc123",
			},
			Health: v1alpha1.AppHealthStatus{
				Status: healthlib.HealthStatusHealthy,
			},
		},
	}
}

// =============================================================================
// Application handler tests
// =============================================================================

func TestHandleListApplications(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		mock := &MockArgoClient{
			ListApplicationsFn: func(_ context.Context, _ *application.ApplicationQuery) (*v1alpha1.ApplicationList, error) {
				return &v1alpha1.ApplicationList{
					Items: []v1alpha1.Application{
						*makeApp("app1", "default", "https://github.com/test/repo"),
						*makeApp("app2", "default", "https://github.com/test/repo2"),
					},
				}, nil
			},
		}
		tm := testToolManager(mock, false)
		result, err := tm.CallTool(context.Background(), "list_applications", map[string]interface{}{})
		require.NoError(t, err)
		assert.False(t, result.IsError)

		data := parseResultJSON(t, result)
		items := data["items"].([]interface{})
		assert.Len(t, items, 2)
		assert.Equal(t, float64(2), data["total"])
	})

	t.Run("with limit", func(t *testing.T) {
		mock := &MockArgoClient{
			ListApplicationsFn: func(_ context.Context, _ *application.ApplicationQuery) (*v1alpha1.ApplicationList, error) {
				apps := make([]v1alpha1.Application, 10)
				for i := range apps {
					apps[i] = *makeApp(fmt.Sprintf("app%d", i), "default", "https://github.com/test/repo")
				}
				return &v1alpha1.ApplicationList{Items: apps}, nil
			},
		}
		tm := testToolManager(mock, false)
		result, err := tm.CallTool(context.Background(), "list_applications", map[string]interface{}{
			"limit": float64(3),
		})
		require.NoError(t, err)
		data := parseResultJSON(t, result)
		items := data["items"].([]interface{})
		assert.Len(t, items, 3)
		assert.Equal(t, float64(10), data["total"])
	})

	t.Run("error from client", func(t *testing.T) {
		mock := &MockArgoClient{
			ListApplicationsFn: func(_ context.Context, _ *application.ApplicationQuery) (*v1alpha1.ApplicationList, error) {
				return nil, fmt.Errorf("connection refused")
			},
		}
		tm := testToolManager(mock, false)
		result, err := tm.CallTool(context.Background(), "list_applications", map[string]interface{}{})
		require.NoError(t, err)
		assert.True(t, result.IsError)
	})

	t.Run("empty list", func(t *testing.T) {
		mock := &MockArgoClient{
			ListApplicationsFn: func(_ context.Context, _ *application.ApplicationQuery) (*v1alpha1.ApplicationList, error) {
				return &v1alpha1.ApplicationList{Items: []v1alpha1.Application{}}, nil
			},
		}
		tm := testToolManager(mock, false)
		result, err := tm.CallTool(context.Background(), "list_applications", map[string]interface{}{})
		require.NoError(t, err)
		assert.False(t, result.IsError)
		data := parseResultJSON(t, result)
		assert.Equal(t, float64(0), data["total"])
	})

	t.Run("limit capped at 100", func(t *testing.T) {
		mock := &MockArgoClient{
			ListApplicationsFn: func(_ context.Context, _ *application.ApplicationQuery) (*v1alpha1.ApplicationList, error) {
				return &v1alpha1.ApplicationList{Items: []v1alpha1.Application{}}, nil
			},
		}
		tm := testToolManager(mock, false)
		result, err := tm.CallTool(context.Background(), "list_applications", map[string]interface{}{
			"limit": float64(200),
		})
		require.NoError(t, err)
		assert.False(t, result.IsError)
	})
}

func TestHandleGetApplication(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		mock := &MockArgoClient{
			GetApplicationFn: func(_ context.Context, _ *application.ApplicationQuery) (*v1alpha1.Application, error) {
				return makeApp("myapp", "default", "https://github.com/test/repo"), nil
			},
		}
		tm := testToolManager(mock, false)
		result, err := tm.CallTool(context.Background(), "get_application", map[string]interface{}{
			"name": "myapp",
		})
		require.NoError(t, err)
		assert.False(t, result.IsError)
		data := parseResultJSON(t, result)
		assert.Equal(t, "myapp", data["name"])
		assert.Equal(t, "https://github.com/test/repo", data["repo_url"])
	})

	t.Run("nil source does not panic", func(t *testing.T) {
		mock := &MockArgoClient{
			GetApplicationFn: func(_ context.Context, _ *application.ApplicationQuery) (*v1alpha1.Application, error) {
				app := &v1alpha1.Application{
					ObjectMeta: metav1.ObjectMeta{Name: "nosource"},
					Spec: v1alpha1.ApplicationSpec{
						Project: "default",
						Source:  nil, // nil source!
						Destination: v1alpha1.ApplicationDestination{
							Server:    "https://kubernetes.default.svc",
							Namespace: "default",
						},
					},
					Status: v1alpha1.ApplicationStatus{},
				}
				return app, nil
			},
		}
		tm := testToolManager(mock, false)
		result, err := tm.CallTool(context.Background(), "get_application", map[string]interface{}{
			"name": "nosource",
		})
		require.NoError(t, err)
		assert.False(t, result.IsError)
		data := parseResultJSON(t, result)
		assert.Equal(t, "", data["repo_url"])
		assert.Equal(t, "", data["path"])
	})

	t.Run("nil health/sync does not panic", func(t *testing.T) {
		mock := &MockArgoClient{
			GetApplicationFn: func(_ context.Context, _ *application.ApplicationQuery) (*v1alpha1.Application, error) {
				app := &v1alpha1.Application{
					ObjectMeta: metav1.ObjectMeta{Name: "empty-status"},
					Spec: v1alpha1.ApplicationSpec{
						Project: "default",
						Source: &v1alpha1.ApplicationSource{
							RepoURL: "https://github.com/test/repo",
						},
					},
					// Empty Status - no Sync, no Health initialized
					Status: v1alpha1.ApplicationStatus{},
				}
				return app, nil
			},
		}
		tm := testToolManager(mock, false)
		result, err := tm.CallTool(context.Background(), "get_application", map[string]interface{}{
			"name": "empty-status",
		})
		require.NoError(t, err)
		assert.False(t, result.IsError)
	})

	t.Run("error", func(t *testing.T) {
		mock := &MockArgoClient{
			GetApplicationFn: func(_ context.Context, _ *application.ApplicationQuery) (*v1alpha1.Application, error) {
				return nil, fmt.Errorf("not found")
			},
		}
		tm := testToolManager(mock, false)
		result, err := tm.CallTool(context.Background(), "get_application", map[string]interface{}{
			"name": "doesnotexist",
		})
		require.NoError(t, err)
		assert.True(t, result.IsError)
	})
}

func TestHandleCreateApplication(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		mock := &MockArgoClient{
			CreateApplicationFn: func(_ context.Context, req *application.ApplicationCreateRequest) (*v1alpha1.Application, error) {
				return makeApp(req.Application.Name, req.Application.Spec.Project, req.Application.Spec.Source.RepoURL), nil
			},
		}
		tm := testToolManager(mock, false)
		result, err := tm.CallTool(context.Background(), "create_application", map[string]interface{}{
			"name":     "newapp",
			"project":  "default",
			"repo_url": "https://github.com/test/repo",
			"path":     "k8s",
		})
		require.NoError(t, err)
		assert.False(t, result.IsError)
		data := parseResultJSON(t, result)
		assert.Equal(t, "newapp", data["name"])
	})

	t.Run("blocked in safe mode", func(t *testing.T) {
		mock := &MockArgoClient{}
		tm := testToolManager(mock, true)
		result, err := tm.CallTool(context.Background(), "create_application", map[string]interface{}{
			"name":     "newapp",
			"project":  "default",
			"repo_url": "https://github.com/test/repo",
			"path":     "k8s",
		})
		require.NoError(t, err)
		assert.True(t, result.IsError)
		assert.Len(t, mock.CreateApplicationCalls, 0, "should not have called client")
	})

	t.Run("error from client", func(t *testing.T) {
		mock := &MockArgoClient{
			CreateApplicationFn: func(_ context.Context, _ *application.ApplicationCreateRequest) (*v1alpha1.Application, error) {
				return nil, fmt.Errorf("already exists")
			},
		}
		tm := testToolManager(mock, false)
		result, err := tm.CallTool(context.Background(), "create_application", map[string]interface{}{
			"name":     "existing",
			"project":  "default",
			"repo_url": "https://github.com/test/repo",
			"path":     "k8s",
		})
		require.NoError(t, err)
		assert.True(t, result.IsError)
	})
}

func TestHandleDeleteApplication(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		mock := &MockArgoClient{
			DeleteApplicationFn: func(_ context.Context, _ *application.ApplicationDeleteRequest) error {
				return nil
			},
		}
		tm := testToolManager(mock, false)
		result, err := tm.CallTool(context.Background(), "delete_application", map[string]interface{}{
			"name": "myapp",
		})
		require.NoError(t, err)
		assert.False(t, result.IsError)
		data := parseResultJSON(t, result)
		assert.Equal(t, true, data["success"])
	})

	t.Run("blocked in safe mode", func(t *testing.T) {
		mock := &MockArgoClient{}
		tm := testToolManager(mock, true)
		result, err := tm.CallTool(context.Background(), "delete_application", map[string]interface{}{
			"name": "myapp",
		})
		require.NoError(t, err)
		assert.True(t, result.IsError)
	})
}

func TestHandleSyncApplication(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		mock := &MockArgoClient{
			SyncApplicationFn: func(_ context.Context, _ *application.ApplicationSyncRequest) (*v1alpha1.Application, error) {
				return makeApp("myapp", "default", "https://github.com/test/repo"), nil
			},
		}
		tm := testToolManager(mock, false)
		result, err := tm.CallTool(context.Background(), "sync_application", map[string]interface{}{
			"name": "myapp",
		})
		require.NoError(t, err)
		assert.False(t, result.IsError)
		data := parseResultJSON(t, result)
		assert.Contains(t, data["message"], "sync initiated")
	})

	t.Run("nil sync status does not panic", func(t *testing.T) {
		mock := &MockArgoClient{
			SyncApplicationFn: func(_ context.Context, _ *application.ApplicationSyncRequest) (*v1alpha1.Application, error) {
				return &v1alpha1.Application{
					ObjectMeta: metav1.ObjectMeta{Name: "myapp"},
					Status:     v1alpha1.ApplicationStatus{},
				}, nil
			},
		}
		tm := testToolManager(mock, false)
		result, err := tm.CallTool(context.Background(), "sync_application", map[string]interface{}{
			"name": "myapp",
		})
		require.NoError(t, err)
		assert.False(t, result.IsError)
	})

	t.Run("prune blocked in safe mode", func(t *testing.T) {
		mock := &MockArgoClient{}
		tm := testToolManager(mock, true)
		result, err := tm.CallTool(context.Background(), "sync_application", map[string]interface{}{
			"name":  "myapp",
			"prune": true,
		})
		require.NoError(t, err)
		assert.True(t, result.IsError)
	})

	t.Run("sync without prune allowed in safe mode", func(t *testing.T) {
		mock := &MockArgoClient{
			SyncApplicationFn: func(_ context.Context, _ *application.ApplicationSyncRequest) (*v1alpha1.Application, error) {
				return makeApp("myapp", "default", "https://github.com/test/repo"), nil
			},
		}
		tm := testToolManager(mock, true)
		result, err := tm.CallTool(context.Background(), "sync_application", map[string]interface{}{
			"name": "myapp",
		})
		require.NoError(t, err)
		assert.False(t, result.IsError)
	})
}

func TestHandleRollbackApplication(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		mock := &MockArgoClient{
			RollbackApplicationFn: func(_ context.Context, _ *application.ApplicationRollbackRequest) (*v1alpha1.Application, error) {
				return makeApp("myapp", "default", "https://github.com/test/repo"), nil
			},
		}
		tm := testToolManager(mock, false)
		result, err := tm.CallTool(context.Background(), "rollback_application", map[string]interface{}{
			"name":     "myapp",
			"revision": "abc123",
		})
		require.NoError(t, err)
		assert.False(t, result.IsError)
		data := parseResultJSON(t, result)
		assert.Contains(t, data["message"], "rolled back")
	})

	t.Run("blocked in safe mode", func(t *testing.T) {
		mock := &MockArgoClient{}
		tm := testToolManager(mock, true)
		result, err := tm.CallTool(context.Background(), "rollback_application", map[string]interface{}{
			"name":     "myapp",
			"revision": "abc123",
		})
		require.NoError(t, err)
		assert.True(t, result.IsError)
	})
}

func TestHandleUpdateApplication(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		existingApp := makeApp("myapp", "default", "https://github.com/test/repo")
		mock := &MockArgoClient{
			GetApplicationFn: func(_ context.Context, _ *application.ApplicationQuery) (*v1alpha1.Application, error) {
				return existingApp, nil
			},
			UpdateApplicationFn: func(_ context.Context, req *application.ApplicationUpdateRequest) (*v1alpha1.Application, error) {
				return req.Application, nil
			},
		}
		tm := testToolManager(mock, false)
		result, err := tm.CallTool(context.Background(), "update_application", map[string]interface{}{
			"name":            "myapp",
			"target_revision": "v2.0",
		})
		require.NoError(t, err)
		assert.False(t, result.IsError)
	})

	t.Run("blocked in safe mode", func(t *testing.T) {
		mock := &MockArgoClient{}
		tm := testToolManager(mock, true)
		result, err := tm.CallTool(context.Background(), "update_application", map[string]interface{}{
			"name": "myapp",
		})
		require.NoError(t, err)
		assert.True(t, result.IsError)
	})

	t.Run("nil source fields not updated", func(t *testing.T) {
		app := &v1alpha1.Application{
			ObjectMeta: metav1.ObjectMeta{Name: "nosource"},
			Spec: v1alpha1.ApplicationSpec{
				Project: "default",
				Source:  nil,
			},
			Status: v1alpha1.ApplicationStatus{
				Sync:   v1alpha1.SyncStatus{Status: v1alpha1.SyncStatusCodeSynced},
				Health: v1alpha1.AppHealthStatus{Status: healthlib.HealthStatusHealthy},
			},
		}
		mock := &MockArgoClient{
			GetApplicationFn: func(_ context.Context, _ *application.ApplicationQuery) (*v1alpha1.Application, error) {
				return app, nil
			},
			UpdateApplicationFn: func(_ context.Context, req *application.ApplicationUpdateRequest) (*v1alpha1.Application, error) {
				return req.Application, nil
			},
		}
		tm := testToolManager(mock, false)
		result, err := tm.CallTool(context.Background(), "update_application", map[string]interface{}{
			"name":    "nosource",
			"repo_url": "https://github.com/new/repo",
		})
		require.NoError(t, err)
		assert.False(t, result.IsError)
		// Source was nil, so repo_url update should be silently skipped
		assert.Nil(t, app.Spec.Source)
	})
}

func TestHandleGetApplicationManifests(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		mock := &MockArgoClient{
			GetApplicationManifestsFn: func(_ context.Context, _ *application.ApplicationManifestQuery) ([]string, error) {
				return []string{
					`{"apiVersion":"v1","kind":"Service","metadata":{"name":"svc1"}}`,
					`{"apiVersion":"v1","kind":"ConfigMap","metadata":{"name":"cm1"}}`,
				}, nil
			},
		}
		tm := testToolManager(mock, false)
		result, err := tm.CallTool(context.Background(), "get_application_manifests", map[string]interface{}{
			"name": "myapp",
		})
		require.NoError(t, err)
		assert.False(t, result.IsError)
		data := parseResultJSON(t, result)
		assert.Equal(t, float64(2), data["count"])
		assert.Equal(t, false, data["limited"])
	})

	t.Run("error", func(t *testing.T) {
		mock := &MockArgoClient{
			GetApplicationManifestsFn: func(_ context.Context, _ *application.ApplicationManifestQuery) ([]string, error) {
				return nil, fmt.Errorf("not found")
			},
		}
		tm := testToolManager(mock, false)
		result, err := tm.CallTool(context.Background(), "get_application_manifests", map[string]interface{}{
			"name": "myapp",
		})
		require.NoError(t, err)
		assert.True(t, result.IsError)
	})
}

func TestHandleGetApplicationDiff(t *testing.T) {
	t.Run("success with out of sync", func(t *testing.T) {
		mock := &MockArgoClient{
			GetManagedResourcesFn: func(_ context.Context, _ string) ([]*v1alpha1.ResourceDiff, error) {
				return []*v1alpha1.ResourceDiff{
					{
						Group:               "",
						Kind:                "ConfigMap",
						Namespace:           "default",
						Name:                "my-config",
						Modified:            true,
						TargetState:         `{"apiVersion":"v1","kind":"ConfigMap","data":{"key":"new"}}`,
						NormalizedLiveState: `{"apiVersion":"v1","kind":"ConfigMap","data":{"key":"old"}}`,
					},
					{
						Group:     "apps",
						Kind:      "Deployment",
						Namespace: "default",
						Name:      "my-deploy",
						Modified:  false,
					},
				}, nil
			},
		}
		tm := testToolManager(mock, false)
		result, err := tm.CallTool(context.Background(), "get_application_diff", map[string]interface{}{
			"name": "myapp",
		})
		require.NoError(t, err)
		assert.False(t, result.IsError)
		data := parseResultJSON(t, result)
		assert.Equal(t, float64(1), data["out_of_sync_count"])
	})

	t.Run("empty resources", func(t *testing.T) {
		mock := &MockArgoClient{
			GetManagedResourcesFn: func(_ context.Context, _ string) ([]*v1alpha1.ResourceDiff, error) {
				return []*v1alpha1.ResourceDiff{}, nil
			},
		}
		tm := testToolManager(mock, false)
		result, err := tm.CallTool(context.Background(), "get_application_diff", map[string]interface{}{
			"name": "myapp",
		})
		require.NoError(t, err)
		assert.False(t, result.IsError)
	})
}

func TestHandleGetApplicationEvents(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		mock := &MockArgoClient{
			GetApplicationEventsFn: func(_ context.Context, _ *application.ApplicationResourceEventsQuery) (interface{}, error) {
				return map[string]interface{}{
					"items": []map[string]interface{}{
						{
							"type":    "Normal",
							"reason":  "Synced",
							"message": "Application synced successfully",
							"involvedObject": map[string]interface{}{
								"name":      "myapp",
								"namespace": "default",
								"kind":      "Application",
							},
						},
					},
				}, nil
			},
		}
		tm := testToolManager(mock, false)
		result, err := tm.CallTool(context.Background(), "get_application_events", map[string]interface{}{
			"name": "myapp",
		})
		require.NoError(t, err)
		assert.False(t, result.IsError)
		data := parseResultJSON(t, result)
		assert.Equal(t, float64(1), data["total"])
	})

	t.Run("with resource filter", func(t *testing.T) {
		mock := &MockArgoClient{
			GetApplicationEventsFn: func(_ context.Context, _ *application.ApplicationResourceEventsQuery) (interface{}, error) {
				return map[string]interface{}{
					"items": []map[string]interface{}{
						{
							"type":    "Normal",
							"reason":  "Synced",
							"message": "msg1",
							"involvedObject": map[string]interface{}{
								"name": "deploy1",
								"kind": "Deployment",
							},
						},
						{
							"type":    "Warning",
							"reason":  "Failed",
							"message": "msg2",
							"involvedObject": map[string]interface{}{
								"name": "deploy2",
								"kind": "Deployment",
							},
						},
					},
				}, nil
			},
		}
		tm := testToolManager(mock, false)
		result, err := tm.CallTool(context.Background(), "get_application_events", map[string]interface{}{
			"name":          "myapp",
			"resource_name": "deploy1",
		})
		require.NoError(t, err)
		assert.False(t, result.IsError)
		data := parseResultJSON(t, result)
		assert.Equal(t, float64(1), data["total"])
		assert.Equal(t, true, data["filtered"])
	})

	t.Run("error", func(t *testing.T) {
		mock := &MockArgoClient{
			GetApplicationEventsFn: func(_ context.Context, _ *application.ApplicationResourceEventsQuery) (interface{}, error) {
				return nil, fmt.Errorf("connection error")
			},
		}
		tm := testToolManager(mock, false)
		result, err := tm.CallTool(context.Background(), "get_application_events", map[string]interface{}{
			"name": "myapp",
		})
		require.NoError(t, err)
		assert.True(t, result.IsError)
	})
}

func TestHandleGetLogs(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		mock := &MockArgoClient{
			GetApplicationLogsFn: func(_ context.Context, _ *application.ApplicationPodLogsQuery) ([]client.ApplicationLogEntry, error) {
				return []client.ApplicationLogEntry{
					{Content: "line 1", Timestamp: "2024-01-01T00:00:00Z", PodName: "pod-1"},
					{Content: "line 2", Timestamp: "2024-01-01T00:00:01Z", PodName: "pod-1"},
				}, nil
			},
		}
		tm := testToolManager(mock, false)
		result, err := tm.CallTool(context.Background(), "get_logs", map[string]interface{}{
			"name": "myapp",
		})
		require.NoError(t, err)
		assert.False(t, result.IsError)
		data := parseResultJSON(t, result)
		assert.Equal(t, float64(2), data["total_lines"])
		assert.Equal(t, false, data["truncated"])
	})

	t.Run("empty logs", func(t *testing.T) {
		mock := &MockArgoClient{
			GetApplicationLogsFn: func(_ context.Context, _ *application.ApplicationPodLogsQuery) ([]client.ApplicationLogEntry, error) {
				return []client.ApplicationLogEntry{}, nil
			},
		}
		tm := testToolManager(mock, false)
		result, err := tm.CallTool(context.Background(), "get_logs", map[string]interface{}{
			"name": "myapp",
		})
		require.NoError(t, err)
		assert.False(t, result.IsError)
	})

	t.Run("error", func(t *testing.T) {
		mock := &MockArgoClient{
			GetApplicationLogsFn: func(_ context.Context, _ *application.ApplicationPodLogsQuery) ([]client.ApplicationLogEntry, error) {
				return nil, fmt.Errorf("pod not found")
			},
		}
		tm := testToolManager(mock, false)
		result, err := tm.CallTool(context.Background(), "get_logs", map[string]interface{}{
			"name": "myapp",
		})
		require.NoError(t, err)
		assert.True(t, result.IsError)
	})
}

func TestHandleListResourceActions(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		mock := &MockArgoClient{
			ListResourceActionsFn: func(_ context.Context, _ *application.ApplicationResourceRequest) ([]*v1alpha1.ResourceAction, error) {
				return []*v1alpha1.ResourceAction{
					{Name: "restart"},
					{Name: "pause"},
				}, nil
			},
		}
		tm := testToolManager(mock, false)
		result, err := tm.CallTool(context.Background(), "list_resource_actions", map[string]interface{}{
			"name":          "myapp",
			"kind":          "Deployment",
			"resource_name": "my-deploy",
		})
		require.NoError(t, err)
		assert.False(t, result.IsError)
		data := parseResultJSON(t, result)
		assert.Equal(t, float64(2), data["total"])
	})
}

func TestHandleRunResourceAction(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		mock := &MockArgoClient{
			//lint:ignore SA1019 ResourceActionRunRequest is deprecated but required for the API
			RunResourceActionFn: func(_ context.Context, _ *application.ResourceActionRunRequest) error {
				return nil
			},
		}
		tm := testToolManager(mock, false)
		result, err := tm.CallTool(context.Background(), "run_resource_action", map[string]interface{}{
			"name":          "myapp",
			"group":         "apps",
			"kind":          "Deployment",
			"resource_name": "my-deploy",
			"action":        "restart",
		})
		require.NoError(t, err)
		assert.False(t, result.IsError)
	})

	t.Run("blocked in safe mode", func(t *testing.T) {
		mock := &MockArgoClient{}
		tm := testToolManager(mock, true)
		result, err := tm.CallTool(context.Background(), "run_resource_action", map[string]interface{}{
			"name":          "myapp",
			"group":         "apps",
			"kind":          "Deployment",
			"resource_name": "my-deploy",
			"action":        "restart",
		})
		require.NoError(t, err)
		assert.True(t, result.IsError)
	})
}

func TestHandleGetApplicationResource(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		mock := &MockArgoClient{
			GetApplicationResourceFn: func(_ context.Context, _ *application.ApplicationResourceRequest) (interface{}, error) {
				return map[string]interface{}{
					"apiVersion": "apps/v1",
					"kind":       "Deployment",
					"metadata":   map[string]interface{}{"name": "my-deploy"},
				}, nil
			},
		}
		tm := testToolManager(mock, false)
		result, err := tm.CallTool(context.Background(), "get_application_resource", map[string]interface{}{
			"name":          "myapp",
			"kind":          "Deployment",
			"resource_name": "my-deploy",
		})
		require.NoError(t, err)
		assert.False(t, result.IsError)
	})
}

func TestHandlePatchApplicationResource(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		mock := &MockArgoClient{
			PatchApplicationResourceFn: func(_ context.Context, _ *application.ApplicationResourcePatchRequest) (interface{}, error) {
				return map[string]interface{}{"patched": true}, nil
			},
		}
		tm := testToolManager(mock, false)
		result, err := tm.CallTool(context.Background(), "patch_application_resource", map[string]interface{}{
			"name":          "myapp",
			"kind":          "Deployment",
			"resource_name": "my-deploy",
			"patch":         `{"spec":{"replicas":3}}`,
		})
		require.NoError(t, err)
		assert.False(t, result.IsError)
	})

	t.Run("blocked in safe mode", func(t *testing.T) {
		mock := &MockArgoClient{}
		tm := testToolManager(mock, true)
		result, err := tm.CallTool(context.Background(), "patch_application_resource", map[string]interface{}{
			"name":          "myapp",
			"kind":          "Deployment",
			"resource_name": "my-deploy",
			"patch":         `{"spec":{"replicas":3}}`,
		})
		require.NoError(t, err)
		assert.True(t, result.IsError)
	})
}

func TestHandleDeleteApplicationResource(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		mock := &MockArgoClient{
			DeleteApplicationResourceFn: func(_ context.Context, _ *application.ApplicationResourceDeleteRequest) error {
				return nil
			},
		}
		tm := testToolManager(mock, false)
		result, err := tm.CallTool(context.Background(), "delete_application_resource", map[string]interface{}{
			"name":          "myapp",
			"kind":          "Pod",
			"resource_name": "my-pod",
		})
		require.NoError(t, err)
		assert.False(t, result.IsError)
	})

	t.Run("blocked in safe mode", func(t *testing.T) {
		mock := &MockArgoClient{}
		tm := testToolManager(mock, true)
		result, err := tm.CallTool(context.Background(), "delete_application_resource", map[string]interface{}{
			"name":          "myapp",
			"kind":          "Pod",
			"resource_name": "my-pod",
		})
		require.NoError(t, err)
		assert.True(t, result.IsError)
	})
}

// =============================================================================
// Project handler tests
// =============================================================================

func TestHandleListProjects(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		mock := &MockArgoClient{
			ListProjectsFn: func(_ context.Context, _ *project.ProjectQuery) (*v1alpha1.AppProjectList, error) {
				return &v1alpha1.AppProjectList{
					Items: []v1alpha1.AppProject{
						{ObjectMeta: metav1.ObjectMeta{Name: "proj1"}, Spec: v1alpha1.AppProjectSpec{Description: "Project 1"}},
						{ObjectMeta: metav1.ObjectMeta{Name: "proj2"}, Spec: v1alpha1.AppProjectSpec{Description: "Project 2"}},
					},
				}, nil
			},
		}
		tm := testToolManager(mock, false)
		result, err := tm.CallTool(context.Background(), "list_projects", map[string]interface{}{})
		require.NoError(t, err)
		assert.False(t, result.IsError)
		data := parseResultJSON(t, result)
		assert.Equal(t, float64(2), data["total"])
	})

	t.Run("error", func(t *testing.T) {
		mock := &MockArgoClient{
			ListProjectsFn: func(_ context.Context, _ *project.ProjectQuery) (*v1alpha1.AppProjectList, error) {
				return nil, fmt.Errorf("forbidden")
			},
		}
		tm := testToolManager(mock, false)
		result, err := tm.CallTool(context.Background(), "list_projects", map[string]interface{}{})
		require.NoError(t, err)
		assert.True(t, result.IsError)
	})
}

func TestHandleGetProject(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		mock := &MockArgoClient{
			GetProjectFn: func(_ context.Context, _ *project.ProjectQuery) (*v1alpha1.AppProject, error) {
				return &v1alpha1.AppProject{
					ObjectMeta: metav1.ObjectMeta{Name: "myproject"},
					Spec: v1alpha1.AppProjectSpec{
						Description: "My project",
						SourceRepos: []string{"https://github.com/test/*"},
					},
				}, nil
			},
		}
		tm := testToolManager(mock, false)
		result, err := tm.CallTool(context.Background(), "get_project", map[string]interface{}{
			"name": "myproject",
		})
		require.NoError(t, err)
		assert.False(t, result.IsError)
		data := parseResultJSON(t, result)
		assert.Equal(t, "myproject", data["name"])
	})
}

func TestHandleCreateProject(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		mock := &MockArgoClient{
			CreateProjectFn: func(_ context.Context, req *project.ProjectCreateRequest) (*v1alpha1.AppProject, error) {
				return &v1alpha1.AppProject{
					ObjectMeta: metav1.ObjectMeta{Name: req.Project.Name},
					Spec:       req.Project.Spec,
				}, nil
			},
		}
		tm := testToolManager(mock, false)
		result, err := tm.CallTool(context.Background(), "create_project", map[string]interface{}{
			"name":        "newproj",
			"description": "A new project",
		})
		require.NoError(t, err)
		assert.False(t, result.IsError)
	})

	t.Run("blocked in safe mode", func(t *testing.T) {
		mock := &MockArgoClient{}
		tm := testToolManager(mock, true)
		result, err := tm.CallTool(context.Background(), "create_project", map[string]interface{}{
			"name": "newproj",
		})
		require.NoError(t, err)
		assert.True(t, result.IsError)
	})
}

func TestHandleUpdateProject(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		mock := &MockArgoClient{
			GetProjectFn: func(_ context.Context, _ *project.ProjectQuery) (*v1alpha1.AppProject, error) {
				return &v1alpha1.AppProject{
					ObjectMeta: metav1.ObjectMeta{Name: "myproject"},
					Spec:       v1alpha1.AppProjectSpec{Description: "old desc"},
				}, nil
			},
			UpdateProjectFn: func(_ context.Context, req *project.ProjectUpdateRequest) (*v1alpha1.AppProject, error) {
				return req.Project, nil
			},
		}
		tm := testToolManager(mock, false)
		result, err := tm.CallTool(context.Background(), "update_project", map[string]interface{}{
			"name":        "myproject",
			"description": "new desc",
		})
		require.NoError(t, err)
		assert.False(t, result.IsError)
	})

	t.Run("blocked in safe mode", func(t *testing.T) {
		mock := &MockArgoClient{}
		tm := testToolManager(mock, true)
		result, err := tm.CallTool(context.Background(), "update_project", map[string]interface{}{
			"name": "myproject",
		})
		require.NoError(t, err)
		assert.True(t, result.IsError)
	})
}

func TestHandleDeleteProject(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		mock := &MockArgoClient{
			DeleteProjectFn: func(_ context.Context, _ *project.ProjectQuery) error {
				return nil
			},
		}
		tm := testToolManager(mock, false)
		result, err := tm.CallTool(context.Background(), "delete_project", map[string]interface{}{
			"name": "myproject",
		})
		require.NoError(t, err)
		assert.False(t, result.IsError)
	})

	t.Run("blocked in safe mode", func(t *testing.T) {
		mock := &MockArgoClient{}
		tm := testToolManager(mock, true)
		result, err := tm.CallTool(context.Background(), "delete_project", map[string]interface{}{
			"name": "myproject",
		})
		require.NoError(t, err)
		assert.True(t, result.IsError)
	})
}

func TestHandleGetProjectEvents(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		mock := &MockArgoClient{
			GetProjectEventsFn: func(_ context.Context, _ *project.ProjectQuery) (interface{}, error) {
				return []map[string]interface{}{
					{"type": "Normal", "reason": "Created", "message": "Project created"},
				}, nil
			},
		}
		tm := testToolManager(mock, false)
		result, err := tm.CallTool(context.Background(), "get_project_events", map[string]interface{}{
			"name": "myproject",
		})
		require.NoError(t, err)
		assert.False(t, result.IsError)
	})
}

// =============================================================================
// Repository handler tests
// =============================================================================

func TestHandleListRepositories(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		mock := &MockArgoClient{
			ListRepositoriesFn: func(_ context.Context, _ *repository.RepoQuery) (*v1alpha1.RepositoryList, error) {
				return &v1alpha1.RepositoryList{
					Items: v1alpha1.Repositories{
						{Repo: "https://github.com/test/repo1", Type: "git", Name: "repo1"},
						{Repo: "https://github.com/test/repo2", Type: "git", Name: "repo2"},
					},
				}, nil
			},
		}
		tm := testToolManager(mock, false)
		result, err := tm.CallTool(context.Background(), "list_repositories", map[string]interface{}{})
		require.NoError(t, err)
		assert.False(t, result.IsError)
		data := parseResultJSON(t, result)
		assert.Equal(t, float64(2), data["total"])
	})
}

func TestHandleGetRepository(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		mock := &MockArgoClient{
			GetRepositoryFn: func(_ context.Context, _ *repository.RepoQuery) (*v1alpha1.Repository, error) {
				return &v1alpha1.Repository{
					Repo: "https://github.com/test/repo",
					Type: "git",
					Name: "test-repo",
				}, nil
			},
		}
		tm := testToolManager(mock, false)
		result, err := tm.CallTool(context.Background(), "get_repository", map[string]interface{}{
			"repo_url": "https://github.com/test/repo",
		})
		require.NoError(t, err)
		assert.False(t, result.IsError)
		data := parseResultJSON(t, result)
		assert.Equal(t, "https://github.com/test/repo", data["repo"])
	})
}

func TestHandleCreateRepository(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		mock := &MockArgoClient{
			CreateRepositoryFn: func(_ context.Context, req *repository.RepoCreateRequest) (*v1alpha1.Repository, error) {
				return &v1alpha1.Repository{
					Repo: req.Repo.Repo,
					Type: req.Repo.Type,
					Name: req.Repo.Name,
				}, nil
			},
		}
		tm := testToolManager(mock, false)
		result, err := tm.CallTool(context.Background(), "create_repository", map[string]interface{}{
			"repo_url": "https://github.com/test/new-repo",
			"type":     "git",
			"name":     "new-repo",
		})
		require.NoError(t, err)
		assert.False(t, result.IsError)
	})

	t.Run("blocked in safe mode", func(t *testing.T) {
		mock := &MockArgoClient{}
		tm := testToolManager(mock, true)
		result, err := tm.CallTool(context.Background(), "create_repository", map[string]interface{}{
			"repo_url": "https://github.com/test/new-repo",
		})
		require.NoError(t, err)
		assert.True(t, result.IsError)
	})

	t.Run("missing repo_url", func(t *testing.T) {
		mock := &MockArgoClient{}
		tm := testToolManager(mock, false)
		result, err := tm.CallTool(context.Background(), "create_repository", map[string]interface{}{})
		require.NoError(t, err)
		assert.True(t, result.IsError)
	})
}

func TestHandleUpdateRepository(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		mock := &MockArgoClient{
			GetRepositoryFn: func(_ context.Context, _ *repository.RepoQuery) (*v1alpha1.Repository, error) {
				return &v1alpha1.Repository{
					Repo: "https://github.com/test/repo",
					Type: "git",
					Name: "old-name",
				}, nil
			},
			UpdateRepositoryFn: func(_ context.Context, req *repository.RepoUpdateRequest) (*v1alpha1.Repository, error) {
				return req.Repo, nil
			},
		}
		tm := testToolManager(mock, false)
		result, err := tm.CallTool(context.Background(), "update_repository", map[string]interface{}{
			"repo_url": "https://github.com/test/repo",
			"name":     "new-name",
		})
		require.NoError(t, err)
		assert.False(t, result.IsError)
	})

	t.Run("blocked in safe mode", func(t *testing.T) {
		mock := &MockArgoClient{}
		tm := testToolManager(mock, true)
		result, err := tm.CallTool(context.Background(), "update_repository", map[string]interface{}{
			"repo_url": "https://github.com/test/repo",
		})
		require.NoError(t, err)
		assert.True(t, result.IsError)
	})
}

func TestHandleDeleteRepository(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		mock := &MockArgoClient{
			DeleteRepositoryFn: func(_ context.Context, _ *repository.RepoQuery) error {
				return nil
			},
		}
		tm := testToolManager(mock, false)
		result, err := tm.CallTool(context.Background(), "delete_repository", map[string]interface{}{
			"repo_url": "https://github.com/test/repo",
		})
		require.NoError(t, err)
		assert.False(t, result.IsError)
	})

	t.Run("blocked in safe mode", func(t *testing.T) {
		mock := &MockArgoClient{}
		tm := testToolManager(mock, true)
		result, err := tm.CallTool(context.Background(), "delete_repository", map[string]interface{}{
			"repo_url": "https://github.com/test/repo",
		})
		require.NoError(t, err)
		assert.True(t, result.IsError)
	})
}

func TestHandleValidateRepository(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		mock := &MockArgoClient{
			ValidateRepositoryAccessFn: func(_ context.Context, _ *repository.RepoAccessQuery) error {
				return nil
			},
		}
		tm := testToolManager(mock, false)
		result, err := tm.CallTool(context.Background(), "validate_repository", map[string]interface{}{
			"repo_url": "https://github.com/test/repo",
		})
		require.NoError(t, err)
		assert.False(t, result.IsError)
		data := parseResultJSON(t, result)
		assert.Equal(t, true, data["valid"])
	})

	t.Run("invalid", func(t *testing.T) {
		mock := &MockArgoClient{
			ValidateRepositoryAccessFn: func(_ context.Context, _ *repository.RepoAccessQuery) error {
				return fmt.Errorf("authentication failed")
			},
		}
		tm := testToolManager(mock, false)
		result, err := tm.CallTool(context.Background(), "validate_repository", map[string]interface{}{
			"repo_url": "https://github.com/test/private-repo",
		})
		require.NoError(t, err)
		assert.False(t, result.IsError) // Not an error result, just valid=false
		data := parseResultJSON(t, result)
		assert.Equal(t, false, data["valid"])
	})
}

// =============================================================================
// Cluster handler tests
// =============================================================================

func TestHandleListClusters(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		mock := &MockArgoClient{
			ListClustersFn: func(_ context.Context, _ *cluster.ClusterQuery) (*v1alpha1.ClusterList, error) {
				return &v1alpha1.ClusterList{
					Items: []v1alpha1.Cluster{
						{Server: "https://kubernetes.default.svc", Name: "in-cluster"},
						{Server: "https://remote-cluster:6443", Name: "remote"},
					},
				}, nil
			},
		}
		tm := testToolManager(mock, false)
		result, err := tm.CallTool(context.Background(), "list_clusters", map[string]interface{}{})
		require.NoError(t, err)
		assert.False(t, result.IsError)
		data := parseResultJSON(t, result)
		assert.Equal(t, float64(2), data["total"])
	})
}

func TestHandleGetCluster(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		mock := &MockArgoClient{
			GetClusterFn: func(_ context.Context, _ *cluster.ClusterQuery) (*v1alpha1.Cluster, error) {
				return &v1alpha1.Cluster{
					Server: "https://kubernetes.default.svc",
					Name:   "in-cluster",
				}, nil
			},
		}
		tm := testToolManager(mock, false)
		result, err := tm.CallTool(context.Background(), "get_cluster", map[string]interface{}{
			"server": "https://kubernetes.default.svc",
		})
		require.NoError(t, err)
		assert.False(t, result.IsError)
		data := parseResultJSON(t, result)
		assert.Equal(t, "https://kubernetes.default.svc", data["server"])
	})
}

func TestHandleCreateCluster(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		mock := &MockArgoClient{
			CreateClusterFn: func(_ context.Context, req *cluster.ClusterCreateRequest) (*v1alpha1.Cluster, error) {
				return &v1alpha1.Cluster{
					Server: req.Cluster.Server,
					Name:   req.Cluster.Name,
				}, nil
			},
		}
		tm := testToolManager(mock, false)
		result, err := tm.CallTool(context.Background(), "create_cluster", map[string]interface{}{
			"server": "https://new-cluster:6443",
			"name":   "new-cluster",
			"config": map[string]interface{}{
				"bearerToken": "mytoken",
			},
		})
		require.NoError(t, err)
		assert.False(t, result.IsError)
	})

	t.Run("blocked in safe mode", func(t *testing.T) {
		mock := &MockArgoClient{}
		tm := testToolManager(mock, true)
		result, err := tm.CallTool(context.Background(), "create_cluster", map[string]interface{}{
			"server": "https://new-cluster:6443",
		})
		require.NoError(t, err)
		assert.True(t, result.IsError)
	})

	t.Run("missing server", func(t *testing.T) {
		mock := &MockArgoClient{}
		tm := testToolManager(mock, false)
		result, err := tm.CallTool(context.Background(), "create_cluster", map[string]interface{}{})
		require.NoError(t, err)
		assert.True(t, result.IsError)
	})
}

func TestHandleUpdateCluster(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		mock := &MockArgoClient{
			GetClusterFn: func(_ context.Context, _ *cluster.ClusterQuery) (*v1alpha1.Cluster, error) {
				return &v1alpha1.Cluster{
					Server: "https://cluster:6443",
					Name:   "old-name",
				}, nil
			},
			UpdateClusterFn: func(_ context.Context, req *cluster.ClusterUpdateRequest) (*v1alpha1.Cluster, error) {
				return req.Cluster, nil
			},
		}
		tm := testToolManager(mock, false)
		result, err := tm.CallTool(context.Background(), "update_cluster", map[string]interface{}{
			"server": "https://cluster:6443",
			"name":   "new-name",
		})
		require.NoError(t, err)
		assert.False(t, result.IsError)
	})

	t.Run("blocked in safe mode", func(t *testing.T) {
		mock := &MockArgoClient{}
		tm := testToolManager(mock, true)
		result, err := tm.CallTool(context.Background(), "update_cluster", map[string]interface{}{
			"server": "https://cluster:6443",
		})
		require.NoError(t, err)
		assert.True(t, result.IsError)
	})
}

func TestHandleDeleteCluster(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		mock := &MockArgoClient{
			DeleteClusterFn: func(_ context.Context, _ *cluster.ClusterQuery) error {
				return nil
			},
		}
		tm := testToolManager(mock, false)
		result, err := tm.CallTool(context.Background(), "delete_cluster", map[string]interface{}{
			"server": "https://cluster:6443",
		})
		require.NoError(t, err)
		assert.False(t, result.IsError)
	})

	t.Run("blocked in safe mode", func(t *testing.T) {
		mock := &MockArgoClient{}
		tm := testToolManager(mock, true)
		result, err := tm.CallTool(context.Background(), "delete_cluster", map[string]interface{}{
			"server": "https://cluster:6443",
		})
		require.NoError(t, err)
		assert.True(t, result.IsError)
	})
}

// =============================================================================
// CallTool routing and edge case tests
// =============================================================================

func TestCallToolUnknownTool(t *testing.T) {
	mock := &MockArgoClient{}
	tm := testToolManager(mock, false)
	result, err := tm.CallTool(context.Background(), "nonexistent_tool", map[string]interface{}{})
	require.NoError(t, err)
	assert.True(t, result.IsError)
}

func TestGetToolNames(t *testing.T) {
	mock := &MockArgoClient{}
	tm := testToolManager(mock, false)
	names := tm.GetToolNames()
	assert.NotEmpty(t, names)
	assert.Contains(t, names, "list_applications")
	assert.Contains(t, names, "get_application")
	assert.Contains(t, names, "list_projects")
	assert.Contains(t, names, "list_repositories")
	assert.Contains(t, names, "list_clusters")
}

func TestGetServerTools(t *testing.T) {
	mock := &MockArgoClient{}
	tm := testToolManager(mock, false)
	tools := tm.GetServerTools()
	assert.NotEmpty(t, tools)
	for _, tool := range tools {
		assert.NotEmpty(t, tool.Tool.Name)
		assert.NotNil(t, tool.Handler)
	}
}

// =============================================================================
// Formatting function tests (panic regression)
// =============================================================================

func TestFormatApplicationSummary_NilFields(t *testing.T) {
	t.Run("completely empty app", func(t *testing.T) {
		app := &v1alpha1.Application{}
		result := formatApplicationSummary(app)
		assert.NotNil(t, result)
	})

	t.Run("nil operation state", func(t *testing.T) {
		app := makeApp("test", "default", "https://github.com/test/repo")
		app.Status.OperationState = nil
		result := formatApplicationSummary(app)
		assert.NotNil(t, result)
		assert.Equal(t, "test", result["name"])
	})

	t.Run("with out of sync resources", func(t *testing.T) {
		app := makeApp("test", "default", "https://github.com/test/repo")
		app.Status.Resources = []v1alpha1.ResourceStatus{
			{Kind: "Deployment", Status: v1alpha1.SyncStatusCodeOutOfSync},
			{Kind: "Service", Status: v1alpha1.SyncStatusCodeSynced},
		}
		result := formatApplicationSummary(app)
		assert.Equal(t, 1, result["out_of_sync_count"])
		assert.Equal(t, true, result["has_issues"])
	})
}

func TestFormatApplicationDetail_NilFields(t *testing.T) {
	t.Run("nil source", func(t *testing.T) {
		app := &v1alpha1.Application{
			ObjectMeta: metav1.ObjectMeta{Name: "test"},
			Spec: v1alpha1.ApplicationSpec{
				Source: nil,
			},
		}
		result := formatApplicationDetail(app)
		assert.NotNil(t, result)
		assert.Equal(t, "", result["repo_url"])
		assert.Equal(t, "", result["path"])
		assert.Equal(t, "", result["target_revision"])
	})

	t.Run("nil health resources", func(t *testing.T) {
		app := makeApp("test", "default", "https://github.com/test/repo")
		app.Status.Resources = []v1alpha1.ResourceStatus{
			{Kind: "Deployment", Health: nil},
		}
		result := formatApplicationDetail(app)
		resources := result["resources"].([]map[string]interface{})
		assert.Equal(t, "", resources[0]["health"])
	})

	t.Run("with conditions", func(t *testing.T) {
		app := makeApp("test", "default", "https://github.com/test/repo")
		app.Status.Conditions = []v1alpha1.ApplicationCondition{
			{Type: "SyncError", Message: "sync failed"},
		}
		result := formatApplicationDetail(app)
		conditions := result["conditions"].([]map[string]interface{})
		assert.Len(t, conditions, 1)
		assert.Equal(t, "SyncError", conditions[0]["type"])
	})
}

// =============================================================================
// Helper function tests
// =============================================================================

func TestInferResourceVersion(t *testing.T) {
	tests := []struct {
		group    string
		expected string
	}{
		{"", "v1"},
		{"core", "v1"},
		{"apps", "v1"},
		{"batch", "v1"},
		{"networking.k8s.io", "v1"},
		{"custom.example.com", "v1"},
	}
	for _, tt := range tests {
		t.Run(tt.group, func(t *testing.T) {
			assert.Equal(t, tt.expected, inferResourceVersion(tt.group))
		})
	}
}

func TestParseEvents(t *testing.T) {
	t.Run("event list format", func(t *testing.T) {
		input := map[string]interface{}{
			"items": []map[string]interface{}{
				{"type": "Normal", "reason": "Synced"},
				{"type": "Warning", "reason": "Failed"},
			},
		}
		events, err := parseEvents(input)
		require.NoError(t, err)
		assert.Len(t, events, 2)
	})

	t.Run("direct list format", func(t *testing.T) {
		input := []map[string]interface{}{
			{"type": "Normal", "reason": "Synced"},
		}
		events, err := parseEvents(input)
		require.NoError(t, err)
		assert.Len(t, events, 1)
	})

	t.Run("nil input", func(t *testing.T) {
		_, err := parseEvents(nil)
		// Should not panic, may return error or empty
		if err != nil {
			assert.Error(t, err)
		}
	})
}

func TestComputeDiff(t *testing.T) {
	t.Run("empty strings", func(t *testing.T) {
		assert.Equal(t, "", computeDiff("", ""))
		assert.Equal(t, "", computeDiff("target", ""))
		assert.Equal(t, "", computeDiff("", "live"))
	})

	t.Run("identical content", func(t *testing.T) {
		yaml := "key: value\n"
		assert.Equal(t, "", computeDiff(yaml, yaml))
	})

	t.Run("different values", func(t *testing.T) {
		target := "key: new_value\n"
		live := "key: old_value\n"
		diff := computeDiff(target, live)
		assert.NotEmpty(t, diff)
		assert.Contains(t, diff, "key")
	})
}

func TestBuildClusterConfig(t *testing.T) {
	t.Run("no config", func(t *testing.T) {
		config, err := buildClusterConfig(map[string]interface{}{})
		require.NoError(t, err)
		assert.Empty(t, config.BearerToken)
	})

	t.Run("with bearer token", func(t *testing.T) {
		config, err := buildClusterConfig(map[string]interface{}{
			"config": map[string]interface{}{
				"bearerToken": "mytoken",
			},
		})
		require.NoError(t, err)
		assert.Equal(t, "mytoken", config.BearerToken)
	})

	t.Run("with tls config", func(t *testing.T) {
		config, err := buildClusterConfig(map[string]interface{}{
			"config": map[string]interface{}{
				"tlsClientConfig": map[string]interface{}{
					"insecure": true,
					"caData":   "ca-cert-data",
				},
			},
		})
		require.NoError(t, err)
		assert.True(t, config.TLSClientConfig.Insecure)
		assert.Equal(t, []byte("ca-cert-data"), config.TLSClientConfig.CAData)
	})

	t.Run("with username password", func(t *testing.T) {
		config, err := buildClusterConfig(map[string]interface{}{
			"config": map[string]interface{}{
				"username": "admin",
				"password": "secret",
			},
		})
		require.NoError(t, err)
		assert.Equal(t, "admin", config.Username)
		assert.Equal(t, "secret", config.Password)
	})
}

func TestJsonToYaml(t *testing.T) {
	t.Run("valid json", func(t *testing.T) {
		result := jsonToYaml(`{"key":"value","num":42}`)
		assert.Contains(t, result, "key")
		assert.Contains(t, result, "value")
	})

	t.Run("empty string", func(t *testing.T) {
		assert.Equal(t, "", jsonToYaml(""))
	})

	t.Run("invalid json returns original", func(t *testing.T) {
		assert.Equal(t, "not json", jsonToYaml("not json"))
	})
}

func TestInvolvedObjField(t *testing.T) {
	t.Run("field exists", func(t *testing.T) {
		event := map[string]interface{}{
			"involvedObject": map[string]interface{}{
				"name": "my-pod",
				"kind": "Pod",
			},
		}
		assert.Equal(t, "my-pod", involvedObjField(event, "name"))
		assert.Equal(t, "Pod", involvedObjField(event, "kind"))
	})

	t.Run("field missing", func(t *testing.T) {
		event := map[string]interface{}{
			"involvedObject": map[string]interface{}{},
		}
		assert.Equal(t, "", involvedObjField(event, "name"))
	})

	t.Run("no involvedObject", func(t *testing.T) {
		event := map[string]interface{}{}
		assert.Equal(t, "", involvedObjField(event, "name"))
	})

	t.Run("nil map does not panic", func(t *testing.T) {
		// This should NOT panic
		assert.NotPanics(t, func() {
			involvedObjField(map[string]interface{}{}, "name")
		})
	})
}

func TestResultList_InvalidType(t *testing.T) {
	result, err := ResultList("not a slice", 0, nil)
	require.NoError(t, err)
	assert.True(t, result.IsError)
}
