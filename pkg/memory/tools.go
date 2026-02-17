package memory

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/harun/ranya/pkg/toolexecutor"
)

// MemorySearchParams defines parameters for memory_search tool
type MemorySearchParams struct {
	Query         string  `json:"query"`
	Limit         int     `json:"limit,omitempty"`
	VectorWeight  float64 `json:"vector_weight,omitempty"`
	KeywordWeight float64 `json:"keyword_weight,omitempty"`
	MinScore      float64 `json:"min_score,omitempty"`
}

// MemorySearchResult represents the result of a memory search
type MemorySearchResult struct {
	Results []SearchResult `json:"results"`
	Query   string         `json:"query"`
	Count   int            `json:"count"`
}

// MemorySearch searches memory files by query
func MemorySearch(ctx context.Context, manager *Manager, params MemorySearchParams) (*MemorySearchResult, error) {
	if params.Query == "" {
		return nil, fmt.Errorf("query is required")
	}

	// Set defaults
	if params.Limit == 0 {
		params.Limit = 20
	}
	if params.VectorWeight == 0 {
		params.VectorWeight = 0.7
	}
	if params.KeywordWeight == 0 {
		params.KeywordWeight = 0.3
	}

	opts := &SearchOptions{
		Limit:         params.Limit,
		VectorWeight:  params.VectorWeight,
		KeywordWeight: params.KeywordWeight,
		MinScore:      params.MinScore,
	}

	results, err := manager.Search(params.Query, opts)
	if err != nil {
		return nil, fmt.Errorf("search failed: %w", err)
	}

	return &MemorySearchResult{
		Results: results,
		Query:   params.Query,
		Count:   len(results),
	}, nil
}

// MemoryWriteParams defines parameters for memory_write tool
type MemoryWriteParams struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

// MemoryWriteResult represents the result of a memory write
type MemoryWriteResult struct {
	Path         string `json:"path"`
	BytesWritten int    `json:"bytes_written"`
	Created      bool   `json:"created"`
}

// MemoryWrite creates or updates a memory file
func MemoryWrite(ctx context.Context, manager *Manager, workspacePath string, params MemoryWriteParams) (*MemoryWriteResult, error) {
	if params.Path == "" {
		return nil, fmt.Errorf("path is required")
	}

	// Validate path - must be relative and end with .md
	if filepath.IsAbs(params.Path) {
		return nil, fmt.Errorf("path must be relative")
	}
	if filepath.Ext(params.Path) != ".md" {
		return nil, fmt.Errorf("path must end with .md")
	}

	// Construct full path
	fullPath := filepath.Join(workspacePath, params.Path)

	// Check if file exists
	_, err := os.Stat(fullPath)
	created := os.IsNotExist(err)

	// Create directory if needed
	dir := filepath.Dir(fullPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create directory: %w", err)
	}

	// Write file
	if err := os.WriteFile(fullPath, []byte(params.Content), 0644); err != nil {
		return nil, fmt.Errorf("failed to write file: %w", err)
	}

	// Mark index as dirty (file watcher will trigger this, but we do it explicitly too)
	manager.MarkDirty()

	return &MemoryWriteResult{
		Path:         params.Path,
		BytesWritten: len(params.Content),
		Created:      created,
	}, nil
}

// MemoryDeleteParams defines parameters for memory_delete tool
type MemoryDeleteParams struct {
	Path string `json:"path"`
}

// MemoryDeleteResult represents the result of a memory delete
type MemoryDeleteResult struct {
	Path    string `json:"path"`
	Deleted bool   `json:"deleted"`
}

// MemoryDelete deletes a memory file
func MemoryDelete(ctx context.Context, manager *Manager, workspacePath string, params MemoryDeleteParams) (*MemoryDeleteResult, error) {
	if params.Path == "" {
		return nil, fmt.Errorf("path is required")
	}

	// Validate path - must be relative
	if filepath.IsAbs(params.Path) {
		return nil, fmt.Errorf("path must be relative")
	}

	// Construct full path
	fullPath := filepath.Join(workspacePath, params.Path)

	// Check if file exists
	_, err := os.Stat(fullPath)
	if os.IsNotExist(err) {
		return &MemoryDeleteResult{
			Path:    params.Path,
			Deleted: false,
		}, nil
	}

	// Delete file
	if err := os.Remove(fullPath); err != nil {
		return nil, fmt.Errorf("failed to delete file: %w", err)
	}

	// Mark index as dirty
	manager.MarkDirty()

	return &MemoryDeleteResult{
		Path:    params.Path,
		Deleted: true,
	}, nil
}

// MemoryListParams defines parameters for memory_list tool
type MemoryListParams struct {
	Pattern string `json:"pattern,omitempty"` // Optional glob pattern
}

// MemoryFileInfo represents information about a memory file
type MemoryFileInfo struct {
	Path         string    `json:"path"`
	SizeBytes    int64     `json:"size_bytes"`
	ModifiedTime time.Time `json:"modified_time"`
}

// MemoryListResult represents the result of a memory list
type MemoryListResult struct {
	Files []MemoryFileInfo `json:"files"`
	Count int              `json:"count"`
}

// MemoryList lists all memory files
func MemoryList(ctx context.Context, workspacePath string, params MemoryListParams) (*MemoryListResult, error) {
	var files []MemoryFileInfo

	// Walk the workspace directory
	err := filepath.Walk(workspacePath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip directories
		if info.IsDir() {
			return nil
		}

		// Only include .md files
		if filepath.Ext(path) != ".md" {
			return nil
		}

		// Get relative path
		relPath, err := filepath.Rel(workspacePath, path)
		if err != nil {
			return err
		}

		// Apply pattern filter if provided
		if params.Pattern != "" {
			matched, err := filepath.Match(params.Pattern, relPath)
			if err != nil {
				return fmt.Errorf("invalid pattern: %w", err)
			}
			if !matched {
				return nil
			}
		}

		files = append(files, MemoryFileInfo{
			Path:         relPath,
			SizeBytes:    info.Size(),
			ModifiedTime: info.ModTime(),
		})

		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to list files: %w", err)
	}

	return &MemoryListResult{
		Files: files,
		Count: len(files),
	}, nil
}

// RegisterTools registers memory tools with the tool executor
func (m *Manager) RegisterTools(executor ToolExecutor) error {
	// Register memory_search tool
	if err := executor.RegisterTool(toolexecutor.ToolDefinition{
		Name:        "memory_search",
		Description: "Search memory files by query using semantic and keyword search",
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
				Description: "Maximum number of results (default: 20)",
				Required:    false,
				Default:     20,
			},
			{
				Name:        "vector_weight",
				Type:        "number",
				Description: "Weight for vector search (default: 0.7)",
				Required:    false,
				Default:     0.7,
			},
			{
				Name:        "keyword_weight",
				Type:        "number",
				Description: "Weight for keyword search (default: 0.3)",
				Required:    false,
				Default:     0.3,
			},
			{
				Name:        "min_score",
				Type:        "number",
				Description: "Minimum relevance score (default: 0)",
				Required:    false,
				Default:     0.0,
			},
		},
		Handler: func(ctx context.Context, params map[string]interface{}) (interface{}, error) {
			// Parse parameters
			var searchParams MemorySearchParams
			if query, ok := params["query"].(string); ok {
				searchParams.Query = query
			}
			if limit, ok := params["limit"].(float64); ok {
				searchParams.Limit = int(limit)
			}
			if vw, ok := params["vector_weight"].(float64); ok {
				searchParams.VectorWeight = vw
			}
			if kw, ok := params["keyword_weight"].(float64); ok {
				searchParams.KeywordWeight = kw
			}
			if ms, ok := params["min_score"].(float64); ok {
				searchParams.MinScore = ms
			}
			return MemorySearch(ctx, m, searchParams)
		},
	}); err != nil {
		return fmt.Errorf("failed to register memory_search tool: %w", err)
	}

	// Register memory_write tool
	if err := executor.RegisterTool(toolexecutor.ToolDefinition{
		Name:        "memory_write",
		Description: "Create or update a memory file",
		Parameters: []toolexecutor.ToolParameter{
			{
				Name:        "path",
				Type:        "string",
				Description: "Relative path to the file (must end with .md)",
				Required:    true,
			},
			{
				Name:        "content",
				Type:        "string",
				Description: "File content",
				Required:    true,
			},
		},
		Handler: func(ctx context.Context, params map[string]interface{}) (interface{}, error) {
			var writeParams MemoryWriteParams
			if path, ok := params["path"].(string); ok {
				writeParams.Path = path
			}
			if content, ok := params["content"].(string); ok {
				writeParams.Content = content
			}
			return MemoryWrite(ctx, m, m.workspacePath, writeParams)
		},
	}); err != nil {
		return fmt.Errorf("failed to register memory_write tool: %w", err)
	}

	// Register memory_delete tool
	if err := executor.RegisterTool(toolexecutor.ToolDefinition{
		Name:        "memory_delete",
		Description: "Delete a memory file",
		Parameters: []toolexecutor.ToolParameter{
			{
				Name:        "path",
				Type:        "string",
				Description: "Relative path to the file",
				Required:    true,
			},
		},
		Handler: func(ctx context.Context, params map[string]interface{}) (interface{}, error) {
			var deleteParams MemoryDeleteParams
			if path, ok := params["path"].(string); ok {
				deleteParams.Path = path
			}
			return MemoryDelete(ctx, m, m.workspacePath, deleteParams)
		},
	}); err != nil {
		return fmt.Errorf("failed to register memory_delete tool: %w", err)
	}

	// Register memory_list tool
	if err := executor.RegisterTool(toolexecutor.ToolDefinition{
		Name:        "memory_list",
		Description: "List all memory files",
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
			if pattern, ok := params["pattern"].(string); ok {
				listParams.Pattern = pattern
			}
			return MemoryList(ctx, m.workspacePath, listParams)
		},
	}); err != nil {
		return fmt.Errorf("failed to register memory_list tool: %w", err)
	}

	m.logger.Info().Msg("Memory tools registered")
	return nil
}
