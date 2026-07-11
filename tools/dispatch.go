package tools

import (
	"context"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// handlerFunc is the signature shared by all tool handlers.
type handlerFunc func(ctx context.Context, arguments map[string]interface{}) (*mcp.CallToolResult, error)

// handlerRegistry maps each tool name to its handler.
func (tm *ToolManager) handlerRegistry() map[string]handlerFunc {
	return map[string]handlerFunc{
		// Applications
		toolListApplications:       tm.handleListApplications,
		toolGetApplication:         tm.handleGetApplication,
		toolCreateApplication:      tm.handleCreateApplication,
		toolUpdateApplication:      tm.handleUpdateApplication,
		toolDeleteApplication:      tm.handleDeleteApplication,
		toolSyncApplication:        tm.handleSyncApplication,
		toolRollbackApplication:    tm.handleRollbackApplication,
		toolRefreshApplication:     tm.handleRefreshApplication,
		toolGetApplicationManifest: tm.handleGetApplicationManifests,
		toolGetApplicationDiff:     tm.handleGetApplicationDiff,
		toolGetApplicationEvents:   tm.handleGetApplicationEvents,
		toolGetLogs:                tm.handleGetLogs,
		toolGetResourceTree:        tm.handleGetResourceTree,

		// Application resources
		toolListResourceActions:       tm.handleListResourceActions,
		toolGetApplicationResource:    tm.handleGetApplicationResource,
		toolRunResourceAction:         tm.handleRunResourceAction,
		toolPatchApplicationResource:  tm.handlePatchApplicationResource,
		toolDeleteApplicationResource: tm.handleDeleteApplicationResource,

		// Operations
		toolTerminateOperation: tm.handleTerminateOperation,
		toolRestartPod:         tm.handleRestartPod,
		toolDeleteHook:         tm.handleDeleteHook,

		// Projects
		toolListProjects:    tm.handleListProjects,
		toolGetProject:      tm.handleGetProject,
		toolCreateProject:   tm.handleCreateProject,
		toolUpdateProject:   tm.handleUpdateProject,
		toolDeleteProject:   tm.handleDeleteProject,
		toolGetProjectEvent: tm.handleGetProjectEvents,

		// Repositories
		toolListRepositories:   tm.handleListRepositories,
		toolGetRepository:      tm.handleGetRepository,
		toolCreateRepository:   tm.handleCreateRepository,
		toolUpdateRepository:   tm.handleUpdateRepository,
		toolDeleteRepository:   tm.handleDeleteRepository,
		toolValidateRepository: tm.handleValidateRepository,

		// Clusters
		toolListClusters:  tm.handleListClusters,
		toolGetCluster:    tm.handleGetCluster,
		toolCreateCluster: tm.handleCreateCluster,
		toolUpdateCluster: tm.handleUpdateCluster,
		toolDeleteCluster: tm.handleDeleteCluster,

		// ApplicationSets
		toolListApplicationSets:   tm.handleListApplicationSets,
		toolGetApplicationSet:     tm.handleGetApplicationSet,
		toolPreviewApplicationSet: tm.handlePreviewApplicationSet,
		toolCreateApplicationSet:  tm.handleCreateApplicationSet,
		toolDeleteApplicationSet:  tm.handleDeleteApplicationSet,

		// Diagnostics
		toolDiagnoseApplication:       tm.handleDiagnoseApplication,
		toolAnalyzeResourceEfficiency: tm.handleAnalyzeResourceEfficiency,
	}
}

// getToolHandler returns the handler for a specific tool
func (tm *ToolManager) getToolHandler(name string) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		arguments, ok := request.Params.Arguments.(map[string]interface{})
		if !ok {
			return errorResult("Invalid arguments format"), nil
		}

		handler, ok := tm.handlerRegistry()[name]
		if !ok {
			return errorResult(fmt.Sprintf("Unknown tool: %s", name)), nil
		}

		ctx, cancel := context.WithTimeout(ctx, defaultSyncTimeout)
		defer cancel()

		return handler(ctx, arguments)
	}
}
