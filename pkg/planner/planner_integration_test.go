//go:build integration
// +build integration

package planner

import (
	"testing"
	"time"
)

func TestPlannerIntegration(t *testing.T) {
	p := NewPlanner()

	t.Run("generate and execute complex plan", func(t *testing.T) {
		// Create a complex plan with multiple dependencies
		steps := []Step{
			{
				ID:            "init",
				Description:   "Initialize project",
				EstimatedTime: 5 * time.Second,
			},
			{
				ID:            "fetch-deps",
				Description:   "Fetch dependencies",
				Dependencies:  []string{"init"},
				EstimatedTime: 10 * time.Second,
			},
			{
				ID:            "build-frontend",
				Description:   "Build frontend",
				Dependencies:  []string{"fetch-deps"},
				EstimatedTime: 15 * time.Second,
			},
			{
				ID:            "build-backend",
				Description:   "Build backend",
				Dependencies:  []string{"fetch-deps"},
				EstimatedTime: 20 * time.Second,
			},
			{
				ID:            "run-tests",
				Description:   "Run tests",
				Dependencies:  []string{"build-frontend", "build-backend"},
				EstimatedTime: 30 * time.Second,
			},
			{
				ID:            "deploy",
				Description:   "Deploy application",
				Dependencies:  []string{"run-tests"},
				EstimatedTime: 10 * time.Second,
			},
		}

		plan, err := p.GeneratePlan("Deploy application", steps)
		if err != nil {
			t.Fatalf("GeneratePlan failed: %v", err)
		}

		// Verify plan structure
		if len(plan.Steps) != 6 {
			t.Errorf("Expected 6 steps, got %d", len(plan.Steps))
		}

		// Get execution order
		levels, err := p.GetExecutionOrder(plan)
		if err != nil {
			t.Fatalf("GetExecutionOrder failed: %v", err)
		}

		// Verify execution levels
		if len(levels) != 5 {
			t.Errorf("Expected 5 levels, got %d", len(levels))
		}

		// Level 0: init
		if len(levels[0]) != 1 || levels[0][0] != "init" {
			t.Errorf("Level 0 should contain only 'init', got %v", levels[0])
		}

		// Level 1: fetch-deps
		if len(levels[1]) != 1 || levels[1][0] != "fetch-deps" {
			t.Errorf("Level 1 should contain only 'fetch-deps', got %v", levels[1])
		}

		// Level 2: build-frontend and build-backend (parallel)
		if len(levels[2]) != 2 {
			t.Errorf("Level 2 should contain 2 steps, got %d", len(levels[2]))
		}

		// Level 3: run-tests
		if len(levels[3]) != 1 || levels[3][0] != "run-tests" {
			t.Errorf("Level 3 should contain only 'run-tests', got %v", levels[3])
		}

		// Level 4: deploy
		if len(levels[4]) != 1 || levels[4][0] != "deploy" {
			t.Errorf("Level 4 should contain only 'deploy', got %v", levels[4])
		}
	})

	t.Run("plan with diamond dependency", func(t *testing.T) {
		// Diamond dependency: A -> B, A -> C, B -> D, C -> D
		steps := []Step{
			{ID: "A", Description: "Step A"},
			{ID: "B", Description: "Step B", Dependencies: []string{"A"}},
			{ID: "C", Description: "Step C", Dependencies: []string{"A"}},
			{ID: "D", Description: "Step D", Dependencies: []string{"B", "C"}},
		}

		plan, err := p.GeneratePlan("Diamond dependency", steps)
		if err != nil {
			t.Fatalf("GeneratePlan failed: %v", err)
		}

		levels, err := p.GetExecutionOrder(plan)
		if err != nil {
			t.Fatalf("GetExecutionOrder failed: %v", err)
		}

		// Verify execution levels
		if len(levels) != 3 {
			t.Errorf("Expected 3 levels, got %d", len(levels))
		}

		// Level 0: A
		if len(levels[0]) != 1 || levels[0][0] != "A" {
			t.Errorf("Level 0 should contain only 'A', got %v", levels[0])
		}

		// Level 1: B and C (parallel)
		if len(levels[1]) != 2 {
			t.Errorf("Level 1 should contain 2 steps, got %d", len(levels[1]))
		}

		// Level 2: D
		if len(levels[2]) != 1 || levels[2][0] != "D" {
			t.Errorf("Level 2 should contain only 'D', got %v", levels[2])
		}
	})

	t.Run("plan with multiple independent chains", func(t *testing.T) {
		// Two independent chains: A -> B -> C and X -> Y -> Z
		steps := []Step{
			{ID: "A", Description: "Step A"},
			{ID: "B", Description: "Step B", Dependencies: []string{"A"}},
			{ID: "C", Description: "Step C", Dependencies: []string{"B"}},
			{ID: "X", Description: "Step X"},
			{ID: "Y", Description: "Step Y", Dependencies: []string{"X"}},
			{ID: "Z", Description: "Step Z", Dependencies: []string{"Y"}},
		}

		plan, err := p.GeneratePlan("Multiple chains", steps)
		if err != nil {
			t.Fatalf("GeneratePlan failed: %v", err)
		}

		levels, err := p.GetExecutionOrder(plan)
		if err != nil {
			t.Fatalf("GetExecutionOrder failed: %v", err)
		}

		// Verify execution levels
		if len(levels) != 3 {
			t.Errorf("Expected 3 levels, got %d", len(levels))
		}

		// Level 0: A and X (parallel)
		if len(levels[0]) != 2 {
			t.Errorf("Level 0 should contain 2 steps, got %d", len(levels[0]))
		}

		// Level 1: B and Y (parallel)
		if len(levels[1]) != 2 {
			t.Errorf("Level 1 should contain 2 steps, got %d", len(levels[1]))
		}

		// Level 2: C and Z (parallel)
		if len(levels[2]) != 2 {
			t.Errorf("Level 2 should contain 2 steps, got %d", len(levels[2]))
		}
	})
}
