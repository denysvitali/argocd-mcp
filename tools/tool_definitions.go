package tools

import "github.com/mark3labs/mcp-go/mcp"

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
		// Diagnostic tools
		{
			Name: "diagnose_application",
			Description: "Perform a comprehensive incident-response diagnosis for a single ArgoCD application. " +
				"This compound tool fans out across all relevant data sources in parallel " +
				"(application status, resource diff, resource tree health, Kubernetes warning events, " +
				"current pod logs, AND previous/crashed container logs) " +
				"and fuses the results into a single structured DiagnosticReport containing: " +
				"a machine-readable failure category (CrashLoopBackOff, OOMKilled, ImagePullBackOff, " +
				"SyncFailed, DegradedDeployment, QuotaExceeded, PodSchedulingFailed, ConfigError, " +
				"NetworkError, OutOfSync, Healthy, Unknown), " +
				"a severity classification (healthy/degraded/critical), identified root-cause signals " +
				"with evidence snippets and exact MCP tool calls for remediation, a plain-English summary, " +
				"and a prioritised list of next actions. " +
				"Use this as the FIRST tool call whenever an application is unhealthy or misbehaving. " +
				"The previous container logs are especially valuable for diagnosing CrashLoopBackOff and OOMKilled.",
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
		// Cost optimization tools
		{
			Name: "analyze_resource_efficiency",
			Description: "Analyze resource efficiency for an ArgoCD application. " +
				"Reports declared CPU/memory requests vs actual usage for all Deployments, StatefulSets and DaemonSets. " +
				"Flags over-provisioned containers, generates right-sizing suggestions with 20% headroom, " +
				"and estimates monthly cost waste. Requires metrics-server in the cluster for live usage data; " +
				"without it the tool still reports declared requests and flags missing resource requests.",
			InputSchema: mcp.ToolInputSchema{
				Type: "object",
				Properties: map[string]interface{}{
					"name": map[string]interface{}{
						"type":        "string",
						"description": "Application name (required)",
					},
					"cpu_cost_per_vcpu_hour": map[string]interface{}{
						"type":        "number",
						"description": "Cost of one vCPU-hour in USD (default: 0.048, a blended AWS/GCP/Azure average)",
					},
					"mem_cost_per_gb_hour": map[string]interface{}{
						"type":        "number",
						"description": "Cost of one GB-hour of memory in USD (default: 0.006)",
					},
				},
				Required: []string{"name"},
			},
		},
		// Operations tools
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

	// Append ApplicationSet tools defined in applicationset.go
	tm.tools = append(tm.tools, applicationSetToolDefinitions()...)
}
