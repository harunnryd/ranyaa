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

// Run runs the event loop with periodic maintenance tasks
func (e *EventLoop) Run(ctx context.Context) {
	e.daemon.logger.Info().Msg("Event loop started")

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			e.daemon.logger.Info().Msg("Event loop stopping")
			return

		case <-ticker.C:
			// Process periodic maintenance tasks
			e.processTasks(ctx)
		}
	}
}

// processTasks processes periodic maintenance tasks
func (e *EventLoop) processTasks(ctx context.Context) {
	// Sync memory manager (if configured)
	if e.daemon.memoryMgr != nil {
		if err := e.daemon.memoryMgr.Sync(); err != nil {
			e.daemon.logger.Warn().Err(err).Msg("Memory sync failed")
		}
	}

	// Log queue stats for monitoring
	stats := e.daemon.queue.GetStats()
	for lane, laneStats := range stats {
		if laneStats["queued"] > 0 || laneStats["running"] > 0 {
			e.daemon.logger.Debug().
				Str("lane", lane).
				Int("queued", laneStats["queued"]).
				Int("running", laneStats["running"]).
				Msg("Queue stats")
		}
	}

	// NOTE: Session archiver and cleanup run in their own goroutines
	// via Start() methods called during daemon initialization
}

// HandleShutdown handles graceful shutdown
func (e *EventLoop) HandleShutdown() {
	e.daemon.logger.Info().Msg("Handling graceful shutdown")

	// Wait for active tasks to complete
	e.daemon.queue.WaitForActive(5 * time.Second)

	e.daemon.logger.Info().Msg("All active tasks completed")
}
