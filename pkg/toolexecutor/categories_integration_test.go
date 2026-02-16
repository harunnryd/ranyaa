package toolexecutor

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Integration test: Tool registry with executor
func TestToolRegistry_Integration_WithExecutor(t *testing.T) {
	// Create tool executor and registry
	executor := New()
	registry := NewToolRegistry()

	// Register tools with categories
	require.NoError(t, registry.Register("read_file", "Read a file", CategoryRead))
	require.NoError(t, registry.Register("write_file", "Write a file", CategoryWrite))
	require.NoError(t, registry.Register("exec_shell", "Execute shell command", CategoryShell))

	// Register tools in executor
	executor.RegisterTool(ToolDefinition{
		Name:        "read_file",
		Description: "Read a file",
		Parameters:  []ToolParameter{},
		Handler: func(ctx context.Context, params map[string]interface{}) (interface{}, error) {
			return "file content", nil
		},
	})

	executor.RegisterTool(ToolDefinition{
		Name:        "write_file",
		Description: "Write a file",
		Parameters:  []ToolParameter{},
		Handler: func(ctx context.Context, params map[string]interface{}) (interface{}, error) {
			return "file written", nil
		},
	})

	executor.RegisterTool(ToolDefinition{
		Name:        "exec_shell",
		Description: "Execute shell command",
		Parameters:  []ToolParameter{},
		Handler: func(ctx context.Context, params map[string]interface{}) (interface{}, error) {
			return "command executed", nil
		},
	})

	// Verify tools are registered in both
	assert.Equal(t, 3, executor.GetToolCount())
	assert.Len(t, registry.List(), 3)

	// Filter by category
	readTools := registry.FilterByCategory(CategoryRead)
	assert.Len(t, readTools, 1)
	assert.Equal(t, "read_file", readTools[0].Name)
}

// Integration test: Category-based policy enforcement
func TestCategoryPolicy_Integration_WithExecutor(t *testing.T) {
	executor := New()
	registry := NewToolRegistry()

	// Register tools
	registry.Register("read_file", "Read file", CategoryRead)
	registry.Register("write_file", "Write file", CategoryWrite)

	executor.RegisterTool(ToolDefinition{
		Name:        "read_file",
		Description: "Read file",
		Parameters:  []ToolParameter{},
		Handler: func(ctx context.Context, params map[string]interface{}) (interface{}, error) {
			return "content", nil
		},
	})

	executor.RegisterTool(ToolDefinition{
		Name:        "write_file",
		Description: "Write file",
		Parameters:  []ToolParameter{},
		Handler: func(ctx context.Context, params map[string]interface{}) (interface{}, error) {
			return "written", nil
		},
	})

	// Create policy that only allows read tools
	policy := &ToolPolicy{
		Allow: []string{"read_file"},
		Deny:  []string{},
	}

	// Execute read tool (should succeed)
	execCtx := &ExecutionContext{
		AgentID:    "test-agent",
		ToolPolicy: policy,
	}

	result := executor.Execute(context.Background(), "read_file", map[string]interface{}{}, execCtx)
	assert.True(t, result.Success)
	assert.Equal(t, "content", result.Output)

	// Execute write tool (should fail due to policy)
	result = executor.Execute(context.Background(), "write_file", map[string]interface{}{}, execCtx)
	assert.False(t, result.Success)
	assert.Contains(t, result.Error, "not allowed by agent policy")
}

// Integration test: Category matcher with policy evaluator
func TestCategoryMatcher_Integration_WithPolicyEvaluator(t *testing.T) {
	registry := NewToolRegistry()
	registry.Register("read_file", "Read file", CategoryRead)
	registry.Register("write_file", "Write file", CategoryWrite)
	registry.Register("exec_command", "Execute command", CategoryShell)

	matcher := NewCategoryMatcher(registry)
	policyEngine := NewPolicyEngine()
	evaluator := NewPolicyEvaluator(policyEngine)

	// Create policy that allows read and write, but denies shell
	policy := &ToolPolicy{
		Allow: []string{"read_file", "write_file"},
		Deny:  []string{"exec_command"},
	}

	// Evaluate tools
	readResult := evaluator.Evaluate("read_file", policy, "test-agent")
	assert.True(t, readResult.Allowed)

	writeResult := evaluator.Evaluate("write_file", policy, "test-agent")
	assert.True(t, writeResult.Allowed)

	shellResult := evaluator.Evaluate("exec_command", policy, "test-agent")
	assert.False(t, shellResult.Allowed)
	assert.Equal(t, "deny_list", shellResult.ViolationType)

	// Verify category matching
	assert.True(t, matcher.MatchesCategory("read_file", CategoryRead))
	assert.True(t, matcher.MatchesCategory("write_file", CategoryWrite))
	assert.True(t, matcher.MatchesCategory("exec_command", CategoryShell))
}

// Integration test: Filter tools by category and apply policy
func TestFilterAndPolicy_Integration(t *testing.T) {
	registry := NewToolRegistry()
	registry.Register("read_file", "Read file", CategoryRead)
	registry.Register("read_dir", "Read directory", CategoryRead)
	registry.Register("write_file", "Write file", CategoryWrite)
	registry.Register("delete_file", "Delete file", CategoryWrite)
	registry.Register("exec_command", "Execute command", CategoryShell)

	// Filter by read category
	readTools := registry.FilterByCategory(CategoryRead)
	assert.Len(t, readTools, 2)

	// Filter by write category
	writeTools := registry.FilterByCategory(CategoryWrite)
	assert.Len(t, writeTools, 2)

	// Filter by multiple categories
	readWriteTools := registry.FilterByCategories([]ToolCategory{CategoryRead, CategoryWrite})
	assert.Len(t, readWriteTools, 4)

	// Apply category policy
	matcher := NewCategoryMatcher(registry)

	// Allow only read tools
	assert.True(t, matcher.ApplyCategoryPolicy("read_file", []ToolCategory{CategoryRead}, []ToolCategory{}))
	assert.True(t, matcher.ApplyCategoryPolicy("read_dir", []ToolCategory{CategoryRead}, []ToolCategory{}))
	assert.False(t, matcher.ApplyCategoryPolicy("write_file", []ToolCategory{CategoryRead}, []ToolCategory{}))
	assert.False(t, matcher.ApplyCategoryPolicy("exec_command", []ToolCategory{CategoryRead}, []ToolCategory{}))

	// Deny shell tools
	assert.True(t, matcher.ApplyCategoryPolicy("read_file", []ToolCategory{}, []ToolCategory{CategoryShell}))
	assert.True(t, matcher.ApplyCategoryPolicy("write_file", []ToolCategory{}, []ToolCategory{CategoryShell}))
	assert.False(t, matcher.ApplyCategoryPolicy("exec_command", []ToolCategory{}, []ToolCategory{CategoryShell}))
}
