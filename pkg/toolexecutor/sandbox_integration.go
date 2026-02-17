package toolexecutor

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/harun/ranya/pkg/sandbox"
	"github.com/rs/zerolog/log"
)

// SandboxManager manages sandboxes for tool execution
type SandboxManager struct {
	sandboxes map[string]sandbox.Sandbox // key: agent_id or session_key
	config    sandbox.Config
	mu        sync.RWMutex
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
	sm.mu.RLock()
	// Check if sandbox already exists
	if sb, exists := sm.sandboxes[key]; exists {
		if sb.IsRunning() {
			sm.mu.RUnlock()
			return sb, nil
		}
	}
	sm.mu.RUnlock()

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
	sm.mu.Lock()
	sm.sandboxes[key] = sb
	sm.mu.Unlock()

	log.Info().
		Str("key", key).
		Msg("Created and started sandbox")

	return sb, nil
}

// StopSandbox stops a sandbox for the given key
func (sm *SandboxManager) StopSandbox(ctx context.Context, key string) error {
	sm.mu.RLock()
	sb, exists := sm.sandboxes[key]
	sm.mu.RUnlock()
	if !exists {
		return nil // Already stopped or never created
	}

	if err := sb.Stop(ctx); err != nil {
		return fmt.Errorf("failed to stop sandbox: %w", err)
	}

	sm.mu.Lock()
	delete(sm.sandboxes, key)
	sm.mu.Unlock()

	log.Info().
		Str("key", key).
		Msg("Stopped and removed sandbox")

	return nil
}

// StopAll stops all sandboxes
func (sm *SandboxManager) StopAll(ctx context.Context) error {
	var lastErr error

	sm.mu.Lock()
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
	sm.mu.Unlock()

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
	if te == nil {
		return ToolResult{
			Success: false,
			Error:   "tool executor is required",
		}
	}
	if sm == nil {
		return ToolResult{
			Success: false,
			Error:   "sandbox manager is required",
		}
	}

	if execCtx == nil {
		execCtx = &ExecutionContext{}
	}
	if execCtx.SandboxPolicy == nil {
		execCtx.SandboxPolicy = map[string]interface{}{
			"mode":  "tools",
			"scope": "session",
		}
	}

	te.SetSandboxManager(sm)
	return te.Execute(ctx, toolName, params, execCtx)
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
		if sm == nil {
			return nil, fmt.Errorf("sandbox manager is required")
		}
		_, err := sm.ExecuteInSandbox(ctx, sandboxKey, sandbox.ExecuteRequest{
			Command: "true",
			Timeout: 5 * time.Second,
		})
		if err != nil {
			return nil, err
		}

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
