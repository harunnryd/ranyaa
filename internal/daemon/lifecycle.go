package daemon

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

// LifecycleManager manages daemon lifecycle (PID file, auto-restart, etc.)
type LifecycleManager struct {
	daemon  *Daemon
	pidFile string
}

// NewLifecycleManager creates a new lifecycle manager
func NewLifecycleManager(d *Daemon) *LifecycleManager {
	pidFile := filepath.Join(d.config.DataDir, "ranya.pid")

	return &LifecycleManager{
		daemon:  d,
		pidFile: pidFile,
	}
}

// Start starts the lifecycle manager
func (l *LifecycleManager) Start() error {
	// Ensure data directory exists
	if err := os.MkdirAll(l.daemon.config.DataDir, 0755); err != nil {
		return fmt.Errorf("failed to create data directory: %w", err)
	}

	// Write PID file
	if err := l.writePIDFile(); err != nil {
		return fmt.Errorf("failed to write PID file: %w", err)
	}

	l.daemon.logger.Info().
		Str("pid_file", l.pidFile).
		Int("pid", os.Getpid()).
		Msg("Lifecycle manager started")

	return nil
}

// Stop stops the lifecycle manager
func (l *LifecycleManager) Stop() error {
	// Remove PID file
	if err := os.Remove(l.pidFile); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove PID file: %w", err)
	}

	l.daemon.logger.Info().Msg("Lifecycle manager stopped")

	return nil
}

// writePIDFile writes the current process ID to the PID file
func (l *LifecycleManager) writePIDFile() error {
	pid := os.Getpid()
	content := fmt.Sprintf("%d", pid)

	return os.WriteFile(l.pidFile, []byte(content), 0644)
}

// GetUptime returns the daemon uptime
func (l *LifecycleManager) GetUptime() time.Duration {
	status := l.daemon.Status()
	return status.Uptime
}

// GetPID returns the daemon PID from the PID file
func (l *LifecycleManager) GetPID() (int, error) {
	data, err := os.ReadFile(l.pidFile)
	if err != nil {
		return 0, err
	}

	pid, err := strconv.Atoi(string(data))
	if err != nil {
		return 0, fmt.Errorf("invalid PID file: %w", err)
	}

	return pid, nil
}

// IsRunning checks if the daemon is running
func (l *LifecycleManager) IsRunning() bool {
	pid, err := l.GetPID()
	if err != nil {
		return false
	}

	// Check if process exists
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}

	// On Unix, FindProcess always succeeds, so we need to send signal 0
	err = process.Signal(os.Signal(nil))
	return err == nil
}
