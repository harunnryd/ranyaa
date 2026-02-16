package routing

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
)

// HandlerFunc is a function that handles message delivery
type HandlerFunc func(ctx context.Context, message interface{}) error

// RoutingService is the main orchestrator for the routing system
type RoutingService struct {
	routeManager       *RouteManager
	patternMatcher     *PatternMatcher
	conditionEvaluator *ConditionEvaluator
	priorityQueue      *PriorityQueue
	statisticsTracker  *StatisticsTracker
	storage            *RouteStorage
	handlers           map[string]HandlerFunc
	handlersMu         sync.RWMutex
	defaultFallback    HandlerFunc
	config             RoutingServiceConfig
	eventHandlers      map[string][]EventHandler
	eventMu            sync.RWMutex
	started            bool
	mu                 sync.RWMutex
}

// RoutingServiceConfig holds configuration for RoutingService
type RoutingServiceConfig struct {
	RouteManager       RouteManagerConfig
	PatternMatcher     PatternMatcherConfig
	ConditionEvaluator ConditionEvaluatorConfig
	PriorityQueue      PriorityQueueConfig
	Storage            StorageConfig
	MaxFallbackDepth   int
	DeliveryTimeout    time.Duration
}

// EventHandler is a function that handles routing events
type EventHandler func(event RoutingEvent)

// RoutingEvent represents a routing event
type RoutingEvent struct {
	Type      string                 // Event type
	RouteID   string                 // Route ID
	MessageID string                 // Message ID (if applicable)
	Data      map[string]interface{} // Event data
	Timestamp time.Time              // Event timestamp
}

// DefaultRoutingServiceConfig returns default configuration
func DefaultRoutingServiceConfig() RoutingServiceConfig {
	return RoutingServiceConfig{
		RouteManager:       DefaultRouteManagerConfig(),
		PatternMatcher:     DefaultPatternMatcherConfig(),
		ConditionEvaluator: DefaultConditionEvaluatorConfig(),
		PriorityQueue:      DefaultPriorityQueueConfig(),
		Storage:            DefaultStorageConfig(),
		MaxFallbackDepth:   10,
		DeliveryTimeout:    30 * time.Second,
	}
}

// NewRoutingService creates a new RoutingService
func NewRoutingService(config RoutingServiceConfig) *RoutingService {
	return &RoutingService{
		routeManager: NewRouteManager(config.RouteManager),
		patternMatcher: NewPatternMatcher(&RoutingConfig{
			PatternCacheSize: config.PatternMatcher.PatternCacheSize,
			ResultCacheSize:  config.PatternMatcher.ResultCacheSize,
			RegexTimeout:     int(config.PatternMatcher.RegexTimeout.Milliseconds()),
		}),
		conditionEvaluator: NewConditionEvaluator(&RoutingConfig{
			ConditionCacheSize: config.ConditionEvaluator.ConditionCacheSize,
		}),
		priorityQueue:     NewPriorityQueue(config.PriorityQueue.AgingFactor),
		statisticsTracker: NewStatisticsTracker(),
		storage:           NewRouteStorage(config.Storage),
		handlers:          make(map[string]HandlerFunc),
		eventHandlers:     make(map[string][]EventHandler),
		config:            config,
	}
}

// Start initializes the routing service
func (rs *RoutingService) Start() error {
	rs.mu.Lock()
	defer rs.mu.Unlock()

	if rs.started {
		return fmt.Errorf("routing service already started")
	}

	// Load routes from storage
	routes, err := rs.storage.LoadRoutes()
	if err != nil {
		return fmt.Errorf("failed to load routes: %w", err)
	}

	// Add routes to manager
	for _, route := range routes {
		if err := rs.routeManager.AddRoute(route); err != nil {
			log.Warn().
				Err(err).
				Str("routeId", route.ID).
				Msg("Failed to load route, skipping")
		}
	}

	// Start auto-save
	rs.storage.StartAutoSave(func() []*Route {
		return rs.routeManager.ListRoutes(nil)
	})

	rs.started = true

	log.Info().
		Int("routesLoaded", len(routes)).
		Msg("Routing service started")

	return nil
}

// Stop gracefully stops the routing service
func (rs *RoutingService) Stop() error {
	rs.mu.Lock()
	defer rs.mu.Unlock()

	if !rs.started {
		return nil
	}

	// Stop auto-save
	rs.storage.StopAutoSave()

	// Save routes one final time
	routes := rs.routeManager.ListRoutes(nil)
	if err := rs.storage.SaveRoutes(routes); err != nil {
		log.Error().Err(err).Msg("Failed to save routes on shutdown")
	}

	rs.started = false

	log.Info().Msg("Routing service stopped")

	return nil
}

// AddRoute adds a new route
func (rs *RoutingService) AddRoute(route *Route) error {
	if err := rs.routeManager.AddRoute(route); err != nil {
		return err
	}

	rs.emit(RoutingEvent{
		Type:      "route:added",
		RouteID:   route.ID,
		Timestamp: time.Now(),
		Data: map[string]interface{}{
			"priority": route.Priority,
			"enabled":  route.Enabled,
		},
	})

	return nil
}

// RemoveRoute removes a route by ID
func (rs *RoutingService) RemoveRoute(routeID string) error {
	if err := rs.routeManager.RemoveRoute(routeID); err != nil {
		return err
	}

	rs.emit(RoutingEvent{
		Type:      "route:removed",
		RouteID:   routeID,
		Timestamp: time.Now(),
	})

	return nil
}

// EnableRoute enables a route by ID
func (rs *RoutingService) EnableRoute(routeID string) error {
	if err := rs.routeManager.EnableRoute(routeID); err != nil {
		return err
	}

	rs.emit(RoutingEvent{
		Type:      "route:enabled",
		RouteID:   routeID,
		Timestamp: time.Now(),
	})

	return nil
}

// DisableRoute disables a route by ID
func (rs *RoutingService) DisableRoute(routeID string) error {
	if err := rs.routeManager.DisableRoute(routeID); err != nil {
		return err
	}

	rs.emit(RoutingEvent{
		Type:      "route:disabled",
		RouteID:   routeID,
		Timestamp: time.Now(),
	})

	return nil
}

// UpdateRoutePriority updates the priority of a route
func (rs *RoutingService) UpdateRoutePriority(routeID string, newPriority int) error {
	return rs.routeManager.UpdatePriority(routeID, newPriority)
}

// ListRoutes returns all routes with optional filtering
func (rs *RoutingService) ListRoutes(filter *RouteFilter) []*Route {
	return rs.routeManager.ListRoutes(filter)
}

// GetRoute retrieves a route by ID
func (rs *RoutingService) GetRoute(routeID string) (*Route, error) {
	return rs.routeManager.GetRoute(routeID)
}

// RegisterHandler registers a message handler
func (rs *RoutingService) RegisterHandler(handlerID string, handler HandlerFunc) {
	rs.handlersMu.Lock()
	defer rs.handlersMu.Unlock()

	rs.handlers[handlerID] = handler

	log.Debug().Str("handlerId", handlerID).Msg("Handler registered")
}

// UnregisterHandler unregisters a message handler
func (rs *RoutingService) UnregisterHandler(handlerID string) {
	rs.handlersMu.Lock()
	defer rs.handlersMu.Unlock()

	delete(rs.handlers, handlerID)

	log.Debug().Str("handlerId", handlerID).Msg("Handler unregistered")
}

// SetDefaultFallback sets the default fallback handler
func (rs *RoutingService) SetDefaultFallback(handler HandlerFunc) {
	rs.defaultFallback = handler
}

// Route performs message routing
func (rs *RoutingService) Route(ctx RoutingContext, message interface{}) (*RoutingResult, error) {
	startTime := time.Now()

	// Get enabled routes
	enabledRoutes := rs.routeManager.GetEnabledRoutes()

	if len(enabledRoutes) == 0 {
		return &RoutingResult{
			Success:   false,
			Error:     "no enabled routes",
			Latency:   time.Since(startTime),
			Timestamp: time.Now(),
		}, nil
	}

	// Evaluate patterns and conditions for all enabled routes
	eligibleRoutes := make([]*Route, 0)

	for _, route := range enabledRoutes {
		// Create message from context for pattern matching
		msg := &Message{
			ID:        ctx.MessageID,
			Content:   ctx.Content,
			Metadata:  ctx.Metadata,
			Timestamp: ctx.Timestamp.Unix(),
		}

		// Check pattern match
		matched := false
		for i := range route.Patterns {
			if rs.patternMatcher.Match(&route.Patterns[i], msg) {
				matched = true
				break
			}
		}

		if !matched {
			continue
		}

		// Record match
		rs.statisticsTracker.RecordMatch(route.ID, route.Name)

		// Check conditions
		if len(route.Conditions) > 0 {
			allConditionsMet := true
			for i := range route.Conditions {
				// Convert RouteCondition to map for evaluator
				condMap := make(map[string]interface{})
				condMap["type"] = route.Conditions[i].Type
				if route.Conditions[i].TimeRange != nil {
					condMap["timeRange"] = route.Conditions[i].TimeRange
				}
				if route.Conditions[i].ContentMatch != nil {
					condMap["contentMatch"] = route.Conditions[i].ContentMatch
				}
				if route.Conditions[i].MetadataMatch != nil {
					condMap["metadataMatch"] = route.Conditions[i].MetadataMatch
				}
				if route.Conditions[i].CustomFunction != "" {
					condMap["customFunction"] = route.Conditions[i].CustomFunction
				}

				if !rs.conditionEvaluator.Evaluate(condMap, &ctx) {
					allConditionsMet = false
					break
				}
			}

			if !allConditionsMet {
				continue
			}
		}

		eligibleRoutes = append(eligibleRoutes, route)
	}

	if len(eligibleRoutes) == 0 {
		// No matching routes, use default fallback
		result := rs.handleDefaultFallback(ctx, message, startTime)
		return result, nil
	}

	// Select highest priority route using priority queue
	rs.priorityQueue.Clear()
	for _, route := range eligibleRoutes {
		rs.priorityQueue.Enqueue(route)
	}

	selectedRoute := rs.priorityQueue.Dequeue()
	if selectedRoute == nil {
		result := rs.handleDefaultFallback(ctx, message, startTime)
		return result, nil
	}

	rs.emit(RoutingEvent{
		Type:      "route:matched",
		RouteID:   selectedRoute.ID,
		MessageID: ctx.MessageID,
		Timestamp: time.Now(),
		Data: map[string]interface{}{
			"priority": selectedRoute.Priority,
		},
	})

	// Attempt delivery with fallback chain
	result := rs.deliverWithFallback(ctx, message, selectedRoute, 0, startTime)

	return result, nil
}

// deliverWithFallback attempts delivery with fallback chain
func (rs *RoutingService) deliverWithFallback(ctx RoutingContext, message interface{}, route *Route, depth int, startTime time.Time) *RoutingResult {
	// Check fallback depth limit
	if depth >= rs.config.MaxFallbackDepth {
		rs.emit(RoutingEvent{
			Type:      "route:failed",
			RouteID:   route.ID,
			MessageID: ctx.MessageID,
			Timestamp: time.Now(),
			Data: map[string]interface{}{
				"error": "fallback chain depth exceeded",
			},
		})

		return &RoutingResult{
			Success:   false,
			RouteID:   route.ID,
			Error:     fmt.Sprintf("fallback chain depth exceeded (max: %d)", rs.config.MaxFallbackDepth),
			Latency:   time.Since(startTime),
			Timestamp: time.Now(),
		}
	}

	// Get handler
	rs.handlersMu.RLock()
	handler, exists := rs.handlers[route.Handler]
	rs.handlersMu.RUnlock()

	if !exists {
		// Handler not found, try fallback
		rs.statisticsTracker.RecordFailure(route.ID, route.Name, time.Since(startTime).Milliseconds(), &RoutingError{
			Code:    ErrCodeNotFound,
			Message: "handler not found",
		})

		rs.emit(RoutingEvent{
			Type:      "route:failed",
			RouteID:   route.ID,
			MessageID: ctx.MessageID,
			Timestamp: time.Now(),
			Data: map[string]interface{}{
				"error": "handler not found",
			},
		})

		return rs.tryFallback(ctx, message, route, depth, startTime)
	}

	// Create delivery context with timeout
	deliveryCtx, cancel := context.WithTimeout(context.Background(), rs.config.DeliveryTimeout)
	defer cancel()

	// Invoke handler
	handlerErr := handler(deliveryCtx, message)

	if handlerErr != nil {
		// Handler failed, try fallback
		rs.statisticsTracker.RecordFailure(route.ID, route.Name, time.Since(startTime).Milliseconds(), &RoutingError{
			Code:    ErrCodeHandlerError,
			Message: handlerErr.Error(),
		})

		rs.emit(RoutingEvent{
			Type:      "route:failed",
			RouteID:   route.ID,
			MessageID: ctx.MessageID,
			Timestamp: time.Now(),
			Data: map[string]interface{}{
				"error": handlerErr.Error(),
			},
		})

		return rs.tryFallback(ctx, message, route, depth, startTime)
	}

	// Success
	rs.statisticsTracker.RecordSuccess(route.ID, route.Name, time.Since(startTime).Milliseconds())

	rs.emit(RoutingEvent{
		Type:      "route:delivered",
		RouteID:   route.ID,
		MessageID: ctx.MessageID,
		Timestamp: time.Now(),
	})

	return &RoutingResult{
		Success:   true,
		RouteID:   route.ID,
		HandlerID: route.Handler,
		Latency:   time.Since(startTime),
		Timestamp: time.Now(),
	}
}

// tryFallback attempts to use fallback routes
func (rs *RoutingService) tryFallback(ctx RoutingContext, message interface{}, route *Route, depth int, startTime time.Time) *RoutingResult {
	if len(route.FallbackRoutes) == 0 {
		// No fallbacks, use default fallback
		return rs.handleDefaultFallback(ctx, message, startTime)
	}

	// Try each fallback route in order
	for _, fallbackID := range route.FallbackRoutes {
		fallbackRoute, err := rs.routeManager.GetRoute(fallbackID)
		if err != nil {
			log.Warn().
				Err(err).
				Str("fallbackId", fallbackID).
				Msg("Fallback route not found")
			continue
		}

		// Check if fallback route is enabled
		if !fallbackRoute.Enabled {
			continue
		}

		rs.statisticsTracker.RecordFallback(route.ID, route.Name)

		rs.emit(RoutingEvent{
			Type:      "route:fallback",
			RouteID:   route.ID,
			MessageID: ctx.MessageID,
			Timestamp: time.Now(),
			Data: map[string]interface{}{
				"fallbackRouteId": fallbackID,
			},
		})

		// Attempt delivery with fallback route
		result := rs.deliverWithFallback(ctx, message, fallbackRoute, depth+1, startTime)
		if result.Success {
			return result
		}
	}

	// All fallbacks exhausted, use default fallback
	return rs.handleDefaultFallback(ctx, message, startTime)
}

// handleDefaultFallback handles message with default fallback handler
func (rs *RoutingService) handleDefaultFallback(ctx RoutingContext, message interface{}, startTime time.Time) *RoutingResult {
	if rs.defaultFallback == nil {
		return &RoutingResult{
			Success:   false,
			Error:     "no matching routes and no default fallback handler",
			Latency:   time.Since(startTime),
			Timestamp: time.Now(),
		}
	}

	rs.emit(RoutingEvent{
		Type:      "route:fallback",
		MessageID: ctx.MessageID,
		Timestamp: time.Now(),
		Data: map[string]interface{}{
			"type": "default",
		},
	})

	// Create delivery context with timeout
	deliveryCtx, cancel := context.WithTimeout(context.Background(), rs.config.DeliveryTimeout)
	defer cancel()

	// Invoke default fallback handler
	if err := rs.defaultFallback(deliveryCtx, message); err != nil {
		return &RoutingResult{
			Success:   false,
			Error:     fmt.Sprintf("default fallback handler failed: %v", err),
			Latency:   time.Since(startTime),
			Timestamp: time.Now(),
		}
	}

	return &RoutingResult{
		Success:   true,
		RouteID:   "default-fallback",
		HandlerID: "default-fallback",
		Latency:   time.Since(startTime),
		Timestamp: time.Now(),
	}
}

// GetStatistics returns statistics for a specific route or all routes
func (rs *RoutingService) GetStatistics(routeID string) interface{} {
	if routeID == "" {
		return rs.statisticsTracker.GetGlobalStatistics()
	}

	stats := rs.statisticsTracker.GetRouteStatistics(routeID)
	return stats
}

// ResetStatistics resets statistics for a specific route or all routes
func (rs *RoutingService) ResetStatistics(routeID string) {
	rs.statisticsTracker.ResetStatistics(routeID)
}

// On registers an event handler
func (rs *RoutingService) On(eventType string, handler EventHandler) {
	rs.eventMu.Lock()
	defer rs.eventMu.Unlock()

	rs.eventHandlers[eventType] = append(rs.eventHandlers[eventType], handler)
}

// Off removes all event handlers for a specific event type
func (rs *RoutingService) Off(eventType string) {
	rs.eventMu.Lock()
	defer rs.eventMu.Unlock()

	delete(rs.eventHandlers, eventType)
}

// emit emits an event to all registered handlers
func (rs *RoutingService) emit(event RoutingEvent) {
	rs.eventMu.RLock()
	handlers := rs.eventHandlers[event.Type]
	rs.eventMu.RUnlock()

	// Call handlers asynchronously
	for _, handler := range handlers {
		go handler(event)
	}
}
