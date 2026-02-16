package toolexecutor

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/harun/ranya/pkg/sandbox"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSandboxE2E_RealToolExecution(t *testing.T) {
	// Create tool executor
	te := New()

	// Register a simple echo tool
	err := te.RegisterTool(ToolDefinition{
		Name:        "echo",
		Description: "Echo a message",
		Parameters: []ToolParameter{
			{
				Name:        "message",
				Type:        "string",
				Description: "Message to echo",
				Required:    true,
			},
		},
		Handler: func(ctx context.Context, params map[string]interface{}) (interface{}, error) {
			message := params["message"].(string)
			return map[string]string{"output": message}, nil
		},
	})
	require.NoError(t, err)

	// Create sandbox manager
	cfg := sandbox.DefaultConfig()
	cfg.ResourceLimits.Timeout = 10 * time.Second
	sm := NewSandboxManager(cfg)
	defer sm.StopAll(context.Background())

	// Create execution context with sandbox enabled
	execCtx := &ExecutionContext{
		AgentID:    "test-agent",
		SessionKey: "test-session",
		SandboxPolicy: map[string]interface{}{
			"mode":  "all",
			"scope": "agent",
		},
		Timeout: 5 * time.Second,
	}

	// Execute tool (currently executes normally, but logs sandbox info)
	ctx := context.Background()
	result := ExecuteWithSandbox(ctx, te, sm, "echo", map[string]interface{}{
		"message": "Hello from sandbox",
	}, execCtx)

	assert.True(t, result.Success)
	assert.NotNil(t, result.Output)
}

func TestSandboxE2E_FilesystemRestrictions(t *testing.T) {
	// Create a temporary directory for testing
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	err := os.WriteFile(testFile, []byte("test content"), 0644)
	require.NoError(t, err)

	// Create sandbox with restricted filesystem access
	cfg := sandbox.DefaultConfig()
	cfg.ResourceLimits.Timeout = 10 * time.Second
	cfg.FilesystemAccess.AllowedPaths = []string{tmpDir}
	cfg.FilesystemAccess.DeniedPaths = []string{"/etc", "/usr"}

	sm := NewSandboxManager(cfg)
	defer sm.StopAll(context.Background())

	ctx := context.Background()

	t.Run("allowed path access", func(t *testing.T) {
		// Should be able to read from allowed path
		req := sandbox.ExecuteRequest{
			Command: "cat",
			Args:    []string{testFile},
			Timeout: 2 * time.Second,
		}

		result, err := sm.ExecuteInSandbox(ctx, "agent-1", req)
		require.NoError(t, err)
		assert.Equal(t, 0, result.ExitCode)
		assert.Contains(t, string(result.Stdout), "test content")
	})

	t.Run("denied path access", func(t *testing.T) {
		// Note: Host sandbox only checks working directory for filesystem access
		// It doesn't prevent commands from accessing arbitrary paths
		// This test verifies that denied working directories are blocked
		req := sandbox.ExecuteRequest{
			Command:    "echo",
			Args:       []string{"test"},
			WorkingDir: "/etc", // Denied path
			Timeout:    2 * time.Second,
		}

		_, err := sm.ExecuteInSandbox(ctx, "agent-1", req)
		// Should error because working directory is denied
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "filesystem access denied")
	})
}
