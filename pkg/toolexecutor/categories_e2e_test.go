package toolexecutor

import (
	"context"
	"testing"

	"github.com/harun/ranya/pkg/sandbox"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// E2E test: Complete workflow with categories, policies, and execution
func TestE2E_CategoryBasedPolicyWorkflow(t *testing.T) {
	// Setup: Create executor and registry
	executor := New()
	registry := NewToolRegistry()
	policyEngine := NewPolicyEngine()
	evaluator := NewPolicyEvaluator(policyEngine)
	matcher := NewCategoryMatcher(registry)

	approvalManager := NewApprovalManager(&MockApprovalHandler{AutoApprove: true})
	executor.SetApprovalManager(approvalManager)

	sandboxCfg := sandbox.DefaultConfig()
	sandboxManager := NewSandboxManager(sandboxCfg)
	t.Cleanup(func() {
		_ = sandboxManager.StopAll(context.Background())
	})
	executor.SetSandboxManager(sandboxManager)

	// Step 1: Register tools with categories
	tools := []struct {
		name        string
		description string
		category    ToolCategory
		handler     ToolHandler
	}{
		{
			name:        "read_file",
			description: "Read a file from disk",
			category:    CategoryRead,
			handler: func(ctx context.Context, params map[string]interface{}) (interface{}, error) {
				return "file content", nil
			},
		},
		{
			name:        "write_file",
			description: "Write a file to disk",
			category:    CategoryWrite,
			handler: func(ctx context.Context, params map[string]interface{}) (interface{}, error) {
				return "file written successfully", nil
			},
		},
		{
			name:        "exec_command",
			description: "Execute a shell command",
			category:    CategoryShell,
			handler: func(ctx context.Context, params map[string]interface{}) (interface{}, error) {
				return "command executed", nil
			},
		},
		{
			name:        "http_get",
			description: "Make an HTTP GET request",
			category:    CategoryWeb,
			handler: func(ctx context.Context, params map[string]interface{}) (interface{}, error) {
				return "HTTP response", nil
			},
		},
	}

	for _, tool := range tools {
		// Register in registry
		require.NoError(t, registry.Register(tool.name, tool.description, tool.category))

		// Register in executor
		require.NoError(t, executor.RegisterTool(ToolDefinition{
			Name:        tool.name,
			Description: tool.description,
			Parameters:  []ToolParameter{},
			Handler:     tool.handler,
		}))
	}

	// Step 2: Verify tools are registered
	assert.Equal(t, 4, executor.GetToolCount())
	assert.Len(t, registry.List(), 4)

	// Step 3: Filter tools by category
	readTools := registry.FilterByCategory(CategoryRead)
	assert.Len(t, readTools, 1)
	assert.Equal(t, "read_file", readTools[0].Name)

	writeTools := registry.FilterByCategory(CategoryWrite)
	assert.Len(t, writeTools, 1)
	assert.Equal(t, "write_file", writeTools[0].Name)

	shellTools := registry.FilterByCategory(CategoryShell)
	assert.Len(t, shellTools, 1)
	assert.Equal(t, "exec_command", shellTools[0].Name)

	webTools := registry.FilterByCategory(CategoryWeb)
	assert.Len(t, webTools, 1)
	assert.Equal(t, "http_get", webTools[0].Name)

	// Step 4: Create policies for different agent roles
	captainPolicy := &ToolPolicy{
		Allow: []string{"read_file", "http_get"}, // Captain can read and search
		Deny:  []string{"exec_command"},          // But cannot execute commands
	}

	executorPolicy := &ToolPolicy{
		Allow: []string{"*"},            // Executor can use all tools
		Deny:  []string{"exec_command"}, // Except shell commands (for safety)
	}

	criticPolicy := &ToolPolicy{
		Allow: []string{"read_file"}, // Critic can only read
		Deny:  []string{},
	}

	// Step 5: Test captain agent
	t.Run("captain_agent", func(t *testing.T) {
		execCtx := &ExecutionContext{
			AgentID:    "captain",
			ToolPolicy: captainPolicy,
		}

		// Captain can read
		result := executor.Execute(context.Background(), "read_file", map[string]interface{}{}, execCtx)
		assert.True(t, result.Success)
		assert.Equal(t, "file content", result.Output)

		// Captain can use web tools
		result = executor.Execute(context.Background(), "http_get", map[string]interface{}{}, execCtx)
		assert.True(t, result.Success)
		assert.Equal(t, "HTTP response", result.Output)

		// Captain cannot write (not in allow list)
		result = executor.Execute(context.Background(), "write_file", map[string]interface{}{}, execCtx)
		assert.False(t, result.Success)
		assert.Contains(t, result.Error, "not allowed by agent policy")

		// Captain cannot execute commands (in deny list)
		result = executor.Execute(context.Background(), "exec_command", map[string]interface{}{}, execCtx)
		assert.False(t, result.Success)
		assert.Contains(t, result.Error, "not allowed by agent policy")
	})

	// Step 6: Test executor agent
	t.Run("executor_agent", func(t *testing.T) {
		execCtx := &ExecutionContext{
			AgentID:    "executor",
			ToolPolicy: executorPolicy,
		}

		// Executor can read
		result := executor.Execute(context.Background(), "read_file", map[string]interface{}{}, execCtx)
		assert.True(t, result.Success)

		// Executor can write
		result = executor.Execute(context.Background(), "write_file", map[string]interface{}{}, execCtx)
		assert.True(t, result.Success)
		assert.Equal(t, "file written successfully", result.Output)

		// Executor can use web tools
		result = executor.Execute(context.Background(), "http_get", map[string]interface{}{}, execCtx)
		assert.True(t, result.Success)

		// Executor cannot execute commands (denied for safety)
		result = executor.Execute(context.Background(), "exec_command", map[string]interface{}{}, execCtx)
		assert.False(t, result.Success)
		assert.Contains(t, result.Error, "not allowed by agent policy")
	})

	// Step 7: Test critic agent
	t.Run("critic_agent", func(t *testing.T) {
		execCtx := &ExecutionContext{
			AgentID:    "critic",
			ToolPolicy: criticPolicy,
		}

		// Critic can read
		result := executor.Execute(context.Background(), "read_file", map[string]interface{}{}, execCtx)
		assert.True(t, result.Success)

		// Critic cannot write
		result = executor.Execute(context.Background(), "write_file", map[string]interface{}{}, execCtx)
		assert.False(t, result.Success)

		// Critic cannot use web tools
		result = executor.Execute(context.Background(), "http_get", map[string]interface{}{}, execCtx)
		assert.False(t, result.Success)

		// Critic cannot execute commands
		result = executor.Execute(context.Background(), "exec_command", map[string]interface{}{}, execCtx)
		assert.False(t, result.Success)
	})

	// Step 8: Test category-based policy evaluation
	t.Run("category_policy_evaluation", func(t *testing.T) {
		// Evaluate all tools against captain policy
		readResult := evaluator.Evaluate("read_file", captainPolicy, "captain")
		assert.True(t, readResult.Allowed)

		writeResult := evaluator.Evaluate("write_file", captainPolicy, "captain")
		assert.False(t, writeResult.Allowed)
		assert.Equal(t, "not_in_allow_list", writeResult.ViolationType)

		shellResult := evaluator.Evaluate("exec_command", captainPolicy, "captain")
		assert.False(t, shellResult.Allowed)
		assert.Equal(t, "deny_list", shellResult.ViolationType)

		// Test category matching
		assert.True(t, matcher.MatchesCategory("read_file", CategoryRead))
		assert.True(t, matcher.MatchesCategory("write_file", CategoryWrite))
		assert.True(t, matcher.MatchesCategory("exec_command", CategoryShell))
		assert.True(t, matcher.MatchesCategory("http_get", CategoryWeb))

		// Test category-based policy
		assert.True(t, matcher.ApplyCategoryPolicy("read_file", []ToolCategory{CategoryRead}, []ToolCategory{}))
		assert.False(t, matcher.ApplyCategoryPolicy("write_file", []ToolCategory{CategoryRead}, []ToolCategory{}))
		assert.False(t, matcher.ApplyCategoryPolicy("exec_command", []ToolCategory{}, []ToolCategory{CategoryShell}))
	})

	// Step 9: Test filtering tools by multiple categories
	t.Run("filter_by_multiple_categories", func(t *testing.T) {
		// Get all read and write tools
		readWriteTools := registry.FilterByCategories([]ToolCategory{CategoryRead, CategoryWrite})
		assert.Len(t, readWriteTools, 2)

		names := []string{readWriteTools[0].Name, readWriteTools[1].Name}
		assert.Contains(t, names, "read_file")
		assert.Contains(t, names, "write_file")

		// Get all shell and web tools
		shellWebTools := registry.FilterByCategories([]ToolCategory{CategoryShell, CategoryWeb})
		assert.Len(t, shellWebTools, 2)

		names = []string{shellWebTools[0].Name, shellWebTools[1].Name}
		assert.Contains(t, names, "exec_command")
		assert.Contains(t, names, "http_get")
	})
}
