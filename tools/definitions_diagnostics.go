package tools

import "github.com/mark3labs/mcp-go/mcp"

// diagnosticsToolDefinitions returns the MCP tool definitions for the diagnostics domain.
func diagnosticsToolDefinitions() []mcp.Tool {
	return []mcp.Tool{
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
	}
}
