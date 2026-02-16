package planner

import "time"

// Plan represents a multi-step execution plan
type Plan struct {
	ID          string    `json:"id"`
	Description string    `json:"description"`
	Steps       []Step    `json:"steps"`
	CreatedAt   time.Time `json:"created_at"`
}

// Step represents a single step in a plan
type Step struct {
	ID            string        `json:"id"`
	Description   string        `json:"description"`
	Dependencies  []string      `json:"dependencies"`   // IDs of steps that must complete first
	EstimatedTime time.Duration `json:"estimated_time"` // Estimated execution time
	Status        StepStatus    `json:"status"`
	Result        *StepResult   `json:"result,omitempty"`
	Attempts      int           `json:"attempts"` // Number of execution attempts
}

// StepStatus represents the execution status of a step
type StepStatus string

const (
	StepStatusPending   StepStatus = "pending"
	StepStatusRunning   StepStatus = "running"
	StepStatusCompleted StepStatus = "completed"
	StepStatusFailed    StepStatus = "failed"
	StepStatusSkipped   StepStatus = "skipped"
)

// StepResult represents the result of executing a step
type StepResult struct {
	Success   bool          `json:"success"`
	Output    string        `json:"output"`
	Error     string        `json:"error,omitempty"`
	Duration  time.Duration `json:"duration"`
	Timestamp time.Time     `json:"timestamp"`
}

// JoinStrategy defines how to handle parallel step execution
type JoinStrategy string

const (
	JoinAll   JoinStrategy = "all"   // Wait for all steps to complete
	JoinFirst JoinStrategy = "first" // Return when first step completes
	JoinAny   JoinStrategy = "any"   // Return when any step succeeds
)

// FailureStrategy defines how to handle step failures
type FailureStrategy string

const (
	FailAbort    FailureStrategy = "abort"    // Abort entire plan on failure
	FailContinue FailureStrategy = "continue" // Continue with remaining steps
	FailRetry    FailureStrategy = "retry"    // Retry failed step
)
