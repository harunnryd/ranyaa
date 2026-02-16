package routing

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRouteManager_AddRoute(t *testing.T) {
	rm := NewRouteManager(DefaultRouteManagerConfig())

	route := &Route{
		ID:       "test-route",
		Handler:  "test-handler",
		Priority: 50,
		Enabled:  true,
		Patterns: []RoutePattern{
			{Type: "exact", Value: "test"},
		},
	}

	err := rm.AddRoute(route)
	require.NoError(t, err)

	// Verify route was added
	retrieved, err := rm.GetRoute("test-route")
	require.NoError(t, err)
	assert.Equal(t, "test-route", retrieved.ID)
	assert.Equal(t, "test-handler", retrieved.Handler)
	assert.Equal(t, 50, retrieved.Priority)
	assert.True(t, retrieved.Enabled)
}

func TestRouteManager_AddRoute_Duplicate(t *testing.T) {
	rm := NewRouteManager(DefaultRouteManagerConfig())

	route := &Route{
		ID:       "test-route",
		Handler:  "test-handler",
		Priority: 50,
		Enabled:  true,
		Patterns: []RoutePattern{
			{Type: "exact", Value: "test"},
		},
	}

	err := rm.AddRoute(route)
	require.NoError(t, err)

	// Try to add duplicate
	err = rm.AddRoute(route)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already exists")
}

func TestRouteManager_RemoveRoute(t *testing.T) {
	rm := NewRouteManager(DefaultRouteManagerConfig())

	route := &Route{
		ID:       "test-route",
		Handler:  "test-handler",
		Priority: 50,
		Enabled:  true,
		Patterns: []RoutePattern{
			{Type: "exact", Value: "test"},
		},
	}

	err := rm.AddRoute(route)
	require.NoError(t, err)

	// Remove route
	err = rm.RemoveRoute("test-route")
	require.NoError(t, err)

	// Verify route was removed
	_, err = rm.GetRoute("test-route")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestRouteManager_EnableDisableRoute(t *testing.T) {
	rm := NewRouteManager(DefaultRouteManagerConfig())

	route := &Route{
		ID:       "test-route",
		Handler:  "test-handler",
		Priority: 50,
		Enabled:  false,
		Patterns: []RoutePattern{
			{Type: "exact", Value: "test"},
		},
	}

	err := rm.AddRoute(route)
	require.NoError(t, err)

	// Enable route
	err = rm.EnableRoute("test-route")
	require.NoError(t, err)

	retrieved, _ := rm.GetRoute("test-route")
	assert.True(t, retrieved.Enabled)

	// Disable route
	err = rm.DisableRoute("test-route")
	require.NoError(t, err)

	retrieved, _ = rm.GetRoute("test-route")
	assert.False(t, retrieved.Enabled)
}

func TestRouteManager_UpdatePriority(t *testing.T) {
	rm := NewRouteManager(DefaultRouteManagerConfig())

	route := &Route{
		ID:       "test-route",
		Handler:  "test-handler",
		Priority: 50,
		Enabled:  true,
		Patterns: []RoutePattern{
			{Type: "exact", Value: "test"},
		},
	}

	err := rm.AddRoute(route)
	require.NoError(t, err)

	// Update priority
	err = rm.UpdatePriority("test-route", 75)
	require.NoError(t, err)

	retrieved, _ := rm.GetRoute("test-route")
	assert.Equal(t, 75, retrieved.Priority)

	// Verify priority index was updated
	routes := rm.GetRoutesByPriority(75)
	assert.Len(t, routes, 1)
	assert.Equal(t, "test-route", routes[0].ID)

	// Old priority should be empty
	routes = rm.GetRoutesByPriority(50)
	assert.Len(t, routes, 0)
}

func TestRouteManager_GetEnabledRoutes(t *testing.T) {
	rm := NewRouteManager(DefaultRouteManagerConfig())

	routes := []*Route{
		{
			ID:       "route1",
			Handler:  "handler1",
			Priority: 50,
			Enabled:  true,
			Patterns: []RoutePattern{{Type: "exact", Value: "test1"}},
		},
		{
			ID:       "route2",
			Handler:  "handler2",
			Priority: 60,
			Enabled:  false,
			Patterns: []RoutePattern{{Type: "exact", Value: "test2"}},
		},
		{
			ID:       "route3",
			Handler:  "handler3",
			Priority: 70,
			Enabled:  true,
			Patterns: []RoutePattern{{Type: "exact", Value: "test3"}},
		},
	}

	for _, route := range routes {
		err := rm.AddRoute(route)
		require.NoError(t, err)
	}

	enabledRoutes := rm.GetEnabledRoutes()
	assert.Len(t, enabledRoutes, 2)

	// Verify only enabled routes are returned
	ids := make(map[string]bool)
	for _, route := range enabledRoutes {
		ids[route.ID] = true
	}
	assert.True(t, ids["route1"])
	assert.False(t, ids["route2"])
	assert.True(t, ids["route3"])
}

func TestRouteManager_ListRoutes_WithFilter(t *testing.T) {
	rm := NewRouteManager(DefaultRouteManagerConfig())

	routes := []*Route{
		{
			ID:       "route1",
			Handler:  "handler1",
			Priority: 50,
			Enabled:  true,
			Patterns: []RoutePattern{{Type: "exact", Value: "test1"}},
		},
		{
			ID:       "route2",
			Handler:  "handler2",
			Priority: 60,
			Enabled:  false,
			Patterns: []RoutePattern{{Type: "exact", Value: "test2"}},
		},
		{
			ID:       "route3",
			Handler:  "handler1",
			Priority: 50,
			Enabled:  true,
			Patterns: []RoutePattern{{Type: "exact", Value: "test3"}},
		},
	}

	for _, route := range routes {
		err := rm.AddRoute(route)
		require.NoError(t, err)
	}

	// Filter by enabled
	enabled := true
	filter := &RouteFilter{Enabled: &enabled}
	filtered := rm.ListRoutes(filter)
	assert.Len(t, filtered, 2)

	// Filter by priority
	priority := 50
	filter = &RouteFilter{Priority: &priority}
	filtered = rm.ListRoutes(filter)
	assert.Len(t, filtered, 2)

	// Filter by handler
	handler := "handler1"
	filter = &RouteFilter{Handler: &handler}
	filtered = rm.ListRoutes(filter)
	assert.Len(t, filtered, 2)
}

func TestRouteManager_ValidateRoute_RequiredFields(t *testing.T) {
	rm := NewRouteManager(DefaultRouteManagerConfig())

	tests := []struct {
		name    string
		route   *Route
		wantErr string
	}{
		{
			name: "missing ID",
			route: &Route{
				Handler:  "handler",
				Patterns: []RoutePattern{{Type: "exact", Value: "test"}},
			},
			wantErr: "route ID is required",
		},
		{
			name: "missing handler",
			route: &Route{
				ID:       "route",
				Patterns: []RoutePattern{{Type: "exact", Value: "test"}},
			},
			wantErr: "handler is required",
		},
		{
			name: "missing patterns",
			route: &Route{
				ID:       "route",
				Handler:  "handler",
				Patterns: []RoutePattern{},
			},
			wantErr: "at least one pattern is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := rm.AddRoute(tt.route)
			assert.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestRouteManager_ValidatePattern(t *testing.T) {
	rm := NewRouteManager(DefaultRouteManagerConfig())

	tests := []struct {
		name    string
		pattern RoutePattern
		wantErr string
	}{
		{
			name:    "missing type",
			pattern: RoutePattern{Value: "test"},
			wantErr: "pattern type is required",
		},
		{
			name:    "missing value",
			pattern: RoutePattern{Type: "exact"},
			wantErr: "pattern value is required",
		},
		{
			name:    "invalid type",
			pattern: RoutePattern{Type: "invalid", Value: "test"},
			wantErr: "invalid pattern type",
		},
		{
			name:    "invalid regex syntax",
			pattern: RoutePattern{Type: "regex", Value: "[invalid"},
			wantErr: "invalid regex syntax",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			route := &Route{
				ID:       "test",
				Handler:  "handler",
				Patterns: []RoutePattern{tt.pattern},
			}

			err := rm.AddRoute(route)
			assert.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestRouteManager_DetectCircularFallbacks(t *testing.T) {
	rm := NewRouteManager(DefaultRouteManagerConfig())

	// Add route1
	route1 := &Route{
		ID:       "route1",
		Handler:  "handler1",
		Patterns: []RoutePattern{{Type: "exact", Value: "test1"}},
	}
	err := rm.AddRoute(route1)
	require.NoError(t, err)

	// Add route2 with fallback to route1
	route2 := &Route{
		ID:             "route2",
		Handler:        "handler2",
		Patterns:       []RoutePattern{{Type: "exact", Value: "test2"}},
		FallbackRoutes: []string{"route1"},
	}
	err = rm.AddRoute(route2)
	require.NoError(t, err)

	// Try to add route3 with circular fallback (route3 -> route2 -> route1 -> route3)
	route3 := &Route{
		ID:             "route3",
		Handler:        "handler3",
		Patterns:       []RoutePattern{{Type: "exact", Value: "test3"}},
		FallbackRoutes: []string{"route2"},
	}
	err = rm.AddRoute(route3)
	require.NoError(t, err)

	// Now try to update route1 to fallback to route3 (creates cycle)
	err = rm.RemoveRoute("route1")
	require.NoError(t, err)

	route1.FallbackRoutes = []string{"route3"}
	err = rm.AddRoute(route1)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "circular fallback")
}

func TestRouteManager_SanitizeString(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "null bytes",
			input:    "test\x00value",
			expected: "testvalue",
		},
		{
			name:     "control characters",
			input:    "test\x01\x02value",
			expected: "testvalue",
		},
		{
			name:     "preserve newline and tab",
			input:    "test\n\tvalue",
			expected: "test\n\tvalue",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := sanitizeString(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestRouteManager_ValidateTimeFormat(t *testing.T) {
	tests := []struct {
		name     string
		timeStr  string
		expected bool
	}{
		{"valid time", "09:30", true},
		{"valid midnight", "00:00", true},
		{"valid end of day", "23:59", true},
		{"invalid hour", "24:00", false},
		{"invalid minute", "12:60", false},
		{"invalid format", "9:30", false},
		{"missing colon", "0930", false},
		{"non-numeric", "ab:cd", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isValidTimeFormat(tt.timeStr)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestRouteManager_ValidateRegexComplexity(t *testing.T) {
	tests := []struct {
		name    string
		pattern string
		wantErr bool
	}{
		{
			name:    "simple pattern",
			pattern: "^test.*$",
			wantErr: false,
		},
		{
			name:    "nested quantifiers",
			pattern: "(a+)+",
			wantErr: true,
		},
		{
			name: "excessive alternations",
			pattern: "a|b|c|d|e|f|g|h|i|j|k|l|m|n|o|p|q|r|s|t|u|v|w|x|y|z|" +
				"a|b|c|d|e|f|g|h|i|j|k|l|m|n|o|p|q|r|s|t|u|v|w|x|y|z|" +
				"a|b|c|d|e|f|g|h|i|j|k|l|m|n|o|p|q|r|s|t|u|v|w|x|y|z",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateRegexComplexity(tt.pattern)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
