package session

import (
	"fmt"
	"time"

	"github.com/rs/zerolog/log"
)

const (
	DefaultIdleTimeout = 30 * time.Minute
)

// Archiver handles session archiving
type Archiver struct {
	manager     *SessionManager
	idleTimeout time.Duration
	stopCh      chan struct{}
	running     bool
}

// NewArchiver creates a new session archiver
func NewArchiver(manager *SessionManager, idleTimeout time.Duration) *Archiver {
	if idleTimeout == 0 {
		idleTimeout = DefaultIdleTimeout
	}

	return &Archiver{
		manager:     manager,
		idleTimeout: idleTimeout,
		stopCh:      make(chan struct{}),
	}
}

// Start starts the archiver
func (a *Archiver) Start() error {
	if a.running {
		return fmt.Errorf("archiver is already running")
	}

	a.running = true
	go a.run()

	log.Info().
		Dur("idle_timeout", a.idleTimeout).
		Msg("Session archiver started")

	return nil
}

// Stop stops the archiver
func (a *Archiver) Stop() error {
	if !a.running {
		return fmt.Errorf("archiver is not running")
	}

	close(a.stopCh)
	a.running = false

	log.Info().Msg("Session archiver stopped")

	return nil
}

// run is the main archiver loop
func (a *Archiver) run() {
	ticker := time.NewTicker(5 * time.Minute) // Check every 5 minutes
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if err := a.archiveIdleSessions(); err != nil {
				log.Error().Err(err).Msg("Failed to archive idle sessions")
			}
		case <-a.stopCh:
			return
		}
	}
}

// archiveIdleSessions archives sessions that have been idle
func (a *Archiver) archiveIdleSessions() error {
	sessions, err := a.manager.ListSessions()
	if err != nil {
		return fmt.Errorf("failed to list sessions: %w", err)
	}

	now := time.Now()
	archived := 0

	for _, sessionKey := range sessions {
		// Skip already archived sessions
		if isArchivedSession(sessionKey) {
			continue
		}

		// Get session info
		info, err := a.manager.GetSessionInfo(sessionKey)
		if err != nil {
			log.Warn().
				Str("session_key", sessionKey).
				Err(err).
				Msg("Failed to get session info")
			continue
		}

		// Check if session is idle
		lastModified, ok := info["lastModified"].(time.Time)
		if !ok {
			continue
		}

		idleTime := now.Sub(lastModified)
		if idleTime >= a.idleTimeout {
			if err := a.archiveSession(sessionKey); err != nil {
				log.Error().
					Str("session_key", sessionKey).
					Err(err).
					Msg("Failed to archive session")
				continue
			}
			archived++
		}
	}

	if archived > 0 {
		log.Info().
			Int("archived", archived).
			Msg("Archived idle sessions")
	}

	return nil
}

// archiveSession marks a session as archived
func (a *Archiver) archiveSession(sessionKey string) error {
	// Rename session file to mark as archived
	archivedKey := fmt.Sprintf("archived_%s", sessionKey)

	// Load session
	entries, err := a.manager.LoadSession(sessionKey)
	if err != nil {
		return fmt.Errorf("failed to load session: %w", err)
	}

	// Create archived session
	if err := a.manager.CreateSession(archivedKey); err != nil {
		return fmt.Errorf("failed to create archived session: %w", err)
	}

	// Copy entries to archived session
	for _, entry := range entries {
		if err := a.manager.AppendMessage(archivedKey, entry.Message); err != nil {
			return fmt.Errorf("failed to copy message: %w", err)
		}
	}

	// Delete original session
	if err := a.manager.DeleteSession(sessionKey); err != nil {
		return fmt.Errorf("failed to delete original session: %w", err)
	}

	log.Info().
		Str("session_key", sessionKey).
		Str("archived_key", archivedKey).
		Msg("Session archived")

	return nil
}

// IsRunning returns whether the archiver is running
func (a *Archiver) IsRunning() bool {
	return a.running
}

// GetIdleTimeout returns the idle timeout
func (a *Archiver) GetIdleTimeout() time.Duration {
	return a.idleTimeout
}

// SetIdleTimeout sets the idle timeout
func (a *Archiver) SetIdleTimeout(timeout time.Duration) {
	a.idleTimeout = timeout
	log.Info().Dur("idle_timeout", timeout).Msg("Idle timeout updated")
}

// isArchivedSession checks if a session key represents an archived session
func isArchivedSession(sessionKey string) bool {
	return len(sessionKey) > 9 && sessionKey[:9] == "archived_"
}

// ArchiveNow immediately archives a specific session
func (a *Archiver) ArchiveNow(sessionKey string) error {
	if isArchivedSession(sessionKey) {
		return fmt.Errorf("session is already archived")
	}

	return a.archiveSession(sessionKey)
}

// GetArchivedSessions returns all archived sessions
func (a *Archiver) GetArchivedSessions() ([]string, error) {
	sessions, err := a.manager.ListSessions()
	if err != nil {
		return nil, err
	}

	var archived []string
	for _, sessionKey := range sessions {
		if isArchivedSession(sessionKey) {
			archived = append(archived, sessionKey)
		}
	}

	return archived, nil
}
