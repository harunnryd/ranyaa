package planner

import (
	"context"
	"fmt"
	"strings"
)

// Reflection represents a review of execution results
type Reflection struct {
	PlanID      string            `json:"plan_id"`
	Issues      []Issue           `json:"issues"`
	Suggestions []Suggestion      `json:"suggestions"`
	ShouldRetry bool              `json:"should_retry"`
	Adjustments map[string]string `json:"adjustments"` // Step ID -> adjustment description
}

// Issue represents a problem identified during reflection
type Issue struct {
	StepID      string        `json:"step_id"`
	Severity    IssueSeverity `json:"severity"`
	Description string        `json:"description"`
	Error       string        `json:"error,omitempty"`
}

// IssueSeverity represents the severity of an issue
type IssueSeverity string

const (
	IssueSeverityLow      IssueSeverity = "low"
	IssueSeverityMedium   IssueSeverity = "medium"
	IssueSeverityHigh     IssueSeverity = "high"
	IssueSeverityCritical IssueSeverity = "critical"
)

// Suggestion represents a recommended correction
type Suggestion struct {
	StepID      string `json:"step_id"`
	Description string `json:"description"`
	Action      string `json:"action"` // retry, skip, modify, etc.
}

// Reflector reviews execution results and suggests corrections
type Reflector struct {
	planner *Planner
}

// NewReflector creates a new reflector instance
func NewReflector(planner *Planner) *Reflector {
	return &Reflector{
		planner: planner,
	}
}

// Reflect reviews the execution results of a plan
func (r *Reflector) Reflect(ctx context.Context, plan *Plan) (*Reflection, error) {
	reflection := &Reflection{
		PlanID:      plan.ID,
		Issues:      []Issue{},
		Suggestions: []Suggestion{},
		Adjustments: make(map[string]string),
	}

	// Analyze each step
	for _, step := range plan.Steps {
		r.analyzeStep(&step, reflection)
	}

	// Determine if retry is recommended
	reflection.ShouldRetry = r.shouldRetry(reflection)

	return reflection, nil
}

// analyzeStep analyzes a single step and adds issues/suggestions
func (r *Reflector) analyzeStep(step *Step, reflection *Reflection) {
	switch step.Status {
	case StepStatusFailed:
		r.analyzeFailedStep(step, reflection)
	case StepStatusCompleted:
		r.analyzeCompletedStep(step, reflection)
	case StepStatusSkipped:
		r.analyzeSkippedStep(step, reflection)
	}
}

// analyzeFailedStep analyzes a failed step
func (r *Reflector) analyzeFailedStep(step *Step, reflection *Reflection) {
	issue := Issue{
		StepID:      step.ID,
		Severity:    r.determineSeverity(step),
		Description: fmt.Sprintf("Step '%s' failed", step.Description),
	}

	if step.Result != nil && step.Result.Error != "" {
		issue.Error = step.Result.Error
	}

	reflection.Issues = append(reflection.Issues, issue)

	// Generate suggestions based on error patterns
	suggestion := r.generateSuggestion(step)
	if suggestion != nil {
		reflection.Suggestions = append(reflection.Suggestions, *suggestion)
	}
}

// analyzeCompletedStep analyzes a completed step for potential issues
func (r *Reflector) analyzeCompletedStep(step *Step, reflection *Reflection) {
	// Check if step took significantly longer than estimated
	if step.Result != nil && step.EstimatedTime > 0 {
		actualTime := step.Result.Duration
		if actualTime > step.EstimatedTime*2 {
			issue := Issue{
				StepID:      step.ID,
				Severity:    IssueSeverityLow,
				Description: fmt.Sprintf("Step took longer than expected (estimated: %v, actual: %v)", step.EstimatedTime, actualTime),
			}
			reflection.Issues = append(reflection.Issues, issue)

			suggestion := Suggestion{
				StepID:      step.ID,
				Description: "Consider optimizing this step or adjusting time estimates",
				Action:      "optimize",
			}
			reflection.Suggestions = append(reflection.Suggestions, suggestion)
		}
	}

	// Check if step required multiple attempts
	if step.Attempts > 1 {
		issue := Issue{
			StepID:      step.ID,
			Severity:    IssueSeverityMedium,
			Description: fmt.Sprintf("Step required %d attempts to complete", step.Attempts),
		}
		reflection.Issues = append(reflection.Issues, issue)

		suggestion := Suggestion{
			StepID:      step.ID,
			Description: "Investigate why step is flaky and improve reliability",
			Action:      "investigate",
		}
		reflection.Suggestions = append(reflection.Suggestions, suggestion)
	}
}

// analyzeSkippedStep analyzes a skipped step
func (r *Reflector) analyzeSkippedStep(step *Step, reflection *Reflection) {
	issue := Issue{
		StepID:      step.ID,
		Severity:    IssueSeverityMedium,
		Description: fmt.Sprintf("Step '%s' was skipped", step.Description),
	}
	reflection.Issues = append(reflection.Issues, issue)
}

// determineSeverity determines the severity of a failed step
func (r *Reflector) determineSeverity(step *Step) IssueSeverity {
	// Check if step has dependents (other steps depend on it)
	// If so, failure is more severe
	// For now, use a simple heuristic based on attempts
	if step.Attempts >= 3 {
		return IssueSeverityCritical
	}
	if step.Attempts >= 2 {
		return IssueSeverityHigh
	}
	return IssueSeverityMedium
}

// generateSuggestion generates a suggestion for a failed step
func (r *Reflector) generateSuggestion(step *Step) *Suggestion {
	if step.Result == nil || step.Result.Error == "" {
		return nil
	}

	errorMsg := strings.ToLower(step.Result.Error)

	// Pattern matching for common errors
	if strings.Contains(errorMsg, "timeout") || strings.Contains(errorMsg, "deadline exceeded") {
		return &Suggestion{
			StepID:      step.ID,
			Description: "Increase timeout or optimize step execution",
			Action:      "retry",
		}
	}

	if strings.Contains(errorMsg, "connection") || strings.Contains(errorMsg, "network") {
		return &Suggestion{
			StepID:      step.ID,
			Description: "Network issue detected - retry may succeed",
			Action:      "retry",
		}
	}

	if strings.Contains(errorMsg, "permission") || strings.Contains(errorMsg, "access denied") {
		return &Suggestion{
			StepID:      step.ID,
			Description: "Permission issue - check access rights",
			Action:      "modify",
		}
	}

	if strings.Contains(errorMsg, "not found") || strings.Contains(errorMsg, "does not exist") {
		return &Suggestion{
			StepID:      step.ID,
			Description: "Resource not found - verify dependencies",
			Action:      "modify",
		}
	}

	// Generic suggestion
	return &Suggestion{
		StepID:      step.ID,
		Description: "Review error and adjust step configuration",
		Action:      "modify",
	}
}

// shouldRetry determines if the plan should be retried
func (r *Reflector) shouldRetry(reflection *Reflection) bool {
	// Don't retry if no issues
	if len(reflection.Issues) == 0 {
		return false
	}

	// Don't retry if there are critical issues
	for _, issue := range reflection.Issues {
		if issue.Severity == IssueSeverityCritical {
			return false
		}
	}

	// Retry if there are suggestions with retry action
	for _, suggestion := range reflection.Suggestions {
		if suggestion.Action == "retry" {
			return true
		}
	}

	return false
}

// ApplyAdjustments applies suggested adjustments to a plan
func (r *Reflector) ApplyAdjustments(plan *Plan, reflection *Reflection) error {
	for stepID, adjustment := range reflection.Adjustments {
		step := r.findStep(plan, stepID)
		if step == nil {
			return fmt.Errorf("step not found: %s", stepID)
		}

		// Reset step status for retry
		step.Status = StepStatusPending
		step.Result = nil
		step.Attempts = 0

		// Apply adjustment (in a real system, this would modify step configuration)
		// For now, we just log the adjustment
		_ = adjustment
	}

	return nil
}

// findStep finds a step by ID in the plan
func (r *Reflector) findStep(plan *Plan, stepID string) *Step {
	for i := range plan.Steps {
		if plan.Steps[i].ID == stepID {
			return &plan.Steps[i]
		}
	}
	return nil
}

// GetIssuesBySeverity returns issues filtered by severity
func (r *Reflector) GetIssuesBySeverity(reflection *Reflection, severity IssueSeverity) []Issue {
	var issues []Issue
	for _, issue := range reflection.Issues {
		if issue.Severity == severity {
			issues = append(issues, issue)
		}
	}
	return issues
}

// GetSuggestionsByAction returns suggestions filtered by action
func (r *Reflector) GetSuggestionsByAction(reflection *Reflection, action string) []Suggestion {
	var suggestions []Suggestion
	for _, suggestion := range reflection.Suggestions {
		if suggestion.Action == action {
			suggestions = append(suggestions, suggestion)
		}
	}
	return suggestions
}
