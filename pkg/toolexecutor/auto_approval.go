package toolexecutor

import "context"

// AutoApproveHandler approves every request without user interaction.
type AutoApproveHandler struct{}

// RequestApproval implements ApprovalHandler.
func (AutoApproveHandler) RequestApproval(_ context.Context, _ ApprovalRequest) (ApprovalResponse, error) {
	return ApprovalResponse{Approved: true, Reason: "auto-approved"}, nil
}
