package planner

import (
	"context"
	"fmt"
)

// ReviewDecision represents the user's decision after reviewing a plan
type ReviewDecision string

const (
	ReviewApprove ReviewDecision = "approve" // Execute plan as-is
	ReviewModify  ReviewDecision = "modify"  // Modify plan before execution
	ReviewReject  ReviewDecision = "reject"  // Reject plan, don't execute
)

// ReviewCallback is called to present a plan to the user for review
// It should return the user's decision and optionally a modified plan
type ReviewCallback func(ctx context.Context, plan *Plan) (ReviewDecision, *Plan, error)

// Reviewer manages the plan review workflow
type Reviewer struct {
	planner  *Planner
	callback ReviewCallback
}

// NewReviewer creates a new reviewer with the given callback
func NewReviewer(planner *Planner, callback ReviewCallback) *Reviewer {
	return &Reviewer{
		planner:  planner,
		callback: callback,
	}
}

// Review presents a plan to the user and returns the reviewed plan
func (r *Reviewer) Review(ctx context.Context, plan *Plan) (*Plan, error) {
	if r.callback == nil {
		// No callback - auto-approve
		return plan, nil
	}

	decision, modifiedPlan, err := r.callback(ctx, plan)
	if err != nil {
		return nil, fmt.Errorf("review callback failed: %w", err)
	}

	switch decision {
	case ReviewApprove:
		return plan, nil

	case ReviewModify:
		if modifiedPlan == nil {
			return nil, fmt.Errorf("modified plan is nil")
		}
		// Validate modified plan
		if err := r.validateModifiedPlan(modifiedPlan); err != nil {
			return nil, fmt.Errorf("invalid modified plan: %w", err)
		}
		return modifiedPlan, nil

	case ReviewReject:
		return nil, fmt.Errorf("plan rejected by user")

	default:
		return nil, fmt.Errorf("invalid review decision: %s", decision)
	}
}

// validateModifiedPlan validates a user-modified plan
func (r *Reviewer) validateModifiedPlan(plan *Plan) error {
	if plan.ID == "" {
		return fmt.Errorf("plan ID is empty")
	}

	if len(plan.Steps) == 0 {
		return fmt.Errorf("plan has no steps")
	}

	// Validate steps
	stepIDs := make(map[string]bool)
	for _, step := range plan.Steps {
		if step.ID == "" {
			return fmt.Errorf("step has empty ID")
		}

		if stepIDs[step.ID] {
			return fmt.Errorf("duplicate step ID: %s", step.ID)
		}
		stepIDs[step.ID] = true

		// Validate dependencies
		for _, depID := range step.Dependencies {
			if !stepIDs[depID] && depID != "" {
				// Check if dependency exists in plan
				found := false
				for _, s := range plan.Steps {
					if s.ID == depID {
						found = true
						break
					}
				}
				if !found {
					return fmt.Errorf("step %s depends on non-existent step: %s", step.ID, depID)
				}
			}
		}
	}

	return nil
}

// FormatPlanForReview formats a plan for display to the user
func (r *Reviewer) FormatPlanForReview(plan *Plan) string {
	output := fmt.Sprintf("Plan: %s\n", plan.Description)
	output += fmt.Sprintf("ID: %s\n", plan.ID)
	output += fmt.Sprintf("Created: %s\n", plan.CreatedAt.Format("2006-01-02 15:04:05"))
	output += fmt.Sprintf("Steps: %d\n\n", len(plan.Steps))

	// Get execution order
	levels, err := r.planner.GetExecutionOrder(plan)
	if err != nil {
		output += fmt.Sprintf("Error: %v\n", err)
		return output
	}

	// Display steps by execution level
	for levelIdx, level := range levels {
		output += fmt.Sprintf("Level %d (parallel):\n", levelIdx)
		for _, stepID := range level {
			step := r.findStep(plan, stepID)
			if step == nil {
				continue
			}

			output += fmt.Sprintf("  - [%s] %s\n", step.ID, step.Description)
			if len(step.Dependencies) > 0 {
				output += fmt.Sprintf("    Dependencies: %v\n", step.Dependencies)
			}
			if step.EstimatedTime > 0 {
				output += fmt.Sprintf("    Estimated time: %v\n", step.EstimatedTime)
			}
		}
		output += "\n"
	}

	return output
}

// findStep finds a step by ID in the plan
func (r *Reviewer) findStep(plan *Plan, stepID string) *Step {
	for i := range plan.Steps {
		if plan.Steps[i].ID == stepID {
			return &plan.Steps[i]
		}
	}
	return nil
}

// ModifyStep modifies a step in the plan
func (r *Reviewer) ModifyStep(plan *Plan, stepID string, modifier func(*Step) error) error {
	step := r.findStep(plan, stepID)
	if step == nil {
		return fmt.Errorf("step not found: %s", stepID)
	}

	return modifier(step)
}

// AddStep adds a new step to the plan
func (r *Reviewer) AddStep(plan *Plan, step Step) error {
	// Validate step ID is unique
	for _, s := range plan.Steps {
		if s.ID == step.ID {
			return fmt.Errorf("step ID already exists: %s", step.ID)
		}
	}

	// Validate dependencies exist
	for _, depID := range step.Dependencies {
		found := false
		for _, s := range plan.Steps {
			if s.ID == depID {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("dependency not found: %s", depID)
		}
	}

	plan.Steps = append(plan.Steps, step)
	return nil
}

// RemoveStep removes a step from the plan
func (r *Reviewer) RemoveStep(plan *Plan, stepID string) error {
	// Check if any other steps depend on this step
	for _, step := range plan.Steps {
		for _, depID := range step.Dependencies {
			if depID == stepID {
				return fmt.Errorf("cannot remove step %s: step %s depends on it", stepID, step.ID)
			}
		}
	}

	// Remove step
	for i, step := range plan.Steps {
		if step.ID == stepID {
			plan.Steps = append(plan.Steps[:i], plan.Steps[i+1:]...)
			return nil
		}
	}

	return fmt.Errorf("step not found: %s", stepID)
}
