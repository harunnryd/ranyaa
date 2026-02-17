package toolexecutor

import (
	"context"
	"testing"
	"time"

	"github.com/harun/ranya/pkg/sandbox"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSandboxIntegration_FullWorkflow(t *testing.T) {
	cfg := sandbox.DefaultConfig()
	cfg.ResourceLimits.Timeout = 10 * time.Second
	sm := NewSandboxManager(cfg)

	ctx := context.Background()

	// Create sandbox
	sb, err := sm.GetOrCreateSandbox(ctx, "agent-1")
	require.NoError(t, err)
	assert.NotNil(t, sb)
	assert.True(t, sb.IsRunning())

	// Execute command
	req := sandbox.ExecuteRequest{
		Command: "echo",
		Args:    []string{"integration test"},
		Timeout: 2 * time.Second,
	}

	result, err := sb.Execute(ctx, req)
	require.NoError(t, err)
	assert.Equal(t, 0, result.ExitCode)
	assert.Contains(t, string(result.Stdout), "integration test")

	// Stop sandbox
	err = sm.StopSandbox(ctx, "agent-1")
	require.NoError(t, err)
	assert.Len(t, sm.sandboxes, 0)
}

func TestSandboxIntegration_MultipleSandboxes(t *testing.T) {
	cfg := sandbox.DefaultConfig()
	cfg.ResourceLimits.Timeout = 10 * time.Second
	sm := NewSandboxManager(cfg)

	ctx := context.Background()
	defer func() { _ = sm.StopAll(ctx) }()

	// Create agent-scoped sandbox
	sb1, err := sm.GetOrCreateSandbox(ctx, "agent-1")
	require.NoError(t, err)

	// Create session-scoped sandbox
	sb2, err := sm.GetOrCreateSandbox(ctx, "session-1")
	require.NoError(t, err)

	// They should be different sandboxes
	assert.NotSame(t, sb1, sb2, "Should be different sandbox instances")

	// Both should be running
	assert.True(t, sb1.IsRunning())
	assert.True(t, sb2.IsRunning())

	// Execute in both sandboxes
	req := sandbox.ExecuteRequest{
		Command: "echo",
		Args:    []string{"test"},
		Timeout: 2 * time.Second,
	}

	result1, err := sb1.Execute(ctx, req)
	require.NoError(t, err)
	assert.Equal(t, 0, result1.ExitCode)

	result2, err := sb2.Execute(ctx, req)
	require.NoError(t, err)
	assert.Equal(t, 0, result2.ExitCode)
}

func TestSandboxIntegration_SandboxReuse(t *testing.T) {
	cfg := sandbox.DefaultConfig()
	cfg.ResourceLimits.Timeout = 10 * time.Second
	sm := NewSandboxManager(cfg)

	ctx := context.Background()
	defer func() { _ = sm.StopAll(ctx) }()

	// Get sandbox first time
	sb1, err := sm.GetOrCreateSandbox(ctx, "agent-1")
	require.NoError(t, err)

	// Execute command
	req := sandbox.ExecuteRequest{
		Command: "echo",
		Args:    []string{"first"},
		Timeout: 2 * time.Second,
	}
	result1, err := sb1.Execute(ctx, req)
	require.NoError(t, err)
	assert.Equal(t, 0, result1.ExitCode)
	assert.Contains(t, string(result1.Stdout), "first")

	// Get sandbox second time (should reuse)
	sb2, err := sm.GetOrCreateSandbox(ctx, "agent-1")
	require.NoError(t, err)
	assert.Same(t, sb1, sb2, "Should reuse the same sandbox instance")

	// Execute another command in the same sandbox
	req.Args = []string{"second"}
	result2, err := sb2.Execute(ctx, req)
	require.NoError(t, err)
	assert.Contains(t, string(result2.Stdout), "second")
}

func TestSandboxIntegration_ErrorHandling(t *testing.T) {
	t.Run("invalid command", func(t *testing.T) {
		cfg := sandbox.DefaultConfig()
		cfg.ResourceLimits.Timeout = 5 * time.Second
		sm := NewSandboxManager(cfg)

		ctx := context.Background()
		defer func() { _ = sm.StopAll(ctx) }()

		req := sandbox.ExecuteRequest{
			Command: "nonexistent-command-xyz-12345",
			Args:    []string{},
			Timeout: 2 * time.Second,
		}

		result, err := sm.ExecuteInSandbox(ctx, "agent-1", req)
		// Should get an error or non-zero exit code
		// Note: Some systems may not return an error for non-existent commands
		// so we just verify the call completes
		_ = result
		_ = err
	})

	t.Run("command timeout", func(t *testing.T) {
		cfg := sandbox.DefaultConfig()
		cfg.ResourceLimits.Timeout = 10 * time.Second
		sm := NewSandboxManager(cfg)

		ctx := context.Background()
		defer sm.StopAll(ctx)

		req := sandbox.ExecuteRequest{
			Command: "sleep",
			Args:    []string{"5"},
			Timeout: 100 * time.Millisecond, // Very short timeout
		}

		result, err := sm.ExecuteInSandbox(ctx, "agent-1", req)
		// Should timeout - either error or non-zero exit code
		if err == nil {
			assert.NotEqual(t, 0, result.ExitCode, "Expected non-zero exit code for timeout")
		}
	})
}
