package sandbox

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHostSandbox_Integration_FullWorkflow(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Mode = ModeAll
	cfg.ResourceLimits.Timeout = 10 * time.Second

	sandbox, err := NewHostSandbox(cfg)
	require.NoError(t, err)

	ctx := context.Background()

	// Start sandbox
	err = sandbox.Start(ctx)
	require.NoError(t, err)
	defer func() { _ = sandbox.Stop(ctx) }()

	// Execute multiple commands
	commands := []struct {
		name    string
		command string
		args    []string
		check   func(t *testing.T, result ExecuteResult, err error)
	}{
		{
			name:    "echo",
			command: "echo",
			args:    []string{"test"},
			check: func(t *testing.T, result ExecuteResult, err error) {
				require.NoError(t, err)
				assert.Equal(t, 0, result.ExitCode)
				assert.Contains(t, string(result.Stdout), "test")
			},
		},
		{
			name:    "pwd",
			command: "pwd",
			args:    []string{},
			check: func(t *testing.T, result ExecuteResult, err error) {
				require.NoError(t, err)
				assert.Equal(t, 0, result.ExitCode)
				assert.NotEmpty(t, result.Stdout)
			},
		},
		{
			name:    "env",
			command: "env",
			args:    []string{},
			check: func(t *testing.T, result ExecuteResult, err error) {
				require.NoError(t, err)
				assert.Equal(t, 0, result.ExitCode)
				assert.Contains(t, string(result.Stdout), "PATH")
			},
		},
	}

	for _, cmd := range commands {
		t.Run(cmd.name, func(t *testing.T) {
			req := ExecuteRequest{
				Command: cmd.command,
				Args:    cmd.args,
				Timeout: 5 * time.Second,
			}

			result, err := sandbox.Execute(ctx, req)
			cmd.check(t, result, err)
		})
	}
}

func TestHostSandbox_Integration_FilesystemAccess(t *testing.T) {
	// Create temp directory for testing
	tmpDir := t.TempDir()

	cfg := DefaultConfig()
	cfg.FilesystemAccess.AllowedPaths = []string{tmpDir}
	cfg.FilesystemAccess.DeniedPaths = []string{"/etc"}

	sandbox, err := NewHostSandbox(cfg)
	require.NoError(t, err)

	ctx := context.Background()
	err = sandbox.Start(ctx)
	require.NoError(t, err)
	defer func() { _ = sandbox.Stop(ctx) }()

	// Test allowed path
	req := ExecuteRequest{
		Command:    "ls",
		Args:       []string{},
		WorkingDir: tmpDir,
		Timeout:    5 * time.Second,
	}

	result, err := sandbox.Execute(ctx, req)
	require.NoError(t, err)
	assert.Equal(t, 0, result.ExitCode)

	// Test denied path
	req = ExecuteRequest{
		Command:    "ls",
		Args:       []string{},
		WorkingDir: "/etc",
		Timeout:    5 * time.Second,
	}

	result, err = sandbox.Execute(ctx, req)
	assert.ErrorIs(t, err, ErrFilesystemAccessDenied)
}

func TestHostSandbox_Integration_ResourceLimits(t *testing.T) {
	cfg := DefaultConfig()
	cfg.ResourceLimits.Timeout = 200 * time.Millisecond

	sandbox, err := NewHostSandbox(cfg)
	require.NoError(t, err)

	ctx := context.Background()
	err = sandbox.Start(ctx)
	require.NoError(t, err)
	defer func() { _ = sandbox.Stop(ctx) }()

	// Test timeout
	req := ExecuteRequest{
		Command: "sleep",
		Args:    []string{"10"},
	}

	start := time.Now()
	result, err := sandbox.Execute(ctx, req)
	duration := time.Since(start)

	assert.ErrorIs(t, err, ErrExecutionTimeout)
	assert.Equal(t, -1, result.ExitCode)
	assert.Less(t, duration, 1*time.Second)
}

func TestHostSandbox_Integration_ComplexCommand(t *testing.T) {
	// Create a temp file
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	err := os.WriteFile(testFile, []byte("test content"), 0644)
	require.NoError(t, err)

	cfg := DefaultConfig()
	cfg.FilesystemAccess.AllowedPaths = []string{tmpDir}

	sandbox, err := NewHostSandbox(cfg)
	require.NoError(t, err)

	ctx := context.Background()
	err = sandbox.Start(ctx)
	require.NoError(t, err)
	defer func() { _ = sandbox.Stop(ctx) }()

	// Read the file using cat
	req := ExecuteRequest{
		Command:    "cat",
		Args:       []string{testFile},
		WorkingDir: tmpDir,
		Timeout:    5 * time.Second,
	}

	result, err := sandbox.Execute(ctx, req)

	require.NoError(t, err)
	assert.Equal(t, 0, result.ExitCode)
	assert.Equal(t, "test content", string(result.Stdout))
}
