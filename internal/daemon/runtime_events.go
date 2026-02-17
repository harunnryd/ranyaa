package daemon

import (
	"strings"
	"sync"

	"github.com/harun/ranya/pkg/agent"
)

type runtimeEventHub struct {
	mu          sync.RWMutex
	subscribers map[string]map[uint64]chan agent.RuntimeEvent
	nextID      uint64
}

func newRuntimeEventHub() *runtimeEventHub {
	return &runtimeEventHub{
		subscribers: make(map[string]map[uint64]chan agent.RuntimeEvent),
	}
}

func (h *runtimeEventHub) Subscribe(sessionKey string, buffer int) (<-chan agent.RuntimeEvent, func()) {
	sessionKey = strings.TrimSpace(sessionKey)
	if sessionKey == "" {
		ch := make(chan agent.RuntimeEvent)
		close(ch)
		return ch, func() {}
	}
	if buffer <= 0 {
		buffer = 64
	}

	ch := make(chan agent.RuntimeEvent, buffer)

	h.mu.Lock()
	h.nextID++
	subID := h.nextID
	if _, exists := h.subscribers[sessionKey]; !exists {
		h.subscribers[sessionKey] = make(map[uint64]chan agent.RuntimeEvent)
	}
	h.subscribers[sessionKey][subID] = ch
	h.mu.Unlock()

	cancel := func() {
		h.mu.Lock()
		defer h.mu.Unlock()
		subs, ok := h.subscribers[sessionKey]
		if !ok {
			return
		}
		sub, exists := subs[subID]
		if !exists {
			return
		}
		delete(subs, subID)
		if len(subs) == 0 {
			delete(h.subscribers, sessionKey)
		}
		close(sub)
	}

	return ch, cancel
}

func (h *runtimeEventHub) Publish(sessionKey string, evt agent.RuntimeEvent) {
	sessionKey = strings.TrimSpace(sessionKey)
	if sessionKey == "" {
		return
	}

	h.mu.RLock()
	subs := h.subscribers[sessionKey]
	for _, sub := range subs {
		select {
		case sub <- evt:
		default:
		}
	}
	h.mu.RUnlock()
}
