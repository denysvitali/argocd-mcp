package tools

import (
	"context"
	"fmt"

	"github.com/argoproj/argo-cd/v3/pkg/apiclient/application"
	"github.com/mark3labs/mcp-go/mcp"
)

// handleRefreshApplication forces ArgoCD to re-fetch manifests from Git
func (tm *ToolManager) handleRefreshApplication(ctx context.Context, arguments map[string]interface{}) (*mcp.CallToolResult, error) {
	if result := tm.checkSafeMode(toolRefreshApplication); result != nil {
		return result, nil
	}

	name := String(arguments, "name", "")
	refreshType := String(arguments, "refresh_type", "hard")

	query := &application.ApplicationQuery{
		Name:    &name,
		Refresh: &refreshType,
	}

	app, err := tm.client.GetApplication(ctx, query)
	if err != nil {
		return errorResult(err.Error()), nil
	}

	type refreshResult struct {
		Message  string `json:"message"`
		Success  bool   `json:"success"`
		Status   string `json:"status"`
		Health   string `json:"health"`
		Revision string `json:"revision"`
	}

	status := "Unknown"
	health := "Unknown"
	revision := ""

	if app.Status.Sync.Status != "" {
		status = string(app.Status.Sync.Status)
	}
	if app.Status.Health.Status != "" {
		health = string(app.Status.Health.Status)
	}
	if app.Status.Sync.Revision != "" {
		revision = app.Status.Sync.Revision
	}

	return Result(refreshResult{
		Message:  fmt.Sprintf("Application %s refreshed successfully (type: %s)", name, refreshType),
		Success:  true,
		Status:   status,
		Health:   health,
		Revision: revision,
	}, nil)
}

// handleTerminateOperation terminates the currently running operation on an application
func (tm *ToolManager) handleTerminateOperation(ctx context.Context, arguments map[string]interface{}) (*mcp.CallToolResult, error) {
	if result := tm.checkSafeMode(toolTerminateOperation); result != nil {
		return result, nil
	}

	name := String(arguments, "name", "")
	appNamespace := String(arguments, "app_namespace", "")
	projectName := String(arguments, "project", "")

	req := &application.OperationTerminateRequest{
		Name: &name,
	}
	if appNamespace != "" {
		req.AppNamespace = &appNamespace
	}
	if projectName != "" {
		req.Project = &projectName
	}

	err := tm.client.TerminateOperation(ctx, req)
	if err != nil {
		return errorResult(err.Error()), nil
	}

	type terminateResult struct {
		Message string `json:"message"`
		Success bool   `json:"success"`
	}

	return Result(terminateResult{
		Message: fmt.Sprintf("Operation on application %s terminated successfully", name),
		Success: true,
	}, nil)
}

// handleRestartPod deletes a pod within an application to trigger a controller restart
func (tm *ToolManager) handleRestartPod(ctx context.Context, arguments map[string]interface{}) (*mcp.CallToolResult, error) {
	if result := tm.checkDeleteAllowed(toolRestartPod); result != nil {
		return result, nil
	}

	appName := String(arguments, "name", "")
	podName := String(arguments, "pod_name", "")
	namespace := String(arguments, "namespace", "")

	group := ""
	kind := "Pod"
	version := "v1"
	forceDelete := true

	deleteReq := &application.ApplicationResourceDeleteRequest{
		Name:         &appName,
		ResourceName: &podName,
		Version:      &version,
		Group:        &group,
		Kind:         &kind,
		Namespace:    &namespace,
		Force:        &forceDelete,
	}

	err := tm.client.DeleteApplicationResource(ctx, deleteReq)
	if err != nil {
		return errorResult(err.Error()), nil
	}

	type restartResult struct {
		Message   string `json:"message"`
		Success   bool   `json:"success"`
		Pod       string `json:"pod"`
		Namespace string `json:"namespace"`
	}

	return Result(restartResult{
		Message:   fmt.Sprintf("Pod %s deleted successfully — its controller will recreate it", podName),
		Success:   true,
		Pod:       podName,
		Namespace: namespace,
	}, nil)
}

// HookInfo holds the details of an ArgoCD hook resource found in the resource tree
type HookInfo struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Group     string `json:"group"`
	Kind      string `json:"kind"`
	HookType  string `json:"hook_type"`
}

// handleDeleteHook finds and deletes hook resources from an application's resource tree
func (tm *ToolManager) handleDeleteHook(ctx context.Context, arguments map[string]interface{}) (*mcp.CallToolResult, error) {
	if result := tm.checkDeleteAllowed(toolDeleteHook); result != nil {
		return result, nil
	}

	appName := String(arguments, "name", "")
	hookName := String(arguments, "hook_name", "")
	namespace := String(arguments, "namespace", "")
	hookType := String(arguments, "hook_type", "")

	// Get the resource tree to find hook resources
	tree, err := tm.client.GetResourceTree(ctx, appName)
	if err != nil {
		return errorResult(fmt.Sprintf("failed to get resource tree: %v", err)), nil
	}

	// Find matching hooks in the resource tree
	var hooks []HookInfo
	for _, node := range tree.Nodes {
		if node.ResourceRef.Name != hookName {
			continue
		}
		// Check if this node is a hook by looking at its info items
		nodeHookType := ""
		for _, info := range node.Info {
			if info.Name == "Hook" {
				nodeHookType = info.Value
				break
			}
		}
		if nodeHookType == "" {
			continue
		}
		// If a specific hook type was requested, filter by it
		if hookType != "" && nodeHookType != hookType {
			continue
		}
		// If a namespace filter was provided, apply it
		if namespace != "" && node.ResourceRef.Namespace != namespace {
			continue
		}
		hooks = append(hooks, HookInfo{
			Name:      node.ResourceRef.Name,
			Namespace: node.ResourceRef.Namespace,
			Group:     node.ResourceRef.Group,
			Kind:      node.ResourceRef.Kind,
			HookType:  nodeHookType,
		})
	}

	if len(hooks) == 0 {
		filterDesc := fmt.Sprintf("hook_name=%s", hookName)
		if hookType != "" {
			filterDesc += fmt.Sprintf(", hook_type=%s", hookType)
		}
		if namespace != "" {
			filterDesc += fmt.Sprintf(", namespace=%s", namespace)
		}
		return errorResult(fmt.Sprintf("no hook resources found matching %s in application %s", filterDesc, appName)), nil
	}

	// Delete each matching hook
	type hookDeleteResult struct {
		Hook    string `json:"hook"`
		Kind    string `json:"kind"`
		Type    string `json:"type"`
		Deleted bool   `json:"deleted"`
		Error   string `json:"error,omitempty"`
	}

	var results []hookDeleteResult
	forceDelete := true
	for _, hook := range hooks {
		version := inferResourceVersion(hook.Group)
		deleteReq := &application.ApplicationResourceDeleteRequest{
			Name:         &appName,
			ResourceName: &hook.Name,
			Version:      &version,
			Group:        &hook.Group,
			Kind:         &hook.Kind,
			Namespace:    &hook.Namespace,
			Force:        &forceDelete,
		}

		deleteErr := tm.client.DeleteApplicationResource(ctx, deleteReq)
		r := hookDeleteResult{
			Hook:    hook.Name,
			Kind:    hook.Kind,
			Type:    hook.HookType,
			Deleted: deleteErr == nil,
		}
		if deleteErr != nil {
			r.Error = deleteErr.Error()
		}
		results = append(results, r)
	}

	type deleteHookResponse struct {
		Message string             `json:"message"`
		Deleted int                `json:"deleted"`
		Failed  int                `json:"failed"`
		Results []hookDeleteResult `json:"results"`
	}

	deleted := 0
	failed := 0
	for _, r := range results {
		if r.Deleted {
			deleted++
		} else {
			failed++
		}
	}

	return Result(deleteHookResponse{
		Message: fmt.Sprintf("Processed %d hook(s) for application %s", len(results), appName),
		Deleted: deleted,
		Failed:  failed,
		Results: results,
	}, nil)
}
