package daemon

import (
	"context"
	"time"
)

// EventLoop handles the main event processing loop
type EventLoop struct {
	daemon *Daemon
}

// NewEventLoop creates a new event loop
func NewEventLoop(d *Daemon) *EventLoop {
	return &EventLoop{
		daemon: d,
	}
}

// Run runs the event loop
func (e *EventLoop) Run(ctx context.Context) {
	e.daemon.logger.Info().Msg("Event loop started")

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			e.daemon.logger.Info().Msg("Event loop stopping")
			return

		case <-ticker.C:
			// Process any pending tasks
			e.processTasks(ctx)
		}
	}
}

// processTasks processes pending tasks from the queue
func (e *EventLoop) processTasks(ctx context.Context) {
	// Get queue stats to check for active tasks
	stats := e.daemon.queue.GetStats()

	// Log stats periodically (every 10 seconds)
	// This is a placeholder for actual task processing
	// In a real implementation, this would process messages from Telegram, Gateway, etc.
	_ = stats
}

// HandleShutdown handles graceful shutdown
func (e *EventLoop) HandleShutdown() {
	e.daemon.logger.Info().Msg("Handling graceful shutdown")

	// Wait for active tasks to complete
	e.daemon.queue.WaitForActive(5 * time.Second)

	e.daemon.logger.Info().Msg("All active tasks completed")
}
