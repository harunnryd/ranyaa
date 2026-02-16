package planner

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// StepExecutor is a function that executes a single step
type StepExecutor func(ctx context.Context, step *Step) error

// Executor executes plans with dependency management and parallel execution
type Executor struct {
	planner         *Planner
	maxRetries      int
	failureStrategy FailureStrategy
	joinStrategy    JoinStrategy
}

// NewExecutor creates a new plan executor
func NewExecutor(planner *Planner) *Executor {
	return &Executor{
		planner:         planner,
		maxRetries:      3,
		failureStrategy: FailAbort,
		joinStrategy:    JoinAll,
	}
}

// SetMaxRetries sets the maximum number of retry attempts for failed steps
func (e *Executor) SetMaxRetries(maxRetries int) {
	e.maxRetries = maxRetries
}

// SetFailureStrategy sets the failure handling strategy
func (e *Executor) SetFailureStrategy(strategy FailureStrategy) {
	e.failureStrategy = strategy
}

// SetJoinStrategy sets the join strategy for parallel execution
func (e *Executor) SetJoinStrategy(strategy JoinStrategy) {
	e.joinStrategy = strategy
}

// Execute executes a plan with the given step executor function
func (e *Executor) Execute(ctx context.Context, plan *Plan, executor StepExecutor) error {
	// Get execution order (levels for parallel execution)
	levels, err := e.planner.GetExecutionOrder(plan)
	if err != nil {
		return fmt.Errorf("failed to get execution order: %w", err)
	}

	// Execute each level
	for levelIdx, level := range levels {
		if err := e.executeLevel(ctx, plan, level, executor, levelIdx); err != nil {
			return err
		}
	}

	return nil
}

// executeLevel executes all steps in a level (potentially in parallel)
func (e *Executor) executeLevel(ctx context.Context, plan *Plan, stepIDs []string, executor StepExecutor, levelIdx int) error {
	if len(stepIDs) == 1 {
		// Single step - execute directly
		step := e.findStep(plan, stepIDs[0])
		if step == nil {
			return fmt.Errorf("step not found: %s", stepIDs[0])
		}
		return e.executeStep(ctx, step, executor)
	}

	// Multiple steps - execute in parallel
	return e.executeParallel(ctx, plan, stepIDs, executor)
}

// executeParallel executes multiple steps in parallel
func (e *Executor) executeParallel(ctx context.Context, plan *Plan, stepIDs []string, executor StepExecutor) error {
	var wg sync.WaitGroup
	errChan := make(chan error, len(stepIDs))
	successChan := make(chan string, len(stepIDs))

	// Start all steps in parallel
	for _, stepID := range stepIDs {
		step := e.findStep(plan, stepID)
		if step == nil {
			return fmt.Errorf("step not found: %s", stepID)
		}

		wg.Add(1)
		go func(s *Step) {
			defer wg.Done()

			if err := e.executeStep(ctx, s, executor); err != nil {
				errChan <- fmt.Errorf("step %s failed: %w", s.ID, err)
			} else {
				successChan <- s.ID
			}
		}(step)
	}

	// Wait for completion based on join strategy
	switch e.joinStrategy {
	case JoinAll:
		// Wait for all steps to complete
		wg.Wait()
		close(errChan)
		close(successChan)

		// Check for errors
		var errors []error
		for err := range errChan {
			errors = append(errors, err)
		}

		if len(errors) > 0 {
			if e.failureStrategy == FailAbort {
				return errors[0]
			}
			// Continue strategy - log errors but don't fail
		}

	case JoinFirst:
		// Return when first step completes
		select {
		case <-successChan:
			return nil
		case err := <-errChan:
			if e.failureStrategy == FailAbort {
				return err
			}
		case <-ctx.Done():
			return ctx.Err()
		}

	case JoinAny:
		// Return when any step succeeds
		select {
		case <-successChan:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	return nil
}

// executeStep executes a single step with retry logic
func (e *Executor) executeStep(ctx context.Context, step *Step, executor StepExecutor) error {
	step.Status = StepStatusRunning
	startTime := time.Now()

	var lastErr error
	for attempt := 0; attempt <= e.maxRetries; attempt++ {
		step.Attempts = attempt + 1

		// Execute step
		err := executor(ctx, step)
		duration := time.Since(startTime)

		if err == nil {
			// Success
			step.Status = StepStatusCompleted
			step.Result = &StepResult{
				Success:   true,
				Duration:  duration,
				Timestamp: time.Now(),
			}
			return nil
		}

		lastErr = err

		// Check if we should retry
		if e.failureStrategy != FailRetry || attempt >= e.maxRetries {
			break
		}

		// Wait before retry (exponential backoff)
		backoff := time.Duration(attempt+1) * time.Second
		select {
		case <-time.After(backoff):
			// Continue to next attempt
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	// All attempts failed
	step.Status = StepStatusFailed
	step.Result = &StepResult{
		Success:   false,
		Error:     lastErr.Error(),
		Duration:  time.Since(startTime),
		Timestamp: time.Now(),
	}

	// Return error if abort or retry strategy (retry exhausted)
	if e.failureStrategy == FailAbort || e.failureStrategy == FailRetry {
		return lastErr
	}

	// Continue strategy - don't return error
	return nil
}

// findStep finds a step by ID in the plan
func (e *Executor) findStep(plan *Plan, stepID string) *Step {
	for i := range plan.Steps {
		if plan.Steps[i].ID == stepID {
			return &plan.Steps[i]
		}
	}
	return nil
}

// GetStepStatus returns the status of a step
func (e *Executor) GetStepStatus(plan *Plan, stepID string) (StepStatus, error) {
	step := e.findStep(plan, stepID)
	if step == nil {
		return "", fmt.Errorf("step not found: %s", stepID)
	}
	return step.Status, nil
}

// GetStepResult returns the result of a step
func (e *Executor) GetStepResult(plan *Plan, stepID string) (*StepResult, error) {
	step := e.findStep(plan, stepID)
	if step == nil {
		return nil, fmt.Errorf("step not found: %s", stepID)
	}
	return step.Result, nil
}
