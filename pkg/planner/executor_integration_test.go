//go:build integration
// +build integration

package planner

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

func TestExecutorIntegration(t *testing.T) {
	p := NewPlanner()
	e := NewExecutor(p)

	t.Run("execute complex plan with dependencies", func(t *testing.T) {
		// Create a realistic build pipeline
		steps := []Step{
			{
				ID:            "checkout",
				Description:   "Checkout code",
				EstimatedTime: 2 * time.Second,
			},
			{
				ID:            "install-deps",
				Description:   "Install dependencies",
				Dependencies:  []string{"checkout"},
				EstimatedTime: 5 * time.Second,
			},
			{
				ID:            "lint",
				Description:   "Run linter",
				Dependencies:  []string{"install-deps"},
				EstimatedTime: 3 * time.Second,
			},
			{
				ID:            "test",
				Description:   "Run tests",
				Dependencies:  []string{"install-deps"},
				EstimatedTime: 10 * time.Second,
			},
			{
				ID:            "build",
				Description:   "Build application",
				Dependencies:  []string{"lint", "test"},
				EstimatedTime: 8 * time.Second,
			},
		}

		plan, err := p.GeneratePlan("Build pipeline", steps)
		if err != nil {
			t.Fatalf("GeneratePlan failed: %v", err)
		}

		// Track execution
		var mu sync.Mutex
		executionLog := []string{}

		executor := func(ctx context.Context, step *Step) error {
			// Simulate work
			time.Sleep(100 * time.Millisecond)

			mu.Lock()
			executionLog = append(executionLog, step.ID)
			mu.Unlock()

			return nil
		}

		startTime := time.Now()
		err = e.Execute(context.Background(), plan, executor)
		duration := time.Since(startTime)

		if err != nil {
			t.Fatalf("Execute failed: %v", err)
		}

		// Verify all steps executed
		if len(executionLog) != 5 {
			t.Errorf("Expected 5 steps executed, got %d", len(executionLog))
		}

		// Verify execution order
		checkoutIdx := indexOf(executionLog, "checkout")
		installIdx := indexOf(executionLog, "install-deps")
		lintIdx := indexOf(executionLog, "lint")
		testIdx := indexOf(executionLog, "test")
		buildIdx := indexOf(executionLog, "build")

		if checkoutIdx >= installIdx {
			t.Error("checkout should execute before install-deps")
		}
		if installIdx >= lintIdx || installIdx >= testIdx {
			t.Error("install-deps should execute before lint and test")
		}
		if lintIdx >= buildIdx || testIdx >= buildIdx {
			t.Error("lint and test should execute before build")
		}

		// Verify parallel execution (should be faster than sequential)
		// Sequential would take 500ms (5 steps * 100ms)
		// Parallel should take ~300ms (3 levels * 100ms)
		if duration > 450*time.Millisecond {
			t.Errorf("Execution took too long: %v (expected < 450ms)", duration)
		}

		t.Logf("Execution completed in %v", duration)
		t.Logf("Execution order: %v", executionLog)
	})

	t.Run("handle partial failures with continue strategy", func(t *testing.T) {
		steps := []Step{
			{ID: "step-1", Description: "Step 1"},
			{ID: "step-2", Description: "Step 2"},
			{ID: "step-3", Description: "Step 3"},
		}

		plan, _ := p.GeneratePlan("Test plan", steps)

		e.SetFailureStrategy(FailContinue)

		failedSteps := []string{}
		successSteps := []string{}

		executor := func(ctx context.Context, step *Step) error {
			if step.ID == "step-2" {
				failedSteps = append(failedSteps, step.ID)
				return errors.New("step 2 failed")
			}
			successSteps = append(successSteps, step.ID)
			return nil
		}

		err := e.Execute(context.Background(), plan, executor)
		if err != nil {
			t.Errorf("Unexpected error with continue strategy: %v", err)
		}

		if len(failedSteps) != 1 {
			t.Errorf("Expected 1 failed step, got %d", len(failedSteps))
		}

		if len(successSteps) != 2 {
			t.Errorf("Expected 2 successful steps, got %d", len(successSteps))
		}

		// Verify step statuses
		if plan.Steps[0].Status != StepStatusCompleted {
			t.Errorf("Step 1 should be completed, got %s", plan.Steps[0].Status)
		}
		if plan.Steps[1].Status != StepStatusFailed {
			t.Errorf("Step 2 should be failed, got %s", plan.Steps[1].Status)
		}
		if plan.Steps[2].Status != StepStatusCompleted {
			t.Errorf("Step 3 should be completed, got %s", plan.Steps[2].Status)
		}
	})

	t.Run("retry with exponential backoff", func(t *testing.T) {
		steps := []Step{
			{ID: "flaky-step", Description: "Flaky step"},
		}

		plan, _ := p.GeneratePlan("Test plan", steps)

		e.SetMaxRetries(3)
		e.SetFailureStrategy(FailRetry)

		attempts := 0
		attemptTimes := []time.Time{}

		executor := func(ctx context.Context, step *Step) error {
			attempts++
			attemptTimes = append(attemptTimes, time.Now())

			if attempts < 3 {
				return errors.New("temporary failure")
			}
			return nil
		}

		startTime := time.Now()
		err := e.Execute(context.Background(), plan, executor)
		duration := time.Since(startTime)

		if err != nil {
			t.Fatalf("Execute failed: %v", err)
		}

		if attempts != 3 {
			t.Errorf("Expected 3 attempts, got %d", attempts)
		}

		// Verify exponential backoff
		// First retry: 1s, Second retry: 2s
		// Total should be at least 3s
		if duration < 3*time.Second {
			t.Errorf("Expected at least 3s for retries, got %v", duration)
		}

		t.Logf("Retry completed in %v with %d attempts", duration, attempts)
	})

	t.Run("context cancellation", func(t *testing.T) {
		steps := []Step{
			{ID: "long-step", Description: "Long running step"},
		}

		plan, _ := p.GeneratePlan("Test plan", steps)

		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()

		executor := func(ctx context.Context, step *Step) error {
			// Simulate long-running work
			select {
			case <-time.After(1 * time.Second):
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		}

		err := e.Execute(ctx, plan, executor)
		if err == nil {
			t.Error("Expected context cancellation error")
		}

		if !errors.Is(err, context.DeadlineExceeded) {
			t.Errorf("Expected DeadlineExceeded, got %v", err)
		}
	})
}

func indexOf(slice []string, value string) int {
	for i, v := range slice {
		if v == value {
			return i
		}
	}
	return -1
}
