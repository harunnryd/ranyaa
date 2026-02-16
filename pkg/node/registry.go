package node

import (
	"fmt"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
)

// NodeRegistry manages node registration and lifecycle
type NodeRegistry struct {
	nodes       map[string]*Node
	mu          sync.RWMutex
	validator   *CapabilityValidator
	defaultNode string
}

// NewNodeRegistry creates a new NodeRegistry
func NewNodeRegistry() *NodeRegistry {
	return &NodeRegistry{
		nodes:     make(map[string]*Node),
		validator: NewCapabilityValidator(),
	}
}

// RegisterNode registers a new node or updates an existing one
func (nr *NodeRegistry) RegisterNode(node *Node) error {
	if node.ID == "" {
		return fmt.Errorf("node ID is required")
	}
	if node.Name == "" {
		return fmt.Errorf("node name is required")
	}
	if node.Platform == "" {
		return fmt.Errorf("node platform is required")
	}

	// Validate capabilities
	if err := nr.validator.ValidateCapabilities(node.Capabilities); err != nil {
		return fmt.Errorf("invalid capabilities: %w", err)
	}

	nr.mu.Lock()
	defer nr.mu.Unlock()

	// Check if node already exists
	existing, exists := nr.nodes[node.ID]
	if exists {
		// Update existing node
		existing.Name = node.Name
		existing.Platform = node.Platform
		existing.Capabilities = node.Capabilities
		existing.Metadata = node.Metadata
		existing.LastSeen = time.Now()
		existing.Online = true
		existing.MissedHeartbeats = 0

		log.Info().
			Str("nodeId", node.ID).
			Str("name", node.Name).
			Msg("Node updated")

		return nil
	}

	// Initialize new node
	now := time.Now()
	node.CreatedAt = now
	node.LastSeen = now
	node.Online = true
	node.MissedHeartbeats = 0
	node.TimeoutCount = 0
	node.Degraded = false

	// Initialize permissions if not set
	if node.Permissions == nil {
		node.Permissions = make(map[PermissionType]bool)
		// Default all permissions to false
		node.Permissions[PermissionCamera] = false
		node.Permissions[PermissionLocation] = false
		node.Permissions[PermissionNotifications] = false
		node.Permissions[PermissionAutomation] = false
	}

	nr.nodes[node.ID] = node

	log.Info().
		Str("nodeId", node.ID).
		Str("name", node.Name).
		Str("platform", string(node.Platform)).
		Int("capabilities", len(node.Capabilities)).
		Msg("Node registered")

	return nil
}

// UnregisterNode removes a node from the registry
func (nr *NodeRegistry) UnregisterNode(nodeID string) error {
	nr.mu.Lock()
	defer nr.mu.Unlock()

	if _, exists := nr.nodes[nodeID]; !exists {
		return fmt.Errorf("node not found: %s", nodeID)
	}

	delete(nr.nodes, nodeID)

	// Clear default node if it was this node
	if nr.defaultNode == nodeID {
		nr.defaultNode = ""
	}

	log.Info().Str("nodeId", nodeID).Msg("Node unregistered")

	return nil
}

// GetNode retrieves a node by ID
func (nr *NodeRegistry) GetNode(nodeID string) (*Node, error) {
	nr.mu.RLock()
	defer nr.mu.RUnlock()

	node, exists := nr.nodes[nodeID]
	if !exists {
		return nil, fmt.Errorf("node not found: %s", nodeID)
	}

	// Return a copy to prevent external modifications
	nodeCopy := *node
	return &nodeCopy, nil
}

// ListNodes returns all nodes matching the filter
func (nr *NodeRegistry) ListNodes(filter *NodeFilter) []*Node {
	nr.mu.RLock()
	defer nr.mu.RUnlock()

	result := make([]*Node, 0)

	for _, node := range nr.nodes {
		if filter != nil {
			// Apply filters
			if filter.Platform != nil && node.Platform != *filter.Platform {
				continue
			}
			if filter.Online != nil && node.Online != *filter.Online {
				continue
			}
			if filter.Degraded != nil && node.Degraded != *filter.Degraded {
				continue
			}
			if filter.Capability != nil {
				hasCapability := false
				for _, cap := range node.Capabilities {
					if cap == *filter.Capability {
						hasCapability = true
						break
					}
				}
				if !hasCapability {
					continue
				}
			}
		}

		// Return a copy
		nodeCopy := *node
		result = append(result, &nodeCopy)
	}

	return result
}

// HandleHeartbeat updates the last seen time for a node
func (nr *NodeRegistry) HandleHeartbeat(nodeID string) error {
	nr.mu.Lock()
	defer nr.mu.Unlock()

	node, exists := nr.nodes[nodeID]
	if !exists {
		return fmt.Errorf("node not found: %s", nodeID)
	}

	node.LastSeen = time.Now()
	node.Online = true
	node.MissedHeartbeats = 0

	return nil
}

// MarkNodeOffline marks a node as offline
func (nr *NodeRegistry) MarkNodeOffline(nodeID string) error {
	nr.mu.Lock()
	defer nr.mu.Unlock()

	node, exists := nr.nodes[nodeID]
	if !exists {
		return fmt.Errorf("node not found: %s", nodeID)
	}

	node.Online = false

	log.Warn().Str("nodeId", nodeID).Msg("Node marked offline")

	return nil
}

// IncrementMissedHeartbeat increments the missed heartbeat counter
func (nr *NodeRegistry) IncrementMissedHeartbeat(nodeID string) error {
	nr.mu.Lock()
	defer nr.mu.Unlock()

	node, exists := nr.nodes[nodeID]
	if !exists {
		return fmt.Errorf("node not found: %s", nodeID)
	}

	node.MissedHeartbeats++

	log.Debug().
		Str("nodeId", nodeID).
		Int("missedHeartbeats", node.MissedHeartbeats).
		Msg("Missed heartbeat incremented")

	return nil
}

// IncrementTimeoutCount increments the timeout counter and checks degraded status
func (nr *NodeRegistry) IncrementTimeoutCount(nodeID string, degradedThreshold int) error {
	nr.mu.Lock()
	defer nr.mu.Unlock()

	node, exists := nr.nodes[nodeID]
	if !exists {
		return fmt.Errorf("node not found: %s", nodeID)
	}

	node.TimeoutCount++

	// Check if node should be marked as degraded
	if node.TimeoutCount >= degradedThreshold && !node.Degraded {
		node.Degraded = true
		log.Warn().
			Str("nodeId", nodeID).
			Int("timeoutCount", node.TimeoutCount).
			Msg("Node marked as degraded")
	}

	return nil
}

// ResetTimeoutCount resets the timeout counter and degraded status
func (nr *NodeRegistry) ResetTimeoutCount(nodeID string) error {
	nr.mu.Lock()
	defer nr.mu.Unlock()

	node, exists := nr.nodes[nodeID]
	if !exists {
		return fmt.Errorf("node not found: %s", nodeID)
	}

	node.TimeoutCount = 0
	node.Degraded = false

	return nil
}

// SetDefaultNode sets the default node for capability invocations
func (nr *NodeRegistry) SetDefaultNode(nodeID string) error {
	nr.mu.Lock()
	defer nr.mu.Unlock()

	if nodeID != "" {
		if _, exists := nr.nodes[nodeID]; !exists {
			return fmt.Errorf("node not found: %s", nodeID)
		}
	}

	nr.defaultNode = nodeID

	log.Info().Str("nodeId", nodeID).Msg("Default node set")

	return nil
}

// GetDefaultNode returns the default node ID
func (nr *NodeRegistry) GetDefaultNode() string {
	nr.mu.RLock()
	defer nr.mu.RUnlock()

	return nr.defaultNode
}

// HasCapability checks if a node has a specific capability
func (nr *NodeRegistry) HasCapability(nodeID string, capability NodeCapability) bool {
	nr.mu.RLock()
	defer nr.mu.RUnlock()

	node, exists := nr.nodes[nodeID]
	if !exists {
		return false
	}

	for _, cap := range node.Capabilities {
		if cap == capability {
			return true
		}
	}

	return false
}

// GetAllNodes returns all nodes (for persistence)
func (nr *NodeRegistry) GetAllNodes() []*Node {
	nr.mu.RLock()
	defer nr.mu.RUnlock()

	result := make([]*Node, 0, len(nr.nodes))
	for _, node := range nr.nodes {
		nodeCopy := *node
		result = append(result, &nodeCopy)
	}

	return result
}

// LoadNodes loads nodes from a list (for persistence)
func (nr *NodeRegistry) LoadNodes(nodes []*Node) error {
	nr.mu.Lock()
	defer nr.mu.Unlock()

	for _, node := range nodes {
		// Validate capabilities
		if err := nr.validator.ValidateCapabilities(node.Capabilities); err != nil {
			log.Warn().
				Err(err).
				Str("nodeId", node.ID).
				Msg("Skipping node with invalid capabilities")
			continue
		}

		nr.nodes[node.ID] = node
	}

	log.Info().Int("count", len(nodes)).Msg("Nodes loaded from storage")

	return nil
}
