package toolexecutor

import (
	"fmt"

	"github.com/rs/zerolog/log"
)

// PolicyEngine manages tool access policies
type PolicyEngine struct {
	// Future: Add policy storage, caching, etc.
}

// NewPolicyEngine creates a new policy engine
func NewPolicyEngine() *PolicyEngine {
	return &PolicyEngine{}
}

// EvaluatePolicy evaluates if a tool execution should be allowed
// Returns true if allowed, false if denied
func (pe *PolicyEngine) EvaluatePolicy(toolName string, policy *ToolPolicy, agentID string) (bool, error) {
	if policy == nil {
		// No policy means allow all
		return true, nil
	}

	allowed := policy.IsToolAllowed(toolName)

	if !allowed {
		log.Warn().
			Str("tool", toolName).
			Str("agent_id", agentID).
			Msg("Tool execution blocked by policy")
	}

	return allowed, nil
}

// ValidatePolicy validates a tool policy configuration
func (pe *PolicyEngine) ValidatePolicy(policy *ToolPolicy) error {
	if policy == nil {
		return nil
	}

	// Check for conflicting wildcard rules
	hasAllowWildcard := false
	hasDenyWildcard := false

	for _, allowed := range policy.Allow {
		if allowed == "*" {
			hasAllowWildcard = true
			break
		}
	}

	for _, denied := range policy.Deny {
		if denied == "*" {
			hasDenyWildcard = true
			break
		}
	}

	// Warn if both allow and deny have wildcards (deny will win)
	if hasAllowWildcard && hasDenyWildcard {
		log.Warn().Msg("Policy has both allow and deny wildcards - deny will override allow")
	}

	// Warn if allow list is empty and no wildcard
	if len(policy.Allow) == 0 {
		log.Warn().Msg("Policy has empty allow list - all tools will be denied by default")
	}

	return nil
}

// CreatePolicyViolationError creates a standardized policy violation error
func (pe *PolicyEngine) CreatePolicyViolationError(toolName string, agentID string) error {
	return fmt.Errorf("tool '%s' is not allowed by agent policy", toolName)
}

// GetPolicyMetadata returns metadata about a policy violation
func (pe *PolicyEngine) GetPolicyMetadata(agentID string) map[string]interface{} {
	return map[string]interface{}{
		"policy_violation": true,
		"agent_id":         agentID,
	}
}

// MergePolicies merges multiple policies into one
// The resulting policy is the intersection of all allow lists
// and the union of all deny lists
func (pe *PolicyEngine) MergePolicies(policies ...*ToolPolicy) *ToolPolicy {
	if len(policies) == 0 {
		return nil
	}

	// Filter out nil policies
	validPolicies := []*ToolPolicy{}
	for _, p := range policies {
		if p != nil {
			validPolicies = append(validPolicies, p)
		}
	}

	if len(validPolicies) == 0 {
		return nil
	}

	if len(validPolicies) == 1 {
		return validPolicies[0]
	}

	merged := &ToolPolicy{
		Allow: []string{},
		Deny:  []string{},
	}

	// Collect all deny rules (union)
	denySet := make(map[string]bool)
	for _, policy := range validPolicies {
		for _, denied := range policy.Deny {
			denySet[denied] = true
		}
	}

	for denied := range denySet {
		merged.Deny = append(merged.Deny, denied)
	}

	// For allow rules, we need intersection
	// Start with first policy's allow list
	allowSet := make(map[string]bool)
	for _, allowed := range validPolicies[0].Allow {
		allowSet[allowed] = true
	}

	// Intersect with remaining policies
	for i := 1; i < len(validPolicies); i++ {
		policyAllowSet := make(map[string]bool)
		for _, allowed := range validPolicies[i].Allow {
			policyAllowSet[allowed] = true
		}

		// Keep only items that exist in both sets
		newAllowSet := make(map[string]bool)
		for allowed := range allowSet {
			if policyAllowSet[allowed] || policyAllowSet["*"] || allowed == "*" {
				newAllowSet[allowed] = true
			}
		}
		allowSet = newAllowSet
	}

	for allowed := range allowSet {
		merged.Allow = append(merged.Allow, allowed)
	}

	return merged
}

// CheckToolCategory checks if a tool belongs to a specific category
// Categories: read, write, shell, web, spec, general
func (pe *PolicyEngine) CheckToolCategory(toolName string, category string, registry *ToolRegistry) bool {
	if registry == nil {
		return false
	}

	toolCategory, err := registry.GetCategory(toolName)
	if err != nil {
		return false
	}

	return string(toolCategory) == category
}

// FilterToolsByPolicy filters a list of tools based on a policy
func (pe *PolicyEngine) FilterToolsByPolicy(tools []string, policy *ToolPolicy) []string {
	if policy == nil {
		return tools
	}

	filtered := []string{}
	for _, tool := range tools {
		if policy.IsToolAllowed(tool) {
			filtered = append(filtered, tool)
		}
	}

	return filtered
}
