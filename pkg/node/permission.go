package node

import (
	"fmt"

	"github.com/rs/zerolog/log"
)

// PermissionManager manages node permissions
type PermissionManager struct {
	registry *NodeRegistry
}

// NewPermissionManager creates a new PermissionManager
func NewPermissionManager(registry *NodeRegistry) *PermissionManager {
	return &PermissionManager{
		registry: registry,
	}
}

// GrantPermission grants a permission to a node
func (pm *PermissionManager) GrantPermission(nodeID string, permission PermissionType) error {
	// Verify node exists
	_, err := pm.registry.GetNode(nodeID)
	if err != nil {
		return err
	}

	// Update the node's permissions
	pm.registry.mu.Lock()
	defer pm.registry.mu.Unlock()

	actualNode, exists := pm.registry.nodes[nodeID]
	if !exists {
		return fmt.Errorf("node not found: %s", nodeID)
	}

	if actualNode.Permissions == nil {
		actualNode.Permissions = make(map[PermissionType]bool)
	}

	actualNode.Permissions[permission] = true

	log.Info().
		Str("nodeId", nodeID).
		Str("permission", string(permission)).
		Msg("Permission granted")

	return nil
}

// RevokePermission revokes a permission from a node
func (pm *PermissionManager) RevokePermission(nodeID string, permission PermissionType) error {
	// Verify node exists
	_, err := pm.registry.GetNode(nodeID)
	if err != nil {
		return err
	}

	// Update the node's permissions
	pm.registry.mu.Lock()
	defer pm.registry.mu.Unlock()

	actualNode, exists := pm.registry.nodes[nodeID]
	if !exists {
		return fmt.Errorf("node not found: %s", nodeID)
	}

	if actualNode.Permissions == nil {
		actualNode.Permissions = make(map[PermissionType]bool)
	}

	actualNode.Permissions[permission] = false

	log.Info().
		Str("nodeId", nodeID).
		Str("permission", string(permission)).
		Msg("Permission revoked")

	return nil
}

// HasPermission checks if a node has a specific permission
func (pm *PermissionManager) HasPermission(nodeID string, permission PermissionType) (bool, error) {
	node, err := pm.registry.GetNode(nodeID)
	if err != nil {
		return false, err
	}

	if node.Permissions == nil {
		return false, nil
	}

	granted, exists := node.Permissions[permission]
	return exists && granted, nil
}

// GetPermissions returns all permissions for a node
func (pm *PermissionManager) GetPermissions(nodeID string) (map[PermissionType]bool, error) {
	node, err := pm.registry.GetNode(nodeID)
	if err != nil {
		return nil, err
	}

	if node.Permissions == nil {
		return make(map[PermissionType]bool), nil
	}

	// Return a copy
	permissions := make(map[PermissionType]bool)
	for k, v := range node.Permissions {
		permissions[k] = v
	}

	return permissions, nil
}

// CheckInvocationPermission checks if a node has permission to invoke a capability
func (pm *PermissionManager) CheckInvocationPermission(nodeID string, capability NodeCapability) error {
	validator := NewCapabilityValidator()

	// Get required permission for capability
	permission, err := validator.GetPermissionForCapability(capability)
	if err != nil {
		return err
	}

	// Check if node has permission
	hasPermission, err := pm.HasPermission(nodeID, permission)
	if err != nil {
		return err
	}

	if !hasPermission {
		return fmt.Errorf("permission denied: %s", permission)
	}

	return nil
}
