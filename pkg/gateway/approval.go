package gateway

import (
	"context"
	"fmt"
	"time"

	"github.com/harun/ranya/pkg/toolexecutor"
)

// ApprovalForwarder adapts the Gateway to the ToolExecutor's approval mechanism.
type ApprovalForwarder struct {
	server *Server
}

// NewApprovalForwarder creates a new approval forwarder.
func NewApprovalForwarder(server *Server) *ApprovalForwarder {
	return &ApprovalForwarder{server: server}
}

// ForwardApproval broadcasts an approval request to connected clients.
func (f *ApprovalForwarder) ForwardApproval(ctx context.Context, pending toolexecutor.PendingApproval) error {
	// Construct payload
	data := map[string]interface{}{
		"approval_id": pending.ID,
		"command":     pending.Request.Command,
		"args":        pending.Request.Args,
		"cwd":         pending.Request.Cwd,
		"timeout_ms":  pending.Request.Timeout.Milliseconds(),
		"agent_id":    pending.Request.AgentID,
		"context":     pending.Request.Context,
		"created_at":  pending.CreatedAt.UnixMilli(),
	}
	if !pending.ExpiresAt.IsZero() {
		data["expires_at"] = pending.ExpiresAt.UnixMilli()
	}

	// Broadcast
	f.server.BroadcastTyped(EventMessage{
		Event:     "tool.approval_request",
		Stream:    StreamTypeTool, // Using Tool stream as it relates to tool execution
		Phase:     "approval_required",
		Data:      data,
		Timestamp: time.Now().UnixMilli(),
	})

	return nil
}

// RegisterApprovalMethods registers RPC methods for handling tool approvals.
func (s *Server) RegisterApprovalMethods(manager *toolexecutor.ApprovalManager) {
	_ = s.router.RegisterMethod("tools.approve", func(ctx context.Context, params map[string]interface{}) (interface{}, error) {
		id, ok := params["approval_id"].(string)
		if !ok || id == "" {
			return nil, &RPCError{Code: InvalidParams, Message: "approval_id is required"}
		}

		actionStr, ok := params["action"].(string)
		if !ok || actionStr == "" {
			return nil, &RPCError{Code: InvalidParams, Message: "action is required"}
		}

		action, err := toolexecutor.ParseApprovalAction(actionStr)
		if err != nil {
			return nil, &RPCError{Code: InvalidParams, Message: fmt.Sprintf("invalid action: %v", err)}
		}

		// Get actor from context/client ID if available
		actor := "unknown"
		if clientID, ok := ctx.Value("clientID").(string); ok {
			actor = clientID
		}

		if err := manager.ResolveApproval(id, action, actor); err != nil {
			return nil, &RPCError{Code: InternalError, Message: err.Error()}
		}

		return map[string]bool{"success": true}, nil
	})
}
