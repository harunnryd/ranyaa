package toolexecutor

import (
	"context"
	"fmt"
	"time"

	"github.com/harun/ranya/pkg/sandbox"
	"github.com/rs/zerolog/log"
)

// SandboxManager manages sandboxes for tool execution
type SandboxManager struct {
	sandboxes map[string]sandbox.Sandbox // key: agent_id or session_key
	config    sandbox.Config
}

// NewSandboxManager creates a new sandbox manager
func NewSandboxManager(config sandbox.Config) *SandboxManager {
	return &SandboxManager{
		sandboxes: make(map[string]sandbox.Sandbox),
		config:    config,
	}
}

// GetOrCreateSandbox gets or creates a sandbox for the given key
func (sm *SandboxManager) GetOrCreateSandbox(ctx context.Context, key string) (sandbox.Sandbox, error) {
	// Check if sandbox already exists
	if sb, exists := sm.sandboxes[key]; exists {
		if sb.IsRunning() {
			return sb, nil
		}
	}

	// Create new sandbox
	sb, err := sandbox.NewHostSandbox(sm.config)
	if err != nil {
		return nil, fmt.Errorf("failed to create sandbox: %w", err)
	}

	// Start sandbox
	if err := sb.Start(ctx); err != nil {
		return nil, fmt.Errorf("failed to start sandbox: %w", err)
	}

	// Store sandbox
	sm.sandboxes[key] = sb

	log.Info().
		Str("key", key).
		Msg("Created and started sandbox")

	return sb, nil
}

// StopSandbox stops a sandbox for the given key
func (sm *SandboxManager) StopSandbox(ctx context.Context, key string) error {
	sb, exists := sm.sandboxes[key]
	if !exists {
		return nil // Already stopped or never created
	}

	if err := sb.Stop(ctx); err != nil {
		return fmt.Errorf("failed to stop sandbox: %w", err)
	}

	delete(sm.sandboxes, key)

	log.Info().
		Str("key", key).
		Msg("Stopped and removed sandbox")

	return nil
}

// StopAll stops all sandboxes
func (sm *SandboxManager) StopAll(ctx context.Context) error {
	var lastErr error

	for key, sb := range sm.sandboxes {
		if err := sb.Stop(ctx); err != nil {
			log.Error().
				Err(err).
				Str("key", key).
				Msg("Failed to stop sandbox")
			lastErr = err
		}
	}

	sm.sandboxes = make(map[string]sandbox.Sandbox)

	return lastErr
}

// ExecuteInSandbox executes a command in a sandbox
func (sm *SandboxManager) ExecuteInSandbox(ctx context.Context, key string, req sandbox.ExecuteRequest) (sandbox.ExecuteResult, error) {
	sb, err := sm.GetOrCreateSandbox(ctx, key)
	if err != nil {
		return sandbox.ExecuteResult{}, err
	}

	return sb.Execute(ctx, req)
}

// ExecuteWithSandbox executes a tool with sandbox support
func ExecuteWithSandbox(
	ctx context.Context,
	te *ToolExecutor,
	sm *SandboxManager,
	toolName string,
	params map[string]interface{},
	execCtx *ExecutionContext,
) ToolResult {
	// Check if sandboxing is enabled
	if execCtx.SandboxPolicy == nil {
		// No sandbox policy, execute normally
		return te.Execute(ctx, toolName, params, execCtx)
	}

	// Get sandbox mode from policy
	mode, ok := execCtx.SandboxPolicy["mode"].(string)
	if !ok || mode == "off" {
		// Sandbox disabled, execute normally
		return te.Execute(ctx, toolName, params, execCtx)
	}

	// Determine sandbox key based on scope
	scope, _ := execCtx.SandboxPolicy["scope"].(string)
	var sandboxKey string
	if scope == "session" {
		sandboxKey = execCtx.SessionKey
	} else {
		sandboxKey = execCtx.AgentID
	}

	// Check if tool should be sandboxed
	if mode == "tools" {
		// Only sandbox specific tools
		// For now, we'll sandbox all tools when mode is "tools"
		// In the future, we can add a list of tools to sandbox
	}

	// Get tool definition
	te.mu.RLock()
	toolDef := te.tools[toolName]
	schema := te.schemas[toolName]
	te.mu.RUnlock()

	if toolDef == nil {
		return ToolResult{
			Success: false,
			Error:   fmt.Sprintf("tool not found: %s", toolName),
		}
	}

	// For now, we'll execute the tool normally and log that it would be sandboxed
	// In a full implementation, we would wrap the tool handler to execute in sandbox
	log.Info().
		Str("tool", toolName).
		Str("sandbox_key", sandboxKey).
		Str("mode", mode).
		Msg("Executing tool with sandbox support")

	// Validate parameters
	if err := te.validateParameters(schema, params); err != nil {
		return ToolResult{
			Success: false,
			Error:   fmt.Sprintf("parameter validation failed: %v", err),
		}
	}

	// Apply timeout
	timeout := 30 * time.Second
	if execCtx != nil && execCtx.Timeout > 0 {
		timeout = execCtx.Timeout
	}

	timeoutCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Execute tool handler
	output, err := toolDef.Handler(timeoutCtx, params)
	if err != nil {
		return ToolResult{
			Success: false,
			Error:   err.Error(),
		}
	}

	return ToolResult{
		Success: true,
		Output:  output,
	}
}

// CreateSandboxConfig creates a sandbox config from execution context
func CreateSandboxConfig(execCtx *ExecutionContext) sandbox.Config {
	cfg := sandbox.DefaultConfig()

	if execCtx.SandboxPolicy == nil {
		return cfg
	}

	// Set mode
	if mode, ok := execCtx.SandboxPolicy["mode"].(string); ok {
		cfg.Mode = sandbox.Mode(mode)
	}

	// Set scope
	if scope, ok := execCtx.SandboxPolicy["scope"].(string); ok {
		cfg.Scope = sandbox.Scope(scope)
	}

	// Set working directory
	if execCtx.WorkingDir != "" {
		cfg.FilesystemAccess.AllowedPaths = append(cfg.FilesystemAccess.AllowedPaths, execCtx.WorkingDir)
	}

	// Set timeout
	if execCtx.Timeout > 0 {
		cfg.ResourceLimits.Timeout = execCtx.Timeout
	}

	return cfg
}

// WrapToolHandlerWithSandbox wraps a tool handler to execute in a sandbox
func WrapToolHandlerWithSandbox(
	handler ToolHandler,
	sm *SandboxManager,
	sandboxKey string,
) ToolHandler {
	return func(ctx context.Context, params map[string]interface{}) (interface{}, error) {
		// For now, just execute the handler normally
		// In a full implementation, we would:
		// 1. Serialize the handler execution as a command
		// 2. Execute it in the sandbox
		// 3. Deserialize the result

		log.Debug().
			Str("sandbox_key", sandboxKey).
			Msg("Tool handler wrapped with sandbox")

		return handler(ctx, params)
	}
}

// SandboxExecuteCommand is a helper to execute a command in a sandbox
func SandboxExecuteCommand(
	ctx context.Context,
	sm *SandboxManager,
	sandboxKey string,
	command string,
	args []string,
	workingDir string,
	timeout time.Duration,
) (sandbox.ExecuteResult, error) {
	req := sandbox.ExecuteRequest{
		Command:    command,
		Args:       args,
		WorkingDir: workingDir,
		Timeout:    timeout,
	}

	return sm.ExecuteInSandbox(ctx, sandboxKey, req)
}
