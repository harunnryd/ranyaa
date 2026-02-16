package node

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNodeRegistry_RegisterNode(t *testing.T) {
	registry := NewNodeRegistry()

	node := &Node{
		ID:           "test-node-1",
		Name:         "Test Node",
		Platform:     PlatformMacOS,
		Capabilities: []NodeCapability{CapabilityCameraSnap},
		Metadata:     map[string]interface{}{"version": "1.0.0"},
	}

	err := registry.RegisterNode(node)
	require.NoError(t, err)

	// Verify node was registered
	retrieved, err := registry.GetNode("test-node-1")
	require.NoError(t, err)
	assert.Equal(t, "test-node-1", retrieved.ID)
	assert.Equal(t, "Test Node", retrieved.Name)
	assert.Equal(t, PlatformMacOS, retrieved.Platform)
	assert.True(t, retrieved.Online)
	assert.Equal(t, 0, retrieved.MissedHeartbeats)
	assert.False(t, retrieved.Degraded)
}

func TestNodeRegistry_RegisterNode_Validation(t *testing.T) {
	registry := NewNodeRegistry()

	tests := []struct {
		name        string
		node        *Node
		expectedErr string
	}{
		{
			name: "missing ID",
			node: &Node{
				Name:         "Test",
				Platform:     PlatformMacOS,
				Capabilities: []NodeCapability{CapabilityCameraSnap},
			},
			expectedErr: "node ID is required",
		},
		{
			name: "missing name",
			node: &Node{
				ID:           "test-1",
				Platform:     PlatformMacOS,
				Capabilities: []NodeCapability{CapabilityCameraSnap},
			},
			expectedErr: "node name is required",
		},
		{
			name: "missing platform",
			node: &Node{
				ID:           "test-1",
				Name:         "Test",
				Capabilities: []NodeCapability{CapabilityCameraSnap},
			},
			expectedErr: "node platform is required",
		},
		{
			name: "invalid capability",
			node: &Node{
				ID:           "test-1",
				Name:         "Test",
				Platform:     PlatformMacOS,
				Capabilities: []NodeCapability{"invalid.capability"},
			},
			expectedErr: "invalid capabilities",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := registry.RegisterNode(tt.node)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.expectedErr)
		})
	}
}

func TestNodeRegistry_RegisterNode_Idempotency(t *testing.T) {
	registry := NewNodeRegistry()

	node := &Node{
		ID:           "test-node-1",
		Name:         "Test Node",
		Platform:     PlatformMacOS,
		Capabilities: []NodeCapability{CapabilityCameraSnap},
	}

	// Register twice
	err := registry.RegisterNode(node)
	require.NoError(t, err)

	node.Name = "Updated Name"
	err = registry.RegisterNode(node)
	require.NoError(t, err)

	// Verify only one node exists with updated name
	nodes := registry.ListNodes(nil)
	assert.Equal(t, 1, len(nodes))
	assert.Equal(t, "Updated Name", nodes[0].Name)
}

func TestNodeRegistry_UnregisterNode(t *testing.T) {
	registry := NewNodeRegistry()

	node := &Node{
		ID:           "test-node-1",
		Name:         "Test Node",
		Platform:     PlatformMacOS,
		Capabilities: []NodeCapability{CapabilityCameraSnap},
	}

	err := registry.RegisterNode(node)
	require.NoError(t, err)

	err = registry.UnregisterNode("test-node-1")
	require.NoError(t, err)

	// Verify node was removed
	_, err = registry.GetNode("test-node-1")
	assert.Error(t, err)
}

func TestNodeRegistry_ListNodes_Filter(t *testing.T) {
	registry := NewNodeRegistry()

	// Register multiple nodes
	nodes := []*Node{
		{
			ID:           "node-1",
			Name:         "Node 1",
			Platform:     PlatformMacOS,
			Capabilities: []NodeCapability{CapabilityCameraSnap},
		},
		{
			ID:           "node-2",
			Name:         "Node 2",
			Platform:     PlatformWindows,
			Capabilities: []NodeCapability{CapabilityLocationGet},
		},
		{
			ID:           "node-3",
			Name:         "Node 3",
			Platform:     PlatformMacOS,
			Capabilities: []NodeCapability{CapabilityCameraSnap, CapabilityLocationGet},
		},
	}

	for _, node := range nodes {
		err := registry.RegisterNode(node)
		require.NoError(t, err)
	}

	// Test platform filter
	platform := PlatformMacOS
	filtered := registry.ListNodes(&NodeFilter{Platform: &platform})
	assert.Equal(t, 2, len(filtered))

	// Test capability filter
	capability := CapabilityLocationGet
	filtered = registry.ListNodes(&NodeFilter{Capability: &capability})
	assert.Equal(t, 2, len(filtered))

	// Test online filter
	online := true
	filtered = registry.ListNodes(&NodeFilter{Online: &online})
	assert.Equal(t, 3, len(filtered))
}

func TestNodeRegistry_HandleHeartbeat(t *testing.T) {
	registry := NewNodeRegistry()

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

	// Increment missed heartbeats
	err = registry.IncrementMissedHeartbeat("test-node-1")
	require.NoError(t, err)

	// Handle heartbeat
	err = registry.HandleHeartbeat("test-node-1")
	require.NoError(t, err)

	// Verify node is back online with reset counters
	retrieved, err := registry.GetNode("test-node-1")
	require.NoError(t, err)
	assert.True(t, retrieved.Online)
	assert.Equal(t, 0, retrieved.MissedHeartbeats)
}

func TestNodeRegistry_TimeoutCount(t *testing.T) {
	registry := NewNodeRegistry()

	node := &Node{
		ID:           "test-node-1",
		Name:         "Test Node",
		Platform:     PlatformMacOS,
		Capabilities: []NodeCapability{CapabilityCameraSnap},
	}

	err := registry.RegisterNode(node)
	require.NoError(t, err)

	// Increment timeout count multiple times
	degradedThreshold := 5
	for i := 0; i < degradedThreshold; i++ {
		err = registry.IncrementTimeoutCount("test-node-1", degradedThreshold)
		require.NoError(t, err)
	}

	// Verify node is marked as degraded
	retrieved, err := registry.GetNode("test-node-1")
	require.NoError(t, err)
	assert.True(t, retrieved.Degraded)
	assert.Equal(t, degradedThreshold, retrieved.TimeoutCount)

	// Reset timeout count
	err = registry.ResetTimeoutCount("test-node-1")
	require.NoError(t, err)

	// Verify degraded status is cleared
	retrieved, err = registry.GetNode("test-node-1")
	require.NoError(t, err)
	assert.False(t, retrieved.Degraded)
	assert.Equal(t, 0, retrieved.TimeoutCount)
}

func TestNodeRegistry_DefaultNode(t *testing.T) {
	registry := NewNodeRegistry()

	node := &Node{
		ID:           "test-node-1",
		Name:         "Test Node",
		Platform:     PlatformMacOS,
		Capabilities: []NodeCapability{CapabilityCameraSnap},
	}

	err := registry.RegisterNode(node)
	require.NoError(t, err)

	// Set default node
	err = registry.SetDefaultNode("test-node-1")
	require.NoError(t, err)

	// Verify default node
	defaultNode := registry.GetDefaultNode()
	assert.Equal(t, "test-node-1", defaultNode)

	// Unregister node should clear default
	err = registry.UnregisterNode("test-node-1")
	require.NoError(t, err)

	defaultNode = registry.GetDefaultNode()
	assert.Equal(t, "", defaultNode)
}

func TestNodeRegistry_HasCapability(t *testing.T) {
	registry := NewNodeRegistry()

	node := &Node{
		ID:           "test-node-1",
		Name:         "Test Node",
		Platform:     PlatformMacOS,
		Capabilities: []NodeCapability{CapabilityCameraSnap, CapabilityLocationGet},
	}

	err := registry.RegisterNode(node)
	require.NoError(t, err)

	// Test existing capabilities
	assert.True(t, registry.HasCapability("test-node-1", CapabilityCameraSnap))
	assert.True(t, registry.HasCapability("test-node-1", CapabilityLocationGet))

	// Test non-existing capability
	assert.False(t, registry.HasCapability("test-node-1", CapabilityNotificationSend))

	// Test non-existing node
	assert.False(t, registry.HasCapability("non-existent", CapabilityCameraSnap))
}

func TestNodeRegistry_Concurrency(t *testing.T) {
	registry := NewNodeRegistry()

	// Register nodes concurrently
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func(id int) {
			node := &Node{
				ID:           fmt.Sprintf("node-%d", id),
				Name:         fmt.Sprintf("Node %d", id),
				Platform:     PlatformMacOS,
				Capabilities: []NodeCapability{CapabilityCameraSnap},
			}
			err := registry.RegisterNode(node)
			assert.NoError(t, err)
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}

	// Verify all nodes were registered
	nodes := registry.ListNodes(nil)
	assert.Equal(t, 10, len(nodes))
}
