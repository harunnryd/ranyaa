package orchestrator

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestContextManager_MultiLevelHierarchy(t *testing.T) {
	logger := &mockLogger{}
	cm := NewContextManager(logger)

	// Create root context
	rootCtx := AgentContext{
		SharedState: map[string]interface{}{
			"root_key": "root_value",
		},
		Files:   []string{"root.txt"},
		TraceID: "trace-123",
		RunID:   "run-456",
	}

	err := cm.CreateContext("root-session", rootCtx)
	require.NoError(t, err)

	// Pass context to level 1 child
	level1Ctx, err := cm.PassContext("root-session", "level1-session", "Level 1 instructions")
	require.NoError(t, err)

	// Modify level 1 context
	level1Ctx.SharedState["level1_key"] = "level1_value"
	err = cm.UpdateContext("level1-session", level1Ctx)
	require.NoError(t, err)

	// Pass context to level 2 child
	level2Ctx, err := cm.PassContext("level1-session", "level2-session", "Level 2 instructions")
	require.NoError(t, err)

	// Verify level 2 has inherited state from level 1
	assert.Equal(t, "level1-session", level2Ctx.ParentSessionKey)
	assert.Equal(t, "root_value", level2Ctx.SharedState["root_key"])
	assert.Equal(t, "level1_value", level2Ctx.SharedState["level1_key"])
	assert.Equal(t, "trace-123", level2Ctx.TraceID)
	assert.Equal(t, "run-456", level2Ctx.RunID)

	// Modify level 2 context
	level2Ctx.SharedState["level2_key"] = "level2_value"
	err = cm.UpdateContext("level2-session", level2Ctx)
	require.NoError(t, err)

	// Merge level 2 back to level 1
	err = cm.MergeSharedState("level1-session", "level2-session")
	require.NoError(t, err)

	// Verify level 1 has level 2 changes
	level1Updated, err := cm.GetContext("level1-session")
	require.NoError(t, err)
	assert.Equal(t, "level2_value", level1Updated.SharedState["level2_key"])

	// Merge level 1 back to root
	err = cm.MergeSharedState("root-session", "level1-session")
	require.NoError(t, err)

	// Verify root has all changes
	rootUpdated, err := cm.GetContext("root-session")
	require.NoError(t, err)
	assert.Equal(t, "root_value", rootUpdated.SharedState["root_key"])
	assert.Equal(t, "level1_value", rootUpdated.SharedState["level1_key"])
	assert.Equal(t, "level2_value", rootUpdated.SharedState["level2_key"])
}

func TestContextManager_ConcurrentAccess(t *testing.T) {
	cm := NewContextManager(nil)

	// Create initial context
	ctx := AgentContext{
		SharedState: map[string]interface{}{
			"counter": 0,
		},
	}

	err := cm.CreateContext("session-123", ctx)
	require.NoError(t, err)

	// Simulate concurrent reads and writes
	done := make(chan bool)
	numGoroutines := 10

	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			// Read context
			ctx, err := cm.GetContext("session-123")
			assert.NoError(t, err)

			// Modify context
			ctx.SharedState[string(rune('a'+id))] = id

			// Update context
			err = cm.UpdateContext("session-123", ctx)
			assert.NoError(t, err)

			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < numGoroutines; i++ {
		<-done
	}

	// Verify context is still valid
	finalCtx, err := cm.GetContext("session-123")
	assert.NoError(t, err)
	assert.NotNil(t, finalCtx.SharedState)
}
