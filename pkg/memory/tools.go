package memory

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"
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
