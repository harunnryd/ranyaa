package toolexecutor

import (
	"context"
	"testing"
	"time"

	"github.com/harun/ranya/pkg/sandbox"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewSandboxManager(t *testing.T) {
	cfg := sandbox.DefaultConfig()
	sm := NewSandboxManager(cfg)

	assert.NotNil(t, sm)
	assert.NotNil(t, sm.sandboxes)
	assert.Equal(t, cfg, sm.config)
}

func TestSandboxManager_GetOrCreateSandbox(t *testing.T) {
	cfg := sandbox.DefaultConfig()
	cfg.ResourceLimits.Timeout = 5 * time.Second
	sm := NewSandboxManager(cfg)

	ctx := context.Background()

	// First call should create a new sandbox
	sb1, err := sm.GetOrCreateSandbox(ctx, "agent-1")
	require.NoError(t, err)
	assert.NotNil(t, sb1)
	assert.True(t, sb1.IsRunning())

	// Second call with same key should return the same sandbox
	sb2, err := sm.GetOrCreateSandbox(ctx, "agent-1")
	require.NoError(t, err)
	assert.Same(t, sb1, sb2, "Should return the same sandbox instance")

	// Different key should create a different sandbox
	sb3, err := sm.GetOrCreateSandbox(ctx, "agent-2")
	require.NoError(t, err)
	assert.NotNil(t, sb3)
	assert.NotSame(t, sb1, sb3, "Should create a different sandbox instance")

	// Cleanup
	_ = sm.StopAll(ctx)
}

func TestSandboxManager_GetOrCreateSandbox_DockerRuntime(t *testing.T) {
	cfg := sandbox.DefaultConfig()
	cfg.Runtime = sandbox.RuntimeDocker
	cfg.Docker.Image = "alpine:3.20"
	cfg.ResourceLimits.Timeout = 5 * time.Second
	sm := NewSandboxManager(cfg)

	ctx := context.Background()
	sb, err := sm.GetOrCreateSandbox(ctx, "docker-agent")
	require.NoError(t, err)
	require.NotNil(t, sb)
	assert.True(t, sb.IsRunning())
	assert.IsType(t, &sandbox.DockerSandbox{}, sb)

	_ = sm.StopAll(ctx)
}

func TestSandboxManager_StopSandbox(t *testing.T) {
	cfg := sandbox.DefaultConfig()
	sm := NewSandboxManager(cfg)

	ctx := context.Background()

	// Create a sandbox
	sb, err := sm.GetOrCreateSandbox(ctx, "agent-1")
	require.NoError(t, err)
	assert.True(t, sb.IsRunning())

	// Stop the sandbox
	err = sm.StopSandbox(ctx, "agent-1")
	require.NoError(t, err)

	// Sandbox should no longer be in the map
	assert.Len(t, sm.sandboxes, 0)

	// Stopping non-existent sandbox should not error
	err = sm.StopSandbox(ctx, "non-existent")
	assert.NoError(t, err)
}

func TestSandboxManager_StopAll(t *testing.T) {
	cfg := sandbox.DefaultConfig()
	sm := NewSandboxManager(cfg)

	ctx := context.Background()

	// Create multiple sandboxes
	_, err := sm.GetOrCreateSandbox(ctx, "agent-1")
	require.NoError(t, err)
	_, err = sm.GetOrCreateSandbox(ctx, "agent-2")
	require.NoError(t, err)
	_, err = sm.GetOrCreateSandbox(ctx, "session-1")
	require.NoError(t, err)

	assert.Len(t, sm.sandboxes, 3)

	// Stop all sandboxes
	err = sm.StopAll(ctx)
	assert.NoError(t, err)

	// All sandboxes should be removed
	assert.Len(t, sm.sandboxes, 0)
}

func TestSandboxManager_ExecuteInSandbox(t *testing.T) {
	cfg := sandbox.DefaultConfig()
	cfg.ResourceLimits.Timeout = 5 * time.Second
	sm := NewSandboxManager(cfg)

	ctx := context.Background()
	defer func() { _ = sm.StopAll(ctx) }()

	req := sandbox.ExecuteRequest{
		Command: "echo",
		Args:    []string{"hello"},
		Timeout: 2 * time.Second,
	}

	result, err := sm.ExecuteInSandbox(ctx, "agent-1", req)
	require.NoError(t, err)
	assert.Equal(t, 0, result.ExitCode)
	assert.Contains(t, string(result.Stdout), "hello")
}

func TestCreateSandboxConfig(t *testing.T) {
	t.Run("nil sandbox policy", func(t *testing.T) {
		execCtx := &ExecutionContext{
			SandboxPolicy: nil,
		}

		cfg := CreateSandboxConfig(execCtx)
		assert.Equal(t, sandbox.DefaultConfig(), cfg)
	})

	t.Run("with sandbox policy", func(t *testing.T) {
		execCtx := &ExecutionContext{
			SandboxPolicy: map[string]interface{}{
				"mode":  "all",
				"scope": "agent",
			},
			WorkingDir: "/tmp/test",
			Timeout:    10 * time.Second,
		}

		cfg := CreateSandboxConfig(execCtx)
		assert.Equal(t, sandbox.Mode("all"), cfg.Mode)
		assert.Equal(t, sandbox.Scope("agent"), cfg.Scope)
		assert.Contains(t, cfg.FilesystemAccess.AllowedPaths, "/tmp/test")
		assert.Equal(t, 10*time.Second, cfg.ResourceLimits.Timeout)
	})

	t.Run("with partial policy", func(t *testing.T) {
		execCtx := &ExecutionContext{
			SandboxPolicy: map[string]interface{}{
				"mode": "tools",
			},
		}

		cfg := CreateSandboxConfig(execCtx)
		assert.Equal(t, sandbox.Mode("tools"), cfg.Mode)
	})
}

func TestSandboxExecuteCommand(t *testing.T) {
	cfg := sandbox.DefaultConfig()
	cfg.ResourceLimits.Timeout = 5 * time.Second
	sm := NewSandboxManager(cfg)

	ctx := context.Background()
	defer func() { _ = sm.StopAll(ctx) }()

	result, err := SandboxExecuteCommand(
		ctx,
		sm,
		"test-agent",
		"echo",
		[]string{"test"},
		"",
		2*time.Second,
	)

	require.NoError(t, err)
	assert.Equal(t, 0, result.ExitCode)
	assert.Contains(t, string(result.Stdout), "test")
}
