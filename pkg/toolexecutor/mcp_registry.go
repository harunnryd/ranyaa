package toolexecutor

import (
	"context"
	"fmt"
	"strings"
)

// RegisterMCPServer registers tools and resource helpers from an MCP server adapter.
func (te *ToolExecutor) RegisterMCPServer(ctx context.Context, serverID string, adapter *MCPServerAdapter) ([]string, error) {
	if strings.TrimSpace(serverID) == "" {
		return nil, fmt.Errorf("mcp server id is required")
	}
	if adapter == nil {
		return nil, fmt.Errorf("mcp adapter is required")
	}

	tools, err := adapter.GetTools(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch MCP tools: %w", err)
	}

	registered := make([]string, 0, len(tools)+2)
	for _, tool := range tools {
		originalName := tool.Name
		if originalName == "" {
			continue
		}

		toolName := originalName
		if te.GetTool(toolName) != nil {
			toolName = fmt.Sprintf("%s_%s", serverID, originalName)
		}
		tool.Name = toolName
		tool.Handler = func(ctx context.Context, params map[string]interface{}) (interface{}, error) {
			return adapter.ExecuteTool(ctx, originalName, params)
		}

		if err := te.RegisterTool(tool); err != nil {
			return registered, fmt.Errorf("failed to register MCP tool %s: %w", toolName, err)
		}
		registered = append(registered, toolName)
	}

	resourcesListName := fmt.Sprintf("mcp_%s_resources_list", serverID)
	if te.GetTool(resourcesListName) != nil {
		resourcesListName = fmt.Sprintf("%s_%s", serverID, resourcesListName)
	}

	listTool := ToolDefinition{
		Name:        resourcesListName,
		Description: "List resources exposed by MCP server",
		Category:    CategoryRead,
		Handler: func(ctx context.Context, _ map[string]interface{}) (interface{}, error) {
			return adapter.ListResources(ctx)
		},
	}
	if err := te.RegisterTool(listTool); err != nil {
		return registered, fmt.Errorf("failed to register MCP resources list tool: %w", err)
	}
	registered = append(registered, resourcesListName)

	resourcesReadName := fmt.Sprintf("mcp_%s_resource_read", serverID)
	if te.GetTool(resourcesReadName) != nil {
		resourcesReadName = fmt.Sprintf("%s_%s", serverID, resourcesReadName)
	}

	readTool := ToolDefinition{
		Name:        resourcesReadName,
		Description: "Read a resource exposed by MCP server",
		Category:    CategoryRead,
		Parameters: []ToolParameter{{
			Name:        "uri",
			Type:        "string",
			Description: "Resource URI",
			Required:    true,
		}},
		Handler: func(ctx context.Context, params map[string]interface{}) (interface{}, error) {
			uri, _ := params["uri"].(string)
			if strings.TrimSpace(uri) == "" {
				return nil, fmt.Errorf("uri parameter is required")
			}
			return adapter.ReadResource(ctx, uri)
		},
	}
	if err := te.RegisterTool(readTool); err != nil {
		return registered, fmt.Errorf("failed to register MCP resource read tool: %w", err)
	}
	registered = append(registered, resourcesReadName)

	return registered, nil
}
