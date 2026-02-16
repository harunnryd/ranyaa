package routing

import (
	"fmt"
	"regexp"
	"strings"
	"sync"

	"github.com/rs/zerolog/log"
)

// RouteManager manages route storage, validation, and indexing
type RouteManager struct {
	routes           map[string]*Route // routeID -> Route
	routesByPriority map[int][]*Route  // priority -> []Route
	enabledRoutes    map[string]bool   // routeID -> enabled
	mu               sync.RWMutex
	config           RouteManagerConfig
}

// RouteManagerConfig holds configuration for RouteManager
type RouteManagerConfig struct {
	MaxRoutes      int
	MaxPriority    int
	MinPriority    int
	MaxRegexLength int
}

// DefaultRouteManagerConfig returns default configuration
func DefaultRouteManagerConfig() RouteManagerConfig {
	return RouteManagerConfig{
		MaxRoutes:      10000,
		MaxPriority:    100,
		MinPriority:    0,
		MaxRegexLength: 500,
	}
}

// NewRouteManager creates a new RouteManager
func NewRouteManager(config RouteManagerConfig) *RouteManager {
	return &RouteManager{
		routes:           make(map[string]*Route),
		routesByPriority: make(map[int][]*Route),
		enabledRoutes:    make(map[string]bool),
		config:           config,
	}
}

// AddRoute adds a new route with validation
func (rm *RouteManager) AddRoute(route *Route) error {
	if route == nil {
		return fmt.Errorf("route cannot be nil")
	}

	// Validate route
	if err := rm.validateRoute(route); err != nil {
		return fmt.Errorf("route validation failed: %w", err)
	}

	rm.mu.Lock()
	defer rm.mu.Unlock()

	// Check if route already exists
	if _, exists := rm.routes[route.ID]; exists {
		return fmt.Errorf("route with ID %s already exists", route.ID)
	}

	// Check max routes limit
	if len(rm.routes) >= rm.config.MaxRoutes {
		return fmt.Errorf("maximum number of routes (%d) reached", rm.config.MaxRoutes)
	}

	// Detect circular fallbacks
	if err := rm.detectCircularFallbacks(route); err != nil {
		return err
	}

	// Add to routes map
	rm.routes[route.ID] = route

	// Add to priority index
	rm.routesByPriority[route.Priority] = append(rm.routesByPriority[route.Priority], route)

	// Add to enabled index if enabled
	if route.Enabled {
		rm.enabledRoutes[route.ID] = true
	}

	log.Debug().
		Str("routeId", route.ID).
		Int("priority", route.Priority).
		Bool("enabled", route.Enabled).
		Msg("Route added")

	return nil
}

// RemoveRoute removes a route by ID
func (rm *RouteManager) RemoveRoute(routeID string) error {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	route, exists := rm.routes[routeID]
	if !exists {
		return fmt.Errorf("route %s not found", routeID)
	}

	// Remove from routes map
	delete(rm.routes, routeID)

	// Remove from priority index
	routes := rm.routesByPriority[route.Priority]
	for i, r := range routes {
		if r.ID == routeID {
			rm.routesByPriority[route.Priority] = append(routes[:i], routes[i+1:]...)
			break
		}
	}

	// Clean up empty priority slice
	if len(rm.routesByPriority[route.Priority]) == 0 {
		delete(rm.routesByPriority, route.Priority)
	}

	// Remove from enabled index
	delete(rm.enabledRoutes, routeID)

	log.Debug().Str("routeId", routeID).Msg("Route removed")

	return nil
}

// GetRoute retrieves a route by ID
func (rm *RouteManager) GetRoute(routeID string) (*Route, error) {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	route, exists := rm.routes[routeID]
	if !exists {
		return nil, fmt.Errorf("route %s not found", routeID)
	}

	return route, nil
}

// ListRoutes returns all routes with optional filtering
func (rm *RouteManager) ListRoutes(filter *RouteFilter) []*Route {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	routes := make([]*Route, 0, len(rm.routes))

	for _, route := range rm.routes {
		// Apply filters
		if filter != nil {
			if filter.Enabled != nil && route.Enabled != *filter.Enabled {
				continue
			}
			if filter.Priority != nil && route.Priority != *filter.Priority {
				continue
			}
			if filter.Handler != nil && route.Handler != *filter.Handler {
				continue
			}
		}

		routes = append(routes, route)
	}

	return routes
}

// RouteFilterCriteria holds filtering criteria for ListRoutes
type RouteFilterCriteria struct {
	Enabled  *bool
	Priority *int
	Handler  *string
}

// EnableRoute enables a route by ID
func (rm *RouteManager) EnableRoute(routeID string) error {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	route, exists := rm.routes[routeID]
	if !exists {
		return fmt.Errorf("route %s not found", routeID)
	}

	if route.Enabled {
		return nil // Already enabled
	}

	route.Enabled = true
	rm.enabledRoutes[routeID] = true

	log.Debug().Str("routeId", routeID).Msg("Route enabled")

	return nil
}

// DisableRoute disables a route by ID
func (rm *RouteManager) DisableRoute(routeID string) error {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	route, exists := rm.routes[routeID]
	if !exists {
		return fmt.Errorf("route %s not found", routeID)
	}

	if !route.Enabled {
		return nil // Already disabled
	}

	route.Enabled = false
	delete(rm.enabledRoutes, routeID)

	log.Debug().Str("routeId", routeID).Msg("Route disabled")

	return nil
}

// UpdatePriority updates the priority of a route
func (rm *RouteManager) UpdatePriority(routeID string, newPriority int) error {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	route, exists := rm.routes[routeID]
	if !exists {
		return fmt.Errorf("route %s not found", routeID)
	}

	// Validate priority range
	if newPriority < rm.config.MinPriority || newPriority > rm.config.MaxPriority {
		return fmt.Errorf("priority %d out of range [%d, %d]", newPriority, rm.config.MinPriority, rm.config.MaxPriority)
	}

	oldPriority := route.Priority

	// Remove from old priority index
	routes := rm.routesByPriority[oldPriority]
	for i, r := range routes {
		if r.ID == routeID {
			rm.routesByPriority[oldPriority] = append(routes[:i], routes[i+1:]...)
			break
		}
	}

	// Clean up empty priority slice
	if len(rm.routesByPriority[oldPriority]) == 0 {
		delete(rm.routesByPriority, oldPriority)
	}

	// Update route priority
	route.Priority = newPriority

	// Add to new priority index
	rm.routesByPriority[newPriority] = append(rm.routesByPriority[newPriority], route)

	log.Debug().
		Str("routeId", routeID).
		Int("oldPriority", oldPriority).
		Int("newPriority", newPriority).
		Msg("Route priority updated")

	return nil
}

// GetEnabledRoutes returns all enabled routes
func (rm *RouteManager) GetEnabledRoutes() []*Route {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	routes := make([]*Route, 0, len(rm.enabledRoutes))

	for routeID := range rm.enabledRoutes {
		if route, exists := rm.routes[routeID]; exists {
			routes = append(routes, route)
		}
	}

	return routes
}

// GetRoutesByPriority returns all routes at a specific priority level
func (rm *RouteManager) GetRoutesByPriority(priority int) []*Route {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	routes := rm.routesByPriority[priority]
	if routes == nil {
		return []*Route{}
	}

	// Return a copy to prevent external modification
	result := make([]*Route, len(routes))
	copy(result, routes)

	return result
}

// validateRoute performs comprehensive validation on a route
func (rm *RouteManager) validateRoute(route *Route) error {
	// Required fields validation
	if route.ID == "" {
		return fmt.Errorf("route ID is required")
	}
	if route.Handler == "" {
		return fmt.Errorf("handler is required")
	}
	if len(route.Patterns) == 0 {
		return fmt.Errorf("at least one pattern is required")
	}

	// Field type validation
	if route.Priority < rm.config.MinPriority || route.Priority > rm.config.MaxPriority {
		return fmt.Errorf("priority %d out of range [%d, %d]", route.Priority, rm.config.MinPriority, rm.config.MaxPriority)
	}

	// Pattern validation
	for i, pattern := range route.Patterns {
		if err := rm.validatePattern(&pattern); err != nil {
			return fmt.Errorf("pattern %d validation failed: %w", i, err)
		}
	}

	// Condition validation
	for i, condition := range route.Conditions {
		if err := rm.validateCondition(&condition); err != nil {
			return fmt.Errorf("condition %d validation failed: %w", i, err)
		}
	}

	// String input sanitization
	route.ID = sanitizeString(route.ID)
	route.Handler = sanitizeString(route.Handler)
	route.Description = sanitizeString(route.Description)

	return nil
}

// validatePattern validates a route pattern
func (rm *RouteManager) validatePattern(pattern *RoutePattern) error {
	// Required fields
	if pattern.Type == "" {
		return fmt.Errorf("pattern type is required")
	}
	if pattern.Value == "" {
		return fmt.Errorf("pattern value is required")
	}

	// Validate pattern type
	validTypes := map[PatternType]bool{
		PatternTypeExact:    true,
		PatternTypeWildcard: true,
		PatternTypeRegex:    true,
		PatternTypePrefix:   true,
		PatternTypeSuffix:   true,
	}

	if !validTypes[pattern.Type] {
		return fmt.Errorf("invalid pattern type: %s", pattern.Type)
	}

	// Regex-specific validation
	if pattern.Type == "regex" {
		// Length validation
		if len(pattern.Value) > rm.config.MaxRegexLength {
			return fmt.Errorf("regex pattern exceeds maximum length of %d", rm.config.MaxRegexLength)
		}

		// Syntax validation
		if _, err := regexp.Compile(pattern.Value); err != nil {
			return fmt.Errorf("invalid regex syntax: %w", err)
		}

		// Complexity validation (basic check for dangerous patterns)
		if err := validateRegexComplexity(pattern.Value); err != nil {
			return err
		}
	}

	// Sanitize strings
	pattern.Value = sanitizeString(pattern.Value)

	return nil
}

// validateCondition validates a route condition
func (rm *RouteManager) validateCondition(condition *RouteCondition) error {
	// Required fields
	if condition.Type == "" {
		return fmt.Errorf("condition type is required")
	}

	// Validate condition type
	validTypes := map[string]bool{
		"time":     true,
		"content":  true,
		"metadata": true,
		"custom":   true,
	}

	if !validTypes[condition.Type] {
		return fmt.Errorf("invalid condition type: %s", condition.Type)
	}

	// Type-specific validation
	switch condition.Type {
	case "time":
		if condition.TimeRange == nil {
			return fmt.Errorf("time condition requires timeRange")
		}
		// Validate time format (HH:MM)
		if !isValidTimeFormat(condition.TimeRange.Start) {
			return fmt.Errorf("invalid start time format: %s (expected HH:MM)", condition.TimeRange.Start)
		}
		if !isValidTimeFormat(condition.TimeRange.End) {
			return fmt.Errorf("invalid end time format: %s (expected HH:MM)", condition.TimeRange.End)
		}

	case "content":
		if condition.ContentMatch == nil {
			return fmt.Errorf("content condition requires contentMatch")
		}
		if condition.ContentMatch.Field == "" {
			return fmt.Errorf("content match field is required")
		}
		if condition.ContentMatch.Pattern == "" {
			return fmt.Errorf("content match pattern is required")
		}

	case "metadata":
		if condition.MetadataMatch == nil {
			return fmt.Errorf("metadata condition requires metadataMatch")
		}
		if condition.MetadataMatch.Key == "" {
			return fmt.Errorf("metadata match key is required")
		}

	case "custom":
		if condition.CustomFunction == "" {
			return fmt.Errorf("custom condition requires customFunction")
		}
	}

	return nil
}

// detectCircularFallbacks detects circular dependencies in fallback chains
func (rm *RouteManager) detectCircularFallbacks(route *Route) error {
	if len(route.FallbackRoutes) == 0 {
		return nil
	}

	visited := make(map[string]bool)
	return rm.detectCircularFallbacksRecursive(route.ID, route.FallbackRoutes, visited)
}

// detectCircularFallbacksRecursive performs DFS to detect cycles
func (rm *RouteManager) detectCircularFallbacksRecursive(startID string, fallbacks []string, visited map[string]bool) error {
	for _, fallbackID := range fallbacks {
		// Check if we've returned to the starting route
		if fallbackID == startID {
			return fmt.Errorf("circular fallback detected: route %s references itself", startID)
		}

		// Check if we've visited this route in the current path
		if visited[fallbackID] {
			return fmt.Errorf("circular fallback detected involving route %s", fallbackID)
		}

		// Mark as visited
		visited[fallbackID] = true

		// Get fallback route
		fallbackRoute, exists := rm.routes[fallbackID]
		if exists && len(fallbackRoute.FallbackRoutes) > 0 {
			// Recursively check fallback's fallbacks
			if err := rm.detectCircularFallbacksRecursive(startID, fallbackRoute.FallbackRoutes, visited); err != nil {
				return err
			}
		}

		// Unmark for other paths
		delete(visited, fallbackID)
	}

	return nil
}

// sanitizeString removes potentially dangerous characters from strings
func sanitizeString(s string) string {
	// Remove null bytes
	s = strings.ReplaceAll(s, "\x00", "")

	// Remove control characters except newline and tab
	var result strings.Builder
	for _, r := range s {
		if r >= 32 || r == '\n' || r == '\t' {
			result.WriteRune(r)
		}
	}

	return result.String()
}

// isValidTimeFormat checks if a time string is in HH:MM format
func isValidTimeFormat(timeStr string) bool {
	parts := strings.Split(timeStr, ":")
	if len(parts) != 2 {
		return false
	}

	// Check hours (00-23)
	if len(parts[0]) != 2 {
		return false
	}
	hour := 0
	for _, c := range parts[0] {
		if c < '0' || c > '9' {
			return false
		}
		hour = hour*10 + int(c-'0')
	}
	if hour > 23 {
		return false
	}

	// Check minutes (00-59)
	if len(parts[1]) != 2 {
		return false
	}
	minute := 0
	for _, c := range parts[1] {
		if c < '0' || c > '9' {
			return false
		}
		minute = minute*10 + int(c-'0')
	}
	if minute > 59 {
		return false
	}

	return true
}

// validateRegexComplexity checks for potentially dangerous regex patterns
func validateRegexComplexity(pattern string) error {
	// Check for nested quantifiers (e.g., (a+)+, (a*)+)
	nestedQuantifiers := regexp.MustCompile(`\([^)]*[*+]\)[*+]`)
	if nestedQuantifiers.MatchString(pattern) {
		return fmt.Errorf("regex contains nested quantifiers which may cause catastrophic backtracking")
	}

	// Check for excessive alternation
	alternations := strings.Count(pattern, "|")
	if alternations > 50 {
		return fmt.Errorf("regex contains too many alternations (%d), maximum is 50", alternations)
	}

	// Check for excessive repetition ranges
	largeRange := regexp.MustCompile(`\{(\d+),(\d+)\}`)
	matches := largeRange.FindAllStringSubmatch(pattern, -1)
	for _, match := range matches {
		if len(match) >= 3 {
			var min, max int
			fmt.Sscanf(match[1], "%d", &min)
			fmt.Sscanf(match[2], "%d", &max)
			if max-min > 1000 {
				return fmt.Errorf("regex contains excessive repetition range {%d,%d}", min, max)
			}
		}
	}

	return nil
}

// GetStats returns statistics about the route manager
func (rm *RouteManager) GetStats() RouteManagerStats {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	return RouteManagerStats{
		TotalRoutes:    len(rm.routes),
		EnabledRoutes:  len(rm.enabledRoutes),
		PriorityLevels: len(rm.routesByPriority),
	}
}

// RouteManagerStats holds statistics about the route manager
type RouteManagerStats struct {
	TotalRoutes    int
	EnabledRoutes  int
	PriorityLevels int
}
