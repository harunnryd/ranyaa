package node

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInvocationManager_Invoke_Success(t *testing.T) {
	registry := NewNodeRegistry()
	pm := NewPermissionManager(registry)
	config := DefaultNodeConfig()
	im := NewInvocationManager(registry, pm, config)

	// Register node
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

	// Create invocation request
	req := &InvocationRequest{
		NodeID:     "test-node-1",
		Capability: CapabilityCameraSnap,
		Parameters: map[string]interface{}{"quality": float64(80)},
		Timeout:    5000,
	}

	// Start invocation in goroutine
	responseChan := make(chan *InvocationResponse)
	go func() {
		response, err := im.Invoke(context.Background(), req)
		require.NoError(t, err)
		responseChan <- response
	}()

	// Simulate node response
	time.Sleep(100 * time.Millisecond)
	nodeResponse := &InvocationResponse{
		InvocationID: req.InvocationID,
		NodeID:       "test-node-1",
		Success:      true,
		Result:       map[string]interface{}{"imageUrl": "http://example.com/image.jpg"},
	}
	_ = im.HandleResponse(nodeResponse)
	require.NoError(t, err)

	// Wait for response
	response := <-responseChan
	assert.True(t, response.Success)
	assert.NotNil(t, response.Result)
}

func TestInvocationManager_Invoke_NodeNotFound(t *testing.T) {
	registry := NewNodeRegistry()
	pm := NewPermissionManager(registry)
	config := DefaultNodeConfig()
	im := NewInvocationManager(registry, pm, config)

	req := &InvocationRequest{
		NodeID:     "non-existent",
		Capability: CapabilityCameraSnap,
		Parameters: map[string]interface{}{},
		Timeout:    5000,
	}

	response, err := im.Invoke(context.Background(), req)
	require.NoError(t, err)
	assert.False(t, response.Success)
	assert.Equal(t, ErrNodeNotFound, response.Error.Code)
}

func TestInvocationManager_Invoke_NodeOffline(t *testing.T) {
	registry := NewNodeRegistry()
	pm := NewPermissionManager(registry)
	config := DefaultNodeConfig()
	im := NewInvocationManager(registry, pm, config)

	// Register node
	node := &Node{
		ID:           "test-node-1",
		Name:         "Test Node",
		Platform:     PlatformMacOS,
		Capabilities: []NodeCapability{CapabilityCameraSnap},
	}
	err := registry.RegisterNode(node)
	require.NoError(t, err)

	// Mark node offline
	err = registry.MarkNodeOffline("test-node-1")
	require.NoError(t, err)

	req := &InvocationRequest{
		NodeID:     "test-node-1",
		Capability: CapabilityCameraSnap,
		Parameters: map[string]interface{}{},
		Timeout:    5000,
	}

	response, err := im.Invoke(context.Background(), req)
	require.NoError(t, err)
	assert.False(t, response.Success)
	assert.Equal(t, ErrNodeOffline, response.Error.Code)
}

func TestInvocationManager_Invoke_CapabilityNotFound(t *testing.T) {
	registry := NewNodeRegistry()
	pm := NewPermissionManager(registry)
	config := DefaultNodeConfig()
	im := NewInvocationManager(registry, pm, config)

	// Register node
	node := &Node{
		ID:           "test-node-1",
		Name:         "Test Node",
		Platform:     PlatformMacOS,
		Capabilities: []NodeCapability{CapabilityCameraSnap},
	}
	err := registry.RegisterNode(node)
	require.NoError(t, err)

	req := &InvocationRequest{
		NodeID:     "test-node-1",
		Capability: CapabilityLocationGet, // Node doesn't have this capability
		Parameters: map[string]interface{}{},
		Timeout:    5000,
	}

	response, err := im.Invoke(context.Background(), req)
	require.NoError(t, err)
	assert.False(t, response.Success)
	assert.Equal(t, ErrCapabilityNotFound, response.Error.Code)
}

func TestInvocationManager_Invoke_PermissionDenied(t *testing.T) {
	registry := NewNodeRegistry()
	pm := NewPermissionManager(registry)
	config := DefaultNodeConfig()
	im := NewInvocationManager(registry, pm, config)

	// Register node
	node := &Node{
		ID:           "test-node-1",
		Name:         "Test Node",
		Platform:     PlatformMacOS,
		Capabilities: []NodeCapability{CapabilityCameraSnap},
	}
	err := registry.RegisterNode(node)
	require.NoError(t, err)

	// Don't grant permission

	req := &InvocationRequest{
		NodeID:     "test-node-1",
		Capability: CapabilityCameraSnap,
		Parameters: map[string]interface{}{},
		Timeout:    5000,
	}

	response, err := im.Invoke(context.Background(), req)
	require.NoError(t, err)
	assert.False(t, response.Success)
	assert.Equal(t, ErrPermissionDenied, response.Error.Code)
}

func TestInvocationManager_Invoke_InvalidParameters(t *testing.T) {
	registry := NewNodeRegistry()
	pm := NewPermissionManager(registry)
	config := DefaultNodeConfig()
	im := NewInvocationManager(registry, pm, config)

	// Register node
	node := &Node{
		ID:           "test-node-1",
		Name:         "Test Node",
		Platform:     PlatformMacOS,
		Capabilities: []NodeCapability{CapabilityNotificationSend},
	}
	err := registry.RegisterNode(node)
	require.NoError(t, err)

	// Grant permission
	err = pm.GrantPermission("test-node-1", PermissionNotifications)
	require.NoError(t, err)

	// Missing required parameters
	req := &InvocationRequest{
		NodeID:     "test-node-1",
		Capability: CapabilityNotificationSend,
		Parameters: map[string]interface{}{}, // Missing title and message
		Timeout:    5000,
	}

	response, err := im.Invoke(context.Background(), req)
	require.NoError(t, err)
	assert.False(t, response.Success)
	assert.Equal(t, ErrInvalidParameters, response.Error.Code)
}

func TestInvocationManager_Invoke_Timeout(t *testing.T) {
	registry := NewNodeRegistry()
	pm := NewPermissionManager(registry)
	config := DefaultNodeConfig()
	im := NewInvocationManager(registry, pm, config)

	// Register node
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

	req := &InvocationRequest{
		NodeID:     "test-node-1",
		Capability: CapabilityCameraSnap,
		Parameters: map[string]interface{}{},
		Timeout:    100, // Very short timeout
	}

	// Don't send response - let it timeout
	response, err := im.Invoke(context.Background(), req)
	require.NoError(t, err)
	assert.False(t, response.Success)
	assert.Equal(t, ErrInvocationTimeout, response.Error.Code)

	// Verify timeout count incremented
	updatedNode, err := registry.GetNode("test-node-1")
	require.NoError(t, err)
	assert.Equal(t, 1, updatedNode.TimeoutCount)
}

func TestInvocationManager_Statistics(t *testing.T) {
	registry := NewNodeRegistry()
	pm := NewPermissionManager(registry)
	config := DefaultNodeConfig()
	im := NewInvocationManager(registry, pm, config)

	// Register node
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

	// Perform successful invocation
	req := &InvocationRequest{
		NodeID:     "test-node-1",
		Capability: CapabilityCameraSnap,
		Parameters: map[string]interface{}{},
		Timeout:    5000,
	}

	go func() {
		time.Sleep(50 * time.Millisecond)
		nodeResponse := &InvocationResponse{
			InvocationID: req.InvocationID,
			NodeID:       "test-node-1",
			Success:      true,
			Result:       map[string]interface{}{},
		}
		_ = im.HandleResponse(nodeResponse)
	}()

	response, err := im.Invoke(context.Background(), req)
	require.NoError(t, err)
	assert.True(t, response.Success)

	// Check statistics
	stats := im.GetStatistics("test-node-1")
	assert.Equal(t, int64(1), stats.TotalInvocations)
	assert.Equal(t, int64(1), stats.SuccessfulInvocations)
	assert.Equal(t, int64(0), stats.FailedInvocations)
	assert.Greater(t, stats.AverageLatency, float64(0))

	// Reset statistics
	im.ResetStatistics("test-node-1")
	stats = im.GetStatistics("test-node-1")
	assert.Equal(t, int64(0), stats.TotalInvocations)
}
