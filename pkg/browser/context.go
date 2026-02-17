package browser

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

// BrowserServerContext manages all browser profiles
type BrowserServerContext struct {
	config     *BrowserConfig
	profiles   map[string]*ProfileContext
	configPath string
	watcher    *fsnotify.Watcher
	mu         sync.RWMutex
	stopChan   chan struct{}
	baseDir    string
}

// NewBrowserServerContext creates a new browser server context
func NewBrowserServerContext(configPath string, baseDir string) (*BrowserServerContext, error) {
	bsc := &BrowserServerContext{
		profiles:   make(map[string]*ProfileContext),
		configPath: configPath,
		stopChan:   make(chan struct{}),
		baseDir:    baseDir,
	}

	return bsc, nil
}

// Initialize loads config and creates profiles
func (bsc *BrowserServerContext) Initialize(ctx context.Context) error {
	// Load configuration
	if err := bsc.loadConfig(); err != nil {
		return err
	}

	// Create profile contexts
	if err := bsc.createProfiles(); err != nil {
		return err
	}

	// Start config watcher
	if err := bsc.watchConfig(); err != nil {
		// Log warning but don't fail
		fmt.Printf("Warning: Failed to start config watcher: %v\n", err)
	}

	return nil
}

// loadConfig reads and validates configuration from file
func (bsc *BrowserServerContext) loadConfig() error {
	// Check if config file exists
	if _, err := os.Stat(bsc.configPath); os.IsNotExist(err) {
		// Create default config
		bsc.config = &BrowserConfig{
			Profiles: []BrowserProfile{},
			Pool: PoolConfig{
				MaxPages:           10,
				MaxMemoryMB:        1024,
				IdlePageTimeout:    300,
				IdleBrowserTimeout: 600,
			},
			Security: SecurityConfig{
				AllowFileUrls:      false,
				AllowLocalhostUrls: false,
				AllowedDomains:     []string{},
				BlockedDomains:     []string{},
			},
		}
		return nil
	}

	// Read config file
	data, err := os.ReadFile(bsc.configPath)
	if err != nil {
		return &BrowserError{
			Code:    ErrCodeConfiguration,
			Message: fmt.Sprintf("Failed to read config file: %v", err),
		}
	}

	// Parse JSON
	var config BrowserConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return &BrowserError{
			Code:    ErrCodeConfiguration,
			Message: fmt.Sprintf("Failed to parse config file: %v", err),
		}
	}

	// Validate configuration
	if err := bsc.validateConfig(&config); err != nil {
		return err
	}

	bsc.config = &config

	return nil
}

// validateConfig validates the configuration
func (bsc *BrowserServerContext) validateConfig(config *BrowserConfig) error {
	// Track used ports to ensure uniqueness
	usedPorts := make(map[int]bool)

	// Validate each profile
	for i := range config.Profiles {
		profile := &config.Profiles[i]

		// Validate profile
		if err := ValidateProfileConfig(profile); err != nil {
			fmt.Printf("Warning: Invalid profile %s: %v\n", profile.Name, err)
			continue
		}

		// Ensure unique CDP port
		if err := EnsureUniquePort(profile, usedPorts); err != nil {
			return &BrowserError{
				Code:    ErrCodeConfiguration,
				Message: fmt.Sprintf("Failed to assign unique port to profile %s: %v", profile.Name, err),
			}
		}
	}

	// Validate pool config
	if config.Pool.MaxPages <= 0 {
		config.Pool.MaxPages = 10
	}
	if config.Pool.IdlePageTimeout <= 0 {
		config.Pool.IdlePageTimeout = 300
	}
	if config.Pool.IdleBrowserTimeout <= 0 {
		config.Pool.IdleBrowserTimeout = 600
	}

	return nil
}

// createProfiles creates ProfileContext for each profile
func (bsc *BrowserServerContext) createProfiles() error {
	bsc.mu.Lock()
	defer bsc.mu.Unlock()

	for i := range bsc.config.Profiles {
		profile := &bsc.config.Profiles[i]

		// Resolve profile
		resolved, err := ResolveProfile(profile, bsc.baseDir)
		if err != nil {
			fmt.Printf("Warning: Failed to resolve profile %s: %v\n", profile.Name, err)
			continue
		}

		// Create profile context
		pc := NewProfileContext(resolved, bsc.config.Pool, bsc.config.Security)

		bsc.profiles[profile.Name] = pc
	}

	return nil
}

// GetProfile retrieves a profile by name
func (bsc *BrowserServerContext) GetProfile(name string) (*ProfileContext, error) {
	bsc.mu.RLock()
	defer bsc.mu.RUnlock()

	pc, exists := bsc.profiles[name]
	if !exists {
		return nil, &BrowserError{
			Code:    ErrCodeNotFound,
			Message: fmt.Sprintf("Profile not found: %s", name),
		}
	}

	return pc, nil
}

// ListProfiles returns all profile names
func (bsc *BrowserServerContext) ListProfiles() []string {
	bsc.mu.RLock()
	defer bsc.mu.RUnlock()

	names := make([]string, 0, len(bsc.profiles))
	for name := range bsc.profiles {
		names = append(names, name)
	}

	return names
}

// GetAllProfiles returns all profile contexts
func (bsc *BrowserServerContext) GetAllProfiles() map[string]*ProfileContext {
	bsc.mu.RLock()
	defer bsc.mu.RUnlock()

	// Return copy
	profiles := make(map[string]*ProfileContext, len(bsc.profiles))
	for name, pc := range bsc.profiles {
		profiles[name] = pc
	}

	return profiles
}

// watchConfig watches the config file for changes
func (bsc *BrowserServerContext) watchConfig() error {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}

	bsc.watcher = watcher

	// Watch config file directory
	configDir := filepath.Dir(bsc.configPath)
	if err := watcher.Add(configDir); err != nil {
		return err
	}

	// Start watch goroutine
	go func() {
		debounce := time.NewTimer(0)
		<-debounce.C // drain initial timer

		for {
			select {
			case event, ok := <-watcher.Events:
				if !ok {
					return
				}

				// Check if it's our config file
				if filepath.Clean(event.Name) == filepath.Clean(bsc.configPath) {
					if event.Op&fsnotify.Write == fsnotify.Write {
						// Debounce: wait 500ms before reloading
						debounce.Reset(500 * time.Millisecond)
						go func() {
							<-debounce.C
							if err := bsc.reloadConfig(context.Background()); err != nil {
								fmt.Printf("Error reloading config: %v\n", err)
							} else {
								fmt.Println("Config reloaded successfully")
							}
						}()
					}
				}

			case err, ok := <-watcher.Errors:
				if !ok {
					return
				}
				fmt.Printf("Config watcher error: %v\n", err)

			case <-bsc.stopChan:
				return
			}
		}
	}()

	return nil
}

// reloadConfig reloads the configuration and updates profiles
func (bsc *BrowserServerContext) reloadConfig(ctx context.Context) error {
	// Load new config
	if err := bsc.loadConfig(); err != nil {
		return err
	}

	bsc.mu.Lock()
	defer bsc.mu.Unlock()

	// Track which profiles exist in new config
	newProfiles := make(map[string]*BrowserProfile)
	for i := range bsc.config.Profiles {
		profile := &bsc.config.Profiles[i]
		newProfiles[profile.Name] = profile
	}

	// Remove profiles that no longer exist
	for name, pc := range bsc.profiles {
		if _, exists := newProfiles[name]; !exists {
			fmt.Printf("Removing profile: %s\n", name)
			_ = pc.Cleanup()
			delete(bsc.profiles, name)
		}
	}

	// Add new profiles
	for name, profile := range newProfiles {
		if _, exists := bsc.profiles[name]; !exists {
			fmt.Printf("Adding profile: %s\n", name)

			// Resolve profile
			resolved, err := ResolveProfile(profile, bsc.baseDir)
			if err != nil {
				fmt.Printf("Warning: Failed to resolve profile %s: %v\n", name, err)
				continue
			}

			// Create profile context
			pc := NewProfileContext(resolved, bsc.config.Pool, bsc.config.Security)
			bsc.profiles[name] = pc
		}
	}

	return nil
}

// Shutdown stops all browsers and cleans up
func (bsc *BrowserServerContext) Shutdown(ctx context.Context) error {
	// Stop config watcher
	close(bsc.stopChan)
	if bsc.watcher != nil {
		bsc.watcher.Close()
	}

	bsc.mu.Lock()
	defer bsc.mu.Unlock()

	// Stop all profiles
	for name, pc := range bsc.profiles {
		fmt.Printf("Stopping profile: %s\n", name)
		if err := pc.Cleanup(); err != nil {
			fmt.Printf("Error stopping profile %s: %v\n", name, err)
		}
	}

	bsc.profiles = make(map[string]*ProfileContext)

	return nil
}

// GetMetrics returns aggregated metrics for all profiles
func (bsc *BrowserServerContext) GetMetrics() *BrowserMetrics {
	bsc.mu.RLock()
	defer bsc.mu.RUnlock()

	metrics := &BrowserMetrics{
		ActiveProfiles: 0,
		ActivePages:    0,
		TotalOps:       0,
		Latency: LatencyMetrics{
			Avg: 0,
			Min: 0,
			Max: 0,
			P95: 0,
			P99: 0,
		},
		Errors:         make(map[string]int64),
		ProfileMetrics: make(map[string]ProfileMetrics),
	}

	for name, pc := range bsc.profiles {
		if pc.IsAvailable() {
			metrics.ActiveProfiles++
		}

		pm := pc.GetMetrics()
		metrics.ActivePages += pm.ActivePages
		metrics.TotalOps += pm.TotalOps

		metrics.ProfileMetrics[name] = *pm
	}

	return metrics
}

// GetConfig returns the current configuration
func (bsc *BrowserServerContext) GetConfig() *BrowserConfig {
	bsc.mu.RLock()
	defer bsc.mu.RUnlock()

	// Return copy
	configCopy := *bsc.config
	return &configCopy
}

// SaveConfig saves the current configuration to file
func (bsc *BrowserServerContext) SaveConfig() error {
	bsc.mu.RLock()
	config := bsc.config
	bsc.mu.RUnlock()

	// Marshal to JSON
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return &BrowserError{
			Code:    ErrCodeConfiguration,
			Message: fmt.Sprintf("Failed to marshal config: %v", err),
		}
	}

	// Write to file
	if err := os.WriteFile(bsc.configPath, data, 0644); err != nil {
		return &BrowserError{
			Code:    ErrCodeConfiguration,
			Message: fmt.Sprintf("Failed to write config file: %v", err),
		}
	}

	return nil
}

// AddProfile adds a new profile
func (bsc *BrowserServerContext) AddProfile(ctx context.Context, profile *BrowserProfile) error {
	// Validate profile
	if err := ValidateProfileConfig(profile); err != nil {
		return err
	}

	bsc.mu.Lock()
	defer bsc.mu.Unlock()

	// Check if profile already exists
	if _, exists := bsc.profiles[profile.Name]; exists {
		return &BrowserError{
			Code:    ErrCodeValidation,
			Message: fmt.Sprintf("Profile already exists: %s", profile.Name),
		}
	}

	// Ensure unique port
	usedPorts := make(map[int]bool)
	for _, p := range bsc.config.Profiles {
		usedPorts[p.CDPPort] = true
	}
	if err := EnsureUniquePort(profile, usedPorts); err != nil {
		return err
	}

	// Add to config
	bsc.config.Profiles = append(bsc.config.Profiles, *profile)

	// Resolve profile
	resolved, err := ResolveProfile(profile, bsc.baseDir)
	if err != nil {
		return err
	}

	// Create profile context
	pc := NewProfileContext(resolved, bsc.config.Pool, bsc.config.Security)
	bsc.profiles[profile.Name] = pc

	// Save config
	return bsc.SaveConfig()
}

// RemoveProfile removes a profile
func (bsc *BrowserServerContext) RemoveProfile(ctx context.Context, name string) error {
	bsc.mu.Lock()
	defer bsc.mu.Unlock()

	// Check if profile exists
	pc, exists := bsc.profiles[name]
	if !exists {
		return &BrowserError{
			Code:    ErrCodeNotFound,
			Message: fmt.Sprintf("Profile not found: %s", name),
		}
	}

	// Stop profile
	if err := pc.Cleanup(); err != nil {
		return err
	}

	// Remove from profiles map
	delete(bsc.profiles, name)

	// Remove from config
	newProfiles := make([]BrowserProfile, 0)
	for _, p := range bsc.config.Profiles {
		if p.Name != name {
			newProfiles = append(newProfiles, p)
		}
	}
	bsc.config.Profiles = newProfiles

	// Save config
	return bsc.SaveConfig()
}

// GetStatus returns the server status
func (bsc *BrowserServerContext) GetStatus() map[string]interface{} {
	bsc.mu.RLock()
	defer bsc.mu.RUnlock()

	profiles := make([]map[string]interface{}, 0)
	for name, pc := range bsc.profiles {
		profiles = append(profiles, map[string]interface{}{
			"name":      name,
			"available": pc.IsAvailable(),
			"metrics":   pc.GetMetrics(),
		})
	}

	return map[string]interface{}{
		"status":   "running",
		"profiles": profiles,
		"config":   bsc.config,
	}
}
