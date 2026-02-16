package node

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPermissionManager_GrantRevoke(t *testing.T) {
	registry := NewNodeRegistry()
	pm := NewPermissionManager(registry)

	node := &Node{
		ID:           "test-node-1",
		Name:         "Test Node",
		Platform:     PlatformMacOS,
		Capabilities: []NodeCapability{CapabilityCameraSnap},
	}

	err := registry.RegisterNode(node)
	require.NoError(t, err)

	// Grant permission
	err = pm.GrantPermission("test-node-1", PermissionCamera)
	require.NoError(t, err)

	// Verify permission granted
	hasPermission, err := pm.HasPermission("test-node-1", PermissionCamera)
	require.NoError(t, err)
	assert.True(t, hasPermission)

	// Revoke permission
	err = pm.RevokePermission("test-node-1", PermissionCamera)
	require.NoError(t, err)

	// Verify permission revoked
	hasPermission, err = pm.HasPermission("test-node-1", PermissionCamera)
	require.NoError(t, err)
	assert.False(t, hasPermission)
}

func TestPermissionManager_GetPermissions(t *testing.T) {
	registry := NewNodeRegistry()
	pm := NewPermissionManager(registry)

	node := &Node{
		ID:           "test-node-1",
		Name:         "Test Node",
		Platform:     PlatformMacOS,
		Capabilities: []NodeCapability{CapabilityCameraSnap, CapabilityLocationGet},
	}

	err := registry.RegisterNode(node)
	require.NoError(t, err)

	// Grant multiple permissions
	err = pm.GrantPermission("test-node-1", PermissionCamera)
	require.NoError(t, err)
	err = pm.GrantPermission("test-node-1", PermissionLocation)
	require.NoError(t, err)

	// Get all permissions
	permissions, err := pm.GetPermissions("test-node-1")
	require.NoError(t, err)
	assert.True(t, permissions[PermissionCamera])
	assert.True(t, permissions[PermissionLocation])
	assert.False(t, permissions[PermissionNotifications])
}

func TestPermissionManager_CheckInvocationPermission(t *testing.T) {
	registry := NewNodeRegistry()
	pm := NewPermissionManager(registry)

	node := &Node{
		ID:           "test-node-1",
		Name:         "Test Node",
		Platform:     PlatformMacOS,
		Capabilities: []NodeCapability{CapabilityCameraSnap, CapabilityLocationGet},
	}

	err := registry.RegisterNode(node)
	require.NoError(t, err)

	// Grant camera permission
	err = pm.GrantPermission("test-node-1", PermissionCamera)
	require.NoError(t, err)

	// Check camera capability - should succeed
	err = pm.CheckInvocationPermission("test-node-1", CapabilityCameraSnap)
	assert.NoError(t, err)

	// Check location capability - should fail (no permission)
	err = pm.CheckInvocationPermission("test-node-1", CapabilityLocationGet)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "permission denied")
}

func TestPermissionManager_NodeNotFound(t *testing.T) {
	registry := NewNodeRegistry()
	pm := NewPermissionManager(registry)

	// Try to grant permission to non-existent node
	err := pm.GrantPermission("non-existent", PermissionCamera)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "node not found")

	// Try to check permission for non-existent node
	_, err = pm.HasPermission("non-existent", PermissionCamera)
	assert.Error(t, err)
}
