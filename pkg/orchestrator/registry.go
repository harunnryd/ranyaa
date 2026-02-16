package orchestrator

import (
	"fmt"
	"sync"
)

// Registry manages agent configurations
type Registry struct {
	agents map[string]AgentConfig
	mu     sync.RWMutex
}

// NewRegistry creates a new agent registry
func NewRegistry() *Registry {
	return &Registry{
		agents: make(map[string]AgentConfig),
	}
}

// Register registers a new agent configuration
func (r *Registry) Register(config AgentConfig) error {
	if err := config.Validate(); err != nil {
		return fmt.Errorf("invalid agent config: %w", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.agents[config.ID]; exists {
		return fmt.Errorf("agent already registered: %s", config.ID)
	}

	r.agents[config.ID] = config
	return nil
}

// Unregister removes an agent configuration
func (r *Registry) Unregister(id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.agents[id]; !exists {
		return fmt.Errorf("agent not found: %s", id)
	}

	delete(r.agents, id)
	return nil
}

// Get retrieves an agent configuration by ID
func (r *Registry) Get(id string) (AgentConfig, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	config, exists := r.agents[id]
	if !exists {
		return AgentConfig{}, fmt.Errorf("agent not found: %s", id)
	}

	return config, nil
}

// List returns all registered agent configurations
func (r *Registry) List() []AgentConfig {
	r.mu.RLock()
	defer r.mu.RUnlock()

	configs := make([]AgentConfig, 0, len(r.agents))
	for _, config := range r.agents {
		configs = append(configs, config)
	}

	return configs
}

// ListByRole returns all agents with a specific role
func (r *Registry) ListByRole(role AgentRole) []AgentConfig {
	r.mu.RLock()
	defer r.mu.RUnlock()

	configs := make([]AgentConfig, 0)
	for _, config := range r.agents {
		if config.Role == role {
			configs = append(configs, config)
		}
	}

	return configs
}

// Exists checks if an agent is registered
func (r *Registry) Exists(id string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	_, exists := r.agents[id]
	return exists
}

// Count returns the number of registered agents
func (r *Registry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return len(r.agents)
}

// Clear removes all registered agents
func (r *Registry) Clear() {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.agents = make(map[string]AgentConfig)
}
