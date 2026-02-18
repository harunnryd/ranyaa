package workspace

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
)

// WorkspaceManager is the main facade that coordinates all workspace components
type WorkspaceManager struct {
	workspacePath string
	config        WorkspaceConfig

	loader    *WorkspaceLoader
	cache     *WorkspaceCache
	validator *WorkspaceValidator
	watcher   *WorkspaceWatcher
	emitter   *WorkspaceEventEmitter

	mu     sync.RWMutex
	closed bool
}

// NewWorkspaceManager creates a new workspace manager
func NewWorkspaceManager(config WorkspaceConfig) (*WorkspaceManager, error) {
	// Validate workspace path
	if config.WorkspacePath == "" {
		return nil, fmt.Errorf("workspace path is required")
	}

	// Set defaults
	if config.MaxFileSize == 0 {
		config.MaxFileSize = MaxFileSize
	}
	if config.StabilityThreshold == 0 {
		config.StabilityThreshold = 100 * time.Millisecond
	}

	// Create components
	loader := NewWorkspaceLoader(config.WorkspacePath)
	cache := NewWorkspaceCache()
	validator := NewWorkspaceValidator()
	emitter := NewWorkspaceEventEmitter()

	manager := &WorkspaceManager{
		workspacePath: config.WorkspacePath,
		config:        config,
		loader:        loader,
		cache:         cache,
		validator:     validator,
		emitter:       emitter,
	}

	// Create watcher
	watcherConfig := WorkspaceWatcherConfig{
		WorkspacePath:      config.WorkspacePath,
		StabilityThreshold: config.StabilityThreshold,
		OnFileAdded:        manager.handleFileAdded,
		OnFileChanged:      manager.handleFileChanged,
		OnFileDeleted:      manager.handleFileDeleted,
	}

	watcher, err := NewWorkspaceWatcher(watcherConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create watcher: %w", err)
	}
	manager.watcher = watcher

	return manager, nil
}

// Init initializes the workspace manager by discovering and loading all files
func (m *WorkspaceManager) Init() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed {
		return fmt.Errorf("workspace manager is closed")
	}

	log.Info().
		Str("path", m.workspacePath).
		Msg("Initializing workspace manager")

	// Discover and load all files
	fileCount := 0
	err := filepath.WalkDir(m.workspacePath, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			log.Warn().
				Err(err).
				Str("path", path).
				Msg("Error walking path")
			return nil // Continue walking
		}

		// Prune ignored directories early to avoid walking them
		if d.IsDir() {
			base := d.Name()
			if (len(base) > 0 && base[0] == '.') || base == "node_modules" {
				return filepath.SkipDir
			}
			return nil
		}

		// Skip ignored files
		if m.shouldIgnoreFile(path) {
			return nil
		}

		// Load file
		if err := m.loadAndCacheFile(path); err != nil {
			log.Error().
				Err(err).
				Str("path", path).
				Msg("Failed to load file")
			// Continue with other files
			return nil
		}

		fileCount++
		return nil
	})

	if err != nil {
		return fmt.Errorf("failed to walk workspace: %w", err)
	}

	// Start watcher
	if err := m.watcher.Start(); err != nil {
		return fmt.Errorf("failed to start watcher: %w", err)
	}

	// Emit initialized event
	m.emitter.EmitInitialized(fileCount)

	log.Info().
		Int("fileCount", fileCount).
		Msg("Workspace manager initialized")

	return nil
}

// GetFile retrieves a file from the cache by its absolute path
func (m *WorkspaceManager) GetFile(path string) (*WorkspaceFile, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.cache.Get(path)
}

// GetFileByName retrieves a file from the cache by its filename
func (m *WorkspaceManager) GetFileByName(filename string) (*WorkspaceFile, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Search for file with matching basename
	files := m.cache.GetAll(func(file *WorkspaceFile) bool {
		return filepath.Base(file.Path) == filename
	})

	if len(files) > 0 {
		return files[0], true
	}

	return nil, false
}

// GetFileContent retrieves the content of a file by its filename
func (m *WorkspaceManager) GetFileContent(filename string) (string, bool) {
	file, ok := m.GetFileByName(filename)
	if !ok {
		return "", false
	}
	return file.Content, true
}

// GetAllFiles retrieves all files from the cache
func (m *WorkspaceManager) GetAllFiles() []*WorkspaceFile {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.cache.GetAll(nil)
}

// GetFilesByType retrieves all files of a specific type
func (m *WorkspaceManager) GetFilesByType(fileType WorkspaceFileType) []*WorkspaceFile {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.cache.GetByType(fileType)
}

// On registers an event handler
func (m *WorkspaceManager) On(event WorkspaceEvent, handler EventHandler) {
	m.emitter.On(event, handler)
}

// Close stops the workspace manager and releases resources
func (m *WorkspaceManager) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed {
		return nil
	}

	log.Info().Msg("Closing workspace manager")

	// Stop watcher
	if err := m.watcher.Stop(); err != nil {
		log.Error().Err(err).Msg("Error stopping watcher")
	}

	// Clear cache
	m.cache.Clear()

	// Remove all listeners
	m.emitter.RemoveAllListeners()

	m.closed = true

	log.Info().Msg("Workspace manager closed")
	return nil
}

// loadAndCacheFile loads a file and stores it in the cache
func (m *WorkspaceManager) loadAndCacheFile(path string) error {
	// Load file
	loaded, err := m.loader.LoadFile(path)
	if err != nil {
		return err
	}

	// Resolve config files (env vars and file references)
	parsedContent := loaded.ParsedContent
	if loaded.Type == FileTypeConfig && parsedContent != nil {
		resolved, err := ResolveConfig(parsedContent, m.loader)
		if err != nil {
			log.Error().
				Err(err).
				Str("path", path).
				Msg("Failed to resolve config")
			// Continue with unresolved content
		} else {
			parsedContent = resolved
		}
	}

	// Validate file if enabled
	if m.config.EnableValidation {
		result := m.validator.Validate(loaded)
		if !result.Valid {
			log.Warn().
				Str("path", path).
				Interface("errors", result.Errors).
				Msg("File validation failed")
		}
		if len(result.Warnings) > 0 {
			log.Warn().
				Str("path", path).
				Interface("warnings", result.Warnings).
				Msg("File validation warnings")
		}
	}

	// Get relative path
	relPath, err := filepath.Rel(m.workspacePath, path)
	if err != nil {
		relPath = filepath.Base(path)
	}

	// Create workspace file
	file := &WorkspaceFile{
		Path:          path,
		RelativePath:  relPath,
		Content:       loaded.Content,
		ParsedContent: parsedContent,
		Hash:          loaded.Hash,
		LoadedAt:      time.Now(),
		Type:          loaded.Type,
		Size:          loaded.Size,
		IsCritical:    m.isCriticalFile(loaded.Type),
	}

	// Store in cache
	m.cache.Set(path, file)

	return nil
}

// handleFileAdded handles a file added event from the watcher
func (m *WorkspaceManager) handleFileAdded(path string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed {
		return nil
	}

	// Skip ignored files
	if m.shouldIgnoreFile(path) {
		return nil
	}

	log.Info().
		Str("path", path).
		Msg("File added")

	// Load and cache file
	if err := m.loadAndCacheFile(path); err != nil {
		m.emitter.EmitError(err, map[string]interface{}{
			"operation": "file_added",
			"path":      path,
		})
		return err
	}

	// Get file from cache
	file, ok := m.cache.Get(path)
	if !ok {
		return fmt.Errorf("file not found in cache after loading")
	}

	// Emit event
	m.emitter.EmitFileAdded(file)

	// Call reload callback if critical file
	if file.IsCritical && m.config.OnReload != nil {
		m.config.OnReload(file)
	}

	return nil
}

// handleFileChanged handles a file changed event from the watcher
func (m *WorkspaceManager) handleFileChanged(path string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed {
		return nil
	}

	// Skip ignored files
	if m.shouldIgnoreFile(path) {
		return nil
	}

	log.Info().
		Str("path", path).
		Msg("File changed")

	// Get old file from cache
	oldFile, hadFile := m.cache.Get(path)

	// Load new file
	loaded, err := m.loader.LoadFile(path)
	if err != nil {
		m.emitter.EmitError(err, map[string]interface{}{
			"operation": "file_changed",
			"path":      path,
		})
		return err
	}

	// Check if content actually changed
	hasChanges := !hadFile || oldFile.Hash != loaded.Hash

	if !hasChanges {
		log.Debug().
			Str("path", path).
			Msg("File touched but content unchanged")
		return nil
	}

	// Resolve config files (env vars and file references)
	parsedContent := loaded.ParsedContent
	if loaded.Type == FileTypeConfig && parsedContent != nil {
		resolved, err := ResolveConfig(parsedContent, m.loader)
		if err != nil {
			log.Error().
				Err(err).
				Str("path", path).
				Msg("Failed to resolve config")
			// Continue with unresolved content
		} else {
			parsedContent = resolved
		}
	}

	// Validate file if enabled
	if m.config.EnableValidation {
		result := m.validator.Validate(loaded)
		if !result.Valid {
			log.Warn().
				Str("path", path).
				Interface("errors", result.Errors).
				Msg("File validation failed")
		}
	}

	// Get relative path
	relPath, err := filepath.Rel(m.workspacePath, path)
	if err != nil {
		relPath = filepath.Base(path)
	}

	// Create updated workspace file
	file := &WorkspaceFile{
		Path:          path,
		RelativePath:  relPath,
		Content:       loaded.Content,
		ParsedContent: parsedContent,
		Hash:          loaded.Hash,
		LoadedAt:      time.Now(),
		Type:          loaded.Type,
		Size:          loaded.Size,
		IsCritical:    m.isCriticalFile(loaded.Type),
	}

	// Store previous hash
	if hadFile {
		file.PreviousHash = oldFile.Hash
	}

	// Update cache
	m.cache.Set(path, file)

	// Emit event
	m.emitter.EmitFileChanged(file, hasChanges)

	// Call reload callback if critical file
	if file.IsCritical && m.config.OnReload != nil {
		m.config.OnReload(file)
	}

	return nil
}

// handleFileDeleted handles a file deleted event from the watcher
func (m *WorkspaceManager) handleFileDeleted(path string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed {
		return nil
	}

	log.Info().
		Str("path", path).
		Msg("File deleted")

	// Get file from cache before deleting
	file, ok := m.cache.Get(path)
	if !ok {
		// File wasn't in cache, nothing to do
		return nil
	}

	// Remove from cache
	m.cache.Delete(path)

	// Emit event
	m.emitter.EmitFileDeleted(file.Path, file.RelativePath, file.Type)

	// Log warning if critical file
	if file.IsCritical {
		log.Warn().
			Str("path", path).
			Str("type", string(file.Type)).
			Msg("Critical file deleted")
	}

	return nil
}

// shouldIgnoreFile checks if a file should be ignored
func (m *WorkspaceManager) shouldIgnoreFile(path string) bool {
	base := filepath.Base(path)

	// Ignore ALL dotfiles and dot-directories
	// This is a simple, predictable rule
	if len(base) > 0 && base[0] == '.' {
		return true
	}

	// Ignore node_modules
	if strings.Contains(path, "node_modules") {
		return true
	}

	// Ignore .git directory
	if strings.Contains(path, ".git") {
		return true
	}

	// Ignore .env files
	if strings.HasSuffix(base, ".env") || strings.Contains(base, ".env.") {
		return true
	}

	return false
}

// isCriticalFile determines if a file type is critical
func (m *WorkspaceManager) isCriticalFile(fileType WorkspaceFileType) bool {
	switch fileType {
	case FileTypeAgents, FileTypeSoul, FileTypeTools, FileTypeWebhooks,
		FileTypeCron, FileTypePlugins, FileTypeConfig:
		return true
	default:
		return false
	}
}
