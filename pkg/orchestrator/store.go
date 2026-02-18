package orchestrator

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// StateStore defines the interface for persisting agent state
type StateStore interface {
	Save(instance *AgentInstance) error
	Delete(id string) error
	List() ([]*AgentInstance, error)
	Get(id string) (*AgentInstance, error)
}

// FileStore implements StateStore using the local filesystem
type FileStore struct {
	baseDir string
	mu      sync.RWMutex
}

// persistedInstance represents the serializable state of an AgentInstance
type persistedInstance struct {
	ID        string      `json:"id"`
	Config    AgentConfig `json:"config"`
	Status    AgentStatus `json:"status"`
	StartedAt time.Time   `json:"started_at"`
}

// NewFileStore creates a new file-based state store
func NewFileStore(baseDir string) (*FileStore, error) {
	if err := os.MkdirAll(baseDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create store directory: %w", err)
	}
	return &FileStore{baseDir: baseDir}, nil
}

// Save persists an agent instance to disk
func (s *FileStore) Save(instance *AgentInstance) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	p := persistedInstance{
		ID:        instance.ID,
		Config:    instance.Config,
		Status:    instance.Status,
		StartedAt: instance.StartedAt,
	}

	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal instance: %w", err)
	}

	path := filepath.Join(s.baseDir, instance.ID+".json")
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("failed to write instance file: %w", err)
	}

	return nil
}

// Delete removes an agent instance from disk
func (s *FileStore) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	path := filepath.Join(s.baseDir, id+".json")
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove instance file: %w", err)
	}
	return nil
}

// Get retrieves a single agent instance state
func (s *FileStore) Get(id string) (*AgentInstance, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	path := filepath.Join(s.baseDir, id+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read instance file: %w", err)
	}

	var p persistedInstance
	if err := json.Unmarshal(data, &p); err != nil {
		return nil, fmt.Errorf("failed to unmarshal instance: %w", err)
	}

	// Rehydrate AgentInstance (Context/Cancel are nil)
	return &AgentInstance{
		ID:        p.ID,
		Config:    p.Config,
		Status:    p.Status,
		StartedAt: p.StartedAt,
	}, nil
}

// List returns all persisted agent instances
func (s *FileStore) List() ([]*AgentInstance, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	entries, err := os.ReadDir(s.baseDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read store directory: %w", err)
	}

	var instances []*AgentInstance
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}

		id := entry.Name()[:len(entry.Name())-5] // remove .json
		instance, err := s.Get(id)
		if err != nil {
			// Skip corrupted files but log/continue?
			// For now, we propagate error to be safe, or we could skip.
			// Let's attempt to read others.
			continue
		}
		instances = append(instances, instance)
	}

	return instances, nil
}
