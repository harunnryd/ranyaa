package tracing

import (
	"context"

	"github.com/rs/zerolog"
)

// PropagateToSubAgent propagates tracing context to a sub-agent
// It keeps the trace ID but generates a new run ID for the sub-agent
func PropagateToSubAgent(ctx context.Context, subAgentID string) context.Context {
	// Keep trace ID from parent
	traceID := GetTraceID(ctx)
	if traceID == "" {
		traceID = NewTraceID()
	}

	// Generate new run ID for sub-agent
	runID := NewRunID()

	// Create new context with propagated trace ID and new run ID
	newCtx := WithTraceID(ctx, traceID)
	newCtx = WithRunID(newCtx, runID)
	newCtx = WithAgentID(newCtx, subAgentID)

	// Propagate session key if present
	sessionKey := GetSessionKey(ctx)
	if sessionKey != "" {
		newCtx = WithSessionKey(newCtx, sessionKey)
	}

	return newCtx
}

// PropagateToLogger adds tracing context to a zerolog logger
func PropagateToLogger(ctx context.Context, logger zerolog.Logger) zerolog.Logger {
	tc := FromContext(ctx)

	if tc.TraceID != "" {
		logger = logger.With().Str("trace_id", tc.TraceID).Logger()
	}
	if tc.RunID != "" {
		logger = logger.With().Str("run_id", tc.RunID).Logger()
	}
	if tc.AgentID != "" {
		logger = logger.With().Str("agent_id", tc.AgentID).Logger()
	}
	if tc.SessionKey != "" {
		logger = logger.With().Str("session_key", tc.SessionKey).Logger()
	}

	return logger
}

// LoggerFromContext creates a logger with tracing context from the given context
func LoggerFromContext(ctx context.Context, baseLogger zerolog.Logger) zerolog.Logger {
	return PropagateToLogger(ctx, baseLogger)
}

// PropagateAcrossFunctions demonstrates how to propagate context across function calls
// This is a helper function showing the pattern
func PropagateAcrossFunctions(ctx context.Context) context.Context {
	// Context is automatically propagated when passed as first parameter
	// No special handling needed - just pass ctx to all functions
	return ctx
}

// MergeContext merges tracing information from source context into target context
// Useful when you need to combine contexts from different sources
func MergeContext(target, source context.Context) context.Context {
	tc := FromContext(source)

	if tc.TraceID != "" && GetTraceID(target) == "" {
		target = WithTraceID(target, tc.TraceID)
	}
	if tc.RunID != "" && GetRunID(target) == "" {
		target = WithRunID(target, tc.RunID)
	}
	if tc.AgentID != "" && GetAgentID(target) == "" {
		target = WithAgentID(target, tc.AgentID)
	}
	if tc.SessionKey != "" && GetSessionKey(target) == "" {
		target = WithSessionKey(target, tc.SessionKey)
	}

	return target
}

// CloneContext creates a new context with the same tracing information
func CloneContext(ctx context.Context) context.Context {
	tc := FromContext(ctx)
	return NewContext(context.Background(), tc)
}

// PropagationChain represents a chain of propagated contexts
type PropagationChain struct {
	contexts []context.Context
}

// NewPropagationChain creates a new propagation chain
func NewPropagationChain(rootCtx context.Context) *PropagationChain {
	return &PropagationChain{
		contexts: []context.Context{rootCtx},
	}
}

// AddSubAgent adds a sub-agent context to the chain
func (pc *PropagationChain) AddSubAgent(agentID string) context.Context {
	parent := pc.contexts[len(pc.contexts)-1]
	child := PropagateToSubAgent(parent, agentID)
	pc.contexts = append(pc.contexts, child)
	return child
}

// GetRoot returns the root context
func (pc *PropagationChain) GetRoot() context.Context {
	if len(pc.contexts) == 0 {
		return context.Background()
	}
	return pc.contexts[0]
}

// GetCurrent returns the current (most recent) context
func (pc *PropagationChain) GetCurrent() context.Context {
	if len(pc.contexts) == 0 {
		return context.Background()
	}
	return pc.contexts[len(pc.contexts)-1]
}

// Depth returns the depth of the propagation chain
func (pc *PropagationChain) Depth() int {
	return len(pc.contexts)
}
