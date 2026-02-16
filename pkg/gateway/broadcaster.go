package gateway

import (
	"encoding/json"
	"time"

	"github.com/rs/zerolog"
)

// EventBroadcaster handles broadcasting events to all authenticated clients
type EventBroadcaster struct {
	clients *ClientRegistry
	logger  zerolog.Logger
}

// NewEventBroadcaster creates a new event broadcaster
func NewEventBroadcaster(clients *ClientRegistry, logger zerolog.Logger) *EventBroadcaster {
	return &EventBroadcaster{
		clients: clients,
		logger:  logger,
	}
}

// Broadcast sends an event to all authenticated clients
func (b *EventBroadcaster) Broadcast(event string, data interface{}) {
	// Create event message
	msg := EventMessage{
		Event:     event,
		Data:      data,
		Timestamp: time.Now().UnixMilli(),
	}

	// Marshal to JSON
	jsonData, err := json.Marshal(msg)
	if err != nil {
		b.logger.Error().Err(err).Str("event", event).Msg("Failed to marshal event")
		return
	}

	// Get all authenticated clients
	clients := b.clients.GetAuthenticatedClients()

	if len(clients) == 0 {
		b.logger.Debug().Str("event", event).Msg("No authenticated clients to broadcast to")
		return
	}

	// Broadcast to all clients
	successCount := 0
	failureCount := 0

	for _, client := range clients {
		if err := client.Conn.WriteMessage(1, jsonData); err != nil {
			b.logger.Warn().
				Err(err).
				Str("clientId", client.ID).
				Str("event", event).
				Msg("Failed to broadcast to client")
			failureCount++
		} else {
			successCount++
		}
	}

	b.logger.Debug().
		Str("event", event).
		Int("success", successCount).
		Int("failed", failureCount).
		Msg("Event broadcast complete")
}
