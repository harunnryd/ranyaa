package workspace

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/rs/zerolog/log"
)

// FileEventType represents the type of file system event
type FileEventType string

const (
	FileEventAdd    FileEventType = "add"
	FileEventChange FileEventType = "change"
	FileEventDelete FileEventType = "delete"
)

// FileEventCallback is called when a file event occurs
type FileEventCallback func(path string) error

// WorkspaceWatcher monitors the workspace directory for file changes
type WorkspaceWatcher struct {
	watcher            *fsnotify.Watcher
	workspacePath      string
	stabilityThreshold time.Duration
	onFileAdded        FileEventCallback
	onFileChanged      FileEventCallback
	onFileDeleted      FileEventCallback
	done               chan struct{}
	debounceTimers     map[string]*time.Timer
	debounceMu         sync.Mutex
	stopOnce           sync.Once
}

// WorkspaceWatcherConfig holds configuration for the watcher
type WorkspaceWatcherConfig struct {
	WorkspacePath      string
	StabilityThreshold time.Duration
	OnFileAdded        FileEventCallback
	OnFileChanged      FileEventCallback
	OnFileDeleted      FileEventCallback
}

// NewWorkspaceWatcher creates a new workspace watcher
func NewWorkspaceWatcher(config WorkspaceWatcherConfig) (*WorkspaceWatcher, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("failed to create watcher: %w", err)
	}

	if config.StabilityThreshold == 0 {
		config.StabilityThreshold = 100 * time.Millisecond
	}

	return &WorkspaceWatcher{
		watcher:            watcher,
		workspacePath:      config.WorkspacePath,
		stabilityThreshold: config.StabilityThreshold,
		onFileAdded:        config.OnFileAdded,
		onFileChanged:      config.OnFileChanged,
		onFileDeleted:      config.OnFileDeleted,
		done:               make(chan struct{}),
		debounceTimers:     make(map[string]*time.Timer),
	}, nil
}

// Start starts watching the workspace directory
func (w *WorkspaceWatcher) Start() error {
	// Add workspace directory to watcher
	if err := w.addDirectoryRecursive(w.workspacePath); err != nil {
		return fmt.Errorf("failed to watch workspace: %w", err)
	}

	// Start event loop
	go w.eventLoop()

	log.Info().
		Str("path", w.workspacePath).
		Msg("Workspace watcher started")

	return nil
}

// Stop stops the watcher
func (w *WorkspaceWatcher) Stop() error {
	w.stopOnce.Do(func() {
		close(w.done)
	})

	// Cancel all pending debounce timers
	w.debounceMu.Lock()
	for _, timer := range w.debounceTimers {
		timer.Stop()
	}
	clear(w.debounceTimers)
	w.debounceMu.Unlock()

	if err := w.watcher.Close(); err != nil {
		return fmt.Errorf("failed to close watcher: %w", err)
	}

	log.Info().Msg("Workspace watcher stopped")
	return nil
}

// eventLoop processes file system events
func (w *WorkspaceWatcher) eventLoop() {
	for {
		select {
		case event, ok := <-w.watcher.Events:
			if !ok {
				return
			}
			w.handleEvent(event)

		case err, ok := <-w.watcher.Errors:
			if !ok {
				return
			}
			log.Error().Err(err).Msg("Watcher error")

		case <-w.done:
			return
		}
	}
}

// handleEvent handles a file system event
func (w *WorkspaceWatcher) handleEvent(event fsnotify.Event) {
	// Ignore if file should be filtered
	if w.shouldIgnore(event.Name) {
		return
	}

	// Debounce rapid changes to the same file
	w.debounceEvent(event)
}

// debounceEvent debounces file events using a timer
func (w *WorkspaceWatcher) debounceEvent(event fsnotify.Event) {
	w.debounceMu.Lock()
	defer w.debounceMu.Unlock()

	// Cancel existing timer for this file
	if timer, exists := w.debounceTimers[event.Name]; exists {
		timer.Stop()
	}

	eventCopy := event

	// Create new timer
	w.debounceTimers[event.Name] = time.AfterFunc(w.stabilityThreshold, func() {
		w.debounceMu.Lock()
		delete(w.debounceTimers, eventCopy.Name)
		w.debounceMu.Unlock()

		select {
		case <-w.done:
			return
		default:
			w.processEvent(eventCopy)
		}
	})
}

// processEvent processes a debounced file system event
func (w *WorkspaceWatcher) processEvent(event fsnotify.Event) {
	switch {
	case event.Op&fsnotify.Create == fsnotify.Create:
		w.handleFileAdded(event.Name)

	case event.Op&fsnotify.Write == fsnotify.Write:
		w.handleFileChanged(event.Name)

	case event.Op&fsnotify.Remove == fsnotify.Remove:
		w.handleFileDeleted(event.Name)

	case event.Op&fsnotify.Rename == fsnotify.Rename:
		// Treat rename as delete (the new name will trigger a create event)
		w.handleFileDeleted(event.Name)
	}
}

// handleFileAdded handles a file added event
func (w *WorkspaceWatcher) handleFileAdded(path string) {
	if w.onFileAdded != nil {
		if err := w.onFileAdded(path); err != nil {
			log.Error().
				Err(err).
				Str("path", path).
				Msg("Error handling file added")
		}
	}

	// If it's a directory, watch it recursively
	if info, err := filepath.Glob(path); err == nil && len(info) > 0 {
		_ = w.addDirectoryRecursive(path)
	}
}

// handleFileChanged handles a file changed event
func (w *WorkspaceWatcher) handleFileChanged(path string) {
	if w.onFileChanged != nil {
		if err := w.onFileChanged(path); err != nil {
			log.Error().
				Err(err).
				Str("path", path).
				Msg("Error handling file changed")
		}
	}
}

// handleFileDeleted handles a file deleted event
func (w *WorkspaceWatcher) handleFileDeleted(path string) {
	if w.onFileDeleted != nil {
		if err := w.onFileDeleted(path); err != nil {
			log.Error().
				Err(err).
				Str("path", path).
				Msg("Error handling file deleted")
		}
	}
}

// addDirectoryRecursive adds a directory and all its subdirectories to the watcher
func (w *WorkspaceWatcher) addDirectoryRecursive(path string) error {
	return filepath.Walk(path, func(walkPath string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip if should be ignored
		if w.shouldIgnore(walkPath) {
			if info != nil && info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		// Add directory to watcher
		if err := w.watcher.Add(walkPath); err != nil {
			log.Warn().
				Err(err).
				Str("path", walkPath).
				Msg("Failed to watch path")
		}

		return nil
	})
}

// shouldIgnore checks if a file should be ignored
func (w *WorkspaceWatcher) shouldIgnore(path string) bool {
	// Check if any component of the path is a dotfile/directory
	pathParts := strings.Split(filepath.Clean(path), string(filepath.Separator))
	for _, part := range pathParts {
		if len(part) > 0 && part[0] == '.' {
			return true
		}
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
	base := filepath.Base(path)
	if strings.HasSuffix(base, ".env") || strings.Contains(base, ".env.") {
		return true
	}

	return false
}
