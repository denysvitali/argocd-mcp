package tools

import (
	"context"
	"testing"

	"github.com/argoproj/argo-cd/v3/pkg/apiclient/application"
	"github.com/argoproj/argo-cd/v3/pkg/apis/application/v1alpha1"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func textResultFor(t *testing.T, text string) *mcp.CallToolResult {
	t.Helper()
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: text}},
	}
}

func TestAttachStructuredContent(t *testing.T) {
	t.Run("mirrors an object payload", func(t *testing.T) {
		result := textResultFor(t, "items:\n- name: foo\ntotal: 1\n")
		attachStructuredContent(result)

		structured, ok := result.StructuredContent.(map[string]interface{})
		require.True(t, ok, "expected a map, got %T", result.StructuredContent)
		assert.Equal(t, float64(1), structured["total"])
		assert.Len(t, structured["items"], 1)

		// The text content is left alone so clients that ignore
		// structured content are unaffected.
		assert.Equal(t, "items:\n- name: foo\ntotal: 1\n", result.Content[0].(*mcp.TextContent).Text)
	})

	t.Run("skips error results", func(t *testing.T) {
		result := textResultFor(t, "something: broke\n")
		result.IsError = true
		attachStructuredContent(result)
		assert.Nil(t, result.StructuredContent)
	})

	t.Run("skips non-object payloads", func(t *testing.T) {
		// Plain log text is not YAML-decodable into an object.
		result := textResultFor(t, "# app logs (2 lines)\nboot ok\n")
		attachStructuredContent(result)
		assert.Nil(t, result.StructuredContent)
	})

	t.Run("skips nil and empty results", func(t *testing.T) {
		attachStructuredContent(nil) // must not panic

		empty := &mcp.CallToolResult{}
		attachStructuredContent(empty)
		assert.Nil(t, empty.StructuredContent)
	})

	t.Run("does not overwrite existing structured content", func(t *testing.T) {
		result := textResultFor(t, "total: 1\n")
		result.StructuredContent = map[string]interface{}{"kept": true}
		attachStructuredContent(result)
		assert.Equal(t, map[string]interface{}{"kept": true}, result.StructuredContent)
	})
}

func TestStructuredOutputIsOptIn(t *testing.T) {
	tm := &ToolManager{}
	assert.False(t, tm.structuredOutput, "structured output must default to off")

	tm.SetStructuredOutput(true)
	assert.True(t, tm.structuredOutput)
}

// TestStructuredOutputThroughDispatch drives a real tool call end to end
// through the dispatcher, which is where structured content is attached.
func TestStructuredOutputThroughDispatch(t *testing.T) {
	newMock := func() *MockArgoClient {
		return &MockArgoClient{
			ListApplicationsFn: func(_ context.Context, _ *application.ApplicationQuery) (*v1alpha1.ApplicationList, error) {
				return &v1alpha1.ApplicationList{
					Items: []v1alpha1.Application{
						{ObjectMeta: metav1.ObjectMeta{Name: "guestbook", Namespace: "argocd"}},
					},
				}, nil
			},
		}
	}

	t.Run("off by default", func(t *testing.T) {
		tm := NewToolManager(newMock(), newTestLogger(), true, false)

		result, err := tm.CallTool(context.Background(), toolListApplications, map[string]interface{}{})
		require.NoError(t, err)
		require.False(t, result.IsError)
		assert.Nil(t, result.StructuredContent)
		assert.NotEmpty(t, result.Content)
	})

	t.Run("enabled", func(t *testing.T) {
		tm := NewToolManager(newMock(), newTestLogger(), true, false)
		tm.SetStructuredOutput(true)

		result, err := tm.CallTool(context.Background(), toolListApplications, map[string]interface{}{})
		require.NoError(t, err)
		require.False(t, result.IsError)

		structured, ok := result.StructuredContent.(map[string]interface{})
		require.True(t, ok, "expected a map, got %T", result.StructuredContent)
		assert.Contains(t, structured, "items")
		assert.Contains(t, structured, "total")

		// Text content is still present for clients that ignore it.
		assert.NotEmpty(t, result.Content)
	})
}
