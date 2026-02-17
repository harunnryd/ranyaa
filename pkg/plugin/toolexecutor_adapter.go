package plugin

// ToolExecutorAdapter adapts PluginRuntime to work with ToolExecutor
// This implements the PluginToolExecutor interface expected by toolexecutor package
type ToolExecutorAdapter struct {
	runtime *PluginRuntime
}

// NewToolExecutorAdapter creates a new adapter
func NewToolExecutorAdapter(runtime *PluginRuntime) *ToolExecutorAdapter {
	return &ToolExecutorAdapter{
		runtime: runtime,
	}
}

// GetPlugin retrieves a plugin that can execute tools
// Returns the pluginToolAdapter which implements toolexecutor.PluginToolProvider
func (a *ToolExecutorAdapter) GetPlugin(pluginID string) (interface{}, error) {
	return a.runtime.GetPluginForTool(pluginID)
}

// GetAllPluginTools returns all tools from all loaded plugins
func (a *ToolExecutorAdapter) GetAllPluginTools() []PluginToolInfo {
	plugins := a.runtime.ListPlugins()
	var allTools []PluginToolInfo

	for _, plugin := range plugins {
		if plugin.State != StateEnabled {
			continue
		}

		for _, tool := range plugin.Tools {
			allTools = append(allTools, PluginToolInfo{
				PluginID:   plugin.ID,
				PluginName: plugin.Manifest.Name,
				Tool:       tool,
			})
		}
	}

	return allTools
}

// PluginToolInfo contains information about a plugin tool
type PluginToolInfo struct {
	PluginID   string
	PluginName string
	Tool       ToolDefinition
}
