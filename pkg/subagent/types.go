package subagent

import "time"

// RunParams contains parameters for registering a subagent run
type RunParams struct {
	ParentSessionKey string                 `json:"parent_session_key"`
	ChildSessionKey  string                 `json:"child_session_key"`
	Prompt           string                 `json:"prompt"`
	Metadata         map[string]interface{} `json:"metadata,omitempty"`
}

// RunRecord represents a subagent execution record
type RunRecord struct {
	ID               string                 `json:"id"`
	ParentSessionKey string                 `json:"parent_session_key"`
	ChildSessionKey  string                 `json:"child_session_key"`
	Prompt           string                 `json:"prompt"`
	Status           RunStatus              `json:"status"`
	StartedAt        int64                  `json:"started_at"`
	CompletedAt      *int64                 `json:"completed_at,omitempty"`
	Result           interface{}            `json:"result,omitempty"`
	Error            string                 `json:"error,omitempty"`
	Metadata         map[string]interface{} `json:"metadata,omitempty"`
}

// RunStatus represents the execution state of a subagent
type RunStatus string

const (
	StatusPending   RunStatus = "pending"
	StatusRunning   RunStatus = "running"
	StatusCompleted RunStatus = "completed"
	StatusFailed    RunStatus = "failed"
	StatusAborted   RunStatus = "aborted"
)

// IsTerminal returns true if the status is terminal
func (s RunStatus) IsTerminal() bool {
	return s == StatusCompleted || s == StatusFailed || s == StatusAborted
}

// LifecycleEvent represents a state change in subagent execution
type LifecycleEvent struct {
	Type       EventType   `json:"type"`
	SessionKey string      `json:"session_key"`
	Timestamp  int64       `json:"timestamp"`
	Data       interface{} `json:"data,omitempty"`
}

// EventType represents the type of lifecycle event
type EventType string

const (
	EventStart EventType = "start"
	EventEnd   EventType = "end"
	EventError EventType = "error"
	EventAbort EventType = "abort"
)

// Registry represents the persistent storage format
type Registry struct {
	Version     int          `json:"version"`
	Runs        []*RunRecord `json:"runs"`
	LastUpdated int64        `json:"last_updated"`
}

// Stats contains coordinator statistics
type Stats struct {
	TotalRuns     int `json:"total_runs"`
	ActiveRuns    int `json:"active_runs"`
	CompletedRuns int `json:"completed_runs"`
	FailedRuns    int `json:"failed_runs"`
	AbortedRuns   int `json:"aborted_runs"`
}

// EventHandler is a function that handles coordinator events
type EventHandler func(event interface{})

// Event names
const (
	EventRunRegistered = "run:registered"
	EventRunUpdated    = "run:updated"
	EventLifecycle     = "lifecycle:event"
)

// NewRegistry creates a new empty registry
func NewRegistry() *Registry {
	return &Registry{
		Version:     1,
		Runs:        []*RunRecord{},
		LastUpdated: time.Now().UnixMilli(),
	}
}
