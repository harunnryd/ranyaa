package toolexecutor

import (
	"context"
	"testing"
	"time"

	"github.com/harun/ranya/pkg/sandbox"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestToolExecutor_WithSandbox(t *testing.T) {
	// Create tool executor
	te := New()

	// Register a test tool
	err := te.RegisterTool(ToolDefinition{
		Name:        "test-tool",
		Description: "A test tool",
		Parameters: []ToolParameter{
			{
				Name:        "input",
				Type:        "string",
				Description: "Input string",
				Required:    true,
			},
		},
		Handler: func(ctx context.Context, params map[string]interface{}) (interface{}, error) {
			input := params["input"].(string)
			return map[string]string{"output": "processed: " + input}, nil
		},
	})
	require.NoError(t, err)

	// Create sandbox manager
	cfg := sandbox.DefaultConfig()
	cfg.ResourceLimits.Timeout = 10 * time.Second
	sm := NewSandboxManager(cfg)
	defer func() { _ = sm.StopAll(context.Background()) }()

	// Set sandbox manager on executor
	te.SetSandboxManager(sm)

	t.Run("execute without sandbox", func(t *testing.T) {
		// Execute without sandbox policy
		execCtx := &ExecutionContext{
			AgentID:    "test-agent",
			SessionKey: "test-session",
			Timeout:    5 * time.Second,
		}

		result := te.Execute(context.Background(), "test-tool", map[string]interface{}{
			"input": "hello",
		}, execCtx)

		assert.True(t, result.Success)
		assert.NotNil(t, result.Output)
	})

	t.Run("execute with sandbox disabled", func(t *testing.T) {
		// Execute with sandbox mode off
		execCtx := &ExecutionContext{
			AgentID:    "test-agent",
			SessionKey: "test-session",
			SandboxPolicy: map[string]interface{}{
				"mode": "off",
			},
			Timeout: 5 * time.Second,
		}

		result := te.Execute(context.Background(), "test-tool", map[string]interface{}{
			"input": "hello",
		}, execCtx)

		assert.True(t, result.Success)
		assert.NotNil(t, result.Output)
	})

	t.Run("execute with sandbox enabled", func(t *testing.T) {
		// Execute with sandbox mode all
		execCtx := &ExecutionContext{
			AgentID:    "test-agent",
			SessionKey: "test-session",
			SandboxPolicy: map[string]interface{}{
				"mode":  "all",
				"scope": "agent",
			},
			Timeout: 5 * time.Second,
		}

		result := te.Execute(context.Background(), "test-tool", map[string]interface{}{
			"input": "hello",
		}, execCtx)

		assert.True(t, result.Success)
		assert.NotNil(t, result.Output)
	})
}

func TestToolExecutor_SetSandboxManager(t *testing.T) {
	te := New()

	// Initially no sandbox manager
	assert.Nil(t, te.sandboxManager)

	// Set sandbox manager
	cfg := sandbox.DefaultConfig()
	sm := NewSandboxManager(cfg)
	te.SetSandboxManager(sm)

	// Should now have sandbox manager
	assert.NotNil(t, te.sandboxManager)
	assert.Equal(t, sm, te.sandboxManager)
}
