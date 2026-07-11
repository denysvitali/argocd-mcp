package tools

import (
	"context"
	"fmt"

	"github.com/argoproj/argo-cd/v3/pkg/apiclient/repository"
	"github.com/argoproj/argo-cd/v3/pkg/apis/application/v1alpha1"
	"github.com/mark3labs/mcp-go/mcp"
)

// Repository handlers

func (tm *ToolManager) handleListRepositories(ctx context.Context, arguments map[string]interface{}) (*mcp.CallToolResult, error) {
	repoURL := String(arguments, "repo_url", "")
	limit := Int(arguments, "limit", MaxListItems)
	query := &repository.RepoQuery{}
	if repoURL != "" {
		query.Repo = repoURL
	}

	repos, err := tm.client.ListRepositories(ctx, query)
	if err != nil {
		return errorResult(err.Error()), nil
	}

	// Apply limit
	total := len(repos.Items)
	if len(repos.Items) > limit {
		repos.Items = repos.Items[:limit]
	}

	items := make([]interface{}, len(repos.Items))
	for i, repo := range repos.Items {
		items[i] = map[string]interface{}{
			"repo": repo.Repo,
			"type": repo.Type,
			"name": repo.Name,
		}
	}

	return ResultList(items, total, nil)
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
	if result := tm.checkSafeMode(toolCreateRepository); result != nil {
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
	if result := tm.checkSafeMode(toolUpdateRepository); result != nil {
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
	if result := tm.checkDeleteAllowed(toolDeleteRepository); result != nil {
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
