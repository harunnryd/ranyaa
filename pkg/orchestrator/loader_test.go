package orchestrator

import (
	"os"
	"path/filepath"
	"testing"
)

// Test 1: Test LoadFromFile with JSON
func TestConfigLoader_LoadFromFile_JSON(t *testing.T) {
	loader := NewConfigLoader(nil)

	// Create a temporary JSON config file
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
			}
		]
	}`

	if err := os.WriteFile(configPath, []byte(jsonContent), 0644); err != nil {
		t.Fatalf("Failed to write test config file: %v", err)
	}

	// Load configs
	configs, err := loader.LoadFromFile(configPath)
	if err != nil {
		t.Fatalf("LoadFromFile() error = %v", err)
	}

	// Verify loaded configs
	if len(configs) != 2 {
		t.Errorf("LoadFromFile() loaded %d configs, want 2", len(configs))
	}

	// Verify first config
	if configs[0].ID != "captain" {
		t.Errorf("configs[0].ID = %v, want captain", configs[0].ID)
	}
	if configs[0].Role != RoleCaptain {
		t.Errorf("configs[0].Role = %v, want captain", configs[0].Role)
	}

	// Verify second config
	if configs[1].ID != "executor" {
		t.Errorf("configs[1].ID = %v, want executor", configs[1].ID)
	}
	if configs[1].Role != RoleExecutor {
		t.Errorf("configs[1].Role = %v, want executor", configs[1].Role)
	}
}

// Test 2: Test LoadFromFile with YAML
func TestConfigLoader_LoadFromFile_YAML(t *testing.T) {
	loader := NewConfigLoader(nil)

	// Create a temporary YAML config file
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	yamlContent := `agents:
  - id: captain
    name: Captain Agent
    role: captain
    model: claude-opus-4
    temperature: 0.7
    max_tokens: 4096
    system_prompt: You are a captain agent.
    tools:
      allow:
        - sessions_spawn
        - memory_search
      deny: []
    sandbox:
      mode: off
      scope: agent
    max_concurrent_sub_agents: 5
  - id: executor
    name: Executor Agent
    role: executor
    model: claude-sonnet-4
    temperature: 0.7
    max_tokens: 4096
    tools:
      allow:
        - "*"
      deny: []
    sandbox:
      mode: all
      scope: agent
    max_concurrent_sub_agents: 5
`

	if err := os.WriteFile(configPath, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("Failed to write test config file: %v", err)
	}

	// Load configs
	configs, err := loader.LoadFromFile(configPath)
	if err != nil {
		t.Fatalf("LoadFromFile() error = %v", err)
	}

	// Verify loaded configs
	if len(configs) != 2 {
		t.Errorf("LoadFromFile() loaded %d configs, want 2", len(configs))
	}

	// Verify first config
	if configs[0].ID != "captain" {
		t.Errorf("configs[0].ID = %v, want captain", configs[0].ID)
	}
	if configs[0].Role != RoleCaptain {
		t.Errorf("configs[0].Role = %v, want captain", configs[0].Role)
	}

	// Verify second config
	if configs[1].ID != "executor" {
		t.Errorf("configs[1].ID = %v, want executor", configs[1].ID)
	}
	if configs[1].Role != RoleExecutor {
		t.Errorf("configs[1].Role = %v, want executor", configs[1].Role)
	}
}

// Test 3: Test LoadFromFile error cases
func TestConfigLoader_LoadFromFile_Errors(t *testing.T) {
	loader := NewConfigLoader(nil)

	tests := []struct {
		name      string
		setupFunc func() string
		wantErr   string
	}{
		{
			name: "empty path",
			setupFunc: func() string {
				return ""
			},
			wantErr: "config file path is required",
		},
		{
			name: "file not found",
			setupFunc: func() string {
				return "/nonexistent/path/config.json"
			},
			wantErr: "config file not found",
		},
		{
			name: "invalid JSON",
			setupFunc: func() string {
				tmpDir := t.TempDir()
				configPath := filepath.Join(tmpDir, "invalid.json")
				os.WriteFile(configPath, []byte("{invalid json}"), 0644)
				return configPath
			},
			wantErr: "failed to parse JSON config",
		},
		{
			name: "invalid YAML",
			setupFunc: func() string {
				tmpDir := t.TempDir()
				configPath := filepath.Join(tmpDir, "invalid.yaml")
				os.WriteFile(configPath, []byte("invalid: yaml: content: ["), 0644)
				return configPath
			},
			wantErr: "failed to parse YAML config",
		},
		{
			name: "unsupported format",
			setupFunc: func() string {
				tmpDir := t.TempDir()
				configPath := filepath.Join(tmpDir, "config.txt")
				os.WriteFile(configPath, []byte("some content"), 0644)
				return configPath
			},
			wantErr: "unsupported config file format",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := tt.setupFunc()
			_, err := loader.LoadFromFile(path)
			if err == nil {
				t.Errorf("LoadFromFile() expected error but got none")
				return
			}
			if len(err.Error()) < len(tt.wantErr) || err.Error()[:len(tt.wantErr)] != tt.wantErr {
				t.Errorf("LoadFromFile() error = %v, want error starting with %v", err.Error(), tt.wantErr)
			}
		})
	}
}

// Test 4: Test ValidateConfigs
func TestConfigLoader_ValidateConfigs(t *testing.T) {
	loader := NewConfigLoader(nil)

	tests := []struct {
		name    string
		configs []AgentConfig
		wantErr string
	}{
		{
			name:    "empty configs",
			configs: []AgentConfig{},
			wantErr: "no agent configurations found",
		},
		{
			name: "valid configs",
			configs: []AgentConfig{
				{
					ID:    "agent1",
					Name:  "Agent 1",
					Role:  RoleExecutor,
					Model: "claude-sonnet-4",
				},
				{
					ID:    "agent2",
					Name:  "Agent 2",
					Role:  RoleCaptain,
					Model: "claude-opus-4",
				},
			},
			wantErr: "",
		},
		{
			name: "invalid config",
			configs: []AgentConfig{
				{
					ID:    "agent1",
					Name:  "Agent 1",
					Role:  RoleExecutor,
					Model: "claude-sonnet-4",
				},
				{
					ID:   "agent2",
					Name: "Agent 2",
					Role: RoleCaptain,
					// Missing model
				},
			},
			wantErr: "agent config at index 1 is invalid",
		},
		{
			name: "duplicate IDs",
			configs: []AgentConfig{
				{
					ID:    "agent1",
					Name:  "Agent 1",
					Role:  RoleExecutor,
					Model: "claude-sonnet-4",
				},
				{
					ID:    "agent1",
					Name:  "Agent 1 Duplicate",
					Role:  RoleCaptain,
					Model: "claude-opus-4",
				},
			},
			wantErr: "duplicate agent ID found: agent1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := loader.ValidateConfigs(tt.configs)
			if tt.wantErr == "" {
				if err != nil {
					t.Errorf("ValidateConfigs() unexpected error = %v", err)
				}
			} else {
				if err == nil {
					t.Errorf("ValidateConfigs() expected error but got none")
					return
				}
				if len(err.Error()) < len(tt.wantErr) || err.Error()[:len(tt.wantErr)] != tt.wantErr {
					t.Errorf("ValidateConfigs() error = %v, want error starting with %v", err.Error(), tt.wantErr)
				}
			}
		})
	}
}

// Test 5: Test LoadAndValidate
func TestConfigLoader_LoadAndValidate(t *testing.T) {
	loader := NewConfigLoader(nil)

	t.Run("valid config file", func(t *testing.T) {
		tmpDir := t.TempDir()
		configPath := filepath.Join(tmpDir, "config.json")

		jsonContent := `{
			"agents": [
				{
					"id": "agent1",
					"name": "Agent 1",
					"role": "executor",
					"model": "claude-sonnet-4"
				}
			]
		}`

		if err := os.WriteFile(configPath, []byte(jsonContent), 0644); err != nil {
			t.Fatalf("Failed to write test config file: %v", err)
		}

		configs, err := loader.LoadAndValidate(configPath)
		if err != nil {
			t.Fatalf("LoadAndValidate() error = %v", err)
		}

		if len(configs) != 1 {
			t.Errorf("LoadAndValidate() loaded %d configs, want 1", len(configs))
		}
	})

	t.Run("invalid config file", func(t *testing.T) {
		tmpDir := t.TempDir()
		configPath := filepath.Join(tmpDir, "config.json")

		jsonContent := `{
			"agents": [
				{
					"id": "agent1",
					"name": "Agent 1",
					"role": "executor"
				}
			]
		}`

		if err := os.WriteFile(configPath, []byte(jsonContent), 0644); err != nil {
			t.Fatalf("Failed to write test config file: %v", err)
		}

		_, err := loader.LoadAndValidate(configPath)
		if err == nil {
			t.Errorf("LoadAndValidate() expected error but got none")
		}
	})

	t.Run("duplicate agent IDs", func(t *testing.T) {
		tmpDir := t.TempDir()
		configPath := filepath.Join(tmpDir, "config.json")

		jsonContent := `{
			"agents": [
				{
					"id": "agent1",
					"name": "Agent 1",
					"role": "executor",
					"model": "claude-sonnet-4"
				},
				{
					"id": "agent1",
					"name": "Agent 1 Duplicate",
					"role": "captain",
					"model": "claude-opus-4"
				}
			]
		}`

		if err := os.WriteFile(configPath, []byte(jsonContent), 0644); err != nil {
			t.Fatalf("Failed to write test config file: %v", err)
		}

		_, err := loader.LoadAndValidate(configPath)
		if err == nil {
			t.Errorf("LoadAndValidate() expected error but got none")
			return
		}

		wantErr := "duplicate agent ID found"
		if len(err.Error()) < len(wantErr) || err.Error()[:len(wantErr)] != wantErr {
			t.Errorf("LoadAndValidate() error = %v, want error starting with %v", err.Error(), wantErr)
		}
	})
}

// Test 6: Test LoadFromJSON
func TestConfigLoader_LoadFromJSON(t *testing.T) {
	loader := NewConfigLoader(nil)

	tests := []struct {
		name      string
		jsonStr   string
		wantCount int
		wantErr   string
	}{
		{
			name: "valid JSON",
			jsonStr: `{
				"agents": [
					{
						"id": "agent1",
						"name": "Agent 1",
						"role": "executor",
						"model": "claude-sonnet-4"
					}
				]
			}`,
			wantCount: 1,
			wantErr:   "",
		},
		{
			name:      "empty JSON string",
			jsonStr:   "",
			wantCount: 0,
			wantErr:   "JSON string is required",
		},
		{
			name:      "invalid JSON",
			jsonStr:   "{invalid json}",
			wantCount: 0,
			wantErr:   "failed to parse JSON",
		},
		{
			name: "multiple agents",
			jsonStr: `{
				"agents": [
					{
						"id": "agent1",
						"name": "Agent 1",
						"role": "executor",
						"model": "claude-sonnet-4"
					},
					{
						"id": "agent2",
						"name": "Agent 2",
						"role": "captain",
						"model": "claude-opus-4"
					}
				]
			}`,
			wantCount: 2,
			wantErr:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			configs, err := loader.LoadFromJSON(tt.jsonStr)
			if tt.wantErr == "" {
				if err != nil {
					t.Errorf("LoadFromJSON() unexpected error = %v", err)
					return
				}
				if len(configs) != tt.wantCount {
					t.Errorf("LoadFromJSON() loaded %d configs, want %d", len(configs), tt.wantCount)
				}
			} else {
				if err == nil {
					t.Errorf("LoadFromJSON() expected error but got none")
					return
				}
				if len(err.Error()) < len(tt.wantErr) || err.Error()[:len(tt.wantErr)] != tt.wantErr {
					t.Errorf("LoadFromJSON() error = %v, want error starting with %v", err.Error(), tt.wantErr)
				}
			}
		})
	}
}

// Test 7: Test LoadFromYAML
func TestConfigLoader_LoadFromYAML(t *testing.T) {
	loader := NewConfigLoader(nil)

	tests := []struct {
		name      string
		yamlStr   string
		wantCount int
		wantErr   string
	}{
		{
			name: "valid YAML",
			yamlStr: `agents:
  - id: agent1
    name: Agent 1
    role: executor
    model: claude-sonnet-4
`,
			wantCount: 1,
			wantErr:   "",
		},
		{
			name:      "empty YAML string",
			yamlStr:   "",
			wantCount: 0,
			wantErr:   "YAML string is required",
		},
		{
			name:      "invalid YAML",
			yamlStr:   "invalid: yaml: content: [",
			wantCount: 0,
			wantErr:   "failed to parse YAML",
		},
		{
			name: "multiple agents",
			yamlStr: `agents:
  - id: agent1
    name: Agent 1
    role: executor
    model: claude-sonnet-4
  - id: agent2
    name: Agent 2
    role: captain
    model: claude-opus-4
`,
			wantCount: 2,
			wantErr:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			configs, err := loader.LoadFromYAML(tt.yamlStr)
			if tt.wantErr == "" {
				if err != nil {
					t.Errorf("LoadFromYAML() unexpected error = %v", err)
					return
				}
				if len(configs) != tt.wantCount {
					t.Errorf("LoadFromYAML() loaded %d configs, want %d", len(configs), tt.wantCount)
				}
			} else {
				if err == nil {
					t.Errorf("LoadFromYAML() expected error but got none")
					return
				}
				if len(err.Error()) < len(tt.wantErr) || err.Error()[:len(tt.wantErr)] != tt.wantErr {
					t.Errorf("LoadFromYAML() error = %v, want error starting with %v", err.Error(), tt.wantErr)
				}
			}
		})
	}
}
