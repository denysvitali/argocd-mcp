package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/argoproj/argo-cd/v3/pkg/apiclient/application"
	"github.com/argoproj/argo-cd/v3/pkg/apis/application/v1alpha1"
	"github.com/denysvitali/argocd-mcp/internal/client"
	"github.com/mark3labs/mcp-go/mcp"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Application handlers

func (tm *ToolManager) handleListApplications(ctx context.Context, arguments map[string]interface{}) (*mcp.CallToolResult, error) {
	name := String(arguments, "name", "")
	project := String(arguments, "project", "")
	limit := Int(arguments, "limit", MaxListItems)
	if limit > 100 {
		limit = 100
	}
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

	// Apply limit
	total := len(apps.Items)
	if len(apps.Items) > limit {
		apps.Items = apps.Items[:limit]
	}

	items := make([]interface{}, len(apps.Items))
	for i, app := range apps.Items {
		items[i] = formatApplicationSummary(&app)
	}

	return ResultList(items, total, nil)
}

func (tm *ToolManager) handleGetApplication(ctx context.Context, arguments map[string]interface{}) (*mcp.CallToolResult, error) {
	name := String(arguments, "name", "")
	query := &application.ApplicationQuery{
		Name: &name,
	}

	app, err := tm.client.GetApplication(ctx, query)
	if err != nil {
		// Fall back to list API which may have broader permissions
		if strings.Contains(err.Error(), "PermissionDenied") || strings.Contains(err.Error(), "permission denied") {
			tm.logger.Infof("get_application permission denied for %q, falling back to list", name)
			return tm.getApplicationFromList(ctx, name)
		}
		return errorResult(err.Error()), nil
	}

	return Result(formatApplicationDetail(app), nil)
}

func (tm *ToolManager) getApplicationFromList(ctx context.Context, name string) (*mcp.CallToolResult, error) {
	listQuery := &application.ApplicationQuery{
		Name: &name,
	}
	apps, err := tm.client.ListApplications(ctx, listQuery)
	if err != nil {
		return errorResult(fmt.Sprintf("fallback list also failed: %v", err)), nil
	}
	for i := range apps.Items {
		if apps.Items[i].Name == name {
			return Result(formatApplicationDetail(&apps.Items[i]), nil)
		}
	}
	return errorResult(fmt.Sprintf("application %q not found", name)), nil
}

func (tm *ToolManager) handleCreateApplication(ctx context.Context, arguments map[string]interface{}) (*mcp.CallToolResult, error) {
	if result := tm.checkSafeMode(toolCreateApplication); result != nil {
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
	if result := tm.checkDeleteAllowed(toolDeleteApplication); result != nil {
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
	if result := tm.checkSafeMode(toolSyncApplication); result != nil {
		return result, nil
	}

	name := String(arguments, "name", "")
	revision := String(arguments, "revision", "")
	prune := Bool(arguments, "prune", false)

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
		"status":   string(app.Status.Sync.Status),
		"health":   string(app.Status.Health.Status),
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

	// Apply limit
	total := len(manifests)
	if len(manifests) > MaxManifests {
		manifests = manifests[:MaxManifests]
	}

	// Convert manifests from JSON to YAML with truncation
	yamlManifests := make([]string, len(manifests))
	for i, m := range manifests {
		yamlManifests[i] = truncateString(jsonToYaml(m), MaxResponseSizeChars)
	}

	return Result(map[string]interface{}{
		"manifests": yamlManifests,
		"count":     len(manifests),
		"total":     total,
		"limited":   total > MaxManifests,
	}, nil)
}

func (tm *ToolManager) handleGetApplicationDiff(ctx context.Context, arguments map[string]interface{}) (*mcp.CallToolResult, error) {
	name := String(arguments, "name", "")
	limit := Int(arguments, "limit", MaxDiffResources)

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
			// Limit the number of out-of-sync resources reported
			if len(outOfSync) >= limit {
				continue
			}
			// Strip managedFields and convert to YAML
			targetState := stripManagedFieldsYaml(r.TargetState)
			liveState := stripManagedFieldsYaml(r.NormalizedLiveState)

			// Compute diff between target and live states
			diff := computeDiff(targetState, liveState)

			resourceInfo["status"] = "OutOfSync"
			resourceInfo["target"] = truncateString(targetState, MaxResponseSizeChars/2)
			resourceInfo["live"] = truncateString(liveState, MaxResponseSizeChars/2)
			resourceInfo["diff"] = diff
			resourceInfo["resource_version"] = r.ResourceVersion
			outOfSync = append(outOfSync, resourceInfo)
		} else if len(synced) < limit {
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
		"limited":           len(resources) > limit,
	}, nil)
}

func (tm *ToolManager) handleGetApplicationEvents(ctx context.Context, arguments map[string]interface{}) (*mcp.CallToolResult, error) {
	name := String(arguments, "name", "")
	resourceName := String(arguments, "resource_name", "")
	group := String(arguments, "group", "")
	kind := String(arguments, "kind", "")
	namespace := String(arguments, "namespace", "")
	limit := Int(arguments, "limit", MaxEvents)

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

	// Filter events by resource if specified
	var filteredEvents []interface{}
	for _, event := range events {
		eventMap, ok := event.(map[string]interface{})
		if !ok {
			continue
		}

		// Check involvedObject for resource filtering
		involvedObj, hasInvolved := eventMap["involvedObject"].(map[string]interface{})
		if !hasInvolved {
			// If no involvedObject, include the event unless filtering is active
			if resourceName == "" && group == "" && kind == "" && namespace == "" {
				filteredEvents = append(filteredEvents, event)
			}
			continue
		}

		// Apply filters
		if resourceName != "" {
			objName, _ := involvedObj["name"].(string)
			if objName != resourceName {
				continue
			}
		}
		if group != "" {
			objGroup, _ := involvedObj["group"].(string)
			if objGroup != group {
				continue
			}
		}
		if kind != "" {
			objKind, _ := involvedObj["kind"].(string)
			if objKind != kind {
				continue
			}
		}
		if namespace != "" {
			objNS, _ := involvedObj["namespace"].(string)
			if objNS != namespace {
				continue
			}
		}

		filteredEvents = append(filteredEvents, event)
	}

	total := len(filteredEvents)
	if len(filteredEvents) > limit {
		filteredEvents = filteredEvents[:limit]
	}

	eventList := make([]interface{}, len(filteredEvents))
	for i, event := range filteredEvents {
		eventMap, ok := event.(map[string]interface{})
		if !ok {
			continue
		}
		eventList[i] = map[string]interface{}{
			"type":            eventMap["type"],
			"reason":          eventMap["reason"],
			"message":         eventMap["message"],
			"timestamp":       eventMap["timestamp"],
			"count":           eventMap["count"],
			"first_timestamp": eventMap["firstTimestamp"],
			"last_timestamp":  eventMap["lastTimestamp"],
			"source":          eventMap["source"],
			"resource": map[string]interface{}{
				"name":      involvedObjField(eventMap, "name"),
				"namespace": involvedObjField(eventMap, "namespace"),
				"kind":      involvedObjField(eventMap, "kind"),
				"group":     involvedObjField(eventMap, "group"),
			},
		}
	}

	return Result(map[string]interface{}{
		"items":    eventList,
		"total":    total,
		"filtered": total != len(events),
		"filter_used": map[string]interface{}{
			"resource_name": resourceName,
			"group":         group,
			"kind":          kind,
			"namespace":     namespace,
		},
	}, nil)
}

// involvedObjField safely extracts a field from involvedObject
func involvedObjField(event map[string]interface{}, field string) string {
	if involved, ok := event["involvedObject"].(map[string]interface{}); ok {
		if val, ok := involved[field].(string); ok {
			return val
		}
	}
	return ""
}

func (tm *ToolManager) handleUpdateApplication(ctx context.Context, arguments map[string]interface{}) (*mcp.CallToolResult, error) {
	if result := tm.checkSafeMode(toolUpdateApplication); result != nil {
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
	if result := tm.checkSafeMode(toolRollbackApplication); result != nil {
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
		"status":   string(app.Status.Sync.Status),
		"health":   string(app.Status.Health.Status),
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
	if result := tm.checkSafeMode(toolRunResourceAction); result != nil {
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

	actionReq := &application.ResourceActionRunRequestV2{
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
	if result := tm.checkSafeMode(toolPatchApplicationResource); result != nil {
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
	if result := tm.checkDeleteAllowed(toolDeleteApplicationResource); result != nil {
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

func (tm *ToolManager) handleGetLogs(ctx context.Context, arguments map[string]interface{}) (*mcp.CallToolResult, error) {
	name := String(arguments, "name", "")
	namespace := String(arguments, "namespace", "")
	podName := String(arguments, "pod_name", "")
	container := String(arguments, "container", "")
	kind := String(arguments, "kind", "")
	group := String(arguments, "group", "")
	resourceName := String(arguments, "resource_name", "")
	tailLines := Int(arguments, "tail_lines", 100)
	sinceSeconds := Int64(arguments, "since_seconds", 0)
	filter := String(arguments, "filter", "")
	previous := Bool(arguments, "previous", false)

	// Limit tail_lines to prevent context explosion
	if tailLines > client.MaxLogEntries {
		tailLines = client.MaxLogEntries
	}
	if tailLines <= 0 {
		tailLines = 100
	}

	// Build the query
	query := &application.ApplicationPodLogsQuery{
		Name: &name,
	}

	if namespace != "" {
		query.Namespace = &namespace
	}
	if podName != "" {
		query.PodName = &podName
	}
	if container != "" {
		query.Container = &container
	}
	if kind != "" {
		query.Kind = &kind
	}
	if group != "" {
		query.Group = &group
	}
	if resourceName != "" {
		query.ResourceName = &resourceName
	}

	tailLinesInt64 := int64(tailLines)
	query.TailLines = &tailLinesInt64

	if sinceSeconds > 0 {
		query.SinceSeconds = &sinceSeconds
	}
	if filter != "" {
		query.Filter = &filter
	}

	previousBool := previous
	query.Previous = &previousBool

	// Get logs from the client
	entries, err := tm.client.GetApplicationLogs(ctx, query)
	if err != nil {
		return errorResult(err.Error()), nil
	}

	// Determine truncation status
	truncated := len(entries) >= client.MaxLogEntries

	// Build compact plain text output: "timestamp pod_name | content"
	var sb strings.Builder
	if truncated {
		sb.WriteString(fmt.Sprintf("# %s logs (truncated at %d lines)\n", name, len(entries)))
	} else {
		sb.WriteString(fmt.Sprintf("# %s logs (%d lines)\n", name, len(entries)))
	}
	for _, entry := range entries {
		if entry.Timestamp != "" && entry.PodName != "" {
			sb.WriteString(fmt.Sprintf("%s %s | %s\n", entry.Timestamp, entry.PodName, entry.Content))
		} else if entry.PodName != "" {
			sb.WriteString(fmt.Sprintf("%s | %s\n", entry.PodName, entry.Content))
		} else {
			sb.WriteString(entry.Content)
			sb.WriteByte('\n')
		}
	}

	return TextResult(sb.String())
}

// ResourceTreeNode represents a node in the formatted resource hierarchy
type ResourceTreeNode struct {
	Kind      string              `json:"kind"`
	Name      string              `json:"name"`
	Namespace string              `json:"ns,omitempty"`
	Health    string              `json:"health,omitempty"`
	Status    string              `json:"status,omitempty"`
	Children  []*ResourceTreeNode `json:"children,omitempty"`
}

func (tm *ToolManager) handleGetResourceTree(ctx context.Context, arguments map[string]interface{}) (*mcp.CallToolResult, error) {
	name := String(arguments, "name", "")

	tree, err := tm.client.GetResourceTree(ctx, name)
	if err != nil {
		return errorResult(err.Error()), nil
	}

	// Build a lookup from UID -> node
	type nodeInfo struct {
		node      v1alpha1.ResourceNode
		children  []string // child UIDs
		parentUID string
	}
	nodesByUID := make(map[string]*nodeInfo)
	for i := range tree.Nodes {
		n := tree.Nodes[i]
		nodesByUID[n.UID] = &nodeInfo{node: n}
	}

	// Build parent->child relationships
	roots := make([]string, 0)
	for uid, info := range nodesByUID {
		if len(info.node.ParentRefs) == 0 {
			roots = append(roots, uid)
		} else {
			for _, ref := range info.node.ParentRefs {
				parentUID := ref.UID
				if parent, ok := nodesByUID[parentUID]; ok {
					parent.children = append(parent.children, uid)
					info.parentUID = parentUID
				} else {
					// Parent not in tree, treat as root
					roots = append(roots, uid)
				}
			}
		}
	}

	// Recursively build tree
	var buildTree func(uid string) *ResourceTreeNode
	buildTree = func(uid string) *ResourceTreeNode {
		info, ok := nodesByUID[uid]
		if !ok {
			return nil
		}
		n := info.node
		health := ""
		if n.Health != nil {
			health = string(n.Health.Status)
		}
		treeNode := &ResourceTreeNode{
			Kind:      n.Kind,
			Name:      n.Name,
			Namespace: n.Namespace,
			Health:    health,
		}
		for _, childUID := range info.children {
			if child := buildTree(childUID); child != nil {
				treeNode.Children = append(treeNode.Children, child)
			}
		}
		return treeNode
	}

	rootNodes := make([]*ResourceTreeNode, 0, len(roots))
	for _, uid := range roots {
		if node := buildTree(uid); node != nil {
			rootNodes = append(rootNodes, node)
		}
	}

	// Add orphaned nodes
	orphanedNodes := make([]*ResourceTreeNode, 0, len(tree.OrphanedNodes))
	for _, n := range tree.OrphanedNodes {
		health := ""
		if n.Health != nil {
			health = string(n.Health.Status)
		}
		orphanedNodes = append(orphanedNodes, &ResourceTreeNode{
			Kind:      n.Kind,
			Name:      n.Name,
			Namespace: n.Namespace,
			Health:    health,
		})
	}

	result := map[string]interface{}{
		"application": name,
		"resources":   rootNodes,
	}
	if len(orphanedNodes) > 0 {
		result["orphaned"] = orphanedNodes
	}

	return Result(result, nil)
}
