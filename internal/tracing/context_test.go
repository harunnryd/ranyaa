package tracing

import (
	"context"
	"testing"
)

func TestNewTraceID(t *testing.T) {
	id1 := NewTraceID()
	id2 := NewTraceID()

	if id1 == "" {
		t.Error("NewTraceID returned empty string")
	}

	if id1 == id2 {
		t.Error("NewTraceID returned duplicate IDs")
	}
}

func TestNewRunID(t *testing.T) {
	id1 := NewRunID()
	id2 := NewRunID()

	if id1 == "" {
		t.Error("NewRunID returned empty string")
	}

	if id1 == id2 {
		t.Error("NewRunID returned duplicate IDs")
	}
}

func TestWithTraceID(t *testing.T) {
	ctx := context.Background()
	traceID := "test-trace-id"

	ctx = WithTraceID(ctx, traceID)

	retrieved := GetTraceID(ctx)
	if retrieved != traceID {
		t.Errorf("Expected trace ID %s, got %s", traceID, retrieved)
	}
}

func TestWithRunID(t *testing.T) {
	ctx := context.Background()
	runID := "test-run-id"

	ctx = WithRunID(ctx, runID)

	retrieved := GetRunID(ctx)
	if retrieved != runID {
		t.Errorf("Expected run ID %s, got %s", runID, retrieved)
	}
}

func TestWithAgentID(t *testing.T) {
	ctx := context.Background()
	agentID := "test-agent"

	ctx = WithAgentID(ctx, agentID)

	retrieved := GetAgentID(ctx)
	if retrieved != agentID {
		t.Errorf("Expected agent ID %s, got %s", agentID, retrieved)
	}
}

func TestWithSessionKey(t *testing.T) {
	ctx := context.Background()
	sessionKey := "test-session"

	ctx = WithSessionKey(ctx, sessionKey)

	retrieved := GetSessionKey(ctx)
	if retrieved != sessionKey {
		t.Errorf("Expected session key %s, got %s", sessionKey, retrieved)
	}
}

func TestGetTraceIDEmpty(t *testing.T) {
	ctx := context.Background()

	traceID := GetTraceID(ctx)
	if traceID != "" {
		t.Errorf("Expected empty trace ID, got %s", traceID)
	}
}

func TestGetRunIDEmpty(t *testing.T) {
	ctx := context.Background()

	runID := GetRunID(ctx)
	if runID != "" {
		t.Errorf("Expected empty run ID, got %s", runID)
	}
}

func TestGetAgentIDEmpty(t *testing.T) {
	ctx := context.Background()

	agentID := GetAgentID(ctx)
	if agentID != "" {
		t.Errorf("Expected empty agent ID, got %s", agentID)
	}
}

func TestGetSessionKeyEmpty(t *testing.T) {
	ctx := context.Background()

	sessionKey := GetSessionKey(ctx)
	if sessionKey != "" {
		t.Errorf("Expected empty session key, got %s", sessionKey)
	}
}

func TestFromContext(t *testing.T) {
	ctx := context.Background()
	ctx = WithTraceID(ctx, "trace-123")
	ctx = WithRunID(ctx, "run-456")
	ctx = WithAgentID(ctx, "agent-789")
	ctx = WithSessionKey(ctx, "session-abc")

	tc := FromContext(ctx)

	if tc.TraceID != "trace-123" {
		t.Errorf("Expected trace ID trace-123, got %s", tc.TraceID)
	}
	if tc.RunID != "run-456" {
		t.Errorf("Expected run ID run-456, got %s", tc.RunID)
	}
	if tc.AgentID != "agent-789" {
		t.Errorf("Expected agent ID agent-789, got %s", tc.AgentID)
	}
	if tc.SessionKey != "session-abc" {
		t.Errorf("Expected session key session-abc, got %s", tc.SessionKey)
	}
}

func TestNewContext(t *testing.T) {
	ctx := context.Background()

	tc := &TraceContext{
		TraceID:    "trace-123",
		RunID:      "run-456",
		AgentID:    "agent-789",
		SessionKey: "session-abc",
	}

	ctx = NewContext(ctx, tc)

	if GetTraceID(ctx) != "trace-123" {
		t.Error("Trace ID not set correctly")
	}
	if GetRunID(ctx) != "run-456" {
		t.Error("Run ID not set correctly")
	}
	if GetAgentID(ctx) != "agent-789" {
		t.Error("Agent ID not set correctly")
	}
	if GetSessionKey(ctx) != "session-abc" {
		t.Error("Session key not set correctly")
	}
}

func TestNewContextPartial(t *testing.T) {
	ctx := context.Background()

	tc := &TraceContext{
		TraceID: "trace-123",
		// Other fields empty
	}

	ctx = NewContext(ctx, tc)

	if GetTraceID(ctx) != "trace-123" {
		t.Error("Trace ID not set correctly")
	}
	if GetRunID(ctx) != "" {
		t.Error("Run ID should be empty")
	}
	if GetAgentID(ctx) != "" {
		t.Error("Agent ID should be empty")
	}
	if GetSessionKey(ctx) != "" {
		t.Error("Session key should be empty")
	}
}

func TestNewRequestContext(t *testing.T) {
	ctx := context.Background()

	ctx = NewRequestContext(ctx)

	traceID := GetTraceID(ctx)
	if traceID == "" {
		t.Error("Trace ID not generated")
	}

	// Verify it's a valid UUID format
	if len(traceID) != 36 {
		t.Errorf("Expected UUID format (36 chars), got %d chars", len(traceID))
	}
}

func TestNewAgentRunContext(t *testing.T) {
	ctx := context.Background()
	agentID := "test-agent"

	ctx = NewAgentRunContext(ctx, agentID)

	runID := GetRunID(ctx)
	if runID == "" {
		t.Error("Run ID not generated")
	}

	retrievedAgentID := GetAgentID(ctx)
	if retrievedAgentID != agentID {
		t.Errorf("Expected agent ID %s, got %s", agentID, retrievedAgentID)
	}

	// Verify it's a valid UUID format
	if len(runID) != 36 {
		t.Errorf("Expected UUID format (36 chars), got %d chars", len(runID))
	}
}

func TestContextPropagation(t *testing.T) {
	// Create parent context with tracing
	parentCtx := context.Background()
	parentCtx = WithTraceID(parentCtx, "trace-parent")
	parentCtx = WithRunID(parentCtx, "run-parent")

	// Create child context (simulating sub-agent)
	childCtx := context.Background()

	// Propagate trace ID but create new run ID
	childCtx = WithTraceID(childCtx, GetTraceID(parentCtx))
	childCtx = WithRunID(childCtx, NewRunID())
	childCtx = WithAgentID(childCtx, "child-agent")

	// Verify trace ID is propagated
	if GetTraceID(childCtx) != "trace-parent" {
		t.Error("Trace ID not propagated to child context")
	}

	// Verify run ID is different
	if GetRunID(childCtx) == "run-parent" {
		t.Error("Run ID should be different for child context")
	}

	// Verify agent ID is set
	if GetAgentID(childCtx) != "child-agent" {
		t.Error("Agent ID not set correctly")
	}
}
