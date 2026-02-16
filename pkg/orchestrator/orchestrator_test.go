package orchestrator

import (
	"context"
	"testing"
	"time"
)

// mockLogger implements the Logger interface for testing
type mockLogger struct {
	infos  []string
	errors []string
	debugs []string
}

func (m *mockLogger) Info(msg string, fields ...interface{}) {
	m.infos = append(m.infos, msg)
}

func (m *mockLogger) Error(msg string, err error, fields ...interface{}) {
	m.errors = append(m.errors, msg)
}

func (m *mockLogger) Debug(msg string, fields ...interface{}) {
	m.debugs = append(m.debugs, msg)
}

func TestNew(t *testing.T) {
	t.Run("creates orchestrator with defaults", func(t *testing.T) {
		o := New()

		if o == nil {
			t.Fatal("expected orchestrator to be created")
		}

		if o.maxConcurrent != 5 {
			t.Errorf("expected maxConcurrent to be 5, got %d", o.maxConcurrent)
		}

		if o.registry == nil {
			t.Error("expected registry to be initialized")
		}

		if o.agents == nil {
			t.Error("expected agents map to be initialized")
		}
	})

	t.Run("creates orchestrator with custom max concurrent", func(t *testing.T) {
		o := New(WithMaxConcurrent(10))

		if o.maxConcurrent != 10 {
			t.Errorf("expected maxConcurrent to be 10, got %d", o.maxConcurrent)
		}
	})

	t.Run("creates orchestrator with logger", func(t *testing.T) {
		logger := &mockLogger{}
		o := New(WithLogger(logger))

		if o.logger == nil {
			t.Error("expected logger to be set")
		}
	})
}

func TestRegisterAgent(t *testing.T) {
	t.Run("registers valid agent", func(t *testing.T) {
		o := New()

		config := AgentConfig{
			ID:          "test-agent",
			Name:        "Test Agent",
			Role:        RoleExecutor,
			Model:       "claude-sonnet-4",
			Temperature: 0.7,
			MaxTokens:   4096,
		}

		err := o.RegisterAgent(config)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		retrieved, err := o.GetAgent("test-agent")
		if err != nil {
			t.Fatalf("expected to retrieve agent, got error: %v", err)
		}

		if retrieved.ID != config.ID {
			t.Errorf("expected ID %s, got %s", config.ID, retrieved.ID)
		}
	})

	t.Run("rejects invalid agent config", func(t *testing.T) {
		o := New()

		config := AgentConfig{
			ID:   "test-agent",
			Name: "Test Agent",
			// Missing required fields
		}

		err := o.RegisterAgent(config)
		if err == nil {
			t.Error("expected error for invalid config")
		}
	})

	t.Run("rejects duplicate agent ID", func(t *testing.T) {
		o := New()

		config := AgentConfig{
			ID:          "test-agent",
			Name:        "Test Agent",
			Role:        RoleExecutor,
			Model:       "claude-sonnet-4",
			Temperature: 0.7,
		}

		_ = o.RegisterAgent(config)
		err := o.RegisterAgent(config)

		if err == nil {
			t.Error("expected error for duplicate agent ID")
		}
	})
}

func TestCreateInstance(t *testing.T) {
	t.Run("creates instance from registered agent", func(t *testing.T) {
		logger := &mockLogger{}
		o := New(WithLogger(logger))

		config := AgentConfig{
			ID:          "test-agent",
			Name:        "Test Agent",
			Role:        RoleExecutor,
			Model:       "claude-sonnet-4",
			Temperature: 0.7,
		}

		_ = o.RegisterAgent(config)

		instance, err := o.CreateInstance("test-agent")
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		if instance.ID == "" {
			t.Error("expected instance ID to be set")
		}

		if instance.Status != AgentStatusIdle {
			t.Errorf("expected status to be idle, got %s", instance.Status)
		}

		if instance.Config.ID != config.ID {
			t.Errorf("expected config ID %s, got %s", config.ID, instance.Config.ID)
		}

		if len(logger.infos) == 0 {
			t.Error("expected info log for instance creation")
		}
	})

	t.Run("fails for unregistered agent", func(t *testing.T) {
		o := New()

		_, err := o.CreateInstance("nonexistent")
		if err == nil {
			t.Error("expected error for unregistered agent")
		}
	})
}

func TestGetInstance(t *testing.T) {
	t.Run("retrieves existing instance", func(t *testing.T) {
		o := New()

		config := AgentConfig{
			ID:          "test-agent",
			Name:        "Test Agent",
			Role:        RoleExecutor,
			Model:       "claude-sonnet-4",
			Temperature: 0.7,
		}

		_ = o.RegisterAgent(config)
		instance, _ := o.CreateInstance("test-agent")

		retrieved, err := o.GetInstance(instance.ID)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		if retrieved.ID != instance.ID {
			t.Errorf("expected ID %s, got %s", instance.ID, retrieved.ID)
		}
	})

	t.Run("fails for nonexistent instance", func(t *testing.T) {
		o := New()

		_, err := o.GetInstance("nonexistent")
		if err == nil {
			t.Error("expected error for nonexistent instance")
		}
	})
}

func TestUpdateInstanceStatus(t *testing.T) {
	t.Run("updates instance status", func(t *testing.T) {
		logger := &mockLogger{}
		o := New(WithLogger(logger))

		config := AgentConfig{
			ID:          "test-agent",
			Name:        "Test Agent",
			Role:        RoleExecutor,
			Model:       "claude-sonnet-4",
			Temperature: 0.7,
		}

		_ = o.RegisterAgent(config)
		instance, _ := o.CreateInstance("test-agent")

		err := o.UpdateInstanceStatus(instance.ID, AgentStatusRunning)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		retrieved, _ := o.GetInstance(instance.ID)
		if retrieved.Status != AgentStatusRunning {
			t.Errorf("expected status running, got %s", retrieved.Status)
		}

		if len(logger.infos) < 2 {
			t.Error("expected info log for status update")
		}
	})

	t.Run("fails for nonexistent instance", func(t *testing.T) {
		o := New()

		err := o.UpdateInstanceStatus("nonexistent", AgentStatusRunning)
		if err == nil {
			t.Error("expected error for nonexistent instance")
		}
	})
}

func TestStopInstance(t *testing.T) {
	t.Run("stops running instance", func(t *testing.T) {
		logger := &mockLogger{}
		o := New(WithLogger(logger))

		config := AgentConfig{
			ID:          "test-agent",
			Name:        "Test Agent",
			Role:        RoleExecutor,
			Model:       "claude-sonnet-4",
			Temperature: 0.7,
		}

		_ = o.RegisterAgent(config)
		instance, _ := o.CreateInstance("test-agent")
		_ = o.UpdateInstanceStatus(instance.ID, AgentStatusRunning)

		err := o.StopInstance(instance.ID)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		retrieved, _ := o.GetInstance(instance.ID)
		if retrieved.Status != AgentStatusStopped {
			t.Errorf("expected status stopped, got %s", retrieved.Status)
		}

		// Verify context was cancelled
		select {
		case <-retrieved.Context.Done():
			// Context was cancelled as expected
		case <-time.After(100 * time.Millisecond):
			t.Error("expected context to be cancelled")
		}
	})
}

func TestGetActiveInstances(t *testing.T) {
	t.Run("counts active instances", func(t *testing.T) {
		o := New()

		config := AgentConfig{
			ID:          "test-agent",
			Name:        "Test Agent",
			Role:        RoleExecutor,
			Model:       "claude-sonnet-4",
			Temperature: 0.7,
		}

		_ = o.RegisterAgent(config)

		// Create 3 instances
		instance1, _ := o.CreateInstance("test-agent")
		instance2, _ := o.CreateInstance("test-agent")
		instance3, _ := o.CreateInstance("test-agent")

		// Set 2 to running
		_ = o.UpdateInstanceStatus(instance1.ID, AgentStatusRunning)
		_ = o.UpdateInstanceStatus(instance2.ID, AgentStatusRunning)
		_ = o.UpdateInstanceStatus(instance3.ID, AgentStatusStopped)

		active := o.GetActiveInstances()
		if active != 2 {
			t.Errorf("expected 2 active instances, got %d", active)
		}
	})
}

func TestCanSpawnMore(t *testing.T) {
	t.Run("allows spawning when under limit", func(t *testing.T) {
		o := New(WithMaxConcurrent(3))

		config := AgentConfig{
			ID:          "test-agent",
			Name:        "Test Agent",
			Role:        RoleExecutor,
			Model:       "claude-sonnet-4",
			Temperature: 0.7,
		}

		_ = o.RegisterAgent(config)

		instance1, _ := o.CreateInstance("test-agent")
		instance2, _ := o.CreateInstance("test-agent")

		_ = o.UpdateInstanceStatus(instance1.ID, AgentStatusRunning)
		_ = o.UpdateInstanceStatus(instance2.ID, AgentStatusRunning)

		if !o.CanSpawnMore() {
			t.Error("expected to allow spawning more agents")
		}
	})

	t.Run("blocks spawning when at limit", func(t *testing.T) {
		o := New(WithMaxConcurrent(2))

		config := AgentConfig{
			ID:          "test-agent",
			Name:        "Test Agent",
			Role:        RoleExecutor,
			Model:       "claude-sonnet-4",
			Temperature: 0.7,
		}

		_ = o.RegisterAgent(config)

		instance1, _ := o.CreateInstance("test-agent")
		instance2, _ := o.CreateInstance("test-agent")

		_ = o.UpdateInstanceStatus(instance1.ID, AgentStatusRunning)
		_ = o.UpdateInstanceStatus(instance2.ID, AgentStatusRunning)

		if o.CanSpawnMore() {
			t.Error("expected to block spawning more agents")
		}
	})
}

func TestCleanup(t *testing.T) {
	t.Run("removes stopped instances", func(t *testing.T) {
		logger := &mockLogger{}
		o := New(WithLogger(logger))

		config := AgentConfig{
			ID:          "test-agent",
			Name:        "Test Agent",
			Role:        RoleExecutor,
			Model:       "claude-sonnet-4",
			Temperature: 0.7,
		}

		_ = o.RegisterAgent(config)

		instance1, _ := o.CreateInstance("test-agent")
		instance2, _ := o.CreateInstance("test-agent")

		_ = o.UpdateInstanceStatus(instance1.ID, AgentStatusStopped)
		_ = o.UpdateInstanceStatus(instance2.ID, AgentStatusRunning)

		o.Cleanup()

		// Stopped instance should be removed
		_, err := o.GetInstance(instance1.ID)
		if err == nil {
			t.Error("expected stopped instance to be removed")
		}

		// Running instance should still exist
		_, err = o.GetInstance(instance2.ID)
		if err != nil {
			t.Error("expected running instance to still exist")
		}
	})
}

func TestShutdown(t *testing.T) {
	t.Run("stops all instances", func(t *testing.T) {
		logger := &mockLogger{}
		o := New(WithLogger(logger))

		config := AgentConfig{
			ID:          "test-agent",
			Name:        "Test Agent",
			Role:        RoleExecutor,
			Model:       "claude-sonnet-4",
			Temperature: 0.7,
		}

		_ = o.RegisterAgent(config)

		instance1, _ := o.CreateInstance("test-agent")
		instance2, _ := o.CreateInstance("test-agent")

		_ = o.UpdateInstanceStatus(instance1.ID, AgentStatusRunning)
		_ = o.UpdateInstanceStatus(instance2.ID, AgentStatusRunning)

		ctx := context.Background()
		err := o.Shutdown(ctx)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// All instances should be stopped
		retrieved1, _ := o.GetInstance(instance1.ID)
		retrieved2, _ := o.GetInstance(instance2.ID)

		if retrieved1.Status != AgentStatusStopped {
			t.Errorf("expected instance1 to be stopped, got %s", retrieved1.Status)
		}

		if retrieved2.Status != AgentStatusStopped {
			t.Errorf("expected instance2 to be stopped, got %s", retrieved2.Status)
		}
	})
}
