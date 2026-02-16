package orchestrator

import (
	"testing"
)

// Unit Tests for Registry

func TestNewRegistry(t *testing.T) {
	t.Run("creates new registry with empty agents map", func(t *testing.T) {
		r := NewRegistry()

		if r == nil {
			t.Fatal("expected registry to be created")
		}

		if r.agents == nil {
			t.Error("expected agents map to be initialized")
		}

		if r.Count() != 0 {
			t.Errorf("expected empty registry, got %d agents", r.Count())
		}
	})
}

func TestRegister(t *testing.T) {
	t.Run("registers valid agent", func(t *testing.T) {
		r := NewRegistry()

		config := AgentConfig{
			ID:          "test-agent",
			Name:        "Test Agent",
			Role:        RoleExecutor,
			Model:       "claude-sonnet-4",
			Temperature: 0.7,
			MaxTokens:   4096,
		}

		err := r.Register(config)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		if r.Count() != 1 {
			t.Errorf("expected 1 agent, got %d", r.Count())
		}

		if !r.Exists("test-agent") {
			t.Error("expected agent to exist")
		}
	})

	t.Run("rejects invalid agent config", func(t *testing.T) {
		r := NewRegistry()

		config := AgentConfig{
			ID:   "test-agent",
			Name: "Test Agent",
			// Missing required fields
		}

		err := r.Register(config)
		if err == nil {
			t.Error("expected error for invalid config")
		}

		if r.Count() != 0 {
			t.Error("expected agent not to be registered")
		}
	})

	t.Run("rejects duplicate agent ID", func(t *testing.T) {
		r := NewRegistry()

		config := AgentConfig{
			ID:          "test-agent",
			Name:        "Test Agent",
			Role:        RoleExecutor,
			Model:       "claude-sonnet-4",
			Temperature: 0.7,
		}

		_ = r.Register(config)
		err := r.Register(config)

		if err == nil {
			t.Error("expected error for duplicate agent ID")
		}

		if r.Count() != 1 {
			t.Errorf("expected 1 agent, got %d", r.Count())
		}
	})

	t.Run("registers multiple agents with different IDs", func(t *testing.T) {
		r := NewRegistry()

		config1 := AgentConfig{
			ID:          "agent-1",
			Name:        "Agent 1",
			Role:        RoleExecutor,
			Model:       "claude-sonnet-4",
			Temperature: 0.7,
		}

		config2 := AgentConfig{
			ID:          "agent-2",
			Name:        "Agent 2",
			Role:        RoleCaptain,
			Model:       "claude-opus-4",
			Temperature: 0.5,
		}

		err1 := r.Register(config1)
		err2 := r.Register(config2)

		if err1 != nil || err2 != nil {
			t.Fatalf("expected no errors, got %v, %v", err1, err2)
		}

		if r.Count() != 2 {
			t.Errorf("expected 2 agents, got %d", r.Count())
		}
	})
}

func TestGet(t *testing.T) {
	t.Run("retrieves registered agent", func(t *testing.T) {
		r := NewRegistry()

		config := AgentConfig{
			ID:          "test-agent",
			Name:        "Test Agent",
			Role:        RoleExecutor,
			Model:       "claude-sonnet-4",
			Temperature: 0.7,
			MaxTokens:   4096,
		}

		_ = r.Register(config)

		retrieved, err := r.Get("test-agent")
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		if retrieved.ID != config.ID {
			t.Errorf("expected ID %s, got %s", config.ID, retrieved.ID)
		}

		if retrieved.Name != config.Name {
			t.Errorf("expected Name %s, got %s", config.Name, retrieved.Name)
		}

		if retrieved.Role != config.Role {
			t.Errorf("expected Role %s, got %s", config.Role, retrieved.Role)
		}

		if retrieved.Model != config.Model {
			t.Errorf("expected Model %s, got %s", config.Model, retrieved.Model)
		}
	})

	t.Run("fails for nonexistent agent", func(t *testing.T) {
		r := NewRegistry()

		_, err := r.Get("nonexistent")
		if err == nil {
			t.Error("expected error for nonexistent agent")
		}
	})

	t.Run("retrieves correct agent from multiple", func(t *testing.T) {
		r := NewRegistry()

		config1 := AgentConfig{
			ID:          "agent-1",
			Name:        "Agent 1",
			Role:        RoleExecutor,
			Model:       "claude-sonnet-4",
			Temperature: 0.7,
		}

		config2 := AgentConfig{
			ID:          "agent-2",
			Name:        "Agent 2",
			Role:        RoleCaptain,
			Model:       "claude-opus-4",
			Temperature: 0.5,
		}

		_ = r.Register(config1)
		_ = r.Register(config2)

		retrieved, err := r.Get("agent-2")
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		if retrieved.ID != "agent-2" {
			t.Errorf("expected ID agent-2, got %s", retrieved.ID)
		}

		if retrieved.Role != RoleCaptain {
			t.Errorf("expected Role captain, got %s", retrieved.Role)
		}
	})
}

func TestList(t *testing.T) {
	t.Run("returns empty list for new registry", func(t *testing.T) {
		r := NewRegistry()

		list := r.List()
		if len(list) != 0 {
			t.Errorf("expected empty list, got %d agents", len(list))
		}
	})

	t.Run("returns all registered agents", func(t *testing.T) {
		r := NewRegistry()

		config1 := AgentConfig{
			ID:          "agent-1",
			Name:        "Agent 1",
			Role:        RoleExecutor,
			Model:       "claude-sonnet-4",
			Temperature: 0.7,
		}

		config2 := AgentConfig{
			ID:          "agent-2",
			Name:        "Agent 2",
			Role:        RoleCaptain,
			Model:       "claude-opus-4",
			Temperature: 0.5,
		}

		config3 := AgentConfig{
			ID:          "agent-3",
			Name:        "Agent 3",
			Role:        RoleCritic,
			Model:       "claude-sonnet-4",
			Temperature: 0.3,
		}

		_ = r.Register(config1)
		_ = r.Register(config2)
		_ = r.Register(config3)

		list := r.List()
		if len(list) != 3 {
			t.Errorf("expected 3 agents, got %d", len(list))
		}

		// Verify all agents are in the list
		ids := make(map[string]bool)
		for _, agent := range list {
			ids[agent.ID] = true
		}

		if !ids["agent-1"] || !ids["agent-2"] || !ids["agent-3"] {
			t.Error("expected all agents to be in the list")
		}
	})
}

func TestUnregister(t *testing.T) {
	t.Run("removes registered agent", func(t *testing.T) {
		r := NewRegistry()

		config := AgentConfig{
			ID:          "test-agent",
			Name:        "Test Agent",
			Role:        RoleExecutor,
			Model:       "claude-sonnet-4",
			Temperature: 0.7,
		}

		_ = r.Register(config)

		err := r.Unregister("test-agent")
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		if r.Exists("test-agent") {
			t.Error("expected agent to be removed")
		}

		if r.Count() != 0 {
			t.Errorf("expected 0 agents, got %d", r.Count())
		}
	})

	t.Run("fails for nonexistent agent", func(t *testing.T) {
		r := NewRegistry()

		err := r.Unregister("nonexistent")
		if err == nil {
			t.Error("expected error for nonexistent agent")
		}
	})

	t.Run("removes only specified agent", func(t *testing.T) {
		r := NewRegistry()

		config1 := AgentConfig{
			ID:          "agent-1",
			Name:        "Agent 1",
			Role:        RoleExecutor,
			Model:       "claude-sonnet-4",
			Temperature: 0.7,
		}

		config2 := AgentConfig{
			ID:          "agent-2",
			Name:        "Agent 2",
			Role:        RoleCaptain,
			Model:       "claude-opus-4",
			Temperature: 0.5,
		}

		_ = r.Register(config1)
		_ = r.Register(config2)

		_ = r.Unregister("agent-1")

		if r.Exists("agent-1") {
			t.Error("expected agent-1 to be removed")
		}

		if !r.Exists("agent-2") {
			t.Error("expected agent-2 to still exist")
		}

		if r.Count() != 1 {
			t.Errorf("expected 1 agent, got %d", r.Count())
		}
	})
}

func TestListByRole(t *testing.T) {
	t.Run("returns agents with specific role", func(t *testing.T) {
		r := NewRegistry()

		executor1 := AgentConfig{
			ID:          "executor-1",
			Name:        "Executor 1",
			Role:        RoleExecutor,
			Model:       "claude-sonnet-4",
			Temperature: 0.7,
		}

		executor2 := AgentConfig{
			ID:          "executor-2",
			Name:        "Executor 2",
			Role:        RoleExecutor,
			Model:       "claude-sonnet-4",
			Temperature: 0.7,
		}

		captain := AgentConfig{
			ID:          "captain-1",
			Name:        "Captain 1",
			Role:        RoleCaptain,
			Model:       "claude-opus-4",
			Temperature: 0.5,
		}

		_ = r.Register(executor1)
		_ = r.Register(executor2)
		_ = r.Register(captain)

		executors := r.ListByRole(RoleExecutor)
		if len(executors) != 2 {
			t.Errorf("expected 2 executors, got %d", len(executors))
		}

		captains := r.ListByRole(RoleCaptain)
		if len(captains) != 1 {
			t.Errorf("expected 1 captain, got %d", len(captains))
		}

		critics := r.ListByRole(RoleCritic)
		if len(critics) != 0 {
			t.Errorf("expected 0 critics, got %d", len(critics))
		}
	})

	t.Run("returns empty list for role with no agents", func(t *testing.T) {
		r := NewRegistry()

		list := r.ListByRole(RoleCritic)
		if len(list) != 0 {
			t.Errorf("expected empty list, got %d agents", len(list))
		}
	})
}

func TestExists(t *testing.T) {
	t.Run("returns true for registered agent", func(t *testing.T) {
		r := NewRegistry()

		config := AgentConfig{
			ID:          "test-agent",
			Name:        "Test Agent",
			Role:        RoleExecutor,
			Model:       "claude-sonnet-4",
			Temperature: 0.7,
		}

		_ = r.Register(config)

		if !r.Exists("test-agent") {
			t.Error("expected agent to exist")
		}
	})

	t.Run("returns false for nonexistent agent", func(t *testing.T) {
		r := NewRegistry()

		if r.Exists("nonexistent") {
			t.Error("expected agent not to exist")
		}
	})
}

func TestCount(t *testing.T) {
	t.Run("returns correct count", func(t *testing.T) {
		r := NewRegistry()

		if r.Count() != 0 {
			t.Errorf("expected 0 agents, got %d", r.Count())
		}

		config1 := AgentConfig{
			ID:          "agent-1",
			Name:        "Agent 1",
			Role:        RoleExecutor,
			Model:       "claude-sonnet-4",
			Temperature: 0.7,
		}

		_ = r.Register(config1)

		if r.Count() != 1 {
			t.Errorf("expected 1 agent, got %d", r.Count())
		}

		config2 := AgentConfig{
			ID:          "agent-2",
			Name:        "Agent 2",
			Role:        RoleCaptain,
			Model:       "claude-opus-4",
			Temperature: 0.5,
		}

		_ = r.Register(config2)

		if r.Count() != 2 {
			t.Errorf("expected 2 agents, got %d", r.Count())
		}

		_ = r.Unregister("agent-1")

		if r.Count() != 1 {
			t.Errorf("expected 1 agent after unregister, got %d", r.Count())
		}
	})
}

func TestClear(t *testing.T) {
	t.Run("removes all agents", func(t *testing.T) {
		r := NewRegistry()

		config1 := AgentConfig{
			ID:          "agent-1",
			Name:        "Agent 1",
			Role:        RoleExecutor,
			Model:       "claude-sonnet-4",
			Temperature: 0.7,
		}

		config2 := AgentConfig{
			ID:          "agent-2",
			Name:        "Agent 2",
			Role:        RoleCaptain,
			Model:       "claude-opus-4",
			Temperature: 0.5,
		}

		_ = r.Register(config1)
		_ = r.Register(config2)

		r.Clear()

		if r.Count() != 0 {
			t.Errorf("expected 0 agents after clear, got %d", r.Count())
		}

		if r.Exists("agent-1") || r.Exists("agent-2") {
			t.Error("expected all agents to be removed")
		}
	})
}

// Integration Tests

func TestRegistryIntegrationWithOrchestrator(t *testing.T) {
	t.Run("orchestrator uses registry for agent management", func(t *testing.T) {
		o := New()

		// Register multiple agents
		captainConfig := AgentConfig{
			ID:          "captain",
			Name:        "Captain Agent",
			Role:        RoleCaptain,
			Model:       "claude-opus-4",
			Temperature: 0.5,
			MaxTokens:   4096,
		}

		executorConfig := AgentConfig{
			ID:          "executor",
			Name:        "Executor Agent",
			Role:        RoleExecutor,
			Model:       "claude-sonnet-4",
			Temperature: 0.7,
			MaxTokens:   4096,
		}

		criticConfig := AgentConfig{
			ID:          "critic",
			Name:        "Critic Agent",
			Role:        RoleCritic,
			Model:       "claude-sonnet-4",
			Temperature: 0.3,
			MaxTokens:   4096,
		}

		err := o.RegisterAgent(captainConfig)
		if err != nil {
			t.Fatalf("failed to register captain: %v", err)
		}

		err = o.RegisterAgent(executorConfig)
		if err != nil {
			t.Fatalf("failed to register executor: %v", err)
		}

		err = o.RegisterAgent(criticConfig)
		if err != nil {
			t.Fatalf("failed to register critic: %v", err)
		}

		// List all agents
		agents := o.ListAgents()
		if len(agents) != 3 {
			t.Errorf("expected 3 agents, got %d", len(agents))
		}

		// Get specific agent
		captain, err := o.GetAgent("captain")
		if err != nil {
			t.Fatalf("failed to get captain: %v", err)
		}

		if captain.Role != RoleCaptain {
			t.Errorf("expected captain role, got %s", captain.Role)
		}

		// Create instances from registered agents
		captainInstance, err := o.CreateInstance("captain")
		if err != nil {
			t.Fatalf("failed to create captain instance: %v", err)
		}

		if captainInstance.Config.ID != "captain" {
			t.Errorf("expected captain config, got %s", captainInstance.Config.ID)
		}

		executorInstance, err := o.CreateInstance("executor")
		if err != nil {
			t.Fatalf("failed to create executor instance: %v", err)
		}

		if executorInstance.Config.ID != "executor" {
			t.Errorf("expected executor config, got %s", executorInstance.Config.ID)
		}
	})

	t.Run("registry prevents duplicate agent IDs across orchestrator", func(t *testing.T) {
		o := New()

		config := AgentConfig{
			ID:          "test-agent",
			Name:        "Test Agent",
			Role:        RoleExecutor,
			Model:       "claude-sonnet-4",
			Temperature: 0.7,
		}

		err := o.RegisterAgent(config)
		if err != nil {
			t.Fatalf("failed to register agent: %v", err)
		}

		// Try to register again with same ID
		duplicateConfig := AgentConfig{
			ID:          "test-agent",
			Name:        "Duplicate Agent",
			Role:        RoleCaptain,
			Model:       "claude-opus-4",
			Temperature: 0.5,
		}

		err = o.RegisterAgent(duplicateConfig)
		if err == nil {
			t.Error("expected error for duplicate agent ID")
		}

		// Verify original agent is unchanged
		agent, _ := o.GetAgent("test-agent")
		if agent.Name != "Test Agent" {
			t.Errorf("expected original agent name, got %s", agent.Name)
		}

		if agent.Role != RoleExecutor {
			t.Errorf("expected original agent role, got %s", agent.Role)
		}
	})
}
