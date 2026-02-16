package orchestrator

import (
	"context"
	"testing"

	"github.com/harun/ranya/pkg/toolexecutor"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestE2E_ToolPolicyEnforcement_AgentExecution tests tool policy enforcement during agent execution
func TestE2E_ToolPolicyEnforcement_AgentExecution(t *testing.T) {
	// Setup tool executor
	te := toolexecutor.New()

	// Register test tools
	tools := []string{"read_file", "write_file", "exec", "search"}
	for _, name := range tools {
		toolName := name // capture for closure
		def := toolexecutor.ToolDefinition{
			Name:        toolName,
			Description: "Test tool: " + toolName,
			Parameters:  []toolexecutor.ToolParameter{},
			Handler: func(ctx context.Context, params map[string]interface{}) (interface{}, error) {
				return map[string]interface{}{
					"tool":   toolName,
					"result": "executed successfully",
				}, nil
			},
		}
		err := te.RegisterTool(def)
		require.NoError(t, err)
	}

	// Test different agent policies
	tests := []struct {
		name          string
		agentID       string
		policy        toolexecutor.ToolPolicy
		toolsToTry    []string
		expectedAllow []string
		expectedDeny  []string
	}{
		{
			name:    "captain with limited tools",
			agentID: "captain",
			policy: toolexecutor.ToolPolicy{
				Allow: []string{"search"},
				Deny:  []string{},
			},
			toolsToTry:    []string{"search", "read_file", "write_file"},
			expectedAllow: []string{"search"},
			expectedDeny:  []string{"read_file", "write_file"},
		},
		{
			name:    "executor with all except exec",
			agentID: "executor",
			policy: toolexecutor.ToolPolicy{
				Allow: []string{"*"},
				Deny:  []string{"exec"},
			},
			toolsToTry:    []string{"read_file", "write_file", "exec"},
			expectedAllow: []string{"read_file", "write_file"},
			expectedDeny:  []string{"exec"},
		},
		{
			name:    "critic with read-only",
			agentID: "critic",
			policy: toolexecutor.ToolPolicy{
				Allow: []string{"read_file"},
				Deny:  []string{},
			},
			toolsToTry:    []string{"read_file", "write_file"},
			expectedAllow: []string{"read_file"},
			expectedDeny:  []string{"write_file"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			allowedTools := []string{}
			blockedTools := []string{}

			for _, toolName := range tt.toolsToTry {
				execCtx := &toolexecutor.ExecutionContext{
					AgentID:    tt.agentID,
					ToolPolicy: &tt.policy,
				}

				result := te.Execute(context.Background(), toolName, map[string]interface{}{}, execCtx)

				if result.Success {
					allowedTools = append(allowedTools, toolName)
				} else {
					blockedTools = append(blockedTools, toolName)
				}
			}

			// Verify allowed tools
			for _, expected := range tt.expectedAllow {
				assert.Contains(t, allowedTools, expected, "Expected %s to be allowed", expected)
			}

			// Verify denied tools
			for _, expected := range tt.expectedDeny {
				assert.Contains(t, blockedTools, expected, "Expected %s to be denied", expected)
			}
		})
	}
}
