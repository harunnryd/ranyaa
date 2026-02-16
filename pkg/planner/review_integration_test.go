//go:build integration
// +build integration

package planner

import (
	"context"
	"testing"
	"time"
)

func TestReviewIntegration(t *testing.T) {
	p := NewPlanner()

	t.Run("review and execute workflow", func(t *testing.T) {
		// Create initial plan
		steps := []Step{
			{ID: "step-1", Description: "Step 1", EstimatedTime: 5 * time.Second},
			{ID: "step-2", Description: "Step 2", Dependencies: []string{"step-1"}, EstimatedTime: 10 * time.Second},
		}

		plan, _ := p.GeneratePlan("Initial plan", steps)

		// Review callback that modifies the plan
		callback := func(ctx context.Context, reviewPlan *Plan) (ReviewDecision, *Plan, error) {
			// User adds a new step
			modifiedPlan := *reviewPlan
			modifiedPlan.Steps = append(modifiedPlan.Steps, Step{
				ID:            "step-3",
				Description:   "Added during review",
				Dependencies:  []string{"step-2"},
				EstimatedTime: 3 * time.Second,
			})
			return ReviewModify, &modifiedPlan, nil
		}

		r := NewReviewer(p, callback)

		// Review plan
		reviewedPlan, err := r.Review(context.Background(), plan)
		if err != nil {
			t.Fatalf("Review failed: %v", err)
		}

		// Verify modification
		if len(reviewedPlan.Steps) != 3 {
			t.Errorf("Expected 3 steps after review, got %d", len(reviewedPlan.Steps))
		}

		// Execute reviewed plan
		e := NewExecutor(p)
		executedSteps := []string{}

		executor := func(ctx context.Context, step *Step) error {
			executedSteps = append(executedSteps, step.ID)
			return nil
		}

		err = e.Execute(context.Background(), reviewedPlan, executor)
		if err != nil {
			t.Fatalf("Execute failed: %v", err)
		}

		// Verify all steps executed
		if len(executedSteps) != 3 {
			t.Errorf("Expected 3 steps executed, got %d", len(executedSteps))
		}

		t.Logf("Executed steps: %v", executedSteps)
	})

	t.Run("interactive plan modification", func(t *testing.T) {
		steps := []Step{
			{ID: "build", Description: "Build application"},
			{ID: "test", Description: "Run tests", Dependencies: []string{"build"}},
			{ID: "deploy", Description: "Deploy", Dependencies: []string{"test"}},
		}

		plan, _ := p.GeneratePlan("Deployment plan", steps)

		// Simulate user modifying plan during review
		callback := func(ctx context.Context, reviewPlan *Plan) (ReviewDecision, *Plan, error) {
			r := NewReviewer(p, nil)

			// User wants to add a security scan step
			newStep := Step{
				ID:           "security-scan",
				Description:  "Run security scan",
				Dependencies: []string{"build"},
			}

			err := r.AddStep(reviewPlan, newStep)
			if err != nil {
				return ReviewReject, nil, err
			}

			// User wants to modify deploy step to depend on security scan
			err = r.ModifyStep(reviewPlan, "deploy", func(step *Step) error {
				step.Dependencies = []string{"test", "security-scan"}
				return nil
			})
			if err != nil {
				return ReviewReject, nil, err
			}

			return ReviewModify, reviewPlan, nil
		}

		r := NewReviewer(p, callback)

		reviewedPlan, err := r.Review(context.Background(), plan)
		if err != nil {
			t.Fatalf("Review failed: %v", err)
		}

		// Verify modifications
		if len(reviewedPlan.Steps) != 4 {
			t.Errorf("Expected 4 steps, got %d", len(reviewedPlan.Steps))
		}

		// Verify deploy step dependencies
		var deployStep *Step
		for i := range reviewedPlan.Steps {
			if reviewedPlan.Steps[i].ID == "deploy" {
				deployStep = &reviewedPlan.Steps[i]
				break
			}
		}

		if deployStep == nil {
			t.Fatal("Deploy step not found")
		}

		if len(deployStep.Dependencies) != 2 {
			t.Errorf("Expected 2 dependencies for deploy, got %d", len(deployStep.Dependencies))
		}

		t.Logf("Modified plan has %d steps", len(reviewedPlan.Steps))
		t.Logf("Deploy dependencies: %v", deployStep.Dependencies)
	})
}
