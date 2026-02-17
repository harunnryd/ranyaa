package gateway

import (
	"encoding/json"
	"sync/atomic"
	"time"

	"github.com/rs/zerolog"
)

// EventBroadcaster handles broadcasting events to all authenticated clients
type EventBroadcaster struct {
	clients *ClientRegistry
	logger  zerolog.Logger
	seq     uint64
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
	msg := EventMessage{
		Type:      "event",
		Event:     event,
		Data:      data,
		Timestamp: time.Now().UnixMilli(),
		Seq:       b.nextSeq(),
	}
	b.broadcastMessage(msg)
}

// BroadcastTyped sends a typed stream event with sequence metadata.
func (b *EventBroadcaster) BroadcastTyped(msg EventMessage) {
	msg.Type = "event"
	if msg.Seq == 0 {
		msg.Seq = b.nextSeq()
	}
	if msg.Timestamp == 0 {
		msg.Timestamp = time.Now().UnixMilli()
	}
	b.broadcastMessage(msg)
}

func (b *EventBroadcaster) broadcastMessage(msg EventMessage) {
	jsonData, err := json.Marshal(msg)
	if err != nil {
		b.logger.Error().
			Err(err).
			Str("event", msg.Event).
			Str("stream", string(msg.Stream)).
			Str("phase", msg.Phase).
			Int64("seq", msg.Seq).
			Msg("Failed to marshal event")
		return
	}

	// Get all authenticated clients
	clients := b.clients.GetAuthenticatedClients()

	if len(clients) == 0 {
		b.logger.Debug().
			Str("event", msg.Event).
			Str("stream", string(msg.Stream)).
			Str("phase", msg.Phase).
			Int64("seq", msg.Seq).
			Msg("No authenticated clients to broadcast to")
		return
	}

	// Broadcast to all clients
	successCount := 0
	failureCount := 0

	for _, client := range clients {
		if err := client.WriteMessage(1, jsonData); err != nil {
			b.logger.Warn().
				Err(err).
				Str("clientId", client.ID).
				Str("event", msg.Event).
				Str("stream", string(msg.Stream)).
				Int64("seq", msg.Seq).
				Msg("Failed to broadcast to client")
			failureCount++
		} else {
			successCount++
		}
	}

	b.logger.Debug().
		Str("event", msg.Event).
		Str("stream", string(msg.Stream)).
		Str("phase", msg.Phase).
		Int64("seq", msg.Seq).
		Int("success", successCount).
		Int("failed", failureCount).
		Msg("Event broadcast complete")
}

func (b *EventBroadcaster) nextSeq() int64 {
	return int64(atomic.AddUint64(&b.seq, 1))
}
