package orchestrator

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// RealisticAgentRunner simulates realistic agent behavior with proper responses
type RealisticAgentRunner struct {
	callCount map[string]int
}

func NewRealisticAgentRunner() *RealisticAgentRunner {
	return &RealisticAgentRunner{
		callCount: make(map[string]int),
	}
}

func (r *RealisticAgentRunner) Run(ctx context.Context, params AgentRunParams) (AgentResult, error) {
	// Simulate realistic processing time
	time.Sleep(50 * time.Millisecond)

	// Track calls
	key := string(params.Config.Role)
	r.callCount[key]++

	// Check for context cancellation
	select {
	case <-ctx.Done():
		return AgentResult{
			Success: false,
		}, ctx.Err()
	default:
	}

	// Generate realistic responses based on role and stage
	var output string
	switch params.Config.Role {
	case RoleCaptain:
		if params.Context.SharedState["execution"] != nil {
			// Synthesis stage
			output = r.generateSynthesis(params)
		} else {
			// Planning stage
			output = r.generatePlan(params)
		}
	case RoleExecutor:
		output = r.generateExecution(params)
	case RoleCritic:
		output = r.generateReview(params)
	default:
		output = "Unknown role"
	}

	return AgentResult{
		AgentID:  params.Config.ID,
		Success:  true,
		Output:   output,
		Duration: 50,
		Metadata: map[string]interface{}{
			"role":       string(params.Config.Role),
			"call_count": r.callCount[key],
		},
	}, nil
}

func (r *RealisticAgentRunner) generatePlan(params AgentRunParams) string {
	task := params.Prompt
	return fmt.Sprintf(`Plan for task:
1. Analyze the requirements
2. Design the solution
3. Implement the code
4. Test the implementation
5. Document the results

Task context: %s`, task)
}

func (r *RealisticAgentRunner) generateExecution(params AgentRunParams) string {
	plan := params.Context.SharedState["plan"]
	return fmt.Sprintf(`Execution completed:
- Analyzed requirements ✓
- Designed solution ✓
- Implemented code ✓
- Tested implementation ✓
- Documented results ✓

Based on plan: %v`, plan)
}

func (r *RealisticAgentRunner) generateReview(params AgentRunParams) string {
	execution := params.Context.SharedState["execution"]
	return fmt.Sprintf(`Review findings:
✓ Correctness: Implementation matches requirements
✓ Quality: Code is well-structured
✓ Completeness: All steps completed
✓ Issues: No critical issues found

Minor suggestions:
- Consider adding more error handling
- Could improve documentation

Execution reviewed: %v`, execution)
}

func (r *RealisticAgentRunner) generateSynthesis(params AgentRunParams) string {
	execution := params.Context.SharedState["execution"]
	review := params.Context.SharedState["review"]
	return fmt.Sprintf(`Final Result:
Task completed successfully with high quality.

Summary:
- All planned steps executed
- Critic review passed with minor suggestions
- Ready for deployment

Execution: %v
Review: %v`, execution, review)
}

// E2E Test: Complete Captain's Chair workflow with realistic agents
func TestCaptainsChair_E2E_CompleteWorkflow(t *testing.T) {
	// Setup
	runner := NewRealisticAgentRunner()

	logger := &MockLogger{}
	logger.On("Info", mock.Anything, mock.Anything).Return()
	logger.On("Error", mock.Anything, mock.Anything, mock.Anything).Return()
	logger.On("Debug", mock.Anything, mock.Anything).Return()

	orchestrator := New(WithLogger(logger), WithMaxConcurrent(10))
	spawner := NewSpawner(orchestrator, runner, logger)
	chair := NewCaptainsChair(orchestrator, spawner, logger)

	// Register realistic agent configurations
	_ = orchestrator.RegisterAgent(AgentConfig{
		ID:                     "production-captain",
		Name:                   "Production Captain",
		Role:                   RoleCaptain,
		Model:                  "claude-opus-4",
		Temperature:            0.7,
		MaxTokens:              4096,
		SystemPrompt:           "You are a captain agent. Coordinate but never execute directly.",
		MaxConcurrentSubAgents: 5,
		Tools: ToolPolicy{
			Allow: []string{"sessions_spawn", "sessions_send", "memory_search"},
		},
	})

	_ = orchestrator.RegisterAgent(AgentConfig{
		ID:           "production-executor",
		Name:         "Production Executor",
		Role:         RoleExecutor,
		Model:        "claude-sonnet-4",
		Temperature:  0.7,
		MaxTokens:    4096,
		SystemPrompt: "You are an executor agent. Execute assigned tasks precisely.",
		Tools: ToolPolicy{
			Allow: []string{"*"},
		},
		Sandbox: SandboxConfig{
			Mode:  SandboxModeAll,
			Scope: SandboxScopeAgent,
		},
	})

	_ = orchestrator.RegisterAgent(AgentConfig{
		ID:           "production-critic",
		Name:         "Production Critic",
		Role:         RoleCritic,
		Model:        "claude-sonnet-4",
		Temperature:  0.7,
		MaxTokens:    4096,
		SystemPrompt: "You are a critic agent. Review work and find issues.",
		Tools: ToolPolicy{
			Allow: []string{"read"},
		},
		Sandbox: SandboxConfig{
			Mode: SandboxModeOff,
		},
	})

	// Execute a realistic task
	req := CaptainsChairRequest{
		CaptainID:  "production-captain",
		ExecutorID: "production-executor",
		CriticID:   "production-critic",
		Task:       "Create a REST API endpoint for user authentication",
		Context: AgentContext{
			ParentSessionKey: "e2e-test-session",
			SharedState: map[string]interface{}{
				"project":  "user-service",
				"language": "Go",
			},
			Files:   []string{"main.go", "auth.go"},
			TraceID: "e2e-trace-123",
		},
		Timeout: 60, // 1 minute timeout
	}

	startTime := time.Now()
	result, err := chair.Execute(context.Background(), req)
	duration := time.Since(startTime)

	// Assertions
	assert.NoError(t, err)
	assert.True(t, result.Success)
	assert.Empty(t, result.Error)

	// Verify all stages completed successfully
	assert.True(t, result.PlanResult.Success)
	assert.Contains(t, result.PlanResult.Output, "Plan for task")
	assert.Contains(t, result.PlanResult.Output, "Analyze the requirements")

	assert.True(t, result.ExecutorResult.Success)
	assert.Contains(t, result.ExecutorResult.Output, "Execution completed")
	assert.Contains(t, result.ExecutorResult.Output, "✓")

	assert.True(t, result.CriticResult.Success)
	assert.Contains(t, result.CriticResult.Output, "Review findings")
	assert.Contains(t, result.CriticResult.Output, "Correctness")

	assert.True(t, result.FinalResult.Success)
	assert.Contains(t, result.FinalResult.Output, "Final Result")
	assert.Contains(t, result.FinalResult.Output, "completed successfully")

	// Verify timing
	assert.Greater(t, result.TotalDuration, int64(0))
	assert.Greater(t, result.TotalDuration, int64(200)) // At least 4 stages * 50ms
	assert.Less(t, duration, 5*time.Second)             // Should complete quickly in test

	// Verify each agent was called exactly once (captain twice: plan + synthesis)
	assert.Equal(t, 2, runner.callCount[string(RoleCaptain)])
	assert.Equal(t, 1, runner.callCount[string(RoleExecutor)])
	assert.Equal(t, 1, runner.callCount[string(RoleCritic)])

	// Verify metadata
	assert.NotNil(t, result.PlanResult.Metadata)
	assert.NotNil(t, result.ExecutorResult.Metadata)
	assert.NotNil(t, result.CriticResult.Metadata)
	assert.NotNil(t, result.FinalResult.Metadata)

	// Verify context propagation
	// The shared state should have been passed through all stages
	assert.NotEmpty(t, result.PlanResult.AgentID)
	assert.NotEmpty(t, result.ExecutorResult.AgentID)
	assert.NotEmpty(t, result.CriticResult.AgentID)
	assert.NotEmpty(t, result.FinalResult.AgentID)

	// Log results for manual inspection
	t.Logf("E2E Test Results:")
	t.Logf("  Total Duration: %dms", result.TotalDuration)
	t.Logf("  Plan Output: %s", truncate(result.PlanResult.Output.(string), 100))
	t.Logf("  Executor Output: %s", truncate(result.ExecutorResult.Output.(string), 100))
	t.Logf("  Critic Output: %s", truncate(result.CriticResult.Output.(string), 100))
	t.Logf("  Final Output: %s", truncate(result.FinalResult.Output.(string), 100))
}

// Helper function to truncate long strings for logging
func truncate(s string, maxLen int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
