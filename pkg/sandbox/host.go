package sandbox

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
)

// HostSandbox implements sandboxed execution using host-based isolation
type HostSandbox struct {
	config  Config
	running bool
	mu      sync.RWMutex
}

// NewHostSandbox creates a new host-based sandbox
func NewHostSandbox(config Config) (*HostSandbox, error) {
	if err := ValidateConfig(config); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	return &HostSandbox{
		config:  config,
		running: false,
	}, nil
}

// Start initializes the sandbox
func (h *HostSandbox) Start(ctx context.Context) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.running {
		return ErrSandboxAlreadyRunning
	}

	log.Info().
		Str("mode", string(h.config.Mode)).
		Str("scope", string(h.config.Scope)).
		Msg("Starting host sandbox")

	h.running = true
	return nil
}

// Stop cleans up the sandbox
func (h *HostSandbox) Stop(ctx context.Context) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	if !h.running {
		return ErrSandboxNotRunning
	}

	log.Info().Msg("Stopping host sandbox")

	h.running = false
	return nil
}

// IsRunning returns whether the sandbox is running
func (h *HostSandbox) IsRunning() bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.running
}

// GetConfig returns the sandbox configuration
func (h *HostSandbox) GetConfig() Config {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.config
}

// Execute runs a command in the sandbox
func (h *HostSandbox) Execute(ctx context.Context, req ExecuteRequest) (ExecuteResult, error) {
	h.mu.RLock()
	if !h.running {
		h.mu.RUnlock()
		return ExecuteResult{}, ErrSandboxNotRunning
	}
	h.mu.RUnlock()

	// Check filesystem access
	if err := h.checkFilesystemAccess(req.WorkingDir); err != nil {
		return ExecuteResult{}, err
	}

	// Apply timeout
	timeout := req.Timeout
	if timeout == 0 {
		timeout = h.config.ResourceLimits.Timeout
	}

	execCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Create command
	cmd := exec.CommandContext(execCtx, req.Command, req.Args...)

	// Set working directory
	if req.WorkingDir != "" {
		cmd.Dir = req.WorkingDir
	}

	// Set environment variables
	cmd.Env = h.buildEnvironment(req.Env)

	// Set up stdin/stdout/stderr
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if len(req.Stdin) > 0 {
		cmd.Stdin = bytes.NewReader(req.Stdin)
	}

	// Execute command
	start := time.Now()
	err := cmd.Run()
	duration := time.Since(start)

	// Check for timeout first
	if execCtx.Err() == context.DeadlineExceeded {
		return ExecuteResult{
			Stdout:   stdout.Bytes(),
			Stderr:   stderr.Bytes(),
			ExitCode: -1,
			Duration: duration,
			Error:    ErrExecutionTimeout,
		}, ErrExecutionTimeout
	}

	// Get exit code
	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		}
	}

	result := ExecuteResult{
		Stdout:   stdout.Bytes(),
		Stderr:   stderr.Bytes(),
		ExitCode: exitCode,
		Duration: duration,
	}

	if err != nil && exitCode == 0 {
		result.Error = err
	}

	log.Debug().
		Str("command", req.Command).
		Strs("args", req.Args).
		Int("exit_code", exitCode).
		Dur("duration", duration).
		Msg("Command executed in sandbox")

	return result, nil
}

// checkFilesystemAccess checks if a path is allowed
func (h *HostSandbox) checkFilesystemAccess(path string) error {
	if path == "" {
		return nil
	}

	// Clean the path
	cleanPath := filepath.Clean(path)

	// Check denied paths first
	for _, denied := range h.config.FilesystemAccess.DeniedPaths {
		if strings.HasPrefix(cleanPath, denied) {
			return fmt.Errorf("%w: %s", ErrFilesystemAccessDenied, path)
		}
	}

	// If allowed paths is empty, allow all (except denied)
	if len(h.config.FilesystemAccess.AllowedPaths) == 0 {
		return nil
	}

	// Check allowed paths
	for _, allowed := range h.config.FilesystemAccess.AllowedPaths {
		if strings.HasPrefix(cleanPath, allowed) {
			return nil
		}
	}

	return fmt.Errorf("%w: %s", ErrFilesystemAccessDenied, path)
}

// buildEnvironment builds the environment variables for the command
func (h *HostSandbox) buildEnvironment(env map[string]string) []string {
	// Start with minimal environment
	result := []string{
		"PATH=/usr/local/bin:/usr/bin:/bin",
		"HOME=/tmp",
	}

	// Add custom environment variables
	for key, value := range env {
		result = append(result, fmt.Sprintf("%s=%s", key, value))
	}

	return result
}

// SetConfig updates the sandbox configuration
func (h *HostSandbox) SetConfig(config Config) error {
	if err := ValidateConfig(config); err != nil {
		return fmt.Errorf("invalid config: %w", err)
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	h.config = config
	return nil
}
