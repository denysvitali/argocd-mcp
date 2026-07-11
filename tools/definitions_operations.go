package tools

import "github.com/mark3labs/mcp-go/mcp"

// operationsToolDefinitions returns the MCP tool definitions for the operations domain.
func operationsToolDefinitions() []mcp.Tool {
	return []mcp.Tool{
		{
			Name:        "terminate_operation",
			Description: "Terminate the currently running operation (sync, rollback, etc.) on an application. Use this when an operation is stuck and you get 'another operation is already in progress' errors.",
			InputSchema: mcp.ToolInputSchema{
				Type: "object",
				Properties: map[string]interface{}{
					"name": map[string]interface{}{
						"type":        "string",
						"description": "Application name (required)",
					},
					"app_namespace": map[string]interface{}{
						"type":        "string",
						"description": "Application namespace (optional, for multi-namespace setups)",
					},
					"project": map[string]interface{}{
						"type":        "string",
						"description": "Project name (optional)",
					},
				},
				Required: []string{"name"},
			},
		},
		{
			Name:        "restart_pod",
			Description: "Delete a pod within an ArgoCD application to trigger a restart by its controller (Deployment, StatefulSet, etc.). This is useful when a spec update (e.g. image change) has been synced but running pods haven't picked it up.",
			InputSchema: mcp.ToolInputSchema{
				Type: "object",
				Properties: map[string]interface{}{
					"name": map[string]interface{}{
						"type":        "string",
						"description": "Application name (required)",
					},
					"pod_name": map[string]interface{}{
						"type":        "string",
						"description": "Name of the pod to restart (required)",
					},
					"namespace": map[string]interface{}{
						"type":        "string",
						"description": "Pod namespace (required)",
					},
				},
				Required: []string{"name", "pod_name", "namespace"},
			},
		},
		{
			Name:        "refresh_application",
			Description: "Force ArgoCD to re-fetch the application manifests from Git and refresh the application state. Use 'hard' refresh to invalidate the manifest cache and re-read from the repository. This is useful when you've pushed new commits and want ArgoCD to pick them up immediately instead of waiting for the polling interval.",
			InputSchema: mcp.ToolInputSchema{
				Type: "object",
				Properties: map[string]interface{}{
					"name": map[string]interface{}{
						"type":        "string",
						"description": "Application name (required)",
					},
					"refresh_type": map[string]interface{}{
						"type":        "string",
						"description": "Refresh type: 'normal' (check for new commits) or 'hard' (invalidate manifest cache and re-read everything). Default: 'hard'",
						"enum":        []string{"normal", "hard"},
					},
				},
				Required: []string{"name"},
			},
		},
		{
			Name:        "delete_hook",
			Description: "Delete a hook resource (PreSync, Sync, PostSync, SyncFail, Skip) from an application. Hooks are protected from deletion via the generic delete_application_resource endpoint. Use this tool to remove stuck hooks that block sync operations.",
			InputSchema: mcp.ToolInputSchema{
				Type: "object",
				Properties: map[string]interface{}{
					"name": map[string]interface{}{
						"type":        "string",
						"description": "Application name (required)",
					},
					"hook_name": map[string]interface{}{
						"type":        "string",
						"description": "Name of the hook resource to delete (required)",
					},
					"namespace": map[string]interface{}{
						"type":        "string",
						"description": "Hook resource namespace (optional, auto-detected from resource tree if omitted)",
					},
					"hook_type": map[string]interface{}{
						"type":        "string",
						"description": "Hook phase to match: PreSync, Sync, PostSync, SyncFail, Skip (optional, deletes all matching hooks if omitted)",
					},
				},
				Required: []string{"name", "hook_name"},
			},
		},
	}
}
