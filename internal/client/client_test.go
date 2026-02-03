package client

import (
	"context"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewClient(t *testing.T) {
	logger := logrus.New()
	// Use an invalid server URL - the client creation may or may not fail immediately
	// depending on the apiclient implementation, but we test both cases
	client, err := NewClient(logger, "http://invalid:9999", "test-token", true, false, "", false, "")
	// Client creation may succeed but operations will fail - verify struct is valid
	if err == nil {
		assert.NotNil(t, client)
		assert.Equal(t, "http://invalid:9999", client.server)
	} else {
		// If it does fail, it should be a meaningful error
		assert.Contains(t, err.Error(), "failed to create ArgoCD client")
	}
}

func TestWaitForRateLimit_Cancelled(t *testing.T) {
	logger := logrus.New()
	client, err := NewClient(logger, "http://localhost:8080", "test-token", true, false, "", false, "")
	require.NoError(t, err)

	// Create a cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err = client.WaitForRateLimit(ctx)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "context canceled")
}
