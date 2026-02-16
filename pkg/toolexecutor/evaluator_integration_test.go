package toolexecutor

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestEvaluatorIntegration_WithToolExecutor tests evaluator integration with tool executor
func TestEvaluatorIntegration_WithToolExecutor(t *testing.T) {
	// Create tool executor
	executor := New()

	// Register test tools
	readTool := ToolDefinition{
		Name:        "read_file",
		Description: "Read a file",
		Parameters:  []ToolParameter{},
		Handler: func(ctx context.Context, params map[string]interface{}) (interface{}, error) {
			return "file content", nil
		},
	}

	writeTool := ToolDefinition{
		Name:        "write_file",
		Description: "Write a file",
		Parameters:  []ToolParameter{},
		Handler: func(ctx context.Context, params map[string]interface{}) (interface{}, error) {
			return "written", nil
		},
	}

	execTool := ToolDefinition{
		Name:        "exec",
		Description: "Execute command",
		Parameters:  []ToolParameter{},
		Handler: func(ctx context.Context, params map[string]interface{}) (interface{}, error) {
			return "executed", nil
		},
	}

	require.NoError(t, executor.RegisterTool(readTool))
	require.NoError(t, executor.RegisterTool(writeTool))
	require.NoError(t, executor.RegisterTool(execTool))

	// Create evaluator
	evaluator := NewPolicyEvaluator(nil)

	t.Run("allow read and write, deny exec", func(t *testing.T) {
		policy := &ToolPolicy{
			Allow: []string{"read_file", "write_file"},
			Deny:  []string{"exec"},
		}

		// Test read_file - should be allowed
		result := evaluator.Evaluate("read_file", policy, "test-agent")
		assert.True(t, result.Allowed)

		execResult := executor.Execute(context.Background(), "read_file", map[string]interface{}{}, &ExecutionContext{
			AgentID:    "test-agent",
			ToolPolicy: policy,
		})
		assert.True(t, execResult.Success)

		// Test exec - should be denied
		result = evaluator.Evaluate("exec", policy, "test-agent")
		assert.False(t, result.Allowed)

		execResult = executor.Execute(context.Background(), "exec", map[string]interface{}{}, &ExecutionContext{
			AgentID:    "test-agent",
			ToolPolicy: policy,
		})
		assert.False(t, execResult.Success)
		assert.Contains(t, execResult.Error, "not allowed by agent policy")
	})

	t.Run("wildcard allow with specific deny", func(t *testing.T) {
		policy := &ToolPolicy{
			Allow: []string{"*"},
			Deny:  []string{"exec"},
		}

		// All tools should be allowed except exec
		tools := []string{"read_file", "write_file", "exec"}
		allowed := evaluator.GetAllowedTools(tools, policy, "test-agent")
		assert.ElementsMatch(t, []string{"read_file", "write_file"}, allowed)

		// Verify with executor
		execResult := executor.Execute(context.Background(), "read_file", map[string]interface{}{}, &ExecutionContext{
			AgentID:    "test-agent",
			ToolPolicy: policy,
		})
		assert.True(t, execResult.Success)

		execResult = executor.Execute(context.Background(), "exec", map[string]interface{}{}, &ExecutionContext{
			AgentID:    "test-agent",
			ToolPolicy: policy,
		})
		assert.False(t, execResult.Success)
	})
}

// TestEvaluatorIntegration_WithPolicyEngine tests evaluator integration with policy engine
func TestEvaluatorIntegration_WithPolicyEngine(t *testing.T) {
	policyEngine := NewPolicyEngine()
	evaluator := NewPolicyEvaluator(policyEngine)

	t.Run("evaluator uses policy engine for validation", func(t *testing.T) {
		policy := &ToolPolicy{
			Allow: []string{"read_file", "write_file"},
			Deny:  []string{"exec"},
		}

		// Validate policy
		err := policyEngine.ValidatePolicy(policy)
		assert.NoError(t, err)

		// Evaluate with evaluator
		result := evaluator.Evaluate("read_file", policy, "test-agent")
		assert.True(t, result.Allowed)

		result = evaluator.Evaluate("exec", policy, "test-agent")
		assert.False(t, result.Allowed)
	})

	t.Run("evaluator handles merged policies", func(t *testing.T) {
		policy1 := &ToolPolicy{
			Allow: []string{"read_file", "write_file", "search"},
			Deny:  []string{},
		}

		policy2 := &ToolPolicy{
			Allow: []string{"read_file", "search", "exec"},
			Deny:  []string{"exec"},
		}

		// Merge policies
		merged := policyEngine.MergePolicies(policy1, policy2)
		require.NotNil(t, merged)

		// Evaluate with merged policy
		// read_file and search should be allowed (intersection of allows)
		result := evaluator.Evaluate("read_file", merged, "test-agent")
		assert.True(t, result.Allowed)

		result = evaluator.Evaluate("search", merged, "test-agent")
		assert.True(t, result.Allowed)

		// exec should be denied (union of denies)
		result = evaluator.Evaluate("exec", merged, "test-agent")
		assert.False(t, result.Allowed)

		// write_file should be denied (not in intersection)
		result = evaluator.Evaluate("write_file", merged, "test-agent")
		assert.False(t, result.Allowed)
	})
}

// TestEvaluatorIntegration_MultipleAgents tests evaluator with multiple agents
func TestEvaluatorIntegration_MultipleAgents(t *testing.T) {
	executor := New()
	evaluator := NewPolicyEvaluator(nil)

	// Register tools
	tools := []ToolDefinition{
		{
			Name:        "read_file",
			Description: "Read a file",
			Parameters:  []ToolParameter{},
			Handler: func(ctx context.Context, params map[string]interface{}) (interface{}, error) {
				return "content", nil
			},
		},
		{
			Name:        "write_file",
			Description: "Write a file",
			Parameters:  []ToolParameter{},
			Handler: func(ctx context.Context, params map[string]interface{}) (interface{}, error) {
				return "written", nil
			},
		},
		{
			Name:        "exec",
			Description: "Execute command",
			Parameters:  []ToolParameter{},
			Handler: func(ctx context.Context, params map[string]interface{}) (interface{}, error) {
				return "executed", nil
			},
		},
	}

	for _, tool := range tools {
		require.NoError(t, executor.RegisterTool(tool))
	}

	// Define policies for different agents
	captainPolicy := &ToolPolicy{
		Allow: []string{"read_file"},
		Deny:  []string{"exec", "write_file"},
	}

	executorPolicy := &ToolPolicy{
		Allow: []string{"*"},
		Deny:  []string{"exec"},
	}

	criticPolicy := &ToolPolicy{
		Allow: []string{"read_file"},
		Deny:  []string{},
	}

	t.Run("captain agent - read only", func(t *testing.T) {
		result := evaluator.Evaluate("read_file", captainPolicy, "captain")
		assert.True(t, result.Allowed)

		result = evaluator.Evaluate("write_file", captainPolicy, "captain")
		assert.False(t, result.Allowed)

		result = evaluator.Evaluate("exec", captainPolicy, "captain")
		assert.False(t, result.Allowed)
	})

	t.Run("executor agent - all except exec", func(t *testing.T) {
		result := evaluator.Evaluate("read_file", executorPolicy, "executor")
		assert.True(t, result.Allowed)

		result = evaluator.Evaluate("write_file", executorPolicy, "executor")
		assert.True(t, result.Allowed)

		result = evaluator.Evaluate("exec", executorPolicy, "executor")
		assert.False(t, result.Allowed)
	})

	t.Run("critic agent - read only", func(t *testing.T) {
		result := evaluator.Evaluate("read_file", criticPolicy, "critic")
		assert.True(t, result.Allowed)

		result = evaluator.Evaluate("write_file", criticPolicy, "critic")
		assert.False(t, result.Allowed)

		result = evaluator.Evaluate("exec", criticPolicy, "critic")
		assert.False(t, result.Allowed)
	})
}

// TestEvaluatorIntegration_PolicyViolationLogging tests that violations are logged
func TestEvaluatorIntegration_PolicyViolationLogging(t *testing.T) {
	evaluator := NewPolicyEvaluator(nil)

	policy := &ToolPolicy{
		Allow: []string{"read_file"},
		Deny:  []string{"exec"},
	}

	t.Run("deny list violation is logged", func(t *testing.T) {
		result := evaluator.Evaluate("exec", policy, "test-agent")
		assert.False(t, result.Allowed)
		assert.Equal(t, "deny_list", result.ViolationType)
		assert.NotNil(t, result.Metadata)
		assert.True(t, result.Metadata["policy_violation"].(bool))
	})

	t.Run("not in allow list violation is logged", func(t *testing.T) {
		result := evaluator.Evaluate("write_file", policy, "test-agent")
		assert.False(t, result.Allowed)
		assert.Equal(t, "not_in_allow_list", result.ViolationType)
		assert.NotNil(t, result.Metadata)
		assert.True(t, result.Metadata["policy_violation"].(bool))
	})

	t.Run("allowed tools have no violation", func(t *testing.T) {
		result := evaluator.Evaluate("read_file", policy, "test-agent")
		assert.True(t, result.Allowed)
		assert.Equal(t, "", result.ViolationType)
	})
}
