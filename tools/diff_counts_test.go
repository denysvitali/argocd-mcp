package tools

import (
	"context"
	"fmt"
	"testing"

	"github.com/argoproj/argo-cd/v3/pkg/apiclient/application"
	"github.com/argoproj/argo-cd/v3/pkg/apis/application/v1alpha1"
	"github.com/denysvitali/argocd-mcp/internal/client"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	yaml "sigs.k8s.io/yaml"
)

func diffResult(t *testing.T, tm *ToolManager, args map[string]interface{}) map[string]interface{} {
	t.Helper()
	result, err := tm.CallTool(context.Background(), toolGetApplicationDiff, args)
	require.NoError(t, err)
	require.False(t, result.IsError)

	var decoded map[string]interface{}
	require.NoError(t, yaml.Unmarshal([]byte(result.Content[0].(*mcp.TextContent).Text), &decoded))
	return decoded
}

// TestDiffReportsDriftTheApplicationKnowsAbout covers the case where the
// tool claimed a clean diff on a genuinely OutOfSync application: a
// resource present in Git but absent from the cluster has nothing to
// modify, so ArgoCD returns Modified=false for it while still marking
// the application OutOfSync.
func TestDiffReportsDriftTheApplicationKnowsAbout(t *testing.T) {
	mock := &MockArgoClient{
		GetManagedResourcesFn: func(context.Context, string) ([]*v1alpha1.ResourceDiff, error) {
			return []*v1alpha1.ResourceDiff{
				{Group: "", Kind: "ConfigMap", Namespace: "ns", Name: "missing-in-cluster", Modified: false},
				{Group: "apps", Kind: "Deployment", Namespace: "ns", Name: "in-sync", Modified: false},
			}, nil
		},
		GetApplicationFn: func(context.Context, *application.ApplicationQuery) (*v1alpha1.Application, error) {
			return &v1alpha1.Application{
				ObjectMeta: metav1.ObjectMeta{Name: "app"},
				Status: v1alpha1.ApplicationStatus{
					Resources: []v1alpha1.ResourceStatus{
						{Group: "", Kind: "ConfigMap", Namespace: "ns", Name: "missing-in-cluster", Status: v1alpha1.SyncStatusCodeOutOfSync},
						{Group: "apps", Kind: "Deployment", Namespace: "ns", Name: "in-sync", Status: v1alpha1.SyncStatusCodeSynced},
					},
				},
			}, nil
		},
	}
	tm := NewToolManager(mock, newTestLogger(), true, false)

	decoded := diffResult(t, tm, map[string]interface{}{"name": "app"})

	assert.Equal(t, float64(1), decoded["out_of_sync_count"], "drift the application reports must not be lost")
	assert.Equal(t, float64(1), decoded["synced_count"])
	outOfSync, ok := decoded["out_of_sync"].([]interface{})
	require.True(t, ok)
	require.Len(t, outOfSync, 1)
	assert.Equal(t, "missing-in-cluster", outOfSync[0].(map[string]interface{})["name"])
}

// TestDiffCountsAreTotalsNotPageSizes covers out_of_sync_count having
// been len(shown): with a small limit it reported the limit back as if
// it were the amount of drift.
func TestDiffCountsAreTotalsNotPageSizes(t *testing.T) {
	var resources []*v1alpha1.ResourceDiff
	var statuses []v1alpha1.ResourceStatus
	for i := range 30 {
		name := fmt.Sprintf("drifted-%d", i)
		resources = append(resources, &v1alpha1.ResourceDiff{Group: "", Kind: "ConfigMap", Namespace: "ns", Name: name, Modified: true})
		statuses = append(statuses, v1alpha1.ResourceStatus{Group: "", Kind: "ConfigMap", Namespace: "ns", Name: name, Status: v1alpha1.SyncStatusCodeOutOfSync})
	}

	tm := NewToolManager(&MockArgoClient{
		GetManagedResourcesFn: func(context.Context, string) ([]*v1alpha1.ResourceDiff, error) {
			return resources, nil
		},
		GetApplicationFn: func(context.Context, *application.ApplicationQuery) (*v1alpha1.Application, error) {
			return &v1alpha1.Application{Status: v1alpha1.ApplicationStatus{Resources: statuses}}, nil
		},
	}, newTestLogger(), true, false)

	decoded := diffResult(t, tm, map[string]interface{}{"name": "app", "limit": float64(3)})

	assert.Equal(t, float64(30), decoded["out_of_sync_count"], "count is of all drift, not of what fitted")
	assert.Equal(t, float64(3), decoded["out_of_sync_shown"])
	assert.Equal(t, true, decoded["limited"])
	assert.Contains(t, decoded["limit_hint"], "3 of 30")
}

// TestDiffSurvivesApplicationLookupFailure: the status cross-check is an
// enhancement, not a dependency.
func TestDiffSurvivesApplicationLookupFailure(t *testing.T) {
	tm := NewToolManager(&MockArgoClient{
		GetManagedResourcesFn: func(context.Context, string) ([]*v1alpha1.ResourceDiff, error) {
			return []*v1alpha1.ResourceDiff{
				{Group: "apps", Kind: "Deployment", Namespace: "ns", Name: "web", Modified: true},
			}, nil
		},
	}, newTestLogger(), true, false)

	decoded := diffResult(t, tm, map[string]interface{}{"name": "app"})
	assert.Equal(t, float64(1), decoded["out_of_sync_count"])
}

func TestDropEmptyLogEntries(t *testing.T) {
	entries := []client.ApplicationLogEntry{
		{Content: "first"},
		{Content: "   "},
		{Content: "second"},
		{Content: ""},
	}
	assert.Len(t, dropEmptyLogEntries(entries), 2)
	assert.Empty(t, dropEmptyLogEntries([]client.ApplicationLogEntry{{Content: ""}}),
		"a stream that is only ArgoCD's terminating empty entry has no lines")
}
