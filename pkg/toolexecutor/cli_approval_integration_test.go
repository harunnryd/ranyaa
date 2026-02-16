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

func TestCLIApprovalHandler_Integration_FullWorkflow(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
		reason   string
	}{
		{
			name:     "approve with y",
			input:    "y\n",
			expected: true,
			reason:   "approved by user",
		},
		{
			name:     "approve with yes",
			input:    "yes\n",
			expected: true,
			reason:   "approved by user",
		},
		{
			name:     "deny with n",
			input:    "n\n",
			expected: false,
			reason:   "denied by user",
		},
		{
			name:     "deny with no",
			input:    "no\n",
			expected: false,
			reason:   "denied by user",
		},
		{
			name:     "deny with empty",
			input:    "\n",
			expected: false,
			reason:   "denied by user",
		},
		{
			name:     "deny with invalid",
			input:    "invalid\n",
			expected: false,
			reason:   "invalid input",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := strings.NewReader(tt.input)
			writer := &bytes.Buffer{}

			handler := NewCLIApprovalHandler(reader, writer)

			req := ApprovalRequest{
				Command: "test",
				Args:    []string{"arg1", "arg2"},
				Cwd:     "/tmp",
				AgentID: "test-agent",
			}

			ctx := context.Background()
			response, err := handler.RequestApproval(ctx, req)

			require.NoError(t, err)
			assert.Equal(t, tt.expected, response.Approved)
			assert.Contains(t, response.Reason, tt.reason)

			// Verify output contains expected elements
			output := writer.String()
			assert.Contains(t, output, "EXEC APPROVAL REQUIRED")
			assert.Contains(t, output, "test")
		})
	}
}

func TestCLIApprovalHandler_Integration_WithContext(t *testing.T) {
	reader := strings.NewReader("y\n")
	writer := &bytes.Buffer{}

	handler := NewCLIApprovalHandler(reader, writer)

	req := ApprovalRequest{
		Command: "git",
		Args:    []string{"push", "origin", "main"},
		Cwd:     "/home/user/project",
		AgentID: "executor-1",
		Timeout: 60 * time.Second,
		Context: map[string]string{
			"branch": "main",
			"remote": "origin",
			"force":  "false",
		},
	}

	ctx := context.Background()
	response, err := handler.RequestApproval(ctx, req)

	require.NoError(t, err)
	assert.True(t, response.Approved)

	// Verify all context is displayed
	output := writer.String()
	assert.Contains(t, output, "git")
	assert.Contains(t, output, "push")
	assert.Contains(t, output, "/home/user/project")
	assert.Contains(t, output, "executor-1")
	assert.Contains(t, output, "branch")
	assert.Contains(t, output, "main")
	assert.Contains(t, output, "remote")
	assert.Contains(t, output, "origin")
}

func TestCLIApprovalHandler_Integration_TimeoutBehavior(t *testing.T) {
	// Create a blocking reader
	reader := &blockingReader{}
	writer := &bytes.Buffer{}

	handler := NewCLIApprovalHandler(reader, writer)

	req := ApprovalRequest{
		Command: "dangerous-command",
		Timeout: 200 * time.Millisecond,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	start := time.Now()
	response, err := handler.RequestApproval(ctx, req)
	duration := time.Since(start)

	assert.Error(t, err)
	assert.False(t, response.Approved)
	assert.Contains(t, response.Reason, "timeout")

	// Verify timeout occurred within reasonable time
	assert.Less(t, duration, 500*time.Millisecond)

	// Verify timeout message was displayed
	output := writer.String()
	assert.Contains(t, output, "TIMED OUT")
}

func TestCLIApprovalHandler_Integration_MultipleRequests(t *testing.T) {
	requests := []struct {
		command  string
		input    string
		expected bool
	}{
		{"ls", "y\n", true},
		{"rm", "n\n", false},
		{"echo", "yes\n", true},
		{"shutdown", "no\n", false},
	}

	for _, req := range requests {
		reader := strings.NewReader(req.input)
		writer := &bytes.Buffer{}

		handler := NewCLIApprovalHandler(reader, writer)

		approvalReq := ApprovalRequest{
			Command: req.command,
		}

		ctx := context.Background()
		response, err := handler.RequestApproval(ctx, approvalReq)

		require.NoError(t, err)
		assert.Equal(t, req.expected, response.Approved)
	}
}
