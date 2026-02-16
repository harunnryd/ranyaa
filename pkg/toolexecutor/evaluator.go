package toolexecutor

import (
	"fmt"

	"github.com/rs/zerolog/log"
)

// PolicyEvaluator evaluates tool policies and determines if tool execution should be allowed
type PolicyEvaluator struct {
	policyEngine *PolicyEngine
}

// NewPolicyEvaluator creates a new policy evaluator
func NewPolicyEvaluator(policyEngine *PolicyEngine) *PolicyEvaluator {
	if policyEngine == nil {
		policyEngine = NewPolicyEngine()
	}
	return &PolicyEvaluator{
		policyEngine: policyEngine,
	}
}

// EvaluationResult represents the result of a policy evaluation
type EvaluationResult struct {
	Allowed       bool                   // Whether the tool execution is allowed
	Reason        string                 // Reason for the decision
	ViolationType string                 // Type of violation (if any): "deny_list", "not_in_allow_list", ""
	Metadata      map[string]interface{} // Additional metadata
}

// Evaluate evaluates if a tool execution should be allowed based on the policy
// This is the main entry point for policy evaluation
func (pe *PolicyEvaluator) Evaluate(toolName string, policy *ToolPolicy, agentID string) EvaluationResult {
	// No policy means allow all
	if policy == nil {
		return EvaluationResult{
			Allowed:       true,
			Reason:        "no policy configured",
			ViolationType: "",
			Metadata:      map[string]interface{}{},
		}
	}

	// Check deny list first (deny list overrides allow list)
	if pe.isDenied(toolName, policy) {
		pe.logViolation(toolName, agentID, "deny_list")
		return EvaluationResult{
			Allowed:       false,
			Reason:        fmt.Sprintf("tool '%s' is in deny list", toolName),
			ViolationType: "deny_list",
			Metadata: map[string]interface{}{
				"policy_violation": true,
				"agent_id":         agentID,
				"tool_name":        toolName,
			},
		}
	}

	// Check allow list
	if !pe.isAllowed(toolName, policy) {
		pe.logViolation(toolName, agentID, "not_in_allow_list")
		return EvaluationResult{
			Allowed:       false,
			Reason:        fmt.Sprintf("tool '%s' is not in allow list", toolName),
			ViolationType: "not_in_allow_list",
			Metadata: map[string]interface{}{
				"policy_violation": true,
				"agent_id":         agentID,
				"tool_name":        toolName,
			},
		}
	}

	// Tool is allowed
	return EvaluationResult{
		Allowed:       true,
		Reason:        "tool is in allow list and not in deny list",
		ViolationType: "",
		Metadata:      map[string]interface{}{},
	}
}

// EvaluateMultiple evaluates multiple tools against a policy
// Returns a map of tool names to evaluation results
func (pe *PolicyEvaluator) EvaluateMultiple(toolNames []string, policy *ToolPolicy, agentID string) map[string]EvaluationResult {
	results := make(map[string]EvaluationResult)
	for _, toolName := range toolNames {
		results[toolName] = pe.Evaluate(toolName, policy, agentID)
	}
	return results
}

// isDenied checks if a tool is in the deny list
func (pe *PolicyEvaluator) isDenied(toolName string, policy *ToolPolicy) bool {
	if policy == nil {
		return false
	}

	for _, denied := range policy.Deny {
		if denied == toolName || denied == "*" {
			return true
		}
	}

	return false
}

// isAllowed checks if a tool is in the allow list
func (pe *PolicyEvaluator) isAllowed(toolName string, policy *ToolPolicy) bool {
	if policy == nil {
		return true
	}

	// Empty allow list means deny all
	if len(policy.Allow) == 0 {
		return false
	}

	for _, allowed := range policy.Allow {
		if allowed == toolName || allowed == "*" {
			return true
		}
	}

	return false
}

// logViolation logs a policy violation
func (pe *PolicyEvaluator) logViolation(toolName string, agentID string, violationType string) {
	log.Warn().
		Str("tool", toolName).
		Str("agent_id", agentID).
		Str("violation_type", violationType).
		Msg("Policy violation: tool execution blocked")
}

// GetAllowedTools returns a list of tools that are allowed by the policy
func (pe *PolicyEvaluator) GetAllowedTools(tools []string, policy *ToolPolicy, agentID string) []string {
	allowed := []string{}
	for _, tool := range tools {
		result := pe.Evaluate(tool, policy, agentID)
		if result.Allowed {
			allowed = append(allowed, tool)
		}
	}
	return allowed
}

// GetDeniedTools returns a list of tools that are denied by the policy
func (pe *PolicyEvaluator) GetDeniedTools(tools []string, policy *ToolPolicy, agentID string) []string {
	denied := []string{}
	for _, tool := range tools {
		result := pe.Evaluate(tool, policy, agentID)
		if !result.Allowed {
			denied = append(denied, tool)
		}
	}
	return denied
}

// CreateViolationError creates a standardized error for policy violations
func (pe *PolicyEvaluator) CreateViolationError(result EvaluationResult) error {
	if result.Allowed {
		return nil
	}
	return fmt.Errorf("policy violation: %s", result.Reason)
}
