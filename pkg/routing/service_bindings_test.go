package routing

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestRoutingServiceResolveRoute_BindingPrecedence(t *testing.T) {
	service := newTestRoutingService(t)

	// Intentionally higher priority to verify binding precedence outranks priority.
	require.NoError(t, service.AddRoute(testBindingRoute("route-channel", "channel-agent", 90, map[string]interface{}{
		"binding": map[string]interface{}{"channel": "telegram"},
	})))
	require.NoError(t, service.AddRoute(testBindingRoute("route-account-any", "account-any-agent", 80, map[string]interface{}{
		"binding": map[string]interface{}{"account_id": "*"},
	})))
	require.NoError(t, service.AddRoute(testBindingRoute("route-account-exact", "account-agent", 70, map[string]interface{}{
		"binding": map[string]interface{}{"account_id": "acct-1"},
	})))
	require.NoError(t, service.AddRoute(testBindingRoute("route-peer", "peer-agent", 10, map[string]interface{}{
		"binding": map[string]interface{}{"peer": "peer-1"},
	})))
	require.NoError(t, service.AddRoute(testBindingRoute("route-default", "default-agent", 1, nil)))

	cases := []struct {
		name       string
		ctx        RoutingContext
		expectedID string
	}{
		{
			name: "peer overrides all lower precedence matches",
			ctx: RoutingContext{
				Content:   "hello",
				Source:    "telegram",
				Channel:   "telegram",
				PeerID:    "peer-1",
				AccountID: "acct-1",
				Timestamp: time.Now(),
			},
			expectedID: "route-peer",
		},
		{
			name: "account exact chosen over wildcard and channel",
			ctx: RoutingContext{
				Content:   "hello",
				Source:    "telegram",
				Channel:   "telegram",
				PeerID:    "other-peer",
				AccountID: "acct-1",
				Timestamp: time.Now(),
			},
			expectedID: "route-account-exact",
		},
		{
			name: "account wildcard chosen over channel",
			ctx: RoutingContext{
				Content:   "hello",
				Source:    "telegram",
				Channel:   "telegram",
				AccountID: "acct-2",
				Timestamp: time.Now(),
			},
			expectedID: "route-account-any",
		},
		{
			name: "channel chosen when higher selectors do not match",
			ctx: RoutingContext{
				Content:   "hello",
				Source:    "telegram",
				Channel:   "telegram",
				Timestamp: time.Now(),
			},
			expectedID: "route-channel",
		},
		{
			name: "default route chosen when no binding route matches",
			ctx: RoutingContext{
				Content:   "hello",
				Source:    "webhook",
				Channel:   "webhook",
				Timestamp: time.Now(),
			},
			expectedID: "route-default",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			route, err := service.ResolveRoute(tc.ctx)
			require.NoError(t, err)
			require.NotNil(t, route)
			require.Equal(t, tc.expectedID, route.ID)
		})
	}
}

func newTestRoutingService(t *testing.T) *RoutingService {
	t.Helper()

	cfg := DefaultRoutingServiceConfig()
	cfg.Storage.FilePath = filepath.Join(t.TempDir(), "routes.json")
	cfg.Storage.AutoSaveInterval = 0
	cfg.Storage.MaxRetries = 1
	cfg.Storage.RetryDelay = time.Millisecond

	return NewRoutingService(cfg)
}

func testBindingRoute(id string, handler string, priority int, metadata map[string]interface{}) *Route {
	return &Route{
		ID:       id,
		Name:     id,
		Handler:  handler,
		Priority: priority,
		Enabled:  true,
		Patterns: []RoutePattern{
			{Type: PatternTypeWildcard, Value: "*"},
		},
		Metadata: metadata,
	}
}
