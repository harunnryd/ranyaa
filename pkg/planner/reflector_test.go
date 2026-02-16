package planner

import (
	"context"
	"testing"
	"time"
)

func TestNewReflector(t *testing.T) {
	p := NewPlanner()
	r := NewReflector(p)
	if r == nil {
		t.Fatal("NewReflector returned nil")
	}
}

func TestReflectOnFailedSteps(t *testing.T) {
	p := NewPlanner()
	r := NewReflector(p)

	t.Run("reflect on failed step", func(t *testing.T) {
		steps := []Step{
			{
				ID:          "step-1",
				Description: "Failed step",
				Status:      StepStatusFailed,
				Attempts:    1,
				Result: &StepResult{
					Success: false,
					Error:   "connection timeout",
				},
			},
		}

		plan, _ := p.GeneratePlan("Test plan", steps)
		reflection, err := r.Reflect(context.Background(), plan)
		if err != nil {
			t.Fatalf("Reflect failed: %v", err)
		}

		if len(reflection.Issues) != 1 {
			t.Errorf("Expected 1 issue, got %d", len(reflection.Issues))
		}

		if reflection.Issues[0].StepID != "step-1" {
			t.Errorf("Expected issue for step-1, got %s", reflection.Issues[0].StepID)
		}

		if len(reflection.Suggestions) != 1 {
			t.Errorf("Expected 1 suggestion, got %d", len(reflection.Suggestions))
		}
	})

	t.Run("reflect on multiple failed steps", func(t *testing.T) {
		steps := []Step{
			{
				ID:          "step-1",
				Description: "Failed step 1",
				Status:      StepStatusFailed,
				Result:      &StepResult{Success: false, Error: "error 1"},
			},
			{
				ID:          "step-2",
				Description: "Failed step 2",
				Status:      StepStatusFailed,
				Result:      &StepResult{Success: false, Error: "error 2"},
			},
		}

		plan, _ := p.GeneratePlan("Test plan", steps)
		reflection, err := r.Reflect(context.Background(), plan)
		if err != nil {
			t.Fatalf("Reflect failed: %v", err)
		}

		if len(reflection.Issues) != 2 {
			t.Errorf("Expected 2 issues, got %d", len(reflection.Issues))
		}
	})
}

func TestReflectOnCompletedSteps(t *testing.T) {
	p := NewPlanner()
	r := NewReflector(p)

	t.Run("reflect on slow step", func(t *testing.T) {
		steps := []Step{
			{
				ID:            "step-1",
				Description:   "Slow step",
				Status:        StepStatusCompleted,
				EstimatedTime: 1 * time.Second,
				Result: &StepResult{
					Success:  true,
					Duration: 3 * time.Second, // 3x slower than estimated
				},
			},
		}

		plan, _ := p.GeneratePlan("Test plan", steps)
		reflection, err := r.Reflect(context.Background(), plan)
		if err != nil {
			t.Fatalf("Reflect failed: %v", err)
		}

		if len(reflection.Issues) != 1 {
			t.Errorf("Expected 1 issue for slow step, got %d", len(reflection.Issues))
		}

		if reflection.Issues[0].Severity != IssueSeverityLow {
			t.Errorf("Expected low severity, got %s", reflection.Issues[0].Severity)
		}
	})

	t.Run("reflect on flaky step", func(t *testing.T) {
		steps := []Step{
			{
				ID:          "step-1",
				Description: "Flaky step",
				Status:      StepStatusCompleted,
				Attempts:    3, // Required multiple attempts
				Result:      &StepResult{Success: true},
			},
		}

		plan, _ := p.GeneratePlan("Test plan", steps)
		reflection, err := r.Reflect(context.Background(), plan)
		if err != nil {
			t.Fatalf("Reflect failed: %v", err)
		}

		if len(reflection.Issues) != 1 {
			t.Errorf("Expected 1 issue for flaky step, got %d", len(reflection.Issues))
		}

		if reflection.Issues[0].Severity != IssueSeverityMedium {
			t.Errorf("Expected medium severity, got %s", reflection.Issues[0].Severity)
		}
	})
}

func TestDetermineSeverity(t *testing.T) {
	p := NewPlanner()
	r := NewReflector(p)

	tests := []struct {
		name     string
		attempts int
		expected IssueSeverity
	}{
		{"single attempt", 1, IssueSeverityMedium},
		{"two attempts", 2, IssueSeverityHigh},
		{"three attempts", 3, IssueSeverityCritical},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			step := &Step{
				ID:       "test-step",
				Attempts: tt.attempts,
			}

			severity := r.determineSeverity(step)
			if severity != tt.expected {
				t.Errorf("Expected severity %s, got %s", tt.expected, severity)
			}
		})
	}
}

func TestGenerateSuggestion(t *testing.T) {
	p := NewPlanner()
	r := NewReflector(p)

	tests := []struct {
		name           string
		error          string
		expectedAction string
	}{
		{"timeout error", "connection timeout", "retry"},
		{"network error", "network unreachable", "retry"},
		{"permission error", "permission denied", "modify"},
		{"not found error", "file not found", "modify"},
		{"generic error", "unknown error", "modify"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			step := &Step{
				ID: "test-step",
				Result: &StepResult{
					Success: false,
					Error:   tt.error,
				},
			}

			suggestion := r.generateSuggestion(step)
			if suggestion == nil {
				t.Fatal("Expected suggestion, got nil")
			}

			if suggestion.Action != tt.expectedAction {
				t.Errorf("Expected action %s, got %s", tt.expectedAction, suggestion.Action)
			}
		})
	}
}

func TestShouldRetry(t *testing.T) {
	p := NewPlanner()
	r := NewReflector(p)

	t.Run("no issues - no retry", func(t *testing.T) {
		reflection := &Reflection{
			Issues:      []Issue{},
			Suggestions: []Suggestion{},
		}

		if r.shouldRetry(reflection) {
			t.Error("Should not retry when no issues")
		}
	})

	t.Run("critical issue - no retry", func(t *testing.T) {
		reflection := &Reflection{
			Issues: []Issue{
				{Severity: IssueSeverityCritical},
			},
			Suggestions: []Suggestion{
				{Action: "retry"},
			},
		}

		if r.shouldRetry(reflection) {
			t.Error("Should not retry when critical issue exists")
		}
	})

	t.Run("retry suggestion - should retry", func(t *testing.T) {
		reflection := &Reflection{
			Issues: []Issue{
				{Severity: IssueSeverityMedium},
			},
			Suggestions: []Suggestion{
				{Action: "retry"},
			},
		}

		if !r.shouldRetry(reflection) {
			t.Error("Should retry when retry suggestion exists")
		}
	})

	t.Run("no retry suggestion - no retry", func(t *testing.T) {
		reflection := &Reflection{
			Issues: []Issue{
				{Severity: IssueSeverityMedium},
			},
			Suggestions: []Suggestion{
				{Action: "modify"},
			},
		}

		if r.shouldRetry(reflection) {
			t.Error("Should not retry without retry suggestion")
		}
	})
}

func TestApplyAdjustments(t *testing.T) {
	p := NewPlanner()
	r := NewReflector(p)

	t.Run("apply adjustments", func(t *testing.T) {
		steps := []Step{
			{
				ID:          "step-1",
				Description: "Step 1",
				Status:      StepStatusFailed,
				Attempts:    2,
				Result:      &StepResult{Success: false},
			},
		}

		plan, _ := p.GeneratePlan("Test plan", steps)

		reflection := &Reflection{
			Adjustments: map[string]string{
				"step-1": "increase timeout",
			},
		}

		err := r.ApplyAdjustments(plan, reflection)
		if err != nil {
			t.Fatalf("ApplyAdjustments failed: %v", err)
		}

		// Verify step was reset
		if plan.Steps[0].Status != StepStatusPending {
			t.Errorf("Expected status 'pending', got '%s'", plan.Steps[0].Status)
		}
		if plan.Steps[0].Result != nil {
			t.Error("Expected result to be nil after reset")
		}
		if plan.Steps[0].Attempts != 0 {
			t.Errorf("Expected attempts=0, got %d", plan.Steps[0].Attempts)
		}
	})

	t.Run("apply adjustments to non-existent step", func(t *testing.T) {
		steps := []Step{
			{ID: "step-1", Description: "Step 1"},
		}

		plan, _ := p.GeneratePlan("Test plan", steps)

		reflection := &Reflection{
			Adjustments: map[string]string{
				"step-999": "adjustment",
			},
		}

		err := r.ApplyAdjustments(plan, reflection)
		if err == nil {
			t.Error("Expected error for non-existent step")
		}
	})
}

func TestGetIssuesBySeverity(t *testing.T) {
	p := NewPlanner()
	r := NewReflector(p)

	reflection := &Reflection{
		Issues: []Issue{
			{StepID: "step-1", Severity: IssueSeverityLow},
			{StepID: "step-2", Severity: IssueSeverityHigh},
			{StepID: "step-3", Severity: IssueSeverityLow},
		},
	}

	lowIssues := r.GetIssuesBySeverity(reflection, IssueSeverityLow)
	if len(lowIssues) != 2 {
		t.Errorf("Expected 2 low severity issues, got %d", len(lowIssues))
	}

	highIssues := r.GetIssuesBySeverity(reflection, IssueSeverityHigh)
	if len(highIssues) != 1 {
		t.Errorf("Expected 1 high severity issue, got %d", len(highIssues))
	}
}

func TestGetSuggestionsByAction(t *testing.T) {
	p := NewPlanner()
	r := NewReflector(p)

	reflection := &Reflection{
		Suggestions: []Suggestion{
			{StepID: "step-1", Action: "retry"},
			{StepID: "step-2", Action: "modify"},
			{StepID: "step-3", Action: "retry"},
		},
	}

	retrySuggestions := r.GetSuggestionsByAction(reflection, "retry")
	if len(retrySuggestions) != 2 {
		t.Errorf("Expected 2 retry suggestions, got %d", len(retrySuggestions))
	}

	modifySuggestions := r.GetSuggestionsByAction(reflection, "modify")
	if len(modifySuggestions) != 1 {
		t.Errorf("Expected 1 modify suggestion, got %d", len(modifySuggestions))
	}
}
