package channels

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
)

// Registry stores registered channels and dispatches inbound messages.
type Registry struct {
	dispatch DispatchFunc

	mu       sync.RWMutex
	channels map[string]Channel
	started  map[string]bool
}

// NewRegistry constructs a channel registry.
func NewRegistry(dispatch DispatchFunc) *Registry {
	return &Registry{
		dispatch: dispatch,
		channels: make(map[string]Channel),
		started:  make(map[string]bool),
	}
}

// Register adds a channel to the registry.
func (r *Registry) Register(ch Channel) error {
	if ch == nil {
		return fmt.Errorf("channel is required")
	}

	name := strings.TrimSpace(ch.Name())
	if name == "" {
		return fmt.Errorf("channel name is required")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.channels[name]; exists {
		return fmt.Errorf("channel %q already registered", name)
	}

	r.channels[name] = ch
	return nil
}

// IsRegistered returns true when channel exists in the registry.
func (r *Registry) IsRegistered(name string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, ok := r.channels[strings.TrimSpace(name)]
	return ok
}

// Names returns sorted registered channel names.
func (r *Registry) Names() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	names := make([]string, 0, len(r.channels))
	for name := range r.channels {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// Dispatch forwards an inbound message to runtime dispatcher.
func (r *Registry) Dispatch(ctx context.Context, msg InboundMessage) (interface{}, error) {
	if r.dispatch == nil {
		return nil, fmt.Errorf("dispatch function is not configured")
	}

	msg.Channel = strings.TrimSpace(msg.Channel)
	if msg.Channel == "" {
		return nil, fmt.Errorf("channel is required")
	}
	if !r.IsRegistered(msg.Channel) {
		return nil, fmt.Errorf("channel %q is not registered", msg.Channel)
	}

	return r.dispatch(ctx, msg)
}

// StartAll starts all registered channels.
func (r *Registry) StartAll(ctx context.Context) error {
	for _, name := range r.Names() {
		if err := r.Start(ctx, name); err != nil {
			return err
		}
	}
	return nil
}

// StopAll stops all registered channels.
func (r *Registry) StopAll(ctx context.Context) error {
	var firstErr error
	names := r.Names()
	for i := len(names) - 1; i >= 0; i-- {
		if err := r.Stop(ctx, names[i]); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// Start starts a registered channel by name.
func (r *Registry) Start(ctx context.Context, name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("channel name is required")
	}

	r.mu.Lock()
	ch, ok := r.channels[name]
	if !ok {
		r.mu.Unlock()
		return fmt.Errorf("channel %q is not registered", name)
	}
	if r.started[name] {
		r.mu.Unlock()
		return nil
	}
	r.mu.Unlock()

	if err := ch.Start(ctx, r.dispatch); err != nil {
		return fmt.Errorf("failed to start channel %q: %w", name, err)
	}

	r.mu.Lock()
	r.started[name] = true
	r.mu.Unlock()

	return nil
}

// Stop stops a registered channel by name.
func (r *Registry) Stop(ctx context.Context, name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("channel name is required")
	}

	r.mu.Lock()
	ch, ok := r.channels[name]
	if !ok {
		r.mu.Unlock()
		return fmt.Errorf("channel %q is not registered", name)
	}
	if !r.started[name] {
		r.mu.Unlock()
		return nil
	}
	r.mu.Unlock()

	if err := ch.Stop(ctx); err != nil {
		return fmt.Errorf("failed to stop channel %q: %w", name, err)
	}

	r.mu.Lock()
	delete(r.started, name)
	r.mu.Unlock()

	return nil
}
