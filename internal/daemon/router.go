package daemon

import (
	"context"
	"fmt"

	"github.com/harun/ranya/internal/config"
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
	r.daemon.logger.Info().
		Str("session_key", msg.SessionKey).
		Str("source", msg.Source).
		Msg("Routing message")

	// Determine which agent should handle this message
	// For now, we use the default agent
	agentID := "default"

	// Find agent configuration
	var agentCfg *config.AgentConfig
	for i := range r.daemon.config.Agents {
		if r.daemon.config.Agents[i].ID == agentID {
			agentCfg = &r.daemon.config.Agents[i]
			break
		}
	}

	if agentCfg == nil {
		return fmt.Errorf("agent %s not found", agentID)
	}

	// Enqueue message for processing
	_, err := r.daemon.queue.Enqueue(msg.SessionKey, func(ctx context.Context) (interface{}, error) {
		return r.processMessage(ctx, msg, agentCfg)
	}, nil)

	if err != nil {
		return fmt.Errorf("failed to enqueue message: %w", err)
	}

	return nil
}

// processMessage processes a message with the specified agent
func (r *Router) processMessage(ctx context.Context, msg Message, agentCfg *config.AgentConfig) (interface{}, error) {
	r.daemon.logger.Info().
		Str("session_key", msg.SessionKey).
		Str("agent_id", agentCfg.ID).
		Msg("Processing message")

	// TODO: This will be implemented when agent runner is integrated (Phase C)
	// For now, just log the message
	r.daemon.logger.Debug().
		Str("content", msg.Content).
		Msg("Message content")

	return "Message processed", nil
}

// Message represents a message to be routed
type Message struct {
	SessionKey string
	Source     string // telegram, gateway, etc.
	Content    string
	Metadata   map[string]interface{}
}
