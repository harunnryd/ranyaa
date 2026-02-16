//go:build integration
// +build integration

package planner

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestReflectorIntegration(t *testing.T) {
	p := NewPlanner()
	e := NewExecutor(p)
	r := NewReflector(p)

	t.Run("plan-execute-reflect cycle", func(t *testing.T) {
		// Create a plan with some steps that will fail
		steps := []Step{
			{ID: "step-1", Description: "Successful step"},
			{ID: "step-2", Description: "Flaky step", Dependencies: []string{"step-1"}},
			{ID: "step-3", Description: "Failed step", Dependencies: []string{"step-2"}},
		}

		plan, _ := p.GeneratePlan("Test plan", steps)

		// First execution - step-2 will fail, step-3 won't execute
		attempt := 0
		executor := func(ctx context.Context, step *Step) error {
			attempt++
			if step.ID == "step-2" {
				return errors.New("network timeout")
			}
			if step.ID == "step-3" {
				return errors.New("permission denied")
			}
			return nil
		}

		e.SetFailureStrategy(FailContinue)
		err := e.Execute(context.Background(), plan, executor)
		if err != nil {
			t.Fatalf("Execute failed: %v", err)
		}

		// Reflect on results
		reflection, err := r.Reflect(context.Background(), plan)
		if err != nil {
			t.Fatalf("Reflect failed: %v", err)
		}

		// Verify reflection results - only step-2 failed
		if len(reflection.Issues) < 1 {
			t.Errorf("Expected at least 1 issue, got %d", len(reflection.Issues))
		}

		if len(reflection.Suggestions) < 1 {
			t.Errorf("Expected at least 1 suggestion, got %d", len(reflection.Suggestions))
		}

		// Check for retry suggestion on network timeout
		retrySuggestions := r.GetSuggestionsByAction(reflection, "retry")
		if len(retrySuggestions) < 1 {
			t.Errorf("Expected at least 1 retry suggestion, got %d", len(retrySuggestions))
		}

		t.Logf("Reflection: %d issues, %d suggestions", len(reflection.Issues), len(reflection.Suggestions))
		t.Logf("Should retry: %v", reflection.ShouldRetry)
	})

	t.Run("reflect and retry with adjustments", func(t *testing.T) {
		steps := []Step{
			{ID: "flaky-step", Description: "Flaky network step"},
		}

		plan, _ := p.GeneratePlan("Test plan", steps)

		// First execution - will fail
		attempts := 0
		executor := func(ctx context.Context, step *Step) error {
			attempts++
			if attempts == 1 {
				return errors.New("connection timeout")
			}
			return nil
		}

		e.SetFailureStrategy(FailAbort)
		err := e.Execute(context.Background(), plan, executor)
		if err == nil {
			t.Fatal("Expected first execution to fail")
		}

		// Reflect
		reflection, err := r.Reflect(context.Background(), plan)
		if err != nil {
			t.Fatalf("Reflect failed: %v", err)
		}

		if !reflection.ShouldRetry {
			t.Error("Expected reflection to recommend retry")
		}

		// Apply adjustments
		reflection.Adjustments["flaky-step"] = "increase timeout"
		err = r.ApplyAdjustments(plan, reflection)
		if err != nil {
			t.Fatalf("ApplyAdjustments failed: %v", err)
		}

		// Retry execution
		err = e.Execute(context.Background(), plan, executor)
		if err != nil {
			t.Fatalf("Retry execution failed: %v", err)
		}

		// Verify success
		if plan.Steps[0].Status != StepStatusCompleted {
			t.Errorf("Expected status 'completed', got '%s'", plan.Steps[0].Status)
		}

		t.Logf("Plan completed after %d attempts", attempts)
	})

	t.Run("reflect on performance issues", func(t *testing.T) {
		steps := []Step{
			{
				ID:            "slow-step",
				Description:   "Slow step",
				EstimatedTime: 100 * time.Millisecond,
			},
		}

		plan, _ := p.GeneratePlan("Test plan", steps)

		// Execute with slow step
		executor := func(ctx context.Context, step *Step) error {
			time.Sleep(300 * time.Millisecond) // 3x slower than estimated
			return nil
		}

		err := e.Execute(context.Background(), plan, executor)
		if err != nil {
			t.Fatalf("Execute failed: %v", err)
		}

		// Reflect
		reflection, err := r.Reflect(context.Background(), plan)
		if err != nil {
			t.Fatalf("Reflect failed: %v", err)
		}

		// Should detect performance issue
		if len(reflection.Issues) != 1 {
			t.Errorf("Expected 1 performance issue, got %d", len(reflection.Issues))
		}

		if reflection.Issues[0].Severity != IssueSeverityLow {
			t.Errorf("Expected low severity for performance issue, got %s", reflection.Issues[0].Severity)
		}

		t.Logf("Detected performance issue: %s", reflection.Issues[0].Description)
	})
}
