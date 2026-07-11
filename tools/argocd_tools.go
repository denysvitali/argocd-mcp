package tools

import (
	"context"
	"fmt"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/sirupsen/logrus"
)

// Default timeout constant
const defaultSyncTimeout = 60 * time.Second

// Tool name constants
const (
	// Applications
	toolListApplications       = "list_applications"
	toolGetApplication         = "get_application"
	toolCreateApplication      = "create_application"
	toolUpdateApplication      = "update_application"
	toolDeleteApplication      = "delete_application"
	toolSyncApplication        = "sync_application"
	toolRollbackApplication    = "rollback_application"
	toolRefreshApplication     = "refresh_application"
	toolGetApplicationManifest = "get_application_manifests"
	toolGetApplicationDiff     = "get_application_diff"
	toolGetApplicationEvents   = "get_application_events"
	toolGetLogs                = "get_logs"
	toolGetResourceTree        = "get_resource_tree"

	// Application resources
	toolListResourceActions       = "list_resource_actions"
	toolGetApplicationResource    = "get_application_resource"
	toolRunResourceAction         = "run_resource_action"
	toolPatchApplicationResource  = "patch_application_resource"
	toolDeleteApplicationResource = "delete_application_resource"

	// Operations
	toolTerminateOperation = "terminate_operation"
	toolRestartPod         = "restart_pod"
	toolDeleteHook         = "delete_hook"

	// Projects
	toolListProjects    = "list_projects"
	toolGetProject      = "get_project"
	toolCreateProject   = "create_project"
	toolUpdateProject   = "update_project"
	toolDeleteProject   = "delete_project"
	toolGetProjectEvent = "get_project_events"

	// Repositories
	toolListRepositories   = "list_repositories"
	toolGetRepository      = "get_repository"
	toolCreateRepository   = "create_repository"
	toolUpdateRepository   = "update_repository"
	toolDeleteRepository   = "delete_repository"
	toolValidateRepository = "validate_repository"

	// Clusters
	toolListClusters  = "list_clusters"
	toolGetCluster    = "get_cluster"
	toolCreateCluster = "create_cluster"
	toolUpdateCluster = "update_cluster"
	toolDeleteCluster = "delete_cluster"

	// ApplicationSets
	toolListApplicationSets   = "list_applicationsets"
	toolGetApplicationSet     = "get_applicationset"
	toolPreviewApplicationSet = "preview_applicationset"
	toolCreateApplicationSet  = "create_applicationset"
	toolDeleteApplicationSet  = "delete_applicationset"

	// Diagnostics
	toolDiagnoseApplication       = "diagnose_application"
	toolAnalyzeResourceEfficiency = "analyze_resource_efficiency"
)

// writeTools lists tools that mutate state and are blocked in safe (read-only) mode.
var writeTools = map[string]bool{
	toolCreateApplication:        true,
	toolUpdateApplication:        true,
	toolSyncApplication:          true,
	toolRollbackApplication:      true,
	toolRefreshApplication:       true,
	toolRunResourceAction:        true,
	toolPatchApplicationResource: true,
	toolTerminateOperation:       true,
	toolCreateProject:            true,
	toolUpdateProject:            true,
	toolCreateRepository:         true,
	toolUpdateRepository:         true,
	toolCreateCluster:            true,
	toolUpdateCluster:            true,
	toolCreateApplicationSet:     true,
}

// deleteTools lists tools that destroy resources and require explicit delete permission.
// They are also blocked in safe mode.
var deleteTools = map[string]bool{
	toolDeleteApplication:         true,
	toolDeleteApplicationResource: true,
	toolDeleteHook:                true,
	toolRestartPod:                true,
	toolDeleteProject:             true,
	toolDeleteRepository:          true,
	toolDeleteCluster:             true,
	toolDeleteApplicationSet:      true,
}

// ToolManager manages the MCP tools for ArgoCD
type ToolManager struct {
	client       ArgoClient
	kubeMetrics  KubeMetricsClient
	logger       *logrus.Logger
	tools        []mcp.Tool
	safeMode     bool
	allowDeletes bool
}

// NewToolManager creates a new tool manager
func NewToolManager(client ArgoClient, logger *logrus.Logger, safeMode bool, allowDeletes bool) *ToolManager {
	return &ToolManager{
		client:       client,
		logger:       logger,
		tools:        []mcp.Tool{},
		safeMode:     safeMode,
		allowDeletes: allowDeletes,
	}
}

// NewToolManagerWithMetrics creates a new tool manager with an optional Kubernetes metrics client.
// When kubeMetrics is non-nil, the analyze_resource_efficiency tool will include live usage data.
func NewToolManagerWithMetrics(client ArgoClient, kubeMetrics KubeMetricsClient, logger *logrus.Logger, safeMode bool, allowDeletes bool) *ToolManager {
	return &ToolManager{
		client:       client,
		kubeMetrics:  kubeMetrics,
		logger:       logger,
		tools:        []mcp.Tool{},
		safeMode:     safeMode,
		allowDeletes: allowDeletes,
	}
}

// GetServerTools returns tools filtered by the current access mode.
// Write and delete tools are omitted in safe (read-only) mode; delete tools
// are also omitted when allowDeletes is false.
func (tm *ToolManager) GetServerTools() []server.ServerTool {
	tm.defineTools()
	var serverTools []server.ServerTool
	for _, tool := range tm.tools {
		if tm.safeMode && (writeTools[tool.Name] || deleteTools[tool.Name]) {
			continue
		}
		if !tm.allowDeletes && deleteTools[tool.Name] {
			continue
		}
		serverTools = append(serverTools, server.ServerTool{
			Tool:    tool,
			Handler: tm.getToolHandler(tool.Name),
		})
	}
	return serverTools
}

// CallTool calls a tool by name and returns the result
func (tm *ToolManager) CallTool(ctx context.Context, name string, arguments map[string]interface{}) (*mcp.CallToolResult, error) {
	handler := tm.getToolHandler(name)
	if handler == nil {
		return nil, fmt.Errorf("unknown tool: %s", name)
	}
	// Create a proper CallToolRequest
	request := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name:      name,
			Arguments: arguments,
		},
	}
	return handler(ctx, request)
}

// GetToolNames returns all available tool names
func (tm *ToolManager) GetToolNames() []string {
	tm.defineTools()
	names := make([]string, len(tm.tools))
	for i, tool := range tm.tools {
		names[i] = tool.Name
	}
	return names
}

// checkSafeMode returns an error result if safe mode is enabled for write operations
func (tm *ToolManager) checkSafeMode(operation string) *mcp.CallToolResult {
	if tm.safeMode {
		return errorResult(fmt.Sprintf("Operation '%s' is not allowed in read-only mode. To enable write operations, start the server with the --read-write flag or set server.safe_mode: false in your config.", operation))
	}
	return nil
}

// checkDeleteAllowed returns an error result if delete operations are not explicitly enabled.
// Delete is gated separately from general write access because it is irreversible.
func (tm *ToolManager) checkDeleteAllowed(operation string) *mcp.CallToolResult {
	if tm.safeMode {
		return errorResult(fmt.Sprintf("Operation '%s' is not allowed in read-only mode. To enable write operations, start the server with the --read-write flag or set server.safe_mode: false in your config.", operation))
	}
	if !tm.allowDeletes {
		return errorResult(fmt.Sprintf("Operation '%s' requires delete permissions. Use the --allow-deletes flag or set server.allow_deletes: true in your config.", operation))
	}
	return nil
}
