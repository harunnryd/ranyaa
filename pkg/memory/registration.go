package memory

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/harun/ranya/pkg/toolexecutor"
)

// ToolExecutor interface for registering tools
// This avoids circular dependency with pkg/toolexecutor
type ToolExecutor interface {
	RegisterTool(def toolexecutor.ToolDefinition) error
}

// RegisterMemoryTools registers all memory tools with the tool executor
func RegisterMemoryTools(executor ToolExecutor, manager *Manager, workspacePath string) error {
	tools := []toolexecutor.ToolDefinition{
		{
			Name:        "memory_search",
			Description: "Search memory files by query using hybrid vector and keyword search",
			Parameters: []toolexecutor.ToolParameter{
				{
					Name:        "query",
					Type:        "string",
					Description: "Search query",
					Required:    true,
				},
				{
					Name:        "limit",
					Type:        "integer",
					Description: "Maximum number of results to return",
					Required:    false,
					Default:     20,
				},
				{
					Name:        "vector_weight",
					Type:        "number",
					Description: "Weight for vector similarity (0-1)",
					Required:    false,
					Default:     0.7,
				},
				{
					Name:        "keyword_weight",
					Type:        "number",
					Description: "Weight for keyword matching (0-1)",
					Required:    false,
					Default:     0.3,
				},
				{
					Name:        "min_score",
					Type:        "number",
					Description: "Minimum relevance score threshold",
					Required:    false,
					Default:     0.0,
				},
			},
			Handler: func(ctx context.Context, params map[string]interface{}) (interface{}, error) {
				// Parse parameters
				var searchParams MemorySearchParams
				jsonData, err := json.Marshal(params)
				if err != nil {
					return nil, fmt.Errorf("failed to marshal params: %w", err)
				}
				if err := json.Unmarshal(jsonData, &searchParams); err != nil {
					return nil, fmt.Errorf("failed to unmarshal params: %w", err)
				}

				return MemorySearch(ctx, manager, searchParams)
			},
		},
		{
			Name:        "memory_write",
			Description: "Create or update a memory file",
			Parameters: []toolexecutor.ToolParameter{
				{
					Name:        "path",
					Type:        "string",
					Description: "Relative path to the memory file (must end with .md)",
					Required:    true,
				},
				{
					Name:        "content",
					Type:        "string",
					Description: "Content to write to the file",
					Required:    true,
				},
			},
			Handler: func(ctx context.Context, params map[string]interface{}) (interface{}, error) {
				var writeParams MemoryWriteParams
				jsonData, err := json.Marshal(params)
				if err != nil {
					return nil, fmt.Errorf("failed to marshal params: %w", err)
				}
				if err := json.Unmarshal(jsonData, &writeParams); err != nil {
					return nil, fmt.Errorf("failed to unmarshal params: %w", err)
				}

				return MemoryWrite(ctx, manager, workspacePath, writeParams)
			},
		},
		{
			Name:        "memory_delete",
			Description: "Delete a memory file",
			Parameters: []toolexecutor.ToolParameter{
				{
					Name:        "path",
					Type:        "string",
					Description: "Relative path to the memory file to delete",
					Required:    true,
				},
			},
			Handler: func(ctx context.Context, params map[string]interface{}) (interface{}, error) {
				var deleteParams MemoryDeleteParams
				jsonData, err := json.Marshal(params)
				if err != nil {
					return nil, fmt.Errorf("failed to marshal params: %w", err)
				}
				if err := json.Unmarshal(jsonData, &deleteParams); err != nil {
					return nil, fmt.Errorf("failed to unmarshal params: %w", err)
				}

				return MemoryDelete(ctx, manager, workspacePath, deleteParams)
			},
		},
		{
			Name:        "memory_list",
			Description: "List all memory files with metadata",
			Parameters: []toolexecutor.ToolParameter{
				{
					Name:        "pattern",
					Type:        "string",
					Description: "Optional glob pattern to filter files",
					Required:    false,
				},
			},
			Handler: func(ctx context.Context, params map[string]interface{}) (interface{}, error) {
				var listParams MemoryListParams
				jsonData, err := json.Marshal(params)
				if err != nil {
					return nil, fmt.Errorf("failed to marshal params: %w", err)
				}
				if err := json.Unmarshal(jsonData, &listParams); err != nil {
					return nil, fmt.Errorf("failed to unmarshal params: %w", err)
				}

				return MemoryList(ctx, workspacePath, listParams)
			},
		},
	}

	// Register each tool
	for _, tool := range tools {
		if err := executor.RegisterTool(tool); err != nil {
			return fmt.Errorf("failed to register tool %s: %w", tool.Name, err)
		}
	}

	return nil
}
