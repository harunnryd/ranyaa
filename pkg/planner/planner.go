package planner

import (
	"fmt"
	"time"

	"github.com/google/uuid"
)

// Planner generates execution plans for complex tasks
type Planner struct {
	planningMode string
}

// NewPlanner creates a new planner instance
func NewPlanner() *Planner {
	return &Planner{
		planningMode: "rule-based",
	}
}

// GeneratePlan creates a plan with the given description and steps
func (p *Planner) GeneratePlan(description string, steps []Step) (*Plan, error) {
	if description == "" {
		return nil, fmt.Errorf("plan description cannot be empty")
	}
	if len(steps) == 0 {
		return nil, fmt.Errorf("plan must have at least one step")
	}

	// Validate steps
	if err := p.validateSteps(steps); err != nil {
		return nil, fmt.Errorf("invalid steps: %w", err)
	}

	// Assign IDs to steps if not provided
	for i := range steps {
		if steps[i].ID == "" {
			steps[i].ID = fmt.Sprintf("step-%d", i+1)
		}
		if steps[i].Status == "" {
			steps[i].Status = StepStatusPending
		}
	}

	plan := &Plan{
		ID:          uuid.New().String(),
		Description: description,
		Steps:       steps,
		CreatedAt:   time.Now(),
	}

	return plan, nil
}

// validateSteps validates the steps in a plan
func (p *Planner) validateSteps(steps []Step) error {
	// Check for duplicate step IDs
	stepIDs := make(map[string]bool)
	for _, step := range steps {
		if step.ID != "" {
			if stepIDs[step.ID] {
				return fmt.Errorf("duplicate step ID: %s", step.ID)
			}
			stepIDs[step.ID] = true
		}
	}

	// Validate dependencies
	for _, step := range steps {
		for _, depID := range step.Dependencies {
			if !stepIDs[depID] && depID != "" {
				return fmt.Errorf("step %s depends on non-existent step: %s", step.ID, depID)
			}
		}
	}

	// Check for circular dependencies
	if err := p.checkCircularDependencies(steps); err != nil {
		return err
	}

	return nil
}

// checkCircularDependencies detects circular dependencies in steps
func (p *Planner) checkCircularDependencies(steps []Step) error {
	// Build dependency graph
	graph := make(map[string][]string)
	for _, step := range steps {
		graph[step.ID] = step.Dependencies
	}

	// Check each step for circular dependencies using DFS
	visited := make(map[string]bool)
	recStack := make(map[string]bool)

	var hasCycle func(string) bool
	hasCycle = func(stepID string) bool {
		visited[stepID] = true
		recStack[stepID] = true

		for _, dep := range graph[stepID] {
			if !visited[dep] {
				if hasCycle(dep) {
					return true
				}
			} else if recStack[dep] {
				return true
			}
		}

		recStack[stepID] = false
		return false
	}

	for stepID := range graph {
		if !visited[stepID] {
			if hasCycle(stepID) {
				return fmt.Errorf("circular dependency detected involving step: %s", stepID)
			}
		}
	}

	return nil
}

// GetExecutionOrder returns steps in dependency order (topological sort)
func (p *Planner) GetExecutionOrder(plan *Plan) ([][]string, error) {
	// Build dependency graph
	graph := make(map[string][]string)
	inDegree := make(map[string]int)

	for _, step := range plan.Steps {
		if _, exists := inDegree[step.ID]; !exists {
			inDegree[step.ID] = 0
		}
		for _, dep := range step.Dependencies {
			graph[dep] = append(graph[dep], step.ID)
			inDegree[step.ID]++
		}
	}

	// Topological sort with level tracking (for parallel execution)
	var levels [][]string
	queue := []string{}

	// Find steps with no dependencies
	for stepID, degree := range inDegree {
		if degree == 0 {
			queue = append(queue, stepID)
		}
	}

	for len(queue) > 0 {
		// All steps in current queue can be executed in parallel
		currentLevel := make([]string, len(queue))
		copy(currentLevel, queue)
		levels = append(levels, currentLevel)

		// Process current level
		nextQueue := []string{}
		for _, stepID := range queue {
			for _, dependent := range graph[stepID] {
				inDegree[dependent]--
				if inDegree[dependent] == 0 {
					nextQueue = append(nextQueue, dependent)
				}
			}
		}
		queue = nextQueue
	}

	// Check if all steps were processed (no cycles)
	if len(levels) == 0 && len(plan.Steps) > 0 {
		return nil, fmt.Errorf("cannot determine execution order: circular dependencies")
	}

	totalSteps := 0
	for _, level := range levels {
		totalSteps += len(level)
	}
	if totalSteps != len(plan.Steps) {
		return nil, fmt.Errorf("cannot determine execution order: circular dependencies")
	}

	return levels, nil
}
