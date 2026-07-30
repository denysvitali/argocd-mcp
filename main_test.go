package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestResolveEndpoint(t *testing.T) {
	tests := []struct {
		name        string
		endpoint    string
		wantKind    string
		wantWarning bool
	}{
		{"empty defaults to stdio", "", endpointStdio, false},
		{"stdio", "stdio", endpointStdio, false},
		{"http", "http", endpointHTTP, false},
		{"streamable-http alias", "streamable-http", endpointHTTP, false},
		{"sse maps to http with a warning", "sse", endpointHTTP, true},
		{"unknown falls back to stdio with a warning", "carrier-pigeon", endpointStdio, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			kind, warning := resolveEndpoint(tt.endpoint)
			assert.Equal(t, tt.wantKind, kind)
			if tt.wantWarning {
				assert.NotEmpty(t, warning)
			} else {
				assert.Empty(t, warning)
			}
		})
	}
}
