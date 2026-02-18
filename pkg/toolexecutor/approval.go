package toolexecutor

import (
	"context"
	"fmt"
	"time"

	"github.com/rs/zerolog/log"
)

// ApprovalRequest represents a request for tool execution approval
type ApprovalRequest struct {
	Command string            `json:"command"`
	Args    []string          `json:"args"`
	Cwd     string            `json:"cwd"`
	Timeout time.Duration     `json:"timeout"`
	AgentID string            `json:"agent_id"`
	Context map[string]string `json:"context"`
}

// ApprovalResponse represents the response to an approval request
type ApprovalResponse struct {
	Approved bool   `json:"approved"`
	Reason   string `json:"reason"`
}

// ApprovalHandler handles approval requests
type ApprovalHandler interface {
	RequestApproval(ctx context.Context, req ApprovalRequest) (ApprovalResponse, error)
}

// ApprovalResolver handles asynchronous approval decisions.
type ApprovalResolver interface {
	ResolveApproval(id string, action ApprovalAction, actor string) error
}

// ApprovalManager manages the approval workflow
type ApprovalManager struct {
	handler        ApprovalHandler
	defaultTimeout time.Duration
}

// NewApprovalManager creates a new approval manager
func NewApprovalManager(handler ApprovalHandler) *ApprovalManager {
	return &ApprovalManager{
		handler:        handler,
		defaultTimeout: 60 * time.Second,
	}
}

// RequestApproval requests approval for a command execution
// Returns true if approved, false if denied
// Returns error if approval request fails or times out
func (am *ApprovalManager) RequestApproval(ctx context.Context, req ApprovalRequest) (bool, error) {
	if am.handler == nil {
		return false, fmt.Errorf("no approval handler configured")
	}

	// Apply default timeout if not specified
	timeout := req.Timeout
	if timeout == 0 {
		timeout = am.defaultTimeout
	}

	// Create timeout context
	timeoutCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	log.Info().
		Str("command", req.Command).
		Strs("args", req.Args).
		Str("agent_id", req.AgentID).
		Msg("Requesting approval")

	// Request approval
	responseChan := make(chan ApprovalResponse, 1)
	errorChan := make(chan error, 1)
	done := make(chan struct{})

	go func() {
		defer close(done)
		response, err := am.handler.RequestApproval(timeoutCtx, req)
		if err != nil {
			errorChan <- err
		} else {
			responseChan <- response
		}
	}()

	// Wait for response or timeout
	select {
	case response := <-responseChan:
		if response.Approved {
			log.Info().
				Str("command", req.Command).
				Str("reason", response.Reason).
				Msg("Approval granted")
		} else {
			log.Warn().
				Str("command", req.Command).
				Str("reason", response.Reason).
				Msg("Approval denied")
		}
		return response.Approved, nil

	case err := <-errorChan:
		log.Error().
			Err(err).
			Str("command", req.Command).
			Msg("Approval request failed")
		return false, fmt.Errorf("approval request failed: %w", err)

	case <-timeoutCtx.Done():
		// Give the handler a chance to observe cancellation and flush output.
		select {
		case <-done:
		case <-time.After(200 * time.Millisecond):
		}
		log.Warn().
			Str("command", req.Command).
			Dur("timeout", timeout).
			Msg("Approval request timed out")
		return false, fmt.Errorf("approval request timed out after %v", timeout)
	}
}

// SetDefaultTimeout sets the default timeout for approval requests
func (am *ApprovalManager) SetDefaultTimeout(timeout time.Duration) {
	am.defaultTimeout = timeout
}

// GetDefaultTimeout returns the default timeout
func (am *ApprovalManager) GetDefaultTimeout() time.Duration {
	return am.defaultTimeout
}

// SetHandler sets the approval handler
func (am *ApprovalManager) SetHandler(handler ApprovalHandler) {
	am.handler = handler
}

// ResolveApproval resolves a pending approval request by ID.
func (am *ApprovalManager) ResolveApproval(id string, action ApprovalAction, actor string) error {
	if am.handler == nil {
		return fmt.Errorf("no approval handler configured")
	}

	resolver, ok := am.handler.(ApprovalResolver)
	if !ok {
		return fmt.Errorf("approval handler does not support asynchronous resolution")
	}

	return resolver.ResolveApproval(id, action, actor)
}
