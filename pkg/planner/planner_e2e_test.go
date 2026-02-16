//go:build e2e
// +build e2e

package planner

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestPlannerE2E(t *testing.T) {
	t.Run("complete plan-execute-reflect-retry workflow", func(t *testing.T) {
		p := NewPlanner()
		e := NewExecutor(p)
		r := NewReflector(p)

		// Create a realistic deployment plan
		steps := []Step{
			{
				ID:            "validate",
				Description:   "Validate configuration",
				EstimatedTime: 2 * time.Second,
			},
			{
				ID:            "build",
				Description:   "Build application",
				Dependencies:  []string{"validate"},
				EstimatedTime: 10 * time.Second,
			},
			{
				ID:            "test",
				Description:   "Run tests",
				Dependencies:  []string{"build"},
				EstimatedTime: 15 * time.Second,
			},
			{
				ID:            "deploy-staging",
				Description:   "Deploy to staging",
				Dependencies:  []string{"test"},
				EstimatedTime: 5 * time.Second,
			},
			{
				ID:            "smoke-test",
				Description:   "Run smoke tests",
				Dependencies:  []string{"deploy-staging"},
				EstimatedTime: 3 * time.Second,
			},
			{
				ID:            "deploy-prod",
				Description:   "Deploy to production",
				Dependencies:  []string{"smoke-test"},
				EstimatedTime: 5 * time.Second,
			},
		}

		plan, err := p.GeneratePlan("Production deployment", steps)
		if err != nil {
			t.Fatalf("GeneratePlan failed: %v", err)
		}

		t.Logf("Created plan: %s with %d steps", plan.Description, len(plan.Steps))

		// Configure executor
		e.SetMaxRetries(2)
		e.SetFailureStrategy(FailRetry)

		// First attempt - smoke test will fail
		attempts := make(map[string]int)
		executor := func(ctx context.Context, step *Step) error {
			attempts[step.ID]++

			// Simulate work
			time.Sleep(50 * time.Millisecond)

			// Smoke test fails on first attempt (flaky test)
			if step.ID == "smoke-test" && attempts[step.ID] == 1 {
				return errors.New("connection timeout during smoke test")
			}

			return nil
		}

		// Execute plan
		startTime := time.Now()
		err = e.Execute(context.Background(), plan, executor)
		duration := time.Since(startTime)

		if err != nil {
			t.Fatalf("Execute failed: %v", err)
		}

		t.Logf("Plan executed in %v", duration)

		// Verify all steps completed
		for _, step := range plan.Steps {
			if step.Status != StepStatusCompleted {
				t.Errorf("Step %s not completed: %s", step.ID, step.Status)
			}
		}

		// Verify smoke test was retried
		if attempts["smoke-test"] != 2 {
			t.Errorf("Expected smoke-test to be attempted 2 times, got %d", attempts["smoke-test"])
		}

		// Reflect on execution
		reflection, err := r.Reflect(context.Background(), plan)
		if err != nil {
			t.Fatalf("Reflect failed: %v", err)
		}

		t.Logf("Reflection: %d issues, %d suggestions", len(reflection.Issues), len(reflection.Suggestions))

		// Should detect flaky smoke test
		if len(reflection.Issues) < 1 {
			t.Error("Expected at least 1 issue for flaky test")
		}

		// Verify execution order was correct
		levels, err := p.GetExecutionOrder(plan)
		if err != nil {
			t.Fatalf("GetExecutionOrder failed: %v", err)
		}

		t.Logf("Execution levels: %d", len(levels))
		for i, level := range levels {
			t.Logf("  Level %d: %v", i, level)
		}

		// Verify final state
		for _, step := range plan.Steps {
			if step.Result == nil {
				t.Errorf("Step %s has no result", step.ID)
				continue
			}

			if !step.Result.Success {
				t.Errorf("Step %s failed: %s", step.ID, step.Result.Error)
			}

			t.Logf("Step %s: %s (attempts: %d, duration: %v)",
				step.ID, step.Status, step.Attempts, step.Result.Duration)
		}
	})
}
