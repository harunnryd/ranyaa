package sandbox

import (
	"context"
	"time"
)

// Mode defines the sandbox mode
type Mode string

const (
	// ModeOff disables sandboxing
	ModeOff Mode = "off"
	// ModeAll sandboxes all tool executions
	ModeAll Mode = "all"
	// ModeTools sandboxes only specific tools
	ModeTools Mode = "tools"
)

// Scope defines the sandbox scope
type Scope string

const (
	// ScopeAgent creates one sandbox per agent
	ScopeAgent Scope = "agent"
	// ScopeSession creates one sandbox per session
	ScopeSession Scope = "session"
)

// Config defines sandbox configuration
type Config struct {
	// Mode specifies the sandbox mode (off, all, tools)
	Mode Mode `json:"mode"`

	// Scope specifies the sandbox scope (agent, session)
	Scope Scope `json:"scope"`

	// ResourceLimits defines resource constraints
	ResourceLimits ResourceLimits `json:"resource_limits"`

	// FilesystemAccess defines filesystem access rules
	FilesystemAccess FilesystemAccess `json:"filesystem_access"`

	// NetworkAccess defines network access rules
	NetworkAccess NetworkAccess `json:"network_access"`
}

// ResourceLimits defines resource constraints for sandboxed execution
type ResourceLimits struct {
	// MaxCPU limits CPU usage (percentage, 0-100)
	MaxCPU int `json:"max_cpu"`

	// MaxMemoryMB limits memory usage in megabytes
	MaxMemoryMB int `json:"max_memory_mb"`

	// MaxProcesses limits number of processes
	MaxProcesses int `json:"max_processes"`

	// Timeout limits execution time
	Timeout time.Duration `json:"timeout"`
}

// FilesystemAccess defines filesystem access rules
type FilesystemAccess struct {
	// AllowedPaths lists paths that can be accessed
	AllowedPaths []string `json:"allowed_paths"`

	// DeniedPaths lists paths that cannot be accessed
	DeniedPaths []string `json:"denied_paths"`

	// ReadOnly makes all filesystem access read-only
	ReadOnly bool `json:"read_only"`
}

// NetworkAccess defines network access rules
type NetworkAccess struct {
	// Enabled allows network access
	Enabled bool `json:"enabled"`

	// AllowedHosts lists hosts that can be accessed
	AllowedHosts []string `json:"allowed_hosts"`

	// DeniedHosts lists hosts that cannot be accessed
	DeniedHosts []string `json:"denied_hosts"`
}

// ExecuteRequest represents a sandbox execution request
type ExecuteRequest struct {
	// Command is the command to execute
	Command string `json:"command"`

	// Args are the command arguments
	Args []string `json:"args"`

	// Env are environment variables
	Env map[string]string `json:"env"`

	// WorkingDir is the working directory
	WorkingDir string `json:"working_dir"`

	// Stdin is the standard input
	Stdin []byte `json:"stdin"`

	// Timeout is the execution timeout
	Timeout time.Duration `json:"timeout"`
}

// ExecuteResult represents a sandbox execution result
type ExecuteResult struct {
	// Stdout is the standard output
	Stdout []byte `json:"stdout"`

	// Stderr is the standard error
	Stderr []byte `json:"stderr"`

	// ExitCode is the process exit code
	ExitCode int `json:"exit_code"`

	// Duration is the execution duration
	Duration time.Duration `json:"duration"`

	// Error is any execution error
	Error error `json:"error,omitempty"`
}

// Sandbox defines the interface for sandboxed execution
type Sandbox interface {
	// Execute runs a command in the sandbox
	Execute(ctx context.Context, req ExecuteRequest) (ExecuteResult, error)

	// Start initializes the sandbox
	Start(ctx context.Context) error

	// Stop cleans up the sandbox
	Stop(ctx context.Context) error

	// IsRunning returns whether the sandbox is running
	IsRunning() bool

	// GetConfig returns the sandbox configuration
	GetConfig() Config
}

// DefaultConfig returns a default sandbox configuration
func DefaultConfig() Config {
	return Config{
		Mode:  ModeOff,
		Scope: ScopeAgent,
		ResourceLimits: ResourceLimits{
			MaxCPU:       50,
			MaxMemoryMB:  512,
			MaxProcesses: 10,
			Timeout:      30 * time.Second,
		},
		FilesystemAccess: FilesystemAccess{
			AllowedPaths: []string{"/tmp"},
			DeniedPaths:  []string{"/etc", "/sys", "/proc"},
			ReadOnly:     false,
		},
		NetworkAccess: NetworkAccess{
			Enabled:      false,
			AllowedHosts: []string{},
			DeniedHosts:  []string{},
		},
	}
}

// ValidateConfig validates a sandbox configuration
func ValidateConfig(cfg Config) error {
	// Validate mode
	switch cfg.Mode {
	case ModeOff, ModeAll, ModeTools:
		// Valid
	default:
		return ErrInvalidMode
	}

	// Validate scope
	switch cfg.Scope {
	case ScopeAgent, ScopeSession:
		// Valid
	default:
		return ErrInvalidScope
	}

	// Validate resource limits
	if cfg.ResourceLimits.MaxCPU < 0 || cfg.ResourceLimits.MaxCPU > 100 {
		return ErrInvalidCPULimit
	}

	if cfg.ResourceLimits.MaxMemoryMB < 0 {
		return ErrInvalidMemoryLimit
	}

	if cfg.ResourceLimits.MaxProcesses < 0 {
		return ErrInvalidProcessLimit
	}

	if cfg.ResourceLimits.Timeout < 0 {
		return ErrInvalidTimeout
	}

	return nil
}
