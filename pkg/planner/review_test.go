package planner

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestNewReviewer(t *testing.T) {
	p := NewPlanner()
	callback := func(ctx context.Context, plan *Plan) (ReviewDecision, *Plan, error) {
		return ReviewApprove, plan, nil
	}

	r := NewReviewer(p, callback)
	if r == nil {
		t.Fatal("NewReviewer returned nil")
	}
}

func TestReviewApprove(t *testing.T) {
	p := NewPlanner()
	callback := func(ctx context.Context, plan *Plan) (ReviewDecision, *Plan, error) {
		return ReviewApprove, plan, nil
	}

	r := NewReviewer(p, callback)

	steps := []Step{
		{ID: "step-1", Description: "Step 1"},
	}

	plan, _ := p.GeneratePlan("Test plan", steps)

	reviewedPlan, err := r.Review(context.Background(), plan)
	if err != nil {
		t.Fatalf("Review failed: %v", err)
	}

	if reviewedPlan.ID != plan.ID {
		t.Error("Reviewed plan ID doesn't match original")
	}
}

func TestReviewModify(t *testing.T) {
	p := NewPlanner()
	callback := func(ctx context.Context, plan *Plan) (ReviewDecision, *Plan, error) {
		// Modify plan - add a new step
		modifiedPlan := *plan
		modifiedPlan.Steps = append(modifiedPlan.Steps, Step{
			ID:          "step-2",
			Description: "Added step",
		})
		return ReviewModify, &modifiedPlan, nil
	}

	r := NewReviewer(p, callback)

	steps := []Step{
		{ID: "step-1", Description: "Step 1"},
	}

	plan, _ := p.GeneratePlan("Test plan", steps)

	reviewedPlan, err := r.Review(context.Background(), plan)
	if err != nil {
		t.Fatalf("Review failed: %v", err)
	}

	if len(reviewedPlan.Steps) != 2 {
		t.Errorf("Expected 2 steps after modification, got %d", len(reviewedPlan.Steps))
	}
}

func TestReviewReject(t *testing.T) {
	p := NewPlanner()
	callback := func(ctx context.Context, plan *Plan) (ReviewDecision, *Plan, error) {
		return ReviewReject, nil, nil
	}

	r := NewReviewer(p, callback)

	steps := []Step{
		{ID: "step-1", Description: "Step 1"},
	}

	plan, _ := p.GeneratePlan("Test plan", steps)

	_, err := r.Review(context.Background(), plan)
	if err == nil {
		t.Error("Expected error for rejected plan")
	}
}

func TestReviewNoCallback(t *testing.T) {
	p := NewPlanner()
	r := NewReviewer(p, nil)

	steps := []Step{
		{ID: "step-1", Description: "Step 1"},
	}

	plan, _ := p.GeneratePlan("Test plan", steps)

	reviewedPlan, err := r.Review(context.Background(), plan)
	if err != nil {
		t.Fatalf("Review failed: %v", err)
	}

	if reviewedPlan.ID != plan.ID {
		t.Error("Plan should be auto-approved when no callback")
	}
}

func TestReviewCallbackError(t *testing.T) {
	p := NewPlanner()
	callback := func(ctx context.Context, plan *Plan) (ReviewDecision, *Plan, error) {
		return ReviewApprove, nil, errors.New("callback error")
	}

	r := NewReviewer(p, callback)

	steps := []Step{
		{ID: "step-1", Description: "Step 1"},
	}

	plan, _ := p.GeneratePlan("Test plan", steps)

	_, err := r.Review(context.Background(), plan)
	if err == nil {
		t.Error("Expected error from callback")
	}
}

func TestValidateModifiedPlan(t *testing.T) {
	p := NewPlanner()
	r := NewReviewer(p, nil)

	t.Run("valid modified plan", func(t *testing.T) {
		plan := &Plan{
			ID:          "plan-1",
			Description: "Test plan",
			Steps: []Step{
				{ID: "step-1", Description: "Step 1"},
			},
		}

		err := r.validateModifiedPlan(plan)
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
	})

	t.Run("empty plan ID", func(t *testing.T) {
		plan := &Plan{
			ID:    "",
			Steps: []Step{{ID: "step-1"}},
		}

		err := r.validateModifiedPlan(plan)
		if err == nil {
			t.Error("Expected error for empty plan ID")
		}
	})

	t.Run("no steps", func(t *testing.T) {
		plan := &Plan{
			ID:    "plan-1",
			Steps: []Step{},
		}

		err := r.validateModifiedPlan(plan)
		if err == nil {
			t.Error("Expected error for no steps")
		}
	})

	t.Run("duplicate step ID", func(t *testing.T) {
		plan := &Plan{
			ID: "plan-1",
			Steps: []Step{
				{ID: "step-1"},
				{ID: "step-1"},
			},
		}

		err := r.validateModifiedPlan(plan)
		if err == nil {
			t.Error("Expected error for duplicate step ID")
		}
	})
}

func TestFormatPlanForReview(t *testing.T) {
	p := NewPlanner()
	r := NewReviewer(p, nil)

	steps := []Step{
		{
			ID:            "step-1",
			Description:   "First step",
			EstimatedTime: 5 * time.Second,
		},
		{
			ID:            "step-2",
			Description:   "Second step",
			Dependencies:  []string{"step-1"},
			EstimatedTime: 10 * time.Second,
		},
	}

	plan, _ := p.GeneratePlan("Test plan", steps)

	output := r.FormatPlanForReview(plan)

	// Verify output contains key information
	if !strings.Contains(output, "Test plan") {
		t.Error("Output should contain plan description")
	}

	if !strings.Contains(output, "step-1") {
		t.Error("Output should contain step-1")
	}

	if !strings.Contains(output, "step-2") {
		t.Error("Output should contain step-2")
	}

	if !strings.Contains(output, "First step") {
		t.Error("Output should contain step descriptions")
	}

	if !strings.Contains(output, "Dependencies") {
		t.Error("Output should show dependencies")
	}

	t.Logf("Formatted plan:\n%s", output)
}

func TestModifyStep(t *testing.T) {
	p := NewPlanner()
	r := NewReviewer(p, nil)

	steps := []Step{
		{ID: "step-1", Description: "Original description"},
	}

	plan, _ := p.GeneratePlan("Test plan", steps)

	t.Run("modify existing step", func(t *testing.T) {
		err := r.ModifyStep(plan, "step-1", func(step *Step) error {
			step.Description = "Modified description"
			return nil
		})

		if err != nil {
			t.Fatalf("ModifyStep failed: %v", err)
		}

		if plan.Steps[0].Description != "Modified description" {
			t.Error("Step description was not modified")
		}
	})

	t.Run("modify non-existent step", func(t *testing.T) {
		err := r.ModifyStep(plan, "step-999", func(step *Step) error {
			return nil
		})

		if err == nil {
			t.Error("Expected error for non-existent step")
		}
	})

	t.Run("modifier returns error", func(t *testing.T) {
		err := r.ModifyStep(plan, "step-1", func(step *Step) error {
			return errors.New("modifier error")
		})

		if err == nil {
			t.Error("Expected error from modifier")
		}
	})
}

func TestAddStep(t *testing.T) {
	p := NewPlanner()
	r := NewReviewer(p, nil)

	steps := []Step{
		{ID: "step-1", Description: "Step 1"},
	}

	plan, _ := p.GeneratePlan("Test plan", steps)

	t.Run("add valid step", func(t *testing.T) {
		newStep := Step{
			ID:           "step-2",
			Description:  "Step 2",
			Dependencies: []string{"step-1"},
		}

		err := r.AddStep(plan, newStep)
		if err != nil {
			t.Fatalf("AddStep failed: %v", err)
		}

		if len(plan.Steps) != 2 {
			t.Errorf("Expected 2 steps, got %d", len(plan.Steps))
		}
	})

	t.Run("add step with duplicate ID", func(t *testing.T) {
		newStep := Step{
			ID:          "step-1",
			Description: "Duplicate",
		}

		err := r.AddStep(plan, newStep)
		if err == nil {
			t.Error("Expected error for duplicate step ID")
		}
	})

	t.Run("add step with invalid dependency", func(t *testing.T) {
		newStep := Step{
			ID:           "step-3",
			Description:  "Step 3",
			Dependencies: []string{"step-999"},
		}

		err := r.AddStep(plan, newStep)
		if err == nil {
			t.Error("Expected error for invalid dependency")
		}
	})
}

func TestRemoveStep(t *testing.T) {
	p := NewPlanner()
	r := NewReviewer(p, nil)

	t.Run("remove step without dependents", func(t *testing.T) {
		steps := []Step{
			{ID: "step-1", Description: "Step 1"},
			{ID: "step-2", Description: "Step 2"},
		}

		plan, _ := p.GeneratePlan("Test plan", steps)

		err := r.RemoveStep(plan, "step-2")
		if err != nil {
			t.Fatalf("RemoveStep failed: %v", err)
		}

		if len(plan.Steps) != 1 {
			t.Errorf("Expected 1 step, got %d", len(plan.Steps))
		}
	})

	t.Run("remove step with dependents", func(t *testing.T) {
		steps := []Step{
			{ID: "step-1", Description: "Step 1"},
			{ID: "step-2", Description: "Step 2", Dependencies: []string{"step-1"}},
		}

		plan, _ := p.GeneratePlan("Test plan", steps)

		err := r.RemoveStep(plan, "step-1")
		if err == nil {
			t.Error("Expected error when removing step with dependents")
		}
	})

	t.Run("remove non-existent step", func(t *testing.T) {
		steps := []Step{
			{ID: "step-1", Description: "Step 1"},
		}

		plan, _ := p.GeneratePlan("Test plan", steps)

		err := r.RemoveStep(plan, "step-999")
		if err == nil {
			t.Error("Expected error for non-existent step")
		}
	})
}
