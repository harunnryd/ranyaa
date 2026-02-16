package toolexecutor

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestIntegration_PolicyEnforcement_MultipleAgents tests policy enforcement across multiple agents
func TestIntegration_PolicyEnforcement_MultipleAgents(t *testing.T) {
	te := New()

	// Register tools
	tools := []string{"read_file", "write_file", "exec", "search"}
	for _, name := range tools {
		def := ToolDefinition{
			Name:        name,
			Description: "Test tool: " + name,
			Parameters:  []ToolParameter{},
			Handler: func(ctx context.Context, params map[string]interface{}) (interface{}, error) {
				return "executed", nil
			},
		}
		err := te.RegisterTool(def)
		require.NoError(t, err)
	}

	// Define agent policies
	captainPolicy := &ToolPolicy{
		Allow: []string{"search", "read_file"},
		Deny:  []string{},
	}

	executorPolicy := &ToolPolicy{
		Allow: []string{"*"},
		Deny:  []string{"exec"},
	}

	criticPolicy := &ToolPolicy{
		Allow: []string{"read_file"},
		Deny:  []string{},
	}

	tests := []struct {
		name        string
		agentID     string
		policy      *ToolPolicy
		toolName    string
		shouldAllow bool
	}{
		// Captain agent tests
		{
			name:        "captain can search",
			agentID:     "captain",
			policy:      captainPolicy,
			toolName:    "search",
			shouldAllow: true,
		},
		{
			name:        "captain can read",
			agentID:     "captain",
			policy:      captainPolicy,
			toolName:    "read_file",
			shouldAllow: true,
		},
		{
			name:        "captain cannot write",
			agentID:     "captain",
			policy:      captainPolicy,
			toolName:    "write_file",
			shouldAllow: false,
		},

		// Executor agent tests
		{
			name:        "executor can read",
			agentID:     "executor",
			policy:      executorPolicy,
			toolName:    "read_file",
			shouldAllow: true,
		},
		{
			name:        "executor can write",
			agentID:     "executor",
			policy:      executorPolicy,
			toolName:    "write_file",
			shouldAllow: true,
		},
		{
			name:        "executor cannot exec",
			agentID:     "executor",
			policy:      executorPolicy,
			toolName:    "exec",
			shouldAllow: false,
		},

		// Critic agent tests
		{
			name:        "critic can read",
			agentID:     "critic",
			policy:      criticPolicy,
			toolName:    "read_file",
			shouldAllow: true,
		},
		{
			name:        "critic cannot write",
			agentID:     "critic",
			policy:      criticPolicy,
			toolName:    "write_file",
			shouldAllow: false,
		},
		{
			name:        "critic cannot search",
			agentID:     "critic",
			policy:      criticPolicy,
			toolName:    "search",
			shouldAllow: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			execCtx := &ExecutionContext{
				AgentID:    tt.agentID,
				ToolPolicy: tt.policy,
			}

			result := te.Execute(context.Background(), tt.toolName, map[string]interface{}{}, execCtx)

			if tt.shouldAllow {
				assert.True(t, result.Success, "Expected tool to be allowed for agent %s", tt.agentID)
				assert.Empty(t, result.Error)
			} else {
				assert.False(t, result.Success, "Expected tool to be blocked for agent %s", tt.agentID)
				assert.Contains(t, result.Error, "not allowed by agent policy")
				assert.Equal(t, tt.agentID, result.Metadata["agent_id"])
			}
		})
	}
}

// TestIntegration_PolicyEnforcement_DynamicPolicyUpdate tests updating policies at runtime
func TestIntegration_PolicyEnforcement_DynamicPolicyUpdate(t *testing.T) {
	te := New()

	def := ToolDefinition{
		Name:        "test_tool",
		Description: "Test tool",
		Parameters:  []ToolParameter{},
		Handler: func(ctx context.Context, params map[string]interface{}) (interface{}, error) {
			return "success", nil
		},
	}

	err := te.RegisterTool(def)
	require.NoError(t, err)

	// Start with restrictive policy
	policy := &ToolPolicy{
		Allow: []string{},
		Deny:  []string{},
	}

	execCtx := &ExecutionContext{
		AgentID:    "test-agent",
		ToolPolicy: policy,
	}

	// Should be blocked
	result := te.Execute(context.Background(), "test_tool", map[string]interface{}{}, execCtx)
	assert.False(t, result.Success)

	// Update policy to allow tool
	policy.Allow = []string{"test_tool"}

	// Should now be allowed
	result = te.Execute(context.Background(), "test_tool", map[string]interface{}{}, execCtx)
	assert.True(t, result.Success)

	// Add to deny list
	policy.Deny = []string{"test_tool"}

	// Should be blocked again (deny overrides allow)
	result = te.Execute(context.Background(), "test_tool", map[string]interface{}{}, execCtx)
	assert.False(t, result.Success)
}

// TestIntegration_PolicyEnforcement_ConcurrentExecution tests policy enforcement with concurrent tool execution
func TestIntegration_PolicyEnforcement_ConcurrentExecution(t *testing.T) {
	te := New()

	// Register multiple tools
	for i := 0; i < 10; i++ {
		def := ToolDefinition{
			Name:        "tool_" + string(rune('0'+i)),
			Description: "Test tool",
			Parameters:  []ToolParameter{},
			Handler: func(ctx context.Context, params map[string]interface{}) (interface{}, error) {
				return "success", nil
			},
		}
		err := te.RegisterTool(def)
		require.NoError(t, err)
	}

	// Create policies for different agents
	policies := map[string]*ToolPolicy{
		"agent1": {
			Allow: []string{"tool_0", "tool_1", "tool_2"},
			Deny:  []string{},
		},
		"agent2": {
			Allow: []string{"*"},
			Deny:  []string{"tool_5", "tool_6"},
		},
		"agent3": {
			Allow: []string{"tool_7"},
			Deny:  []string{},
		},
	}

	// Execute tools concurrently from different agents
	done := make(chan bool, 30)

	for agentID, policy := range policies {
		for i := 0; i < 10; i++ {
			go func(aid string, pol *ToolPolicy, toolIdx int) {
				execCtx := &ExecutionContext{
					AgentID:    aid,
					ToolPolicy: pol,
				}

				toolName := "tool_" + string(rune('0'+toolIdx))
				result := te.Execute(context.Background(), toolName, map[string]interface{}{}, execCtx)

				// Verify policy is correctly enforced
				expectedAllow := pol.IsToolAllowed(toolName)
				assert.Equal(t, expectedAllow, result.Success,
					"Agent %s, tool %s: expected allow=%v, got success=%v",
					aid, toolName, expectedAllow, result.Success)

				done <- true
			}(agentID, policy, i)
		}
	}

	// Wait for all executions to complete
	for i := 0; i < 30; i++ {
		<-done
	}
}
