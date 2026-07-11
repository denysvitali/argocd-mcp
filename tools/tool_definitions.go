package tools

// defineTools assembles the MCP tool definitions from all domains.
func (tm *ToolManager) defineTools() {
	tm.tools = nil
	tm.tools = append(tm.tools, applicationToolDefinitions()...)
	tm.tools = append(tm.tools, projectToolDefinitions()...)
	tm.tools = append(tm.tools, repositoryToolDefinitions()...)
	tm.tools = append(tm.tools, clusterToolDefinitions()...)
	tm.tools = append(tm.tools, diagnosticsToolDefinitions()...)
	tm.tools = append(tm.tools, operationsToolDefinitions()...)
	tm.tools = append(tm.tools, applicationSetToolDefinitions()...)
}
