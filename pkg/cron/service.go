package cron

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
)

// Service manages cron job scheduling and execution
type Service struct {
	jobs    map[string]*Job
	timers  map[string]*time.Timer
	options ServiceOptions
	mu      sync.RWMutex
	stopped bool
	ctx     context.Context
	cancel  context.CancelFunc
}

// NewService creates a new cron service
func NewService(opts ServiceOptions) (*Service, error) {
	if opts.StorePath == "" {
		return nil, fmt.Errorf("store path is required")
	}
	if opts.EnqueueSystemEvent == nil {
		return nil, fmt.Errorf("enqueue system event callback is required")
	}
	if opts.RunIsolatedAgentJob == nil {
		return nil, fmt.Errorf("run isolated agent job callback is required")
	}
	if opts.RequestHeartbeatNow == nil {
		return nil, fmt.Errorf("request heartbeat now callback is required")
	}
	if opts.OnEvent == nil {
		return nil, fmt.Errorf("on event callback is required")
	}

	ctx, cancel := context.WithCancel(context.Background())

	s := &Service{
		jobs:    make(map[string]*Job),
		timers:  make(map[string]*time.Timer),
		options: opts,
		ctx:     ctx,
		cancel:  cancel,
	}

	// Load jobs from storage
	if err := s.loadJobs(); err != nil {
		log.Warn().Err(err).Msg("Failed to load jobs, starting with empty registry")
	}

	// Schedule all enabled jobs
	s.scheduleAll()

	log.Info().Int("jobCount", len(s.jobs)).Msg("Cron service initialized")

	return s, nil
}

// AddJob creates a new cron job
func (s *Service) AddJob(params AddParams) (*Job, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.stopped {
		return nil, fmt.Errorf("service is stopped")
	}

	// Validate schedule
	nextRunAtMs, err := CalculateNextRun(params.Schedule)
	if err != nil {
		return nil, fmt.Errorf("invalid schedule: %w", err)
	}

	// Validate required fields
	if params.Name == "" {
		return nil, fmt.Errorf("job name is required")
	}

	// Generate unique ID
	jobID := uuid.New().String()

	// Use default agent ID if not provided
	agentID := params.AgentID
	if agentID == "" {
		agentID = s.options.DefaultAgentID
	}

	// Create job
	now := Now()
	job := &Job{
		ID:             jobID,
		AgentID:        agentID,
		Name:           params.Name,
		Description:    params.Description,
		Enabled:        params.Enabled,
		DeleteAfterRun: params.DeleteAfterRun,
		CreatedAtMs:    now,
		UpdatedAtMs:    now,
		Schedule:       params.Schedule,
		SessionTarget:  params.SessionTarget,
		WakeMode:       params.WakeMode,
		Payload:        params.Payload,
		Delivery:       params.Delivery,
		State: JobState{
			NextRunAtMs: Int64Ptr(nextRunAtMs),
		},
	}

	// Store job
	s.jobs[jobID] = job

	// Persist to disk
	if err := s.persist(); err != nil {
		delete(s.jobs, jobID)
		return nil, fmt.Errorf("failed to persist job: %w", err)
	}

	// Schedule if enabled
	if job.Enabled {
		s.scheduleJobLocked(job)
	}

	log.Info().
		Str("jobId", jobID).
		Str("name", job.Name).
		Bool("enabled", job.Enabled).
		Msg("Job created")

	// Emit event
	s.options.OnEvent(Event{
		Action: EventActionAdded,
		JobID:  jobID,
	})

	return job, nil
}

// UpdateJob updates an existing job
func (s *Service) UpdateJob(id string, patch JobPatch) (*Job, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.stopped {
		return nil, fmt.Errorf("service is stopped")
	}

	job, exists := s.jobs[id]
	if !exists {
		return nil, fmt.Errorf("job not found: %s", id)
	}

	// Track if schedule or enabled changed
	scheduleChanged := false
	enabledChanged := false
	oldEnabled := job.Enabled

	// Apply patch
	if patch.Name != nil {
		job.Name = *patch.Name
	}
	if patch.Description != nil {
		job.Description = *patch.Description
	}
	if patch.Enabled != nil {
		job.Enabled = *patch.Enabled
		enabledChanged = oldEnabled != job.Enabled
	}
	if patch.DeleteAfterRun != nil {
		job.DeleteAfterRun = *patch.DeleteAfterRun
	}
	if patch.Schedule != nil {
		job.Schedule = *patch.Schedule
		scheduleChanged = true
	}
	if patch.SessionTarget != nil {
		job.SessionTarget = *patch.SessionTarget
	}
	if patch.WakeMode != nil {
		job.WakeMode = *patch.WakeMode
	}
	if patch.Payload != nil {
		job.Payload = *patch.Payload
	}
	if patch.Delivery != nil {
		job.Delivery = patch.Delivery
	}

	// Update timestamp
	job.UpdatedAtMs = Now()

	// Recalculate next run if schedule changed
	if scheduleChanged {
		nextRunAtMs, err := CalculateNextRun(job.Schedule)
		if err != nil {
			return nil, fmt.Errorf("invalid schedule: %w", err)
		}
		job.State.NextRunAtMs = Int64Ptr(nextRunAtMs)
	}

	// Persist to disk
	if err := s.persist(); err != nil {
		return nil, fmt.Errorf("failed to persist job: %w", err)
	}

	// Reschedule if needed
	if scheduleChanged || enabledChanged {
		s.cancelJobLocked(id)
		if job.Enabled {
			s.scheduleJobLocked(job)
		}
	}

	log.Info().
		Str("jobId", id).
		Str("name", job.Name).
		Bool("scheduleChanged", scheduleChanged).
		Bool("enabledChanged", enabledChanged).
		Msg("Job updated")

	// Emit event
	s.options.OnEvent(Event{
		Action: EventActionUpdated,
		JobID:  id,
	})

	return job, nil
}

// RemoveJob deletes a job
func (s *Service) RemoveJob(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.stopped {
		return fmt.Errorf("service is stopped")
	}

	job, exists := s.jobs[id]
	if !exists {
		return fmt.Errorf("job not found: %s", id)
	}

	// Cancel timer
	s.cancelJobLocked(id)

	// Remove from map
	delete(s.jobs, id)

	// Persist to disk
	if err := s.persist(); err != nil {
		return fmt.Errorf("failed to persist job: %w", err)
	}

	log.Info().
		Str("jobId", id).
		Str("name", job.Name).
		Msg("Job removed")

	// Emit event
	s.options.OnEvent(Event{
		Action: EventActionDeleted,
		JobID:  id,
	})

	return nil
}

// RunJob manually executes a job
func (s *Service) RunJob(id string, mode RunMode) error {
	s.mu.RLock()
	job, exists := s.jobs[id]
	s.mu.RUnlock()

	if !exists {
		return fmt.Errorf("job not found: %s", id)
	}

	// Check enabled flag for "due" mode
	if mode == RunModeDue && !job.Enabled {
		log.Debug().Str("jobId", id).Msg("Skipping disabled job in 'due' mode")
		return nil
	}

	// Execute job
	go s.executeJob(job)

	return nil
}

// ListJobs returns all jobs, optionally filtered
func (s *Service) ListJobs(agentID *string, enabled *bool) []*Job {
	s.mu.RLock()
	defer s.mu.RUnlock()

	jobs := make([]*Job, 0, len(s.jobs))

	for _, job := range s.jobs {
		// Apply filters
		if agentID != nil && job.AgentID != *agentID {
			continue
		}
		if enabled != nil && job.Enabled != *enabled {
			continue
		}

		jobs = append(jobs, job)
	}

	// Sort by creation time
	for i := 0; i < len(jobs)-1; i++ {
		for j := i + 1; j < len(jobs); j++ {
			if jobs[j].CreatedAtMs < jobs[i].CreatedAtMs {
				jobs[i], jobs[j] = jobs[j], jobs[i]
			}
		}
	}

	return jobs
}

// GetJob returns a specific job
func (s *Service) GetJob(id string) *Job {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.jobs[id]
}

// Stop gracefully shuts down the service
func (s *Service) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.stopped {
		return nil
	}

	s.stopped = true
	s.cancel()

	// Cancel all timers
	for id := range s.timers {
		s.cancelJobLocked(id)
	}

	// Persist final state
	if err := s.persist(); err != nil {
		log.Error().Err(err).Msg("Failed to persist state on shutdown")
		return err
	}

	log.Info().Msg("Cron service stopped")

	return nil
}

// scheduleAll schedules all enabled jobs
func (s *Service) scheduleAll() {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, job := range s.jobs {
		if job.Enabled {
			s.scheduleJobLocked(job)
		}
	}
}

// scheduleJobLocked schedules a job (must hold lock)
func (s *Service) scheduleJobLocked(job *Job) {
	if job.State.NextRunAtMs == nil {
		log.Warn().Str("jobId", job.ID).Msg("Cannot schedule job without next run time")
		return
	}

	nextRunAtMs := *job.State.NextRunAtMs
	now := Now()
	delay := nextRunAtMs - now

	// If already past due, execute immediately
	if delay <= 0 {
		delay = 0
	}

	timer := time.AfterFunc(time.Duration(delay)*time.Millisecond, func() {
		s.executeJob(job)
	})

	s.timers[job.ID] = timer

	log.Debug().
		Str("jobId", job.ID).
		Int64("delayMs", delay).
		Time("nextRun", time.UnixMilli(nextRunAtMs)).
		Msg("Job scheduled")
}

// cancelJobLocked cancels a job's timer (must hold lock)
func (s *Service) cancelJobLocked(id string) {
	if timer, exists := s.timers[id]; exists {
		timer.Stop()
		delete(s.timers, id)
		log.Debug().Str("jobId", id).Msg("Job timer cancelled")
	}
}

// executeJob executes a job
func (s *Service) executeJob(job *Job) {
	s.mu.Lock()

	// Check if job still exists
	currentJob, exists := s.jobs[job.ID]
	if !exists {
		s.mu.Unlock()
		log.Debug().Str("jobId", job.ID).Msg("Job no longer exists, skipping execution")
		return
	}

	// Check if already running
	if currentJob.State.RunningAtMs != nil {
		s.mu.Unlock()
		log.Debug().Str("jobId", job.ID).Msg("Job already running, skipping execution")
		return
	}

	// Mark as running
	startMs := Now()
	currentJob.State.RunningAtMs = Int64Ptr(startMs)
	s.mu.Unlock()

	log.Info().Str("jobId", job.ID).Str("name", job.Name).Msg("Executing job")

	// Execute payload
	var err error
	if job.Payload.Kind == PayloadKindSystemEvent {
		s.options.EnqueueSystemEvent(job.Payload.Text, job.AgentID)
	} else if job.Payload.Kind == PayloadKindAgentTurn {
		err = s.options.RunIsolatedAgentJob(job, job.Payload.Message)
	}

	// Handle wake mode
	if job.WakeMode == WakeModeNextHeartbeat {
		s.options.RequestHeartbeatNow()
	}

	// Update state
	s.mu.Lock()
	defer s.mu.Unlock()

	endMs := Now()
	durationMs := endMs - startMs

	currentJob.State.RunningAtMs = nil
	currentJob.State.LastRunAtMs = Int64Ptr(startMs)
	currentJob.State.LastDurationMs = Int64Ptr(durationMs)

	if err != nil {
		currentJob.State.LastStatus = "error"
		currentJob.State.LastError = err.Error()
		currentJob.State.ConsecutiveErrors++

		log.Error().
			Str("jobId", job.ID).
			Err(err).
			Int("consecutiveErrors", currentJob.State.ConsecutiveErrors).
			Msg("Job execution failed")
	} else {
		currentJob.State.LastStatus = "ok"
		currentJob.State.LastError = ""
		currentJob.State.ConsecutiveErrors = 0

		log.Info().
			Str("jobId", job.ID).
			Int64("durationMs", durationMs).
			Msg("Job execution completed")
	}

	// Calculate next run
	nextRunAtMs, calcErr := CalculateNextRun(currentJob.Schedule)
	if calcErr != nil {
		log.Error().Str("jobId", job.ID).Err(calcErr).Msg("Failed to calculate next run")
	} else {
		currentJob.State.NextRunAtMs = Int64Ptr(nextRunAtMs)
	}

	// Persist state
	if persistErr := s.persist(); persistErr != nil {
		log.Error().Err(persistErr).Msg("Failed to persist job state")
	}

	// Emit finished event
	s.options.OnEvent(Event{
		Action:      EventActionFinished,
		JobID:       job.ID,
		Status:      currentJob.State.LastStatus,
		Error:       currentJob.State.LastError,
		DurationMs:  Int64Ptr(durationMs),
		NextRunAtMs: currentJob.State.NextRunAtMs,
	})

	// Handle deleteAfterRun
	if currentJob.DeleteAfterRun && err == nil {
		log.Info().Str("jobId", job.ID).Msg("Deleting job after successful run")
		s.cancelJobLocked(job.ID)
		delete(s.jobs, job.ID)
		if persistErr := s.persist(); persistErr != nil {
			log.Error().Err(persistErr).Msg("Failed to persist after delete")
		}
		s.options.OnEvent(Event{
			Action: EventActionDeleted,
			JobID:  job.ID,
		})
		return
	}

	// Reschedule if enabled and not deleted
	if currentJob.Enabled && calcErr == nil {
		s.scheduleJobLocked(currentJob)
	}
}

// loadJobs loads jobs from storage
func (s *Service) loadJobs() error {
	// Check if file exists
	if _, err := os.Stat(s.options.StorePath); os.IsNotExist(err) {
		log.Info().Msg("No existing job registry, starting with empty registry")
		return nil
	}

	// Read file
	data, err := os.ReadFile(s.options.StorePath)
	if err != nil {
		return fmt.Errorf("failed to read jobs file: %w", err)
	}

	// Parse JSON
	var jobs []*Job
	if err := json.Unmarshal(data, &jobs); err != nil {
		return fmt.Errorf("failed to parse jobs file: %w", err)
	}

	// Load into map
	s.jobs = make(map[string]*Job)
	for _, job := range jobs {
		s.jobs[job.ID] = job
	}

	log.Info().Int("count", len(jobs)).Msg("Loaded jobs from registry")

	return nil
}

// persist saves jobs to storage
func (s *Service) persist() error {
	// Convert map to slice
	jobs := make([]*Job, 0, len(s.jobs))
	for _, job := range s.jobs {
		jobs = append(jobs, job)
	}

	// Marshal to JSON
	data, err := json.MarshalIndent(jobs, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal jobs: %w", err)
	}

	// Ensure directory exists
	dir := filepath.Dir(s.options.StorePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	// Write to temporary file
	tempFile := s.options.StorePath + ".tmp"
	if err := os.WriteFile(tempFile, data, 0644); err != nil {
		return fmt.Errorf("failed to write temp file: %w", err)
	}

	// Atomic rename
	if err := os.Rename(tempFile, s.options.StorePath); err != nil {
		return fmt.Errorf("failed to rename temp file: %w", err)
	}

	log.Debug().Int("count", len(jobs)).Msg("Persisted jobs to registry")

	return nil
}
