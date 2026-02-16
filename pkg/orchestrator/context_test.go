package orchestrator

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewContextManager(t *testing.T) {
	logger := &mockLogger{}
	cm := NewContextManager(logger)

	assert.NotNil(t, cm)
	assert.NotNil(t, cm.contexts)
	assert.Equal(t, logger, cm.logger)
}

func TestCreateContext(t *testing.T) {
	cm := NewContextManager(nil)

	ctx := AgentContext{
		ParentSessionKey: "parent-123",
		SharedState: map[string]interface{}{
			"key1": "value1",
			"key2": 42,
		},
		Files:        []string{"file1.txt", "file2.txt"},
		Instructions: "Execute this task",
		TraceID:      "trace-123",
		RunID:        "run-456",
	}

	err := cm.CreateContext("session-123", ctx)

	assert.NoError(t, err)
	assert.Contains(t, cm.contexts, "session-123")

	// Verify context was stored correctly
	stored := cm.contexts["session-123"]
	assert.Equal(t, "parent-123", stored.ParentSessionKey)
	assert.Equal(t, "Execute this task", stored.Instructions)
	assert.Equal(t, "trace-123", stored.TraceID)
	assert.Equal(t, "run-456", stored.RunID)
	assert.Len(t, stored.SharedState, 2)
	assert.Len(t, stored.Files, 2)
}

func TestCreateContext_EmptySessionKey(t *testing.T) {
	cm := NewContextManager(nil)

	ctx := AgentContext{
		Instructions: "Execute this task",
	}

	err := cm.CreateContext("", ctx)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "session key is required")
}

func TestCreateContext_AlreadyExists(t *testing.T) {
	cm := NewContextManager(nil)

	ctx := AgentContext{
		Instructions: "Execute this task",
	}

	// Create first time
	err := cm.CreateContext("session-123", ctx)
	require.NoError(t, err)

	// Try to create again
	err = cm.CreateContext("session-123", ctx)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "context already exists")
}

func TestCreateContext_Isolation(t *testing.T) {
	cm := NewContextManager(nil)

	// Create context with mutable data
	sharedState := map[string]interface{}{
		"key": "value",
	}
	files := []string{"file1.txt"}

	ctx := AgentContext{
		SharedState: sharedState,
		Files:       files,
	}

	err := cm.CreateContext("session-123", ctx)
	require.NoError(t, err)

	// Modify original data
	sharedState["key"] = "modified"
	files[0] = "modified.txt"

	// Verify stored context is not affected
	stored := cm.contexts["session-123"]
	assert.Equal(t, "value", stored.SharedState["key"])
	assert.Equal(t, "file1.txt", stored.Files[0])
}

func TestGetContext(t *testing.T) {
	cm := NewContextManager(nil)

	original := AgentContext{
		ParentSessionKey: "parent-123",
		SharedState: map[string]interface{}{
			"key": "value",
		},
		Files:        []string{"file1.txt"},
		Instructions: "Execute this task",
		TraceID:      "trace-123",
		RunID:        "run-456",
	}

	err := cm.CreateContext("session-123", original)
	require.NoError(t, err)

	// Get context
	retrieved, err := cm.GetContext("session-123")

	assert.NoError(t, err)
	assert.Equal(t, original.ParentSessionKey, retrieved.ParentSessionKey)
	assert.Equal(t, original.Instructions, retrieved.Instructions)
	assert.Equal(t, original.TraceID, retrieved.TraceID)
	assert.Equal(t, original.RunID, retrieved.RunID)
	assert.Equal(t, original.SharedState["key"], retrieved.SharedState["key"])
	assert.Equal(t, original.Files[0], retrieved.Files[0])
}

func TestGetContext_EmptySessionKey(t *testing.T) {
	cm := NewContextManager(nil)

	_, err := cm.GetContext("")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "session key is required")
}

func TestGetContext_NotFound(t *testing.T) {
	cm := NewContextManager(nil)

	_, err := cm.GetContext("nonexistent")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "context not found")
}

func TestGetContext_Isolation(t *testing.T) {
	cm := NewContextManager(nil)

	ctx := AgentContext{
		SharedState: map[string]interface{}{
			"key": "value",
		},
		Files: []string{"file1.txt"},
	}

	err := cm.CreateContext("session-123", ctx)
	require.NoError(t, err)

	// Get context and modify it
	retrieved, err := cm.GetContext("session-123")
	require.NoError(t, err)

	retrieved.SharedState["key"] = "modified"
	retrieved.Files[0] = "modified.txt"

	// Verify stored context is not affected
	stored := cm.contexts["session-123"]
	assert.Equal(t, "value", stored.SharedState["key"])
	assert.Equal(t, "file1.txt", stored.Files[0])
}

func TestUpdateContext(t *testing.T) {
	cm := NewContextManager(nil)

	original := AgentContext{
		Instructions: "Original instructions",
		SharedState: map[string]interface{}{
			"key": "value",
		},
	}

	err := cm.CreateContext("session-123", original)
	require.NoError(t, err)

	// Update context
	updated := AgentContext{
		Instructions: "Updated instructions",
		SharedState: map[string]interface{}{
			"key":    "new value",
			"newkey": "new data",
		},
	}

	err = cm.UpdateContext("session-123", updated)

	assert.NoError(t, err)

	// Verify update
	stored := cm.contexts["session-123"]
	assert.Equal(t, "Updated instructions", stored.Instructions)
	assert.Equal(t, "new value", stored.SharedState["key"])
	assert.Equal(t, "new data", stored.SharedState["newkey"])
}

func TestUpdateContext_EmptySessionKey(t *testing.T) {
	cm := NewContextManager(nil)

	err := cm.UpdateContext("", AgentContext{})

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "session key is required")
}

func TestUpdateContext_NotFound(t *testing.T) {
	cm := NewContextManager(nil)

	err := cm.UpdateContext("nonexistent", AgentContext{})

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "context not found")
}

func TestDeleteContext(t *testing.T) {
	cm := NewContextManager(nil)

	ctx := AgentContext{
		Instructions: "Execute this task",
	}

	err := cm.CreateContext("session-123", ctx)
	require.NoError(t, err)

	// Delete context
	err = cm.DeleteContext("session-123")

	assert.NoError(t, err)
	assert.NotContains(t, cm.contexts, "session-123")
}

func TestDeleteContext_EmptySessionKey(t *testing.T) {
	cm := NewContextManager(nil)

	err := cm.DeleteContext("")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "session key is required")
}

func TestDeleteContext_NotFound(t *testing.T) {
	cm := NewContextManager(nil)

	err := cm.DeleteContext("nonexistent")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "context not found")
}

func TestPassContext(t *testing.T) {
	logger := &mockLogger{}
	cm := NewContextManager(logger)

	// Create parent context
	parentCtx := AgentContext{
		ParentSessionKey: "",
		SharedState: map[string]interface{}{
			"key1": "value1",
			"key2": 42,
		},
		Files:   []string{"file1.txt", "file2.txt"},
		TraceID: "trace-123",
		RunID:   "run-456",
	}

	err := cm.CreateContext("parent-session", parentCtx)
	require.NoError(t, err)

	// Pass context to child
	childCtx, err := cm.PassContext("parent-session", "child-session", "Child instructions")

	assert.NoError(t, err)
	assert.Equal(t, "parent-session", childCtx.ParentSessionKey)
	assert.Equal(t, "Child instructions", childCtx.Instructions)
	assert.Equal(t, "trace-123", childCtx.TraceID)
	assert.Equal(t, "run-456", childCtx.RunID)
	assert.Len(t, childCtx.SharedState, 2)
	assert.Len(t, childCtx.Files, 2)

	// Verify child context was stored
	assert.Contains(t, cm.contexts, "child-session")

	// Verify logging
	assert.Contains(t, logger.infos, "Context passed to child")
}

func TestPassContext_EmptyParentSessionKey(t *testing.T) {
	cm := NewContextManager(nil)

	_, err := cm.PassContext("", "child-session", "Instructions")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "parent session key is required")
}

func TestPassContext_EmptyChildSessionKey(t *testing.T) {
	cm := NewContextManager(nil)

	_, err := cm.PassContext("parent-session", "", "Instructions")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "child session key is required")
}

func TestPassContext_ParentNotFound(t *testing.T) {
	cm := NewContextManager(nil)

	_, err := cm.PassContext("nonexistent", "child-session", "Instructions")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get parent context")
}

func TestPassContext_Isolation(t *testing.T) {
	cm := NewContextManager(nil)

	// Create parent context
	parentCtx := AgentContext{
		SharedState: map[string]interface{}{
			"key": "value",
		},
		Files: []string{"file1.txt"},
	}

	err := cm.CreateContext("parent-session", parentCtx)
	require.NoError(t, err)

	// Pass context to child
	childCtx, err := cm.PassContext("parent-session", "child-session", "Child instructions")
	require.NoError(t, err)

	// Modify child context
	childCtx.SharedState["key"] = "modified"
	childCtx.Files[0] = "modified.txt"

	// Verify parent context is not affected
	parentStored := cm.contexts["parent-session"]
	assert.Equal(t, "value", parentStored.SharedState["key"])
	assert.Equal(t, "file1.txt", parentStored.Files[0])

	// Verify child stored context is also not affected
	childStored := cm.contexts["child-session"]
	assert.Equal(t, "value", childStored.SharedState["key"])
	assert.Equal(t, "file1.txt", childStored.Files[0])
}

func TestMergeSharedState(t *testing.T) {
	cm := NewContextManager(nil)

	// Create parent context
	parentCtx := AgentContext{
		SharedState: map[string]interface{}{
			"key1": "value1",
			"key2": "value2",
		},
	}

	err := cm.CreateContext("parent-session", parentCtx)
	require.NoError(t, err)

	// Create child context with modified state
	childCtx := AgentContext{
		ParentSessionKey: "parent-session",
		SharedState: map[string]interface{}{
			"key2": "modified",
			"key3": "new value",
		},
	}

	err = cm.CreateContext("child-session", childCtx)
	require.NoError(t, err)

	// Merge child state back to parent
	err = cm.MergeSharedState("parent-session", "child-session")

	assert.NoError(t, err)

	// Verify parent state was updated
	parentStored := cm.contexts["parent-session"]
	assert.Equal(t, "value1", parentStored.SharedState["key1"])
	assert.Equal(t, "modified", parentStored.SharedState["key2"])
	assert.Equal(t, "new value", parentStored.SharedState["key3"])
}

func TestMergeSharedState_EmptyParentSessionKey(t *testing.T) {
	cm := NewContextManager(nil)

	err := cm.MergeSharedState("", "child-session")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "parent session key is required")
}

func TestMergeSharedState_EmptyChildSessionKey(t *testing.T) {
	cm := NewContextManager(nil)

	err := cm.MergeSharedState("parent-session", "")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "child session key is required")
}

func TestMergeSharedState_ParentNotFound(t *testing.T) {
	cm := NewContextManager(nil)

	// Create child context
	childCtx := AgentContext{
		SharedState: map[string]interface{}{
			"key": "value",
		},
	}

	err := cm.CreateContext("child-session", childCtx)
	require.NoError(t, err)

	// Try to merge with nonexistent parent
	err = cm.MergeSharedState("nonexistent", "child-session")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get parent context")
}

func TestMergeSharedState_ChildNotFound(t *testing.T) {
	cm := NewContextManager(nil)

	// Create parent context
	parentCtx := AgentContext{
		SharedState: map[string]interface{}{
			"key": "value",
		},
	}

	err := cm.CreateContext("parent-session", parentCtx)
	require.NoError(t, err)

	// Try to merge with nonexistent child
	err = cm.MergeSharedState("parent-session", "nonexistent")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get child context")
}

func TestMergeSharedState_NilParentState(t *testing.T) {
	cm := NewContextManager(nil)

	// Create parent context with nil shared state
	parentCtx := AgentContext{
		SharedState: nil,
	}

	err := cm.CreateContext("parent-session", parentCtx)
	require.NoError(t, err)

	// Create child context with state
	childCtx := AgentContext{
		SharedState: map[string]interface{}{
			"key": "value",
		},
	}

	err = cm.CreateContext("child-session", childCtx)
	require.NoError(t, err)

	// Merge should initialize parent state
	err = cm.MergeSharedState("parent-session", "child-session")

	assert.NoError(t, err)

	// Verify parent state was initialized and updated
	parentStored := cm.contexts["parent-session"]
	assert.NotNil(t, parentStored.SharedState)
	assert.Equal(t, "value", parentStored.SharedState["key"])
}

func TestListContexts(t *testing.T) {
	cm := NewContextManager(nil)

	// Initially empty
	keys := cm.ListContexts()
	assert.Empty(t, keys)

	// Add contexts
	ctx1 := AgentContext{Instructions: "Task 1"}
	ctx2 := AgentContext{Instructions: "Task 2"}
	ctx3 := AgentContext{Instructions: "Task 3"}

	err := cm.CreateContext("session-1", ctx1)
	require.NoError(t, err)
	err = cm.CreateContext("session-2", ctx2)
	require.NoError(t, err)
	err = cm.CreateContext("session-3", ctx3)
	require.NoError(t, err)

	// List contexts
	keys = cm.ListContexts()

	assert.Len(t, keys, 3)
	assert.Contains(t, keys, "session-1")
	assert.Contains(t, keys, "session-2")
	assert.Contains(t, keys, "session-3")
}

func TestContextManager_Clear(t *testing.T) {
	logger := &mockLogger{}
	cm := NewContextManager(logger)

	// Add contexts
	ctx1 := AgentContext{Instructions: "Task 1"}
	ctx2 := AgentContext{Instructions: "Task 2"}

	err := cm.CreateContext("session-1", ctx1)
	require.NoError(t, err)
	err = cm.CreateContext("session-2", ctx2)
	require.NoError(t, err)

	assert.Len(t, cm.contexts, 2)

	// Clear all contexts
	cm.Clear()

	assert.Empty(t, cm.contexts)
	assert.Contains(t, logger.debugs, "All contexts cleared")
}
