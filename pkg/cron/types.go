package cron

import "time"

// ScheduleKind represents the type of schedule
type ScheduleKind string

const (
	ScheduleKindAt    ScheduleKind = "at"
	ScheduleKindEvery ScheduleKind = "every"
	ScheduleKindCron  ScheduleKind = "cron"
)

// Schedule represents a time specification for job execution
type Schedule struct {
	Kind ScheduleKind `json:"kind"`

	// For "at" schedule
	At string `json:"at,omitempty"` // ISO 8601 timestamp

	// For "every" schedule
	EveryMs  int64  `json:"everyMs,omitempty"`  // Interval in milliseconds
	AnchorMs *int64 `json:"anchorMs,omitempty"` // Optional anchor point

	// For "cron" schedule
	Expr string `json:"expr,omitempty"` // Cron expression (5-field format)
	TZ   string `json:"tz,omitempty"`   // Optional timezone
}

// PayloadKind represents the type of payload
type PayloadKind string

const (
	PayloadKindSystemEvent PayloadKind = "systemEvent"
	PayloadKindAgentTurn   PayloadKind = "agentTurn"
)

// Payload represents the action to perform when a job executes
type Payload struct {
	Kind PayloadKind `json:"kind"`

	// For "systemEvent" payload
	Text string `json:"text,omitempty"`

	// For "agentTurn" payload
	Message                    string `json:"message,omitempty"`
	Model                      string `json:"model,omitempty"`
	Thinking                   string `json:"thinking,omitempty"`
	TimeoutSeconds             int    `json:"timeoutSeconds,omitempty"`
	AllowUnsafeExternalContent bool   `json:"allowUnsafeExternalContent,omitempty"`
	Deliver                    bool   `json:"deliver,omitempty"`
	Channel                    string `json:"channel,omitempty"`
	To                         string `json:"to,omitempty"`
	BestEffortDeliver          bool   `json:"bestEffortDeliver,omitempty"`
}

// JobState tracks runtime state of a job
type JobState struct {
	NextRunAtMs       *int64 `json:"nextRunAtMs,omitempty"`       // When to run next
	RunningAtMs       *int64 `json:"runningAtMs,omitempty"`       // When started (if running)
	LastRunAtMs       *int64 `json:"lastRunAtMs,omitempty"`       // When last executed
	LastStatus        string `json:"lastStatus,omitempty"`        // "ok", "error", or "skipped"
	LastError         string `json:"lastError,omitempty"`         // Last error message
	LastDurationMs    *int64 `json:"lastDurationMs,omitempty"`    // Last execution duration
	ConsecutiveErrors int    `json:"consecutiveErrors,omitempty"` // Sequential failure count
}

// Delivery configures result delivery for agent turns
type Delivery struct {
	Channel           string `json:"channel,omitempty"`
	To                string `json:"to,omitempty"`
	BestEffortDeliver bool   `json:"bestEffortDeliver,omitempty"`
}

// SessionTarget specifies the session context for job execution
type SessionTarget string

const (
	SessionTargetMain     SessionTarget = "main"
	SessionTargetIsolated SessionTarget = "isolated"
)

// WakeMode specifies when to trigger execution
type WakeMode string

const (
	WakeModeNow           WakeMode = "now"
	WakeModeNextHeartbeat WakeMode = "next-heartbeat"
)

// Job represents a complete cron job definition
type Job struct {
	ID             string        `json:"id"`
	AgentID        string        `json:"agentId,omitempty"`
	Name           string        `json:"name"`
	Description    string        `json:"description,omitempty"`
	Enabled        bool          `json:"enabled"`
	DeleteAfterRun bool          `json:"deleteAfterRun,omitempty"`
	CreatedAtMs    int64         `json:"createdAtMs"`
	UpdatedAtMs    int64         `json:"updatedAtMs"`
	Schedule       Schedule      `json:"schedule"`
	SessionTarget  SessionTarget `json:"sessionTarget"`
	WakeMode       WakeMode      `json:"wakeMode"`
	Payload        Payload       `json:"payload"`
	Delivery       *Delivery     `json:"delivery,omitempty"`
	State          JobState      `json:"state"`
}

// AddParams contains parameters for creating a job
type AddParams struct {
	AgentID        string        `json:"agentId,omitempty"`
	Name           string        `json:"name"`
	Description    string        `json:"description,omitempty"`
	Enabled        bool          `json:"enabled"`
	DeleteAfterRun bool          `json:"deleteAfterRun,omitempty"`
	Schedule       Schedule      `json:"schedule"`
	SessionTarget  SessionTarget `json:"sessionTarget"`
	WakeMode       WakeMode      `json:"wakeMode"`
	Payload        Payload       `json:"payload"`
	Delivery       *Delivery     `json:"delivery,omitempty"`
}

// JobPatch contains fields that can be updated
type JobPatch struct {
	Name           *string        `json:"name,omitempty"`
	Description    *string        `json:"description,omitempty"`
	Enabled        *bool          `json:"enabled,omitempty"`
	DeleteAfterRun *bool          `json:"deleteAfterRun,omitempty"`
	Schedule       *Schedule      `json:"schedule,omitempty"`
	SessionTarget  *SessionTarget `json:"sessionTarget,omitempty"`
	WakeMode       *WakeMode      `json:"wakeMode,omitempty"`
	Payload        *Payload       `json:"payload,omitempty"`
	Delivery       *Delivery      `json:"delivery,omitempty"`
}

// EventAction represents the type of event
type EventAction string

const (
	EventActionFinished EventAction = "finished"
	EventActionAdded    EventAction = "added"
	EventActionUpdated  EventAction = "updated"
	EventActionDeleted  EventAction = "deleted"
)

// Event represents a cron system event
type Event struct {
	Action      EventAction `json:"action"`
	JobID       string      `json:"jobId"`
	Status      string      `json:"status,omitempty"`      // "ok", "error", or "skipped"
	Error       string      `json:"error,omitempty"`       // Error message if failed
	DurationMs  *int64      `json:"durationMs,omitempty"`  // Execution duration
	NextRunAtMs *int64      `json:"nextRunAtMs,omitempty"` // Next scheduled run
}

// RunMode specifies how to run a job manually
type RunMode string

const (
	RunModeDue   RunMode = "due"
	RunModeForce RunMode = "force"
)

// ServiceOptions configures the cron service
type ServiceOptions struct {
	StorePath           string                               // Path to jobs.json
	CronEnabled         bool                                 // Master enable flag
	DefaultAgentID      string                               // Default agent ID
	EnqueueSystemEvent  func(text string, agentID string)    // Callback for system events
	RunIsolatedAgentJob func(job *Job, message string) error // Callback for agent turns
	RequestHeartbeatNow func()                               // Callback for heartbeat requests
	OnEvent             func(evt Event)                      // Event callback
}

// IsolatedAgentJobParams contains parameters for isolated agent job execution
type IsolatedAgentJobParams struct {
	Job     *Job
	Message string
}

// Now returns current time in milliseconds
func Now() int64 {
	return time.Now().UnixMilli()
}

// Int64Ptr returns a pointer to an int64 value
func Int64Ptr(v int64) *int64 {
	return &v
}

// StringPtr returns a pointer to a string value
func StringPtr(v string) *string {
	return &v
}

// BoolPtr returns a pointer to a bool value
func BoolPtr(v bool) *bool {
	return &v
}
