package toolexecutor

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNewPolicyEngine tests creating a new policy engine
func TestNewPolicyEngine(t *testing.T) {
	pe := NewPolicyEngine()
	assert.NotNil(t, pe)
}

// TestPolicyEngine_EvaluatePolicy tests policy evaluation
func TestPolicyEngine_EvaluatePolicy(t *testing.T) {
	pe := NewPolicyEngine()

	tests := []struct {
		name        string
		toolName    string
		policy      *ToolPolicy
		agentID     string
		expectAllow bool
	}{
		{
			name:        "nil policy allows all",
			toolName:    "any_tool",
			policy:      nil,
			agentID:     "test-agent",
			expectAllow: true,
		},
		{
			name:     "wildcard allow",
			toolName: "any_tool",
			policy: &ToolPolicy{
				Allow: []string{"*"},
				Deny:  []string{},
			},
			agentID:     "test-agent",
			expectAllow: true,
		},
		{
			name:     "specific allow",
			toolName: "read_file",
			policy: &ToolPolicy{
				Allow: []string{"read_file"},
				Deny:  []string{},
			},
			agentID:     "test-agent",
			expectAllow: true,
		},
		{
			name:     "specific deny",
			toolName: "exec",
			policy: &ToolPolicy{
				Allow: []string{"*"},
				Deny:  []string{"exec"},
			},
			agentID:     "test-agent",
			expectAllow: false,
		},
		{
			name:     "not in allow list",
			toolName: "write_file",
			policy: &ToolPolicy{
				Allow: []string{"read_file"},
				Deny:  []string{},
			},
			agentID:     "test-agent",
			expectAllow: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			allowed, err := pe.EvaluatePolicy(tt.toolName, tt.policy, tt.agentID)
			require.NoError(t, err)
			assert.Equal(t, tt.expectAllow, allowed)
		})
	}
}

// TestPolicyEngine_ValidatePolicy tests policy validation
func TestPolicyEngine_ValidatePolicy(t *testing.T) {
	pe := NewPolicyEngine()

	tests := []struct {
		name    string
		policy  *ToolPolicy
		wantErr bool
	}{
		{
			name:    "nil policy is valid",
			policy:  nil,
			wantErr: false,
		},
		{
			name: "valid policy with allow list",
			policy: &ToolPolicy{
				Allow: []string{"read_file", "write_file"},
				Deny:  []string{},
			},
			wantErr: false,
		},
		{
			name: "valid policy with deny list",
			policy: &ToolPolicy{
				Allow: []string{"*"},
				Deny:  []string{"exec"},
			},
			wantErr: false,
		},
		{
			name: "policy with both wildcards (warns but valid)",
			policy: &ToolPolicy{
				Allow: []string{"*"},
				Deny:  []string{"*"},
			},
			wantErr: false,
		},
		{
			name: "policy with empty allow list (warns but valid)",
			policy: &ToolPolicy{
				Allow: []string{},
				Deny:  []string{},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := pe.ValidatePolicy(tt.policy)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestPolicyEngine_CreatePolicyViolationError tests error creation
func TestPolicyEngine_CreatePolicyViolationError(t *testing.T) {
	pe := NewPolicyEngine()

	err := pe.CreatePolicyViolationError("exec", "test-agent")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "exec")
	assert.Contains(t, err.Error(), "not allowed by agent policy")
}

// TestPolicyEngine_GetPolicyMetadata tests metadata generation
func TestPolicyEngine_GetPolicyMetadata(t *testing.T) {
	pe := NewPolicyEngine()

	metadata := pe.GetPolicyMetadata("test-agent")
	assert.NotNil(t, metadata)
	assert.True(t, metadata["policy_violation"].(bool))
	assert.Equal(t, "test-agent", metadata["agent_id"])
}

// TestPolicyEngine_MergePolicies tests policy merging
func TestPolicyEngine_MergePolicies(t *testing.T) {
	pe := NewPolicyEngine()

	tests := []struct {
		name          string
		policies      []*ToolPolicy
		testTool      string
		expectedAllow bool
	}{
		{
			name:          "no policies returns nil",
			policies:      []*ToolPolicy{},
			testTool:      "any_tool",
			expectedAllow: true, // nil policy allows all
		},
		{
			name: "single policy",
			policies: []*ToolPolicy{
				{
					Allow: []string{"read_file"},
					Deny:  []string{},
				},
			},
			testTool:      "read_file",
			expectedAllow: true,
		},
		{
			name: "merge two policies - intersection of allows",
			policies: []*ToolPolicy{
				{
					Allow: []string{"read_file", "write_file"},
					Deny:  []string{},
				},
				{
					Allow: []string{"read_file", "search"},
					Deny:  []string{},
				},
			},
			testTool:      "read_file",
			expectedAllow: true,
		},
		{
			name: "merge two policies - union of denies",
			policies: []*ToolPolicy{
				{
					Allow: []string{"*"},
					Deny:  []string{"exec"},
				},
				{
					Allow: []string{"*"},
					Deny:  []string{"delete"},
				},
			},
			testTool:      "exec",
			expectedAllow: false,
		},
		{
			name: "merge with wildcard",
			policies: []*ToolPolicy{
				{
					Allow: []string{"*"},
					Deny:  []string{},
				},
				{
					Allow: []string{"read_file"},
					Deny:  []string{},
				},
			},
			testTool:      "read_file",
			expectedAllow: true,
		},
		{
			name: "nil policies filtered out",
			policies: []*ToolPolicy{
				nil,
				{
					Allow: []string{"read_file"},
					Deny:  []string{},
				},
				nil,
			},
			testTool:      "read_file",
			expectedAllow: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			merged := pe.MergePolicies(tt.policies...)

			if merged == nil {
				// nil policy allows all
				assert.True(t, tt.expectedAllow)
			} else {
				allowed := merged.IsToolAllowed(tt.testTool)
				assert.Equal(t, tt.expectedAllow, allowed)
			}
		})
	}
}

// TestPolicyEngine_FilterToolsByPolicy tests filtering tools by policy
func TestPolicyEngine_FilterToolsByPolicy(t *testing.T) {
	pe := NewPolicyEngine()

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
			filtered := pe.FilterToolsByPolicy(tools, tt.policy)
			assert.ElementsMatch(t, tt.expected, filtered)
		})
	}
}

// TestPolicyEngine_CheckToolCategory tests category checking
func TestPolicyEngine_CheckToolCategory(t *testing.T) {
	pe := NewPolicyEngine()
	registry := NewToolRegistry()

	// Register tools with categories
	require.NoError(t, registry.Register("read_file", "Read file", CategoryRead))
	require.NoError(t, registry.Register("write_file", "Write file", CategoryWrite))

	// Check categories
	assert.True(t, pe.CheckToolCategory("read_file", "read", registry))
	assert.False(t, pe.CheckToolCategory("read_file", "write", registry))
	assert.True(t, pe.CheckToolCategory("write_file", "write", registry))
	assert.False(t, pe.CheckToolCategory("unknown_tool", "read", registry))
}

// TestPolicyEngine_MergePolicies_ComplexScenarios tests complex merge scenarios
func TestPolicyEngine_MergePolicies_ComplexScenarios(t *testing.T) {
	pe := NewPolicyEngine()

	t.Run("three policies with overlapping allows", func(t *testing.T) {
		policies := []*ToolPolicy{
			{
				Allow: []string{"read_file", "write_file", "search"},
				Deny:  []string{},
			},
			{
				Allow: []string{"read_file", "search", "exec"},
				Deny:  []string{},
			},
			{
				Allow: []string{"read_file", "write_file"},
				Deny:  []string{},
			},
		}

		merged := pe.MergePolicies(policies...)
		require.NotNil(t, merged)

		// Only read_file is in all three allow lists
		assert.True(t, merged.IsToolAllowed("read_file"))
		// write_file is not in second policy
		assert.False(t, merged.IsToolAllowed("write_file"))
		// search is not in third policy
		assert.False(t, merged.IsToolAllowed("search"))
		// exec is not in first and third policies
		assert.False(t, merged.IsToolAllowed("exec"))
	})

	t.Run("policies with multiple denies", func(t *testing.T) {
		policies := []*ToolPolicy{
			{
				Allow: []string{"*"},
				Deny:  []string{"exec", "delete"},
			},
			{
				Allow: []string{"*"},
				Deny:  []string{"format", "exec"},
			},
		}

		merged := pe.MergePolicies(policies...)
		require.NotNil(t, merged)

		// All denied tools should be in merged deny list
		assert.False(t, merged.IsToolAllowed("exec"))
		assert.False(t, merged.IsToolAllowed("delete"))
		assert.False(t, merged.IsToolAllowed("format"))
		// Other tools should be allowed
		assert.True(t, merged.IsToolAllowed("read_file"))
	})

	t.Run("all nil policies", func(t *testing.T) {
		policies := []*ToolPolicy{nil, nil, nil}

		merged := pe.MergePolicies(policies...)
		assert.Nil(t, merged)
	})
}
