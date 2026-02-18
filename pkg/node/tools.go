package node

import (
	"context"
	"fmt"

	"github.com/harun/ranya/pkg/toolexecutor"
)

// RegisterNodeTools registers node invocation tools with the tool executor.
func RegisterNodeTools(executor *toolexecutor.ToolExecutor, manager *NodeManager) error {
	if executor == nil || manager == nil {
		return nil
	}

	tool := toolexecutor.ToolDefinition{
		Name:        "node_invoke",
		Description: "Invoke a capability on a paired node.",
		Category:    toolexecutor.CategoryGeneral,
		Parameters: []toolexecutor.ToolParameter{
			{Name: "node_id", Type: "string", Description: "Target node ID (optional)", Required: false},
			{Name: "capability", Type: "string", Description: "Capability name", Required: true},
			{Name: "parameters", Type: "object", Description: "Capability parameters", Required: false},
			{Name: "timeout", Type: "number", Description: "Timeout in seconds", Required: false},
		},
		Handler: func(ctx context.Context, params map[string]interface{}) (interface{}, error) {
			req := &InvocationRequest{}
			if val, ok := params["node_id"].(string); ok {
				req.NodeID = val
			}
			if val, ok := params["capability"].(string); ok && val != "" {
				req.Capability = NodeCapability(val)
			} else {
				return nil, fmt.Errorf("capability is required")
			}
			if val, ok := params["parameters"].(map[string]interface{}); ok {
				req.Parameters = val
			}
			if val, ok := params["timeout"].(float64); ok && val > 0 {
				req.Timeout = int(val)
			}

			resp, err := manager.InvokeCapability(ctx, req)
			if err != nil {
				return nil, err
			}
			return resp, nil
		},
	}

	return executor.RegisterTool(tool)
}
