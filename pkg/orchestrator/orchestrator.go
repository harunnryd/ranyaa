package orchestrator

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
)

// Orchestrator coordinates multiple agents and manages their lifecycle
type Orchestrator struct {
	registry      *Registry
	agents        map[string]*AgentInstance
	store         StateStore
	mu            sync.RWMutex
	maxConcurrent int
	logger        Logger
}

// AgentInstance represents a running agent instance
type AgentInstance struct {
	ID        string
	Config    AgentConfig
	Status    AgentStatus
	StartedAt time.Time
	Context   context.Context
	Cancel    context.CancelFunc
}

// AgentStatus represents the current status of an agent
type AgentStatus string

const (
	AgentStatusIdle    AgentStatus = "idle"
	AgentStatusRunning AgentStatus = "running"
	AgentStatusStopped AgentStatus = "stopped"
	AgentStatusFailed  AgentStatus = "failed"
)

// Logger interface for logging
type Logger interface {
	Info(msg string, fields ...interface{})
	Error(msg string, err error, fields ...interface{})
	Debug(msg string, fields ...interface{})
}

// New creates a new Orchestrator instance
func New(opts ...Option) *Orchestrator {
	o := &Orchestrator{
		registry:      NewRegistry(),
		agents:        make(map[string]*AgentInstance),
		maxConcurrent: 5, // default max concurrent agents
	}

	for _, opt := range opts {
		opt(o)
	}

	return o
}

// Option is a functional option for configuring the Orchestrator
type Option func(*Orchestrator)

// WithMaxConcurrent sets the maximum number of concurrent agents
func WithMaxConcurrent(max int) Option {
	return func(o *Orchestrator) {
		o.maxConcurrent = max
	}
}

// WithLogger sets the logger for the orchestrator
func WithLogger(logger Logger) Option {
	return func(o *Orchestrator) {
		o.logger = logger
	}
}

// WithStore sets the state store for the orchestrator
func WithStore(store StateStore) Option {
	return func(o *Orchestrator) {
		o.store = store
	}
}

// Recover loads persisted state and reconciles statuses
func (o *Orchestrator) Recover() error {
	if o.store == nil {
		return nil
	}

	o.mu.Lock()
	defer o.mu.Unlock()

	instances, err := o.store.List()
	if err != nil {
		return fmt.Errorf("failed to list stored instances: %w", err)
	}

	for _, instance := range instances {
		// If an instance was apparently running but we just started, it must have crashed/been killed.
		if instance.Status == AgentStatusRunning {
			instance.Status = AgentStatusFailed
			if err := o.store.Save(instance); err != nil {
				if o.logger != nil {
					o.logger.Error("Failed to update crashed instance status", err, "id", instance.ID)
				}
			}
		}

		// Rehydrate context for viewing purposes (though cancelled)
		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Immediately cancel since it's not actually running
		instance.Context = ctx
		instance.Cancel = cancel

		o.agents[instance.ID] = instance
	}

	if o.logger != nil {
		o.logger.Info("Recovered state from store", "count", len(instances))
	}

	return nil
}

// RegisterAgent registers an agent configuration
func (o *Orchestrator) RegisterAgent(config AgentConfig) error {
	if err := config.Validate(); err != nil {
		return fmt.Errorf("invalid agent config: %w", err)
	}

	return o.registry.Register(config)
}

// GetAgent retrieves an agent configuration by ID
func (o *Orchestrator) GetAgent(id string) (AgentConfig, error) {
	return o.registry.Get(id)
}

// ListAgents returns all registered agent configurations
func (o *Orchestrator) ListAgents() []AgentConfig {
	return o.registry.List()
}

// CreateInstance creates a new agent instance from a registered configuration
func (o *Orchestrator) CreateInstance(agentID string) (*AgentInstance, error) {
	config, err := o.registry.Get(agentID)
	if err != nil {
		return nil, fmt.Errorf("agent not found: %w", err)
	}

	instanceID := uuid.New().String()
	ctx, cancel := context.WithCancel(context.Background())

	instance := &AgentInstance{
		ID:        instanceID,
		Config:    config,
		Status:    AgentStatusIdle,
		StartedAt: time.Now(),
		Context:   ctx,
		Cancel:    cancel,
	}

	o.mu.Lock()
	o.agents[instanceID] = instance

	if o.store != nil {
		if err := o.store.Save(instance); err != nil {
			o.mu.Unlock()
			return nil, fmt.Errorf("failed to persist instance: %w", err)
		}
	}
	o.mu.Unlock()

	if o.logger != nil {
		o.logger.Info("Agent instance created",
			"instance_id", instanceID,
			"agent_id", agentID,
			"role", config.Role)
	}

	return instance, nil
}

// GetInstance retrieves an agent instance by ID
func (o *Orchestrator) GetInstance(instanceID string) (*AgentInstance, error) {
	o.mu.RLock()
	defer o.mu.RUnlock()

	instance, ok := o.agents[instanceID]
	if !ok {
		return nil, fmt.Errorf("instance not found: %s", instanceID)
	}

	return instance, nil
}

// UpdateInstanceStatus updates the status of an agent instance
func (o *Orchestrator) UpdateInstanceStatus(instanceID string, status AgentStatus) error {
	o.mu.Lock()
	defer o.mu.Unlock()

	instance, ok := o.agents[instanceID]
	if !ok {
		return fmt.Errorf("instance not found: %s", instanceID)
	}

	instance.Status = status

	if o.store != nil {
		if err := o.store.Save(instance); err != nil {
			return fmt.Errorf("failed to persist instance status: %w", err)
		}
	}

	if o.logger != nil {
		o.logger.Info("Agent instance status updated",
			"instance_id", instanceID,
			"status", status)
	}

	return nil
}

// StopInstance stops an agent instance
func (o *Orchestrator) StopInstance(instanceID string) error {
	o.mu.Lock()
	defer o.mu.Unlock()

	instance, ok := o.agents[instanceID]
	if !ok {
		return fmt.Errorf("instance not found: %s", instanceID)
	}

	instance.Cancel()
	instance.Status = AgentStatusStopped

	if o.logger != nil {
		o.logger.Info("Agent instance stopped",
			"instance_id", instanceID)
	}

	return nil
}

// GetActiveInstances returns the number of currently running instances
func (o *Orchestrator) GetActiveInstances() int {
	o.mu.RLock()
	defer o.mu.RUnlock()

	count := 0
	for _, instance := range o.agents {
		if instance.Status == AgentStatusRunning {
			count++
		}
	}

	return count
}

// CanSpawnMore checks if more agents can be spawned based on max concurrent limit
func (o *Orchestrator) CanSpawnMore() bool {
	return o.GetActiveInstances() < o.maxConcurrent
}

// Cleanup removes stopped instances from memory
func (o *Orchestrator) Cleanup() {
	o.mu.Lock()
	defer o.mu.Unlock()

	for id, instance := range o.agents {
		if instance.Status == AgentStatusStopped || instance.Status == AgentStatusFailed {
			if o.store != nil {
				if err := o.store.Delete(id); err != nil {
					if o.logger != nil {
						o.logger.Error("Failed to delete instance from store", err, "id", id)
					}
				}
			}
			delete(o.agents, id)
			if o.logger != nil {
				o.logger.Debug("Cleaned up agent instance", "instance_id", id)
			}
		}
	}
}

// Shutdown gracefully shuts down all agent instances
func (o *Orchestrator) Shutdown(ctx context.Context) error {
	o.mu.Lock()
	defer o.mu.Unlock()

	if o.logger != nil {
		o.logger.Info("Shutting down orchestrator", "active_instances", len(o.agents))
	}

	for id, instance := range o.agents {
		instance.Cancel()
		instance.Status = AgentStatusStopped
		if o.store != nil {
			_ = o.store.Save(instance)
		}
		if o.logger != nil {
			o.logger.Debug("Stopped agent instance", "instance_id", id)
		}
	}

	return nil
}
