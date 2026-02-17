package daemon

import (
	"context"
	"fmt"

	"github.com/harun/ranya/internal/tracing"
	"github.com/harun/ranya/pkg/agent"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
)

// Router routes messages to appropriate agents
type Router struct {
	daemon *Daemon
}

// NewRouter creates a new message router
func NewRouter(d *Daemon) *Router {
	return &Router{
		daemon: d,
	}
}

// RouteMessage routes a message to the appropriate agent
func (r *Router) RouteMessage(ctx context.Context, msg Message) error {
	_, err := r.RouteMessageAndWait(ctx, msg)
	return err
}

// RouteMessageAndWait routes a message through the canonical runtime flow and waits for the result.
func (r *Router) RouteMessageAndWait(ctx context.Context, msg Message) (interface{}, error) {
	if tracing.GetTraceID(ctx) == "" {
		ctx = tracing.NewRequestContext(ctx)
	}
	if tracing.GetRunID(ctx) == "" {
		ctx = tracing.WithRunID(ctx, tracing.NewRunID())
	}
	ctx = tracing.WithSessionKey(ctx, msg.SessionKey)
	ctx, span := tracing.StartSpan(
		ctx,
		"ranya.daemon",
		"daemon.router.route_message",
		attribute.String("session_key", msg.SessionKey),
		attribute.String("source", msg.Source),
	)
	defer span.End()

	logger := tracing.LoggerFromContext(ctx, r.daemon.logger.GetZerolog())
	logger.Info().
		Str("session_key", msg.SessionKey).
		Str("source", msg.Source).
		Msg("Routing message")

	// Enqueue message for processing
	result, err := r.daemon.queue.EnqueueWithContext(ctx, msg.SessionKey, func(taskCtx context.Context) (interface{}, error) {
		return r.processMessage(taskCtx, msg)
	}, nil)

	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, fmt.Errorf("failed to enqueue message: %w", err)
	}

	return result, nil
}

// processMessage processes a message with the specified agent
func (r *Router) processMessage(ctx context.Context, msg Message) (interface{}, error) {
	logger := tracing.LoggerFromContext(ctx, r.daemon.logger.GetZerolog())
	logger.Info().
		Str("session_key", msg.SessionKey).
		Str("agent_id", msg.AgentID).
		Msg("Processing message")

	result, _, err := r.daemon.executeRuntimeFlow(ctx, RuntimeRequest{
		Prompt:     msg.Content,
		SessionKey: msg.SessionKey,
		Source:     msg.Source,
		AgentID:    msg.AgentID,
		RunConfig:  msg.RunConfig,
		CWD:        msg.CWD,
		Metadata:   msg.Metadata,
	})
	if err != nil {
		logger.Error().Err(err).Msg("Runtime flow execution failed")
		return nil, err
	}

	return result, nil
}

// Message represents a message to be routed
type Message struct {
	SessionKey string
	Source     string // telegram, gateway, etc.
	Content    string
	Metadata   map[string]interface{}
	AgentID    string
	RunConfig  agent.AgentConfig
	CWD        string
}
