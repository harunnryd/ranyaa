package workspace

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestWorkspaceEventEmitter_On(t *testing.T) {
	emitter := NewWorkspaceEventEmitter()

	var called atomic.Bool
	done := make(chan struct{}, 1)
	emitter.On(EventFileAdded, func(payload interface{}) {
		called.Store(true)
		select {
		case done <- struct{}{}:
		default:
		}
	})

	emitter.Emit(EventFileAdded, nil)

	select {
	case <-done:
	case <-time.After(1 * time.Second):
		t.Fatal("timed out waiting for event handler")
	}
	assert.True(t, called.Load())
}

func TestWorkspaceEventEmitter_EmitFileAdded(t *testing.T) {
	emitter := NewWorkspaceEventEmitter()

	var receivedPayload FileEventPayload
	var wg sync.WaitGroup
	wg.Add(1)

	emitter.On(EventFileAdded, func(payload interface{}) {
		defer wg.Done()
		receivedPayload = payload.(FileEventPayload)
	})

	file := &WorkspaceFile{
		Path:         "/workspace/AGENTS.md",
		RelativePath: "AGENTS.md",
		Type:         FileTypeAgents,
	}

	emitter.EmitFileAdded(file)
	wg.Wait()

	assert.Equal(t, file.Path, receivedPayload.Path)
	assert.Equal(t, file.RelativePath, receivedPayload.RelativePath)
	assert.Equal(t, file.Type, receivedPayload.Type)
	assert.False(t, receivedPayload.Timestamp.IsZero())
}

func TestWorkspaceEventEmitter_EmitFileChanged(t *testing.T) {
	emitter := NewWorkspaceEventEmitter()

	var receivedPayload FileChangedPayload
	var wg sync.WaitGroup
	wg.Add(1)

	emitter.On(EventFileChanged, func(payload interface{}) {
		defer wg.Done()
		receivedPayload = payload.(FileChangedPayload)
	})

	file := &WorkspaceFile{
		Path:         "/workspace/AGENTS.md",
		RelativePath: "AGENTS.md",
		Type:         FileTypeAgents,
	}

	emitter.EmitFileChanged(file, true)
	wg.Wait()

	assert.Equal(t, file.Path, receivedPayload.Path)
	assert.Equal(t, file.RelativePath, receivedPayload.RelativePath)
	assert.Equal(t, file.Type, receivedPayload.Type)
	assert.True(t, receivedPayload.HasChanges)
	assert.False(t, receivedPayload.Timestamp.IsZero())
}

func TestWorkspaceEventEmitter_EmitFileDeleted(t *testing.T) {
	emitter := NewWorkspaceEventEmitter()

	var receivedPayload FileEventPayload
	var wg sync.WaitGroup
	wg.Add(1)

	emitter.On(EventFileDeleted, func(payload interface{}) {
		defer wg.Done()
		receivedPayload = payload.(FileEventPayload)
	})

	emitter.EmitFileDeleted("/workspace/AGENTS.md", "AGENTS.md", FileTypeAgents)
	wg.Wait()

	assert.Equal(t, "/workspace/AGENTS.md", receivedPayload.Path)
	assert.Equal(t, "AGENTS.md", receivedPayload.RelativePath)
	assert.Equal(t, FileTypeAgents, receivedPayload.Type)
	assert.False(t, receivedPayload.Timestamp.IsZero())
}

func TestWorkspaceEventEmitter_EmitInitialized(t *testing.T) {
	emitter := NewWorkspaceEventEmitter()

	var receivedPayload InitializedPayload
	var wg sync.WaitGroup
	wg.Add(1)

	emitter.On(EventInitialized, func(payload interface{}) {
		defer wg.Done()
		receivedPayload = payload.(InitializedPayload)
	})

	emitter.EmitInitialized(10)
	wg.Wait()

	assert.Equal(t, 10, receivedPayload.FileCount)
	assert.False(t, receivedPayload.Timestamp.IsZero())
}

func TestWorkspaceEventEmitter_EmitError(t *testing.T) {
	emitter := NewWorkspaceEventEmitter()

	var receivedPayload ErrorPayload
	var wg sync.WaitGroup
	wg.Add(1)

	emitter.On(EventError, func(payload interface{}) {
		defer wg.Done()
		receivedPayload = payload.(ErrorPayload)
	})

	testErr := errors.New("test error")
	context := map[string]interface{}{"file": "test.md"}

	emitter.EmitError(testErr, context)
	wg.Wait()

	assert.Equal(t, testErr, receivedPayload.Error)
	assert.Equal(t, context, receivedPayload.Context)
	assert.False(t, receivedPayload.Timestamp.IsZero())
}

func TestWorkspaceEventEmitter_MultipleListeners(t *testing.T) {
	emitter := NewWorkspaceEventEmitter()

	var wg sync.WaitGroup
	wg.Add(3)

	count := 0
	var mu sync.Mutex

	// Register multiple listeners
	for i := 0; i < 3; i++ {
		emitter.On(EventFileAdded, func(payload interface{}) {
			defer wg.Done()
			mu.Lock()
			count++
			mu.Unlock()
		})
	}

	file := &WorkspaceFile{
		Path: "/workspace/test.md",
		Type: FileTypeOther,
	}

	emitter.EmitFileAdded(file)
	wg.Wait()

	assert.Equal(t, 3, count)
}

func TestWorkspaceEventEmitter_AsyncEmission(t *testing.T) {
	emitter := NewWorkspaceEventEmitter()

	var wg sync.WaitGroup
	wg.Add(1)

	// Handler that takes some time
	emitter.On(EventFileAdded, func(payload interface{}) {
		defer wg.Done()
		time.Sleep(50 * time.Millisecond)
	})

	file := &WorkspaceFile{
		Path: "/workspace/test.md",
		Type: FileTypeOther,
	}

	start := time.Now()
	emitter.EmitFileAdded(file)
	elapsed := time.Since(start)

	// Emit should return immediately (not block)
	assert.Less(t, elapsed, 10*time.Millisecond)

	// Wait for handler to complete
	wg.Wait()
}

func TestWorkspaceEventEmitter_RemoveAllListeners(t *testing.T) {
	emitter := NewWorkspaceEventEmitter()

	var called atomic.Bool
	emitter.On(EventFileAdded, func(payload interface{}) {
		called.Store(true)
	})

	emitter.RemoveAllListeners()

	file := &WorkspaceFile{
		Path: "/workspace/test.md",
		Type: FileTypeOther,
	}

	emitter.EmitFileAdded(file)

	// Wait a bit
	time.Sleep(10 * time.Millisecond)

	// Handler should not have been called
	assert.False(t, called.Load())
}
