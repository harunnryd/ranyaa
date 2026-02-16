package planner

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

func TestNewExecutor(t *testing.T) {
	p := NewPlanner()
	e := NewExecutor(p)
	if e == nil {
		t.Fatal("NewExecutor returned nil")
	}
	if e.maxRetries != 3 {
		t.Errorf("Expected maxRetries=3, got %d", e.maxRetries)
	}
}

func TestExecutorConfiguration(t *testing.T) {
	p := NewPlanner()
	e := NewExecutor(p)

	t.Run("set max retries", func(t *testing.T) {
		e.SetMaxRetries(5)
		if e.maxRetries != 5 {
			t.Errorf("Expected maxRetries=5, got %d", e.maxRetries)
		}
	})

	t.Run("set failure strategy", func(t *testing.T) {
		e.SetFailureStrategy(FailContinue)
		if e.failureStrategy != FailContinue {
			t.Errorf("Expected failureStrategy=continue, got %s", e.failureStrategy)
		}
	})

	t.Run("set join strategy", func(t *testing.T) {
		e.SetJoinStrategy(JoinFirst)
		if e.joinStrategy != JoinFirst {
			t.Errorf("Expected joinStrategy=first, got %s", e.joinStrategy)
		}
	})
}

func TestExecuteSimplePlan(t *testing.T) {
	p := NewPlanner()
	e := NewExecutor(p)

	t.Run("execute single step", func(t *testing.T) {
		steps := []Step{
			{ID: "step-1", Description: "Step 1"},
		}

		plan, _ := p.GeneratePlan("Test plan", steps)

		executed := false
		executor := func(ctx context.Context, step *Step) error {
			executed = true
			return nil
		}

		err := e.Execute(context.Background(), plan, executor)
		if err != nil {
			t.Fatalf("Execute failed: %v", err)
		}

		if !executed {
			t.Error("Step was not executed")
		}

		if plan.Steps[0].Status != StepStatusCompleted {
			t.Errorf("Expected status 'completed', got '%s'", plan.Steps[0].Status)
		}
	})

	t.Run("execute multiple steps", func(t *testing.T) {
		steps := []Step{
			{ID: "step-1", Description: "Step 1"},
			{ID: "step-2", Description: "Step 2", Dependencies: []string{"step-1"}},
		}

		plan, _ := p.GeneratePlan("Test plan", steps)

		executionOrder := []string{}
		executor := func(ctx context.Context, step *Step) error {
			executionOrder = append(executionOrder, step.ID)
			return nil
		}

		err := e.Execute(context.Background(), plan, executor)
		if err != nil {
			t.Fatalf("Execute failed: %v", err)
		}

		if len(executionOrder) != 2 {
			t.Errorf("Expected 2 steps executed, got %d", len(executionOrder))
		}

		if executionOrder[0] != "step-1" || executionOrder[1] != "step-2" {
			t.Errorf("Unexpected execution order: %v", executionOrder)
		}
	})
}

func TestExecuteWithFailure(t *testing.T) {
	p := NewPlanner()
	e := NewExecutor(p)

	t.Run("abort on failure", func(t *testing.T) {
		steps := []Step{
			{ID: "step-1", Description: "Step 1"},
			{ID: "step-2", Description: "Step 2", Dependencies: []string{"step-1"}},
		}

		plan, _ := p.GeneratePlan("Test plan", steps)

		e.SetFailureStrategy(FailAbort)
		executor := func(ctx context.Context, step *Step) error {
			if step.ID == "step-1" {
				return errors.New("step failed")
			}
			return nil
		}

		err := e.Execute(context.Background(), plan, executor)
		if err == nil {
			t.Error("Expected error for failed step")
		}

		if plan.Steps[0].Status != StepStatusFailed {
			t.Errorf("Expected status 'failed', got '%s'", plan.Steps[0].Status)
		}
	})

	t.Run("continue on failure", func(t *testing.T) {
		steps := []Step{
			{ID: "step-1", Description: "Step 1"},
			{ID: "step-2", Description: "Step 2"},
		}

		plan, _ := p.GeneratePlan("Test plan", steps)

		e.SetFailureStrategy(FailContinue)
		failedSteps := []string{}
		executor := func(ctx context.Context, step *Step) error {
			if step.ID == "step-1" {
				failedSteps = append(failedSteps, step.ID)
				return errors.New("step failed")
			}
			return nil
		}

		err := e.Execute(context.Background(), plan, executor)
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}

		if len(failedSteps) != 1 {
			t.Errorf("Expected 1 failed step, got %d", len(failedSteps))
		}
	})
}

func TestExecuteWithRetry(t *testing.T) {
	p := NewPlanner()
	e := NewExecutor(p)

	t.Run("retry failed step", func(t *testing.T) {
		steps := []Step{
			{ID: "step-1", Description: "Step 1"},
		}

		plan, _ := p.GeneratePlan("Test plan", steps)

		e.SetMaxRetries(2)
		e.SetFailureStrategy(FailRetry)

		attempts := 0
		executor := func(ctx context.Context, step *Step) error {
			attempts++
			if attempts < 3 {
				return errors.New("step failed")
			}
			return nil
		}

		err := e.Execute(context.Background(), plan, executor)
		if err != nil {
			t.Fatalf("Execute failed: %v", err)
		}

		if attempts != 3 {
			t.Errorf("Expected 3 attempts, got %d", attempts)
		}

		if plan.Steps[0].Status != StepStatusCompleted {
			t.Errorf("Expected status 'completed', got '%s'", plan.Steps[0].Status)
		}
	})

	t.Run("max retries exceeded", func(t *testing.T) {
		steps := []Step{
			{ID: "step-1", Description: "Step 1"},
		}

		plan, _ := p.GeneratePlan("Test plan", steps)

		e.SetMaxRetries(2)
		e.SetFailureStrategy(FailRetry)

		attempts := 0
		executor := func(ctx context.Context, step *Step) error {
			attempts++
			return errors.New("step failed")
		}

		err := e.Execute(context.Background(), plan, executor)
		if err == nil {
			t.Error("Expected error after max retries")
		}

		if attempts != 3 {
			t.Errorf("Expected 3 attempts, got %d", attempts)
		}

		if plan.Steps[0].Status != StepStatusFailed {
			t.Errorf("Expected status 'failed', got '%s'", plan.Steps[0].Status)
		}
	})
}

func TestExecuteParallel(t *testing.T) {
	p := NewPlanner()
	e := NewExecutor(p)

	t.Run("parallel execution", func(t *testing.T) {
		steps := []Step{
			{ID: "step-1", Description: "Step 1"},
			{ID: "step-2", Description: "Step 2"},
		}

		plan, _ := p.GeneratePlan("Test plan", steps)

		executedSteps := make(map[string]bool)
		var mu sync.Mutex

		executor := func(ctx context.Context, step *Step) error {
			time.Sleep(10 * time.Millisecond)
			mu.Lock()
			executedSteps[step.ID] = true
			mu.Unlock()
			return nil
		}

		err := e.Execute(context.Background(), plan, executor)
		if err != nil {
			t.Fatalf("Execute failed: %v", err)
		}

		if len(executedSteps) != 2 {
			t.Errorf("Expected 2 steps executed, got %d", len(executedSteps))
		}
	})
}

func TestGetStepStatus(t *testing.T) {
	p := NewPlanner()
	e := NewExecutor(p)

	steps := []Step{
		{ID: "step-1", Description: "Step 1"},
	}

	plan, _ := p.GeneratePlan("Test plan", steps)

	t.Run("get pending status", func(t *testing.T) {
		status, err := e.GetStepStatus(plan, "step-1")
		if err != nil {
			t.Fatalf("GetStepStatus failed: %v", err)
		}

		if status != StepStatusPending {
			t.Errorf("Expected status 'pending', got '%s'", status)
		}
	})

	t.Run("get non-existent step", func(t *testing.T) {
		_, err := e.GetStepStatus(plan, "step-999")
		if err == nil {
			t.Error("Expected error for non-existent step")
		}
	})
}

func TestGetStepResult(t *testing.T) {
	p := NewPlanner()
	e := NewExecutor(p)

	steps := []Step{
		{ID: "step-1", Description: "Step 1"},
	}

	plan, _ := p.GeneratePlan("Test plan", steps)

	executor := func(ctx context.Context, step *Step) error {
		step.Result = &StepResult{
			Success: true,
			Output:  "test output",
		}
		return nil
	}

	e.Execute(context.Background(), plan, executor)

	t.Run("get step result", func(t *testing.T) {
		result, err := e.GetStepResult(plan, "step-1")
		if err != nil {
			t.Fatalf("GetStepResult failed: %v", err)
		}

		if result == nil {
			t.Fatal("Result is nil")
		}

		if !result.Success {
			t.Error("Expected success=true")
		}
	})

	t.Run("get non-existent step result", func(t *testing.T) {
		_, err := e.GetStepResult(plan, "step-999")
		if err == nil {
			t.Error("Expected error for non-existent step")
		}
	})
}
