package workspace

import "sync"

// WorkspaceCache provides in-memory caching of loaded workspace files
type WorkspaceCache struct {
	mu    sync.RWMutex
	files map[string]*WorkspaceFile
}

// NewWorkspaceCache creates a new workspace cache
func NewWorkspaceCache() *WorkspaceCache {
	return &WorkspaceCache{
		files: make(map[string]*WorkspaceFile),
	}
}

// Set stores a workspace file in the cache
func (c *WorkspaceCache) Set(path string, file *WorkspaceFile) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.files[path] = file
}

// Get retrieves a workspace file from the cache
func (c *WorkspaceCache) Get(path string) (*WorkspaceFile, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	file, ok := c.files[path]
	return file, ok
}

// Has checks if a file exists in the cache
func (c *WorkspaceCache) Has(path string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	_, ok := c.files[path]
	return ok
}

// Delete removes a file from the cache
func (c *WorkspaceCache) Delete(path string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	_, ok := c.files[path]
	if ok {
		delete(c.files, path)
	}
	return ok
}

// GetAll retrieves all files from the cache, optionally filtered by a predicate
func (c *WorkspaceCache) GetAll(filter func(*WorkspaceFile) bool) []*WorkspaceFile {
	c.mu.RLock()
	defer c.mu.RUnlock()

	result := make([]*WorkspaceFile, 0, len(c.files))
	for _, file := range c.files {
		if filter == nil || filter(file) {
			result = append(result, file)
		}
	}
	return result
}

// GetByType retrieves all files of a specific type
func (c *WorkspaceCache) GetByType(fileType WorkspaceFileType) []*WorkspaceFile {
	return c.GetAll(func(file *WorkspaceFile) bool {
		return file.Type == fileType
	})
}

// Clear removes all files from the cache
func (c *WorkspaceCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.files = make(map[string]*WorkspaceFile)
}

// Size returns the number of files in the cache
func (c *WorkspaceCache) Size() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.files)
}
