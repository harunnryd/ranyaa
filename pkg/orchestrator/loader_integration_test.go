package orchestrator

import (
	"os"
	"path/filepath"
	"testing"
)

// Integration Test 1: Test loading and registering agents from config file
func TestConfigLoader_Integration_LoadAndRegister(t *testing.T) {
	// Create a temporary config file
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")

	jsonContent := `{
		"agents": [
			{
				"id": "captain",
				"name": "Captain Agent",
				"role": "captain",
				"model": "claude-opus-4",
				"temperature": 0.7,
				"max_tokens": 4096,
				"system_prompt": "You are a captain agent.",
				"tools": {
					"allow": ["sessions_spawn", "memory_search"],
					"deny": []
				},
				"sandbox": {
					"mode": "off",
					"scope": "agent"
				},
				"max_concurrent_sub_agents": 5
			},
			{
				"id": "executor",
				"name": "Executor Agent",
				"role": "executor",
				"model": "claude-sonnet-4",
				"temperature": 0.7,
				"max_tokens": 4096,
				"tools": {
					"allow": ["*"],
					"deny": []
				},
				"sandbox": {
					"mode": "all",
					"scope": "agent"
				},
				"max_concurrent_sub_agents": 5
			},
			{
				"id": "critic",
				"name": "Critic Agent",
				"role": "critic",
				"model": "claude-sonnet-4",
				"temperature": 0.5,
				"max_tokens": 2048,
				"system_prompt": "You are a critic agent.",
				"tools": {
					"allow": ["read"],
					"deny": []
				},
				"sandbox": {
					"mode": "off",
					"scope": "agent"
				},
				"max_concurrent_sub_agents": 0
			}
		]
	}`

	if err := os.WriteFile(configPath, []byte(jsonContent), 0644); err != nil {
		t.Fatalf("Failed to write test config file: %v", err)
	}

	// Create loader and orchestrator
	loader := NewConfigLoader(nil)
	orchestrator := New()

	// Load and validate configs
	configs, err := loader.LoadAndValidate(configPath)
	if err != nil {
		t.Fatalf("LoadAndValidate() error = %v", err)
	}

	// Register all agents
	for _, config := range configs {
		if err := orchestrator.RegisterAgent(config); err != nil {
			t.Fatalf("RegisterAgent() error = %v", err)
		}
	}

	// Verify all agents are registered
	agents := orchestrator.ListAgents()
	if len(agents) != 3 {
		t.Errorf("ListAgents() returned %d agents, want 3", len(agents))
	}

	// Verify captain agent
	captain, err := orchestrator.GetAgent("captain")
	if err != nil {
		t.Errorf("GetAgent(captain) error = %v", err)
	}
	if captain.Role != RoleCaptain {
		t.Errorf("captain.Role = %v, want captain", captain.Role)
	}

	// Verify executor agent
	executor, err := orchestrator.GetAgent("executor")
	if err != nil {
		t.Errorf("GetAgent(executor) error = %v", err)
	}
	if executor.Role != RoleExecutor {
		t.Errorf("executor.Role = %v, want executor", executor.Role)
	}
	if executor.Sandbox.Mode != SandboxModeAll {
		t.Errorf("executor.Sandbox.Mode = %v, want all", executor.Sandbox.Mode)
	}

	// Verify critic agent
	critic, err := orchestrator.GetAgent("critic")
	if err != nil {
		t.Errorf("GetAgent(critic) error = %v", err)
	}
	if critic.Role != RoleCritic {
		t.Errorf("critic.Role = %v, want critic", critic.Role)
	}
}

// Integration Test 2: Test loading from YAML and validating complex configurations
func TestConfigLoader_Integration_ComplexYAML(t *testing.T) {
	// Create a temporary YAML config file with complex configurations
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	yamlContent := `agents:
  - id: multi-tool-agent
    name: Multi-Tool Agent
    role: executor
    model: claude-sonnet-4
    temperature: 0.8
    max_tokens: 8192
    system_prompt: |
      You are a multi-tool agent.
      You can use various tools to complete tasks.
    tools:
      allow:
        - file_read
        - file_write
        - exec
        - memory_search
      deny:
        - dangerous_tool
    workspace: /tmp/workspace
    sandbox:
      mode: tools
      scope: session
    max_concurrent_sub_agents: 10
    metadata:
      version: "1.0"
      environment: production
      team: platform
  - id: restricted-agent
    name: Restricted Agent
    role: general
    model: claude-sonnet-4
    temperature: 0.5
    max_tokens: 2048
    tools:
      allow:
        - read
      deny:
        - "*"
    sandbox:
      mode: all
      scope: agent
    max_concurrent_sub_agents: 0
`

	if err := os.WriteFile(configPath, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("Failed to write test config file: %v", err)
	}

	// Create loader
	loader := NewConfigLoader(nil)

	// Load and validate configs
	configs, err := loader.LoadAndValidate(configPath)
	if err != nil {
		t.Fatalf("LoadAndValidate() error = %v", err)
	}

	// Verify loaded configs
	if len(configs) != 2 {
		t.Errorf("LoadAndValidate() loaded %d configs, want 2", len(configs))
	}

	// Verify multi-tool agent
	multiTool := configs[0]
	if multiTool.ID != "multi-tool-agent" {
		t.Errorf("multiTool.ID = %v, want multi-tool-agent", multiTool.ID)
	}
	if multiTool.Temperature != 0.8 {
		t.Errorf("multiTool.Temperature = %v, want 0.8", multiTool.Temperature)
	}
	if multiTool.MaxTokens != 8192 {
		t.Errorf("multiTool.MaxTokens = %v, want 8192", multiTool.MaxTokens)
	}
	if len(multiTool.Tools.Allow) != 4 {
		t.Errorf("multiTool.Tools.Allow has %d items, want 4", len(multiTool.Tools.Allow))
	}
	if multiTool.Workspace != "/tmp/workspace" {
		t.Errorf("multiTool.Workspace = %v, want /tmp/workspace", multiTool.Workspace)
	}
	if multiTool.Sandbox.Mode != SandboxModeTools {
		t.Errorf("multiTool.Sandbox.Mode = %v, want tools", multiTool.Sandbox.Mode)
	}
	if multiTool.Sandbox.Scope != SandboxScopeSession {
		t.Errorf("multiTool.Sandbox.Scope = %v, want session", multiTool.Sandbox.Scope)
	}
	if multiTool.MaxConcurrentSubAgents != 10 {
		t.Errorf("multiTool.MaxConcurrentSubAgents = %v, want 10", multiTool.MaxConcurrentSubAgents)
	}
	if multiTool.Metadata["version"] != "1.0" {
		t.Errorf("multiTool.Metadata[version] = %v, want 1.0", multiTool.Metadata["version"])
	}

	// Verify restricted agent
	restricted := configs[1]
	if restricted.ID != "restricted-agent" {
		t.Errorf("restricted.ID = %v, want restricted-agent", restricted.ID)
	}
	if restricted.Role != RoleGeneral {
		t.Errorf("restricted.Role = %v, want general", restricted.Role)
	}
	if len(restricted.Tools.Allow) != 1 || restricted.Tools.Allow[0] != "read" {
		t.Errorf("restricted.Tools.Allow = %v, want [read]", restricted.Tools.Allow)
	}
	if len(restricted.Tools.Deny) != 1 || restricted.Tools.Deny[0] != "*" {
		t.Errorf("restricted.Tools.Deny = %v, want [*]", restricted.Tools.Deny)
	}

	// Test tool policy enforcement
	if restricted.IsToolAllowed("file_write") {
		t.Error("restricted agent should not allow file_write")
	}
	if restricted.IsToolAllowed("read") {
		t.Error("restricted agent should not allow read (deny * overrides)")
	}
}
