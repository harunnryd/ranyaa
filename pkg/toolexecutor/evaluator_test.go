package toolexecutor

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNewPolicyEvaluator tests creating a new policy evaluator
func TestNewPolicyEvaluator(t *testing.T) {
	t.Run("with policy engine", func(t *testing.T) {
		pe := NewPolicyEngine()
		evaluator := NewPolicyEvaluator(pe)
		assert.NotNil(t, evaluator)
		assert.NotNil(t, evaluator.policyEngine)
	})

	t.Run("with nil policy engine", func(t *testing.T) {
		evaluator := NewPolicyEvaluator(nil)
		assert.NotNil(t, evaluator)
		assert.NotNil(t, evaluator.policyEngine, "should create default policy engine")
	})
}

// TestPolicyEvaluator_Evaluate tests the main evaluation logic
func TestPolicyEvaluator_Evaluate(t *testing.T) {
	evaluator := NewPolicyEvaluator(nil)

	tests := []struct {
		name              string
		toolName          string
		policy            *ToolPolicy
		agentID           string
		expectAllowed     bool
		expectViolation   string
		expectReasonMatch string
	}{
		{
			name:              "nil policy allows all",
			toolName:          "any_tool",
			policy:            nil,
			agentID:           "test-agent",
			expectAllowed:     true,
			expectViolation:   "",
			expectReasonMatch: "no policy",
		},
		{
			name:     "wildcard allow",
			toolName: "any_tool",
			policy: &ToolPolicy{
				Allow: []string{"*"},
				Deny:  []string{},
			},
			agentID:           "test-agent",
			expectAllowed:     true,
			expectViolation:   "",
			expectReasonMatch: "allow list",
		},
		{
			name:     "specific tool in allow list",
			toolName: "read_file",
			policy: &ToolPolicy{
				Allow: []string{"read_file", "write_file"},
				Deny:  []string{},
			},
			agentID:           "test-agent",
			expectAllowed:     true,
			expectViolation:   "",
			expectReasonMatch: "allow list",
		},
		{
			name:     "tool in deny list",
			toolName: "exec",
			policy: &ToolPolicy{
				Allow: []string{"*"},
				Deny:  []string{"exec"},
			},
			agentID:           "test-agent",
			expectAllowed:     false,
			expectViolation:   "deny_list",
			expectReasonMatch: "deny list",
		},
		{
			name:     "deny list overrides allow list",
			toolName: "dangerous_tool",
			policy: &ToolPolicy{
				Allow: []string{"dangerous_tool", "safe_tool"},
				Deny:  []string{"dangerous_tool"},
			},
			agentID:           "test-agent",
			expectAllowed:     false,
			expectViolation:   "deny_list",
			expectReasonMatch: "deny list",
		},
		{
			name:     "tool not in allow list",
			toolName: "unknown_tool",
			policy: &ToolPolicy{
				Allow: []string{"read_file", "write_file"},
				Deny:  []string{},
			},
			agentID:           "test-agent",
			expectAllowed:     false,
			expectViolation:   "not_in_allow_list",
			expectReasonMatch: "not in allow list",
		},
		{
			name:     "wildcard deny blocks all",
			toolName: "any_tool",
			policy: &ToolPolicy{
				Allow: []string{"*"},
				Deny:  []string{"*"},
			},
			agentID:           "test-agent",
			expectAllowed:     false,
			expectViolation:   "deny_list",
			expectReasonMatch: "deny list",
		},
		{
			name:     "empty allow list denies all",
			toolName: "any_tool",
			policy: &ToolPolicy{
				Allow: []string{},
				Deny:  []string{},
			},
			agentID:           "test-agent",
			expectAllowed:     false,
			expectViolation:   "not_in_allow_list",
			expectReasonMatch: "not in allow list",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := evaluator.Evaluate(tt.toolName, tt.policy, tt.agentID)

			assert.Equal(t, tt.expectAllowed, result.Allowed, "allowed mismatch")
			assert.Equal(t, tt.expectViolation, result.ViolationType, "violation type mismatch")
			assert.Contains(t, result.Reason, tt.expectReasonMatch, "reason should contain expected text")

			if !tt.expectAllowed {
				assert.NotNil(t, result.Metadata)
				assert.True(t, result.Metadata["policy_violation"].(bool))
				assert.Equal(t, tt.agentID, result.Metadata["agent_id"])
				assert.Equal(t, tt.toolName, result.Metadata["tool_name"])
			}
		})
	}
}

// TestPolicyEvaluator_EvaluateMultiple tests evaluating multiple tools
func TestPolicyEvaluator_EvaluateMultiple(t *testing.T) {
	evaluator := NewPolicyEvaluator(nil)

	tools := []string{"read_file", "write_file", "exec", "search"}
	policy := &ToolPolicy{
		Allow: []string{"read_file", "write_file", "search"},
		Deny:  []string{"exec"},
	}

	results := evaluator.EvaluateMultiple(tools, policy, "test-agent")

	assert.Len(t, results, 4)
	assert.True(t, results["read_file"].Allowed)
	assert.True(t, results["write_file"].Allowed)
	assert.False(t, results["exec"].Allowed)
	assert.True(t, results["search"].Allowed)

	// Check violation type for denied tool
	assert.Equal(t, "deny_list", results["exec"].ViolationType)
}

// TestPolicyEvaluator_GetAllowedTools tests filtering allowed tools
func TestPolicyEvaluator_GetAllowedTools(t *testing.T) {
	evaluator := NewPolicyEvaluator(nil)

	tools := []string{"read_file", "write_file", "exec", "search"}

	tests := []struct {
		name     string
		policy   *ToolPolicy
		expected []string
	}{
		{
			name:     "nil policy allows all",
			policy:   nil,
			expected: []string{"read_file", "write_file", "exec", "search"},
		},
		{
			name: "wildcard allows all",
			policy: &ToolPolicy{
				Allow: []string{"*"},
				Deny:  []string{},
			},
			expected: []string{"read_file", "write_file", "exec", "search"},
		},
		{
			name: "specific allow list",
			policy: &ToolPolicy{
				Allow: []string{"read_file", "search"},
				Deny:  []string{},
			},
			expected: []string{"read_file", "search"},
		},
		{
			name: "deny list filters out tools",
			policy: &ToolPolicy{
				Allow: []string{"*"},
				Deny:  []string{"exec", "write_file"},
			},
			expected: []string{"read_file", "search"},
		},
		{
			name: "empty allow list denies all",
			policy: &ToolPolicy{
				Allow: []string{},
				Deny:  []string{},
			},
			expected: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			allowed := evaluator.GetAllowedTools(tools, tt.policy, "test-agent")
			assert.ElementsMatch(t, tt.expected, allowed)
		})
	}
}

// TestPolicyEvaluator_GetDeniedTools tests filtering denied tools
func TestPolicyEvaluator_GetDeniedTools(t *testing.T) {
	evaluator := NewPolicyEvaluator(nil)

	tools := []string{"read_file", "write_file", "exec", "search"}

	tests := []struct {
		name     string
		policy   *ToolPolicy
		expected []string
	}{
		{
			name:     "nil policy denies none",
			policy:   nil,
			expected: []string{},
		},
		{
			name: "wildcard allows all, denies none",
			policy: &ToolPolicy{
				Allow: []string{"*"},
				Deny:  []string{},
			},
			expected: []string{},
		},
		{
			name: "specific allow list denies others",
			policy: &ToolPolicy{
				Allow: []string{"read_file", "search"},
				Deny:  []string{},
			},
			expected: []string{"write_file", "exec"},
		},
		{
			name: "deny list blocks specific tools",
			policy: &ToolPolicy{
				Allow: []string{"*"},
				Deny:  []string{"exec", "write_file"},
			},
			expected: []string{"write_file", "exec"},
		},
		{
			name: "empty allow list denies all",
			policy: &ToolPolicy{
				Allow: []string{},
				Deny:  []string{},
			},
			expected: []string{"read_file", "write_file", "exec", "search"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			denied := evaluator.GetDeniedTools(tools, tt.policy, "test-agent")
			assert.ElementsMatch(t, tt.expected, denied)
		})
	}
}

// TestPolicyEvaluator_CreateViolationError tests error creation
func TestPolicyEvaluator_CreateViolationError(t *testing.T) {
	evaluator := NewPolicyEvaluator(nil)

	t.Run("allowed result returns nil error", func(t *testing.T) {
		result := EvaluationResult{
			Allowed: true,
			Reason:  "tool is allowed",
		}
		err := evaluator.CreateViolationError(result)
		assert.NoError(t, err)
	})

	t.Run("denied result returns error", func(t *testing.T) {
		result := EvaluationResult{
			Allowed: false,
			Reason:  "tool 'exec' is in deny list",
		}
		err := evaluator.CreateViolationError(result)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "policy violation")
		assert.Contains(t, err.Error(), "deny list")
	})
}

// TestPolicyEvaluator_DenyOverridesAllow tests that deny list always overrides allow list
func TestPolicyEvaluator_DenyOverridesAllow(t *testing.T) {
	evaluator := NewPolicyEvaluator(nil)

	tests := []struct {
		name     string
		toolName string
		policy   *ToolPolicy
	}{
		{
			name:     "specific tool in both lists",
			toolName: "dangerous_tool",
			policy: &ToolPolicy{
				Allow: []string{"dangerous_tool", "safe_tool"},
				Deny:  []string{"dangerous_tool"},
			},
		},
		{
			name:     "wildcard allow with specific deny",
			toolName: "exec",
			policy: &ToolPolicy{
				Allow: []string{"*"},
				Deny:  []string{"exec"},
			},
		},
		{
			name:     "specific allow with wildcard deny",
			toolName: "any_tool",
			policy: &ToolPolicy{
				Allow: []string{"any_tool"},
				Deny:  []string{"*"},
			},
		},
		{
			name:     "both wildcards",
			toolName: "any_tool",
			policy: &ToolPolicy{
				Allow: []string{"*"},
				Deny:  []string{"*"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := evaluator.Evaluate(tt.toolName, tt.policy, "test-agent")
			assert.False(t, result.Allowed, "deny should override allow")
			assert.Equal(t, "deny_list", result.ViolationType)
		})
	}
}
