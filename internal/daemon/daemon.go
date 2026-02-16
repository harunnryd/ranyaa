package daemon

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/harun/ranya/internal/config"
	"github.com/harun/ranya/internal/logger"
	"github.com/harun/ranya/internal/telegram"
	"github.com/harun/ranya/pkg/commandqueue"
	"github.com/harun/ranya/pkg/session"
)

// Daemon represents the Ranya daemon service
type Daemon struct {
	config      *config.Config
	logger      *logger.Logger
	queue       *commandqueue.CommandQueue
	sessionMgr  *session.SessionManager
	telegramBot *telegram.Bot
	archiver    *session.Archiver
	cleanup     *session.Cleanup
	eventLoop   *EventLoop
	router      *Router
	lifecycle   *LifecycleManager

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	startTime time.Time
	running   bool
	mu        sync.RWMutex
}

// New creates a new daemon instance
func New(cfg *config.Config, log *logger.Logger) (*Daemon, error) {
	// Create session manager
	sessionMgr, err := session.New(cfg.DataDir + "/sessions")
	if err != nil {
		return nil, fmt.Errorf("failed to create session manager: %w", err)
	}

	// Create command queue
	queue := commandqueue.New()

	// Create context
	ctx, cancel := context.WithCancel(context.Background())

	d := &Daemon{
		config:     cfg,
		logger:     log,
		queue:      queue,
		sessionMgr: sessionMgr,
		ctx:        ctx,
		cancel:     cancel,
	}

	// Create Telegram bot if enabled
	if cfg.Channels.Telegram.Enabled {
		bot, err := telegram.New(&cfg.Telegram, log)
		if err != nil {
			cancel()
			return nil, fmt.Errorf("failed to create telegram bot: %w", err)
		}
		d.telegramBot = bot
	}

	// Create session archiver
	d.archiver = session.NewArchiver(sessionMgr, 30*time.Minute)

	// Create session cleanup
	d.cleanup = session.NewCleanup(sessionMgr, 7*24*time.Hour)

	// Create event loop
	d.eventLoop = NewEventLoop(d)

	// Create router
	d.router = NewRouter(d)

	// Create lifecycle manager
	d.lifecycle = NewLifecycleManager(d)

	return d, nil
}

// Start starts the daemon service
func (d *Daemon) Start() error {
	d.mu.Lock()
	if d.running {
		d.mu.Unlock()
		return fmt.Errorf("daemon is already running")
	}
	d.running = true
	d.startTime = time.Now()
	d.mu.Unlock()

	d.logger.Info().Msg("Starting Ranya daemon")

	// Start lifecycle manager
	if err := d.lifecycle.Start(); err != nil {
		return fmt.Errorf("failed to start lifecycle manager: %w", err)
	}

	// Start Telegram bot if enabled
	if d.telegramBot != nil {
		if err := d.telegramBot.Start(); err != nil {
			return fmt.Errorf("failed to start telegram bot: %w", err)
		}
		d.logger.Info().Msg("Telegram bot started")
	}

	// Start session archiver
	if err := d.archiver.Start(); err != nil {
		d.logger.Warn().Err(err).Msg("Failed to start session archiver")
	}

	// Start session cleanup
	if err := d.cleanup.Start(); err != nil {
		d.logger.Warn().Err(err).Msg("Failed to start session cleanup")
	}

	// Start event loop
	d.wg.Add(1)
	go func() {
		defer d.wg.Done()
		d.eventLoop.Run(d.ctx)
	}()

	d.logger.Info().Msg("Daemon started successfully")

	return nil
}

// Stop stops the daemon service gracefully
func (d *Daemon) Stop() error {
	d.mu.Lock()
	if !d.running {
		d.mu.Unlock()
		return fmt.Errorf("daemon is not running")
	}
	d.running = false
	d.mu.Unlock()

	d.logger.Info().Msg("Stopping Ranya daemon")

	// Stop Telegram bot
	if d.telegramBot != nil {
		if err := d.telegramBot.Stop(); err != nil {
			d.logger.Error().Err(err).Msg("Failed to stop telegram bot")
		}
	}

	// Stop session archiver
	if d.archiver != nil && d.archiver.IsRunning() {
		if err := d.archiver.Stop(); err != nil {
			d.logger.Error().Err(err).Msg("Failed to stop session archiver")
		}
	}

	// Stop session cleanup
	if d.cleanup != nil && d.cleanup.IsRunning() {
		if err := d.cleanup.Stop(); err != nil {
			d.logger.Error().Err(err).Msg("Failed to stop session cleanup")
		}
	}

	// Cancel context
	d.cancel()

	// Wait for goroutines to finish (with timeout)
	done := make(chan struct{})
	go func() {
		d.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		d.logger.Info().Msg("All goroutines stopped")
	case <-time.After(5 * time.Second):
		d.logger.Warn().Msg("Timeout waiting for goroutines to stop")
	}

	// Stop lifecycle manager
	if err := d.lifecycle.Stop(); err != nil {
		d.logger.Error().Err(err).Msg("Failed to stop lifecycle manager")
	}

	// Close session manager
	if err := d.sessionMgr.Close(); err != nil {
		d.logger.Error().Err(err).Msg("Failed to close session manager")
	}

	d.logger.Info().Msg("Daemon stopped successfully")

	return nil
}

// Status returns the daemon status
func (d *Daemon) Status() Status {
	d.mu.RLock()
	defer d.mu.RUnlock()

	status := Status{
		Running: d.running,
	}

	if d.running {
		status.Uptime = time.Since(d.startTime)
		status.StartTime = d.startTime
	}

	return status
}

// Wait waits for the daemon to stop
func (d *Daemon) Wait() {
	// Setup signal handling
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Wait for signal
	sig := <-sigChan
	d.logger.Info().Str("signal", sig.String()).Msg("Received signal")

	// Stop daemon
	if err := d.Stop(); err != nil {
		d.logger.Error().Err(err).Msg("Failed to stop daemon")
	}
}

// GetConfig returns the daemon configuration
func (d *Daemon) GetConfig() *config.Config {
	return d.config
}

// GetLogger returns the daemon logger
func (d *Daemon) GetLogger() *logger.Logger {
	return d.logger
}

// GetQueue returns the command queue
func (d *Daemon) GetQueue() *commandqueue.CommandQueue {
	return d.queue
}

// GetSessionManager returns the session manager
func (d *Daemon) GetSessionManager() *session.SessionManager {
	return d.sessionMgr
}

// GetRouter returns the message router
func (d *Daemon) GetRouter() *Router {
	return d.router
}

// Status represents daemon status
type Status struct {
	Running   bool
	Uptime    time.Duration
	StartTime time.Time
}

// GetTelegramBot returns the Telegram bot
func (d *Daemon) GetTelegramBot() *telegram.Bot {
	return d.telegramBot
}

// GetArchiver returns the session archiver
func (d *Daemon) GetArchiver() *session.Archiver {
	return d.archiver
}

// GetCleanup returns the session cleanup
func (d *Daemon) GetCleanup() *session.Cleanup {
	return d.cleanup
}
