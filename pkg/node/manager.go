package node

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/harun/ranya/pkg/commandqueue"
	"github.com/rs/zerolog/log"
)

// NodeManager is the main orchestrator for the node system
type NodeManager struct {
	registry          *NodeRegistry
	permissionManager *PermissionManager
	invocationManager *InvocationManager
	heartbeatMonitor  *HeartbeatMonitor
	storage           *NodeStorage
	commandQueue      *commandqueue.CommandQueue
	config            NodeConfig
	eventHandlers     map[string][]EventHandler
	eventMu           sync.RWMutex
	started           bool
	mu                sync.RWMutex
}

// NodeManagerConfig holds configuration for NodeManager
type NodeManagerConfig struct {
	NodeConfig   NodeConfig
	CommandQueue *commandqueue.CommandQueue
}

// NewNodeManager creates a new NodeManager
func NewNodeManager(config NodeManagerConfig) *NodeManager {
	registry := NewNodeRegistry()
	permissionManager := NewPermissionManager(registry)
	invocationManager := NewInvocationManager(registry, permissionManager, config.NodeConfig)
	heartbeatMonitor := NewHeartbeatMonitor(registry, config.NodeConfig)
	storage := NewNodeStorage(config.NodeConfig.StoragePath, config.NodeConfig.AutoSaveInterval)

	nm := &NodeManager{
		registry:          registry,
		permissionManager: permissionManager,
		invocationManager: invocationManager,
		heartbeatMonitor:  heartbeatMonitor,
		storage:           storage,
		commandQueue:      config.CommandQueue,
		config:            config.NodeConfig,
		eventHandlers:     make(map[string][]EventHandler),
	}

	// Forward events from sub-components
	nm.setupEventForwarding()

	return nm
}

// Start initializes the node manager
func (nm *NodeManager) Start() error {
	nm.mu.Lock()
	defer nm.mu.Unlock()

	if nm.started {
		return fmt.Errorf("node manager already started")
	}

	// Load nodes from storage
	nodes, defaultNode, err := nm.storage.LoadNodes()
	if err != nil {
		return fmt.Errorf("failed to load nodes: %w", err)
	}

	// Load nodes into registry
	if err := nm.registry.LoadNodes(nodes); err != nil {
		return fmt.Errorf("failed to load nodes into registry: %w", err)
	}

	// Set default node
	if defaultNode != "" {
		if err := nm.registry.SetDefaultNode(defaultNode); err != nil {
			log.Warn().Err(err).Str("defaultNode", defaultNode).Msg("Failed to set default node")
		}
	}

	// Start auto-save
	nm.storage.StartAutoSave(
		func() []*Node { return nm.registry.GetAllNodes() },
		func() string { return nm.registry.GetDefaultNode() },
	)

	// Start heartbeat monitor
	nm.heartbeatMonitor.Start()

	// Initialize per-node command queue lanes
	for _, node := range nodes {
		nm.commandQueue.SetConcurrency(nm.getNodeLane(node.ID), 1)
	}

	nm.started = true

	log.Info().
		Int("nodesLoaded", len(nodes)).
		Msg("Node manager started")

	return nil
}

// Stop gracefully stops the node manager
func (nm *NodeManager) Stop() error {
	nm.mu.Lock()
	defer nm.mu.Unlock()

	if !nm.started {
		return nil
	}

	// Stop heartbeat monitor
	nm.heartbeatMonitor.Stop()

	// Stop auto-save
	nm.storage.StopAutoSave()

	// Save nodes one final time
	nodes := nm.registry.GetAllNodes()
	defaultNode := nm.registry.GetDefaultNode()
	if err := nm.storage.SaveNodes(nodes, defaultNode); err != nil {
		log.Error().Err(err).Msg("Failed to save nodes on shutdown")
	}

	nm.started = false

	log.Info().Msg("Node manager stopped")

	return nil
}

// RegisterNode registers a new node
func (nm *NodeManager) RegisterNode(node *Node) error {
	// Use command queue for serialization
	result, err := nm.commandQueue.Enqueue("main", func(ctx context.Context) (interface{}, error) {
		if err := nm.registry.RegisterNode(node); err != nil {
			return nil, err
		}

		// Initialize per-node command queue lane
		nm.commandQueue.SetConcurrency(nm.getNodeLane(node.ID), 1)

		// Emit node registered event
		nm.emit(NodeEvent{
			Type:   "node.registered",
			NodeID: node.ID,
			Data: map[string]interface{}{
				"name":     node.Name,
				"platform": string(node.Platform),
			},
		})

		return nil, nil
	}, nil)

	if err != nil {
		return err
	}

	if result != nil {
		return result.(error)
	}

	return nil
}

// UnregisterNode unregisters a node
func (nm *NodeManager) UnregisterNode(nodeID string) error {
	result, err := nm.commandQueue.Enqueue("main", func(ctx context.Context) (interface{}, error) {
		if err := nm.registry.UnregisterNode(nodeID); err != nil {
			return nil, err
		}

		// Clear node's command queue lane
		nm.commandQueue.ClearLane(nm.getNodeLane(nodeID))

		// Emit node unregistered event
		nm.emit(NodeEvent{
			Type:   "node.unregistered",
			NodeID: nodeID,
		})

		return nil, nil
	}, nil)

	if err != nil {
		return err
	}

	if result != nil {
		return result.(error)
	}

	return nil
}

// GetNode retrieves a node by ID
func (nm *NodeManager) GetNode(nodeID string) (*Node, error) {
	return nm.registry.GetNode(nodeID)
}

// ListNodes returns all nodes matching the filter
func (nm *NodeManager) ListNodes(filter *NodeFilter) []*Node {
	return nm.registry.ListNodes(filter)
}

// HandleHeartbeat handles a heartbeat from a node
func (nm *NodeManager) HandleHeartbeat(nodeID string) error {
	return nm.registry.HandleHeartbeat(nodeID)
}

// InvokeCapability invokes a capability on a node
func (nm *NodeManager) InvokeCapability(ctx context.Context, req *InvocationRequest) (*InvocationResponse, error) {
	// If no node ID specified, use default node
	if req.NodeID == "" {
		defaultNode := nm.registry.GetDefaultNode()
		if defaultNode == "" {
			return &InvocationResponse{
				InvocationID: req.InvocationID,
				Success:      false,
				Error: &InvocationError{
					Code:    ErrDefaultNodeNotSet,
					Message: "no node ID specified and no default node set",
				},
			}, nil
		}
		req.NodeID = defaultNode
	}

	// Use per-node command queue lane for serialization
	lane := nm.getNodeLane(req.NodeID)

	result, err := nm.commandQueue.Enqueue(lane, func(ctx context.Context) (interface{}, error) {
		return nm.invocationManager.Invoke(ctx, req)
	}, nil)

	if err != nil {
		return nil, err
	}

	return result.(*InvocationResponse), nil
}

// HandleInvocationResponse handles a response from a node
func (nm *NodeManager) HandleInvocationResponse(response *InvocationResponse) error {
	return nm.invocationManager.HandleResponse(response)
}

// GrantPermission grants a permission to a node
func (nm *NodeManager) GrantPermission(nodeID string, permission PermissionType) error {
	result, err := nm.commandQueue.Enqueue("main", func(ctx context.Context) (interface{}, error) {
		return nil, nm.permissionManager.GrantPermission(nodeID, permission)
	}, nil)

	if err != nil {
		return err
	}

	if result != nil {
		return result.(error)
	}

	return nil
}

// RevokePermission revokes a permission from a node
func (nm *NodeManager) RevokePermission(nodeID string, permission PermissionType) error {
	result, err := nm.commandQueue.Enqueue("main", func(ctx context.Context) (interface{}, error) {
		return nil, nm.permissionManager.RevokePermission(nodeID, permission)
	}, nil)

	if err != nil {
		return err
	}

	if result != nil {
		return result.(error)
	}

	return nil
}

// GetPermissions returns all permissions for a node
func (nm *NodeManager) GetPermissions(nodeID string) (map[PermissionType]bool, error) {
	return nm.permissionManager.GetPermissions(nodeID)
}

// SetDefaultNode sets the default node
func (nm *NodeManager) SetDefaultNode(nodeID string) error {
	result, err := nm.commandQueue.Enqueue("main", func(ctx context.Context) (interface{}, error) {
		return nil, nm.registry.SetDefaultNode(nodeID)
	}, nil)

	if err != nil {
		return err
	}

	if result != nil {
		return result.(error)
	}

	return nil
}

// GetDefaultNode returns the default node ID
func (nm *NodeManager) GetDefaultNode() string {
	return nm.registry.GetDefaultNode()
}

// GetStatistics returns statistics for a node
func (nm *NodeManager) GetStatistics(nodeID string) *NodeStatistics {
	return nm.invocationManager.GetStatistics(nodeID)
}

// ResetStatistics resets statistics for a node
func (nm *NodeManager) ResetStatistics(nodeID string) {
	nm.invocationManager.ResetStatistics(nodeID)
}

// getNodeLane returns the command queue lane name for a node
func (nm *NodeManager) getNodeLane(nodeID string) string {
	return fmt.Sprintf("node-%s", nodeID)
}

// setupEventForwarding sets up event forwarding from sub-components
func (nm *NodeManager) setupEventForwarding() {
	// Forward heartbeat monitor events
	nm.heartbeatMonitor.On("node.heartbeat.missed", func(event NodeEvent) {
		nm.emit(event)
	})
	nm.heartbeatMonitor.On("node.offline", func(event NodeEvent) {
		nm.emit(event)
	})

	// Forward invocation manager events
	nm.invocationManager.On("invocation.started", func(event NodeEvent) {
		nm.emit(event)
	})
	nm.invocationManager.On("invocation.completed", func(event NodeEvent) {
		nm.emit(event)
	})
	nm.invocationManager.On("invocation.timeout", func(event NodeEvent) {
		nm.emit(event)
	})
	nm.invocationManager.On("node.degraded", func(event NodeEvent) {
		nm.emit(event)
	})
}

// On registers an event handler
func (nm *NodeManager) On(eventType string, handler EventHandler) {
	nm.eventMu.Lock()
	defer nm.eventMu.Unlock()

	nm.eventHandlers[eventType] = append(nm.eventHandlers[eventType], handler)
}

// Off removes all event handlers for a specific event type
func (nm *NodeManager) Off(eventType string) {
	nm.eventMu.Lock()
	defer nm.eventMu.Unlock()

	delete(nm.eventHandlers, eventType)
}

// emit emits an event to all registered handlers
func (nm *NodeManager) emit(event NodeEvent) {
	// Set timestamp if not set
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}

	nm.eventMu.RLock()
	handlers := nm.eventHandlers[event.Type]
	nm.eventMu.RUnlock()

	// Call handlers asynchronously
	for _, handler := range handlers {
		go handler(event)
	}
}
