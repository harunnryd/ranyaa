package toolexecutor

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/rs/zerolog/log"
)

// AllowlistEntry represents an entry in the exec allowlist
type AllowlistEntry struct {
	Command string   `json:"command"`
	Args    []string `json:"args,omitempty"`
	Pattern string   `json:"pattern,omitempty"` // Glob pattern
	Reason  string   `json:"reason,omitempty"`
	AddedAt string   `json:"added_at"`
}

// AllowlistManager manages the exec approvals allowlist
type AllowlistManager struct {
	filePath string
	entries  []AllowlistEntry
	mu       sync.RWMutex
}

// NewAllowlistManager creates a new allowlist manager
func NewAllowlistManager(filePath string) (*AllowlistManager, error) {
	if filePath == "" {
		// Default to ~/.ranya/exec-approvals.json
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("failed to get home directory: %w", err)
		}
		filePath = filepath.Join(home, ".ranya", "exec-approvals.json")
	}

	am := &AllowlistManager{
		filePath: filePath,
		entries:  []AllowlistEntry{},
	}

	// Load existing allowlist
	if err := am.Load(); err != nil {
		// If file doesn't exist, that's okay
		if !os.IsNotExist(err) {
			return nil, fmt.Errorf("failed to load allowlist: %w", err)
		}
		log.Info().Str("path", filePath).Msg("Allowlist file does not exist, will create on first save")
	}

	return am, nil
}

// Load loads the allowlist from file
func (am *AllowlistManager) Load() error {
	am.mu.Lock()
	defer am.mu.Unlock()

	data, err := os.ReadFile(am.filePath)
	if err != nil {
		return err
	}

	var entries []AllowlistEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		return fmt.Errorf("failed to parse allowlist: %w", err)
	}

	am.entries = entries

	log.Info().
		Str("path", am.filePath).
		Int("count", len(entries)).
		Msg("Allowlist loaded")

	return nil
}

// Save saves the allowlist to file
func (am *AllowlistManager) Save() error {
	am.mu.RLock()
	defer am.mu.RUnlock()

	// Ensure directory exists
	dir := filepath.Dir(am.filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	data, err := json.MarshalIndent(am.entries, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal allowlist: %w", err)
	}

	if err := os.WriteFile(am.filePath, data, 0644); err != nil {
		return fmt.Errorf("failed to write allowlist: %w", err)
	}

	log.Info().
		Str("path", am.filePath).
		Int("count", len(am.entries)).
		Msg("Allowlist saved")

	return nil
}

// Add adds a command to the allowlist
func (am *AllowlistManager) Add(entry AllowlistEntry) error {
	am.mu.Lock()
	defer am.mu.Unlock()

	// Validate entry
	if entry.Command == "" && entry.Pattern == "" {
		return fmt.Errorf("either command or pattern must be specified")
	}

	// Check if already exists
	for _, existing := range am.entries {
		if existing.Command == entry.Command &&
			strings.Join(existing.Args, " ") == strings.Join(entry.Args, " ") &&
			existing.Pattern == entry.Pattern {
			log.Debug().
				Str("command", entry.Command).
				Str("pattern", entry.Pattern).
				Msg("Entry already exists in allowlist")
			return nil
		}
	}

	am.entries = append(am.entries, entry)

	log.Info().
		Str("command", entry.Command).
		Str("pattern", entry.Pattern).
		Msg("Added to allowlist")

	return nil
}

// Remove removes a command from the allowlist
func (am *AllowlistManager) Remove(command string, args []string) error {
	am.mu.Lock()
	defer am.mu.Unlock()

	argsStr := strings.Join(args, " ")
	found := false

	newEntries := []AllowlistEntry{}
	for _, entry := range am.entries {
		entryArgsStr := strings.Join(entry.Args, " ")
		if entry.Command == command && entryArgsStr == argsStr {
			found = true
			log.Info().
				Str("command", command).
				Strs("args", args).
				Msg("Removed from allowlist")
			continue
		}
		newEntries = append(newEntries, entry)
	}

	if !found {
		return fmt.Errorf("entry not found in allowlist")
	}

	am.entries = newEntries

	return nil
}

// IsAllowed checks if a command is in the allowlist
func (am *AllowlistManager) IsAllowed(command string, args []string) bool {
	am.mu.RLock()
	defer am.mu.RUnlock()

	commandStr := command
	if len(args) > 0 {
		commandStr = command + " " + strings.Join(args, " ")
	}

	for _, entry := range am.entries {
		// Exact match
		if entry.Command == command {
			// If entry has no args, match any args
			if len(entry.Args) == 0 {
				return true
			}
			// If entry has args, match exactly
			if strings.Join(entry.Args, " ") == strings.Join(args, " ") {
				return true
			}
		}

		// Pattern match
		if entry.Pattern != "" {
			if matchGlob(entry.Pattern, commandStr) {
				return true
			}
		}
	}

	return false
}

// List returns all entries in the allowlist
func (am *AllowlistManager) List() []AllowlistEntry {
	am.mu.RLock()
	defer am.mu.RUnlock()

	// Return a copy to prevent external modification
	entries := make([]AllowlistEntry, len(am.entries))
	copy(entries, am.entries)

	return entries
}

// Clear removes all entries from the allowlist
func (am *AllowlistManager) Clear() error {
	am.mu.Lock()
	defer am.mu.Unlock()

	am.entries = []AllowlistEntry{}

	log.Info().Msg("Allowlist cleared")

	return nil
}

// Count returns the number of entries in the allowlist
func (am *AllowlistManager) Count() int {
	am.mu.RLock()
	defer am.mu.RUnlock()

	return len(am.entries)
}

// matchGlob performs simple glob pattern matching
// Supports * (any characters) and ? (single character)
func matchGlob(pattern, str string) bool {
	// Simple implementation - can be enhanced with filepath.Match
	// For now, support basic wildcards
	if pattern == "*" {
		return true
	}

	// Use filepath.Match for proper glob matching
	matched, err := filepath.Match(pattern, str)
	if err != nil {
		log.Warn().
			Err(err).
			Str("pattern", pattern).
			Msg("Invalid glob pattern")
		return false
	}

	return matched
}
