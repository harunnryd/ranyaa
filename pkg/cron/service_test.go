package cron

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Test helpers

type mockCallbacks struct {
	systemEvents []string
	agentJobs    []*Job
	heartbeats   int
	events       []Event
}

func newMockCallbacks() *mockCallbacks {
	return &mockCallbacks{
		systemEvents: make([]string, 0),
		agentJobs:    make([]*Job, 0),
		events:       make([]Event, 0),
	}
}

func (m *mockCallbacks) enqueueSystemEvent(text string, agentID string) {
	m.systemEvents = append(m.systemEvents, text)
}

func (m *mockCallbacks) runIsolatedAgentJob(job *Job, message string) error {
	m.agentJobs = append(m.agentJobs, job)
	return nil
}

func (m *mockCallbacks) requestHeartbeatNow() {
	m.heartbeats++
}

func (m *mockCallbacks) onEvent(evt Event) {
	m.events = append(m.events, evt)
}

func createTestService(t *testing.T) (*Service, *mockCallbacks, string) {
	tempDir := t.TempDir()
	storePath := filepath.Join(tempDir, "jobs.json")

	callbacks := newMockCallbacks()

	opts := ServiceOptions{
		StorePath:           storePath,
		CronEnabled:         true,
		DefaultAgentID:      "test-agent",
		EnqueueSystemEvent:  callbacks.enqueueSystemEvent,
		RunIsolatedAgentJob: callbacks.runIsolatedAgentJob,
		RequestHeartbeatNow: callbacks.requestHeartbeatNow,
		OnEvent:             callbacks.onEvent,
	}

	service, err := NewService(opts)
	require.NoError(t, err)

	return service, callbacks, storePath
}

func createTestJob() AddParams {
	return AddParams{
		Name:    "Test Job",
		Enabled: true,
		Schedule: Schedule{
			Kind:    ScheduleKindEvery,
			EveryMs: 60000,
		},
		SessionTarget: SessionTargetIsolated,
		WakeMode:      WakeModeNow,
		Payload: Payload{
			Kind: PayloadKindSystemEvent,
			Text: "test event",
		},
	}
}

// Tests

func TestNewService(t *testing.T) {
	t.Run("creates service successfully", func(t *testing.T) {
		service, _, _ := createTestService(t)
		assert.NotNil(t, service)
	})

	t.Run("requires store path", func(t *testing.T) {
		opts := ServiceOptions{
			EnqueueSystemEvent:  func(string, string) {},
			RunIsolatedAgentJob: func(*Job, string) error { return nil },
			RequestHeartbeatNow: func() {},
			OnEvent:             func(Event) {},
		}

		_, err := NewService(opts)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "store path")
	})

	t.Run("requires callbacks", func(t *testing.T) {
		tempDir := t.TempDir()

		opts := ServiceOptions{
			StorePath: filepath.Join(tempDir, "jobs.json"),
		}

		_, err := NewService(opts)
		assert.Error(t, err)
	})
}

func TestAddJob(t *testing.T) {
	t.Run("creates job with unique ID", func(t *testing.T) {
		service, callbacks, _ := createTestService(t)
		defer func() { _ = service.Stop() }()

		params := createTestJob()
		job, err := service.AddJob(params)
		require.NoError(t, err)

		assert.NotEmpty(t, job.ID)
		assert.Equal(t, params.Name, job.Name)
		assert.Equal(t, params.Enabled, job.Enabled)

		// Check event emitted
		assert.Len(t, callbacks.events, 1)
		assert.Equal(t, EventActionAdded, callbacks.events[0].Action)
	})

	t.Run("sets creation timestamp", func(t *testing.T) {
		service, _, _ := createTestService(t)
		defer func() { _ = service.Stop() }()

		before := Now()
		job, err := service.AddJob(createTestJob())
		require.NoError(t, err)
		after := Now()

		assert.GreaterOrEqual(t, job.CreatedAtMs, before)
		assert.LessOrEqual(t, job.CreatedAtMs, after)
		assert.Equal(t, job.CreatedAtMs, job.UpdatedAtMs)
	})

	t.Run("calculates next run time", func(t *testing.T) {
		service, _, _ := createTestService(t)
		defer func() { _ = service.Stop() }()

		params := createTestJob()
		params.Schedule = Schedule{
			Kind:    ScheduleKindEvery,
			EveryMs: 60000,
		}

		job, err := service.AddJob(params)
		require.NoError(t, err)

		assert.NotNil(t, job.State.NextRunAtMs)
		assert.Greater(t, *job.State.NextRunAtMs, Now())
	})

	t.Run("uses default agent ID", func(t *testing.T) {
		service, _, _ := createTestService(t)
		defer func() { _ = service.Stop() }()

		params := createTestJob()
		params.AgentID = ""

		job, err := service.AddJob(params)
		require.NoError(t, err)

		assert.Equal(t, "test-agent", job.AgentID)
	})

	t.Run("validates schedule", func(t *testing.T) {
		service, _, _ := createTestService(t)
		defer func() { _ = service.Stop() }()

		params := createTestJob()
		params.Schedule = Schedule{
			Kind: ScheduleKindAt,
			At:   "invalid",
		}

		_, err := service.AddJob(params)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid schedule")
	})

	t.Run("requires job name", func(t *testing.T) {
		service, _, _ := createTestService(t)
		defer func() { _ = service.Stop() }()

		params := createTestJob()
		params.Name = ""

		_, err := service.AddJob(params)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "name is required")
	})

	t.Run("persists job to disk", func(t *testing.T) {
		service, _, storePath := createTestService(t)
		defer func() { _ = service.Stop() }()

		_, err := service.AddJob(createTestJob())
		require.NoError(t, err)

		// Check file exists
		_, err = os.Stat(storePath)
		assert.NoError(t, err)
	})

	t.Run("schedules enabled job", func(t *testing.T) {
		service, _, _ := createTestService(t)
		defer func() { _ = service.Stop() }()

		params := createTestJob()
		params.Enabled = true

		job, err := service.AddJob(params)
		require.NoError(t, err)

		// Check timer exists
		service.mu.RLock()
		_, exists := service.timers[job.ID]
		service.mu.RUnlock()

		assert.True(t, exists)
	})

	t.Run("does not schedule disabled job", func(t *testing.T) {
		service, _, _ := createTestService(t)
		defer func() { _ = service.Stop() }()

		params := createTestJob()
		params.Enabled = false

		job, err := service.AddJob(params)
		require.NoError(t, err)

		// Check timer does not exist
		service.mu.RLock()
		_, exists := service.timers[job.ID]
		service.mu.RUnlock()

		assert.False(t, exists)
	})
}

func TestUpdateJob(t *testing.T) {
	t.Run("updates job fields", func(t *testing.T) {
		service, _, _ := createTestService(t)
		defer service.Stop()

		job, err := service.AddJob(createTestJob())
		require.NoError(t, err)

		// Wait a bit to ensure timestamp difference
		time.Sleep(10 * time.Millisecond)

		newName := "Updated Job"
		patch := JobPatch{
			Name: &newName,
		}

		updated, err := service.UpdateJob(job.ID, patch)
		require.NoError(t, err)

		assert.Equal(t, newName, updated.Name)
		assert.GreaterOrEqual(t, updated.UpdatedAtMs, job.UpdatedAtMs)
	})

	t.Run("recalculates next run on schedule change", func(t *testing.T) {
		service, _, _ := createTestService(t)
		defer service.Stop()

		job, err := service.AddJob(createTestJob())
		require.NoError(t, err)

		oldNextRun := *job.State.NextRunAtMs

		newSchedule := Schedule{
			Kind:    ScheduleKindEvery,
			EveryMs: 120000,
		}
		patch := JobPatch{
			Schedule: &newSchedule,
		}

		updated, err := service.UpdateJob(job.ID, patch)
		require.NoError(t, err)

		assert.NotEqual(t, oldNextRun, *updated.State.NextRunAtMs)
	})

	t.Run("reschedules on enabled change", func(t *testing.T) {
		service, _, _ := createTestService(t)
		defer service.Stop()

		params := createTestJob()
		params.Enabled = false

		job, err := service.AddJob(params)
		require.NoError(t, err)

		// Enable job
		enabled := true
		patch := JobPatch{
			Enabled: &enabled,
		}

		_, err = service.UpdateJob(job.ID, patch)
		require.NoError(t, err)

		// Check timer exists
		service.mu.RLock()
		_, exists := service.timers[job.ID]
		service.mu.RUnlock()

		assert.True(t, exists)
	})

	t.Run("cancels timer on disable", func(t *testing.T) {
		service, _, _ := createTestService(t)
		defer service.Stop()

		job, err := service.AddJob(createTestJob())
		require.NoError(t, err)

		// Disable job
		enabled := false
		patch := JobPatch{
			Enabled: &enabled,
		}

		_, err = service.UpdateJob(job.ID, patch)
		require.NoError(t, err)

		// Check timer does not exist
		service.mu.RLock()
		_, exists := service.timers[job.ID]
		service.mu.RUnlock()

		assert.False(t, exists)
	})

	t.Run("returns error for non-existent job", func(t *testing.T) {
		service, _, _ := createTestService(t)
		defer service.Stop()

		patch := JobPatch{}
		_, err := service.UpdateJob("non-existent", patch)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})

	t.Run("emits updated event", func(t *testing.T) {
		service, callbacks, _ := createTestService(t)
		defer service.Stop()

		job, err := service.AddJob(createTestJob())
		require.NoError(t, err)

		// Clear events
		callbacks.events = nil

		newName := "Updated"
		patch := JobPatch{Name: &newName}
		_, err = service.UpdateJob(job.ID, patch)
		require.NoError(t, err)

		assert.Len(t, callbacks.events, 1)
		assert.Equal(t, EventActionUpdated, callbacks.events[0].Action)
	})
}

func TestRemoveJob(t *testing.T) {
	t.Run("removes job", func(t *testing.T) {
		service, _, _ := createTestService(t)
		defer service.Stop()

		job, err := service.AddJob(createTestJob())
		require.NoError(t, err)

		err = service.RemoveJob(job.ID)
		require.NoError(t, err)

		// Check job no longer exists
		retrieved := service.GetJob(job.ID)
		assert.Nil(t, retrieved)
	})

	t.Run("cancels timer", func(t *testing.T) {
		service, _, _ := createTestService(t)
		defer service.Stop()

		job, err := service.AddJob(createTestJob())
		require.NoError(t, err)

		err = service.RemoveJob(job.ID)
		require.NoError(t, err)

		// Check timer does not exist
		service.mu.RLock()
		_, exists := service.timers[job.ID]
		service.mu.RUnlock()

		assert.False(t, exists)
	})

	t.Run("persists change", func(t *testing.T) {
		service, _, storePath := createTestService(t)
		defer service.Stop()

		job, err := service.AddJob(createTestJob())
		require.NoError(t, err)

		err = service.RemoveJob(job.ID)
		require.NoError(t, err)

		// Load jobs from disk
		data, err := os.ReadFile(storePath)
		require.NoError(t, err)

		assert.NotContains(t, string(data), job.ID)
	})

	t.Run("returns error for non-existent job", func(t *testing.T) {
		service, _, _ := createTestService(t)
		defer service.Stop()

		err := service.RemoveJob("non-existent")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})

	t.Run("emits deleted event", func(t *testing.T) {
		service, callbacks, _ := createTestService(t)
		defer service.Stop()

		job, err := service.AddJob(createTestJob())
		require.NoError(t, err)

		// Clear events
		callbacks.events = nil

		err = service.RemoveJob(job.ID)
		require.NoError(t, err)

		assert.Len(t, callbacks.events, 1)
		assert.Equal(t, EventActionDeleted, callbacks.events[0].Action)
	})
}

func TestRunJob(t *testing.T) {
	t.Run("executes system event payload", func(t *testing.T) {
		service, callbacks, _ := createTestService(t)
		defer service.Stop()

		params := createTestJob()
		params.Payload = Payload{
			Kind: PayloadKindSystemEvent,
			Text: "test event",
		}

		job, err := service.AddJob(params)
		require.NoError(t, err)

		err = service.RunJob(job.ID, RunModeForce)
		require.NoError(t, err)

		// Wait for execution
		time.Sleep(100 * time.Millisecond)

		assert.Len(t, callbacks.systemEvents, 1)
		assert.Equal(t, "test event", callbacks.systemEvents[0])
	})

	t.Run("executes agent turn payload", func(t *testing.T) {
		service, callbacks, _ := createTestService(t)
		defer service.Stop()

		params := createTestJob()
		params.Payload = Payload{
			Kind:    PayloadKindAgentTurn,
			Message: "test message",
		}

		job, err := service.AddJob(params)
		require.NoError(t, err)

		err = service.RunJob(job.ID, RunModeForce)
		require.NoError(t, err)

		// Wait for execution
		time.Sleep(100 * time.Millisecond)

		assert.Len(t, callbacks.agentJobs, 1)
		assert.Equal(t, job.ID, callbacks.agentJobs[0].ID)
	})

	t.Run("respects enabled flag in due mode", func(t *testing.T) {
		service, callbacks, _ := createTestService(t)
		defer service.Stop()

		params := createTestJob()
		params.Enabled = false

		job, err := service.AddJob(params)
		require.NoError(t, err)

		err = service.RunJob(job.ID, RunModeDue)
		require.NoError(t, err)

		// Wait
		time.Sleep(100 * time.Millisecond)

		// Should not execute
		assert.Len(t, callbacks.systemEvents, 0)
	})

	t.Run("ignores enabled flag in force mode", func(t *testing.T) {
		service, callbacks, _ := createTestService(t)
		defer service.Stop()

		params := createTestJob()
		params.Enabled = false

		job, err := service.AddJob(params)
		require.NoError(t, err)

		err = service.RunJob(job.ID, RunModeForce)
		require.NoError(t, err)

		// Wait for execution
		time.Sleep(100 * time.Millisecond)

		// Should execute
		assert.Len(t, callbacks.systemEvents, 1)
	})

	t.Run("returns error for non-existent job", func(t *testing.T) {
		service, _, _ := createTestService(t)
		defer service.Stop()

		err := service.RunJob("non-existent", RunModeForce)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})
}

func TestListJobs(t *testing.T) {
	t.Run("returns all jobs", func(t *testing.T) {
		service, _, _ := createTestService(t)
		defer service.Stop()

		_, err := service.AddJob(createTestJob())
		require.NoError(t, err)

		_, err = service.AddJob(createTestJob())
		require.NoError(t, err)

		jobs := service.ListJobs(nil, nil)
		assert.Len(t, jobs, 2)
	})

	t.Run("filters by agent ID", func(t *testing.T) {
		service, _, _ := createTestService(t)
		defer service.Stop()

		params1 := createTestJob()
		params1.AgentID = "agent1"
		_, err := service.AddJob(params1)
		require.NoError(t, err)

		params2 := createTestJob()
		params2.AgentID = "agent2"
		_, err = service.AddJob(params2)
		require.NoError(t, err)

		agentID := "agent1"
		jobs := service.ListJobs(&agentID, nil)
		assert.Len(t, jobs, 1)
		assert.Equal(t, "agent1", jobs[0].AgentID)
	})

	t.Run("filters by enabled", func(t *testing.T) {
		service, _, _ := createTestService(t)
		defer service.Stop()

		params1 := createTestJob()
		params1.Enabled = true
		_, err := service.AddJob(params1)
		require.NoError(t, err)

		params2 := createTestJob()
		params2.Enabled = false
		_, err = service.AddJob(params2)
		require.NoError(t, err)

		enabled := true
		jobs := service.ListJobs(nil, &enabled)
		assert.Len(t, jobs, 1)
		assert.True(t, jobs[0].Enabled)
	})

	t.Run("sorts by creation time", func(t *testing.T) {
		service, _, _ := createTestService(t)
		defer service.Stop()

		job1, err := service.AddJob(createTestJob())
		require.NoError(t, err)

		time.Sleep(10 * time.Millisecond)

		job2, err := service.AddJob(createTestJob())
		require.NoError(t, err)

		jobs := service.ListJobs(nil, nil)
		assert.Len(t, jobs, 2)
		assert.Equal(t, job1.ID, jobs[0].ID)
		assert.Equal(t, job2.ID, jobs[1].ID)
	})

	t.Run("returns empty array when no jobs", func(t *testing.T) {
		service, _, _ := createTestService(t)
		defer service.Stop()

		jobs := service.ListJobs(nil, nil)
		assert.Len(t, jobs, 0)
	})
}

func TestGetJob(t *testing.T) {
	t.Run("returns job by ID", func(t *testing.T) {
		service, _, _ := createTestService(t)
		defer service.Stop()

		job, err := service.AddJob(createTestJob())
		require.NoError(t, err)

		retrieved := service.GetJob(job.ID)
		assert.NotNil(t, retrieved)
		assert.Equal(t, job.ID, retrieved.ID)
	})

	t.Run("returns nil for non-existent job", func(t *testing.T) {
		service, _, _ := createTestService(t)
		defer service.Stop()

		retrieved := service.GetJob("non-existent")
		assert.Nil(t, retrieved)
	})
}

func TestPersistence(t *testing.T) {
	t.Run("loads jobs on startup", func(t *testing.T) {
		tempDir := t.TempDir()
		storePath := filepath.Join(tempDir, "jobs.json")

		// Create first service with explicit store path
		callbacks1 := newMockCallbacks()
		opts1 := ServiceOptions{
			StorePath:           storePath,
			CronEnabled:         true,
			DefaultAgentID:      "test-agent",
			EnqueueSystemEvent:  callbacks1.enqueueSystemEvent,
			RunIsolatedAgentJob: callbacks1.runIsolatedAgentJob,
			RequestHeartbeatNow: callbacks1.requestHeartbeatNow,
			OnEvent:             callbacks1.onEvent,
		}

		service1, err := NewService(opts1)
		require.NoError(t, err)

		job, err := service1.AddJob(createTestJob())
		require.NoError(t, err)
		_ = service1.Stop()

		// Create second service with same store path
		callbacks2 := newMockCallbacks()
		opts2 := ServiceOptions{
			StorePath:           storePath,
			CronEnabled:         true,
			DefaultAgentID:      "test-agent",
			EnqueueSystemEvent:  callbacks2.enqueueSystemEvent,
			RunIsolatedAgentJob: callbacks2.runIsolatedAgentJob,
			RequestHeartbeatNow: callbacks2.requestHeartbeatNow,
			OnEvent:             callbacks2.onEvent,
		}

		service2, err := NewService(opts2)
		require.NoError(t, err)
		defer func() { _ = service2.Stop() }()

		// Check job was loaded
		retrieved := service2.GetJob(job.ID)
		assert.NotNil(t, retrieved)
		assert.Equal(t, job.Name, retrieved.Name)
	})

	t.Run("handles missing file gracefully", func(t *testing.T) {
		service, _, _ := createTestService(t)
		defer service.Stop()

		// Should start with empty registry
		jobs := service.ListJobs(nil, nil)
		assert.Len(t, jobs, 0)
	})

	t.Run("uses atomic file replacement", func(t *testing.T) {
		service, _, storePath := createTestService(t)
		defer service.Stop()

		_, err := service.AddJob(createTestJob())
		require.NoError(t, err)

		// Check temp file doesn't exist
		tempFile := storePath + ".tmp"
		_, err = os.Stat(tempFile)
		assert.True(t, os.IsNotExist(err))

		// Check main file exists
		_, err = os.Stat(storePath)
		assert.NoError(t, err)
	})
}

func TestStop(t *testing.T) {
	t.Run("cancels all timers", func(t *testing.T) {
		service, _, _ := createTestService(t)

		_, err := service.AddJob(createTestJob())
		require.NoError(t, err)

		_, err = service.AddJob(createTestJob())
		require.NoError(t, err)

		err = service.Stop()
		require.NoError(t, err)

		// Check all timers cancelled
		service.mu.RLock()
		timerCount := len(service.timers)
		service.mu.RUnlock()

		assert.Equal(t, 0, timerCount)
	})

	t.Run("persists state", func(t *testing.T) {
		service, _, storePath := createTestService(t)

		job, err := service.AddJob(createTestJob())
		require.NoError(t, err)

		err = service.Stop()
		require.NoError(t, err)

		// Check file contains job
		data, err := os.ReadFile(storePath)
		require.NoError(t, err)

		assert.Contains(t, string(data), job.ID)
	})

	t.Run("prevents new operations", func(t *testing.T) {
		service, _, _ := createTestService(t)

		err := service.Stop()
		require.NoError(t, err)

		// Try to add job
		_, err = service.AddJob(createTestJob())
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "stopped")
	})
}

func TestCalculateRetryBackoff(t *testing.T) {
	t.Run("returns zero for non-error state", func(t *testing.T) {
		assert.Equal(t, time.Duration(0), calculateRetryBackoff(Schedule{Kind: ScheduleKindEvery}, 0))
	})

	t.Run("returns zero for one-shot schedules", func(t *testing.T) {
		assert.Equal(t, time.Duration(0), calculateRetryBackoff(Schedule{Kind: ScheduleKindAt}, 3))
	})

	t.Run("uses expected progression and caps at max", func(t *testing.T) {
		schedule := Schedule{Kind: ScheduleKindEvery}
		assert.Equal(t, 30*time.Second, calculateRetryBackoff(schedule, 1))
		assert.Equal(t, 1*time.Minute, calculateRetryBackoff(schedule, 2))
		assert.Equal(t, 5*time.Minute, calculateRetryBackoff(schedule, 3))
		assert.Equal(t, 15*time.Minute, calculateRetryBackoff(schedule, 4))
		assert.Equal(t, 60*time.Minute, calculateRetryBackoff(schedule, 5))
		assert.Equal(t, 60*time.Minute, calculateRetryBackoff(schedule, 8))
	})
}

func TestRunJobAppliesRetryBackoffAndResetsOnSuccess(t *testing.T) {
	tempDir := t.TempDir()
	storePath := filepath.Join(tempDir, "jobs.json")
	callbacks := newMockCallbacks()

	var failCount int
	service, err := NewService(ServiceOptions{
		StorePath:          storePath,
		CronEnabled:        true,
		DefaultAgentID:     "test-agent",
		EnqueueSystemEvent: callbacks.enqueueSystemEvent,
		RunIsolatedAgentJob: func(_ *Job, _ string) error {
			if failCount < 1 {
				failCount++
				return fmt.Errorf("injected failure")
			}
			return nil
		},
		RequestHeartbeatNow: callbacks.requestHeartbeatNow,
		OnEvent:             callbacks.onEvent,
	})
	require.NoError(t, err)
	defer service.Stop()

	job, err := service.AddJob(AddParams{
		Name:    "backoff-test",
		Enabled: true,
		Schedule: Schedule{
			Kind:    ScheduleKindEvery,
			EveryMs: 100,
		},
		SessionTarget: SessionTargetIsolated,
		WakeMode:      WakeModeNow,
		Payload: Payload{
			Kind:    PayloadKindAgentTurn,
			Message: "run",
		},
	})
	require.NoError(t, err)

	require.NoError(t, service.RunJob(job.ID, RunModeForce))
	waitForStatus(t, service, job.ID, "error")

	stateAfterError := service.GetJob(job.ID).State
	require.NotNil(t, stateAfterError.LastRunAtMs)
	require.NotNil(t, stateAfterError.NextRunAtMs)
	assert.Equal(t, 1, stateAfterError.ConsecutiveErrors)
	assert.GreaterOrEqual(
		t,
		*stateAfterError.NextRunAtMs-*stateAfterError.LastRunAtMs,
		int64((30*time.Second)/time.Millisecond),
	)

	require.NoError(t, service.RunJob(job.ID, RunModeForce))
	waitForStatus(t, service, job.ID, "ok")

	stateAfterSuccess := service.GetJob(job.ID).State
	assert.Equal(t, 0, stateAfterSuccess.ConsecutiveErrors)
}

func waitForStatus(t *testing.T, service *Service, jobID string, status string) {
	t.Helper()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		job := service.GetJob(jobID)
		if job != nil && job.State.LastStatus == status && job.State.RunningAtMs == nil {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}

	t.Fatalf("timed out waiting for job %s status %s", jobID, status)
}
