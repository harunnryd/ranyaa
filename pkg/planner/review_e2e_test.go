//go:build e2e
// +build e2e

package planner

import (
	"context"
	"testing"
	"time"
)

func TestReviewE2E(t *testing.T) {
	t.Run("complete review-execute-reflect workflow", func(t *testing.T) {
		p := NewPlanner()
		e := NewExecutor(p)
		ref := NewReflector(p)

		// Create initial plan
		steps := []Step{
			{ID: "init", Description: "Initialize", EstimatedTime: 2 * time.Second},
			{ID: "process", Description: "Process data", Dependencies: []string{"init"}, EstimatedTime: 10 * time.Second},
			{ID: "finalize", Description: "Finalize", Dependencies: []string{"process"}, EstimatedTime: 3 * time.Second},
		}

		plan, err := p.GeneratePlan("Data processing pipeline", steps)
		if err != nil {
			t.Fatalf("GeneratePlan failed: %v", err)
		}

		t.Logf("Initial plan: %d steps", len(plan.Steps))

		// Review callback - user adds validation step
		reviewCallback := func(ctx context.Context, reviewPlan *Plan) (ReviewDecision, *Plan, error) {
			r := NewReviewer(p, nil)

			// Display plan to user
			formatted := r.FormatPlanForReview(reviewPlan)
			t.Logf("Plan for review:\n%s", formatted)

			// User decides to add validation step
			validationStep := Step{
				ID:            "validate",
				Description:   "Validate results",
				Dependencies:  []string{"process"},
				EstimatedTime: 5 * time.Second,
			}

			err := r.AddStep(reviewPlan, validationStep)
			if err != nil {
				return ReviewReject, nil, err
			}

			// Update finalize to depend on validation
			err = r.ModifyStep(reviewPlan, "finalize", func(step *Step) error {
				step.Dependencies = []string{"process", "validate"}
				return nil
			})
			if err != nil {
				return ReviewReject, nil, err
			}

			t.Logf("User added validation step and updated dependencies")
			return ReviewModify, reviewPlan, nil
		}

		rev := NewReviewer(p, reviewCallback)

		// Review plan
		reviewedPlan, err := rev.Review(context.Background(), plan)
		if err != nil {
			t.Fatalf("Review failed: %v", err)
		}

		t.Logf("Reviewed plan: %d steps", len(reviewedPlan.Steps))

		// Execute reviewed plan
		executionLog := []string{}
		executor := func(ctx context.Context, step *Step) error {
			time.Sleep(50 * time.Millisecond)
			executionLog = append(executionLog, step.ID)
			t.Logf("Executing: %s", step.ID)
			return nil
		}

		err = e.Execute(context.Background(), reviewedPlan, executor)
		if err != nil {
			t.Fatalf("Execute failed: %v", err)
		}

		// Verify execution order
		if len(executionLog) != 4 {
			t.Errorf("Expected 4 steps executed, got %d", len(executionLog))
		}

		// Verify init executed first
		if executionLog[0] != "init" {
			t.Errorf("Expected init first, got %s", executionLog[0])
		}

		// Verify finalize executed last
		if executionLog[len(executionLog)-1] != "finalize" {
			t.Errorf("Expected finalize last, got %s", executionLog[len(executionLog)-1])
		}

		t.Logf("Execution order: %v", executionLog)

		// Reflect on execution
		reflection, err := ref.Reflect(context.Background(), reviewedPlan)
		if err != nil {
			t.Fatalf("Reflect failed: %v", err)
		}

		t.Logf("Reflection: %d issues, %d suggestions", len(reflection.Issues), len(reflection.Suggestions))

		// Verify all steps completed successfully
		for _, step := range reviewedPlan.Steps {
			if step.Status != StepStatusCompleted {
				t.Errorf("Step %s not completed: %s", step.ID, step.Status)
			}

			if step.Result == nil {
				t.Errorf("Step %s has no result", step.ID)
				continue
			}

			t.Logf("Step %s: %s (duration: %v)", step.ID, step.Status, step.Result.Duration)
		}

		// Display final formatted plan
		formatted := rev.FormatPlanForReview(reviewedPlan)
		t.Logf("Final plan:\n%s", formatted)
	})
}
