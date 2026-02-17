package channels

import (
	"context"
	"fmt"
	"strings"
)

// DirectChannel is a no-op channel implementation used for direct ingress paths.
type DirectChannel struct {
	name string
}

// NewDirectChannel creates a direct channel by name.
func NewDirectChannel(name string) *DirectChannel {
	return &DirectChannel{name: strings.TrimSpace(name)}
}

// Name returns channel name.
func (c *DirectChannel) Name() string {
	return c.name
}

// Start validates dispatcher availability.
func (c *DirectChannel) Start(_ context.Context, dispatch DispatchFunc) error {
	if strings.TrimSpace(c.name) == "" {
		return fmt.Errorf("channel name is required")
	}
	if dispatch == nil {
		return fmt.Errorf("dispatch function is required")
	}
	return nil
}

// Stop is a no-op for direct channels.
func (c *DirectChannel) Stop(_ context.Context) error {
	return nil
}
