package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	yaml "sigs.k8s.io/yaml"
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
func (tm *ToolManager) getToolHandler(name string) mcp.ToolHandler {
	return func(ctx context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		// The official SDK's low-level ToolHandler hands us the raw
		// arguments as received on the wire; our handlers all work on a
		// decoded map.
		arguments := map[string]interface{}{}
		if raw := request.Params.Arguments; len(raw) > 0 {
			if err := json.Unmarshal(raw, &arguments); err != nil {
				return errorResult("Invalid arguments format"), nil
			}
		}

		handler, ok := tm.handlerRegistry()[name]
		if !ok {
			return errorResult(fmt.Sprintf("Unknown tool: %s", name)), nil
		}

		ctx, cancel := context.WithTimeout(ctx, defaultSyncTimeout)
		defer cancel()

		result, err := handler(ctx, arguments)
		if err != nil {
			return nil, err
		}
		if tm.structuredOutput {
			attachStructuredContent(result)
		}
		return result, nil
	}
}

// attachStructuredContent mirrors a result's YAML text payload into
// CallToolResult.StructuredContent, so clients on the official SDK can
// consume tool output as JSON instead of re-parsing YAML out of a string.
//
// This is opt-in (see ToolManager.structuredOutput) because it repeats the
// whole payload on the wire, and most clients today render only the text
// content — for them the copy is pure context cost, which is exactly what
// the truncation limits in helpers.go exist to control.
//
// Only object-shaped payloads are mirrored: per the MCP spec a tool's
// structured content is validated against its output schema, and every
// payload we produce that is worth addressing by field is an object.
func attachStructuredContent(result *mcp.CallToolResult) {
	if result == nil || result.IsError || result.StructuredContent != nil {
		return
	}
	if len(result.Content) != 1 {
		return
	}
	text, ok := result.Content[0].(*mcp.TextContent)
	if !ok {
		return
	}

	var structured map[string]interface{}
	if err := yaml.Unmarshal([]byte(text.Text), &structured); err != nil {
		return
	}
	if structured == nil {
		return
	}
	result.StructuredContent = structured
}
