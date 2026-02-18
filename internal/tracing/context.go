package tracing

import (
	"context"

	"github.com/google/uuid"
)

// ContextKey is the type for context keys
type ContextKey string

const (
	// TraceIDKey is the context key for trace ID
	TraceIDKey ContextKey = "trace_id"
	// RunIDKey is the context key for run ID
	RunIDKey ContextKey = "run_id"
	// AgentIDKey is the context key for agent ID
	AgentIDKey ContextKey = "agent_id"
	// SessionKeyKey is the context key for session key
	SessionKeyKey ContextKey = "session_key"
	// RequestIDKey is the context key for request ID (for idempotency)
	RequestIDKey ContextKey = "request_id"
)

// TraceContext holds tracing information
type TraceContext struct {
	TraceID    string
	RunID      string
	AgentID    string
	SessionKey string
	RequestID  string
}

// NewTraceID generates a new trace ID
func NewTraceID() string {
	return uuid.New().String()
}

// NewRunID generates a new run ID
func NewRunID() string {
	return uuid.New().String()
}

// WithTraceID adds a trace ID to the context
func WithTraceID(ctx context.Context, traceID string) context.Context {
	return context.WithValue(ctx, TraceIDKey, traceID)
}

// WithRunID adds a run ID to the context
func WithRunID(ctx context.Context, runID string) context.Context {
	return context.WithValue(ctx, RunIDKey, runID)
}

// WithAgentID adds an agent ID to the context
func WithAgentID(ctx context.Context, agentID string) context.Context {
	return context.WithValue(ctx, AgentIDKey, agentID)
}

// WithSessionKey adds a session key to the context
func WithSessionKey(ctx context.Context, sessionKey string) context.Context {
	return context.WithValue(ctx, SessionKeyKey, sessionKey)
}

// WithRequestID adds a request ID to the context for idempotency
func WithRequestID(ctx context.Context, requestID string) context.Context {
	return context.WithValue(ctx, RequestIDKey, requestID)
}

// GetTraceID retrieves the trace ID from the context
func GetTraceID(ctx context.Context) string {
	if traceID, ok := ctx.Value(TraceIDKey).(string); ok {
		return traceID
	}
	return ""
}

// GetRunID retrieves the run ID from the context
func GetRunID(ctx context.Context) string {
	if runID, ok := ctx.Value(RunIDKey).(string); ok {
		return runID
	}
	return ""
}

// GetAgentID retrieves the agent ID from the context
func GetAgentID(ctx context.Context) string {
	if agentID, ok := ctx.Value(AgentIDKey).(string); ok {
		return agentID
	}
	return ""
}

// GetSessionKey retrieves the session key from the context
func GetSessionKey(ctx context.Context) string {
	if sessionKey, ok := ctx.Value(SessionKeyKey).(string); ok {
		return sessionKey
	}
	return ""
}

// GetRequestID retrieves the request ID from the context
func GetRequestID(ctx context.Context) string {
	if requestID, ok := ctx.Value(RequestIDKey).(string); ok {
		return requestID
	}
	return ""
}

// FromContext extracts all tracing information from the context
func FromContext(ctx context.Context) *TraceContext {
	return &TraceContext{
		TraceID:    GetTraceID(ctx),
		RunID:      GetRunID(ctx),
		AgentID:    GetAgentID(ctx),
		SessionKey: GetSessionKey(ctx),
		RequestID:  GetRequestID(ctx),
	}
}

// NewContext creates a new context with tracing information
func NewContext(ctx context.Context, tc *TraceContext) context.Context {
	if tc.TraceID != "" {
		ctx = WithTraceID(ctx, tc.TraceID)
	}
	if tc.RunID != "" {
		ctx = WithRunID(ctx, tc.RunID)
	}
	if tc.AgentID != "" {
		ctx = WithAgentID(ctx, tc.AgentID)
	}
	if tc.SessionKey != "" {
		ctx = WithSessionKey(ctx, tc.SessionKey)
	}
	if tc.RequestID != "" {
		ctx = WithRequestID(ctx, tc.RequestID)
	}
	return ctx
}

// NewRequestContext creates a new context for a request with a new trace ID
func NewRequestContext(ctx context.Context) context.Context {
	traceID := NewTraceID()
	return WithTraceID(ctx, traceID)
}

// NewAgentRunContext creates a new context for an agent run with a new run ID
func NewAgentRunContext(ctx context.Context, agentID string) context.Context {
	runID := NewRunID()
	ctx = WithRunID(ctx, runID)
	ctx = WithAgentID(ctx, agentID)
	return ctx
}
