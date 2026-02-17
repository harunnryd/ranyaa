package commandqueue

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/harun/ranya/internal/observability"
	"github.com/harun/ranya/internal/tracing"
	"github.com/rs/zerolog/log"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
)

// Task represents an asynchronous operation to be executed
type Task func(ctx context.Context) (interface{}, error)

// TaskOptions provides configuration for task execution
type TaskOptions struct {
	WarnAfterMs int
	OnWait      func(waitMs int64, queuePos int)
}

// taskRecord tracks a task's execution state
type taskRecord struct {
	id         string
	task       Task
	ctx        context.Context
	generation int
	enqueuedAt time.Time
	options    TaskOptions
	result     chan taskResult
}

type taskResult struct {
	value interface{}
	err   error
}

// laneState manages execution state for a single lane
type laneState struct {
	generation  int
	concurrency int
	queue       []*taskRecord
	running     int
	activeIDs   map[string]bool
	mu          sync.Mutex
}

// EventHandler is a function that handles queue events
type EventHandler func(event Event)

// Event represents a queue event
type Event struct {
	Type     string                 // "enqueued" or "completed"
	Lane     string                 // Lane name
	TaskID   string                 // Task ID
	Data     map[string]interface{} // Additional event data
	Metadata map[string]interface{} // Event metadata
}

// CommandQueue provides lane-based task serialization with concurrency control
type CommandQueue struct {
	lanes     map[string]*laneState
	taskIDSeq int
	mu        sync.RWMutex
	wg        sync.WaitGroup
	ctx       context.Context
	cancel    context.CancelFunc
	// Event handling
	eventHandlers map[string][]EventHandler
	eventMu       sync.RWMutex
}

// New creates a new CommandQueue with default lanes
func New() *CommandQueue {
	observability.EnsureRegistered()

	ctx, cancel := context.WithCancel(context.Background())

	cq := &CommandQueue{
		lanes:         make(map[string]*laneState),
		ctx:           ctx,
		cancel:        cancel,
		eventHandlers: make(map[string][]EventHandler),
	}

	// Initialize default lanes
	cq.initLane("main", 1)
	cq.initLane("cron", 5)

	return cq
}

// initLane initializes a lane with specified concurrency
func (cq *CommandQueue) initLane(lane string, concurrency int) {
	cq.mu.Lock()
	defer cq.mu.Unlock()

	if _, exists := cq.lanes[lane]; !exists {
		cq.lanes[lane] = &laneState{
			generation:  0,
			concurrency: concurrency,
			queue:       make([]*taskRecord, 0),
			activeIDs:   make(map[string]bool),
		}
		log.Debug().Str("lane", lane).Int("concurrency", concurrency).Msg("Lane initialized")
	}
}

// Enqueue adds a task to the specified lane
func (cq *CommandQueue) Enqueue(lane string, task Task, options *TaskOptions) (interface{}, error) {
	return cq.EnqueueWithContext(context.Background(), lane, task, options)
}

// EnqueueWithContext adds a task to the specified lane and propagates context metadata.
func (cq *CommandQueue) EnqueueWithContext(ctx context.Context, lane string, task Task, options *TaskOptions) (interface{}, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	ctx, span := tracing.StartSpan(
		ctx,
		"ranya.commandqueue",
		"commandqueue.enqueue",
		attribute.String("lane", lane),
	)
	defer span.End()

	if tracing.GetSessionKey(ctx) == "" {
		ctx = tracing.WithSessionKey(ctx, lane)
	}

	logger := tracing.LoggerFromContext(ctx, log.Logger).With().Str("session_key", lane).Logger()

	// Ensure lane exists
	cq.ensureLane(lane)

	// Create task record
	cq.mu.Lock()
	cq.taskIDSeq++
	taskID := fmt.Sprintf("%s-%d", lane, cq.taskIDSeq)
	cq.mu.Unlock()

	opts := TaskOptions{}
	if options != nil {
		opts = *options
	}

	record := &taskRecord{
		id:         taskID,
		task:       task,
		ctx:        ctx,
		generation: cq.lanes[lane].generation,
		enqueuedAt: time.Now(),
		options:    opts,
		result:     make(chan taskResult, 1),
	}

	// Add to lane queue
	ls := cq.lanes[lane]
	ls.mu.Lock()
	ls.queue = append(ls.queue, record)
	queueSize := len(ls.queue)
	ls.mu.Unlock()

	logger.Debug().
		Str("lane", lane).
		Str("taskId", taskID).
		Int("queueSize", queueSize).
		Msg("Task enqueued")

	observability.RecordQueueEnqueue(lane, queueSize)

	// Emit enqueued event (synchronous)
	cq.emit(Event{
		Type:   "enqueued",
		Lane:   lane,
		TaskID: taskID,
		Data: map[string]interface{}{
			"queueSize": queueSize,
		},
	})

	// Start warning timer if configured
	if opts.WarnAfterMs > 0 {
		go cq.startWarnTimer(record, lane)
	}

	// Process queue
	go cq.processLane(lane)

	// Wait for result
	result := <-record.result
	if result.err != nil {
		span.RecordError(result.err)
		span.SetStatus(codes.Error, result.err.Error())
	}
	return result.value, result.err
}

// ensureLane creates a lane if it doesn't exist
func (cq *CommandQueue) ensureLane(lane string) {
	cq.mu.RLock()
	_, exists := cq.lanes[lane]
	cq.mu.RUnlock()

	if !exists {
		cq.initLane(lane, 1)
	}
}

// processLane processes queued tasks for a lane
func (cq *CommandQueue) processLane(lane string) {
	ls := cq.lanes[lane]
	ls.mu.Lock()
	defer ls.mu.Unlock()

	// Process tasks while we have capacity and queued tasks
	for ls.running < ls.concurrency && len(ls.queue) > 0 {
		record := ls.queue[0]
		ls.queue = ls.queue[1:]

		// Check if task is stale (from previous generation)
		if record.generation != ls.generation {
			record.result <- taskResult{
				err: fmt.Errorf("task cancelled due to restart"),
			}
			continue
		}

		// Mark as running
		ls.running++
		ls.activeIDs[record.id] = true

		logger := tracing.LoggerFromContext(record.ctx, log.Logger).With().Str("session_key", lane).Logger()
		logger.Debug().
			Str("lane", lane).
			Str("taskId", record.id).
			Int("running", ls.running).
			Msg("Task started")

		// Execute task in goroutine
		cq.wg.Add(1)
		go cq.executeTask(lane, record)
	}
}

// executeTask executes a single task
func (cq *CommandQueue) executeTask(lane string, record *taskRecord) {
	defer cq.wg.Done()

	taskCtx := record.ctx
	if taskCtx == nil {
		taskCtx = context.Background()
	}
	taskCtx, span := tracing.StartSpan(
		taskCtx,
		"ranya.commandqueue",
		"commandqueue.execute_task",
		attribute.String("lane", lane),
		attribute.String("task_id", record.id),
	)
	defer span.End()

	taskCtx = tracing.WithSessionKey(taskCtx, lane)
	logger := tracing.LoggerFromContext(taskCtx, log.Logger).With().Str("session_key", lane).Logger()

	runCtx, cancel := context.WithCancel(taskCtx)
	stopCancel := context.AfterFunc(cq.ctx, cancel)
	defer func() {
		stopCancel()
		cancel()
	}()

	startTime := time.Now()

	// Execute task
	value, err := record.task(runCtx)

	duration := time.Since(startTime)

	// Update lane state
	ls := cq.lanes[lane]
	ls.mu.Lock()
	ls.running--
	delete(ls.activeIDs, record.id)
	queueSize := len(ls.queue)
	ls.mu.Unlock()

	// Send result
	record.result <- taskResult{value: value, err: err}
	close(record.result)

	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		logger.Error().
			Str("lane", lane).
			Str("taskId", record.id).
			Dur("duration", duration).
			Err(err).
			Msg("Task failed")
	} else {
		logger.Debug().
			Str("lane", lane).
			Str("taskId", record.id).
			Dur("duration", duration).
			Msg("Task completed")
	}

	observability.RecordQueueCompletion(lane, duration, err == nil, queueSize)

	// Emit completed event (synchronous)
	cq.emit(Event{
		Type:   "completed",
		Lane:   lane,
		TaskID: record.id,
		Data: map[string]interface{}{
			"duration": duration.Milliseconds(),
			"success":  err == nil,
		},
	})

	// Process next task in queue
	go cq.processLane(lane)
}

// startWarnTimer starts a timer to warn about long wait times
func (cq *CommandQueue) startWarnTimer(record *taskRecord, lane string) {
	timer := time.NewTimer(time.Duration(record.options.WarnAfterMs) * time.Millisecond)
	defer timer.Stop()

	select {
	case <-timer.C:
		// Check if task is still queued
		ls := cq.lanes[lane]
		ls.mu.Lock()
		queuePos := -1
		for i, r := range ls.queue {
			if r.id == record.id {
				queuePos = i
				break
			}
		}
		ls.mu.Unlock()

		if queuePos >= 0 {
			waitMs := time.Since(record.enqueuedAt).Milliseconds()
			log.Warn().
				Str("lane", lane).
				Str("taskId", record.id).
				Int64("waitMs", waitMs).
				Int("queuePos", queuePos).
				Msg("Task waiting longer than expected")

			if record.options.OnWait != nil {
				record.options.OnWait(waitMs, queuePos)
			}
		}
	case <-cq.ctx.Done():
		return
	}
}

// GetQueueSize returns the number of queued tasks for a lane
func (cq *CommandQueue) GetQueueSize(lane string) int {
	cq.mu.RLock()
	ls, exists := cq.lanes[lane]
	cq.mu.RUnlock()

	if !exists {
		return 0
	}

	ls.mu.Lock()
	defer ls.mu.Unlock()
	return len(ls.queue)
}

// GetRunningCount returns the number of currently executing tasks for a lane
func (cq *CommandQueue) GetRunningCount(lane string) int {
	cq.mu.RLock()
	ls, exists := cq.lanes[lane]
	cq.mu.RUnlock()

	if !exists {
		return 0
	}

	ls.mu.Lock()
	defer ls.mu.Unlock()
	return ls.running
}

// GetStats returns statistics for all lanes
func (cq *CommandQueue) GetStats() map[string]map[string]int {
	cq.mu.RLock()
	defer cq.mu.RUnlock()

	stats := make(map[string]map[string]int)
	for lane, ls := range cq.lanes {
		ls.mu.Lock()
		stats[lane] = map[string]int{
			"queued":      len(ls.queue),
			"running":     ls.running,
			"concurrency": ls.concurrency,
		}
		ls.mu.Unlock()
	}

	return stats
}

// ClearLane removes all queued tasks from a lane
func (cq *CommandQueue) ClearLane(lane string) int {
	cq.mu.RLock()
	ls, exists := cq.lanes[lane]
	cq.mu.RUnlock()

	if !exists {
		return 0
	}

	ls.mu.Lock()
	defer ls.mu.Unlock()

	count := len(ls.queue)

	// Reject all queued tasks
	for _, record := range ls.queue {
		record.result <- taskResult{
			err: fmt.Errorf("lane cleared"),
		}
		close(record.result)
	}

	ls.queue = make([]*taskRecord, 0)

	log.Info().Str("lane", lane).Int("cleared", count).Msg("Lane cleared")
	observability.SetQueueSize(lane, 0)

	return count
}

// ResetLane increments the generation counter for a lane
func (cq *CommandQueue) ResetLane(lane string) {
	cq.mu.RLock()
	ls, exists := cq.lanes[lane]
	cq.mu.RUnlock()

	if !exists {
		return
	}

	ls.mu.Lock()
	defer ls.mu.Unlock()

	ls.generation++

	// Reject all queued tasks
	for _, record := range ls.queue {
		record.result <- taskResult{
			err: fmt.Errorf("lane reset"),
		}
		close(record.result)
	}

	ls.queue = make([]*taskRecord, 0)

	log.Info().Str("lane", lane).Int("generation", ls.generation).Msg("Lane reset")
	observability.SetQueueSize(lane, 0)
}

// SetConcurrency updates the concurrency limit for a lane
func (cq *CommandQueue) SetConcurrency(lane string, concurrency int) {
	cq.ensureLane(lane)

	ls := cq.lanes[lane]
	ls.mu.Lock()
	oldMax := ls.concurrency
	ls.concurrency = concurrency
	ls.mu.Unlock()

	log.Info().
		Str("lane", lane).
		Int("oldMax", oldMax).
		Int("newMax", concurrency).
		Msg("Lane concurrency updated")

	// Process queue in case we increased concurrency
	if concurrency > oldMax {
		go cq.processLane(lane)
	}
}

// WaitForActive waits for all active tasks to complete with timeout
func (cq *CommandQueue) WaitForActive(timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		allDrained := true

		cq.mu.RLock()
		for _, ls := range cq.lanes {
			ls.mu.Lock()
			if len(ls.activeIDs) > 0 {
				allDrained = false
			}
			ls.mu.Unlock()
		}
		cq.mu.RUnlock()

		if allDrained {
			log.Info().Msg("All active tasks completed")
			return true
		}

		if time.Now().After(deadline) {
			log.Warn().Dur("timeout", timeout).Msg("Timeout waiting for active tasks")
			return false
		}

		<-ticker.C
	}
}

// Close gracefully shuts down the command queue
func (cq *CommandQueue) Close() error {
	cq.cancel()
	cq.wg.Wait()
	return nil
}

// Global singleton instance
var (
	globalQueue     *CommandQueue
	globalQueueOnce sync.Once
)

// GetCommandQueue returns the global command queue instance
func GetCommandQueue() *CommandQueue {
	globalQueueOnce.Do(func() {
		globalQueue = New()
	})
	return globalQueue
}

// EnqueueCommand enqueues a task to the main lane of the global instance
func EnqueueCommand(task Task, options *TaskOptions) (interface{}, error) {
	return GetCommandQueue().Enqueue("main", task, options)
}

// EnqueueCommandInLane enqueues a task to a specific lane of the global instance
func EnqueueCommandInLane(lane string, task Task, options *TaskOptions) (interface{}, error) {
	return GetCommandQueue().Enqueue(lane, task, options)
}

// On registers an event handler for a specific event type
func (cq *CommandQueue) On(eventType string, handler EventHandler) {
	cq.eventMu.Lock()
	defer cq.eventMu.Unlock()

	cq.eventHandlers[eventType] = append(cq.eventHandlers[eventType], handler)
}

// Off removes an event handler (removes all handlers for the event type)
func (cq *CommandQueue) Off(eventType string) {
	cq.eventMu.Lock()
	defer cq.eventMu.Unlock()

	delete(cq.eventHandlers, eventType)
}

// emit emits an event synchronously to all registered handlers
func (cq *CommandQueue) emit(event Event) {
	cq.eventMu.RLock()
	handlers := cq.eventHandlers[event.Type]
	cq.eventMu.RUnlock()

	// Call handlers synchronously
	for _, handler := range handlers {
		handler(event)
	}
}
