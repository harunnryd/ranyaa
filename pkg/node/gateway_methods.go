package node

import (
	"context"
	"fmt"

	"github.com/harun/ranya/pkg/commandqueue"
	"github.com/harun/ranya/pkg/gateway"
	"github.com/rs/zerolog/log"
)

// RegisterGatewayMethods registers node RPC methods with the Gateway server
func RegisterGatewayMethods(gw *gateway.Server, manager *NodeManager, cq *commandqueue.CommandQueue) error {
	// node.register - Register a new node
	if err := gw.RegisterMethod("node.register", func(params map[string]interface{}) (interface{}, error) {
		result, err := cq.Enqueue("main", func(ctx context.Context) (interface{}, error) {
			// Parse node from params
			node := &Node{}

			if val, ok := params["id"].(string); ok {
				node.ID = val
			}
			if val, ok := params["name"].(string); ok {
				node.Name = val
			}
			if val, ok := params["platform"].(string); ok {
				node.Platform = NodePlatform(val)
			}

			// Parse capabilities
			if capsRaw, ok := params["capabilities"].([]interface{}); ok {
				for _, c := range capsRaw {
					if capStr, ok := c.(string); ok {
						node.Capabilities = append(node.Capabilities, NodeCapability(capStr))
					}
				}
			}

			// Parse metadata
			if metaRaw, ok := params["metadata"].(map[string]interface{}); ok {
				node.Metadata = metaRaw
			}

			// Register node
			if err := manager.RegisterNode(node); err != nil {
				return nil, err
			}

			log.Info().
				Str("nodeId", node.ID).
				Str("name", node.Name).
				Msg("Node registered via RPC")

			return map[string]interface{}{
				"success": true,
				"nodeId":  node.ID,
			}, nil
		}, nil)

		return result, err
	}); err != nil {
		return fmt.Errorf("failed to register node.register: %w", err)
	}

	// node.unregister - Unregister a node
	if err := gw.RegisterMethod("node.unregister", func(params map[string]interface{}) (interface{}, error) {
		result, err := cq.Enqueue("main", func(ctx context.Context) (interface{}, error) {
			nodeID, ok := params["nodeId"].(string)
			if !ok || nodeID == "" {
				return nil, fmt.Errorf("nodeId is required")
			}

			// Unregister node
			if err := manager.UnregisterNode(nodeID); err != nil {
				return nil, err
			}

			log.Info().
				Str("nodeId", nodeID).
				Msg("Node unregistered via RPC")

			return map[string]interface{}{
				"success": true,
				"nodeId":  nodeID,
			}, nil
		}, nil)

		return result, err
	}); err != nil {
		return fmt.Errorf("failed to register node.unregister: %w", err)
	}

	// node.list - List all nodes
	if err := gw.RegisterMethod("node.list", func(params map[string]interface{}) (interface{}, error) {
		result, err := cq.Enqueue("main", func(ctx context.Context) (interface{}, error) {
			// Parse filter
			filter := &NodeFilter{}

			if val, ok := params["platform"].(string); ok {
				platform := NodePlatform(val)
				filter.Platform = &platform
			}
			if val, ok := params["online"].(bool); ok {
				filter.Online = &val
			}
			if val, ok := params["capability"].(string); ok {
				capability := NodeCapability(val)
				filter.Capability = &capability
			}
			if val, ok := params["degraded"].(bool); ok {
				filter.Degraded = &val
			}

			// List nodes
			nodes := manager.ListNodes(filter)

			return map[string]interface{}{
				"nodes": nodes,
				"count": len(nodes),
			}, nil
		}, nil)

		return result, err
	}); err != nil {
		return fmt.Errorf("failed to register node.list: %w", err)
	}

	// node.get - Get a specific node
	if err := gw.RegisterMethod("node.get", func(params map[string]interface{}) (interface{}, error) {
		result, err := cq.Enqueue("main", func(ctx context.Context) (interface{}, error) {
			nodeID, ok := params["nodeId"].(string)
			if !ok || nodeID == "" {
				return nil, fmt.Errorf("nodeId is required")
			}

			// Get node
			node, err := manager.GetNode(nodeID)
			if err != nil {
				return nil, err
			}

			return node, nil
		}, nil)

		return result, err
	}); err != nil {
		return fmt.Errorf("failed to register node.get: %w", err)
	}

	// node.heartbeat - Handle heartbeat from a node
	if err := gw.RegisterMethod("node.heartbeat", func(params map[string]interface{}) (interface{}, error) {
		result, err := cq.Enqueue("main", func(ctx context.Context) (interface{}, error) {
			nodeID, ok := params["nodeId"].(string)
			if !ok || nodeID == "" {
				return nil, fmt.Errorf("nodeId is required")
			}

			// Handle heartbeat
			if err := manager.HandleHeartbeat(nodeID); err != nil {
				return nil, err
			}

			return map[string]interface{}{
				"success": true,
			}, nil
		}, nil)

		return result, err
	}); err != nil {
		return fmt.Errorf("failed to register node.heartbeat: %w", err)
	}

	// node.invoke - Invoke a capability on a node
	if err := gw.RegisterMethod("node.invoke", func(params map[string]interface{}) (interface{}, error) {
		// Parse invocation request
		req := &InvocationRequest{}

		if val, ok := params["invocationId"].(string); ok {
			req.InvocationID = val
		}
		if val, ok := params["nodeId"].(string); ok {
			req.NodeID = val
		}
		if val, ok := params["capability"].(string); ok {
			req.Capability = NodeCapability(val)
		}
		if val, ok := params["timeout"].(float64); ok {
			req.Timeout = int(val)
		}
		if val, ok := params["parameters"].(map[string]interface{}); ok {
			req.Parameters = val
		}

		// Invoke capability (uses per-node command queue lane internally)
		response, err := manager.InvokeCapability(context.Background(), req)
		if err != nil {
			return nil, err
		}

		return response, nil
	}); err != nil {
		return fmt.Errorf("failed to register node.invoke: %w", err)
	}

	// node.invoke.response - Handle invocation response from a node
	if err := gw.RegisterMethod("node.invoke.response", func(params map[string]interface{}) (interface{}, error) {
		result, err := cq.Enqueue("main", func(ctx context.Context) (interface{}, error) {
			// Parse invocation response
			response := &InvocationResponse{}

			if val, ok := params["invocationId"].(string); ok {
				response.InvocationID = val
			}
			if val, ok := params["nodeId"].(string); ok {
				response.NodeID = val
			}
			if val, ok := params["success"].(bool); ok {
				response.Success = val
			}
			if val, ok := params["result"].(map[string]interface{}); ok {
				response.Result = val
			}
			if errRaw, ok := params["error"].(map[string]interface{}); ok {
				invErr := &InvocationError{}
				if code, ok := errRaw["code"].(string); ok {
					invErr.Code = NodeErrorCode(code)
				}
				if msg, ok := errRaw["message"].(string); ok {
					invErr.Message = msg
				}
				response.Error = invErr
			}

			// Handle response
			if err := manager.HandleInvocationResponse(response); err != nil {
				return nil, err
			}

			return map[string]interface{}{
				"success": true,
			}, nil
		}, nil)

		return result, err
	}); err != nil {
		return fmt.Errorf("failed to register node.invoke.response: %w", err)
	}

	// node.permission.grant - Grant a permission to a node
	if err := gw.RegisterMethod("node.permission.grant", func(params map[string]interface{}) (interface{}, error) {
		result, err := cq.Enqueue("main", func(ctx context.Context) (interface{}, error) {
			nodeID, ok := params["nodeId"].(string)
			if !ok || nodeID == "" {
				return nil, fmt.Errorf("nodeId is required")
			}

			permission, ok := params["permission"].(string)
			if !ok || permission == "" {
				return nil, fmt.Errorf("permission is required")
			}

			// Grant permission
			if err := manager.GrantPermission(nodeID, PermissionType(permission)); err != nil {
				return nil, err
			}

			log.Info().
				Str("nodeId", nodeID).
				Str("permission", permission).
				Msg("Permission granted via RPC")

			return map[string]interface{}{
				"success": true,
			}, nil
		}, nil)

		return result, err
	}); err != nil {
		return fmt.Errorf("failed to register node.permission.grant: %w", err)
	}

	// node.permission.revoke - Revoke a permission from a node
	if err := gw.RegisterMethod("node.permission.revoke", func(params map[string]interface{}) (interface{}, error) {
		result, err := cq.Enqueue("main", func(ctx context.Context) (interface{}, error) {
			nodeID, ok := params["nodeId"].(string)
			if !ok || nodeID == "" {
				return nil, fmt.Errorf("nodeId is required")
			}

			permission, ok := params["permission"].(string)
			if !ok || permission == "" {
				return nil, fmt.Errorf("permission is required")
			}

			// Revoke permission
			if err := manager.RevokePermission(nodeID, PermissionType(permission)); err != nil {
				return nil, err
			}

			log.Info().
				Str("nodeId", nodeID).
				Str("permission", permission).
				Msg("Permission revoked via RPC")

			return map[string]interface{}{
				"success": true,
			}, nil
		}, nil)

		return result, err
	}); err != nil {
		return fmt.Errorf("failed to register node.permission.revoke: %w", err)
	}

	// node.permission.list - List permissions for a node
	if err := gw.RegisterMethod("node.permission.list", func(params map[string]interface{}) (interface{}, error) {
		result, err := cq.Enqueue("main", func(ctx context.Context) (interface{}, error) {
			nodeID, ok := params["nodeId"].(string)
			if !ok || nodeID == "" {
				return nil, fmt.Errorf("nodeId is required")
			}

			// Get permissions
			permissions, err := manager.GetPermissions(nodeID)
			if err != nil {
				return nil, err
			}

			return map[string]interface{}{
				"permissions": permissions,
			}, nil
		}, nil)

		return result, err
	}); err != nil {
		return fmt.Errorf("failed to register node.permission.list: %w", err)
	}

	// node.default.set - Set the default node
	if err := gw.RegisterMethod("node.default.set", func(params map[string]interface{}) (interface{}, error) {
		result, err := cq.Enqueue("main", func(ctx context.Context) (interface{}, error) {
			nodeID, ok := params["nodeId"].(string)
			if !ok {
				return nil, fmt.Errorf("nodeId is required")
			}

			// Set default node
			if err := manager.SetDefaultNode(nodeID); err != nil {
				return nil, err
			}

			log.Info().
				Str("nodeId", nodeID).
				Msg("Default node set via RPC")

			return map[string]interface{}{
				"success": true,
				"nodeId":  nodeID,
			}, nil
		}, nil)

		return result, err
	}); err != nil {
		return fmt.Errorf("failed to register node.default.set: %w", err)
	}

	// node.default.get - Get the default node
	if err := gw.RegisterMethod("node.default.get", func(params map[string]interface{}) (interface{}, error) {
		result, err := cq.Enqueue("main", func(ctx context.Context) (interface{}, error) {
			defaultNode := manager.GetDefaultNode()

			return map[string]interface{}{
				"nodeId": defaultNode,
			}, nil
		}, nil)

		return result, err
	}); err != nil {
		return fmt.Errorf("failed to register node.default.get: %w", err)
	}

	// node.stats - Get statistics for a node
	if err := gw.RegisterMethod("node.stats", func(params map[string]interface{}) (interface{}, error) {
		result, err := cq.Enqueue("main", func(ctx context.Context) (interface{}, error) {
			nodeID, ok := params["nodeId"].(string)
			if !ok || nodeID == "" {
				return nil, fmt.Errorf("nodeId is required")
			}

			// Get statistics
			stats := manager.GetStatistics(nodeID)

			return stats, nil
		}, nil)

		return result, err
	}); err != nil {
		return fmt.Errorf("failed to register node.stats: %w", err)
	}

	// node.stats.reset - Reset statistics for a node
	if err := gw.RegisterMethod("node.stats.reset", func(params map[string]interface{}) (interface{}, error) {
		result, err := cq.Enqueue("main", func(ctx context.Context) (interface{}, error) {
			nodeID, ok := params["nodeId"].(string)
			if !ok || nodeID == "" {
				return nil, fmt.Errorf("nodeId is required")
			}

			// Reset statistics
			manager.ResetStatistics(nodeID)

			log.Info().
				Str("nodeId", nodeID).
				Msg("Statistics reset via RPC")

			return map[string]interface{}{
				"success": true,
			}, nil
		}, nil)

		return result, err
	}); err != nil {
		return fmt.Errorf("failed to register node.stats.reset: %w", err)
	}

	log.Info().Msg("Node Gateway RPC methods registered")

	return nil
}
