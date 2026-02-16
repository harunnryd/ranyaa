package tracing

import (
	"bytes"
	"context"
	"testing"

	"github.com/rs/zerolog"
)

func TestPropagateToSubAgent(t *testing.T) {
	// Create parent context
	parentCtx := context.Background()
	parentCtx = WithTraceID(parentCtx, "trace-123")
	parentCtx = WithRunID(parentCtx, "run-parent")
	parentCtx = WithAgentID(parentCtx, "parent-agent")
	parentCtx = WithSessionKey(parentCtx, "session-abc")

	// Propagate to sub-agent
	childCtx := PropagateToSubAgent(parentCtx, "child-agent")

	// Verify trace ID is propagated
	if GetTraceID(childCtx) != "trace-123" {
		t.Error("Trace ID not propagated")
	}

	// Verify run ID is different
	if GetRunID(childCtx) == "run-parent" {
		t.Error("Run ID should be different for sub-agent")
	}
	if GetRunID(childCtx) == "" {
		t.Error("Run ID not generated for sub-agent")
	}

	// Verify agent ID is updated
	if GetAgentID(childCtx) != "child-agent" {
		t.Error("Agent ID not updated")
	}

	// Verify session key is propagated
	if GetSessionKey(childCtx) != "session-abc" {
		t.Error("Session key not propagated")
	}
}

func TestPropagateToSubAgentNoTraceID(t *testing.T) {
	// Create parent context without trace ID
	parentCtx := context.Background()

	// Propagate to sub-agent
	childCtx := PropagateToSubAgent(parentCtx, "child-agent")

	// Verify trace ID is generated
	if GetTraceID(childCtx) == "" {
		t.Error("Trace ID not generated when missing")
	}

	// Verify run ID is generated
	if GetRunID(childCtx) == "" {
		t.Error("Run ID not generated")
	}

	// Verify agent ID is set
	if GetAgentID(childCtx) != "child-agent" {
		t.Error("Agent ID not set")
	}
}

func TestPropagateToLogger(t *testing.T) {
	// Create context with tracing
	ctx := context.Background()
	ctx = WithTraceID(ctx, "trace-123")
	ctx = WithRunID(ctx, "run-456")
	ctx = WithAgentID(ctx, "agent-789")
	ctx = WithSessionKey(ctx, "session-abc")

	// Create logger
	var buf bytes.Buffer
	baseLogger := zerolog.New(&buf)

	// Propagate to logger
	logger := PropagateToLogger(ctx, baseLogger)

	// Log a message
	logger.Info().Msg("test message")

	// Verify tracing fields are in log output
	output := buf.String()

	if !contains(output, "trace-123") {
		t.Error("Trace ID not in log output")
	}
	if !contains(output, "run-456") {
		t.Error("Run ID not in log output")
	}
	if !contains(output, "agent-789") {
		t.Error("Agent ID not in log output")
	}
	if !contains(output, "session-abc") {
		t.Error("Session key not in log output")
	}
}

func TestLoggerFromContext(t *testing.T) {
	// Create context with tracing
	ctx := context.Background()
	ctx = WithTraceID(ctx, "trace-xyz")

	// Create logger
	var buf bytes.Buffer
	baseLogger := zerolog.New(&buf)

	// Get logger from context
	logger := LoggerFromContext(ctx, baseLogger)

	// Log a message
	logger.Info().Msg("test")

	// Verify trace ID is in output
	output := buf.String()
	if !contains(output, "trace-xyz") {
		t.Error("Trace ID not in log output")
	}
}

func TestMergeContext(t *testing.T) {
	// Create source context with tracing
	sourceCtx := context.Background()
	sourceCtx = WithTraceID(sourceCtx, "trace-source")
	sourceCtx = WithRunID(sourceCtx, "run-source")

	// Create target context (empty)
	targetCtx := context.Background()

	// Merge contexts
	mergedCtx := MergeContext(targetCtx, sourceCtx)

	// Verify tracing info is merged
	if GetTraceID(mergedCtx) != "trace-source" {
		t.Error("Trace ID not merged")
	}
	if GetRunID(mergedCtx) != "run-source" {
		t.Error("Run ID not merged")
	}
}

func TestMergeContextNoOverwrite(t *testing.T) {
	// Create source context
	sourceCtx := context.Background()
	sourceCtx = WithTraceID(sourceCtx, "trace-source")

	// Create target context with existing trace ID
	targetCtx := context.Background()
	targetCtx = WithTraceID(targetCtx, "trace-target")

	// Merge contexts
	mergedCtx := MergeContext(targetCtx, sourceCtx)

	// Verify target trace ID is not overwritten
	if GetTraceID(mergedCtx) != "trace-target" {
		t.Error("Trace ID should not be overwritten")
	}
}

func TestCloneContext(t *testing.T) {
	// Create original context
	originalCtx := context.Background()
	originalCtx = WithTraceID(originalCtx, "trace-123")
	originalCtx = WithRunID(originalCtx, "run-456")
	originalCtx = WithAgentID(originalCtx, "agent-789")

	// Clone context
	clonedCtx := CloneContext(originalCtx)

	// Verify tracing info is cloned
	if GetTraceID(clonedCtx) != "trace-123" {
		t.Error("Trace ID not cloned")
	}
	if GetRunID(clonedCtx) != "run-456" {
		t.Error("Run ID not cloned")
	}
	if GetAgentID(clonedCtx) != "agent-789" {
		t.Error("Agent ID not cloned")
	}
}

func TestPropagationChain(t *testing.T) {
	// Create root context
	rootCtx := context.Background()
	rootCtx = WithTraceID(rootCtx, "trace-root")

	// Create propagation chain
	chain := NewPropagationChain(rootCtx)

	if chain.Depth() != 1 {
		t.Errorf("Expected depth 1, got %d", chain.Depth())
	}

	// Add first sub-agent
	ctx1 := chain.AddSubAgent("agent-1")
	if chain.Depth() != 2 {
		t.Errorf("Expected depth 2, got %d", chain.Depth())
	}

	// Verify trace ID is propagated
	if GetTraceID(ctx1) != "trace-root" {
		t.Error("Trace ID not propagated in chain")
	}
	if GetAgentID(ctx1) != "agent-1" {
		t.Error("Agent ID not set correctly")
	}

	// Add second sub-agent
	ctx2 := chain.AddSubAgent("agent-2")
	if chain.Depth() != 3 {
		t.Errorf("Expected depth 3, got %d", chain.Depth())
	}

	// Verify trace ID is still propagated
	if GetTraceID(ctx2) != "trace-root" {
		t.Error("Trace ID not propagated through chain")
	}
	if GetAgentID(ctx2) != "agent-2" {
		t.Error("Agent ID not set correctly")
	}

	// Verify run IDs are different
	if GetRunID(ctx1) == GetRunID(ctx2) {
		t.Error("Run IDs should be different for each agent")
	}

	// Test GetRoot
	if GetTraceID(chain.GetRoot()) != "trace-root" {
		t.Error("GetRoot returned wrong context")
	}

	// Test GetCurrent
	if GetAgentID(chain.GetCurrent()) != "agent-2" {
		t.Error("GetCurrent returned wrong context")
	}
}

func TestPropagationChainEmpty(t *testing.T) {
	chain := &PropagationChain{}

	if chain.Depth() != 0 {
		t.Errorf("Expected depth 0, got %d", chain.Depth())
	}

	root := chain.GetRoot()
	if root == nil {
		t.Error("GetRoot returned nil")
	}

	current := chain.GetCurrent()
	if current == nil {
		t.Error("GetCurrent returned nil")
	}
}

// Helper function to check if string contains substring
func contains(s, substr string) bool {
	return bytes.Contains([]byte(s), []byte(substr))
}
