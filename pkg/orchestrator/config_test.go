package orchestrator

import (
	"encoding/json"
	"testing"
)

// Test 1: Test AgentConfig validation
func TestAgentConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		config  AgentConfig
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid config",
			config: AgentConfig{
				ID:                     "test-agent",
				Name:                   "Test Agent",
				Role:                   RoleExecutor,
				Model:                  "claude-sonnet-4",
				Temperature:            0.7,
				MaxTokens:              4096,
				MaxConcurrentSubAgents: 5,
			},
			wantErr: false,
		},
		{
			name: "missing ID",
			config: AgentConfig{
				Name:  "Test Agent",
				Role:  RoleExecutor,
				Model: "claude-sonnet-4",
			},
			wantErr: true,
			errMsg:  "agent ID is required",
		},
		{
			name: "missing name",
			config: AgentConfig{
				ID:    "test-agent",
				Role:  RoleExecutor,
				Model: "claude-sonnet-4",
			},
			wantErr: true,
			errMsg:  "agent name is required",
		},
		{
			name: "missing model",
			config: AgentConfig{
				ID:   "test-agent",
				Name: "Test Agent",
				Role: RoleExecutor,
			},
			wantErr: true,
			errMsg:  "agent model is required",
		},
		{
			name: "invalid role",
			config: AgentConfig{
				ID:    "test-agent",
				Name:  "Test Agent",
				Role:  "invalid",
				Model: "claude-sonnet-4",
			},
			wantErr: true,
			errMsg:  "invalid agent role: invalid",
		},
		{
			name: "temperature too low",
			config: AgentConfig{
				ID:          "test-agent",
				Name:        "Test Agent",
				Role:        RoleExecutor,
				Model:       "claude-sonnet-4",
				Temperature: -0.1,
			},
			wantErr: true,
			errMsg:  "temperature must be between 0 and 1",
		},
		{
			name: "temperature too high",
			config: AgentConfig{
				ID:          "test-agent",
				Name:        "Test Agent",
				Role:        RoleExecutor,
				Model:       "claude-sonnet-4",
				Temperature: 1.1,
			},
			wantErr: true,
			errMsg:  "temperature must be between 0 and 1",
		},
		{
			name: "negative max tokens",
			config: AgentConfig{
				ID:        "test-agent",
				Name:      "Test Agent",
				Role:      RoleExecutor,
				Model:     "claude-sonnet-4",
				MaxTokens: -100,
			},
			wantErr: true,
			errMsg:  "max_tokens must be positive",
		},
		{
			name: "negative max concurrent sub-agents",
			config: AgentConfig{
				ID:                     "test-agent",
				Name:                   "Test Agent",
				Role:                   RoleExecutor,
				Model:                  "claude-sonnet-4",
				MaxConcurrentSubAgents: -1,
			},
			wantErr: true,
			errMsg:  "max_concurrent_sub_agents must be non-negative",
		},
		{
			name: "valid captain config",
			config: AgentConfig{
				ID:                     "captain",
				Name:                   "Captain Agent",
				Role:                   RoleCaptain,
				Model:                  "claude-opus-4",
				Temperature:            0.7,
				MaxTokens:              4096,
				MaxConcurrentSubAgents: 5,
				SystemPrompt:           "You are a captain agent.",
			},
			wantErr: false,
		},
		{
			name: "valid critic config",
			config: AgentConfig{
				ID:           "critic",
				Name:         "Critic Agent",
				Role:         RoleCritic,
				Model:        "claude-sonnet-4",
				Temperature:  0.5,
				MaxTokens:    2048,
				SystemPrompt: "You are a critic agent.",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if tt.wantErr {
				if err == nil {
					t.Errorf("Validate() expected error but got none")
					return
				}
				// For error messages with dynamic values, just check if the error contains the expected substring
				if tt.errMsg != "" {
					errStr := err.Error()
					// Check if error message starts with the expected prefix
					if len(errStr) < len(tt.errMsg) || errStr[:len(tt.errMsg)] != tt.errMsg {
						t.Errorf("Validate() error = %v, want error starting with %v", errStr, tt.errMsg)
					}
				}
			} else {
				if err != nil {
					t.Errorf("Validate() unexpected error = %v", err)
				}
			}
		})
	}
}

// Test 2: Test AgentConfig JSON serialization/deserialization
func TestAgentConfig_JSON(t *testing.T) {
	tests := []struct {
		name   string
		config AgentConfig
	}{
		{
			name: "full config",
			config: AgentConfig{
				ID:           "executor-1",
				Name:         "Executor Agent",
				Role:         RoleExecutor,
				Model:        "claude-sonnet-4",
				Temperature:  0.7,
				MaxTokens:    4096,
				SystemPrompt: "You are an executor agent.",
				Tools: ToolPolicy{
					Allow: []string{"*"},
					Deny:  []string{"dangerous_tool"},
				},
				Workspace: "/tmp/workspace",
				Sandbox: SandboxConfig{
					Mode:  SandboxModeAll,
					Scope: SandboxScopeAgent,
				},
				MaxConcurrentSubAgents: 5,
				Metadata: map[string]string{
					"version": "1.0",
					"env":     "production",
				},
			},
		},
		{
			name: "minimal config",
			config: AgentConfig{
				ID:    "minimal",
				Name:  "Minimal Agent",
				Role:  RoleGeneral,
				Model: "claude-sonnet-4",
			},
		},
		{
			name: "captain config",
			config: AgentConfig{
				ID:                     "captain",
				Name:                   "Captain Agent",
				Role:                   RoleCaptain,
				Model:                  "claude-opus-4",
				Temperature:            0.7,
				MaxTokens:              4096,
				MaxConcurrentSubAgents: 10,
				SystemPrompt:           "You are a captain agent. Coordinate but never execute directly.",
				Tools: ToolPolicy{
					Allow: []string{"sessions_spawn", "sessions_send", "memory_search"},
					Deny:  []string{},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test ToJSON
			jsonStr, err := tt.config.ToJSON()
			if err != nil {
				t.Fatalf("ToJSON() error = %v", err)
			}

			// Test FromJSON
			decoded, err := FromJSON(jsonStr)
			if err != nil {
				t.Fatalf("FromJSON() error = %v", err)
			}

			// Compare fields
			if decoded.ID != tt.config.ID {
				t.Errorf("ID mismatch: got %v, want %v", decoded.ID, tt.config.ID)
			}
			if decoded.Name != tt.config.Name {
				t.Errorf("Name mismatch: got %v, want %v", decoded.Name, tt.config.Name)
			}
			if decoded.Role != tt.config.Role {
				t.Errorf("Role mismatch: got %v, want %v", decoded.Role, tt.config.Role)
			}
			if decoded.Model != tt.config.Model {
				t.Errorf("Model mismatch: got %v, want %v", decoded.Model, tt.config.Model)
			}
			if decoded.Temperature != tt.config.Temperature {
				t.Errorf("Temperature mismatch: got %v, want %v", decoded.Temperature, tt.config.Temperature)
			}
			if decoded.MaxTokens != tt.config.MaxTokens {
				t.Errorf("MaxTokens mismatch: got %v, want %v", decoded.MaxTokens, tt.config.MaxTokens)
			}
			if decoded.SystemPrompt != tt.config.SystemPrompt {
				t.Errorf("SystemPrompt mismatch: got %v, want %v", decoded.SystemPrompt, tt.config.SystemPrompt)
			}
			if decoded.Workspace != tt.config.Workspace {
				t.Errorf("Workspace mismatch: got %v, want %v", decoded.Workspace, tt.config.Workspace)
			}
			if decoded.MaxConcurrentSubAgents != tt.config.MaxConcurrentSubAgents {
				t.Errorf("MaxConcurrentSubAgents mismatch: got %v, want %v", decoded.MaxConcurrentSubAgents, tt.config.MaxConcurrentSubAgents)
			}

			// Test that we can unmarshal back to JSON
			var jsonMap map[string]interface{}
			if err := json.Unmarshal([]byte(jsonStr), &jsonMap); err != nil {
				t.Fatalf("Failed to unmarshal JSON: %v", err)
			}

			// Verify required fields are present
			if _, ok := jsonMap["id"]; !ok {
				t.Error("JSON missing 'id' field")
			}
			if _, ok := jsonMap["name"]; !ok {
				t.Error("JSON missing 'name' field")
			}
			if _, ok := jsonMap["role"]; !ok {
				t.Error("JSON missing 'role' field")
			}
			if _, ok := jsonMap["model"]; !ok {
				t.Error("JSON missing 'model' field")
			}
		})
	}
}

// Test 3: Test AgentConfig helper functions and tool policies
func TestAgentConfig_Helpers(t *testing.T) {
	t.Run("DefaultAgentConfig", func(t *testing.T) {
		config := DefaultAgentConfig()

		if config.Temperature != 0.7 {
			t.Errorf("Default temperature = %v, want 0.7", config.Temperature)
		}
		if config.MaxTokens != 4096 {
			t.Errorf("Default MaxTokens = %v, want 4096", config.MaxTokens)
		}
		if config.MaxConcurrentSubAgents != 5 {
			t.Errorf("Default MaxConcurrentSubAgents = %v, want 5", config.MaxConcurrentSubAgents)
		}
		if len(config.Tools.Allow) != 1 || config.Tools.Allow[0] != "*" {
			t.Errorf("Default Tools.Allow = %v, want [*]", config.Tools.Allow)
		}
		if config.Sandbox.Mode != SandboxModeOff {
			t.Errorf("Default Sandbox.Mode = %v, want off", config.Sandbox.Mode)
		}
	})

	t.Run("NewAgentConfig", func(t *testing.T) {
		config := NewAgentConfig("test-id", "Test Agent", RoleExecutor, "claude-sonnet-4")

		if config.ID != "test-id" {
			t.Errorf("ID = %v, want test-id", config.ID)
		}
		if config.Name != "Test Agent" {
			t.Errorf("Name = %v, want Test Agent", config.Name)
		}
		if config.Role != RoleExecutor {
			t.Errorf("Role = %v, want executor", config.Role)
		}
		if config.Model != "claude-sonnet-4" {
			t.Errorf("Model = %v, want claude-sonnet-4", config.Model)
		}

		// Should have defaults
		if config.Temperature != 0.7 {
			t.Errorf("Temperature = %v, want 0.7", config.Temperature)
		}
	})

	t.Run("Builder methods", func(t *testing.T) {
		config := NewAgentConfig("test", "Test", RoleExecutor, "model").
			WithTemperature(0.5).
			WithMaxTokens(2048).
			WithSystemPrompt("Test prompt").
			WithWorkspace("/tmp/test").
			WithSandbox(SandboxModeAll, SandboxScopeSession).
			WithMaxConcurrentSubAgents(10).
			WithMetadata("key1", "value1").
			WithMetadata("key2", "value2")

		if config.Temperature != 0.5 {
			t.Errorf("Temperature = %v, want 0.5", config.Temperature)
		}
		if config.MaxTokens != 2048 {
			t.Errorf("MaxTokens = %v, want 2048", config.MaxTokens)
		}
		if config.SystemPrompt != "Test prompt" {
			t.Errorf("SystemPrompt = %v, want 'Test prompt'", config.SystemPrompt)
		}
		if config.Workspace != "/tmp/test" {
			t.Errorf("Workspace = %v, want '/tmp/test'", config.Workspace)
		}
		if config.Sandbox.Mode != SandboxModeAll {
			t.Errorf("Sandbox.Mode = %v, want all", config.Sandbox.Mode)
		}
		if config.Sandbox.Scope != SandboxScopeSession {
			t.Errorf("Sandbox.Scope = %v, want session", config.Sandbox.Scope)
		}
		if config.MaxConcurrentSubAgents != 10 {
			t.Errorf("MaxConcurrentSubAgents = %v, want 10", config.MaxConcurrentSubAgents)
		}
		if config.Metadata["key1"] != "value1" {
			t.Errorf("Metadata[key1] = %v, want value1", config.Metadata["key1"])
		}
		if config.Metadata["key2"] != "value2" {
			t.Errorf("Metadata[key2] = %v, want value2", config.Metadata["key2"])
		}
	})

	t.Run("Clone", func(t *testing.T) {
		original := NewAgentConfig("test", "Test", RoleExecutor, "model").
			WithTools([]string{"tool1", "tool2"}, []string{"tool3"}).
			WithMetadata("key", "value")

		clone := original.Clone()

		// Modify clone
		clone.ID = "modified"
		clone.Tools.Allow[0] = "modified-tool"
		clone.Metadata["key"] = "modified-value"

		// Original should be unchanged
		if original.ID == "modified" {
			t.Error("Clone modified original ID")
		}
		if original.Tools.Allow[0] == "modified-tool" {
			t.Error("Clone modified original Tools.Allow")
		}
		if original.Metadata["key"] == "modified-value" {
			t.Error("Clone modified original Metadata")
		}
	})

	t.Run("IsToolAllowed", func(t *testing.T) {
		tests := []struct {
			name     string
			config   AgentConfig
			toolName string
			want     bool
		}{
			{
				name: "allow all",
				config: AgentConfig{
					Tools: ToolPolicy{
						Allow: []string{"*"},
						Deny:  []string{},
					},
				},
				toolName: "any_tool",
				want:     true,
			},
			{
				name: "deny overrides allow all",
				config: AgentConfig{
					Tools: ToolPolicy{
						Allow: []string{"*"},
						Deny:  []string{"dangerous_tool"},
					},
				},
				toolName: "dangerous_tool",
				want:     false,
			},
			{
				name: "specific allow",
				config: AgentConfig{
					Tools: ToolPolicy{
						Allow: []string{"tool1", "tool2"},
						Deny:  []string{},
					},
				},
				toolName: "tool1",
				want:     true,
			},
			{
				name: "not in allow list",
				config: AgentConfig{
					Tools: ToolPolicy{
						Allow: []string{"tool1", "tool2"},
						Deny:  []string{},
					},
				},
				toolName: "tool3",
				want:     false,
			},
			{
				name: "deny all",
				config: AgentConfig{
					Tools: ToolPolicy{
						Allow: []string{"*"},
						Deny:  []string{"*"},
					},
				},
				toolName: "any_tool",
				want:     false,
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				got := tt.config.IsToolAllowed(tt.toolName)
				if got != tt.want {
					t.Errorf("IsToolAllowed(%v) = %v, want %v", tt.toolName, got, tt.want)
				}
			})
		}
	})

	t.Run("IsSandboxEnabled", func(t *testing.T) {
		tests := []struct {
			name   string
			config AgentConfig
			want   bool
		}{
			{
				name: "sandbox off",
				config: AgentConfig{
					Sandbox: SandboxConfig{Mode: SandboxModeOff},
				},
				want: false,
			},
			{
				name: "sandbox all",
				config: AgentConfig{
					Sandbox: SandboxConfig{Mode: SandboxModeAll},
				},
				want: true,
			},
			{
				name: "sandbox tools",
				config: AgentConfig{
					Sandbox: SandboxConfig{Mode: SandboxModeTools},
				},
				want: true,
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				got := tt.config.IsSandboxEnabled()
				if got != tt.want {
					t.Errorf("IsSandboxEnabled() = %v, want %v", got, tt.want)
				}
			})
		}
	})
}
