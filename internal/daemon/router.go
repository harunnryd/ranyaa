package daemon

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/harun/ranya/internal/tracing"
	"github.com/harun/ranya/pkg/agent"
	"github.com/harun/ranya/pkg/routing"
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
	agentID, err := r.resolveAgentID(ctx, msg)
	if err != nil {
		return nil, err
	}

	logger := tracing.LoggerFromContext(ctx, r.daemon.logger.GetZerolog())
	logger.Info().
		Str("session_key", msg.SessionKey).
		Str("agent_id", agentID).
		Msg("Processing message")

	if err := r.daemon.maybeResetSession(ctx, msg.SessionKey); err != nil {
		logger.Warn().Err(err).Msg("Failed to apply session reset policy")
	}

	// Extract or generate RequestID for idempotency
	requestID := ""
	if msg.Metadata != nil {
		if rid, ok := msg.Metadata["request_id"].(string); ok {
			requestID = strings.TrimSpace(rid)
		}
	}
	if requestID == "" {
		requestID = tracing.NewTraceID()
	}
	ctx = tracing.WithRequestID(ctx, requestID)

	result, _, err := r.daemon.executeRuntimeFlow(ctx, RuntimeRequest{
		Prompt:     msg.Content,
		SessionKey: msg.SessionKey,
		Source:     msg.Source,
		AgentID:    agentID,
		RunConfig:  msg.RunConfig,
		CWD:        msg.CWD,
		Metadata:   msg.Metadata,
		RequestID:  requestID,
	})
	if err != nil {
		logger.Error().Err(err).Msg("Runtime flow execution failed")
		return nil, err
	}

	return result, nil
}

func (r *Router) resolveAgentID(ctx context.Context, msg Message) (string, error) {
	if strings.TrimSpace(msg.AgentID) != "" || r.daemon.routingService == nil {
		return msg.AgentID, nil
	}

	routingCtx := routing.RoutingContext{
		MessageID: msg.SessionKey,
		Content:   msg.Content,
		Metadata:  msg.Metadata,
		Timestamp: time.Now(),
		Source:    msg.Source,
		Channel:   msg.Source,
		PeerID:    routingMetadataValue(msg.Metadata, "peer", "peer_id", "peerId", "chat_id", "chatId"),
		GuildID:   routingMetadataValue(msg.Metadata, "guild_id", "guildId"),
		TeamID:    routingMetadataValue(msg.Metadata, "team_id", "teamId"),
		AccountID: routingMetadataValue(msg.Metadata, "account_id", "accountId", "account"),
	}

	route, err := r.daemon.routingService.ResolveRoute(routingCtx)
	if err != nil {
		return "", fmt.Errorf("failed to resolve routing target: %w", err)
	}
	if route == nil || route.Handler == "" {
		return msg.AgentID, nil
	}

	logger := tracing.LoggerFromContext(ctx, r.daemon.logger.GetZerolog())
	logger.Info().
		Str("route_id", route.ID).
		Str("handler", route.Handler).
		Msg("Routing service resolved agent handler")

	return route.Handler, nil
}

func routingMetadataValue(metadata map[string]interface{}, keys ...string) string {
	if metadata == nil {
		return ""
	}
	for _, key := range keys {
		raw, ok := metadata[key]
		if !ok {
			continue
		}
		if value, ok := raw.(string); ok {
			trimmed := strings.TrimSpace(value)
			if trimmed != "" {
				return trimmed
			}
		}
	}
	return ""
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
