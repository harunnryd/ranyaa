package gateway

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/harun/ranya/pkg/agent"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type stubEventHub struct {
	mu   sync.Mutex
	subs map[string][]chan agent.RuntimeEvent
}

func newStubEventHub() *stubEventHub {
	return &stubEventHub{subs: make(map[string][]chan agent.RuntimeEvent)}
}

func (h *stubEventHub) Subscribe(sessionKey string, buffer int) (<-chan agent.RuntimeEvent, func()) {
	ch := make(chan agent.RuntimeEvent, buffer)
	h.mu.Lock()
	h.subs[sessionKey] = append(h.subs[sessionKey], ch)
	h.mu.Unlock()

	return ch, func() {
		h.mu.Lock()
		defer h.mu.Unlock()
		channels := h.subs[sessionKey]
		filtered := make([]chan agent.RuntimeEvent, 0, len(channels))
		for _, existing := range channels {
			if existing != ch {
				filtered = append(filtered, existing)
			}
		}
		if len(filtered) == 0 {
			delete(h.subs, sessionKey)
		} else {
			h.subs[sessionKey] = filtered
		}
		close(ch)
	}
}

func (h *stubEventHub) Publish(sessionKey string, evt agent.RuntimeEvent) {
	h.mu.Lock()
	subs := append([]chan agent.RuntimeEvent{}, h.subs[sessionKey]...)
	h.mu.Unlock()

	for _, ch := range subs {
		ch <- evt
	}
}

func TestHandleAgentStream_SendsSSEEvents(t *testing.T) {
	hub := newStubEventHub()

	srv := &Server{
		sharedSecret:    "secret",
		runtimeEventHub: hub,
		agentDispatcher: func(ctx context.Context, req AgentDispatchRequest) (agent.AgentResult, error) {
			hub.Publish(req.SessionKey, agent.RuntimeEvent{Stream: agent.RuntimeStreamAssistant, Phase: "output", Content: "hello"})
			hub.Publish(req.SessionKey, agent.RuntimeEvent{Stream: agent.RuntimeStreamTool, Phase: "start", ToolName: "calculator", ToolCall: "call-1"})
			return agent.AgentResult{Response: "done", SessionKey: req.SessionKey}, nil
		},
	}

	testServer := httptest.NewServer(http.HandlerFunc(srv.handleAgentStream))
	defer testServer.Close()

	payload := map[string]interface{}{
		"prompt":     "hi",
		"sessionKey": "stream:test",
	}
	body, err := json.Marshal(payload)
	require.NoError(t, err)

	req, err := http.NewRequest(http.MethodPost, testServer.URL, bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("X-Ranya-Secret", "secret")

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, "text/event-stream", resp.Header.Get("Content-Type"))

	scanner := bufio.NewScanner(resp.Body)
	events := []string{}
	deadline := time.Now().Add(2 * time.Second)

	for time.Now().Before(deadline) && scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "event:") {
			eventName := strings.TrimSpace(strings.TrimPrefix(line, "event:"))
			events = append(events, eventName)
			if eventName == "complete" {
				break
			}
		}
	}

	assert.Contains(t, events, "connected")
	assert.Contains(t, events, "token")
	assert.Contains(t, events, "tool_call")
	assert.Contains(t, events, "complete")
}
