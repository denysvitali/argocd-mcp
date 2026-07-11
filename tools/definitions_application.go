package tools

import "github.com/mark3labs/mcp-go/mcp"

// applicationToolDefinitions returns the MCP tool definitions for the application domain.
func applicationToolDefinitions() []mcp.Tool {
	return []mcp.Tool{
		{
			Name:        "list_applications",
			Description: "List all applications with optional filtering by name or project",
			InputSchema: mcp.ToolInputSchema{
				Type: "object",
				Properties: map[string]interface{}{
					"name": map[string]interface{}{
						"type":        "string",
						"description": "Filter applications by name (partial match)",
					},
					"project": map[string]interface{}{
						"type":        "string",
						"description": "Filter applications by project name",
					},
					"limit": map[string]interface{}{
						"type":        "integer",
						"description": "Maximum number of applications to return (default: 50, max: 100)",
					},
				},
			},
		},
		{
			Name:        "get_application",
			Description: "Get detailed information about a specific application",
			InputSchema: mcp.ToolInputSchema{
				Type: "object",
				Properties: map[string]interface{}{
					"name": map[string]interface{}{
						"type":        "string",
						"description": "Application name (required)",
					},
				},
				Required: []string{"name"},
			},
		},
		{
			Name:        "create_application",
			Description: "Create a new ArgoCD application",
			InputSchema: mcp.ToolInputSchema{
				Type: "object",
				Properties: map[string]interface{}{
					"name": map[string]interface{}{
						"type":        "string",
						"description": "Application name (required)",
					},
					"project": map[string]interface{}{
						"type":        "string",
						"description": "Project name (required)",
					},
					"repo_url": map[string]interface{}{
						"type":        "string",
						"description": "Git repository URL (required)",
					},
					"path": map[string]interface{}{
						"type":        "string",
						"description": "Path to Kubernetes manifests in the repository (required)",
					},
					"target_revision": map[string]interface{}{
						"type":        "string",
						"description": "Target revision (branch, tag, or commit) to sync to (default: HEAD)",
					},
				},
				Required: []string{"name", "project", "repo_url", "path"},
			},
		},
		{
			Name:        "delete_application",
			Description: "Delete an application",
			InputSchema: mcp.ToolInputSchema{
				Type: "object",
				Properties: map[string]interface{}{
					"name": map[string]interface{}{
						"type":        "string",
						"description": "Application name (required)",
					},
					"cascade": map[string]interface{}{
						"type":        "boolean",
						"description": "Cascade delete resources (default: true)",
					},
				},
				Required: []string{"name"},
			},
		},
		{
			Name:        "sync_application",
			Description: "Trigger a manual sync for an application",
			InputSchema: mcp.ToolInputSchema{
				Type: "object",
				Properties: map[string]interface{}{
					"name": map[string]interface{}{
						"type":        "string",
						"description": "Application name (required)",
					},
					"revision": map[string]interface{}{
						"type":        "string",
						"description": "Specific revision to sync to (optional)",
					},
					"prune": map[string]interface{}{
						"type":        "boolean",
						"description": "Prune resources during sync (default: false)",
					},
				},
				Required: []string{"name"},
			},
		},
		{
			Name:        "get_application_manifests",
			Description: "Get the manifests for an application",
			InputSchema: mcp.ToolInputSchema{
				Type: "object",
				Properties: map[string]interface{}{
					"name": map[string]interface{}{
						"type":        "string",
						"description": "Application name (required)",
					},
					"revision": map[string]interface{}{
						"type":        "string",
						"description": "Specific revision to get manifests for (optional)",
					},
				},
				Required: []string{"name"},
			},
		},
		{
			Name:        "get_application_diff",
			Description: "Get the diff between live and desired state for an application",
			InputSchema: mcp.ToolInputSchema{
				Type: "object",
				Properties: map[string]interface{}{
					"name": map[string]interface{}{
						"type":        "string",
						"description": "Application name (required)",
					},
					"limit": map[string]interface{}{
						"type":        "integer",
						"description": "Maximum number of resources to show diff for (default: 20)",
					},
				},
				Required: []string{"name"},
			},
		},
		{
			Name:        "get_application_events",
			Description: "Get events for an application, optionally filtered by a specific resource",
			InputSchema: mcp.ToolInputSchema{
				Type: "object",
				Properties: map[string]interface{}{
					"name": map[string]interface{}{
						"type":        "string",
						"description": "Application name (required)",
					},
					"resource_name": map[string]interface{}{
						"type":        "string",
						"description": "Filter events by resource name",
					},
					"group": map[string]interface{}{
						"type":        "string",
						"description": "Filter events by resource group (e.g., apps, core)",
					},
					"kind": map[string]interface{}{
						"type":        "string",
						"description": "Filter events by resource kind (e.g., Deployment, Pod)",
					},
					"namespace": map[string]interface{}{
						"type":        "string",
						"description": "Filter events by resource namespace",
					},
					"limit": map[string]interface{}{
						"type":        "integer",
						"description": "Maximum number of events to return (default: 20)",
					},
				},
				Required: []string{"name"},
			},
		},
		{
			Name:        "update_application",
			Description: "Update an existing application",
			InputSchema: mcp.ToolInputSchema{
				Type: "object",
				Properties: map[string]interface{}{
					"name": map[string]interface{}{
						"type":        "string",
						"description": "Application name (required)",
					},
					"project": map[string]interface{}{
						"type":        "string",
						"description": "Project name (optional)",
					},
					"repo_url": map[string]interface{}{
						"type":        "string",
						"description": "Git repository URL (optional)",
					},
					"path": map[string]interface{}{
						"type":        "string",
						"description": "Path to Kubernetes manifests (optional)",
					},
					"target_revision": map[string]interface{}{
						"type":        "string",
						"description": "Target revision (optional)",
					},
				},
				Required: []string{"name"},
			},
		},
		{
			Name:        "rollback_application",
			Description: "Rollback an application to a previous revision",
			InputSchema: mcp.ToolInputSchema{
				Type: "object",
				Properties: map[string]interface{}{
					"name": map[string]interface{}{
						"type":        "string",
						"description": "Application name (required)",
					},
					"revision": map[string]interface{}{
						"type":        "string",
						"description": "Revision to rollback to (required)",
					},
				},
				Required: []string{"name", "revision"},
			},
		},
		{
			Name:        "list_resource_actions",
			Description: "List available actions for a resource in an application",
			InputSchema: mcp.ToolInputSchema{
				Type: "object",
				Properties: map[string]interface{}{
					"name": map[string]interface{}{
						"type":        "string",
						"description": "Application name (required)",
					},
					"group": map[string]interface{}{
						"type":        "string",
						"description": "Resource group (e.g., apps, core)",
					},
					"kind": map[string]interface{}{
						"type":        "string",
						"description": "Resource kind (e.g., Deployment, Pod)",
					},
					"namespace": map[string]interface{}{
						"type":        "string",
						"description": "Resource namespace",
					},
					"resource_name": map[string]interface{}{
						"type":        "string",
						"description": "Resource name (required)",
					},
				},
				Required: []string{"name", "kind", "resource_name"},
			},
		},
		{
			Name:        "run_resource_action",
			Description: "Run an action on a resource in an application",
			InputSchema: mcp.ToolInputSchema{
				Type: "object",
				Properties: map[string]interface{}{
					"name": map[string]interface{}{
						"type":        "string",
						"description": "Application name (required)",
					},
					"group": map[string]interface{}{
						"type":        "string",
						"description": "Resource group (e.g., apps, core)",
					},
					"kind": map[string]interface{}{
						"type":        "string",
						"description": "Resource kind (e.g., Deployment, Pod)",
					},
					"namespace": map[string]interface{}{
						"type":        "string",
						"description": "Resource namespace",
					},
					"resource_name": map[string]interface{}{
						"type":        "string",
						"description": "Resource name",
					},
					"action": map[string]interface{}{
						"type":        "string",
						"description": "Action to run (e.g., restart)",
					},
				},
				Required: []string{"name", "group", "kind", "resource_name", "action"},
			},
		},
		{
			Name:        "get_application_resource",
			Description: "Get details of a specific resource in an application",
			InputSchema: mcp.ToolInputSchema{
				Type: "object",
				Properties: map[string]interface{}{
					"name": map[string]interface{}{
						"type":        "string",
						"description": "Application name (required)",
					},
					"group": map[string]interface{}{
						"type":        "string",
						"description": "Resource group (e.g., apps, core)",
					},
					"kind": map[string]interface{}{
						"type":        "string",
						"description": "Resource kind (e.g., Deployment, Pod)",
					},
					"namespace": map[string]interface{}{
						"type":        "string",
						"description": "Resource namespace",
					},
					"resource_name": map[string]interface{}{
						"type":        "string",
						"description": "Resource name (required)",
					},
				},
				Required: []string{"name", "kind", "resource_name"},
			},
		},
		{
			Name:        "patch_application_resource",
			Description: "Patch a resource in an application using JSON patch",
			InputSchema: mcp.ToolInputSchema{
				Type: "object",
				Properties: map[string]interface{}{
					"name": map[string]interface{}{
						"type":        "string",
						"description": "Application name (required)",
					},
					"group": map[string]interface{}{
						"type":        "string",
						"description": "Resource group (e.g., apps, core)",
					},
					"kind": map[string]interface{}{
						"type":        "string",
						"description": "Resource kind (e.g., Deployment, Pod)",
					},
					"namespace": map[string]interface{}{
						"type":        "string",
						"description": "Resource namespace",
					},
					"resource_name": map[string]interface{}{
						"type":        "string",
						"description": "Resource name (required)",
					},
					"patch": map[string]interface{}{
						"type":        "string",
						"description": "JSON patch to apply (required)",
					},
					"patch_type": map[string]interface{}{
						"type":        "string",
						"description": "Patch type: merge, json, or strategic (default: merge)",
					},
				},
				Required: []string{"name", "kind", "resource_name", "patch"},
			},
		},
		{
			Name:        "delete_application_resource",
			Description: "Delete a resource from an application",
			InputSchema: mcp.ToolInputSchema{
				Type: "object",
				Properties: map[string]interface{}{
					"name": map[string]interface{}{
						"type":        "string",
						"description": "Application name (required)",
					},
					"group": map[string]interface{}{
						"type":        "string",
						"description": "Resource group (e.g., apps, core)",
					},
					"kind": map[string]interface{}{
						"type":        "string",
						"description": "Resource kind (e.g., Deployment, Pod)",
					},
					"namespace": map[string]interface{}{
						"type":        "string",
						"description": "Resource namespace",
					},
					"resource_name": map[string]interface{}{
						"type":        "string",
						"description": "Resource name (required)",
					},
					"force": map[string]interface{}{
						"type":        "boolean",
						"description": "Force deletion (default: false)",
					},
					"orphan": map[string]interface{}{
						"type":        "boolean",
						"description": "Orphan the resource (default: false)",
					},
				},
				Required: []string{"name", "kind", "resource_name"},
			},
		},
		{
			Name:        "get_logs",
			Description: "Get logs from pods/resources in an application",
			InputSchema: mcp.ToolInputSchema{
				Type: "object",
				Properties: map[string]interface{}{
					"name": map[string]interface{}{
						"type":        "string",
						"description": "Application name (required)",
					},
					"namespace": map[string]interface{}{
						"type":        "string",
						"description": "Resource namespace",
					},
					"pod_name": map[string]interface{}{
						"type":        "string",
						"description": "Specific pod name (optional, can infer from kind/resource_name)",
					},
					"container": map[string]interface{}{
						"type":        "string",
						"description": "Container name (optional, defaults to first container)",
					},
					"kind": map[string]interface{}{
						"type":        "string",
						"description": "Resource kind (e.g., Pod, Deployment)",
					},
					"group": map[string]interface{}{
						"type":        "string",
						"description": "Resource group (e.g., apps, core)",
					},
					"resource_name": map[string]interface{}{
						"type":        "string",
						"description": "Resource name",
					},
					"tail_lines": map[string]interface{}{
						"type":        "integer",
						"description": "Number of lines to return (default: 100, max: 500)",
					},
					"since_seconds": map[string]interface{}{
						"type":        "integer",
						"description": "Show logs since N seconds ago",
					},
					"filter": map[string]interface{}{
						"type":        "string",
						"description": "Regex pattern to filter log lines",
					},
					"previous": map[string]interface{}{
						"type":        "boolean",
						"description": "Return previous terminated container logs (default: false)",
					},
				},
				Required: []string{"name"},
			},
		},
		{
			Name:        "get_resource_tree",
			Description: "Get the resource hierarchy tree for an application, showing parent-child relationships between all Kubernetes resources",
			InputSchema: mcp.ToolInputSchema{
				Type: "object",
				Properties: map[string]interface{}{
					"name": map[string]interface{}{
						"type":        "string",
						"description": "Application name (required)",
					},
				},
				Required: []string{"name"},
			},
		},
	}
}
