package toolexecutor

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

// Unit test: Policy check before execution
func TestPolicyIntegration_CheckBeforeExecution(t *testing.T) {
	executor := New()
	registry := NewToolRegistry()

	// Register tool
	registry.Register("test_tool", "Test tool", CategoryGeneral)
	_ = executor.RegisterTool(ToolDefinition{
		Name:        "test_tool",
		Description: "Test tool",
		Parameters:  []ToolParameter{},
		Handler: func(ctx context.Context, params map[string]interface{}) (interface{}, error) {
			return "executed", nil
		},
	})

	t.Run("policy checked before execution", func(t *testing.T) {
		policy := &ToolPolicy{
			Allow: []string{"other_tool"},
			Deny:  []string{},
		}

		execCtx := &ExecutionContext{
			AgentID:    "test-agent",
			ToolPolicy: policy,
		}

		result := executor.Execute(context.Background(), "test_tool", map[string]interface{}{}, execCtx)
		assert.False(t, result.Success)
		assert.Contains(t, result.Error, "not allowed by agent policy")
		assert.NotNil(t, result.Metadata)
		assert.True(t, result.Metadata["policy_violation"].(bool))
	})

	t.Run("execution proceeds when policy allows", func(t *testing.T) {
		policy := &ToolPolicy{
			Allow: []string{"test_tool"},
			Deny:  []string{},
		}

		execCtx := &ExecutionContext{
			AgentID:    "test-agent",
			ToolPolicy: policy,
		}

		result := executor.Execute(context.Background(), "test_tool", map[string]interface{}{}, execCtx)
		assert.True(t, result.Success)
		assert.Equal(t, "executed", result.Output)
	})
}

// Unit test: Block execution if not allowed
func TestPolicyIntegration_BlockExecution(t *testing.T) {
	executor := New()

	_ = executor.RegisterTool(ToolDefinition{
		Name:        "blocked_tool",
		Description: "Tool that should be blocked",
		Parameters:  []ToolParameter{},
		Handler: func(ctx context.Context, params map[string]interface{}) (interface{}, error) {
			// This should never be called
			t.Fatal("Handler should not be called when tool is blocked")
			return nil, nil
		},
	})

	policy := &ToolPolicy{
		Allow: []string{},
		Deny:  []string{"blocked_tool"},
	}

	execCtx := &ExecutionContext{
		AgentID:    "test-agent",
		ToolPolicy: policy,
	}

	result := executor.Execute(context.Background(), "blocked_tool", map[string]interface{}{}, execCtx)
	assert.False(t, result.Success)
	assert.Contains(t, result.Error, "not allowed by agent policy")
}

// Unit test: Return policy violation error
func TestPolicyIntegration_PolicyViolationError(t *testing.T) {
	executor := New()

	executor.RegisterTool(ToolDefinition{
		Name:        "test_tool",
		Description: "Test tool",
		Parameters:  []ToolParameter{},
		Handler: func(ctx context.Context, params map[string]interface{}) (interface{}, error) {
			return "result", nil
		},
	})

	t.Run("policy violation error format", func(t *testing.T) {
		policy := &ToolPolicy{
			Allow: []string{"other_tool"},
			Deny:  []string{},
		}

		execCtx := &ExecutionContext{
			AgentID:    "test-agent",
			ToolPolicy: policy,
		}

		result := executor.Execute(context.Background(), "test_tool", map[string]interface{}{}, execCtx)
		assert.False(t, result.Success)
		assert.Contains(t, result.Error, "tool 'test_tool' is not allowed by agent policy")
		assert.NotNil(t, result.Metadata)
		assert.Equal(t, true, result.Metadata["policy_violation"])
		assert.Equal(t, "test-agent", result.Metadata["agent_id"])
	})

	t.Run("no error when policy allows", func(t *testing.T) {
		policy := &ToolPolicy{
			Allow: []string{"test_tool"},
			Deny:  []string{},
		}

		execCtx := &ExecutionContext{
			AgentID:    "test-agent",
			ToolPolicy: policy,
		}

		result := executor.Execute(context.Background(), "test_tool", map[string]interface{}{}, execCtx)
		assert.True(t, result.Success)
		assert.Empty(t, result.Error)
	})
}

// Unit test: Policy integration with nil policy
func TestPolicyIntegration_NilPolicy(t *testing.T) {
	executor := New()

	executor.RegisterTool(ToolDefinition{
		Name:        "test_tool",
		Description: "Test tool",
		Parameters:  []ToolParameter{},
		Handler: func(ctx context.Context, params map[string]interface{}) (interface{}, error) {
			return "result", nil
		},
	})

	t.Run("nil policy allows all tools", func(t *testing.T) {
		execCtx := &ExecutionContext{
			AgentID:    "test-agent",
			ToolPolicy: nil,
		}

		result := executor.Execute(context.Background(), "test_tool", map[string]interface{}{}, execCtx)
		assert.True(t, result.Success)
		assert.Equal(t, "result", result.Output)
	})

	t.Run("nil execution context allows all tools", func(t *testing.T) {
		result := executor.Execute(context.Background(), "test_tool", map[string]interface{}{}, nil)
		assert.True(t, result.Success)
		assert.Equal(t, "result", result.Output)
	})
}

// Integration test: Policy integration with multiple tools
func TestPolicyIntegration_Integration_MultipleTools(t *testing.T) {
	executor := New()
	registry := NewToolRegistry()

	// Register multiple tools
	tools := []string{"tool1", "tool2", "tool3", "tool4"}
	for _, toolName := range tools {
		registry.Register(toolName, "Test tool", CategoryGeneral)
		executor.RegisterTool(ToolDefinition{
			Name:        toolName,
			Description: "Test tool",
			Parameters:  []ToolParameter{},
			Handler: func(ctx context.Context, params map[string]interface{}) (interface{}, error) {
				return "executed", nil
			},
		})
	}

	policy := &ToolPolicy{
		Allow: []string{"tool1", "tool2"},
		Deny:  []string{"tool3"},
	}

	execCtx := &ExecutionContext{
		AgentID:    "test-agent",
		ToolPolicy: policy,
	}

	// tool1 should be allowed
	result := executor.Execute(context.Background(), "tool1", map[string]interface{}{}, execCtx)
	assert.True(t, result.Success)

	// tool2 should be allowed
	result = executor.Execute(context.Background(), "tool2", map[string]interface{}{}, execCtx)
	assert.True(t, result.Success)

	// tool3 should be denied (in deny list)
	result = executor.Execute(context.Background(), "tool3", map[string]interface{}{}, execCtx)
	assert.False(t, result.Success)
	assert.Contains(t, result.Error, "not allowed by agent policy")

	// tool4 should be denied (not in allow list)
	result = executor.Execute(context.Background(), "tool4", map[string]interface{}{}, execCtx)
	assert.False(t, result.Success)
	assert.Contains(t, result.Error, "not allowed by agent policy")
}

// Integration test: Policy integration with categories
func TestPolicyIntegration_Integration_WithCategories(t *testing.T) {
	executor := New()
	registry := NewToolRegistry()

	// Register tools with different categories
	registry.Register("read_tool", "Read tool", CategoryRead)
	registry.Register("write_tool", "Write tool", CategoryWrite)
	registry.Register("shell_tool", "Shell tool", CategoryShell)

	for _, toolName := range []string{"read_tool", "write_tool", "shell_tool"} {
		executor.RegisterTool(ToolDefinition{
			Name:        toolName,
			Description: "Test tool",
			Parameters:  []ToolParameter{},
			Handler: func(ctx context.Context, params map[string]interface{}) (interface{}, error) {
				return "executed", nil
			},
		})
	}

	// Policy that allows read and write, but denies shell
	policy := &ToolPolicy{
		Allow: []string{"read_tool", "write_tool"},
		Deny:  []string{"shell_tool"},
	}

	execCtx := &ExecutionContext{
		AgentID:    "test-agent",
		ToolPolicy: policy,
	}

	// Verify category matching
	matcher := NewCategoryMatcher(registry)
	assert.True(t, matcher.MatchesCategory("read_tool", CategoryRead))
	assert.True(t, matcher.MatchesCategory("write_tool", CategoryWrite))
	assert.True(t, matcher.MatchesCategory("shell_tool", CategoryShell))

	// Test execution with policy
	result := executor.Execute(context.Background(), "read_tool", map[string]interface{}{}, execCtx)
	assert.True(t, result.Success)

	result = executor.Execute(context.Background(), "write_tool", map[string]interface{}{}, execCtx)
	assert.True(t, result.Success)

	result = executor.Execute(context.Background(), "shell_tool", map[string]interface{}{}, execCtx)
	assert.False(t, result.Success)
}

// Integration test: Policy integration with concurrent execution
func TestPolicyIntegration_Integration_ConcurrentExecution(t *testing.T) {
	executor := New()

	// Register tools
	for i := 0; i < 10; i++ {
		toolName := fmt.Sprintf("tool_%d", i)
		executor.RegisterTool(ToolDefinition{
			Name:        toolName,
			Description: "Test tool",
			Parameters:  []ToolParameter{},
			Handler: func(ctx context.Context, params map[string]interface{}) (interface{}, error) {
				return "executed", nil
			},
		})
	}

	policy := &ToolPolicy{
		Allow: []string{"tool_0", "tool_1", "tool_2", "tool_3", "tool_4"},
		Deny:  []string{},
	}

	execCtx := &ExecutionContext{
		AgentID:    "test-agent",
		ToolPolicy: policy,
	}

	// Execute tools concurrently
	results := make(chan ToolResult, 10)
	for i := 0; i < 10; i++ {
		go func(index int) {
			toolName := fmt.Sprintf("tool_%d", index)
			result := executor.Execute(context.Background(), toolName, map[string]interface{}{}, execCtx)
			results <- result
		}(i)
	}

	// Collect results
	allowedCount := 0
	deniedCount := 0
	for i := 0; i < 10; i++ {
		result := <-results
		if result.Success {
			allowedCount++
		} else {
			deniedCount++
		}
	}

	assert.Equal(t, 5, allowedCount)
	assert.Equal(t, 5, deniedCount)
}

// Integration test: Policy integration with policy engine
func TestPolicyIntegration_Integration_WithPolicyEngine(t *testing.T) {
	executor := New()
	policyEngine := NewPolicyEngine()
	evaluator := NewPolicyEvaluator(policyEngine)

	// Register tools
	executor.RegisterTool(ToolDefinition{
		Name:        "allowed_tool",
		Description: "Allowed tool",
		Parameters:  []ToolParameter{},
		Handler: func(ctx context.Context, params map[string]interface{}) (interface{}, error) {
			return "executed", nil
		},
	})

	executor.RegisterTool(ToolDefinition{
		Name:        "denied_tool",
		Description: "Denied tool",
		Parameters:  []ToolParameter{},
		Handler: func(ctx context.Context, params map[string]interface{}) (interface{}, error) {
			return "executed", nil
		},
	})

	policy := &ToolPolicy{
		Allow: []string{"allowed_tool"},
		Deny:  []string{"denied_tool"},
	}

	// Evaluate policies
	allowedResult := evaluator.Evaluate("allowed_tool", policy, "test-agent")
	assert.True(t, allowedResult.Allowed)

	deniedResult := evaluator.Evaluate("denied_tool", policy, "test-agent")
	assert.False(t, deniedResult.Allowed)

	// Execute with policy
	execCtx := &ExecutionContext{
		AgentID:    "test-agent",
		ToolPolicy: policy,
	}

	result := executor.Execute(context.Background(), "allowed_tool", map[string]interface{}{}, execCtx)
	assert.True(t, result.Success)

	result = executor.Execute(context.Background(), "denied_tool", map[string]interface{}{}, execCtx)
	assert.False(t, result.Success)
}
