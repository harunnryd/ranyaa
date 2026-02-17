package channels

import (
	"context"

	"github.com/harun/ranya/pkg/agent"
)

// InboundMessage is the normalized ingress payload from any channel.
type InboundMessage struct {
	Channel    string
	SessionKey string
	Content    string
	Metadata   map[string]interface{}
	AgentID    string
	RunConfig  agent.AgentConfig
	CWD        string
}

// DispatchFunc routes an inbound channel message into the canonical runtime flow.
type DispatchFunc func(ctx context.Context, msg InboundMessage) (interface{}, error)

// Channel is a channel runtime abstraction (telegram, gateway, webhook, ...).
type Channel interface {
	Name() string
	Start(ctx context.Context, dispatch DispatchFunc) error
	Stop(ctx context.Context) error
}
