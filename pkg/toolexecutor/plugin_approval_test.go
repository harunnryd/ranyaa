package toolexecutor

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/harun/ranya/pkg/sandbox"
)

// TestPluginToolApproval_SensitivePermissions tests that plugin tools with sensitive permissions require approval
func TestPluginToolApproval_SensitivePermissions(t *testing.T) {
	tests := []struct {
		name           string
		permissions    []string
		expectApproval bool
	}{
		{
			name:           "filesystem:write requires approval",
			permissions:    []string{"filesystem:write"},
			expectApproval: true,
		},
		{
			name:           "process:spawn requires approval",
			permissions:    []string{"process:spawn"},
			expectApproval: true,
		},
		{
			name:           "database:write requires approval",
			permissions:    []string{"database:write"},
			expectApproval: true,
		},
		{
			name:           "network:http requires approval",
			permissions:    []string{"network:http"},
			expectApproval: true,
		},
		{
			name:           "network:websocket requires approval",
			permissions:    []string{"network:websocket"},
			expectApproval: true,
		},
		{
			name:           "filesystem:read does not require approval",
			permissions:    []string{"filesystem:read"},
			expectApproval: false,
		},
		{
			name:           "database:read does not require approval",
			permissions:    []string{"database:read"},
			expectApproval: false,
		},
		{
			name:           "gateway:register does not require approval",
			permissions:    []string{"gateway:register"},
			expectApproval: false,
		},
		{
			name:           "multiple permissions with one sensitive",
			permissions:    []string{"filesystem:read", "database:write", "gateway:register"},
			expectApproval: true,
		},
		{
			name:           "no permissions",
			permissions:    []string{},
			expectApproval: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			te := New()

			toolDef := &ToolDefinition{
				Name:        "test_tool",
				Description: "Test tool",
				PluginID:    "test-plugin",
				Permissions: tt.permissions,
			}

			requiresApproval := te.requiresApproval(toolDef)

			if requiresApproval != tt.expectApproval {
				t.Errorf("Expected requiresApproval=%v, got %v for permissions %v",
					tt.expectApproval, requiresApproval, tt.permissions)
			}
		})
	}
}

// TestPluginToolApproval_NonPluginTool tests that non-plugin tools don't require approval
func TestPluginToolApproval_NonPluginTool(t *testing.T) {
	te := New()

	toolDef := &ToolDefinition{
		Name:        "regular_tool",
		Description: "Regular tool",
		PluginID:    "", // Not a plugin tool
		Permissions: []string{"filesystem:write", "process:spawn"},
	}

	requiresApproval := te.requiresApproval(toolDef)

	if requiresApproval {
		t.Error("Non-plugin tools should not require approval")
	}
}

// TestPluginToolExecution_WithApproval tests plugin tool execution with approval workflow
func TestPluginToolExecution_WithApproval(t *testing.T) {
	te := New()

	// Track approval requests
	var approvalRequests []ApprovalRequest
	var mu sync.Mutex

	// Create mock approval handler that approves and tracks requests
	approvalHandler := &MockApprovalHandler{
		Response: ApprovalResponse{
			Approved: true,
			Reason:   "test approval",
		},
	}

	// Wrap the handler to track requests
	wrappedHandler := &trackingApprovalHandler{
		handler:  approvalHandler,
		requests: &approvalRequests,
		mu:       &mu,
	}

	approvalManager := NewApprovalManager(wrappedHandler)
	te.SetApprovalManager(approvalManager)

	sandboxCfg := sandbox.DefaultConfig()
	sandboxManager := NewSandboxManager(sandboxCfg)
	t.Cleanup(func() {
		_ = sandboxManager.StopAll(context.Background())
	})
	te.SetSandboxManager(sandboxManager)

	// Create mock plugin runtime
	mockRuntime := &mockPluginToolExecutor{
		plugins: map[string]PluginToolProvider{
			"test-plugin": &mockPluginWithInfo{
				id:          "test-plugin",
				permissions: []string{"filesystem:write"},
				executeFunc: func(ctx context.Context, name string, params map[string]interface{}) (map[string]interface{}, error) {
					return map[string]interface{}{"result": "success"}, nil
				},
			},
		},
	}

	// Register plugin tool with sensitive permission
	toolDef := ToolDefinition{
		Name:        "write_file",
		Description: "Write file tool",
		Parameters:  []ToolParameter{},
	}

	err := te.RegisterPluginTool("test-plugin", toolDef, mockRuntime)
	if err != nil {
		t.Fatalf("Failed to register plugin tool: %v", err)
	}

	// Execute the tool
	ctx := context.Background()
	execCtx := &ExecutionContext{
		SessionKey: "test-session",
		AgentID:    "test-agent",
	}

	result := te.Execute(ctx, "write_file", map[string]interface{}{}, execCtx)

	if !result.Success {
		t.Fatalf("Tool execution failed: %s", result.Error)
	}

	// Verify approval was requested
	mu.Lock()
	defer mu.Unlock()

	if len(approvalRequests) != 1 {
		t.Errorf("Expected 1 approval request, got %d", len(approvalRequests))
	}

	if len(approvalRequests) > 0 {
		req := approvalRequests[0]
		if req.Context["plugin"] != "test-plugin" {
			t.Errorf("Expected plugin 'test-plugin', got '%s'", req.Context["plugin"])
		}
		if req.Context["tool"] != "write_file" {
			t.Errorf("Expected tool 'write_file', got '%s'", req.Context["tool"])
		}
	}
}

// TestPluginToolExecution_ApprovalDenied tests plugin tool execution when approval is denied
func TestPluginToolExecution_ApprovalDenied(t *testing.T) {
	te := New()

	// Create mock approval handler that denies
	approvalHandler := &MockApprovalHandler{
		Response: ApprovalResponse{
			Approved: false,
			Reason:   "user denied",
		},
	}
	approvalManager := NewApprovalManager(approvalHandler)
	te.SetApprovalManager(approvalManager)

	// Create mock plugin runtime
	mockRuntime := &mockPluginToolExecutor{
		plugins: map[string]PluginToolProvider{
			"test-plugin": &mockPluginWithInfo{
				id:          "test-plugin",
				permissions: []string{"process:spawn"},
				executeFunc: func(ctx context.Context, name string, params map[string]interface{}) (map[string]interface{}, error) {
					t.Error("Tool should not be executed when approval is denied")
					return nil, errors.New("should not execute")
				},
			},
		},
	}

	// Register plugin tool
	toolDef := ToolDefinition{
		Name:        "spawn_process",
		Description: "Spawn process tool",
		Parameters:  []ToolParameter{},
	}

	err := te.RegisterPluginTool("test-plugin", toolDef, mockRuntime)
	if err != nil {
		t.Fatalf("Failed to register plugin tool: %v", err)
	}

	// Execute the tool
	ctx := context.Background()
	execCtx := &ExecutionContext{
		SessionKey: "test-session",
		AgentID:    "test-agent",
	}

	result := te.Execute(ctx, "spawn_process", map[string]interface{}{}, execCtx)

	if result.Success {
		t.Fatal("Tool execution should fail when approval is denied")
	}

	if result.Error != "approval denied by user" {
		t.Errorf("Expected denial error, got: %s", result.Error)
	}

	// Verify metadata
	if result.Metadata["approval_denied"] != true {
		t.Error("Expected approval_denied metadata to be true")
	}
}

// TestPluginToolExecution_ApprovalTimeout tests plugin tool execution when approval times out
func TestPluginToolExecution_ApprovalTimeout(t *testing.T) {
	te := New()

	// Create mock approval handler that times out
	approvalHandler := &MockApprovalHandler{
		Delay: 200 * time.Millisecond,
	}
	approvalManager := NewApprovalManager(approvalHandler)
	approvalManager.SetDefaultTimeout(50 * time.Millisecond)
	te.SetApprovalManager(approvalManager)

	// Create mock plugin runtime
	mockRuntime := &mockPluginToolExecutor{
		plugins: map[string]PluginToolProvider{
			"test-plugin": &mockPluginWithInfo{
				id:          "test-plugin",
				permissions: []string{"database:write"},
			},
		},
	}

	// Register plugin tool
	toolDef := ToolDefinition{
		Name:        "write_db",
		Description: "Write database tool",
		Parameters:  []ToolParameter{},
	}

	err := te.RegisterPluginTool("test-plugin", toolDef, mockRuntime)
	if err != nil {
		t.Fatalf("Failed to register plugin tool: %v", err)
	}

	// Execute the tool
	ctx := context.Background()
	execCtx := &ExecutionContext{
		SessionKey: "test-session",
		AgentID:    "test-agent",
	}

	result := te.Execute(ctx, "write_db", map[string]interface{}{}, execCtx)

	if result.Success {
		t.Fatal("Tool execution should fail when approval times out")
	}

	if result.Metadata["approval_failed"] != true {
		t.Error("Expected approval_failed metadata to be true")
	}
}

// TestPluginToolExecution_NoApprovalManager tests that plugin tools work without approval manager
func TestPluginToolExecution_NoApprovalManager(t *testing.T) {
	te := New()
	// No approval manager set

	// Create mock plugin runtime
	mockRuntime := &mockPluginToolExecutor{
		plugins: map[string]PluginToolProvider{
			"test-plugin": &mockPluginWithInfo{
				id:          "test-plugin",
				permissions: []string{"filesystem:write"},
				executeFunc: func(ctx context.Context, name string, params map[string]interface{}) (map[string]interface{}, error) {
					return map[string]interface{}{"result": "success"}, nil
				},
			},
		},
	}

	// Register plugin tool
	toolDef := ToolDefinition{
		Name:        "write_file",
		Description: "Write file tool",
		Parameters:  []ToolParameter{},
	}

	err := te.RegisterPluginTool("test-plugin", toolDef, mockRuntime)
	if err != nil {
		t.Fatalf("Failed to register plugin tool: %v", err)
	}

	// Execute the tool - should succeed without approval
	ctx := context.Background()
	result := te.Execute(ctx, "write_file", map[string]interface{}{}, nil)

	if result.Success {
		t.Fatal("Tool execution should fail without approval manager")
	}
	if result.Error == "" {
		t.Fatal("Expected error when approval manager is missing")
	}
	if !strings.Contains(result.Error, "approval manager is required") {
		t.Fatalf("Expected approval manager error, got: %s", result.Error)
	}
}

// TestPluginToolExecution_TraceIDLogging tests that trace_id is logged for plugin tool execution
func TestPluginToolExecution_TraceIDLogging(t *testing.T) {
	te := New()

	// Create mock plugin runtime
	mockRuntime := &mockPluginToolExecutor{
		plugins: map[string]PluginToolProvider{
			"test-plugin": &mockPluginWithInfo{
				id:          "test-plugin",
				permissions: []string{"filesystem:read"},
				executeFunc: func(ctx context.Context, name string, params map[string]interface{}) (map[string]interface{}, error) {
					return map[string]interface{}{"result": "success"}, nil
				},
			},
		},
	}

	// Register plugin tool
	toolDef := ToolDefinition{
		Name:        "read_file",
		Description: "Read file tool",
		Parameters:  []ToolParameter{},
	}

	err := te.RegisterPluginTool("test-plugin", toolDef, mockRuntime)
	if err != nil {
		t.Fatalf("Failed to register plugin tool: %v", err)
	}

	// Execute the tool with trace_id
	ctx := context.Background()
	execCtx := &ExecutionContext{
		SessionKey: "trace-123",
		AgentID:    "test-agent",
	}

	result := te.Execute(ctx, "read_file", map[string]interface{}{}, execCtx)

	if !result.Success {
		t.Fatalf("Tool execution failed: %s", result.Error)
	}

	// Verify plugin metadata is included
	if result.Metadata["plugin"] != "test-plugin" {
		t.Errorf("Expected plugin metadata 'test-plugin', got '%v'", result.Metadata["plugin"])
	}
}

// mockPluginWithInfo implements both PluginToolProvider and PluginInfo interfaces
type mockPluginWithInfo struct {
	id          string
	permissions []string
	executeFunc func(ctx context.Context, name string, params map[string]interface{}) (map[string]interface{}, error)
}

func (m *mockPluginWithInfo) ExecuteTool(ctx context.Context, name string, params map[string]interface{}) (map[string]interface{}, error) {
	if m.executeFunc != nil {
		return m.executeFunc(ctx, name, params)
	}
	return map[string]interface{}{"result": "success"}, nil
}

func (m *mockPluginWithInfo) GetID() string {
	return m.id
}

func (m *mockPluginWithInfo) GetPermissions() []string {
	return m.permissions
}

// trackingApprovalHandler wraps an approval handler to track requests
type trackingApprovalHandler struct {
	handler  ApprovalHandler
	requests *[]ApprovalRequest
	mu       *sync.Mutex
}

func (t *trackingApprovalHandler) RequestApproval(ctx context.Context, req ApprovalRequest) (ApprovalResponse, error) {
	t.mu.Lock()
	*t.requests = append(*t.requests, req)
	t.mu.Unlock()
	return t.handler.RequestApproval(ctx, req)
}
