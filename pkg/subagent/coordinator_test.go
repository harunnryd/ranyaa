package subagent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupTestCoordinator(t *testing.T) (*Coordinator, string, func()) {
	tmpDir, err := os.MkdirTemp("", "subagent-test-*")
	require.NoError(t, err)

	registryPath := filepath.Join(tmpDir, "subagents.json")
	logger := zerolog.New(os.Stdout).Level(zerolog.ErrorLevel)

	coordinator, err := NewCoordinator(Config{
		RegistryPath: registryPath,
		AutoSave:     true,
		Logger:       logger,
	})
	require.NoError(t, err)

	err = coordinator.Initialize()
	require.NoError(t, err)

	cleanup := func() {
		coordinator.Close()
		os.RemoveAll(tmpDir)
	}

	return coordinator, tmpDir, cleanup
}

func TestNewCoordinator(t *testing.T) {
	t.Run("should create coordinator with custom path", func(t *testing.T) {
		tmpDir, _ := os.MkdirTemp("", "test-*")
		defer os.RemoveAll(tmpDir)

		registryPath := filepath.Join(tmpDir, "test.json")
		logger := zerolog.New(os.Stdout)

		coordinator, err := NewCoordinator(Config{
			RegistryPath: registryPath,
			AutoSave:     false,
			Logger:       logger,
		})

		assert.NoError(t, err)
		assert.NotNil(t, coordinator)
		assert.Equal(t, registryPath, coordinator.registryPath)
		assert.False(t, coordinator.autoSave)
	})

	t.Run("should use default path if not provided", func(t *testing.T) {
		logger := zerolog.New(os.Stdout)

		coordinator, err := NewCoordinator(Config{
			Logger: logger,
		})

		assert.NoError(t, err)
		assert.NotNil(t, coordinator)
		assert.Contains(t, coordinator.registryPath, ".ranya")
	})
}

func TestInitialize(t *testing.T) {
	t.Run("should initialize with empty registry", func(t *testing.T) {
		coordinator, _, cleanup := setupTestCoordinator(t)
		defer cleanup()

		stats := coordinator.GetStats()
		assert.Equal(t, 0, stats.TotalRuns)
	})

	t.Run("should load existing registry", func(t *testing.T) {
		tmpDir, _ := os.MkdirTemp("", "test-*")
		defer os.RemoveAll(tmpDir)

		registryPath := filepath.Join(tmpDir, "test.json")

		// Create registry file
		registry := Registry{
			Version: 1,
			Runs: []*RunRecord{
				{
					ID:               "test-1",
					ParentSessionKey: "parent",
					ChildSessionKey:  "child",
					Prompt:           "test",
					Status:           StatusPending,
					StartedAt:        time.Now().UnixMilli(),
				},
			},
			LastUpdated: time.Now().UnixMilli(),
		}

		data, _ := json.Marshal(registry)
		_ = os.WriteFile(registryPath, data, 0600)

		// Initialize coordinator
		logger := zerolog.New(os.Stdout).Level(zerolog.ErrorLevel)
		coordinator, _ := NewCoordinator(Config{
			RegistryPath: registryPath,
			Logger:       logger,
		})

		err := coordinator.Initialize()
		assert.NoError(t, err)

		stats := coordinator.GetStats()
		assert.Equal(t, 1, stats.TotalRuns)
	})

	t.Run("should handle corrupted registry gracefully", func(t *testing.T) {
		tmpDir, _ := os.MkdirTemp("", "test-*")
		defer os.RemoveAll(tmpDir)

		registryPath := filepath.Join(tmpDir, "corrupted.json")
		_ = os.WriteFile(registryPath, []byte("invalid json{{{"), 0600)

		logger := zerolog.New(os.Stdout).Level(zerolog.ErrorLevel)
		coordinator, _ := NewCoordinator(Config{
			RegistryPath: registryPath,
			Logger:       logger,
		})

		err := coordinator.Initialize()
		assert.NoError(t, err)

		// Should start with empty registry
		stats := coordinator.GetStats()
		assert.Equal(t, 0, stats.TotalRuns)
	})
}

func TestRegisterRun(t *testing.T) {
	t.Run("should register run with correct initial state", func(t *testing.T) {
		coordinator, _, cleanup := setupTestCoordinator(t)
		defer cleanup()

		runID, err := coordinator.RegisterRun(RunParams{
			ParentSessionKey: "parent-1",
			ChildSessionKey:  "child-1",
			Prompt:           "test prompt",
		})

		assert.NoError(t, err)
		assert.NotEmpty(t, runID)

		run := coordinator.GetRun(runID)
		assert.NotNil(t, run)
		assert.Equal(t, "parent-1", run.ParentSessionKey)
		assert.Equal(t, "child-1", run.ChildSessionKey)
		assert.Equal(t, "test prompt", run.Prompt)
		assert.Equal(t, StatusPending, run.Status)
		assert.Greater(t, run.StartedAt, int64(0))
	})

	t.Run("should generate unique run IDs", func(t *testing.T) {
		coordinator, _, cleanup := setupTestCoordinator(t)
		defer cleanup()

		runIDs := make(map[string]bool)

		for i := 0; i < 100; i++ {
			runID, err := coordinator.RegisterRun(RunParams{
				ParentSessionKey: "parent",
				ChildSessionKey:  "child-" + string(rune(i)),
				Prompt:           "test",
			})

			assert.NoError(t, err)
			assert.False(t, runIDs[runID], "Duplicate run ID: %s", runID)
			runIDs[runID] = true
		}
	})

	t.Run("should emit run:registered event", func(t *testing.T) {
		coordinator, _, cleanup := setupTestCoordinator(t)
		defer cleanup()

		eventReceived := false
		coordinator.On(EventRunRegistered, func(event interface{}) {
			eventReceived = true
		})

		_, err := coordinator.RegisterRun(RunParams{
			ParentSessionKey: "parent",
			ChildSessionKey:  "child",
			Prompt:           "test",
		})

		assert.NoError(t, err)
		assert.True(t, eventReceived)
	})

	t.Run("should persist run if autoSave enabled", func(t *testing.T) {
		coordinator, tmpDir, cleanup := setupTestCoordinator(t)
		defer cleanup()

		runID, err := coordinator.RegisterRun(RunParams{
			ParentSessionKey: "parent",
			ChildSessionKey:  "child",
			Prompt:           "test",
		})

		assert.NoError(t, err)

		// Check registry file exists
		registryPath := filepath.Join(tmpDir, "subagents.json")
		_, err = os.Stat(registryPath)
		assert.NoError(t, err)

		// Load and verify
		data, _ := os.ReadFile(registryPath)
		var registry Registry
		_ = json.Unmarshal(data, &registry)

		assert.Len(t, registry.Runs, 1)
		assert.Equal(t, runID, registry.Runs[0].ID)
	})
}

func TestUpdateRunStatus(t *testing.T) {
	t.Run("should update status correctly", func(t *testing.T) {
		coordinator, _, cleanup := setupTestCoordinator(t)
		defer cleanup()

		runID, _ := coordinator.RegisterRun(RunParams{
			ParentSessionKey: "parent",
			ChildSessionKey:  "child",
			Prompt:           "test",
		})

		err := coordinator.UpdateRunStatus(runID, StatusRunning, nil)
		assert.NoError(t, err)

		run := coordinator.GetRun(runID)
		assert.Equal(t, StatusRunning, run.Status)
	})

	t.Run("should set completion timestamp for terminal status", func(t *testing.T) {
		coordinator, _, cleanup := setupTestCoordinator(t)
		defer cleanup()

		runID, _ := coordinator.RegisterRun(RunParams{
			ParentSessionKey: "parent",
			ChildSessionKey:  "child",
			Prompt:           "test",
		})

		err := coordinator.UpdateRunStatus(runID, StatusCompleted, nil)
		assert.NoError(t, err)

		run := coordinator.GetRun(runID)
		assert.NotNil(t, run.CompletedAt)
		assert.Greater(t, *run.CompletedAt, int64(0))
	})

	t.Run("should store result data", func(t *testing.T) {
		coordinator, _, cleanup := setupTestCoordinator(t)
		defer cleanup()

		runID, _ := coordinator.RegisterRun(RunParams{
			ParentSessionKey: "parent",
			ChildSessionKey:  "child",
			Prompt:           "test",
		})

		err := coordinator.UpdateRunStatus(runID, StatusCompleted, map[string]interface{}{
			"result": "test result",
		})
		assert.NoError(t, err)

		run := coordinator.GetRun(runID)
		assert.Equal(t, "test result", run.Result)
	})

	t.Run("should store error data", func(t *testing.T) {
		coordinator, _, cleanup := setupTestCoordinator(t)
		defer cleanup()

		runID, _ := coordinator.RegisterRun(RunParams{
			ParentSessionKey: "parent",
			ChildSessionKey:  "child",
			Prompt:           "test",
		})

		err := coordinator.UpdateRunStatus(runID, StatusFailed, map[string]interface{}{
			"error": "test error",
		})
		assert.NoError(t, err)

		run := coordinator.GetRun(runID)
		assert.Equal(t, "test error", run.Error)
	})

	t.Run("should fail for non-existent run", func(t *testing.T) {
		coordinator, _, cleanup := setupTestCoordinator(t)
		defer cleanup()

		err := coordinator.UpdateRunStatus("non-existent", StatusRunning, nil)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})

	t.Run("should emit run:updated event", func(t *testing.T) {
		coordinator, _, cleanup := setupTestCoordinator(t)
		defer cleanup()

		runID, _ := coordinator.RegisterRun(RunParams{
			ParentSessionKey: "parent",
			ChildSessionKey:  "child",
			Prompt:           "test",
		})

		eventReceived := false
		coordinator.On(EventRunUpdated, func(event interface{}) {
			eventReceived = true
		})

		_ = coordinator.UpdateRunStatus(runID, StatusRunning, nil)
		assert.True(t, eventReceived)
	})
}

func TestHandleLifecycleEvent(t *testing.T) {
	t.Run("should handle start event", func(t *testing.T) {
		coordinator, _, cleanup := setupTestCoordinator(t)
		defer cleanup()

		_, _ = coordinator.RegisterRun(RunParams{
			ParentSessionKey: "parent",
			ChildSessionKey:  "child",
			Prompt:           "test",
		})

		err := coordinator.HandleLifecycleEvent(LifecycleEvent{
			Type:       EventStart,
			SessionKey: "child",
			Timestamp:  time.Now().UnixMilli(),
		})

		assert.NoError(t, err)

		run := coordinator.GetRunByChildSession("child")
		assert.Equal(t, StatusRunning, run.Status)
	})

	t.Run("should handle end event with result", func(t *testing.T) {
		coordinator, _, cleanup := setupTestCoordinator(t)
		defer cleanup()

		_, _ = coordinator.RegisterRun(RunParams{
			ParentSessionKey: "parent",
			ChildSessionKey:  "child",
			Prompt:           "test",
		})

		err := coordinator.HandleLifecycleEvent(LifecycleEvent{
			Type:       EventEnd,
			SessionKey: "child",
			Timestamp:  time.Now().UnixMilli(),
			Data:       "success result",
		})

		assert.NoError(t, err)

		run := coordinator.GetRunByChildSession("child")
		assert.Equal(t, StatusCompleted, run.Status)
		assert.Equal(t, "success result", run.Result)
	})

	t.Run("should handle error event", func(t *testing.T) {
		coordinator, _, cleanup := setupTestCoordinator(t)
		defer cleanup()

		_, _ = coordinator.RegisterRun(RunParams{
			ParentSessionKey: "parent",
			ChildSessionKey:  "child",
			Prompt:           "test",
		})

		err := coordinator.HandleLifecycleEvent(LifecycleEvent{
			Type:       EventError,
			SessionKey: "child",
			Timestamp:  time.Now().UnixMilli(),
			Data:       "error message",
		})

		assert.NoError(t, err)

		run := coordinator.GetRunByChildSession("child")
		assert.Equal(t, StatusFailed, run.Status)
		assert.Equal(t, "error message", run.Error)
	})

	t.Run("should handle abort event", func(t *testing.T) {
		coordinator, _, cleanup := setupTestCoordinator(t)
		defer cleanup()

		_, _ = coordinator.RegisterRun(RunParams{
			ParentSessionKey: "parent",
			ChildSessionKey:  "child",
			Prompt:           "test",
		})

		err := coordinator.HandleLifecycleEvent(LifecycleEvent{
			Type:       EventAbort,
			SessionKey: "child",
			Timestamp:  time.Now().UnixMilli(),
		})

		assert.NoError(t, err)

		run := coordinator.GetRunByChildSession("child")
		assert.Equal(t, StatusAborted, run.Status)
	})

	t.Run("should handle unknown session key gracefully", func(t *testing.T) {
		coordinator, _, cleanup := setupTestCoordinator(t)
		defer cleanup()

		err := coordinator.HandleLifecycleEvent(LifecycleEvent{
			Type:       EventStart,
			SessionKey: "unknown",
			Timestamp:  time.Now().UnixMilli(),
		})

		assert.NoError(t, err) // Should not error
	})

	t.Run("should emit lifecycle:event", func(t *testing.T) {
		coordinator, _, cleanup := setupTestCoordinator(t)
		defer cleanup()

		_, _ = coordinator.RegisterRun(RunParams{
			ParentSessionKey: "parent",
			ChildSessionKey:  "child",
			Prompt:           "test",
		})

		eventReceived := false
		coordinator.On(EventLifecycle, func(event interface{}) {
			eventReceived = true
		})

		_ = coordinator.HandleLifecycleEvent(LifecycleEvent{
			Type:       EventStart,
			SessionKey: "child",
			Timestamp:  time.Now().UnixMilli(),
		})

		assert.True(t, eventReceived)
	})
}

func TestGetRun(t *testing.T) {
	t.Run("should retrieve run by ID", func(t *testing.T) {
		coordinator, _, cleanup := setupTestCoordinator(t)
		defer cleanup()

		runID, _ := coordinator.RegisterRun(RunParams{
			ParentSessionKey: "parent",
			ChildSessionKey:  "child",
			Prompt:           "test",
		})

		run := coordinator.GetRun(runID)
		assert.NotNil(t, run)
		assert.Equal(t, runID, run.ID)
	})

	t.Run("should return nil for non-existent run", func(t *testing.T) {
		coordinator, _, cleanup := setupTestCoordinator(t)
		defer cleanup()

		run := coordinator.GetRun("non-existent")
		assert.Nil(t, run)
	})
}

func TestGetRunByChildSession(t *testing.T) {
	t.Run("should retrieve run by child session key", func(t *testing.T) {
		coordinator, _, cleanup := setupTestCoordinator(t)
		defer cleanup()

		_, _ = coordinator.RegisterRun(RunParams{
			ParentSessionKey: "parent",
			ChildSessionKey:  "child-unique",
			Prompt:           "test",
		})

		run := coordinator.GetRunByChildSession("child-unique")
		assert.NotNil(t, run)
		assert.Equal(t, "child-unique", run.ChildSessionKey)
	})

	t.Run("should return nil for non-existent session", func(t *testing.T) {
		coordinator, _, cleanup := setupTestCoordinator(t)
		defer cleanup()

		run := coordinator.GetRunByChildSession("non-existent")
		assert.Nil(t, run)
	})
}

func TestListChildren(t *testing.T) {
	t.Run("should list direct children", func(t *testing.T) {
		coordinator, _, cleanup := setupTestCoordinator(t)
		defer cleanup()

		// Register parent and children
		_, _ = coordinator.RegisterRun(RunParams{
			ParentSessionKey: "parent",
			ChildSessionKey:  "child-1",
			Prompt:           "test",
		})
		_, _ = coordinator.RegisterRun(RunParams{
			ParentSessionKey: "parent",
			ChildSessionKey:  "child-2",
			Prompt:           "test",
		})
		_, _ = coordinator.RegisterRun(RunParams{
			ParentSessionKey: "other-parent",
			ChildSessionKey:  "child-3",
			Prompt:           "test",
		})

		children := coordinator.ListChildren("parent")
		assert.Len(t, children, 2)
	})

	t.Run("should return empty for session with no children", func(t *testing.T) {
		coordinator, _, cleanup := setupTestCoordinator(t)
		defer cleanup()

		children := coordinator.ListChildren("no-children")
		assert.Empty(t, children)
	})
}

func TestListDescendants(t *testing.T) {
	t.Run("should list all descendants recursively", func(t *testing.T) {
		coordinator, _, cleanup := setupTestCoordinator(t)
		defer cleanup()

		// Build hierarchy: root -> child1 -> grandchild1
		//                       -> child2
		_, _ = coordinator.RegisterRun(RunParams{
			ParentSessionKey: "root",
			ChildSessionKey:  "child1",
			Prompt:           "test",
		})
		_, _ = coordinator.RegisterRun(RunParams{
			ParentSessionKey: "root",
			ChildSessionKey:  "child2",
			Prompt:           "test",
		})
		_, _ = coordinator.RegisterRun(RunParams{
			ParentSessionKey: "child1",
			ChildSessionKey:  "grandchild1",
			Prompt:           "test",
		})

		descendants := coordinator.ListDescendants("root")
		assert.Len(t, descendants, 3) // child1, child2, grandchild1
	})

	t.Run("should return empty for leaf node", func(t *testing.T) {
		coordinator, _, cleanup := setupTestCoordinator(t)
		defer cleanup()

		_, _ = coordinator.RegisterRun(RunParams{
			ParentSessionKey: "parent",
			ChildSessionKey:  "leaf",
			Prompt:           "test",
		})

		descendants := coordinator.ListDescendants("leaf")
		assert.Empty(t, descendants)
	})
}

func TestCountActiveRuns(t *testing.T) {
	t.Run("should count only active runs", func(t *testing.T) {
		coordinator, _, cleanup := setupTestCoordinator(t)
		defer cleanup()

		// Register runs with different statuses
		runID1, _ := coordinator.RegisterRun(RunParams{
			ParentSessionKey: "parent",
			ChildSessionKey:  "child1",
			Prompt:           "test",
		})
		runID2, _ := coordinator.RegisterRun(RunParams{
			ParentSessionKey: "parent",
			ChildSessionKey:  "child2",
			Prompt:           "test",
		})
		runID3, _ := coordinator.RegisterRun(RunParams{
			ParentSessionKey: "parent",
			ChildSessionKey:  "child3",
			Prompt:           "test",
		})

		// Update statuses
		_ = coordinator.UpdateRunStatus(runID1, StatusRunning, nil)
		_ = coordinator.UpdateRunStatus(runID2, StatusCompleted, nil)
		_ = coordinator.UpdateRunStatus(runID3, StatusPending, nil)

		count := coordinator.CountActiveRuns("parent")
		assert.Equal(t, 2, count) // pending + running
	})
}

func TestCountDescendants(t *testing.T) {
	t.Run("should count all descendants", func(t *testing.T) {
		coordinator, _, cleanup := setupTestCoordinator(t)
		defer cleanup()

		_, _ = coordinator.RegisterRun(RunParams{
			ParentSessionKey: "root",
			ChildSessionKey:  "child1",
			Prompt:           "test",
		})
		_, _ = coordinator.RegisterRun(RunParams{
			ParentSessionKey: "child1",
			ChildSessionKey:  "grandchild1",
			Prompt:           "test",
		})

		count := coordinator.CountDescendants("root")
		assert.Equal(t, 2, count)
	})
}

func TestCleanup(t *testing.T) {
	t.Run("should remove old completed runs", func(t *testing.T) {
		coordinator, _, cleanup := setupTestCoordinator(t)
		defer cleanup()

		// Register old completed run
		runID, _ := coordinator.RegisterRun(RunParams{
			ParentSessionKey: "parent",
			ChildSessionKey:  "child",
			Prompt:           "test",
		})

		oldTime := time.Now().UnixMilli() - (8 * 24 * 60 * 60 * 1000) // 8 days ago
		_ = coordinator.UpdateRunStatus(runID, StatusCompleted, nil)

		// Manually set old completion time
		run := coordinator.GetRun(runID)
		run.CompletedAt = &oldTime

		// Cleanup with 7 day retention
		removed, err := coordinator.Cleanup(7 * 24 * 60 * 60 * 1000)

		assert.NoError(t, err)
		assert.Equal(t, 1, removed)
		assert.Nil(t, coordinator.GetRun(runID))
	})

	t.Run("should not remove active runs", func(t *testing.T) {
		coordinator, _, cleanup := setupTestCoordinator(t)
		defer cleanup()

		runID, _ := coordinator.RegisterRun(RunParams{
			ParentSessionKey: "parent",
			ChildSessionKey:  "child",
			Prompt:           "test",
		})

		removed, err := coordinator.Cleanup(0)

		assert.NoError(t, err)
		assert.Equal(t, 0, removed)
		assert.NotNil(t, coordinator.GetRun(runID))
	})

	t.Run("should not remove recent completed runs", func(t *testing.T) {
		coordinator, _, cleanup := setupTestCoordinator(t)
		defer cleanup()

		runID, _ := coordinator.RegisterRun(RunParams{
			ParentSessionKey: "parent",
			ChildSessionKey:  "child",
			Prompt:           "test",
		})

		_ = coordinator.UpdateRunStatus(runID, StatusCompleted, nil)

		removed, err := coordinator.Cleanup(7 * 24 * 60 * 60 * 1000)

		assert.NoError(t, err)
		assert.Equal(t, 0, removed)
		assert.NotNil(t, coordinator.GetRun(runID))
	})
}

func TestGetStats(t *testing.T) {
	t.Run("should return accurate statistics", func(t *testing.T) {
		coordinator, _, cleanup := setupTestCoordinator(t)
		defer cleanup()

		// Register runs with different statuses
		runID1, _ := coordinator.RegisterRun(RunParams{
			ParentSessionKey: "parent",
			ChildSessionKey:  "child1",
			Prompt:           "test",
		})
		runID2, _ := coordinator.RegisterRun(RunParams{
			ParentSessionKey: "parent",
			ChildSessionKey:  "child2",
			Prompt:           "test",
		})
		runID3, _ := coordinator.RegisterRun(RunParams{
			ParentSessionKey: "parent",
			ChildSessionKey:  "child3",
			Prompt:           "test",
		})
		runID4, _ := coordinator.RegisterRun(RunParams{
			ParentSessionKey: "parent",
			ChildSessionKey:  "child4",
			Prompt:           "test",
		})

		_ = coordinator.UpdateRunStatus(runID1, StatusRunning, nil)
		_ = coordinator.UpdateRunStatus(runID2, StatusCompleted, nil)
		_ = coordinator.UpdateRunStatus(runID3, StatusFailed, nil)
		coordinator.UpdateRunStatus(runID4, StatusAborted, nil)

		stats := coordinator.GetStats()

		assert.Equal(t, 4, stats.TotalRuns)
		assert.Equal(t, 1, stats.ActiveRuns) // pending + running
		assert.Equal(t, 1, stats.CompletedRuns)
		assert.Equal(t, 1, stats.FailedRuns)
		assert.Equal(t, 1, stats.AbortedRuns)
	})
}

func TestEventHandlers(t *testing.T) {
	t.Run("should register and call event handlers", func(t *testing.T) {
		coordinator, _, cleanup := setupTestCoordinator(t)
		defer cleanup()

		called := false
		coordinator.On("test-event", func(event interface{}) {
			called = true
		})

		coordinator.emit("test-event", nil)
		assert.True(t, called)
	})

	t.Run("should remove event handlers", func(t *testing.T) {
		coordinator, _, cleanup := setupTestCoordinator(t)
		defer cleanup()

		called := false
		coordinator.On("test-event", func(event interface{}) {
			called = true
		})

		coordinator.Off("test-event")
		coordinator.emit("test-event", nil)

		assert.False(t, called)
	})
}

func TestPersistence(t *testing.T) {
	t.Run("should persist and load registry", func(t *testing.T) {
		tmpDir, _ := os.MkdirTemp("", "test-*")
		defer os.RemoveAll(tmpDir)

		registryPath := filepath.Join(tmpDir, "test.json")
		logger := zerolog.New(os.Stdout).Level(zerolog.ErrorLevel)

		// Create first coordinator and register run
		coordinator1, _ := NewCoordinator(Config{
			RegistryPath: registryPath,
			AutoSave:     true,
			Logger:       logger,
		})
		_ = coordinator1.Initialize()

		runID, _ := coordinator1.RegisterRun(RunParams{
			ParentSessionKey: "parent",
			ChildSessionKey:  "child",
			Prompt:           "test prompt",
		})

		coordinator1.Close()

		// Create second coordinator and load
		coordinator2, _ := NewCoordinator(Config{
			RegistryPath: registryPath,
			Logger:       logger,
		})
		_ = coordinator2.Initialize()

		run := coordinator2.GetRun(runID)
		assert.NotNil(t, run)
		assert.Equal(t, "test prompt", run.Prompt)
	})
}
