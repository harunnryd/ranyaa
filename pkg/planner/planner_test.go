package planner

import (
	"testing"
	"time"
)

func TestNewPlanner(t *testing.T) {
	p := NewPlanner()
	if p == nil {
		t.Fatal("NewPlanner returned nil")
	}
}

func TestGeneratePlan(t *testing.T) {
	p := NewPlanner()

	t.Run("valid plan", func(t *testing.T) {
		steps := []Step{
			{Description: "Step 1", EstimatedTime: 5 * time.Second},
			{Description: "Step 2", EstimatedTime: 10 * time.Second},
		}

		plan, err := p.GeneratePlan("Test plan", steps)
		if err != nil {
			t.Fatalf("GeneratePlan failed: %v", err)
		}

		if plan.ID == "" {
			t.Error("Plan ID is empty")
		}
		if plan.Description != "Test plan" {
			t.Errorf("Expected description 'Test plan', got '%s'", plan.Description)
		}
		if len(plan.Steps) != 2 {
			t.Errorf("Expected 2 steps, got %d", len(plan.Steps))
		}
		if plan.CreatedAt.IsZero() {
			t.Error("Plan CreatedAt is zero")
		}
	})

	t.Run("empty description", func(t *testing.T) {
		steps := []Step{{Description: "Step 1"}}
		_, err := p.GeneratePlan("", steps)
		if err == nil {
			t.Error("Expected error for empty description")
		}
	})

	t.Run("no steps", func(t *testing.T) {
		_, err := p.GeneratePlan("Test plan", []Step{})
		if err == nil {
			t.Error("Expected error for no steps")
		}
	})

	t.Run("auto-assign step IDs", func(t *testing.T) {
		steps := []Step{
			{Description: "Step 1"},
			{Description: "Step 2"},
		}

		plan, err := p.GeneratePlan("Test plan", steps)
		if err != nil {
			t.Fatalf("GeneratePlan failed: %v", err)
		}

		if plan.Steps[0].ID == "" {
			t.Error("Step 1 ID not assigned")
		}
		if plan.Steps[1].ID == "" {
			t.Error("Step 2 ID not assigned")
		}
		if plan.Steps[0].ID == plan.Steps[1].ID {
			t.Error("Step IDs are not unique")
		}
	})

	t.Run("auto-assign step status", func(t *testing.T) {
		steps := []Step{{Description: "Step 1"}}

		plan, err := p.GeneratePlan("Test plan", steps)
		if err != nil {
			t.Fatalf("GeneratePlan failed: %v", err)
		}

		if plan.Steps[0].Status != StepStatusPending {
			t.Errorf("Expected status 'pending', got '%s'", plan.Steps[0].Status)
		}
	})
}

func TestValidateSteps(t *testing.T) {
	p := NewPlanner()

	t.Run("duplicate step IDs", func(t *testing.T) {
		steps := []Step{
			{ID: "step-1", Description: "Step 1"},
			{ID: "step-1", Description: "Step 2"},
		}

		err := p.validateSteps(steps)
		if err == nil {
			t.Error("Expected error for duplicate step IDs")
		}
	})

	t.Run("invalid dependency", func(t *testing.T) {
		steps := []Step{
			{ID: "step-1", Description: "Step 1"},
			{ID: "step-2", Description: "Step 2", Dependencies: []string{"step-3"}},
		}

		err := p.validateSteps(steps)
		if err == nil {
			t.Error("Expected error for invalid dependency")
		}
	})

	t.Run("circular dependency", func(t *testing.T) {
		steps := []Step{
			{ID: "step-1", Description: "Step 1", Dependencies: []string{"step-2"}},
			{ID: "step-2", Description: "Step 2", Dependencies: []string{"step-1"}},
		}

		err := p.validateSteps(steps)
		if err == nil {
			t.Error("Expected error for circular dependency")
		}
	})

	t.Run("valid dependencies", func(t *testing.T) {
		steps := []Step{
			{ID: "step-1", Description: "Step 1"},
			{ID: "step-2", Description: "Step 2", Dependencies: []string{"step-1"}},
		}

		err := p.validateSteps(steps)
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
	})
}

func TestGetExecutionOrder(t *testing.T) {
	p := NewPlanner()

	t.Run("no dependencies", func(t *testing.T) {
		steps := []Step{
			{ID: "step-1", Description: "Step 1"},
			{ID: "step-2", Description: "Step 2"},
		}

		plan, _ := p.GeneratePlan("Test plan", steps)
		levels, err := p.GetExecutionOrder(plan)
		if err != nil {
			t.Fatalf("GetExecutionOrder failed: %v", err)
		}

		if len(levels) != 1 {
			t.Errorf("Expected 1 level, got %d", len(levels))
		}
		if len(levels[0]) != 2 {
			t.Errorf("Expected 2 steps in level 0, got %d", len(levels[0]))
		}
	})

	t.Run("linear dependencies", func(t *testing.T) {
		steps := []Step{
			{ID: "step-1", Description: "Step 1"},
			{ID: "step-2", Description: "Step 2", Dependencies: []string{"step-1"}},
			{ID: "step-3", Description: "Step 3", Dependencies: []string{"step-2"}},
		}

		plan, _ := p.GeneratePlan("Test plan", steps)
		levels, err := p.GetExecutionOrder(plan)
		if err != nil {
			t.Fatalf("GetExecutionOrder failed: %v", err)
		}

		if len(levels) != 3 {
			t.Errorf("Expected 3 levels, got %d", len(levels))
		}
		if levels[0][0] != "step-1" {
			t.Errorf("Expected step-1 in level 0, got %s", levels[0][0])
		}
		if levels[1][0] != "step-2" {
			t.Errorf("Expected step-2 in level 1, got %s", levels[1][0])
		}
		if levels[2][0] != "step-3" {
			t.Errorf("Expected step-3 in level 2, got %s", levels[2][0])
		}
	})

	t.Run("parallel dependencies", func(t *testing.T) {
		steps := []Step{
			{ID: "step-1", Description: "Step 1"},
			{ID: "step-2", Description: "Step 2"},
			{ID: "step-3", Description: "Step 3", Dependencies: []string{"step-1", "step-2"}},
		}

		plan, _ := p.GeneratePlan("Test plan", steps)
		levels, err := p.GetExecutionOrder(plan)
		if err != nil {
			t.Fatalf("GetExecutionOrder failed: %v", err)
		}

		if len(levels) != 2 {
			t.Errorf("Expected 2 levels, got %d", len(levels))
		}
		if len(levels[0]) != 2 {
			t.Errorf("Expected 2 steps in level 0, got %d", len(levels[0]))
		}
		if len(levels[1]) != 1 {
			t.Errorf("Expected 1 step in level 1, got %d", len(levels[1]))
		}
	})

	t.Run("complex dependencies", func(t *testing.T) {
		steps := []Step{
			{ID: "step-1", Description: "Step 1"},
			{ID: "step-2", Description: "Step 2", Dependencies: []string{"step-1"}},
			{ID: "step-3", Description: "Step 3", Dependencies: []string{"step-1"}},
			{ID: "step-4", Description: "Step 4", Dependencies: []string{"step-2", "step-3"}},
		}

		plan, _ := p.GeneratePlan("Test plan", steps)
		levels, err := p.GetExecutionOrder(plan)
		if err != nil {
			t.Fatalf("GetExecutionOrder failed: %v", err)
		}

		if len(levels) != 3 {
			t.Errorf("Expected 3 levels, got %d", len(levels))
		}
		if len(levels[0]) != 1 {
			t.Errorf("Expected 1 step in level 0, got %d", len(levels[0]))
		}
		if len(levels[1]) != 2 {
			t.Errorf("Expected 2 steps in level 1, got %d", len(levels[1]))
		}
		if len(levels[2]) != 1 {
			t.Errorf("Expected 1 step in level 2, got %d", len(levels[2]))
		}
	})
}
