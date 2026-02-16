package subagent

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	gonanoid "github.com/matoous/go-nanoid/v2"
	"github.com/rs/zerolog"
)

// Coordinator manages subagent lifecycle and tracking
type Coordinator struct {
	activeRuns   map[string]*RunRecord
	registryPath string
	autoSave     bool
	logger       zerolog.Logger
	mu           sync.RWMutex

	// Event handlers
	eventHandlers map[string][]EventHandler
	eventMu       sync.RWMutex
}

// Config holds coordinator configuration
type Config struct {
	RegistryPath string
	AutoSave     bool
	Logger       zerolog.Logger
}

// NewCoordinator creates a new subagent coordinator
func NewCoordinator(cfg Config) (*Coordinator, error) {
	if cfg.RegistryPath == "" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("failed to get home directory: %w", err)
		}
		cfg.RegistryPath = filepath.Join(homeDir, ".ranya", "subagents.json")
	}

	c := &Coordinator{
		activeRuns:    make(map[string]*RunRecord),
		registryPath:  cfg.RegistryPath,
		autoSave:      cfg.AutoSave,
		logger:        cfg.Logger,
		eventHandlers: make(map[string][]EventHandler),
	}

	return c, nil
}

// Initialize loads the registry from disk
func (c *Coordinator) Initialize() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Check if registry file exists
	if _, err := os.Stat(c.registryPath); os.IsNotExist(err) {
		c.logger.Info().Msg("Registry file does not exist, starting with empty registry")
		return nil
	}

	// Read registry file
	data, err := os.ReadFile(c.registryPath)
	if err != nil {
		c.logger.Error().Err(err).Msg("Failed to read registry file")
		return nil // Continue with empty registry
	}

	// Parse JSON
	var registry Registry
	if err := json.Unmarshal(data, &registry); err != nil {
		c.logger.Error().Err(err).Msg("Failed to parse registry file, starting with empty registry")
		return nil // Continue with empty registry
	}

	// Load runs into memory
	for _, run := range registry.Runs {
		c.activeRuns[run.ID] = run
	}

	c.logger.Info().
		Int("runs", len(c.activeRuns)).
		Msg("Registry loaded")

	return nil
}

// Close saves the registry and cleans up
func (c *Coordinator) Close() error {
	return c.saveRegistry()
}

// RegisterRun registers a new subagent run
func (c *Coordinator) RegisterRun(params RunParams) (string, error) {
	// Generate unique run ID
	runID, err := gonanoid.New()
	if err != nil {
		return "", fmt.Errorf("failed to generate run ID: %w", err)
	}

	// Create run record
	record := &RunRecord{
		ID:               runID,
		ParentSessionKey: params.ParentSessionKey,
		ChildSessionKey:  params.ChildSessionKey,
		Prompt:           params.Prompt,
		Status:           StatusPending,
		StartedAt:        time.Now().UnixMilli(),
		Metadata:         params.Metadata,
	}

	// Store in memory
	c.mu.Lock()
	c.activeRuns[runID] = record
	c.mu.Unlock()

	// Persist if autoSave enabled
	if c.autoSave {
		if err := c.saveRegistry(); err != nil {
			c.logger.Error().Err(err).Msg("Failed to save registry after registration")
		}
	}

	c.logger.Info().
		Str("runId", runID).
		Str("parentSession", params.ParentSessionKey).
		Str("childSession", params.ChildSessionKey).
		Msg("Run registered")

	// Emit event
	c.emit(EventRunRegistered, record)

	return runID, nil
}

// UpdateRunStatus updates the status of a run
func (c *Coordinator) UpdateRunStatus(runID string, status RunStatus, data map[string]interface{}) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	record, exists := c.activeRuns[runID]
	if !exists {
		return fmt.Errorf("run not found: %s", runID)
	}

	// Update status
	record.Status = status

	// Set completion timestamp for terminal states
	if status.IsTerminal() {
		now := time.Now().UnixMilli()
		record.CompletedAt = &now
	}

	// Store result or error
	if data != nil {
		if result, ok := data["result"]; ok {
			record.Result = result
		}
		if errMsg, ok := data["error"]; ok {
			record.Error = errMsg.(string)
		}
	}

	// Persist if autoSave enabled
	if c.autoSave {
		if err := c.saveRegistry(); err != nil {
			c.logger.Error().Err(err).Msg("Failed to save registry after status update")
		}
	}

	c.logger.Info().
		Str("runId", runID).
		Str("status", string(status)).
		Msg("Run status updated")

	// Emit event
	c.emit(EventRunUpdated, record)

	return nil
}

// HandleLifecycleEvent processes a lifecycle event
func (c *Coordinator) HandleLifecycleEvent(event LifecycleEvent) error {
	// Find run by child session key
	record := c.GetRunByChildSession(event.SessionKey)
	if record == nil {
		c.logger.Warn().
			Str("sessionKey", event.SessionKey).
			Msg("Run not found for lifecycle event")
		return nil // Graceful degradation
	}

	// Update status based on event type
	var newStatus RunStatus
	var data map[string]interface{}

	switch event.Type {
	case EventStart:
		newStatus = StatusRunning
	case EventEnd:
		newStatus = StatusCompleted
		if event.Data != nil {
			data = map[string]interface{}{"result": event.Data}
		}
	case EventError:
		newStatus = StatusFailed
		if event.Data != nil {
			data = map[string]interface{}{"error": fmt.Sprintf("%v", event.Data)}
		}
	case EventAbort:
		newStatus = StatusAborted
	default:
		return fmt.Errorf("unknown event type: %s", event.Type)
	}

	// Update run status
	if err := c.UpdateRunStatus(record.ID, newStatus, data); err != nil {
		return err
	}

	// Emit lifecycle event
	c.emit(EventLifecycle, map[string]interface{}{
		"event":  event,
		"record": record,
	})

	return nil
}

// GetRun retrieves a run by ID
func (c *Coordinator) GetRun(runID string) *RunRecord {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return c.activeRuns[runID]
}

// GetRunByChildSession retrieves a run by child session key
func (c *Coordinator) GetRunByChildSession(childSessionKey string) *RunRecord {
	c.mu.RLock()
	defer c.mu.RUnlock()

	for _, record := range c.activeRuns {
		if record.ChildSessionKey == childSessionKey {
			return record
		}
	}

	return nil
}

// ListChildren returns all direct children of a parent session
func (c *Coordinator) ListChildren(sessionKey string) []*RunRecord {
	c.mu.RLock()
	defer c.mu.RUnlock()

	children := []*RunRecord{}
	for _, record := range c.activeRuns {
		if record.ParentSessionKey == sessionKey {
			children = append(children, record)
		}
	}

	return children
}

// ListDescendants returns all nested descendants of a session
func (c *Coordinator) ListDescendants(sessionKey string) []*RunRecord {
	c.mu.RLock()
	defer c.mu.RUnlock()

	descendants := []*RunRecord{}
	c.addDescendants(sessionKey, &descendants)
	return descendants
}

// addDescendants recursively adds descendants
func (c *Coordinator) addDescendants(parentKey string, descendants *[]*RunRecord) {
	for _, record := range c.activeRuns {
		if record.ParentSessionKey == parentKey {
			*descendants = append(*descendants, record)
			c.addDescendants(record.ChildSessionKey, descendants)
		}
	}
}

// CountActiveRuns counts active runs for a parent session
func (c *Coordinator) CountActiveRuns(sessionKey string) int {
	c.mu.RLock()
	defer c.mu.RUnlock()

	count := 0
	for _, record := range c.activeRuns {
		if record.ParentSessionKey == sessionKey &&
			(record.Status == StatusPending || record.Status == StatusRunning) {
			count++
		}
	}

	return count
}

// CountDescendants counts all descendants of a session
func (c *Coordinator) CountDescendants(sessionKey string) int {
	return len(c.ListDescendants(sessionKey))
}

// Cleanup removes old completed runs
func (c *Coordinator) Cleanup(retentionMs int64) (int, error) {
	if retentionMs <= 0 {
		retentionMs = 7 * 24 * 60 * 60 * 1000 // 7 days default
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	cutoffTime := time.Now().UnixMilli() - retentionMs
	removed := 0

	for runID, record := range c.activeRuns {
		// Only remove terminal runs
		if !record.Status.IsTerminal() {
			continue
		}

		// Check if old enough
		if record.CompletedAt != nil && *record.CompletedAt < cutoffTime {
			delete(c.activeRuns, runID)
			removed++
		}
	}

	// Persist if autoSave enabled
	if c.autoSave && removed > 0 {
		if err := c.saveRegistry(); err != nil {
			c.logger.Error().Err(err).Msg("Failed to save registry after cleanup")
		}
	}

	c.logger.Info().Int("removed", removed).Msg("Cleanup completed")

	return removed, nil
}

// GetStats returns coordinator statistics
func (c *Coordinator) GetStats() Stats {
	c.mu.RLock()
	defer c.mu.RUnlock()

	stats := Stats{
		TotalRuns: len(c.activeRuns),
	}

	for _, record := range c.activeRuns {
		switch record.Status {
		case StatusPending, StatusRunning:
			stats.ActiveRuns++
		case StatusCompleted:
			stats.CompletedRuns++
		case StatusFailed:
			stats.FailedRuns++
		case StatusAborted:
			stats.AbortedRuns++
		}
	}

	return stats
}

// On registers an event handler
func (c *Coordinator) On(eventType string, handler EventHandler) {
	c.eventMu.Lock()
	defer c.eventMu.Unlock()

	c.eventHandlers[eventType] = append(c.eventHandlers[eventType], handler)
}

// Off removes all handlers for an event type
func (c *Coordinator) Off(eventType string) {
	c.eventMu.Lock()
	defer c.eventMu.Unlock()

	delete(c.eventHandlers, eventType)
}

// emit emits an event to all registered handlers
func (c *Coordinator) emit(eventType string, data interface{}) {
	c.eventMu.RLock()
	handlers := c.eventHandlers[eventType]
	c.eventMu.RUnlock()

	for _, handler := range handlers {
		handler(data)
	}
}

// saveRegistry persists the registry to disk using atomic writes
func (c *Coordinator) saveRegistry() error {
	// Create registry directory if it doesn't exist
	dir := filepath.Dir(c.registryPath)
	if err := os.MkdirAll(dir, 0700); err != nil {
		c.logger.Error().Err(err).Msg("Failed to create registry directory")
		return nil // Continue operation
	}

	// Build registry
	runs := make([]*RunRecord, 0, len(c.activeRuns))
	for _, record := range c.activeRuns {
		runs = append(runs, record)
	}

	registry := Registry{
		Version:     1,
		Runs:        runs,
		LastUpdated: time.Now().UnixMilli(),
	}

	// Marshal to JSON
	data, err := json.MarshalIndent(registry, "", "  ")
	if err != nil {
		c.logger.Error().Err(err).Msg("Failed to marshal registry")
		return nil // Continue operation
	}

	// Write to temp file
	tempPath := c.registryPath + ".tmp"
	if err := os.WriteFile(tempPath, data, 0600); err != nil {
		c.logger.Error().Err(err).Msg("Failed to write temp registry file")
		return nil // Continue operation
	}

	// Atomic rename
	if err := os.Rename(tempPath, c.registryPath); err != nil {
		c.logger.Error().Err(err).Msg("Failed to rename registry file")
		os.Remove(tempPath) // Clean up temp file
		return nil          // Continue operation
	}

	c.logger.Debug().Msg("Registry saved")
	return nil
}
