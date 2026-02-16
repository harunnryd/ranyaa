package workspace

import (
	"sync"
	"time"
)

// EventHandler is a function that handles workspace events
type EventHandler func(payload interface{})

// WorkspaceEventEmitter broadcasts workspace events to subscribers
type WorkspaceEventEmitter struct {
	mu        sync.RWMutex
	listeners map[WorkspaceEvent][]EventHandler
}

// NewWorkspaceEventEmitter creates a new event emitter
func NewWorkspaceEventEmitter() *WorkspaceEventEmitter {
	return &WorkspaceEventEmitter{
		listeners: make(map[WorkspaceEvent][]EventHandler),
	}
}

// On registers an event handler for a specific event type
func (e *WorkspaceEventEmitter) On(event WorkspaceEvent, handler EventHandler) {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.listeners[event] = append(e.listeners[event], handler)
}

// Emit emits an event with a payload (asynchronously)
func (e *WorkspaceEventEmitter) Emit(event WorkspaceEvent, payload interface{}) {
	e.mu.RLock()
	handlers := e.listeners[event]
	e.mu.RUnlock()

	// Emit asynchronously to avoid blocking
	for _, handler := range handlers {
		go handler(payload)
	}
}

// EmitFileAdded emits a file added event
func (e *WorkspaceEventEmitter) EmitFileAdded(file *WorkspaceFile) {
	payload := FileEventPayload{
		EventPayload: EventPayload{
			Timestamp: time.Now(),
		},
		Path:         file.Path,
		RelativePath: file.RelativePath,
		Type:         file.Type,
	}
	e.Emit(EventFileAdded, payload)
}

// EmitFileChanged emits a file changed event
func (e *WorkspaceEventEmitter) EmitFileChanged(file *WorkspaceFile, hasChanges bool) {
	payload := FileChangedPayload{
		FileEventPayload: FileEventPayload{
			EventPayload: EventPayload{
				Timestamp: time.Now(),
			},
			Path:         file.Path,
			RelativePath: file.RelativePath,
			Type:         file.Type,
		},
		HasChanges: hasChanges,
	}
	e.Emit(EventFileChanged, payload)
}

// EmitFileDeleted emits a file deleted event
func (e *WorkspaceEventEmitter) EmitFileDeleted(path, relativePath string, fileType WorkspaceFileType) {
	payload := FileEventPayload{
		EventPayload: EventPayload{
			Timestamp: time.Now(),
		},
		Path:         path,
		RelativePath: relativePath,
		Type:         fileType,
	}
	e.Emit(EventFileDeleted, payload)
}

// EmitInitialized emits a workspace initialized event
func (e *WorkspaceEventEmitter) EmitInitialized(fileCount int) {
	payload := InitializedPayload{
		EventPayload: EventPayload{
			Timestamp: time.Now(),
		},
		FileCount: fileCount,
	}
	e.Emit(EventInitialized, payload)
}

// EmitError emits an error event
func (e *WorkspaceEventEmitter) EmitError(err error, context map[string]interface{}) {
	payload := ErrorPayload{
		EventPayload: EventPayload{
			Timestamp: time.Now(),
		},
		Error:   err,
		Context: context,
	}
	e.Emit(EventError, payload)
}

// RemoveAllListeners removes all event listeners
func (e *WorkspaceEventEmitter) RemoveAllListeners() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.listeners = make(map[WorkspaceEvent][]EventHandler)
}
