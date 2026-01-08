# CLAUDE.md

This file provides guidance to Claude when working with this codebase.

## Project Overview

argocd-mcp is a Model Context Protocol (MCP) server for ArgoCD, allowing LLMs to interact with ArgoCD installations.

## Code Style Guidelines

**USE TYPES. Using `map[string]interface{}` is FORBIDDEN.**

Always define proper struct types for:
- API responses
- Data transfer objects
- Configuration structures
- Event structures

Example of correct approach:
```go
// Instead of:
response := map[string]interface{}{"name": "value"}

// Define a type:
type ApplicationResponse struct {
    Name string `json:"name"`
    Status string `json:"status"`
}

// Then use:
response := ApplicationResponse{Name: "value", Status: "synced"}
```

## Key Technologies

- Go 1.25+
- ArgoCD v3.2.2 API
- MCP (Model Context Protocol)
- Kubernetes client-go

## Common Commands

```bash
# Build the project
go build ./...

# Run tests
go test ./...

# Lint the code
golangci-lint run
```

## Architecture

- `tools/argocd_tools.go` - MCP tool definitions and handlers
- `internal/client/` - ArgoCD API client wrapper
- `cmd/` - Command entrypoints
