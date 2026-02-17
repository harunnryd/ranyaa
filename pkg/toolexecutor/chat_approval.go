package toolexecutor

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

// ApprovalAction defines available chat approval actions.
type ApprovalAction string

const (
	ApprovalActionAllowOnce   ApprovalAction = "allow-once"
	ApprovalActionAllowAlways ApprovalAction = "allow-always"
	ApprovalActionDeny        ApprovalAction = "deny"
)

// ParseApprovalAction parses a user-provided action string.
func ParseApprovalAction(value string) (ApprovalAction, error) {
	action := ApprovalAction(strings.ToLower(strings.TrimSpace(value)))
	switch action {
	case ApprovalActionAllowOnce, ApprovalActionAllowAlways, ApprovalActionDeny:
		return action, nil
	default:
		return "", fmt.Errorf("invalid approval action %q", value)
	}
}

// PendingApproval describes an approval request waiting for user decision.
type PendingApproval struct {
	ID        string
	Request   ApprovalRequest
	CreatedAt time.Time
	ExpiresAt time.Time
}

// ApprovalForwarder forwards pending approvals to a chat channel.
type ApprovalForwarder interface {
	ForwardApproval(ctx context.Context, pending PendingApproval) error
}

// ChatApprovalHandler handles approval requests resolved by chat commands.
type ChatApprovalHandler struct {
	forwarder ApprovalForwarder
	allowlist *AllowlistManager

	mu       sync.RWMutex
	pending  map[string]chan ApprovalResponse
	requests map[string]ApprovalRequest
}

// NewChatApprovalHandler creates a chat-driven approval handler.
func NewChatApprovalHandler(forwarder ApprovalForwarder, allowlist *AllowlistManager) *ChatApprovalHandler {
	return &ChatApprovalHandler{
		forwarder: forwarder,
		allowlist: allowlist,
		pending:   make(map[string]chan ApprovalResponse),
		requests:  make(map[string]ApprovalRequest),
	}
}

// RequestApproval forwards an approval request and waits for a chat response.
func (h *ChatApprovalHandler) RequestApproval(ctx context.Context, req ApprovalRequest) (ApprovalResponse, error) {
	if h.forwarder == nil {
		return ApprovalResponse{}, fmt.Errorf("approval forwarder is not configured")
	}

	if h.allowlist != nil && h.allowlist.IsAllowed(req.Command, req.Args) {
		return ApprovalResponse{
			Approved: true,
			Reason:   "approved by allowlist",
		}, nil
	}

	id := uuid.NewString()
	responseCh := make(chan ApprovalResponse, 1)

	pending := PendingApproval{
		ID:        id,
		Request:   req,
		CreatedAt: time.Now(),
	}
	if deadline, ok := ctx.Deadline(); ok {
		pending.ExpiresAt = deadline
	}

	h.mu.Lock()
	h.pending[id] = responseCh
	h.requests[id] = req
	h.mu.Unlock()

	defer func() {
		h.mu.Lock()
		delete(h.pending, id)
		delete(h.requests, id)
		h.mu.Unlock()
	}()

	if err := h.forwarder.ForwardApproval(ctx, pending); err != nil {
		return ApprovalResponse{}, err
	}

	select {
	case response := <-responseCh:
		return response, nil
	case <-ctx.Done():
		return ApprovalResponse{}, ctx.Err()
	}
}

// ResolveApproval resolves a pending approval by ID and action.
func (h *ChatApprovalHandler) ResolveApproval(id string, action ApprovalAction, actor string) error {
	h.mu.RLock()
	responseCh, exists := h.pending[id]
	req := h.requests[id]
	h.mu.RUnlock()

	if !exists {
		return fmt.Errorf("approval %s not found", id)
	}

	response, err := h.responseForAction(action, actor, req)
	if err != nil {
		return err
	}

	select {
	case responseCh <- response:
		return nil
	default:
		return fmt.Errorf("approval %s already resolved", id)
	}
}

func (h *ChatApprovalHandler) responseForAction(action ApprovalAction, actor string, req ApprovalRequest) (ApprovalResponse, error) {
	switch action {
	case ApprovalActionAllowOnce:
		return ApprovalResponse{
			Approved: true,
			Reason:   fmt.Sprintf("approved once by %s", actor),
		}, nil
	case ApprovalActionAllowAlways:
		if h.allowlist != nil {
			entry := AllowlistEntry{
				Command: req.Command,
				Args:    append([]string{}, req.Args...),
				Reason:  fmt.Sprintf("allow-always by %s", actor),
				AddedAt: time.Now().Format(time.RFC3339),
			}
			if err := h.allowlist.Add(entry); err != nil {
				return ApprovalResponse{}, fmt.Errorf("failed to add allowlist entry: %w", err)
			}
			if err := h.allowlist.Save(); err != nil {
				return ApprovalResponse{}, fmt.Errorf("failed to persist allowlist entry: %w", err)
			}
		}

		return ApprovalResponse{
			Approved: true,
			Reason:   fmt.Sprintf("approved always by %s", actor),
		}, nil
	case ApprovalActionDeny:
		return ApprovalResponse{
			Approved: false,
			Reason:   fmt.Sprintf("denied by %s", actor),
		}, nil
	default:
		return ApprovalResponse{}, fmt.Errorf("unsupported approval action %q", action)
	}
}
