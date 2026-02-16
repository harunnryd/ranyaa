package routing

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
)

// RouteStorage handles persistence of routes to disk
type RouteStorage struct {
	filePath         string
	autoSaveInterval time.Duration
	stopCh           chan struct{}
	wg               sync.WaitGroup
	mu               sync.RWMutex
	config           StorageConfig
}

// StorageConfig holds configuration for RouteStorage
type StorageConfig struct {
	FilePath         string
	AutoSaveInterval time.Duration
	MaxRetries       int
	RetryDelay       time.Duration
}

// DefaultStorageConfig returns default storage configuration
func DefaultStorageConfig() StorageConfig {
	return StorageConfig{
		FilePath:         "./data/routes.json",
		AutoSaveInterval: 5 * time.Minute,
		MaxRetries:       3,
		RetryDelay:       1 * time.Second,
	}
}

// NewRouteStorage creates a new RouteStorage
func NewRouteStorage(config StorageConfig) *RouteStorage {
	return &RouteStorage{
		filePath:         config.FilePath,
		autoSaveInterval: config.AutoSaveInterval,
		stopCh:           make(chan struct{}),
		config:           config,
	}
}

// SaveRoutes saves routes to disk with atomic write
func (rs *RouteStorage) SaveRoutes(routes []*Route) error {
	rs.mu.Lock()
	defer rs.mu.Unlock()

	return rs.saveRoutesWithRetry(routes)
}

// saveRoutesWithRetry attempts to save routes with retry logic
func (rs *RouteStorage) saveRoutesWithRetry(routes []*Route) error {
	var lastErr error

	for attempt := 0; attempt < rs.config.MaxRetries; attempt++ {
		if attempt > 0 {
			log.Warn().
				Int("attempt", attempt+1).
				Int("maxRetries", rs.config.MaxRetries).
				Err(lastErr).
				Msg("Retrying route save")
			time.Sleep(rs.config.RetryDelay)
		}

		if err := rs.saveRoutesAtomic(routes); err != nil {
			lastErr = err
			continue
		}

		return nil
	}

	return fmt.Errorf("failed to save routes after %d attempts: %w", rs.config.MaxRetries, lastErr)
}

// saveRoutesAtomic performs atomic write using temp file + rename
func (rs *RouteStorage) saveRoutesAtomic(routes []*Route) error {
	// Ensure directory exists
	dir := filepath.Dir(rs.filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	// Marshal routes to JSON
	data, err := json.MarshalIndent(routes, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal routes: %w", err)
	}

	// Write to temporary file
	tempFile := rs.filePath + ".tmp"
	if err := os.WriteFile(tempFile, data, 0644); err != nil {
		return fmt.Errorf("failed to write temp file: %w", err)
	}

	// Atomic rename
	if err := os.Rename(tempFile, rs.filePath); err != nil {
		// Clean up temp file on failure
		os.Remove(tempFile)
		return fmt.Errorf("failed to rename temp file: %w", err)
	}

	log.Debug().
		Str("filePath", rs.filePath).
		Int("routeCount", len(routes)).
		Msg("Routes saved to disk")

	return nil
}

// LoadRoutes loads routes from disk
func (rs *RouteStorage) LoadRoutes() ([]*Route, error) {
	rs.mu.RLock()
	defer rs.mu.RUnlock()

	// Check if file exists
	if _, err := os.Stat(rs.filePath); os.IsNotExist(err) {
		log.Info().Str("filePath", rs.filePath).Msg("Routes file does not exist, starting with empty routes")
		return []*Route{}, nil
	}

	// Read file
	data, err := os.ReadFile(rs.filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read routes file: %w", err)
	}

	// Parse JSON
	var routes []*Route
	if err := json.Unmarshal(data, &routes); err != nil {
		return nil, fmt.Errorf("failed to parse routes JSON: %w", err)
	}

	log.Info().
		Str("filePath", rs.filePath).
		Int("routeCount", len(routes)).
		Msg("Routes loaded from disk")

	return routes, nil
}

// DeleteRoute removes a specific route from storage
func (rs *RouteStorage) DeleteRoute(routeID string, routes []*Route) error {
	// Filter out the route to delete
	filtered := make([]*Route, 0, len(routes))
	found := false

	for _, route := range routes {
		if route.ID == routeID {
			found = true
			continue
		}
		filtered = append(filtered, route)
	}

	if !found {
		return fmt.Errorf("route %s not found in storage", routeID)
	}

	// Save filtered routes
	return rs.SaveRoutes(filtered)
}

// StartAutoSave starts the auto-save mechanism
func (rs *RouteStorage) StartAutoSave(getRoutes func() []*Route) {
	if rs.autoSaveInterval <= 0 {
		log.Info().Msg("Auto-save disabled")
		return
	}

	rs.wg.Add(1)
	go rs.autoSaveLoop(getRoutes)

	log.Info().
		Dur("interval", rs.autoSaveInterval).
		Msg("Auto-save started")
}

// autoSaveLoop runs the auto-save loop
func (rs *RouteStorage) autoSaveLoop(getRoutes func() []*Route) {
	defer rs.wg.Done()

	ticker := time.NewTicker(rs.autoSaveInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			routes := getRoutes()
			if err := rs.SaveRoutes(routes); err != nil {
				log.Error().Err(err).Msg("Auto-save failed")
			} else {
				log.Debug().Int("routeCount", len(routes)).Msg("Auto-save completed")
			}

		case <-rs.stopCh:
			log.Info().Msg("Auto-save stopped")
			return
		}
	}
}

// StopAutoSave stops the auto-save mechanism
func (rs *RouteStorage) StopAutoSave() {
	close(rs.stopCh)
	rs.wg.Wait()
}

// Backup creates a backup of the current routes file
func (rs *RouteStorage) Backup() error {
	rs.mu.RLock()
	defer rs.mu.RUnlock()

	// Check if file exists
	if _, err := os.Stat(rs.filePath); os.IsNotExist(err) {
		return fmt.Errorf("routes file does not exist")
	}

	// Create backup filename with timestamp
	timestamp := time.Now().Format("20060102-150405")
	backupPath := fmt.Sprintf("%s.backup.%s", rs.filePath, timestamp)

	// Read original file
	data, err := os.ReadFile(rs.filePath)
	if err != nil {
		return fmt.Errorf("failed to read routes file: %w", err)
	}

	// Write backup file
	if err := os.WriteFile(backupPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write backup file: %w", err)
	}

	log.Info().
		Str("backupPath", backupPath).
		Msg("Routes backup created")

	return nil
}

// GetFilePath returns the storage file path
func (rs *RouteStorage) GetFilePath() string {
	return rs.filePath
}
