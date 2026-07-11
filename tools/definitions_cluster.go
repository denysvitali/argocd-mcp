package tools

import "github.com/mark3labs/mcp-go/mcp"

// clusterToolDefinitions returns the MCP tool definitions for the cluster domain.
func clusterToolDefinitions() []mcp.Tool {
	return []mcp.Tool{
		{
			Name:        "list_clusters",
			Description: "List all configured clusters",
			InputSchema: mcp.ToolInputSchema{
				Type: "object",
				Properties: map[string]interface{}{
					"server": map[string]interface{}{
						"type":        "string",
						"description": "Filter by cluster server URL (partial match)",
					},
					"limit": map[string]interface{}{
						"type":        "integer",
						"description": "Maximum number of clusters to return (default: 50)",
					},
				},
			},
		},
		{
			Name:        "get_cluster",
			Description: "Get details of a specific cluster",
			InputSchema: mcp.ToolInputSchema{
				Type: "object",
				Properties: map[string]interface{}{
					"server": map[string]interface{}{
						"type":        "string",
						"description": "Cluster server URL (required)",
					},
				},
				Required: []string{"server"},
			},
		},
		{
			Name:        "create_cluster",
			Description: "Create a new cluster connection",
			InputSchema: mcp.ToolInputSchema{
				Type: "object",
				Properties: map[string]interface{}{
					"server": map[string]interface{}{
						"type":        "string",
						"description": "Cluster server URL (required)",
					},
					"name": map[string]interface{}{
						"type":        "string",
						"description": "Cluster name",
					},
					"config": map[string]interface{}{
						"type":        "object",
						"description": "Cluster configuration",
						"properties": map[string]interface{}{
							"username": map[string]interface{}{
								"type": "string",
							},
							"password": map[string]interface{}{
								"type": "string",
							},
							"bearerToken": map[string]interface{}{
								"type": "string",
							},
							"tlsClientConfig": map[string]interface{}{
								"type": "object",
								"properties": map[string]interface{}{
									"insecure": map[string]interface{}{
										"type": "boolean",
									},
									"caData": map[string]interface{}{
										"type": "string",
									},
									"certData": map[string]interface{}{
										"type": "string",
									},
									"keyData": map[string]interface{}{
										"type": "string",
									},
								},
							},
						},
					},
				},
				Required: []string{"server"},
			},
		},
		{
			Name:        "update_cluster",
			Description: "Update an existing cluster",
			InputSchema: mcp.ToolInputSchema{
				Type: "object",
				Properties: map[string]interface{}{
					"server": map[string]interface{}{
						"type":        "string",
						"description": "Cluster server URL (required)",
					},
					"name": map[string]interface{}{
						"type":        "string",
						"description": "Cluster name",
					},
					"config": map[string]interface{}{
						"type":        "object",
						"description": "Cluster configuration",
						"properties": map[string]interface{}{
							"username": map[string]interface{}{
								"type": "string",
							},
							"password": map[string]interface{}{
								"type": "string",
							},
							"bearerToken": map[string]interface{}{
								"type": "string",
							},
						},
					},
				},
				Required: []string{"server"},
			},
		},
		{
			Name:        "delete_cluster",
			Description: "Delete a cluster",
			InputSchema: mcp.ToolInputSchema{
				Type: "object",
				Properties: map[string]interface{}{
					"server": map[string]interface{}{
						"type":        "string",
						"description": "Cluster server URL (required)",
					},
				},
				Required: []string{"server"},
			},
		},
	}
}
