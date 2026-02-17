package gateway

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEventBroadcaster_BroadcastTypedAddsSequence(t *testing.T) {
	serverConn, clientConn, cleanup := websocketConnPair(t)
	defer cleanup()

	registry := NewClientRegistry()
	registry.Add(&Client{
		ID:            "client-1",
		Conn:          serverConn,
		Authenticated: true,
	})

	broadcaster := NewEventBroadcaster(registry, zerolog.Nop())
	broadcaster.BroadcastTyped(EventMessage{
		Event:   "tool",
		Stream:  StreamTypeTool,
		Phase:   "start",
		Data:    map[string]interface{}{"tool": "example"},
		TraceID: "trace-1",
		RunID:   "run-1",
	})
	broadcaster.BroadcastTyped(EventMessage{
		Event:   "tool",
		Stream:  StreamTypeTool,
		Phase:   "end",
		Data:    map[string]interface{}{"tool": "example"},
		TraceID: "trace-1",
		RunID:   "run-1",
	})

	var first EventMessage
	require.NoError(t, clientConn.SetReadDeadline(time.Now().Add(2*time.Second)))
	require.NoError(t, clientConn.ReadJSON(&first))

	var second EventMessage
	require.NoError(t, clientConn.SetReadDeadline(time.Now().Add(2*time.Second)))
	require.NoError(t, clientConn.ReadJSON(&second))

	assert.Equal(t, "event", first.Type)
	assert.Equal(t, "tool", first.Event)
	assert.Equal(t, StreamTypeTool, first.Stream)
	assert.Equal(t, "start", first.Phase)
	assert.NotZero(t, first.Seq)
	assert.Equal(t, "trace-1", first.TraceID)
	assert.Equal(t, "run-1", first.RunID)

	assert.Equal(t, "event", second.Type)
	assert.Equal(t, "tool", second.Event)
	assert.Equal(t, StreamTypeTool, second.Stream)
	assert.Equal(t, "end", second.Phase)
	assert.Greater(t, second.Seq, first.Seq)
}

func TestEventBroadcaster_BroadcastAssignsTypeAndSequence(t *testing.T) {
	serverConn, clientConn, cleanup := websocketConnPair(t)
	defer cleanup()

	registry := NewClientRegistry()
	registry.Add(&Client{
		ID:            "client-1",
		Conn:          serverConn,
		Authenticated: true,
	})

	broadcaster := NewEventBroadcaster(registry, zerolog.Nop())
	broadcaster.Broadcast("session.message", map[string]interface{}{"ok": true})

	var event EventMessage
	require.NoError(t, clientConn.SetReadDeadline(time.Now().Add(2*time.Second)))
	require.NoError(t, clientConn.ReadJSON(&event))

	assert.Equal(t, "event", event.Type)
	assert.Equal(t, "session.message", event.Event)
	assert.NotZero(t, event.Seq)
	assert.NotZero(t, event.Timestamp)
}

func websocketConnPair(t *testing.T) (*websocket.Conn, *websocket.Conn, func()) {
	t.Helper()

	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	serverConnCh := make(chan *websocket.Conn, 1)
	errCh := make(chan error, 1)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			errCh <- err
			return
		}
		serverConnCh <- conn
	}))

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")
	clientConn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	require.NoError(t, err)

	var serverConn *websocket.Conn
	select {
	case serverConn = <-serverConnCh:
	case err := <-errCh:
		require.NoError(t, err)
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for server websocket connection")
	}

	cleanup := func() {
		_ = clientConn.Close()
		_ = serverConn.Close()
		srv.Close()
	}

	return serverConn, clientConn, cleanup
}
