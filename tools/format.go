package tools

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/argoproj/argo-cd/v3/pkg/apis/application/v1alpha1"
	healthlib "github.com/argoproj/gitops-engine/pkg/health"
	synccommon "github.com/argoproj/gitops-engine/pkg/sync/common"
	"github.com/denysvitali/argocd-mcp/internal/client"
)

// inClusterServer is ArgoCD's address for the cluster it runs in. It is
// the destination for most applications, so repeating it on every row of
// a list costs tokens and says nothing.
const inClusterServer = "https://kubernetes.default.svc"

// dropEmptyLogEntries removes entries carrying no content. ArgoCD
// terminates a log stream with one, and counting it inflated every line
// count by one — "no matching lines" rendered as "1 lines" of nothing.
func dropEmptyLogEntries(entries []client.ApplicationLogEntry) []client.ApplicationLogEntry {
	kept := make([]client.ApplicationLogEntry, 0, len(entries))
	for _, entry := range entries {
		if strings.TrimSpace(entry.Content) == "" {
			continue
		}
		kept = append(kept, entry)
	}
	return kept
}

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
		"apps":                      true,
		"batch":                     true,
		"networking.k8s.io":         true,
		"policy":                    true,
		"storage.k8s.io":            true,
		"rbac.authorization.k8s.io": true,
		"coordination.k8s.io":       true,
		"apiserverinternal.k8s.io":  true,
		"scheduling.k8s.io":         true,
	}

	if commonV1Groups[group] {
		return "v1"
	}

	// For custom groups (like postgresql.cnpg.io), also default to v1
	// as most CRDs use v1
	return "v1"
}

// resolveResourceVersion determines the API version to address a resource
// with. It prefers the version ArgoCD reports for that resource in the
// application's resource tree, which is authoritative, and only falls back
// to guessing from the group.
//
// The guess is wrong for any CRD not served at v1 — e.g. an
// actions.github.com/v1alpha1 AutoscalingRunnerSet, which the API rejects
// with "could not find the requested resource" — so anything addressable
// in the tree should be resolved from it.
func (tm *ToolManager) resolveResourceVersion(ctx context.Context, appName, group, kind, namespace, resourceName string) string {
	if appName == "" || kind == "" || resourceName == "" {
		return inferResourceVersion(group)
	}

	tree, err := tm.client.GetResourceTree(ctx, appName)
	if err != nil || tree == nil {
		return inferResourceVersion(group)
	}

	for _, node := range tree.Nodes {
		if node.Kind != kind || node.Name != resourceName || node.Group != group {
			continue
		}
		if namespace != "" && node.Namespace != namespace {
			continue
		}
		if node.Version != "" {
			return node.Version
		}
	}

	return inferResourceVersion(group)
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
	// Count out-of-sync resources
	outOfSyncCount := 0
	for _, r := range app.Status.Resources {
		if r.Status == v1alpha1.SyncStatusCodeOutOfSync {
			outOfSyncCount++
		}
	}

	// Safely extract health status
	var healthStatus healthlib.HealthStatusCode
	if app.Status.Health.Status != "" {
		healthStatus = app.Status.Health.Status
	}

	// Safely extract sync status
	var syncStatus v1alpha1.SyncStatusCode
	if app.Status.Sync.Status != "" {
		syncStatus = app.Status.Sync.Status
	}

	// Get operation state info
	var operationPhase string
	var operationMessage string
	if app.Status.OperationState != nil {
		operationPhase = string(app.Status.OperationState.Phase)
		operationMessage = app.Status.OperationState.Message
	}

	// Format conditions
	conditions := make([]map[string]string, 0, len(app.Status.Conditions))
	for _, c := range app.Status.Conditions {
		conditions = append(conditions, map[string]string{
			"type":    string(c.Type),
			"message": c.Message,
		})
	}

	// Determine if there are any issues
	hasIssues := outOfSyncCount > 0 ||
		healthStatus != healthlib.HealthStatusHealthy ||
		(syncStatus != v1alpha1.SyncStatusCodeSynced && syncStatus != "") ||
		(app.Status.OperationState != nil &&
			(app.Status.OperationState.Phase == synccommon.OperationFailed ||
				app.Status.OperationState.Phase == synccommon.OperationError)) ||
		len(app.Status.Conditions) > 0

	// Fields that carry no information are omitted rather than repeated
	// on every row: across 55 applications, "server: <in-cluster>",
	// "has_issues: false", "out_of_sync_count: 0" and a successful
	// operation message accounted for most of the response. Absence is
	// the default, and the tool description says so.
	result := map[string]interface{}{
		"name":      app.Name,
		"project":   app.Spec.Project,
		"namespace": app.Spec.Destination.Namespace,
		"status":    syncStatus,
		"health":    healthStatus,
	}
	if app.Spec.Destination.Server != "" && app.Spec.Destination.Server != inClusterServer {
		result["server"] = app.Spec.Destination.Server
	}
	if outOfSyncCount > 0 {
		result["out_of_sync_count"] = outOfSyncCount
	}
	if hasIssues {
		result["has_issues"] = true
	}

	// Include conditions if present
	if len(conditions) > 0 {
		result["conditions"] = conditions
	}

	// A succeeded operation is the uninteresting case; its phase and
	// "successfully synced" message are noise on every healthy row.
	if operationPhase != "" && operationPhase != string(synccommon.OperationSucceeded) {
		result["operation_phase"] = operationPhase
		if operationMessage != "" {
			result["operation_message"] = operationMessage
		}
	}

	return result
}

func formatApplicationDetail(app *v1alpha1.Application) map[string]interface{} {
	// Safely extract health info
	var healthStatus healthlib.HealthStatusCode
	var healthMessage string
	healthStatus = app.Status.Health.Status
	// Health.Message is deprecated but we still use it for backward compatibility
	//lint:ignore SA1019 Health.Message is deprecated
	healthMessage = app.Status.Health.Message

	// Safely extract sync info
	var syncStatus v1alpha1.SyncStatusCode
	var syncRevision string
	syncStatus = app.Status.Sync.Status
	syncRevision = app.Status.Sync.Revision

	// Safely extract source info
	var repoURL, path, targetRevision string
	if app.Spec.Source != nil {
		repoURL = app.Spec.Source.RepoURL
		path = app.Spec.Source.Path
		targetRevision = app.Spec.Source.TargetRevision
	}

	// Count out-of-sync resources
	outOfSyncCount := 0
	for _, r := range app.Status.Resources {
		if r.Status == v1alpha1.SyncStatusCodeOutOfSync {
			outOfSyncCount++
		}
	}

	// Determine if there are any issues
	hasIssues := outOfSyncCount > 0 ||
		healthStatus != healthlib.HealthStatusHealthy ||
		(app.Status.OperationState != nil &&
			(app.Status.OperationState.Phase == synccommon.OperationFailed ||
				app.Status.OperationState.Phase == synccommon.OperationError))

	// Get operation state info
	var operationPhase string
	var operationMessage string
	if app.Status.OperationState != nil {
		operationPhase = string(app.Status.OperationState.Phase)
		operationMessage = app.Status.OperationState.Message
	}

	// Format conditions
	conditions := make([]map[string]interface{}, 0, len(app.Status.Conditions))
	for _, c := range app.Status.Conditions {
		conditions = append(conditions, map[string]interface{}{
			"type":    c.Type,
			"message": c.Message,
		})
	}

	// Format resources with sync status
	resources := make([]map[string]interface{}, 0, len(app.Status.Resources))
	for _, r := range app.Status.Resources {
		resHealthStatus := ""
		if r.Health != nil {
			resHealthStatus = string(r.Health.Status)
		}
		resources = append(resources, map[string]interface{}{
			"group":     r.Group,
			"kind":      r.Kind,
			"namespace": r.Namespace,
			"name":      r.Name,
			"status":    r.Status,
			"health":    resHealthStatus,
		})
	}

	return map[string]interface{}{
		"name":              app.Name,
		"project":           app.Spec.Project,
		"repo_url":          repoURL,
		"path":              path,
		"target_revision":   targetRevision,
		"server":            app.Spec.Destination.Server,
		"namespace":         app.Spec.Destination.Namespace,
		"status":            syncStatus,
		"health":            healthStatus,
		"health_message":    healthMessage,
		"revision":          syncRevision,
		"out_of_sync_count": outOfSyncCount,
		"has_issues":        hasIssues,
		"operation_phase":   operationPhase,
		"operation_message": operationMessage,
		"conditions":        conditions,
		"resources":         resources,
	}
}
