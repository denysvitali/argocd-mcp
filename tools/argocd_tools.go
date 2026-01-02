package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/argocd-mcp/argocd-mcp/internal/client"
	"github.com/argoproj/argo-cd/v3/pkg/apiclient/application"
	"github.com/argoproj/argo-cd/v3/pkg/apiclient/cluster"
	"github.com/argoproj/argo-cd/v3/pkg/apiclient/project"
	"github.com/argoproj/argo-cd/v3/pkg/apiclient/repository"
	"github.com/argoproj/argo-cd/v3/pkg/apis/application/v1alpha1"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/sirupsen/logrus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"gopkg.in/yaml.v2"
)

// Default timeout and retry constants
const (
	defaultSyncTimeout = 60 * time.Second
	defaultRetryCount  = 3
)

// ToolManager manages the MCP tools for ArgoCD
type ToolManager struct {
	client   *client.Client
	logger   *logrus.Logger
	tools    []mcp.Tool
	safeMode bool
}

// NewToolManager creates a new tool manager
func NewToolManager(client *client.Client, logger *logrus.Logger, safeMode bool) *ToolManager {
	return &ToolManager{
		client:   client,
		logger:   logger,
		tools:    []mcp.Tool{},
		safeMode: safeMode,
	}
}

// GetServerTools returns all the server tools
func (tm *ToolManager) GetServerTools() []server.ServerTool {
	tm.defineTools()
	var serverTools []server.ServerTool
	for _, tool := range tm.tools {
		serverTools = append(serverTools, server.ServerTool{
			Tool:    tool,
			Handler: tm.getToolHandler(tool.Name),
		})
	}
	return serverTools
}

// defineTools defines all the MCP tools
func (tm *ToolManager) defineTools() {
	tm.tools = []mcp.Tool{
		// Application tools
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
				},
				Required: []string{"name"},
			},
		},
		{
			Name:        "get_application_events",
			Description: "Get events for an application",
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
		// Project tools
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
		// Repository tools
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
		// Cluster tools
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

// getToolHandler returns the handler for a specific tool
func (tm *ToolManager) getToolHandler(name string) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		arguments, ok := request.Params.Arguments.(map[string]interface{})
		if !ok {
			return errorResult("Invalid arguments format"), nil
		}

		ctx, cancel := context.WithTimeout(ctx, defaultSyncTimeout)
		defer cancel()

		switch name {
		case "list_applications":
			return tm.handleListApplications(ctx, arguments)
		case "get_application":
			return tm.handleGetApplication(ctx, arguments)
		case "create_application":
			return tm.handleCreateApplication(ctx, arguments)
		case "update_application":
			return tm.handleUpdateApplication(ctx, arguments)
		case "delete_application":
			return tm.handleDeleteApplication(ctx, arguments)
		case "sync_application":
			return tm.handleSyncApplication(ctx, arguments)
		case "rollback_application":
			return tm.handleRollbackApplication(ctx, arguments)
		case "get_application_manifests":
			return tm.handleGetApplicationManifests(ctx, arguments)
		case "get_application_diff":
			return tm.handleGetApplicationDiff(ctx, arguments)
		case "get_application_events":
			return tm.handleGetApplicationEvents(ctx, arguments)
		case "list_resource_actions":
			return tm.handleListResourceActions(ctx, arguments)
		case "run_resource_action":
			return tm.handleRunResourceAction(ctx, arguments)
		case "get_application_resource":
			return tm.handleGetApplicationResource(ctx, arguments)
		case "patch_application_resource":
			return tm.handlePatchApplicationResource(ctx, arguments)
		case "delete_application_resource":
			return tm.handleDeleteApplicationResource(ctx, arguments)
		case "list_projects":
			return tm.handleListProjects(ctx, arguments)
		case "get_project":
			return tm.handleGetProject(ctx, arguments)
		case "create_project":
			return tm.handleCreateProject(ctx, arguments)
		case "update_project":
			return tm.handleUpdateProject(ctx, arguments)
		case "delete_project":
			return tm.handleDeleteProject(ctx, arguments)
		case "get_project_events":
			return tm.handleGetProjectEvents(ctx, arguments)
		case "list_repositories":
			return tm.handleListRepositories(ctx, arguments)
		case "get_repository":
			return tm.handleGetRepository(ctx, arguments)
		case "create_repository":
			return tm.handleCreateRepository(ctx, arguments)
		case "update_repository":
			return tm.handleUpdateRepository(ctx, arguments)
		case "delete_repository":
			return tm.handleDeleteRepository(ctx, arguments)
		case "validate_repository":
			return tm.handleValidateRepository(ctx, arguments)
		case "list_clusters":
			return tm.handleListClusters(ctx, arguments)
		case "get_cluster":
			return tm.handleGetCluster(ctx, arguments)
		case "create_cluster":
			return tm.handleCreateCluster(ctx, arguments)
		case "update_cluster":
			return tm.handleUpdateCluster(ctx, arguments)
		case "delete_cluster":
			return tm.handleDeleteCluster(ctx, arguments)
		default:
			return errorResult(fmt.Sprintf("Unknown tool: %s", name)), nil
		}
	}
}

// Application handlers

func (tm *ToolManager) handleListApplications(ctx context.Context, arguments map[string]interface{}) (*mcp.CallToolResult, error) {
	name := String(arguments, "name", "")
	project := String(arguments, "project", "")
	query := &application.ApplicationQuery{}
	if name != "" {
		query.Name = &name
	}
	if project != "" {
		query.Project = []string{project}
	}

	apps, err := tm.client.ListApplications(ctx, query)
	if err != nil {
		return errorResult(err.Error()), nil
	}

	items := make([]interface{}, len(apps.Items))
	for i, app := range apps.Items {
		items[i] = formatApplicationSummary(&app)
	}

	return ResultList(items, len(items), nil)
}

func (tm *ToolManager) handleGetApplication(ctx context.Context, arguments map[string]interface{}) (*mcp.CallToolResult, error) {
	name := String(arguments, "name", "")
	query := &application.ApplicationQuery{
		Name: &name,
	}

	app, err := tm.client.GetApplication(ctx, query)
	if err != nil {
		return errorResult(err.Error()), nil
	}

	return Result(formatApplicationDetail(app), nil)
}

func (tm *ToolManager) handleCreateApplication(ctx context.Context, arguments map[string]interface{}) (*mcp.CallToolResult, error) {
	if result := tm.checkSafeMode("create_application"); result != nil {
		return result, nil
	}

	name := String(arguments, "name", "")
	project := String(arguments, "project", "")
	repoURL := String(arguments, "repo_url", "")
	path := String(arguments, "path", "")
	targetRevision := String(arguments, "target_revision", "HEAD")

	spec := v1alpha1.ApplicationSpec{
		Destination: v1alpha1.ApplicationDestination{
			Server:    "https://kubernetes.default.svc",
			Namespace: "",
		},
		Source: &v1alpha1.ApplicationSource{
			RepoURL:        repoURL,
			Path:           path,
			TargetRevision: targetRevision,
		},
		Project: project,
	}

	appName := name
	createReq := &application.ApplicationCreateRequest{
		Application: &v1alpha1.Application{
			ObjectMeta: metav1.ObjectMeta{
				Name:      appName,
				Namespace: "argocd",
			},
			Spec: spec,
		},
	}

	app, err := tm.client.CreateApplication(ctx, createReq)
	if err != nil {
		return errorResult(err.Error()), nil
	}

	return Result(formatApplicationDetail(app), nil)
}

func (tm *ToolManager) handleDeleteApplication(ctx context.Context, arguments map[string]interface{}) (*mcp.CallToolResult, error) {
	if result := tm.checkSafeMode("delete_application"); result != nil {
		return result, nil
	}

	name := String(arguments, "name", "")
	cascade := Bool(arguments, "cascade", true)
	deleteReq := &application.ApplicationDeleteRequest{
		Name:    &name,
		Cascade: &cascade,
	}

	err := tm.client.DeleteApplication(ctx, deleteReq)
	if err != nil {
		return errorResult(err.Error()), nil
	}

	return Result(map[string]interface{}{
		"message": fmt.Sprintf("Application %s deleted successfully", name),
		"success": true,
	}, nil)
}

func (tm *ToolManager) handleSyncApplication(ctx context.Context, arguments map[string]interface{}) (*mcp.CallToolResult, error) {
	name := String(arguments, "name", "")
	revision := String(arguments, "revision", "")
	prune := Bool(arguments, "prune", false)

	// In safe mode, prune is not allowed
	if tm.safeMode && prune {
		return errorResult("sync_application with prune=true is not allowed in safe mode"), nil
	}

	pruneValue := prune
	syncReq := &application.ApplicationSyncRequest{
		Name:     &name,
		Revision: &revision,
		Prune:    &pruneValue,
	}

	app, err := tm.client.SyncApplication(ctx, syncReq)
	if err != nil {
		return errorResult(err.Error()), nil
	}

	return Result(map[string]interface{}{
		"message":  fmt.Sprintf("Application %s sync initiated", name),
		"status":   app.Status.Sync.Status,
		"health":   app.Status.Health.Status,
		"revision": app.Status.Sync.Revision,
	}, nil)
}

func (tm *ToolManager) handleGetApplicationManifests(ctx context.Context, arguments map[string]interface{}) (*mcp.CallToolResult, error) {
	name := String(arguments, "name", "")
	revision := String(arguments, "revision", "")
	query := &application.ApplicationManifestQuery{
		Name:     &name,
		Revision: &revision,
	}

	manifests, err := tm.client.GetApplicationManifests(ctx, query)
	if err != nil {
		return errorResult(err.Error()), nil
	}

	// Convert all manifests from JSON to YAML
	yamlManifests := make([]string, len(manifests))
	for i, m := range manifests {
		yamlManifests[i] = jsonToYaml(m)
	}

	return Result(map[string]interface{}{
		"manifests": yamlManifests,
		"count":     len(manifests),
	}, nil)
}

func (tm *ToolManager) handleGetApplicationDiff(ctx context.Context, arguments map[string]interface{}) (*mcp.CallToolResult, error) {
	name := String(arguments, "name", "")

	resources, err := tm.client.GetManagedResources(ctx, name)
	if err != nil {
		return errorResult(err.Error()), nil
	}

	// Format the diff information
	outOfSync := make([]interface{}, 0)
	synced := make([]interface{}, 0)

	for _, r := range resources {
		resourceInfo := map[string]interface{}{
			"group":     r.Group,
			"kind":      r.Kind,
			"namespace": r.Namespace,
			"name":      r.Name,
		}

		// Use Modified flag to determine sync status (preferred over deprecated Diff field)
		if r.Modified || r.Diff != "" {
			resourceInfo["status"] = "OutOfSync"
			resourceInfo["target_state"] = jsonToYaml(r.TargetState)
			resourceInfo["normalized_live_state"] = jsonToYaml(r.NormalizedLiveState)
			resourceInfo["predicted_live_state"] = jsonToYaml(r.PredictedLiveState)
			resourceInfo["resource_version"] = r.ResourceVersion
			// Include legacy diff for backward compatibility
			if r.Diff != "" {
				resourceInfo["diff"] = r.Diff
			}
			outOfSync = append(outOfSync, resourceInfo)
		} else {
			resourceInfo["status"] = "Synced"
			synced = append(synced, resourceInfo)
		}
	}

	return Result(map[string]interface{}{
		"application":       name,
		"out_of_sync":       outOfSync,
		"synced":            synced,
		"total":             len(resources),
		"out_of_sync_count": len(outOfSync),
	}, nil)
}

// jsonToYaml converts JSON string to YAML string
func jsonToYaml(jsonStr string) string {
	if jsonStr == "" {
		return ""
	}
	var data interface{}
	if err := json.Unmarshal([]byte(jsonStr), &data); err != nil {
		// If JSON parsing fails, return original string
		return jsonStr
	}
	yamlBytes, err := yaml.Marshal(data)
	if err != nil {
		return jsonStr
	}
	return string(yamlBytes)
}

func (tm *ToolManager) handleGetApplicationEvents(ctx context.Context, arguments map[string]interface{}) (*mcp.CallToolResult, error) {
	name := String(arguments, "name", "")
	query := &application.ApplicationResourceEventsQuery{
		Name: &name,
	}

	eventsRaw, err := tm.client.GetApplicationEvents(ctx, query)
	if err != nil {
		return errorResult(err.Error()), nil
	}

	events, parseErr := parseEvents(eventsRaw)
	if parseErr != nil {
		return errorResult(fmt.Sprintf("Failed to parse events: %v", parseErr)), nil
	}

	eventList := make([]interface{}, len(events))
	for i, event := range events {
		eventMap, ok := event.(map[string]interface{})
		if !ok {
			continue
		}
		eventList[i] = map[string]interface{}{
			"type":      eventMap["type"],
			"reason":    eventMap["reason"],
			"message":   eventMap["message"],
			"timestamp": eventMap["timestamp"],
		}
	}

	return Result(map[string]interface{}{
		"items": eventList,
		"total": len(events),
	}, nil)
}

func (tm *ToolManager) handleUpdateApplication(ctx context.Context, arguments map[string]interface{}) (*mcp.CallToolResult, error) {
	if result := tm.checkSafeMode("update_application"); result != nil {
		return result, nil
	}

	name := String(arguments, "name", "")
	project := String(arguments, "project", "")
	repoURL := String(arguments, "repo_url", "")
	path := String(arguments, "path", "")
	targetRevision := String(arguments, "target_revision", "")

	// First get the existing application
	query := &application.ApplicationQuery{Name: &name}
	existingApp, err := tm.client.GetApplication(ctx, query)
	if err != nil {
		return errorResult(err.Error()), nil
	}

	// Update fields if provided
	if project != "" {
		existingApp.Spec.Project = project
	}
	if repoURL != "" && existingApp.Spec.Source != nil {
		existingApp.Spec.Source.RepoURL = repoURL
	}
	if path != "" && existingApp.Spec.Source != nil {
		existingApp.Spec.Source.Path = path
	}
	if targetRevision != "" && existingApp.Spec.Source != nil {
		existingApp.Spec.Source.TargetRevision = targetRevision
	}

	updateReq := &application.ApplicationUpdateRequest{
		Application: existingApp,
	}

	app, err := tm.client.UpdateApplication(ctx, updateReq)
	if err != nil {
		return errorResult(err.Error()), nil
	}

	return Result(formatApplicationDetail(app), nil)
}

func (tm *ToolManager) handleRollbackApplication(ctx context.Context, arguments map[string]interface{}) (*mcp.CallToolResult, error) {
	if result := tm.checkSafeMode("rollback_application"); result != nil {
		return result, nil
	}

	name := String(arguments, "name", "")

	namePtr := &name
	rollbackReq := &application.ApplicationRollbackRequest{
		Name: namePtr,
	}

	app, err := tm.client.RollbackApplication(ctx, rollbackReq)
	if err != nil {
		return errorResult(err.Error()), nil
	}

	return Result(map[string]interface{}{
		"message":  fmt.Sprintf("Application %s rolled back", name),
		"status":   app.Status.Sync.Status,
		"health":   app.Status.Health.Status,
		"revision": app.Status.Sync.Revision,
	}, nil)
}

func (tm *ToolManager) handleListResourceActions(ctx context.Context, arguments map[string]interface{}) (*mcp.CallToolResult, error) {
	name := String(arguments, "name", "")
	group := String(arguments, "group", "")
	kind := String(arguments, "kind", "")
	namespace := String(arguments, "namespace", "")
	resourceName := String(arguments, "resource_name", "")

	namePtr := &name
	groupPtr := &group
	kindPtr := &kind
	namespacePtr := &namespace
	resourceNamePtr := &resourceName

	// Determine the API version from the group
	version := inferResourceVersion(group)
	versionPtr := &version

	query := &application.ApplicationResourceRequest{
		Name:         namePtr,
		ResourceName: resourceNamePtr,
		Version:      versionPtr,
		Group:        groupPtr,
		Kind:         kindPtr,
		Namespace:    namespacePtr,
	}

	actions, err := tm.client.ListResourceActions(ctx, query)
	if err != nil {
		return errorResult(err.Error()), nil
	}

	actionList := make([]interface{}, len(actions))
	for i, action := range actions {
		actionList[i] = map[string]interface{}{
			"name": action.Name,
		}
	}

	return Result(map[string]interface{}{
		"actions": actionList,
		"total":   len(actions),
	}, nil)
}

func (tm *ToolManager) handleRunResourceAction(ctx context.Context, arguments map[string]interface{}) (*mcp.CallToolResult, error) {
	if result := tm.checkSafeMode("run_resource_action"); result != nil {
		return result, nil
	}

	name := String(arguments, "name", "")
	group := String(arguments, "group", "")
	kind := String(arguments, "kind", "")
	namespace := String(arguments, "namespace", "")
	resourceName := String(arguments, "resource_name", "")
	action := String(arguments, "action", "")

	namePtr := &name
	groupPtr := &group
	kindPtr := &kind
	namespacePtr := &namespace
	resourceNamePtr := &resourceName
	actionPtr := &action

	// Create resource action request using deprecated type
	//lint:ignore SA1019 ResourceActionRunRequest is deprecated but required for resource action execution
	actionReq := &application.ResourceActionRunRequest{
		Name:         namePtr,
		Group:        groupPtr,
		Kind:         kindPtr,
		Namespace:    namespacePtr,
		ResourceName: resourceNamePtr,
		Action:       actionPtr,
	}

	err := tm.client.RunResourceAction(ctx, actionReq)
	if err != nil {
		return errorResult(err.Error()), nil
	}

	return Result(map[string]interface{}{
		"message": fmt.Sprintf("Action '%s' executed on %s/%s/%s", action, kind, namespace, resourceName),
		"success": true,
	}, nil)
}

func (tm *ToolManager) handleGetApplicationResource(ctx context.Context, arguments map[string]interface{}) (*mcp.CallToolResult, error) {
	name := String(arguments, "name", "")
	group := String(arguments, "group", "")
	kind := String(arguments, "kind", "")
	namespace := String(arguments, "namespace", "")
	resourceName := String(arguments, "resource_name", "")

	namePtr := &name
	groupPtr := &group
	kindPtr := &kind
	namespacePtr := &namespace
	resourceNamePtr := &resourceName

	// Determine the API version from the group
	// Most Kubernetes resources use v1, but we should allow override
	version := inferResourceVersion(group)
	versionPtr := &version

	resourceReq := &application.ApplicationResourceRequest{
		Name:         namePtr,
		ResourceName: resourceNamePtr,
		Version:      versionPtr,
		Group:        groupPtr,
		Kind:         kindPtr,
		Namespace:    namespacePtr,
	}

	resource, err := tm.client.GetApplicationResource(ctx, resourceReq)
	if err != nil {
		return errorResult(err.Error()), nil
	}

	return Result(map[string]interface{}{
		"resource": resource,
		"success":  true,
	}, nil)
}

func (tm *ToolManager) handlePatchApplicationResource(ctx context.Context, arguments map[string]interface{}) (*mcp.CallToolResult, error) {
	if result := tm.checkSafeMode("patch_application_resource"); result != nil {
		return result, nil
	}

	name := String(arguments, "name", "")
	group := String(arguments, "group", "")
	kind := String(arguments, "kind", "")
	namespace := String(arguments, "namespace", "")
	resourceName := String(arguments, "resource_name", "")
	patch := String(arguments, "patch", "")
	patchType := String(arguments, "patch_type", "merge")

	namePtr := &name
	groupPtr := &group
	kindPtr := &kind
	namespacePtr := &namespace
	resourceNamePtr := &resourceName
	patchPtr := &patch
	patchTypePtr := &patchType

	// Determine the API version from the group
	version := inferResourceVersion(group)
	versionPtr := &version

	patchReq := &application.ApplicationResourcePatchRequest{
		Name:         namePtr,
		ResourceName: resourceNamePtr,
		Version:      versionPtr,
		Group:        groupPtr,
		Kind:         kindPtr,
		Namespace:    namespacePtr,
		Patch:        patchPtr,
		PatchType:    patchTypePtr,
	}

	resource, err := tm.client.PatchApplicationResource(ctx, patchReq)
	if err != nil {
		return errorResult(err.Error()), nil
	}

	return Result(map[string]interface{}{
		"resource": resource,
		"message":  fmt.Sprintf("Resource %s/%s patched successfully", kind, resourceName),
		"success":  true,
	}, nil)
}

func (tm *ToolManager) handleDeleteApplicationResource(ctx context.Context, arguments map[string]interface{}) (*mcp.CallToolResult, error) {
	if result := tm.checkSafeMode("delete_application_resource"); result != nil {
		return result, nil
	}

	name := String(arguments, "name", "")
	group := String(arguments, "group", "")
	kind := String(arguments, "kind", "")
	namespace := String(arguments, "namespace", "")
	resourceName := String(arguments, "resource_name", "")
	force := Bool(arguments, "force", false)
	orphan := Bool(arguments, "orphan", false)

	namePtr := &name
	groupPtr := &group
	kindPtr := &kind
	namespacePtr := &namespace
	resourceNamePtr := &resourceName
	forcePtr := &force
	orphanPtr := &orphan

	// Determine the API version from the group
	version := inferResourceVersion(group)
	versionPtr := &version

	deleteReq := &application.ApplicationResourceDeleteRequest{
		Name:         namePtr,
		ResourceName: resourceNamePtr,
		Version:      versionPtr,
		Group:        groupPtr,
		Kind:         kindPtr,
		Namespace:    namespacePtr,
		Force:        forcePtr,
		Orphan:       orphanPtr,
	}

	err := tm.client.DeleteApplicationResource(ctx, deleteReq)
	if err != nil {
		return errorResult(err.Error()), nil
	}

	return Result(map[string]interface{}{
		"message": fmt.Sprintf("Resource %s/%s deleted successfully", kind, resourceName),
		"success": true,
	}, nil)
}

// Project handlers

func (tm *ToolManager) handleListProjects(ctx context.Context, arguments map[string]interface{}) (*mcp.CallToolResult, error) {
	name := String(arguments, "name", "")
	query := &project.ProjectQuery{}
	if name != "" {
		query.Name = name
	}

	projects, err := tm.client.ListProjects(ctx, query)
	if err != nil {
		return errorResult(err.Error()), nil
	}

	items := make([]interface{}, len(projects.Items))
	for i, proj := range projects.Items {
		items[i] = map[string]interface{}{
			"name":        proj.Name,
			"description": proj.Spec.Description,
		}
	}

	return ResultList(items, len(items), nil)
}

func (tm *ToolManager) handleGetProject(ctx context.Context, arguments map[string]interface{}) (*mcp.CallToolResult, error) {
	name := String(arguments, "name", "")
	query := &project.ProjectQuery{
		Name: name,
	}

	proj, err := tm.client.GetProject(ctx, query)
	if err != nil {
		return errorResult(err.Error()), nil
	}

	return Result(map[string]interface{}{
		"name":         proj.Name,
		"description":  proj.Spec.Description,
		"source_repos": proj.Spec.SourceRepos,
		"destinations": proj.Spec.Destinations,
	}, nil)
}

func (tm *ToolManager) handleCreateProject(ctx context.Context, arguments map[string]interface{}) (*mcp.CallToolResult, error) {
	if result := tm.checkSafeMode("create_project"); result != nil {
		return result, nil
	}

	name := String(arguments, "name", "")
	description := String(arguments, "description", "")

	createReq := &project.ProjectCreateRequest{
		Project: &v1alpha1.AppProject{
			ObjectMeta: metav1.ObjectMeta{
				Name: name,
			},
			Spec: v1alpha1.AppProjectSpec{
				Description: description,
			},
		},
	}

	proj, err := tm.client.CreateProject(ctx, createReq)
	if err != nil {
		return errorResult(err.Error()), nil
	}

	return Result(map[string]interface{}{
		"name":        proj.Name,
		"description": proj.Spec.Description,
		"message":     fmt.Sprintf("Project %s created successfully", name),
	}, nil)
}

func (tm *ToolManager) handleUpdateProject(ctx context.Context, arguments map[string]interface{}) (*mcp.CallToolResult, error) {
	if result := tm.checkSafeMode("update_project"); result != nil {
		return result, nil
	}

	name := String(arguments, "name", "")
	description := String(arguments, "description", "")

	// Get existing project
	query := &project.ProjectQuery{Name: name}
	existingProj, err := tm.client.GetProject(ctx, query)
	if err != nil {
		return errorResult(err.Error()), nil
	}

	// Update fields if provided
	if description != "" {
		existingProj.Spec.Description = description
	}

	updateReq := &project.ProjectUpdateRequest{
		Project: existingProj,
	}

	proj, err := tm.client.UpdateProject(ctx, updateReq)
	if err != nil {
		return errorResult(err.Error()), nil
	}

	return Result(map[string]interface{}{
		"name":        proj.Name,
		"description": proj.Spec.Description,
		"message":     fmt.Sprintf("Project %s updated successfully", name),
	}, nil)
}

func (tm *ToolManager) handleDeleteProject(ctx context.Context, arguments map[string]interface{}) (*mcp.CallToolResult, error) {
	if result := tm.checkSafeMode("delete_project"); result != nil {
		return result, nil
	}

	name := String(arguments, "name", "")
	query := &project.ProjectQuery{Name: name}

	err := tm.client.DeleteProject(ctx, query)
	if err != nil {
		return errorResult(err.Error()), nil
	}

	return Result(map[string]interface{}{
		"message": fmt.Sprintf("Project %s deleted successfully", name),
		"success": true,
	}, nil)
}

func (tm *ToolManager) handleGetProjectEvents(ctx context.Context, arguments map[string]interface{}) (*mcp.CallToolResult, error) {
	name := String(arguments, "name", "")
	query := &project.ProjectQuery{Name: name}

	eventsRaw, err := tm.client.GetProjectEvents(ctx, query)
	if err != nil {
		return errorResult(err.Error()), nil
	}

	events, parseErr := parseEvents(eventsRaw)
	if parseErr != nil {
		return errorResult(fmt.Sprintf("Failed to parse events: %v", parseErr)), nil
	}

	eventList := make([]interface{}, len(events))
	for i, event := range events {
		eventMap, ok := event.(map[string]interface{})
		if !ok {
			continue
		}
		eventList[i] = map[string]interface{}{
			"type":      eventMap["type"],
			"reason":    eventMap["reason"],
			"message":   eventMap["message"],
			"timestamp": eventMap["timestamp"],
		}
	}

	return Result(map[string]interface{}{
		"items": eventList,
		"total": len(events),
	}, nil)
}

// Repository handlers

func (tm *ToolManager) handleListRepositories(ctx context.Context, arguments map[string]interface{}) (*mcp.CallToolResult, error) {
	repoURL := String(arguments, "repo_url", "")
	query := &repository.RepoQuery{}
	if repoURL != "" {
		query.Repo = repoURL
	}

	repos, err := tm.client.ListRepositories(ctx, query)
	if err != nil {
		return errorResult(err.Error()), nil
	}

	items := make([]interface{}, len(repos.Items))
	for i, repo := range repos.Items {
		items[i] = map[string]interface{}{
			"repo": repo.Repo,
			"type": repo.Type,
			"name": repo.Name,
		}
	}

	return ResultList(items, len(items), nil)
}

func (tm *ToolManager) handleGetRepository(ctx context.Context, arguments map[string]interface{}) (*mcp.CallToolResult, error) {
	repoURL := String(arguments, "repo_url", "")
	query := &repository.RepoQuery{
		Repo: repoURL,
	}

	repo, err := tm.client.GetRepository(ctx, query)
	if err != nil {
		return errorResult(err.Error()), nil
	}

	return Result(map[string]interface{}{
		"repo":             repo.Repo,
		"type":             repo.Type,
		"name":             repo.Name,
		"connection_state": repo.ConnectionState,
	}, nil)
}

func (tm *ToolManager) handleCreateRepository(ctx context.Context, arguments map[string]interface{}) (*mcp.CallToolResult, error) {
	if result := tm.checkSafeMode("create_repository"); result != nil {
		return result, nil
	}

	repoURL := String(arguments, "repo_url", "")
	repoType := String(arguments, "type", "git")
	name := String(arguments, "name", "")
	username := String(arguments, "username", "")
	password := String(arguments, "password", "")
	sshPrivateKey := String(arguments, "ssh_private_key", "")
	insecure := Bool(arguments, "insecure", false)

	if repoURL == "" {
		return errorResult("repo_url is required"), nil
	}

	repo := &v1alpha1.Repository{
		Repo:          repoURL,
		Type:          repoType,
		Name:          name,
		Username:      username,
		Password:      password,
		SSHPrivateKey: sshPrivateKey,
		Insecure:      insecure,
	}

	createReq := &repository.RepoCreateRequest{
		Repo:   repo,
		Upsert: false,
	}

	createdRepo, err := tm.client.CreateRepository(ctx, createReq)
	if err != nil {
		return errorResult(err.Error()), nil
	}

	return Result(map[string]interface{}{
		"repo":             createdRepo.Repo,
		"type":             createdRepo.Type,
		"name":             createdRepo.Name,
		"connection_state": createdRepo.ConnectionState,
		"message":          fmt.Sprintf("Repository %s created successfully", repoURL),
		"success":          true,
	}, nil)
}

func (tm *ToolManager) handleUpdateRepository(ctx context.Context, arguments map[string]interface{}) (*mcp.CallToolResult, error) {
	if result := tm.checkSafeMode("update_repository"); result != nil {
		return result, nil
	}

	repoURL := String(arguments, "repo_url", "")
	name := String(arguments, "name", "")
	username := String(arguments, "username", "")
	password := String(arguments, "password", "")
	sshPrivateKey := String(arguments, "ssh_private_key", "")

	if repoURL == "" {
		return errorResult("repo_url is required"), nil
	}

	// Get existing repository first
	query := &repository.RepoQuery{Repo: repoURL}
	existingRepo, err := tm.client.GetRepository(ctx, query)
	if err != nil {
		return errorResult(fmt.Sprintf("failed to get existing repository: %v", err)), nil
	}

	// Update fields if provided
	if name != "" {
		existingRepo.Name = name
	}
	if username != "" {
		existingRepo.Username = username
	}
	if password != "" {
		existingRepo.Password = password
	}
	if sshPrivateKey != "" {
		existingRepo.SSHPrivateKey = sshPrivateKey
	}

	updateReq := &repository.RepoUpdateRequest{
		Repo: existingRepo,
	}

	updatedRepo, err := tm.client.UpdateRepository(ctx, updateReq)
	if err != nil {
		return errorResult(err.Error()), nil
	}

	return Result(map[string]interface{}{
		"repo":             updatedRepo.Repo,
		"type":             updatedRepo.Type,
		"name":             updatedRepo.Name,
		"connection_state": updatedRepo.ConnectionState,
		"message":          fmt.Sprintf("Repository %s updated successfully", repoURL),
		"success":          true,
	}, nil)
}

func (tm *ToolManager) handleDeleteRepository(ctx context.Context, arguments map[string]interface{}) (*mcp.CallToolResult, error) {
	if result := tm.checkSafeMode("delete_repository"); result != nil {
		return result, nil
	}

	repoURL := String(arguments, "repo_url", "")
	query := &repository.RepoQuery{
		Repo: repoURL,
	}

	err := tm.client.DeleteRepository(ctx, query)
	if err != nil {
		return errorResult(err.Error()), nil
	}

	return Result(map[string]interface{}{
		"message": fmt.Sprintf("Repository %s deleted successfully", repoURL),
		"success": true,
	}, nil)
}

func (tm *ToolManager) handleValidateRepository(ctx context.Context, arguments map[string]interface{}) (*mcp.CallToolResult, error) {
	repoURL := String(arguments, "repo_url", "")
	query := &repository.RepoAccessQuery{
		Repo: repoURL,
	}

	err := tm.client.ValidateRepositoryAccess(ctx, query)
	if err != nil {
		return Result(map[string]interface{}{
			"repo":    repoURL,
			"valid":   false,
			"message": err.Error(),
			"success": false,
		}, nil)
	}

	return Result(map[string]interface{}{
		"repo":    repoURL,
		"valid":   true,
		"message": "Repository access is valid",
		"success": true,
	}, nil)
}

// Cluster handlers

func (tm *ToolManager) handleListClusters(ctx context.Context, arguments map[string]interface{}) (*mcp.CallToolResult, error) {
	server := String(arguments, "server", "")
	query := &cluster.ClusterQuery{}
	if server != "" {
		query.Server = server
	}

	clusters, err := tm.client.ListClusters(ctx, query)
	if err != nil {
		return errorResult(err.Error()), nil
	}

	items := make([]interface{}, len(clusters.Items))
	for i, c := range clusters.Items {
		items[i] = map[string]interface{}{
			"server": c.Server,
			"name":   c.Name,
		}
	}

	return ResultList(items, len(items), nil)
}

func (tm *ToolManager) handleGetCluster(ctx context.Context, arguments map[string]interface{}) (*mcp.CallToolResult, error) {
	server := String(arguments, "server", "")
	query := &cluster.ClusterQuery{
		Server: server,
	}

	c, err := tm.client.GetCluster(ctx, query)
	if err != nil {
		return errorResult(err.Error()), nil
	}

	// ConnectionState is deprecated but we need to use it for backward compatibility
	//lint:ignore SA1019 ConnectionState is deprecated
	connectionState := c.ConnectionState
	return Result(map[string]interface{}{
		"server":           c.Server,
		"name":             c.Name,
		"config":           c.Config,
		"connection_state": connectionState,
	}, nil)
}

func (tm *ToolManager) handleCreateCluster(ctx context.Context, arguments map[string]interface{}) (*mcp.CallToolResult, error) {
	if result := tm.checkSafeMode("create_cluster"); result != nil {
		return result, nil
	}

	server := String(arguments, "server", "")
	name := String(arguments, "name", "")

	if server == "" {
		return errorResult("server is required"), nil
	}

	// Build cluster config from arguments
	config, err := buildClusterConfig(arguments)
	if err != nil {
		return errorResult(fmt.Sprintf("invalid config: %v", err)), nil
	}

	newCluster := &v1alpha1.Cluster{
		Server: server,
		Name:   name,
		Config: config,
	}

	createReq := &cluster.ClusterCreateRequest{
		Cluster: newCluster,
		Upsert:  false,
	}

	createdCluster, err := tm.client.CreateCluster(ctx, createReq)
	if err != nil {
		return errorResult(err.Error()), nil
	}

	// ConnectionState is deprecated but we need to use it for backward compatibility
	//lint:ignore SA1019 ConnectionState is deprecated
	connectionState := createdCluster.ConnectionState
	return Result(map[string]interface{}{
		"server":           createdCluster.Server,
		"name":             createdCluster.Name,
		"config":           createdCluster.Config,
		"connection_state": connectionState,
		"message":          fmt.Sprintf("Cluster %s created successfully", server),
		"success":          true,
	}, nil)
}

func (tm *ToolManager) handleUpdateCluster(ctx context.Context, arguments map[string]interface{}) (*mcp.CallToolResult, error) {
	if result := tm.checkSafeMode("update_cluster"); result != nil {
		return result, nil
	}

	server := String(arguments, "server", "")
	name := String(arguments, "name", "")

	if server == "" {
		return errorResult("server is required"), nil
	}

	// Get existing cluster first
	query := &cluster.ClusterQuery{Server: server}
	existingCluster, err := tm.client.GetCluster(ctx, query)
	if err != nil {
		return errorResult(fmt.Sprintf("failed to get existing cluster: %v", err)), nil
	}

	// Update fields if provided
	if name != "" {
		existingCluster.Name = name
	}

	// Update config if provided
	if configMap, ok := arguments["config"].(map[string]interface{}); len(configMap) > 0 && ok {
		config, err := buildClusterConfig(arguments)
		if err != nil {
			return errorResult(fmt.Sprintf("invalid config: %v", err)), nil
		}
		existingCluster.Config = config
	}

	updateReq := &cluster.ClusterUpdateRequest{
		Cluster:       existingCluster,
		UpdatedFields: []string{"config", "name"},
	}

	updatedCluster, err := tm.client.UpdateCluster(ctx, updateReq)
	if err != nil {
		return errorResult(err.Error()), nil
	}

	// ConnectionState is deprecated but we need to use it for backward compatibility
	//lint:ignore SA1019 ConnectionState is deprecated
	connectionState := updatedCluster.ConnectionState
	return Result(map[string]interface{}{
		"server":           updatedCluster.Server,
		"name":             updatedCluster.Name,
		"config":           updatedCluster.Config,
		"connection_state": connectionState,
		"message":          fmt.Sprintf("Cluster %s updated successfully", server),
		"success":          true,
	}, nil)
}

func (tm *ToolManager) handleDeleteCluster(ctx context.Context, arguments map[string]interface{}) (*mcp.CallToolResult, error) {
	if result := tm.checkSafeMode("delete_cluster"); result != nil {
		return result, nil
	}

	server := String(arguments, "server", "")
	query := &cluster.ClusterQuery{
		Server: server,
	}

	err := tm.client.DeleteCluster(ctx, query)
	if err != nil {
		return errorResult(err.Error()), nil
	}

	return Result(map[string]interface{}{
		"message": fmt.Sprintf("Cluster %s deleted successfully", server),
		"success": true,
	}, nil)
}

// Helper functions

// inferResourceVersion infers the Kubernetes API version from the resource group.
// Most Kubernetes resources use v1. This is a simplified inference that covers
// common cases. For more accuracy, the version should be obtained from the
// resource manifest itself or from API discovery.
func inferResourceVersion(group string) string {
	// For core API (empty group), use v1
	if group == "" || group == "core" {
		return "v1"
	}

	// Common API groups and their typical versions
	// Most stable Kubernetes resources use v1
	commonV1Groups := map[string]bool{
		"apps":           true,
		"batch":          true,
		"networking.k8s.io": true,
		"policy":         true,
		"storage.k8s.io":    true,
		"rbac.authorization.k8s.io": true,
		"coordination.k8s.io": true,
		"apiserverinternal.k8s.io": true,
		"scheduling.k8s.io": true,
	}

	if commonV1Groups[group] {
		return "v1"
	}

	// For custom groups (like postgresql.cnpg.io), also default to v1
	// as most CRDs use v1
	return "v1"
}

// parseEvents converts interface{} to []interface{} with proper type handling
// The input may be a direct list of events or an EventList struct with an Items field
func parseEvents(eventsRaw interface{}) ([]interface{}, error) {
	// First, JSON marshal the input to normalize it
	data, err := json.Marshal(eventsRaw)
	if err != nil {
		return nil, err
	}

	// Try to parse as EventList (object with items field)
	var eventList struct {
		Items json.RawMessage `json:"items"`
	}
	if err := json.Unmarshal(data, &eventList); err == nil && len(eventList.Items) > 0 {
		// Unmarshal items as a slice of generic objects
		var items []map[string]interface{}
		if err := json.Unmarshal(eventList.Items, &items); err == nil {
			result := make([]interface{}, len(items))
			for i, item := range items {
				result[i] = item
			}
			return result, nil
		}
	}

	// Fallback to parsing as direct list
	var parsed []map[string]interface{}
	if err := json.Unmarshal(data, &parsed); err != nil {
		return nil, err
	}

	result := make([]interface{}, len(parsed))
	for i, item := range parsed {
		result[i] = item
	}
	return result, nil
}

func formatApplicationSummary(app *v1alpha1.Application) map[string]interface{} {
	return map[string]interface{}{
		"name":      app.Name,
		"project":   app.Spec.Project,
		"server":    app.Spec.Destination.Server,
		"namespace": app.Spec.Destination.Namespace,
		"status":    app.Status.Sync.Status,
		"health":    app.Status.Health.Status,
	}
}

func formatApplicationDetail(app *v1alpha1.Application) map[string]interface{} {
	// Health.Message is deprecated but we still use it for backward compatibility
	//lint:ignore SA1019 Health.Message is deprecated
	healthMessage := app.Status.Health.Message
	return map[string]interface{}{
		"name":            app.Name,
		"project":         app.Spec.Project,
		"repo_url":        app.Spec.Source.RepoURL,
		"path":            app.Spec.Source.Path,
		"target_revision": app.Spec.Source.TargetRevision,
		"server":          app.Spec.Destination.Server,
		"namespace":       app.Spec.Destination.Namespace,
		"status":          app.Status.Sync.Status,
		"health":          app.Status.Health.Status,
		"health_message":  healthMessage,
		"revision":        app.Status.Sync.Revision,
	}
}

// checkSafeMode returns an error result if safe mode is enabled for write operations
func (tm *ToolManager) checkSafeMode(operation string) *mcp.CallToolResult {
	if tm.safeMode {
		return errorResult(fmt.Sprintf("Operation '%s' is not allowed in safe mode. Safe mode restricts write operations for security.", operation))
	}
	return nil
}

// buildClusterConfig builds a v1alpha1.ClusterConfig from the arguments map
func buildClusterConfig(arguments map[string]interface{}) (v1alpha1.ClusterConfig, error) {
	config := v1alpha1.ClusterConfig{}

	// Get config map if it exists
	configMap, ok := arguments["config"].(map[string]interface{})
	if !ok || len(configMap) == 0 {
		return config, nil
	}

	// Parse username
	if username, ok := configMap["username"].(string); ok {
		config.Username = username
	}

	// Parse password
	if password, ok := configMap["password"].(string); ok {
		config.Password = password
	}

	// Parse bearer token
	if bearerToken, ok := configMap["bearerToken"].(string); ok {
		config.BearerToken = bearerToken
	}

	// Parse TLS client config if provided
	if tlsClientConfigMap, ok := configMap["tlsClientConfig"].(map[string]interface{}); ok {
		tlsClientConfig := v1alpha1.TLSClientConfig{}
		if insecure, ok := tlsClientConfigMap["insecure"].(bool); ok {
			tlsClientConfig.Insecure = insecure
		}
		if caData, ok := tlsClientConfigMap["caData"].(string); ok {
			tlsClientConfig.CAData = []byte(caData)
		}
		if certData, ok := tlsClientConfigMap["certData"].(string); ok {
			tlsClientConfig.CertData = []byte(certData)
		}
		if keyData, ok := tlsClientConfigMap["keyData"].(string); ok {
			tlsClientConfig.KeyData = []byte(keyData)
		}
		config.TLSClientConfig = tlsClientConfig
	}

	return config, nil
}
