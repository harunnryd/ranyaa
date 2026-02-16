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

func TestNewCLIApprovalHandler(t *testing.T) {
	reader := strings.NewReader("")
	writer := &bytes.Buffer{}

	handler := NewCLIApprovalHandler(reader, writer)

	assert.NotNil(t, handler)
	assert.Equal(t, reader, handler.reader)
	assert.Equal(t, writer, handler.writer)
}

func TestCLIApprovalHandler_RequestApproval_Approve(t *testing.T) {
	reader := strings.NewReader("y\n")
	writer := &bytes.Buffer{}

	handler := NewCLIApprovalHandler(reader, writer)

	req := ApprovalRequest{
		Command: "ls",
		Args:    []string{"-la"},
		Cwd:     "/tmp",
		AgentID: "test-agent",
	}

	ctx := context.Background()
	response, err := handler.RequestApproval(ctx, req)

	require.NoError(t, err)
	assert.True(t, response.Approved)
	assert.Contains(t, response.Reason, "approved by user")

	// Check output
	output := writer.String()
	assert.Contains(t, output, "EXEC APPROVAL REQUIRED")
	assert.Contains(t, output, "ls")
	assert.Contains(t, output, "APPROVED")
}

func TestCLIApprovalHandler_RequestApproval_Deny(t *testing.T) {
	reader := strings.NewReader("n\n")
	writer := &bytes.Buffer{}

	handler := NewCLIApprovalHandler(reader, writer)

	req := ApprovalRequest{
		Command: "rm",
		Args:    []string{"-rf", "/"},
	}

	ctx := context.Background()
	response, err := handler.RequestApproval(ctx, req)

	require.NoError(t, err)
	assert.False(t, response.Approved)
	assert.Contains(t, response.Reason, "denied by user")

	// Check output
	output := writer.String()
	assert.Contains(t, output, "DENIED")
}

func TestCLIApprovalHandler_RequestApproval_Yes(t *testing.T) {
	reader := strings.NewReader("yes\n")
	writer := &bytes.Buffer{}

	handler := NewCLIApprovalHandler(reader, writer)

	req := ApprovalRequest{
		Command: "echo",
		Args:    []string{"hello"},
	}

	ctx := context.Background()
	response, err := handler.RequestApproval(ctx, req)

	require.NoError(t, err)
	assert.True(t, response.Approved)
	assert.Contains(t, response.Reason, "approved by user")
}

func TestCLIApprovalHandler_RequestApproval_No(t *testing.T) {
	reader := strings.NewReader("no\n")
	writer := &bytes.Buffer{}

	handler := NewCLIApprovalHandler(reader, writer)

	req := ApprovalRequest{
		Command: "shutdown",
	}

	ctx := context.Background()
	response, err := handler.RequestApproval(ctx, req)

	require.NoError(t, err)
	assert.False(t, response.Approved)
	assert.Contains(t, response.Reason, "denied by user")
}

func TestCLIApprovalHandler_RequestApproval_Empty(t *testing.T) {
	reader := strings.NewReader("\n")
	writer := &bytes.Buffer{}

	handler := NewCLIApprovalHandler(reader, writer)

	req := ApprovalRequest{
		Command: "test",
	}

	ctx := context.Background()
	response, err := handler.RequestApproval(ctx, req)

	require.NoError(t, err)
	assert.False(t, response.Approved)
	assert.Contains(t, response.Reason, "denied by user")
}

func TestCLIApprovalHandler_RequestApproval_InvalidInput(t *testing.T) {
	reader := strings.NewReader("maybe\n")
	writer := &bytes.Buffer{}

	handler := NewCLIApprovalHandler(reader, writer)

	req := ApprovalRequest{
		Command: "test",
	}

	ctx := context.Background()
	response, err := handler.RequestApproval(ctx, req)

	require.NoError(t, err)
	assert.False(t, response.Approved)
	assert.Contains(t, response.Reason, "invalid input")

	// Check output
	output := writer.String()
	assert.Contains(t, output, "Invalid input")
}

func TestCLIApprovalHandler_RequestApproval_Timeout(t *testing.T) {
	// Create a reader that never returns
	reader := &blockingReader{}
	writer := &bytes.Buffer{}

	handler := NewCLIApprovalHandler(reader, writer)

	req := ApprovalRequest{
		Command: "test",
		Timeout: 100 * time.Millisecond,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	response, err := handler.RequestApproval(ctx, req)

	assert.Error(t, err)
	assert.False(t, response.Approved)
	assert.Contains(t, response.Reason, "timeout")

	// Check output
	output := writer.String()
	assert.Contains(t, output, "TIMED OUT")
}

func TestCLIApprovalHandler_DisplayApprovalRequest(t *testing.T) {
	writer := &bytes.Buffer{}
	handler := &CLIApprovalHandler{
		writer: writer,
	}

	req := ApprovalRequest{
		Command: "git",
		Args:    []string{"push", "origin", "main"},
		Cwd:     "/home/user/project",
		AgentID: "executor-1",
		Timeout: 60 * time.Second,
		Context: map[string]string{
			"branch": "main",
			"remote": "origin",
		},
	}

	handler.displayApprovalRequest(req)

	output := writer.String()
	assert.Contains(t, output, "EXEC APPROVAL REQUIRED")
	assert.Contains(t, output, "git")
	assert.Contains(t, output, "push")
	assert.Contains(t, output, "/home/user/project")
	assert.Contains(t, output, "executor-1")
	assert.Contains(t, output, "1m0s")
	assert.Contains(t, output, "branch")
	assert.Contains(t, output, "main")
}

func TestCLIApprovalHandler_SetReaderWriter(t *testing.T) {
	handler := &CLIApprovalHandler{}

	reader := strings.NewReader("test")
	writer := &bytes.Buffer{}

	handler.SetReader(reader)
	handler.SetWriter(writer)

	assert.Equal(t, reader, handler.reader)
	assert.Equal(t, writer, handler.writer)
}

func TestCLIApprovalHandler_ReadUserInput_EOF(t *testing.T) {
	reader := strings.NewReader("")
	handler := &CLIApprovalHandler{
		reader: reader,
		writer: &bytes.Buffer{},
	}

	req := ApprovalRequest{
		Command: "test",
	}

	response, err := handler.readUserInput(req)

	require.NoError(t, err)
	assert.False(t, response.Approved)
	assert.Contains(t, response.Reason, "no input provided")
}

func TestCLIApprovalHandler_PromptWithTimeout(t *testing.T) {
	reader := strings.NewReader("y\n")
	writer := &bytes.Buffer{}

	handler := NewCLIApprovalHandler(reader, writer)

	req := ApprovalRequest{
		Command: "test",
	}

	ctx := context.Background()
	response, err := handler.PromptWithTimeout(ctx, req, 5*time.Second)

	require.NoError(t, err)
	assert.True(t, response.Approved)
}

// blockingReader is a reader that blocks forever (shared across test files)
type blockingReader struct{}

func (b *blockingReader) Read(p []byte) (n int, err error) {
	// Block forever
	select {}
}
