package sandbox

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewHostSandbox(t *testing.T) {
	cfg := DefaultConfig()
	sandbox, err := NewHostSandbox(cfg)

	require.NoError(t, err)
	assert.NotNil(t, sandbox)
	assert.False(t, sandbox.IsRunning())
	assert.Equal(t, cfg, sandbox.GetConfig())
}

func TestNewHostSandbox_InvalidConfig(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Mode = Mode("invalid")

	sandbox, err := NewHostSandbox(cfg)

	assert.Error(t, err)
	assert.Nil(t, sandbox)
	assert.Contains(t, err.Error(), "invalid config")
}

func TestHostSandbox_StartStop(t *testing.T) {
	cfg := DefaultConfig()
	sandbox, err := NewHostSandbox(cfg)
	require.NoError(t, err)

	ctx := context.Background()

	// Start sandbox
	err = sandbox.Start(ctx)
	require.NoError(t, err)
	assert.True(t, sandbox.IsRunning())

	// Try to start again (should fail)
	err = sandbox.Start(ctx)
	assert.ErrorIs(t, err, ErrSandboxAlreadyRunning)

	// Stop sandbox
	err = sandbox.Stop(ctx)
	require.NoError(t, err)
	assert.False(t, sandbox.IsRunning())

	// Try to stop again (should fail)
	err = sandbox.Stop(ctx)
	assert.ErrorIs(t, err, ErrSandboxNotRunning)
}

func TestHostSandbox_Execute_SimpleCommand(t *testing.T) {
	cfg := DefaultConfig()
	sandbox, err := NewHostSandbox(cfg)
	require.NoError(t, err)

	ctx := context.Background()
	err = sandbox.Start(ctx)
	require.NoError(t, err)
	defer func() { _ = sandbox.Stop(ctx) }()

	req := ExecuteRequest{
		Command: "echo",
		Args:    []string{"hello", "world"},
		Timeout: 5 * time.Second,
	}

	result, err := sandbox.Execute(ctx, req)

	require.NoError(t, err)
	assert.Equal(t, 0, result.ExitCode)
	assert.Contains(t, string(result.Stdout), "hello world")
	assert.Empty(t, result.Stderr)
}

func TestHostSandbox_Execute_NotRunning(t *testing.T) {
	cfg := DefaultConfig()
	sandbox, err := NewHostSandbox(cfg)
	require.NoError(t, err)

	ctx := context.Background()
	req := ExecuteRequest{
		Command: "echo",
		Args:    []string{"test"},
	}

	result, err := sandbox.Execute(ctx, req)

	assert.ErrorIs(t, err, ErrSandboxNotRunning)
	assert.Equal(t, 0, result.ExitCode)
}

func TestHostSandbox_Execute_Timeout(t *testing.T) {
	cfg := DefaultConfig()
	sandbox, err := NewHostSandbox(cfg)
	require.NoError(t, err)

	ctx := context.Background()
	err = sandbox.Start(ctx)
	require.NoError(t, err)
	defer func() { _ = sandbox.Stop(ctx) }()

	req := ExecuteRequest{
		Command: "sleep",
		Args:    []string{"10"},
		Timeout: 100 * time.Millisecond,
	}

	result, err := sandbox.Execute(ctx, req)

	assert.ErrorIs(t, err, ErrExecutionTimeout)
	assert.Equal(t, -1, result.ExitCode)
}

func TestHostSandbox_Execute_NonZeroExit(t *testing.T) {
	cfg := DefaultConfig()
	sandbox, err := NewHostSandbox(cfg)
	require.NoError(t, err)

	ctx := context.Background()
	err = sandbox.Start(ctx)
	require.NoError(t, err)
	defer func() { _ = sandbox.Stop(ctx) }()

	req := ExecuteRequest{
		Command: "sh",
		Args:    []string{"-c", "exit 42"},
		Timeout: 5 * time.Second,
	}

	result, err := sandbox.Execute(ctx, req)

	require.NoError(t, err)
	assert.Equal(t, 42, result.ExitCode)
}

func TestHostSandbox_Execute_WithStdin(t *testing.T) {
	cfg := DefaultConfig()
	sandbox, err := NewHostSandbox(cfg)
	require.NoError(t, err)

	ctx := context.Background()
	err = sandbox.Start(ctx)
	require.NoError(t, err)
	defer func() { _ = sandbox.Stop(ctx) }()

	req := ExecuteRequest{
		Command: "cat",
		Args:    []string{},
		Stdin:   []byte("test input"),
		Timeout: 5 * time.Second,
	}

	result, err := sandbox.Execute(ctx, req)

	require.NoError(t, err)
	assert.Equal(t, 0, result.ExitCode)
	assert.Equal(t, "test input", string(result.Stdout))
}

func TestHostSandbox_Execute_WithEnv(t *testing.T) {
	cfg := DefaultConfig()
	sandbox, err := NewHostSandbox(cfg)
	require.NoError(t, err)

	ctx := context.Background()
	err = sandbox.Start(ctx)
	require.NoError(t, err)
	defer func() { _ = sandbox.Stop(ctx) }()

	req := ExecuteRequest{
		Command: "sh",
		Args:    []string{"-c", "echo $TEST_VAR"},
		Env: map[string]string{
			"TEST_VAR": "test_value",
		},
		Timeout: 5 * time.Second,
	}

	result, err := sandbox.Execute(ctx, req)

	require.NoError(t, err)
	assert.Equal(t, 0, result.ExitCode)
	assert.Contains(t, string(result.Stdout), "test_value")
}

func TestHostSandbox_CheckFilesystemAccess_Allowed(t *testing.T) {
	cfg := DefaultConfig()
	cfg.FilesystemAccess.AllowedPaths = []string{"/tmp", "/home"}
	cfg.FilesystemAccess.DeniedPaths = []string{"/etc"}

	sandbox, err := NewHostSandbox(cfg)
	require.NoError(t, err)

	// Test allowed paths
	err = sandbox.checkFilesystemAccess("/tmp/test")
	assert.NoError(t, err)

	err = sandbox.checkFilesystemAccess("/home/user")
	assert.NoError(t, err)
}

func TestHostSandbox_CheckFilesystemAccess_Denied(t *testing.T) {
	cfg := DefaultConfig()
	cfg.FilesystemAccess.AllowedPaths = []string{"/tmp"}
	cfg.FilesystemAccess.DeniedPaths = []string{"/etc", "/sys"}

	sandbox, err := NewHostSandbox(cfg)
	require.NoError(t, err)

	// Test denied paths
	err = sandbox.checkFilesystemAccess("/etc/passwd")
	assert.ErrorIs(t, err, ErrFilesystemAccessDenied)

	err = sandbox.checkFilesystemAccess("/sys/kernel")
	assert.ErrorIs(t, err, ErrFilesystemAccessDenied)

	// Test not in allowed paths
	err = sandbox.checkFilesystemAccess("/home/user")
	assert.ErrorIs(t, err, ErrFilesystemAccessDenied)
}

func TestHostSandbox_SetConfig(t *testing.T) {
	cfg := DefaultConfig()
	sandbox, err := NewHostSandbox(cfg)
	require.NoError(t, err)

	newCfg := DefaultConfig()
	newCfg.Mode = ModeAll

	err = sandbox.SetConfig(newCfg)
	require.NoError(t, err)

	assert.Equal(t, ModeAll, sandbox.GetConfig().Mode)
}

func TestHostSandbox_SetConfig_Invalid(t *testing.T) {
	cfg := DefaultConfig()
	sandbox, err := NewHostSandbox(cfg)
	require.NoError(t, err)

	invalidCfg := DefaultConfig()
	invalidCfg.Mode = Mode("invalid")

	err = sandbox.SetConfig(invalidCfg)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid config")
}
