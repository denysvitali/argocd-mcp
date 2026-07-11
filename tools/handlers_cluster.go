package tools

import (
	"context"
	"fmt"

	"github.com/argoproj/argo-cd/v3/pkg/apiclient/cluster"
	"github.com/argoproj/argo-cd/v3/pkg/apis/application/v1alpha1"
	"github.com/mark3labs/mcp-go/mcp"
)

// Cluster handlers

func (tm *ToolManager) handleListClusters(ctx context.Context, arguments map[string]interface{}) (*mcp.CallToolResult, error) {
	server := String(arguments, "server", "")
	limit := Int(arguments, "limit", MaxListItems)
	query := &cluster.ClusterQuery{}
	if server != "" {
		query.Server = server
	}

	clusters, err := tm.client.ListClusters(ctx, query)
	if err != nil {
		return errorResult(err.Error()), nil
	}

	// Apply limit
	total := len(clusters.Items)
	if len(clusters.Items) > limit {
		clusters.Items = clusters.Items[:limit]
	}

	items := make([]interface{}, len(clusters.Items))
	for i, c := range clusters.Items {
		items[i] = map[string]interface{}{
			"server": c.Server,
			"name":   c.Name,
		}
	}

	return ResultList(items, total, nil)
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
	if result := tm.checkSafeMode(toolCreateCluster); result != nil {
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
	if result := tm.checkSafeMode(toolUpdateCluster); result != nil {
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
	if result := tm.checkDeleteAllowed(toolDeleteCluster); result != nil {
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
