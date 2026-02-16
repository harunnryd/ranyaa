package node

import (
	"context"
	"fmt"
	"sync"
	"time"

	gonanoid "github.com/matoous/go-nanoid/v2"
	"github.com/rs/zerolog/log"
)

// InvocationManager manages capability invocations
type InvocationManager struct {
	registry          *NodeRegistry
	permissionManager *PermissionManager
	validator         *CapabilityValidator
	config            NodeConfig
	activeInvocations map[string]*invocationTracker
	mu                sync.RWMutex
	eventHandlers     map[string][]EventHandler
	eventMu           sync.RWMutex
	statistics        map[string]*NodeStatistics
	statsMu           sync.RWMutex
}

// NewInvocationManager creates a new InvocationManager
func NewInvocationManager(registry *NodeRegistry, permissionManager *PermissionManager, config NodeConfig) *InvocationManager {
	return &InvocationManager{
		registry:          registry,
		permissionManager: permissionManager,
		validator:         NewCapabilityValidator(),
		config:            config,
		activeInvocations: make(map[string]*invocationTracker),
		eventHandlers:     make(map[string][]EventHandler),
		statistics:        make(map[string]*NodeStatistics),
	}
}

// Invoke invokes a capability on a node
func (im *InvocationManager) Invoke(ctx context.Context, req *InvocationRequest) (*InvocationResponse, error) {
	startTime := time.Now()

	// Generate invocation ID if not provided
	if req.InvocationID == "" {
		id, _ := gonanoid.New()
		req.InvocationID = id
	}

	// Set timestamp if not provided
	if req.Timestamp.IsZero() {
		req.Timestamp = time.Now()
	}

	// Set default timeout if not provided
	if req.Timeout == 0 {
		req.Timeout = int(im.config.InvocationTimeout.Milliseconds())
	}

	// Validate node exists
	node, err := im.registry.GetNode(req.NodeID)
	if err != nil {
		return im.createErrorResponse(req, ErrNodeNotFound, fmt.Sprintf("node not found: %s", req.NodeID)), nil
	}

	// Check if node is online
	if !node.Online {
		return im.createErrorResponse(req, ErrNodeOffline, "node is offline"), nil
	}

	// Check if node has capability
	if !im.registry.HasCapability(req.NodeID, req.Capability) {
		return im.createErrorResponse(req, ErrCapabilityNotFound, fmt.Sprintf("node does not have capability: %s", req.Capability)), nil
	}

	// Check permission
	if err := im.permissionManager.CheckInvocationPermission(req.NodeID, req.Capability); err != nil {
		return im.createErrorResponse(req, ErrPermissionDenied, err.Error()), nil
	}

	// Validate parameters
	if err := im.validator.ValidateInvocationParameters(req.Capability, req.Parameters); err != nil {
		return im.createErrorResponse(req, ErrInvalidParameters, err.Error()), nil
	}

	// Create invocation tracker
	tracker := &invocationTracker{
		InvocationID: req.InvocationID,
		NodeID:       req.NodeID,
		Capability:   req.Capability,
		StartTime:    startTime,
		ResponseChan: make(chan *InvocationResponse, 1),
	}

	// Register active invocation
	im.mu.Lock()
	im.activeInvocations[req.InvocationID] = tracker
	im.mu.Unlock()

	// Emit invocation started event
	im.emit(NodeEvent{
		Type:      "invocation.started",
		NodeID:    req.NodeID,
		Timestamp: time.Now(),
		Data: map[string]interface{}{
			"invocationId": req.InvocationID,
			"capability":   string(req.Capability),
		},
	})

	log.Debug().
		Str("invocationId", req.InvocationID).
		Str("nodeId", req.NodeID).
		Str("capability", string(req.Capability)).
		Msg("Invocation started")

	// Wait for response with timeout
	timeoutCtx, cancel := context.WithTimeout(ctx, time.Duration(req.Timeout)*time.Millisecond)
	defer cancel()

	select {
	case response := <-tracker.ResponseChan:
		// Remove from active invocations
		im.mu.Lock()
		delete(im.activeInvocations, req.InvocationID)
		im.mu.Unlock()

		// Record statistics
		latency := time.Since(startTime).Milliseconds()
		if response.Success {
			im.recordSuccess(req.NodeID, latency)
		} else {
			im.recordFailure(req.NodeID, latency)
		}

		// Emit invocation completed event
		im.emit(NodeEvent{
			Type:      "invocation.completed",
			NodeID:    req.NodeID,
			Timestamp: time.Now(),
			Data: map[string]interface{}{
				"invocationId": req.InvocationID,
				"capability":   string(req.Capability),
				"success":      response.Success,
				"latency":      latency,
			},
		})

		return response, nil

	case <-timeoutCtx.Done():
		// Remove from active invocations
		im.mu.Lock()
		delete(im.activeInvocations, req.InvocationID)
		im.mu.Unlock()

		// Increment timeout count
		if err := im.registry.IncrementTimeoutCount(req.NodeID, im.config.DegradedThreshold); err != nil {
			log.Error().Err(err).Str("nodeId", req.NodeID).Msg("Failed to increment timeout count")
		}

		// Record timeout statistics
		latency := time.Since(startTime).Milliseconds()
		im.recordTimeout(req.NodeID, latency)

		// Check if node is now degraded
		updatedNode, _ := im.registry.GetNode(req.NodeID)
		if updatedNode != nil && updatedNode.Degraded {
			im.emit(NodeEvent{
				Type:      "node.degraded",
				NodeID:    req.NodeID,
				Timestamp: time.Now(),
				Data: map[string]interface{}{
					"timeoutCount": updatedNode.TimeoutCount,
				},
			})
		}

		// Emit invocation timeout event
		im.emit(NodeEvent{
			Type:      "invocation.timeout",
			NodeID:    req.NodeID,
			Timestamp: time.Now(),
			Data: map[string]interface{}{
				"invocationId": req.InvocationID,
				"capability":   string(req.Capability),
				"timeout":      req.Timeout,
			},
		})

		log.Warn().
			Str("invocationId", req.InvocationID).
			Str("nodeId", req.NodeID).
			Int("timeout", req.Timeout).
			Msg("Invocation timeout")

		return im.createErrorResponse(req, ErrInvocationTimeout, "invocation timeout"), nil
	}
}

// HandleResponse handles a response from a node
func (im *InvocationManager) HandleResponse(response *InvocationResponse) error {
	im.mu.RLock()
	tracker, exists := im.activeInvocations[response.InvocationID]
	im.mu.RUnlock()

	if !exists {
		return fmt.Errorf("invocation not found: %s", response.InvocationID)
	}

	// Send response to waiting goroutine
	select {
	case tracker.ResponseChan <- response:
		return nil
	default:
		return fmt.Errorf("response channel full")
	}
}

// GetActiveInvocations returns all active invocations for a node
func (im *InvocationManager) GetActiveInvocations(nodeID string) []string {
	im.mu.RLock()
	defer im.mu.RUnlock()

	result := make([]string, 0)
	for invocationID, tracker := range im.activeInvocations {
		if tracker.NodeID == nodeID {
			result = append(result, invocationID)
		}
	}

	return result
}

// GetStatistics returns statistics for a node
func (im *InvocationManager) GetStatistics(nodeID string) *NodeStatistics {
	im.statsMu.RLock()
	defer im.statsMu.RUnlock()

	stats, exists := im.statistics[nodeID]
	if !exists {
		return &NodeStatistics{
			NodeID: nodeID,
		}
	}

	// Return a copy
	statsCopy := *stats
	return &statsCopy
}

// ResetStatistics resets statistics for a node
func (im *InvocationManager) ResetStatistics(nodeID string) {
	im.statsMu.Lock()
	defer im.statsMu.Unlock()

	delete(im.statistics, nodeID)

	log.Info().Str("nodeId", nodeID).Msg("Statistics reset")
}

// createErrorResponse creates an error response
func (im *InvocationManager) createErrorResponse(req *InvocationRequest, code NodeErrorCode, message string) *InvocationResponse {
	return &InvocationResponse{
		InvocationID: req.InvocationID,
		NodeID:       req.NodeID,
		Success:      false,
		Error: &InvocationError{
			Code:    code,
			Message: message,
		},
		Timestamp: time.Now(),
	}
}

// recordSuccess records a successful invocation
func (im *InvocationManager) recordSuccess(nodeID string, latency int64) {
	im.statsMu.Lock()
	defer im.statsMu.Unlock()

	stats, exists := im.statistics[nodeID]
	if !exists {
		node, _ := im.registry.GetNode(nodeID)
		uptime := time.Duration(0)
		if node != nil {
			uptime = time.Since(node.CreatedAt)
		}

		stats = &NodeStatistics{
			NodeID: nodeID,
			Uptime: uptime,
		}
		im.statistics[nodeID] = stats
	}

	stats.TotalInvocations++
	stats.SuccessfulInvocations++
	stats.LastInvocation = time.Now()

	// Update average latency
	totalLatency := stats.AverageLatency*float64(stats.TotalInvocations-1) + float64(latency)
	stats.AverageLatency = totalLatency / float64(stats.TotalInvocations)
}

// recordFailure records a failed invocation
func (im *InvocationManager) recordFailure(nodeID string, latency int64) {
	im.statsMu.Lock()
	defer im.statsMu.Unlock()

	stats, exists := im.statistics[nodeID]
	if !exists {
		node, _ := im.registry.GetNode(nodeID)
		uptime := time.Duration(0)
		if node != nil {
			uptime = time.Since(node.CreatedAt)
		}

		stats = &NodeStatistics{
			NodeID: nodeID,
			Uptime: uptime,
		}
		im.statistics[nodeID] = stats
	}

	stats.TotalInvocations++
	stats.FailedInvocations++
	stats.LastInvocation = time.Now()

	// Update average latency
	totalLatency := stats.AverageLatency*float64(stats.TotalInvocations-1) + float64(latency)
	stats.AverageLatency = totalLatency / float64(stats.TotalInvocations)
}

// recordTimeout records a timeout invocation
func (im *InvocationManager) recordTimeout(nodeID string, latency int64) {
	im.statsMu.Lock()
	defer im.statsMu.Unlock()

	stats, exists := im.statistics[nodeID]
	if !exists {
		node, _ := im.registry.GetNode(nodeID)
		uptime := time.Duration(0)
		if node != nil {
			uptime = time.Since(node.CreatedAt)
		}

		stats = &NodeStatistics{
			NodeID: nodeID,
			Uptime: uptime,
		}
		im.statistics[nodeID] = stats
	}

	stats.TotalInvocations++
	stats.TimeoutInvocations++
	stats.LastInvocation = time.Now()

	// Update average latency
	totalLatency := stats.AverageLatency*float64(stats.TotalInvocations-1) + float64(latency)
	stats.AverageLatency = totalLatency / float64(stats.TotalInvocations)
}

// On registers an event handler
func (im *InvocationManager) On(eventType string, handler EventHandler) {
	im.eventMu.Lock()
	defer im.eventMu.Unlock()

	im.eventHandlers[eventType] = append(im.eventHandlers[eventType], handler)
}

// Off removes all event handlers for a specific event type
func (im *InvocationManager) Off(eventType string) {
	im.eventMu.Lock()
	defer im.eventMu.Unlock()

	delete(im.eventHandlers, eventType)
}

// emit emits an event to all registered handlers
func (im *InvocationManager) emit(event NodeEvent) {
	im.eventMu.RLock()
	handlers := im.eventHandlers[event.Type]
	im.eventMu.RUnlock()

	// Call handlers asynchronously
	for _, handler := range handlers {
		go handler(event)
	}
}
