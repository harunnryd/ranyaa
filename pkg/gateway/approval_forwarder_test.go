package gateway

import (
	"context"
	"testing"
	"time"

	"github.com/harun/ranya/pkg/toolexecutor"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestApprovalForwarder_EmitsApprovalRequiredEvent(t *testing.T) {
	serverConn, clientConn, cleanup := websocketConnPair(t)
	defer cleanup()

	registry := NewClientRegistry()
	registry.Add(&Client{
		ID:            "client-1",
		Conn:          serverConn,
		Authenticated: true,
	})

	broadcaster := NewEventBroadcaster(registry, zerolog.Nop())
	server := &Server{broadcaster: broadcaster}
	forwarder := NewApprovalForwarder(server)

	err := forwarder.ForwardApproval(context.Background(), toolexecutor.PendingApproval{
		ID: "approval-1",
		Request: toolexecutor.ApprovalRequest{
			Command: "execute_shell_command",
			Args:    []string{"rm", "-rf", "/"},
			Timeout: 5 * time.Second,
			AgentID: "agent-1",
		},
		CreatedAt: time.Now(),
	})
	require.NoError(t, err)

	var evt EventMessage
	require.NoError(t, clientConn.SetReadDeadline(time.Now().Add(2*time.Second)))
	require.NoError(t, clientConn.ReadJSON(&evt))

	assert.Equal(t, "approval_required", evt.Event)
	assert.Equal(t, StreamTypeTool, evt.Stream)
	assert.Equal(t, "approval_required", evt.Phase)
}
