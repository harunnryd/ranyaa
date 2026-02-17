package channels

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type testChannel struct {
	name       string
	startCalls int
	stopCalls  int
}

func (c *testChannel) Name() string {
	return c.name
}

func (c *testChannel) Start(_ context.Context, dispatch DispatchFunc) error {
	if dispatch == nil {
		return assert.AnError
	}
	c.startCalls++
	return nil
}

func (c *testChannel) Stop(_ context.Context) error {
	c.stopCalls++
	return nil
}

func TestRegistry_RegisterStartDispatchStop(t *testing.T) {
	dispatched := 0
	reg := NewRegistry(func(_ context.Context, msg InboundMessage) (interface{}, error) {
		dispatched++
		return msg.Channel + ":" + msg.Content, nil
	})

	ch := &testChannel{name: "gateway"}
	require.NoError(t, reg.Register(ch))
	assert.True(t, reg.IsRegistered("gateway"))
	assert.Equal(t, []string{"gateway"}, reg.Names())

	require.NoError(t, reg.StartAll(context.Background()))
	assert.Equal(t, 1, ch.startCalls)

	result, err := reg.Dispatch(context.Background(), InboundMessage{
		Channel: "gateway",
		Content: "hello",
	})
	require.NoError(t, err)
	assert.Equal(t, "gateway:hello", result)
	assert.Equal(t, 1, dispatched)

	require.NoError(t, reg.StopAll(context.Background()))
	assert.Equal(t, 1, ch.stopCalls)
}

func TestRegistry_DispatchUnknownChannel(t *testing.T) {
	reg := NewRegistry(func(_ context.Context, msg InboundMessage) (interface{}, error) {
		return msg, nil
	})

	_, err := reg.Dispatch(context.Background(), InboundMessage{
		Channel: "telegram",
		Content: "ping",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not registered")
}

func TestRegistry_RejectsDuplicateChannel(t *testing.T) {
	reg := NewRegistry(func(_ context.Context, msg InboundMessage) (interface{}, error) {
		return msg, nil
	})

	require.NoError(t, reg.Register(&testChannel{name: "gateway"}))
	err := reg.Register(&testChannel{name: "gateway"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "already registered")
}
