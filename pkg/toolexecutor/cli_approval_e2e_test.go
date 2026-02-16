package toolexecutor

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCLIApprovalHandler_E2E_RealWorldScenario(t *testing.T) {
	// Simulate a real-world scenario where an agent requests approval
	// for a potentially dangerous command

	// Scenario 1: User approves a safe command
	t.Run("approve safe command", func(t *testing.T) {
		reader := strings.NewReader("y\n")
		writer := &bytes.Buffer{}

		handler := NewCLIApprovalHandler(reader, writer)
		manager := NewApprovalManager(handler)

		req := ApprovalRequest{
			Command: "ls",
			Args:    []string{"-la", "/tmp"},
			Cwd:     "/home/user",
			AgentID: "executor-1",
			Timeout: 30 * time.Second,
			Context: map[string]string{
				"task": "list temporary files",
			},
		}

		ctx := context.Background()
		approved, err := manager.RequestApproval(ctx, req)

		require.NoError(t, err)
		assert.True(t, approved)

		// Verify user saw the request
		output := writer.String()
		assert.Contains(t, output, "EXEC APPROVAL REQUIRED")
		assert.Contains(t, output, "ls")
		assert.Contains(t, output, "/tmp")
		assert.Contains(t, output, "APPROVED")
	})

	// Scenario 2: User denies a dangerous command
	t.Run("deny dangerous command", func(t *testing.T) {
		reader := strings.NewReader("n\n")
		writer := &bytes.Buffer{}

		handler := NewCLIApprovalHandler(reader, writer)
		manager := NewApprovalManager(handler)

		req := ApprovalRequest{
			Command: "rm",
			Args:    []string{"-rf", "/"},
			Cwd:     "/",
			AgentID: "executor-1",
			Timeout: 30 * time.Second,
			Context: map[string]string{
				"task": "clean up system",
			},
		}

		ctx := context.Background()
		approved, err := manager.RequestApproval(ctx, req)

		require.NoError(t, err)
		assert.False(t, approved)

		// Verify user saw the request
		output := writer.String()
		assert.Contains(t, output, "EXEC APPROVAL REQUIRED")
		assert.Contains(t, output, "rm")
		assert.Contains(t, output, "-rf")
		assert.Contains(t, output, "DENIED")
	})

	// Scenario 3: Approval times out
	t.Run("timeout on no response", func(t *testing.T) {
		reader := &blockingReader{}
		writer := &bytes.Buffer{}

		handler := NewCLIApprovalHandler(reader, writer)
		manager := NewApprovalManager(handler)
		manager.SetDefaultTimeout(200 * time.Millisecond)

		req := ApprovalRequest{
			Command: "shutdown",
			Args:    []string{"-h", "now"},
			AgentID: "executor-1",
		}

		ctx := context.Background()
		approved, err := manager.RequestApproval(ctx, req)

		assert.Error(t, err)
		assert.False(t, approved)
		assert.Contains(t, err.Error(), "timed out")

		// Verify timeout message was displayed
		output := writer.String()
		assert.Contains(t, output, "TIMED OUT")
	})

	// Scenario 4: Multiple sequential approvals
	t.Run("multiple sequential approvals", func(t *testing.T) {
		commands := []struct {
			command  string
			args     []string
			input    string
			expected bool
		}{
			{"git", []string{"status"}, "y\n", true},
			{"git", []string{"add", "."}, "y\n", true},
			{"git", []string{"commit", "-m", "test"}, "y\n", true},
			{"git", []string{"push", "--force"}, "n\n", false},
		}

		for _, cmd := range commands {
			reader := strings.NewReader(cmd.input)
			writer := &bytes.Buffer{}

			handler := NewCLIApprovalHandler(reader, writer)
			manager := NewApprovalManager(handler)

			req := ApprovalRequest{
				Command: cmd.command,
				Args:    cmd.args,
				AgentID: "executor-1",
			}

			ctx := context.Background()
			approved, err := manager.RequestApproval(ctx, req)

			require.NoError(t, err)
			assert.Equal(t, cmd.expected, approved)
		}
	})

	// Scenario 5: Approval with rich context
	t.Run("approval with rich context", func(t *testing.T) {
		reader := strings.NewReader("y\n")
		writer := &bytes.Buffer{}

		handler := NewCLIApprovalHandler(reader, writer)
		manager := NewApprovalManager(handler)

		req := ApprovalRequest{
			Command: "docker",
			Args:    []string{"run", "-d", "-p", "8080:80", "nginx"},
			Cwd:     "/home/user/project",
			AgentID: "executor-1",
			Timeout: 60 * time.Second,
			Context: map[string]string{
				"task":        "deploy web server",
				"image":       "nginx:latest",
				"port":        "8080",
				"environment": "development",
			},
		}

		ctx := context.Background()
		approved, err := manager.RequestApproval(ctx, req)

		require.NoError(t, err)
		assert.True(t, approved)

		// Verify all context was displayed
		output := writer.String()
		assert.Contains(t, output, "docker")
		assert.Contains(t, output, "nginx")
		assert.Contains(t, output, "task")
		assert.Contains(t, output, "deploy web server")
		assert.Contains(t, output, "environment")
		assert.Contains(t, output, "development")
	})
}
