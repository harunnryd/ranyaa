package commandqueue

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestCommandQueue_BasicEnqueue(t *testing.T) {
	cq := New()
	defer cq.Close()

	executed := false
	task := func(ctx context.Context) (interface{}, error) {
		executed = true
		return "result", nil
	}

	result, err := cq.Enqueue("test", task, nil)

	assert.NoError(t, err)
	assert.Equal(t, "result", result)
	assert.True(t, executed)
}

func TestCommandQueue_TaskError(t *testing.T) {
	cq := New()
	defer cq.Close()

	expectedErr := errors.New("task failed")
	task := func(ctx context.Context) (interface{}, error) {
		return nil, expectedErr
	}

	result, err := cq.Enqueue("test", task, nil)

	assert.Error(t, err)
	assert.Equal(t, expectedErr, err)
	assert.Nil(t, result)
}

func TestCommandQueue_SerialExecution(t *testing.T) {
	cq := New()
	defer cq.Close()

	var order []int
	var mu sync.Mutex

	for i := 0; i < 5; i++ {
		i := i
		go func() {
			task := func(ctx context.Context) (interface{}, error) {
				time.Sleep(10 * time.Millisecond)
				mu.Lock()
				order = append(order, i)
				mu.Unlock()
				return nil, nil
			}
			_, _ = cq.Enqueue("serial", task, nil)
		}()
	}

	time.Sleep(200 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, 5, len(order))
}

func TestCommandQueue_ConcurrentLanes(t *testing.T) {
	cq := New()
	defer cq.Close()

	var count1, count2 int
	var mu sync.Mutex

	// Lane 1
	for i := 0; i < 3; i++ {
		go func() {
			task := func(ctx context.Context) (interface{}, error) {
				mu.Lock()
				count1++
				mu.Unlock()
				time.Sleep(50 * time.Millisecond)
				return nil, nil
			}
			_, _ = cq.Enqueue("lane1", task, nil)
		}()
	}

	// Lane 2
	for i := 0; i < 3; i++ {
		go func() {
			task := func(ctx context.Context) (interface{}, error) {
				mu.Lock()
				count2++
				mu.Unlock()
				time.Sleep(50 * time.Millisecond)
				return nil, nil
			}
			_, _ = cq.Enqueue("lane2", task, nil)
		}()
	}

	time.Sleep(300 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, 3, count1)
	assert.Equal(t, 3, count2)
}

func TestCommandQueue_GetStats(t *testing.T) {
	cq := New()
	defer cq.Close()

	stats := cq.GetStats()

	assert.Contains(t, stats, "main")
	assert.Contains(t, stats, "cron")
	assert.Equal(t, 1, stats["main"]["concurrency"])
	assert.Equal(t, 5, stats["cron"]["concurrency"])
}

func TestCommandQueue_ClearLane(t *testing.T) {
	cq := New()
	defer cq.Close()

	// Enqueue tasks that will block
	for i := 0; i < 5; i++ {
		go func() {
			task := func(ctx context.Context) (interface{}, error) {
				time.Sleep(1 * time.Second)
				return nil, nil
			}
			_, _ = cq.Enqueue("test", task, nil)
		}()
	}

	time.Sleep(50 * time.Millisecond)

	cleared := cq.ClearLane("test")
	assert.Greater(t, cleared, 0)
}

func TestCommandQueue_SetConcurrency(t *testing.T) {
	cq := New()
	defer cq.Close()

	cq.SetConcurrency("test", 3)

	stats := cq.GetStats()
	assert.Equal(t, 3, stats["test"]["concurrency"])
}

func TestCommandQueue_WaitForActive(t *testing.T) {
	cq := New()
	defer cq.Close()

	// Start a quick task
	go func() {
		task := func(ctx context.Context) (interface{}, error) {
			time.Sleep(50 * time.Millisecond)
			return nil, nil
		}
		_, _ = cq.Enqueue("test", task, nil)
	}()

	time.Sleep(10 * time.Millisecond)

	drained := cq.WaitForActive(200 * time.Millisecond)
	assert.True(t, drained)
}

func TestCommandQueue_GlobalInstance(t *testing.T) {
	q1 := GetCommandQueue()
	q2 := GetCommandQueue()

	assert.Equal(t, q1, q2, "Should return same instance")
}

func TestCommandQueue_EventEmission(t *testing.T) {
	queue := New()
	defer queue.Close()

	var events []Event
	var mu sync.Mutex

	// Register event handlers
	queue.On("enqueued", func(event Event) {
		mu.Lock()
		events = append(events, event)
		mu.Unlock()
	})

	queue.On("completed", func(event Event) {
		mu.Lock()
		events = append(events, event)
		mu.Unlock()
	})

	// Enqueue a task
	_, err := queue.Enqueue("test", Task(func(ctx context.Context) (interface{}, error) {
		return "result", nil
	}), nil)
	assert.NoError(t, err)

	// Wait a bit for events
	time.Sleep(100 * time.Millisecond)

	// Check events
	mu.Lock()
	defer mu.Unlock()

	assert.GreaterOrEqual(t, len(events), 2, "Should have at least enqueued and completed events")

	// Check enqueued event
	enqueuedFound := false
	completedFound := false

	for _, event := range events {
		if event.Type == "enqueued" {
			enqueuedFound = true
			assert.Equal(t, "test", event.Lane)
			assert.NotEmpty(t, event.TaskID)
			assert.Contains(t, event.Data, "queueSize")
		}
		if event.Type == "completed" {
			completedFound = true
			assert.Equal(t, "test", event.Lane)
			assert.NotEmpty(t, event.TaskID)
			assert.Contains(t, event.Data, "duration")
			assert.Contains(t, event.Data, "success")
		}
	}

	assert.True(t, enqueuedFound, "Should have enqueued event")
	assert.True(t, completedFound, "Should have completed event")
}

func TestCommandQueue_EventOff(t *testing.T) {
	queue := New()
	defer queue.Close()

	eventCount := 0

	// Register handler
	queue.On("enqueued", func(event Event) {
		eventCount++
	})

	// Enqueue task
	_, _ = queue.Enqueue("test", Task(func(ctx context.Context) (interface{}, error) {
		return nil, nil
	}), nil)

	time.Sleep(50 * time.Millisecond)
	assert.Equal(t, 1, eventCount)

	// Remove handler
	queue.Off("enqueued")

	// Enqueue another task
	_, _ = queue.Enqueue("test", Task(func(ctx context.Context) (interface{}, error) {
		return nil, nil
	}), nil)

	time.Sleep(50 * time.Millisecond)
	assert.Equal(t, 1, eventCount, "Should not receive events after Off")
}
