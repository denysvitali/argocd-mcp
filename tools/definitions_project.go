package tools

import "github.com/mark3labs/mcp-go/mcp"

// projectToolDefinitions returns the MCP tool definitions for the project domain.
func projectToolDefinitions() []mcp.Tool {
	return []mcp.Tool{
		{
			Name:        "list_projects",
			Description: "List all ArgoCD projects",
			InputSchema: mcp.ToolInputSchema{
				Type: "object",
				Properties: map[string]interface{}{
					"name": map[string]interface{}{
						"type":        "string",
						"description": "Filter projects by name (partial match)",
					},
					"limit": map[string]interface{}{
						"type":        "integer",
						"description": "Maximum number of projects to return (default: 50)",
					},
				},
			},
		},
		{
			Name:        "get_project",
			Description: "Get detailed information about a specific project",
			InputSchema: mcp.ToolInputSchema{
				Type: "object",
				Properties: map[string]interface{}{
					"name": map[string]interface{}{
						"type":        "string",
						"description": "Project name (required)",
					},
				},
				Required: []string{"name"},
			},
		},
		{
			Name:        "create_project",
			Description: "Create a new ArgoCD project",
			InputSchema: mcp.ToolInputSchema{
				Type: "object",
				Properties: map[string]interface{}{
					"name": map[string]interface{}{
						"type":        "string",
						"description": "Project name (required)",
					},
					"description": map[string]interface{}{
						"type":        "string",
						"description": "Project description",
					},
					"source_repos": map[string]interface{}{
						"type":        "array",
						"description": "Allowed source repositories",
						"items": map[string]interface{}{
							"type": "string",
						},
					},
					"destinations": map[string]interface{}{
						"type":        "array",
						"description": "Allowed destinations",
						"items": map[string]interface{}{
							"type": "object",
							"properties": map[string]interface{}{
								"server": map[string]interface{}{
									"type": "string",
								},
								"namespace": map[string]interface{}{
									"type": "string",
								},
							},
						},
					},
				},
				Required: []string{"name"},
			},
		},
		{
			Name:        "update_project",
			Description: "Update an existing project",
			InputSchema: mcp.ToolInputSchema{
				Type: "object",
				Properties: map[string]interface{}{
					"name": map[string]interface{}{
						"type":        "string",
						"description": "Project name (required)",
					},
					"description": map[string]interface{}{
						"type":        "string",
						"description": "Project description",
					},
					"source_repos": map[string]interface{}{
						"type":        "array",
						"description": "Allowed source repositories",
						"items": map[string]interface{}{
							"type": "string",
						},
					},
				},
				Required: []string{"name"},
			},
		},
		{
			Name:        "delete_project",
			Description: "Delete a project",
			InputSchema: mcp.ToolInputSchema{
				Type: "object",
				Properties: map[string]interface{}{
					"name": map[string]interface{}{
						"type":        "string",
						"description": "Project name (required)",
					},
				},
				Required: []string{"name"},
			},
		},
		{
			Name:        "get_project_events",
			Description: "Get events for a project",
			InputSchema: mcp.ToolInputSchema{
				Type: "object",
				Properties: map[string]interface{}{
					"name": map[string]interface{}{
						"type":        "string",
						"description": "Project name (required)",
					},
				},
				Required: []string{"name"},
			},
		},
	}
}
