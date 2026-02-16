package toolexecutor

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestToolPolicy_IsToolAllowed_AllowAll tests allowing all tools with wildcard
func TestToolPolicy_IsToolAllowed_AllowAll(t *testing.T) {
	policy := &ToolPolicy{
		Allow: []string{"*"},
		Deny:  []string{},
	}

	assert.True(t, policy.IsToolAllowed("any_tool"))
	assert.True(t, policy.IsToolAllowed("another_tool"))
	assert.True(t, policy.IsToolAllowed("exec"))
}

// TestToolPolicy_IsToolAllowed_DenyAll tests denying all tools with wildcard
func TestToolPolicy_IsToolAllowed_DenyAll(t *testing.T) {
	policy := &ToolPolicy{
		Allow: []string{"*"},
		Deny:  []string{"*"},
	}

	// Deny overrides allow
	assert.False(t, policy.IsToolAllowed("any_tool"))
	assert.False(t, policy.IsToolAllowed("another_tool"))
	assert.False(t, policy.IsToolAllowed("exec"))
}

// TestToolPolicy_IsToolAllowed_SpecificAllow tests allowing specific tools
func TestToolPolicy_IsToolAllowed_SpecificAllow(t *testing.T) {
	policy := &ToolPolicy{
		Allow: []string{"read_file", "list_files"},
		Deny:  []string{},
	}

	assert.True(t, policy.IsToolAllowed("read_file"))
	assert.True(t, policy.IsToolAllowed("list_files"))
	assert.False(t, policy.IsToolAllowed("write_file"))
	assert.False(t, policy.IsToolAllowed("exec"))
}

// TestToolPolicy_IsToolAllowed_DenyOverridesAllow tests that deny list overrides allow list
func TestToolPolicy_IsToolAllowed_DenyOverridesAllow(t *testing.T) {
	policy := &ToolPolicy{
		Allow: []string{"*"},
		Deny:  []string{"exec", "delete_file"},
	}

	assert.True(t, policy.IsToolAllowed("read_file"))
	assert.True(t, policy.IsToolAllowed("write_file"))
	assert.False(t, policy.IsToolAllowed("exec"))
	assert.False(t, policy.IsToolAllowed("delete_file"))
}

// TestToolPolicy_IsToolAllowed_NilPolicy tests that nil policy allows all
func TestToolPolicy_IsToolAllowed_NilPolicy(t *testing.T) {
	var policy *ToolPolicy = nil

	assert.True(t, policy.IsToolAllowed("any_tool"))
	assert.True(t, policy.IsToolAllowed("exec"))
}

// TestToolExecutor_Execute_PolicyEnforcement tests policy enforcement during execution
func TestToolExecutor_Execute_PolicyEnforcement(t *testing.T) {
	te := New()

	// Register test tools
	tools := []string{"read_file", "write_file", "exec"}
	for _, name := range tools {
		def := ToolDefinition{
			Name:        name,
			Description: "Test tool",
			Parameters:  []ToolParameter{},
			Handler: func(ctx context.Context, params map[string]interface{}) (interface{}, error) {
				return "success", nil
			},
		}
		err := te.RegisterTool(def)
		require.NoError(t, err)
	}

	tests := []struct {
		name        string
		policy      *ToolPolicy
		toolName    string
		shouldAllow bool
	}{
		{
			name: "allow all with wildcard",
			policy: &ToolPolicy{
				Allow: []string{"*"},
				Deny:  []string{},
			},
			toolName:    "read_file",
			shouldAllow: true,
		},
		{
			name: "deny specific tool",
			policy: &ToolPolicy{
				Allow: []string{"*"},
				Deny:  []string{"exec"},
			},
			toolName:    "exec",
			shouldAllow: false,
		},
		{
			name: "allow specific tools only",
			policy: &ToolPolicy{
				Allow: []string{"read_file"},
				Deny:  []string{},
			},
			toolName:    "read_file",
			shouldAllow: true,
		},
		{
			name: "deny tool not in allow list",
			policy: &ToolPolicy{
				Allow: []string{"read_file"},
				Deny:  []string{},
			},
			toolName:    "write_file",
			shouldAllow: false,
		},
		{
			name:        "nil policy allows all",
			policy:      nil,
			toolName:    "exec",
			shouldAllow: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			execCtx := &ExecutionContext{
				AgentID:    "test-agent",
				ToolPolicy: tt.policy,
			}

			result := te.Execute(context.Background(), tt.toolName, map[string]interface{}{}, execCtx)

			if tt.shouldAllow {
				assert.True(t, result.Success, "Expected tool to be allowed")
				assert.Empty(t, result.Error)
			} else {
				assert.False(t, result.Success, "Expected tool to be blocked")
				assert.Contains(t, result.Error, "not allowed by agent policy")
				assert.NotNil(t, result.Metadata)
				assert.True(t, result.Metadata["policy_violation"].(bool))
			}
		})
	}
}

// TestToolExecutor_Execute_PolicyViolationLogging tests that policy violations are logged
func TestToolExecutor_Execute_PolicyViolationLogging(t *testing.T) {
	te := New()

	def := ToolDefinition{
		Name:        "dangerous_tool",
		Description: "A dangerous tool",
		Parameters:  []ToolParameter{},
		Handler: func(ctx context.Context, params map[string]interface{}) (interface{}, error) {
			return "should not execute", nil
		},
	}

	err := te.RegisterTool(def)
	require.NoError(t, err)

	policy := &ToolPolicy{
		Allow: []string{"safe_tool"},
		Deny:  []string{},
	}

	execCtx := &ExecutionContext{
		AgentID:    "test-agent",
		ToolPolicy: policy,
	}

	result := te.Execute(context.Background(), "dangerous_tool", map[string]interface{}{}, execCtx)

	assert.False(t, result.Success)
	assert.Contains(t, result.Error, "not allowed by agent policy")
	assert.Equal(t, "test-agent", result.Metadata["agent_id"])
	assert.True(t, result.Metadata["policy_violation"].(bool))
}

// TestToolExecutor_Execute_NoPolicy tests execution without policy (should allow all)
func TestToolExecutor_Execute_NoPolicy(t *testing.T) {
	te := New()

	def := ToolDefinition{
		Name:        "any_tool",
		Description: "Any tool",
		Parameters:  []ToolParameter{},
		Handler: func(ctx context.Context, params map[string]interface{}) (interface{}, error) {
			return "success", nil
		},
	}

	err := te.RegisterTool(def)
	require.NoError(t, err)

	// Execute without execution context
	result := te.Execute(context.Background(), "any_tool", map[string]interface{}{}, nil)
	assert.True(t, result.Success)

	// Execute with execution context but no policy
	execCtx := &ExecutionContext{
		AgentID: "test-agent",
	}
	result = te.Execute(context.Background(), "any_tool", map[string]interface{}{}, execCtx)
	assert.True(t, result.Success)
}

// TestToolPolicy_ComplexScenarios tests complex policy scenarios
func TestToolPolicy_ComplexScenarios(t *testing.T) {
	tests := []struct {
		name     string
		policy   *ToolPolicy
		toolName string
		expected bool
	}{
		{
			name: "allow all except specific",
			policy: &ToolPolicy{
				Allow: []string{"*"},
				Deny:  []string{"exec", "delete"},
			},
			toolName: "read",
			expected: true,
		},
		{
			name: "deny overrides allow for same tool",
			policy: &ToolPolicy{
				Allow: []string{"exec"},
				Deny:  []string{"exec"},
			},
			toolName: "exec",
			expected: false,
		},
		{
			name: "empty allow list denies all",
			policy: &ToolPolicy{
				Allow: []string{},
				Deny:  []string{},
			},
			toolName: "any_tool",
			expected: false,
		},
		{
			name: "multiple specific allows",
			policy: &ToolPolicy{
				Allow: []string{"read", "write", "list"},
				Deny:  []string{},
			},
			toolName: "write",
			expected: true,
		},
		{
			name: "multiple specific denies",
			policy: &ToolPolicy{
				Allow: []string{"*"},
				Deny:  []string{"exec", "delete", "format"},
			},
			toolName: "format",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.policy.IsToolAllowed(tt.toolName)
			assert.Equal(t, tt.expected, result)
		})
	}
}
