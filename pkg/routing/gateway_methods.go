package routing

import (
	"context"
	"fmt"

	"github.com/harun/ranya/pkg/commandqueue"
	"github.com/harun/ranya/pkg/gateway"
	"github.com/rs/zerolog/log"
)

// RegisterGatewayMethods registers routing RPC methods with the Gateway server
func RegisterGatewayMethods(gw *gateway.Server, service *RoutingService, cq *commandqueue.CommandQueue) error {
	// route.list - List all routes
	if err := gw.RegisterMethod("route.list", func(params map[string]interface{}) (interface{}, error) {
		result, err := cq.Enqueue("main", func(ctx context.Context) (interface{}, error) {
			// Parse params
			var enabled *bool
			var priority *int
			var handler *string

			if val, ok := params["enabled"].(bool); ok {
				enabled = &val
			}
			if val, ok := params["priority"].(float64); ok {
				p := int(val)
				priority = &p
			}
			if val, ok := params["handler"].(string); ok {
				handler = &val
			}

			// Build filter
			filter := &RouteFilter{
				Enabled:  enabled,
				Priority: priority,
				Handler:  handler,
			}

			// List routes
			routes := service.ListRoutes(filter)

			return map[string]interface{}{
				"routes": routes,
				"count":  len(routes),
			}, nil
		}, nil)

		return result, err
	}); err != nil {
		return fmt.Errorf("failed to register route.list: %w", err)
	}

	// route.add - Add a new route
	if err := gw.RegisterMethod("route.add", func(params map[string]interface{}) (interface{}, error) {
		result, err := cq.Enqueue("main", func(ctx context.Context) (interface{}, error) {
			// Extract route fields from params
			route := &Route{}

			if val, ok := params["id"].(string); ok {
				route.ID = val
			}
			if val, ok := params["name"].(string); ok {
				route.Name = val
			}
			if val, ok := params["description"].(string); ok {
				route.Description = val
			}
			if val, ok := params["handler"].(string); ok {
				route.Handler = val
			}
			if val, ok := params["priority"].(float64); ok {
				route.Priority = int(val)
			}
			if val, ok := params["enabled"].(bool); ok {
				route.Enabled = val
			}
			if val, ok := params["metadata"].(map[string]interface{}); ok {
				route.Metadata = val
			}

			// Parse patterns
			if patternsRaw, ok := params["patterns"].([]interface{}); ok {
				for _, p := range patternsRaw {
					if patternMap, ok := p.(map[string]interface{}); ok {
						pattern := RoutePattern{}
						if t, ok := patternMap["type"].(string); ok {
							pattern.Type = PatternType(t)
						}
						if v, ok := patternMap["value"].(string); ok {
							pattern.Value = v
						}
						route.Patterns = append(route.Patterns, pattern)
					}
				}
			}

			// Add route
			if err := service.AddRoute(route); err != nil {
				return nil, err
			}

			log.Info().
				Str("routeId", route.ID).
				Str("handler", route.Handler).
				Int("priority", route.Priority).
				Msg("Route added via RPC")

			return map[string]interface{}{
				"success": true,
				"routeId": route.ID,
			}, nil
		}, nil)

		return result, err
	}); err != nil {
		return fmt.Errorf("failed to register route.add: %w", err)
	}

	// route.remove - Remove a route
	if err := gw.RegisterMethod("route.remove", func(params map[string]interface{}) (interface{}, error) {
		result, err := cq.Enqueue("main", func(ctx context.Context) (interface{}, error) {
			routeID, ok := params["routeId"].(string)
			if !ok || routeID == "" {
				return nil, fmt.Errorf("routeId is required")
			}

			// Remove route
			if err := service.RemoveRoute(routeID); err != nil {
				return nil, err
			}

			log.Info().
				Str("routeId", routeID).
				Msg("Route removed via RPC")

			return map[string]interface{}{
				"success": true,
				"routeId": routeID,
			}, nil
		}, nil)

		return result, err
	}); err != nil {
		return fmt.Errorf("failed to register route.remove: %w", err)
	}

	// route.enable - Enable a route
	if err := gw.RegisterMethod("route.enable", func(params map[string]interface{}) (interface{}, error) {
		result, err := cq.Enqueue("main", func(ctx context.Context) (interface{}, error) {
			routeID, ok := params["routeId"].(string)
			if !ok || routeID == "" {
				return nil, fmt.Errorf("routeId is required")
			}

			// Enable route
			if err := service.EnableRoute(routeID); err != nil {
				return nil, err
			}

			log.Info().
				Str("routeId", routeID).
				Msg("Route enabled via RPC")

			return map[string]interface{}{
				"success": true,
				"routeId": routeID,
			}, nil
		}, nil)

		return result, err
	}); err != nil {
		return fmt.Errorf("failed to register route.enable: %w", err)
	}

	// route.disable - Disable a route
	if err := gw.RegisterMethod("route.disable", func(params map[string]interface{}) (interface{}, error) {
		result, err := cq.Enqueue("main", func(ctx context.Context) (interface{}, error) {
			routeID, ok := params["routeId"].(string)
			if !ok || routeID == "" {
				return nil, fmt.Errorf("routeId is required")
			}

			// Disable route
			if err := service.DisableRoute(routeID); err != nil {
				return nil, err
			}

			log.Info().
				Str("routeId", routeID).
				Msg("Route disabled via RPC")

			return map[string]interface{}{
				"success": true,
				"routeId": routeID,
			}, nil
		}, nil)

		return result, err
	}); err != nil {
		return fmt.Errorf("failed to register route.disable: %w", err)
	}

	// route.stats - Get statistics
	if err := gw.RegisterMethod("route.stats", func(params map[string]interface{}) (interface{}, error) {
		result, err := cq.Enqueue("main", func(ctx context.Context) (interface{}, error) {
			routeID, _ := params["routeId"].(string)

			// Get statistics
			stats := service.GetStatistics(routeID)

			// Note: GetStatistics never returns nil, so no need to check
			return stats, nil
		}, nil)

		return result, err
	}); err != nil {
		return fmt.Errorf("failed to register route.stats: %w", err)
	}

	// route.stats.reset - Reset statistics
	if err := gw.RegisterMethod("route.stats.reset", func(params map[string]interface{}) (interface{}, error) {
		result, err := cq.Enqueue("main", func(ctx context.Context) (interface{}, error) {
			routeID, _ := params["routeId"].(string)

			// Reset statistics
			service.ResetStatistics(routeID)

			log.Info().
				Str("routeId", routeID).
				Msg("Statistics reset via RPC")

			return map[string]interface{}{
				"success": true,
			}, nil
		}, nil)

		return result, err
	}); err != nil {
		return fmt.Errorf("failed to register route.stats.reset: %w", err)
	}

	// route.get - Get a specific route
	if err := gw.RegisterMethod("route.get", func(params map[string]interface{}) (interface{}, error) {
		result, err := cq.Enqueue("main", func(ctx context.Context) (interface{}, error) {
			routeID, ok := params["routeId"].(string)
			if !ok || routeID == "" {
				return nil, fmt.Errorf("routeId is required")
			}

			// Get route
			route, err := service.GetRoute(routeID)
			if err != nil {
				return nil, err
			}

			return route, nil
		}, nil)

		return result, err
	}); err != nil {
		return fmt.Errorf("failed to register route.get: %w", err)
	}

	// route.update.priority - Update route priority
	if err := gw.RegisterMethod("route.update.priority", func(params map[string]interface{}) (interface{}, error) {
		result, err := cq.Enqueue("main", func(ctx context.Context) (interface{}, error) {
			routeID, ok := params["routeId"].(string)
			if !ok || routeID == "" {
				return nil, fmt.Errorf("routeId is required")
			}

			priority, ok := params["priority"].(float64)
			if !ok {
				return nil, fmt.Errorf("priority is required")
			}

			// Update priority
			if err := service.UpdateRoutePriority(routeID, int(priority)); err != nil {
				return nil, err
			}

			log.Info().
				Str("routeId", routeID).
				Int("priority", int(priority)).
				Msg("Route priority updated via RPC")

			return map[string]interface{}{
				"success": true,
				"routeId": routeID,
			}, nil
		}, nil)

		return result, err
	}); err != nil {
		return fmt.Errorf("failed to register route.update.priority: %w", err)
	}

	log.Info().Msg("Routing Gateway RPC methods registered")

	return nil
}
