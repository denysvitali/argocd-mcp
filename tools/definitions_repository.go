package tools

import "github.com/mark3labs/mcp-go/mcp"

// repositoryToolDefinitions returns the MCP tool definitions for the repository domain.
func repositoryToolDefinitions() []mcp.Tool {
	return []mcp.Tool{
		{
			Name:        "list_repositories",
			Description: "List all configured repositories",
			InputSchema: mcp.ToolInputSchema{
				Type: "object",
				Properties: map[string]interface{}{
					"repo_url": map[string]interface{}{
						"type":        "string",
						"description": "Filter by repository URL (partial match)",
					},
					"limit": map[string]interface{}{
						"type":        "integer",
						"description": "Maximum number of repositories to return (default: 50)",
					},
				},
			},
		},
		{
			Name:        "get_repository",
			Description: "Get details of a specific repository",
			InputSchema: mcp.ToolInputSchema{
				Type: "object",
				Properties: map[string]interface{}{
					"repo_url": map[string]interface{}{
						"type":        "string",
						"description": "Repository URL (required)",
					},
				},
				Required: []string{"repo_url"},
			},
		},
		{
			Name:        "create_repository",
			Description: "Create a new repository connection",
			InputSchema: mcp.ToolInputSchema{
				Type: "object",
				Properties: map[string]interface{}{
					"repo_url": map[string]interface{}{
						"type":        "string",
						"description": "Repository URL (required)",
					},
					"type": map[string]interface{}{
						"type":        "string",
						"description": "Repository type (git or helm)",
					},
					"name": map[string]interface{}{
						"type":        "string",
						"description": "Repository name",
					},
					"username": map[string]interface{}{
						"type":        "string",
						"description": "Username for authentication",
					},
					"password": map[string]interface{}{
						"type":        "string",
						"description": "Password or token for authentication",
					},
					"ssh_private_key": map[string]interface{}{
						"type":        "string",
						"description": "SSH private key for SSH authentication",
					},
				},
				Required: []string{"repo_url"},
			},
		},
		{
			Name:        "update_repository",
			Description: "Update an existing repository",
			InputSchema: mcp.ToolInputSchema{
				Type: "object",
				Properties: map[string]interface{}{
					"repo_url": map[string]interface{}{
						"type":        "string",
						"description": "Repository URL (required)",
					},
					"name": map[string]interface{}{
						"type":        "string",
						"description": "Repository name",
					},
					"username": map[string]interface{}{
						"type":        "string",
						"description": "Username for authentication",
					},
					"password": map[string]interface{}{
						"type":        "string",
						"description": "Password or token for authentication",
					},
				},
				Required: []string{"repo_url"},
			},
		},
		{
			Name:        "delete_repository",
			Description: "Delete a repository",
			InputSchema: mcp.ToolInputSchema{
				Type: "object",
				Properties: map[string]interface{}{
					"repo_url": map[string]interface{}{
						"type":        "string",
						"description": "Repository URL (required)",
					},
				},
				Required: []string{"repo_url"},
			},
		},
		{
			Name:        "validate_repository",
			Description: "Validate repository access",
			InputSchema: mcp.ToolInputSchema{
				Type: "object",
				Properties: map[string]interface{}{
					"repo_url": map[string]interface{}{
						"type":        "string",
						"description": "Repository URL (required)",
					},
				},
				Required: []string{"repo_url"},
			},
		},
	}
}
