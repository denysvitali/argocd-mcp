package tools

import (
	"context"
	"fmt"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

func TestResult_ListWithZeroItems(t *testing.T) {
	result, err := ResultList([]interface{}{}, 0, nil)
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.False(t, result.IsError)
	assert.Contains(t, result.Content[0].(mcp.TextContent).Text, "items")
	assert.Contains(t, result.Content[0].(mcp.TextContent).Text, "total")
}

func TestResult_ErrorResult(t *testing.T) {
	result, err := Result(nil, fmt.Errorf("test error message"))
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.True(t, result.IsError)
	assert.Equal(t, "test error message", result.Content[0].(mcp.TextContent).Text)
}

func TestIsContextCancelled_Cancelled(t *testing.T) {
	logger := logrus.New()
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	result := IsContextCancelled(ctx, logger)
	assert.True(t, result)
}

func TestIsContextCancelled_NotCancelled(t *testing.T) {
	logger := logrus.New()
	ctx := context.Background()

	result := IsContextCancelled(ctx, logger)
	assert.False(t, result)
}
