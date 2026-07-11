package tools

import (
	"context"
	"fmt"

	"github.com/argoproj/argo-cd/v3/pkg/apiclient/project"
	"github.com/argoproj/argo-cd/v3/pkg/apis/application/v1alpha1"
	"github.com/mark3labs/mcp-go/mcp"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Project handlers

func (tm *ToolManager) handleListProjects(ctx context.Context, arguments map[string]interface{}) (*mcp.CallToolResult, error) {
	name := String(arguments, "name", "")
	limit := Int(arguments, "limit", MaxListItems)
	query := &project.ProjectQuery{}
	if name != "" {
		query.Name = name
	}

	projects, err := tm.client.ListProjects(ctx, query)
	if err != nil {
		return errorResult(err.Error()), nil
	}

	// Apply limit
	total := len(projects.Items)
	if len(projects.Items) > limit {
		projects.Items = projects.Items[:limit]
	}

	items := make([]interface{}, len(projects.Items))
	for i, proj := range projects.Items {
		items[i] = map[string]interface{}{
			"name":        proj.Name,
			"description": proj.Spec.Description,
		}
	}

	return ResultList(items, total, nil)
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
	if result := tm.checkSafeMode(toolCreateProject); result != nil {
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
	if result := tm.checkSafeMode(toolUpdateProject); result != nil {
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
	if result := tm.checkDeleteAllowed(toolDeleteProject); result != nil {
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
