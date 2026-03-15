package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/argoproj/argo-cd/v3/pkg/apiclient/applicationset"
	"github.com/argoproj/argo-cd/v3/pkg/apis/application/v1alpha1"
	"github.com/mark3labs/mcp-go/mcp"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	yaml "sigs.k8s.io/yaml"
)

// ApplicationSetSummary is the typed summary of an ApplicationSet returned to the LLM.
// sigs.k8s.io/yaml marshals using json tags, so we use json tags for wire format.
type ApplicationSetSummary struct {
	Name             string                           `json:"name"`
	Namespace        string                           `json:"namespace,omitempty"`
	GeneratorTypes   []string                         `json:"generator_types"`
	ApplicationCount int64                            `json:"application_count"`
	Conditions       []ApplicationSetConditionSummary `json:"conditions,omitempty"`
	HasErrors        bool                             `json:"has_errors"`
	Strategy         string                           `json:"strategy,omitempty"`
}

// ApplicationSetConditionSummary is a concise representation of an ApplicationSet condition.
type ApplicationSetConditionSummary struct {
	Type    string `json:"type"`
	Status  string `json:"status"`
	Reason  string `json:"reason"`
	Message string `json:"message"`
}

// ApplicationSetDetail is the detailed view of an ApplicationSet.
type ApplicationSetDetail struct {
	Name              string                          `json:"name"`
	Namespace         string                          `json:"namespace,omitempty"`
	GeneratorTypes    []string                        `json:"generator_types"`
	ApplicationCount  int64                           `json:"application_count"`
	Conditions        []ApplicationSetConditionSummary `json:"conditions,omitempty"`
	HasErrors         bool                            `json:"has_errors"`
	Strategy          string                          `json:"strategy,omitempty"`
	Template          ApplicationSetTemplateSummary   `json:"template"`
	ApplicationStatus []ApplicationSetAppStatusSummary `json:"application_status,omitempty"`
}

// ApplicationSetTemplateSummary captures the key fields of the app template.
type ApplicationSetTemplateSummary struct {
	Project        string `json:"project"`
	RepoURL        string `json:"repo_url,omitempty"`
	Path           string `json:"path,omitempty"`
	TargetRevision string `json:"target_revision,omitempty"`
	DestServer     string `json:"destination_server,omitempty"`
	DestNamespace  string `json:"destination_namespace,omitempty"`
}

// ApplicationSetAppStatusSummary holds per-app rollout status from progressive sync.
type ApplicationSetAppStatusSummary struct {
	Application string `json:"application"`
	Status      string `json:"status"`
	Step        string `json:"step,omitempty"`
	Message     string `json:"message,omitempty"`
}

// ApplicationPreviewSummary is the concise view of a would-be generated Application.
type ApplicationPreviewSummary struct {
	Name           string `json:"name"`
	Project        string `json:"project"`
	DestServer     string `json:"destination_server,omitempty"`
	DestNamespace  string `json:"destination_namespace,omitempty"`
	RepoURL        string `json:"repo_url,omitempty"`
	Path           string `json:"path,omitempty"`
	TargetRevision string `json:"target_revision,omitempty"`
	AutoSync       bool   `json:"auto_sync"`
	SelfHeal       bool   `json:"self_heal"`
}

// ApplicationSetPreviewResult is the full output of preview_applicationset.
type ApplicationSetPreviewResult struct {
	TotalApplications int                         `json:"total_applications"`
	Applications      []ApplicationPreviewSummary `json:"applications"`
	Note              string                      `json:"note"`
}

// applicationSetToolDefinitions returns the tool definitions for ApplicationSet tools.
// These are appended to the main tool list by defineTools().
func applicationSetToolDefinitions() []mcp.Tool {
	return []mcp.Tool{
		{
			Name:        "list_applicationsets",
			Description: "List all ArgoCD ApplicationSets with their generator types, condition status, and managed application counts",
			InputSchema: mcp.ToolInputSchema{
				Type: "object",
				Properties: map[string]interface{}{
					"project": map[string]interface{}{
						"type":        "string",
						"description": "Filter ApplicationSets by project name",
					},
					"limit": map[string]interface{}{
						"type":        "integer",
						"description": "Maximum number of ApplicationSets to return (default: 50, max: 100)",
					},
				},
			},
		},
		{
			Name:        "get_applicationset",
			Description: "Get full details of a specific ApplicationSet including generator config, conditions, progressive rollout status, and the resource tree of generated applications",
			InputSchema: mcp.ToolInputSchema{
				Type: "object",
				Properties: map[string]interface{}{
					"name": map[string]interface{}{
						"type":        "string",
						"description": "ApplicationSet name (required)",
					},
				},
				Required: []string{"name"},
			},
		},
		{
			Name: "preview_applicationset",
			Description: "Dry-run preview of an ApplicationSet spec: calls the ArgoCD Generate API to show " +
				"exactly which Applications would be created without making any changes. " +
				"Accepts either a full ApplicationSet YAML/JSON spec string or a name of an existing ApplicationSet to re-evaluate. " +
				"This is a read-only operation — nothing is created or modified.",
			InputSchema: mcp.ToolInputSchema{
				Type: "object",
				Properties: map[string]interface{}{
					"spec": map[string]interface{}{
						"type": "string",
						"description": "Full ApplicationSet YAML or JSON spec to preview. " +
							"Mutually exclusive with 'name'. The spec must include at least: " +
							"metadata.name, spec.generators, and spec.template.",
					},
					"name": map[string]interface{}{
						"type": "string",
						"description": "Name of an existing ApplicationSet to re-evaluate through the generator. " +
							"Mutually exclusive with 'spec'.",
					},
				},
			},
		},
		{
			Name:        "create_applicationset",
			Description: "Create a new ArgoCD ApplicationSet from a YAML or JSON spec. Blocked in safe mode.",
			InputSchema: mcp.ToolInputSchema{
				Type: "object",
				Properties: map[string]interface{}{
					"spec": map[string]interface{}{
						"type":        "string",
						"description": "Full ApplicationSet YAML or JSON spec (required)",
					},
					"upsert": map[string]interface{}{
						"type":        "boolean",
						"description": "If true, update the ApplicationSet if it already exists (default: false)",
					},
				},
				Required: []string{"spec"},
			},
		},
		{
			Name:        "delete_applicationset",
			Description: "Delete an ArgoCD ApplicationSet. Blocked in safe mode.",
			InputSchema: mcp.ToolInputSchema{
				Type: "object",
				Properties: map[string]interface{}{
					"name": map[string]interface{}{
						"type":        "string",
						"description": "ApplicationSet name (required)",
					},
				},
				Required: []string{"name"},
			},
		},
	}
}

// handleListApplicationSets lists ApplicationSets with optional project filter.
func (tm *ToolManager) handleListApplicationSets(ctx context.Context, arguments map[string]interface{}) (*mcp.CallToolResult, error) {
	project := String(arguments, "project", "")
	limit := Int(arguments, "limit", MaxListItems)
	if limit > 100 {
		limit = 100
	}

	query := &applicationset.ApplicationSetListQuery{}
	if project != "" {
		query.Projects = []string{project}
	}

	list, err := tm.client.ListApplicationSets(ctx, query)
	if err != nil {
		return errorResult(fmt.Sprintf("failed to list applicationsets: %v", err)), nil
	}

	total := len(list.Items)
	if len(list.Items) > limit {
		list.Items = list.Items[:limit]
	}

	items := make([]interface{}, len(list.Items))
	for i, as := range list.Items {
		items[i] = formatApplicationSetSummary(&as)
	}

	return ResultList(items, total, nil)
}

// handleGetApplicationSet returns full detail for a single ApplicationSet.
func (tm *ToolManager) handleGetApplicationSet(ctx context.Context, arguments map[string]interface{}) (*mcp.CallToolResult, error) {
	name := String(arguments, "name", "")
	if name == "" {
		return errorResult("name is required"), nil
	}

	as, err := tm.client.GetApplicationSet(ctx, &applicationset.ApplicationSetGetQuery{Name: name})
	if err != nil {
		return errorResult(fmt.Sprintf("failed to get applicationset %q: %v", name, err)), nil
	}

	detail := formatApplicationSetDetail(as)
	return Result(detail, nil)
}

// handlePreviewApplicationSet runs the Generate dry-run API and returns a structured preview.
func (tm *ToolManager) handlePreviewApplicationSet(ctx context.Context, arguments map[string]interface{}) (*mcp.CallToolResult, error) {
	specStr := String(arguments, "spec", "")
	nameStr := String(arguments, "name", "")

	if specStr == "" && nameStr == "" {
		return errorResult("either 'spec' or 'name' must be provided"), nil
	}
	if specStr != "" && nameStr != "" {
		return errorResult("only one of 'spec' or 'name' may be provided, not both"), nil
	}

	var appSet *v1alpha1.ApplicationSet

	if nameStr != "" {
		// Fetch the existing ApplicationSet from ArgoCD and use it as the spec.
		existing, err := tm.client.GetApplicationSet(ctx, &applicationset.ApplicationSetGetQuery{Name: nameStr})
		if err != nil {
			return errorResult(fmt.Sprintf("failed to fetch applicationset %q: %v", nameStr, err)), nil
		}
		appSet = existing
	} else {
		// Parse the user-supplied YAML or JSON spec.
		appSet = &v1alpha1.ApplicationSet{}
		if err := yaml.Unmarshal([]byte(specStr), appSet); err != nil {
			// Try JSON as fallback
			if jsonErr := json.Unmarshal([]byte(specStr), appSet); jsonErr != nil {
				return errorResult(fmt.Sprintf("failed to parse applicationset spec (tried YAML and JSON): yaml=%v json=%v", err, jsonErr)), nil
			}
		}
		// Ensure required API fields are populated so the server can process the request.
		if appSet.APIVersion == "" {
			appSet.APIVersion = "argoproj.io/v1alpha1"
		}
		if appSet.Kind == "" {
			appSet.Kind = "ApplicationSet"
		}
		if appSet.Name == "" {
			appSet.Name = "preview"
		}
		if appSet.CreationTimestamp.IsZero() {
			appSet.CreationTimestamp = metav1.Now()
		}
	}

	apps, err := tm.client.PreviewApplicationSet(ctx, appSet)
	if err != nil {
		return errorResult(fmt.Sprintf("preview failed: %v", err)), nil
	}

	summaries := make([]ApplicationPreviewSummary, 0, len(apps))
	for _, app := range apps {
		if app == nil {
			continue
		}
		s := ApplicationPreviewSummary{
			Name:    app.Name,
			Project: app.Spec.Project,
		}
		if app.Spec.Source != nil {
			s.RepoURL = app.Spec.Source.RepoURL
			s.Path = app.Spec.Source.Path
			s.TargetRevision = app.Spec.Source.TargetRevision
		}
		s.DestServer = app.Spec.Destination.Server
		s.DestNamespace = app.Spec.Destination.Namespace
		if app.Spec.SyncPolicy != nil {
			s.AutoSync = app.Spec.SyncPolicy.Automated != nil
			if app.Spec.SyncPolicy.Automated != nil {
				s.SelfHeal = app.Spec.SyncPolicy.Automated.SelfHeal
			}
		}
		summaries = append(summaries, s)
	}

	result := ApplicationSetPreviewResult{
		TotalApplications: len(summaries),
		Applications:      summaries,
		Note:              "This is a dry-run preview. No Applications have been created or modified.",
	}

	return Result(result, nil)
}

// handleCreateApplicationSet creates a new ApplicationSet from a YAML/JSON spec.
func (tm *ToolManager) handleCreateApplicationSet(ctx context.Context, arguments map[string]interface{}) (*mcp.CallToolResult, error) {
	if result := tm.checkSafeMode("create_applicationset"); result != nil {
		return result, nil
	}

	specStr := String(arguments, "spec", "")
	if specStr == "" {
		return errorResult("spec is required"), nil
	}
	upsert := Bool(arguments, "upsert", false)

	appSet := &v1alpha1.ApplicationSet{}
	if err := yaml.Unmarshal([]byte(specStr), appSet); err != nil {
		if jsonErr := json.Unmarshal([]byte(specStr), appSet); jsonErr != nil {
			return errorResult(fmt.Sprintf("failed to parse applicationset spec: yaml=%v json=%v", err, jsonErr)), nil
		}
	}
	if appSet.APIVersion == "" {
		appSet.APIVersion = "argoproj.io/v1alpha1"
	}
	if appSet.Kind == "" {
		appSet.Kind = "ApplicationSet"
	}

	created, err := tm.client.CreateApplicationSet(ctx, &applicationset.ApplicationSetCreateRequest{
		Applicationset: appSet,
		Upsert:         upsert,
	})
	if err != nil {
		return errorResult(fmt.Sprintf("failed to create applicationset: %v", err)), nil
	}

	return Result(formatApplicationSetDetail(created), nil)
}

// handleDeleteApplicationSet deletes an ApplicationSet by name.
func (tm *ToolManager) handleDeleteApplicationSet(ctx context.Context, arguments map[string]interface{}) (*mcp.CallToolResult, error) {
	if result := tm.checkSafeMode("delete_applicationset"); result != nil {
		return result, nil
	}

	name := String(arguments, "name", "")
	if name == "" {
		return errorResult("name is required"), nil
	}

	if err := tm.client.DeleteApplicationSet(ctx, &applicationset.ApplicationSetDeleteRequest{Name: name}); err != nil {
		return errorResult(fmt.Sprintf("failed to delete applicationset %q: %v", name, err)), nil
	}

	return Result(map[string]string{
		"status":  "deleted",
		"name":    name,
		"message": fmt.Sprintf("ApplicationSet %q has been deleted", name),
	}, nil)
}

// formatApplicationSetSummary returns a concise summary of an ApplicationSet.
func formatApplicationSetSummary(as *v1alpha1.ApplicationSet) ApplicationSetSummary {
	genTypes := generatorTypes(as)
	conditions := formatAppSetConditions(as.Status.Conditions)
	hasErrors := appSetHasErrors(as.Status.Conditions)
	strategy := appSetStrategyName(as)

	count := as.Status.ResourcesCount
	if count == 0 {
		count = int64(len(as.Status.Resources))
	}

	return ApplicationSetSummary{
		Name:             as.Name,
		Namespace:        as.Namespace,
		GeneratorTypes:   genTypes,
		ApplicationCount: count,
		Conditions:       conditions,
		HasErrors:        hasErrors,
		Strategy:         strategy,
	}
}

// formatApplicationSetDetail returns a detailed view of an ApplicationSet.
func formatApplicationSetDetail(as *v1alpha1.ApplicationSet) ApplicationSetDetail {
	genTypes := generatorTypes(as)
	conditions := formatAppSetConditions(as.Status.Conditions)
	hasErrors := appSetHasErrors(as.Status.Conditions)
	strategy := appSetStrategyName(as)

	count := as.Status.ResourcesCount
	if count == 0 {
		count = int64(len(as.Status.Resources))
	}

	tmpl := ApplicationSetTemplateSummary{
		Project:       as.Spec.Template.Spec.Project,
		DestServer:    as.Spec.Template.Spec.Destination.Server,
		DestNamespace: as.Spec.Template.Spec.Destination.Namespace,
	}
	if as.Spec.Template.Spec.Source != nil {
		tmpl.RepoURL = as.Spec.Template.Spec.Source.RepoURL
		tmpl.Path = as.Spec.Template.Spec.Source.Path
		tmpl.TargetRevision = as.Spec.Template.Spec.Source.TargetRevision
	}

	appStatuses := make([]ApplicationSetAppStatusSummary, 0, len(as.Status.ApplicationStatus))
	for _, s := range as.Status.ApplicationStatus {
		appStatuses = append(appStatuses, ApplicationSetAppStatusSummary{
			Application: s.Application,
			Status:      s.Status,
			Step:        s.Step,
			Message:     s.Message,
		})
	}

	return ApplicationSetDetail{
		Name:              as.Name,
		Namespace:         as.Namespace,
		GeneratorTypes:    genTypes,
		ApplicationCount:  count,
		Conditions:        conditions,
		HasErrors:         hasErrors,
		Strategy:          strategy,
		Template:          tmpl,
		ApplicationStatus: appStatuses,
	}
}

// generatorTypes extracts the names of active generator types from an ApplicationSet spec.
func generatorTypes(as *v1alpha1.ApplicationSet) []string {
	seen := make(map[string]struct{})
	for _, g := range as.Spec.Generators {
		for _, name := range generatorTypeNames(&g) {
			seen[name] = struct{}{}
		}
	}
	names := make([]string, 0, len(seen))
	for n := range seen {
		names = append(names, n)
	}
	return names
}

// generatorTypeNames returns a list of type names for a single ApplicationSetGenerator.
func generatorTypeNames(g *v1alpha1.ApplicationSetGenerator) []string {
	var names []string
	if g.List != nil {
		names = append(names, "List")
	}
	if g.Clusters != nil {
		names = append(names, "Clusters")
	}
	if g.Git != nil {
		names = append(names, "Git")
	}
	if g.SCMProvider != nil {
		names = append(names, "SCMProvider")
	}
	if g.ClusterDecisionResource != nil {
		names = append(names, "ClusterDecisionResource")
	}
	if g.PullRequest != nil {
		names = append(names, "PullRequest")
	}
	if g.Matrix != nil {
		names = append(names, "Matrix")
	}
	if g.Merge != nil {
		names = append(names, "Merge")
	}
	return names
}

// formatAppSetConditions converts ApplicationSet conditions to a concise summary slice.
func formatAppSetConditions(conditions []v1alpha1.ApplicationSetCondition) []ApplicationSetConditionSummary {
	result := make([]ApplicationSetConditionSummary, 0, len(conditions))
	for _, c := range conditions {
		result = append(result, ApplicationSetConditionSummary{
			Type:    string(c.Type),
			Status:  string(c.Status),
			Reason:  c.Reason,
			Message: c.Message,
		})
	}
	return result
}

// appSetHasErrors returns true if any condition indicates an error.
func appSetHasErrors(conditions []v1alpha1.ApplicationSetCondition) bool {
	for _, c := range conditions {
		if c.Type == v1alpha1.ApplicationSetConditionErrorOccurred &&
			c.Status == v1alpha1.ApplicationSetConditionStatusTrue {
			return true
		}
	}
	return false
}

// appSetStrategyName returns a human-readable strategy name.
func appSetStrategyName(as *v1alpha1.ApplicationSet) string {
	if as.Spec.Strategy != nil && as.Spec.Strategy.RollingSync != nil {
		return "RollingSync"
	}
	return ""
}
