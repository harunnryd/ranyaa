package session

import (
	"fmt"
	"time"

	"github.com/rs/zerolog/log"
)

const (
	DefaultCleanupAge = 7 * 24 * time.Hour // 7 days
	DefaultMaxEntries = 500
)

// Cleanup handles session cleanup
type Cleanup struct {
	manager    *SessionManager
	cleanupAge time.Duration
	maxEntries int
	stopCh     chan struct{}
	running    bool
}

// NewCleanup creates a new session cleanup handler
func NewCleanup(manager *SessionManager, cleanupAge time.Duration) *Cleanup {
	if cleanupAge == 0 {
		cleanupAge = DefaultCleanupAge
	}

	return &Cleanup{
		manager:    manager,
		cleanupAge: cleanupAge,
		maxEntries: DefaultMaxEntries,
		stopCh:     make(chan struct{}),
	}
}

// Start starts the cleanup handler
func (c *Cleanup) Start() error {
	if c.running {
		return fmt.Errorf("cleanup is already running")
	}

	c.running = true
	go c.run()

	log.Info().
		Dur("cleanup_age", c.cleanupAge).
		Msg("Session cleanup started")

	return nil
}

// Stop stops the cleanup handler
func (c *Cleanup) Stop() error {
	if !c.running {
		return fmt.Errorf("cleanup is not running")
	}

	close(c.stopCh)
	c.running = false

	log.Info().Msg("Session cleanup stopped")

	return nil
}

// run is the main cleanup loop
func (c *Cleanup) run() {
	ticker := time.NewTicker(24 * time.Hour) // Check daily
	defer ticker.Stop()

	// Run immediately on start
	if err := c.cleanupOldSessions(); err != nil {
		log.Error().Err(err).Msg("Failed to cleanup old sessions")
	}

	for {
		select {
		case <-ticker.C:
			if err := c.cleanupOldSessions(); err != nil {
				log.Error().Err(err).Msg("Failed to cleanup old sessions")
			}
		case <-c.stopCh:
			return
		}
	}
}

// cleanupOldSessions deletes archived sessions older than cleanupAge
func (c *Cleanup) cleanupOldSessions() error {
	sessions, err := c.manager.ListSessions()
	if err != nil {
		return fmt.Errorf("failed to list sessions: %w", err)
	}

	now := time.Now()
	deleted := 0

	for _, sessionKey := range sessions {
		if err := c.pruneSession(sessionKey); err != nil {
			log.Warn().
				Str("session_key", sessionKey).
				Err(err).
				Msg("Failed to prune session")
		}

		// Only cleanup archived sessions
		if !isArchivedSession(sessionKey) {
			continue
		}

		// Get session info
		info, err := c.manager.GetSessionInfo(sessionKey)
		if err != nil {
			log.Warn().
				Str("session_key", sessionKey).
				Err(err).
				Msg("Failed to get session info")
			continue
		}

		// Check if session is old enough
		lastModified, ok := info["lastModified"].(time.Time)
		if !ok {
			continue
		}

		age := now.Sub(lastModified)
		if age >= c.cleanupAge {
			if err := c.manager.DeleteSession(sessionKey); err != nil {
				log.Error().
					Str("session_key", sessionKey).
					Err(err).
					Msg("Failed to delete session")
				continue
			}
			deleted++

			log.Debug().
				Str("session_key", sessionKey).
				Dur("age", age).
				Msg("Session deleted")
		}
	}

	if deleted > 0 {
		log.Info().
			Int("deleted", deleted).
			Msg("Cleaned up old sessions")
	}

	return nil
}

func (c *Cleanup) pruneSession(sessionKey string) error {
	if c.maxEntries <= 0 {
		return nil
	}

	entries, err := c.manager.LoadSession(sessionKey)
	if err != nil {
		return err
	}

	if len(entries) <= c.maxEntries {
		return nil
	}

	pruned := entries[len(entries)-c.maxEntries:]
	if err := c.manager.ReplaceSession(sessionKey, pruned); err != nil {
		return err
	}

	log.Debug().
		Str("session_key", sessionKey).
		Int("from_entries", len(entries)).
		Int("to_entries", len(pruned)).
		Msg("Session pruned")

	return nil
}

// IsRunning returns whether the cleanup is running
func (c *Cleanup) IsRunning() bool {
	return c.running
}

// GetCleanupAge returns the cleanup age
func (c *Cleanup) GetCleanupAge() time.Duration {
	return c.cleanupAge
}

// SetCleanupAge sets the cleanup age
func (c *Cleanup) SetCleanupAge(age time.Duration) {
	c.cleanupAge = age
	log.Info().Dur("cleanup_age", age).Msg("Cleanup age updated")
}

// GetMaxEntries returns max entries retained per session after pruning.
func (c *Cleanup) GetMaxEntries() int {
	return c.maxEntries
}

// SetMaxEntries sets max entries retained per session after pruning.
func (c *Cleanup) SetMaxEntries(maxEntries int) {
	c.maxEntries = maxEntries
	log.Info().Int("max_entries", maxEntries).Msg("Session pruning max entries updated")
}

// CleanupNow immediately runs cleanup
func (c *Cleanup) CleanupNow() error {
	return c.cleanupOldSessions()
}

// GetCleanupStats returns cleanup statistics
func (c *Cleanup) GetCleanupStats() (map[string]interface{}, error) {
	sessions, err := c.manager.ListSessions()
	if err != nil {
		return nil, err
	}

	totalSessions := len(sessions)
	archivedSessions := 0
	eligibleForCleanup := 0

	now := time.Now()

	for _, sessionKey := range sessions {
		if isArchivedSession(sessionKey) {
			archivedSessions++

			info, err := c.manager.GetSessionInfo(sessionKey)
			if err != nil {
				continue
			}

			lastModified, ok := info["lastModified"].(time.Time)
			if !ok {
				continue
			}

			age := now.Sub(lastModified)
			if age >= c.cleanupAge {
				eligibleForCleanup++
			}
		}
	}

	return map[string]interface{}{
		"total_sessions":       totalSessions,
		"archived_sessions":    archivedSessions,
		"eligible_for_cleanup": eligibleForCleanup,
		"cleanup_age":          c.cleanupAge.String(),
		"running":              c.running,
	}, nil
}
