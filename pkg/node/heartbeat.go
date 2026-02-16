package node

import (
	"context"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
)

// HeartbeatMonitor monitors node heartbeats and marks nodes offline
type HeartbeatMonitor struct {
	registry      *NodeRegistry
	config        NodeConfig
	ctx           context.Context
	cancel        context.CancelFunc
	wg            sync.WaitGroup
	eventHandlers map[string][]EventHandler
	eventMu       sync.RWMutex
}

// NewHeartbeatMonitor creates a new HeartbeatMonitor
func NewHeartbeatMonitor(registry *NodeRegistry, config NodeConfig) *HeartbeatMonitor {
	ctx, cancel := context.WithCancel(context.Background())

	return &HeartbeatMonitor{
		registry:      registry,
		config:        config,
		ctx:           ctx,
		cancel:        cancel,
		eventHandlers: make(map[string][]EventHandler),
	}
}

// Start starts the heartbeat monitor
func (hm *HeartbeatMonitor) Start() {
	hm.wg.Add(1)
	go hm.monitorLoop()

	log.Info().
		Dur("interval", hm.config.HeartbeatInterval).
		Dur("timeout", hm.config.HeartbeatTimeout).
		Msg("Heartbeat monitor started")
}

// Stop stops the heartbeat monitor
func (hm *HeartbeatMonitor) Stop() {
	hm.cancel()
	hm.wg.Wait()

	log.Info().Msg("Heartbeat monitor stopped")
}

// monitorLoop is the main monitoring loop
func (hm *HeartbeatMonitor) monitorLoop() {
	defer hm.wg.Done()

	ticker := time.NewTicker(hm.config.HeartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-hm.ctx.Done():
			return
		case <-ticker.C:
			hm.checkHeartbeats()
		}
	}
}

// checkHeartbeats checks all nodes for missed heartbeats
func (hm *HeartbeatMonitor) checkHeartbeats() {
	nodes := hm.registry.GetAllNodes()
	now := time.Now()

	for _, node := range nodes {
		if !node.Online {
			continue
		}

		timeSinceLastSeen := now.Sub(node.LastSeen)

		// Check if heartbeat timeout exceeded
		if timeSinceLastSeen > hm.config.HeartbeatTimeout {
			// Increment missed heartbeat counter
			if err := hm.registry.IncrementMissedHeartbeat(node.ID); err != nil {
				log.Error().
					Err(err).
					Str("nodeId", node.ID).
					Msg("Failed to increment missed heartbeat")
				continue
			}

			// Get updated node
			updatedNode, err := hm.registry.GetNode(node.ID)
			if err != nil {
				continue
			}

			// Emit heartbeat missed event
			hm.emit(NodeEvent{
				Type:      "node.heartbeat.missed",
				NodeID:    node.ID,
				Timestamp: now,
				Data: map[string]interface{}{
					"missedHeartbeats":  updatedNode.MissedHeartbeats,
					"timeSinceLastSeen": timeSinceLastSeen.Seconds(),
				},
			})

			// Check if max missed heartbeats exceeded
			if updatedNode.MissedHeartbeats >= hm.config.MaxMissedHeartbeats {
				// Mark node as offline
				if err := hm.registry.MarkNodeOffline(node.ID); err != nil {
					log.Error().
						Err(err).
						Str("nodeId", node.ID).
						Msg("Failed to mark node offline")
					continue
				}

				// Emit node offline event
				hm.emit(NodeEvent{
					Type:      "node.offline",
					NodeID:    node.ID,
					Timestamp: now,
					Data: map[string]interface{}{
						"reason":           "heartbeat timeout",
						"missedHeartbeats": updatedNode.MissedHeartbeats,
					},
				})

				log.Warn().
					Str("nodeId", node.ID).
					Int("missedHeartbeats", updatedNode.MissedHeartbeats).
					Msg("Node marked offline due to missed heartbeats")
			}
		}
	}
}

// On registers an event handler
func (hm *HeartbeatMonitor) On(eventType string, handler EventHandler) {
	hm.eventMu.Lock()
	defer hm.eventMu.Unlock()

	hm.eventHandlers[eventType] = append(hm.eventHandlers[eventType], handler)
}

// Off removes all event handlers for a specific event type
func (hm *HeartbeatMonitor) Off(eventType string) {
	hm.eventMu.Lock()
	defer hm.eventMu.Unlock()

	delete(hm.eventHandlers, eventType)
}

// emit emits an event to all registered handlers
func (hm *HeartbeatMonitor) emit(event NodeEvent) {
	hm.eventMu.RLock()
	handlers := hm.eventHandlers[event.Type]
	hm.eventMu.RUnlock()

	// Call handlers asynchronously
	for _, handler := range handlers {
		go handler(event)
	}
}
