package tools

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/argoproj/argo-cd/v3/pkg/apiclient/application"
	"github.com/argoproj/argo-cd/v3/pkg/apis/application/v1alpha1"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// TestLimitClamping covers the input that used to take the whole server
// down: a negative limit reached items[:limit] unvalidated, and the
// resulting "slice bounds out of range" panic killed the process along
// with every in-flight request.
func TestLimitClamping(t *testing.T) {
	tests := []struct {
		name  string
		value interface{}
		want  int
	}{
		{"absent uses default", nil, 50},
		{"negative uses default", float64(-5), 50},
		{"zero uses default", float64(0), 50},
		{"one is honoured", float64(1), 1},
		{"in range is honoured", float64(37), 37},
		{"above ceiling is capped", float64(500), MaxLimit},
		{"absurdly large is capped", float64(1 << 40), MaxLimit},
		{"wrong type uses default", "not a number", 50},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := map[string]interface{}{}
			if tt.value != nil {
				args["limit"] = tt.value
			}
			assert.Equal(t, tt.want, Limit(args, "limit", 50, MaxLimit))
		})
	}
}

// TestListApplicationsSurvivesHostileLimits drives the handler that
// panicked, through its real slicing path.
func TestListApplicationsSurvivesHostileLimits(t *testing.T) {
	items := make([]v1alpha1.Application, 0, 120)
	for i := range 120 {
		items = append(items, v1alpha1.Application{
			ObjectMeta: metav1.ObjectMeta{Name: fmt.Sprintf("app-%d", i), Namespace: "argocd"},
		})
	}

	for _, limit := range []float64{-5, -1, 0, 1, 500, 1 << 40} {
		t.Run(fmt.Sprintf("limit=%.0f", limit), func(t *testing.T) {
			tm := NewToolManager(&MockArgoClient{
				ListApplicationsFn: func(context.Context, *application.ApplicationQuery) (*v1alpha1.ApplicationList, error) {
					list := &v1alpha1.ApplicationList{Items: make([]v1alpha1.Application, len(items))}
					copy(list.Items, items)
					return list, nil
				},
			}, newTestLogger(), true, false)

			require.NotPanics(t, func() {
				result, err := tm.CallTool(context.Background(), toolListApplications, map[string]interface{}{"limit": limit})
				require.NoError(t, err)
				require.False(t, result.IsError)
			})
		})
	}
}

// TestEveryListToolSurvivesNegativeLimit guards the remaining handlers
// that slice by a caller-supplied limit.
func TestEveryListToolSurvivesNegativeLimit(t *testing.T) {
	for _, tool := range []string{
		toolListApplications,
		toolListProjects,
		toolListClusters,
		toolListRepositories,
		toolListApplicationSets,
	} {
		t.Run(tool, func(t *testing.T) {
			tm := NewToolManager(&MockArgoClient{}, newTestLogger(), true, false)
			require.NotPanics(t, func() {
				_, err := tm.CallTool(context.Background(), tool, map[string]interface{}{"limit": float64(-5)})
				require.NoError(t, err)
			})
		})
	}
}

func TestErrorFromRPCStripsTransportNoise(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{
			"not found",
			errors.New(`rpc error: code = NotFound desc = appprojects.argoproj.io "x" not found`),
			`appprojects.argoproj.io "x" not found`,
		},
		{
			"permission denied is labelled",
			errors.New("rpc error: code = PermissionDenied desc = repositories, create"),
			"permission denied: repositories, create",
		},
		{
			"unavailable is labelled",
			errors.New("rpc error: code = Unavailable desc = connection refused"),
			"ArgoCD unreachable: connection refused",
		},
		{
			"plain errors pass through",
			errors.New("something broke"),
			"something broke",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := errorFromRPC(tt.err)
			require.True(t, result.IsError)
			assert.Equal(t, tt.want, result.Content[0].(*mcp.TextContent).Text)
		})
	}
}

func TestErrorFromRPCHandlesNil(t *testing.T) {
	result := errorFromRPC(nil)
	require.True(t, result.IsError)
	assert.NotEmpty(t, result.Content)
}
