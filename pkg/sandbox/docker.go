package sandbox

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
)

// CheckDocker verifies that the Docker daemon is available and responsive.
func CheckDocker() error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "docker", "ps", "-q")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("docker is not available or not running: %w", err)
	}
	return nil
}

// DockerSandbox implements sandboxed execution using ephemeral Docker containers.
type DockerSandbox struct {
	config  Config
	running bool
	mu      sync.RWMutex
}

// NewDockerSandbox creates a new Docker-based sandbox.
func NewDockerSandbox(config Config) (*DockerSandbox, error) {
	if config.Runtime == "" {
		config.Runtime = RuntimeDocker
	}
	if err := ValidateConfig(config); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	return &DockerSandbox{
		config:  config,
		running: false,
	}, nil
}

// Start initializes the Docker sandbox.
func (d *DockerSandbox) Start(ctx context.Context) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.running {
		return ErrSandboxAlreadyRunning
	}

	log.Info().
		Str("mode", string(d.config.Mode)).
		Str("scope", string(d.config.Scope)).
		Str("runtime", string(RuntimeDocker)).
		Str("image", d.config.Docker.Image).
		Msg("Starting docker sandbox")

	d.running = true
	return nil
}

// Stop marks the Docker sandbox as stopped.
func (d *DockerSandbox) Stop(ctx context.Context) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if !d.running {
		return ErrSandboxNotRunning
	}

	log.Info().Msg("Stopping docker sandbox")
	d.running = false
	return nil
}

// IsRunning returns whether the sandbox is currently running.
func (d *DockerSandbox) IsRunning() bool {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.running
}

// GetConfig returns sandbox configuration.
func (d *DockerSandbox) GetConfig() Config {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.config
}

// Execute runs a command inside an ephemeral Docker container.
func (d *DockerSandbox) Execute(ctx context.Context, req ExecuteRequest) (ExecuteResult, error) {
	d.mu.RLock()
	if !d.running {
		d.mu.RUnlock()
		return ExecuteResult{}, ErrSandboxNotRunning
	}
	cfg := d.config
	d.mu.RUnlock()

	if strings.TrimSpace(req.Command) == "" {
		return ExecuteResult{}, fmt.Errorf("command is required")
	}

	if err := d.checkFilesystemAccess(req.WorkingDir); err != nil {
		return ExecuteResult{}, err
	}

	timeout := req.Timeout
	if timeout == 0 {
		timeout = cfg.ResourceLimits.Timeout
	}
	execCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	args := d.buildDockerRunArgs(req)
	cmd := exec.CommandContext(execCtx, "docker", args...)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if len(req.Stdin) > 0 {
		cmd.Stdin = bytes.NewReader(req.Stdin)
	}

	start := time.Now()
	err := cmd.Run()
	duration := time.Since(start)

	if execCtx.Err() == context.DeadlineExceeded {
		return ExecuteResult{
			Stdout:   stdout.Bytes(),
			Stderr:   stderr.Bytes(),
			ExitCode: -1,
			Duration: duration,
			Error:    ErrExecutionTimeout,
		}, ErrExecutionTimeout
	}

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
		Str("runtime", string(RuntimeDocker)).
		Str("image", cfg.Docker.Image).
		Str("command", req.Command).
		Strs("args", req.Args).
		Int("exit_code", exitCode).
		Dur("duration", duration).
		Msg("Command executed in docker sandbox")

	return result, nil
}

func (d *DockerSandbox) buildDockerRunArgs(req ExecuteRequest) []string {
	d.mu.RLock()
	cfg := d.config
	d.mu.RUnlock()

	args := []string{"run", "--rm", "--init"}

	networkMode := strings.TrimSpace(cfg.Docker.Network)
	if networkMode == "" {
		if cfg.NetworkAccess.Enabled {
			networkMode = "bridge"
		} else {
			networkMode = "none"
		}
	}
	args = append(args, "--network", networkMode)

	if cfg.ResourceLimits.MaxCPU > 0 {
		cpus := float64(cfg.ResourceLimits.MaxCPU) / 100.0
		args = append(args, "--cpus", strconv.FormatFloat(cpus, 'f', 2, 64))
	}
	if cfg.ResourceLimits.MaxMemoryMB > 0 {
		args = append(args, "--memory", fmt.Sprintf("%dm", cfg.ResourceLimits.MaxMemoryMB))
	}
	if cfg.ResourceLimits.MaxProcesses > 0 {
		args = append(args, "--pids-limit", strconv.Itoa(cfg.ResourceLimits.MaxProcesses))
	}

	if cfg.FilesystemAccess.ReadOnly {
		args = append(args, "--read-only")
	}

	if user := strings.TrimSpace(cfg.Docker.User); user != "" {
		args = append(args, "--user", user)
	}
	for _, secOpt := range cfg.Docker.SecurityOpt {
		if trimmed := strings.TrimSpace(secOpt); trimmed != "" {
			args = append(args, "--security-opt", trimmed)
		}
	}
	for _, cap := range cfg.Docker.CapDrop {
		if trimmed := strings.TrimSpace(cap); trimmed != "" {
			args = append(args, "--cap-drop", trimmed)
		}
	}
	args = append(args, cfg.Docker.ExtraArgs...)

	volumeMode := "rw"
	if cfg.FilesystemAccess.ReadOnly {
		volumeMode = "ro"
	}

	mounts := make(map[string]struct{})
	if wd := strings.TrimSpace(req.WorkingDir); wd != "" {
		mounts[filepath.Clean(wd)] = struct{}{}
	}
	for _, allowed := range cfg.FilesystemAccess.AllowedPaths {
		if trimmed := strings.TrimSpace(allowed); trimmed != "" {
			mounts[filepath.Clean(trimmed)] = struct{}{}
		}
	}

	paths := make([]string, 0, len(mounts))
	for path := range mounts {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	for _, path := range paths {
		args = append(args, "-v", fmt.Sprintf("%s:%s:%s", path, path, volumeMode))
	}

	if wd := strings.TrimSpace(req.WorkingDir); wd != "" {
		args = append(args, "-w", filepath.Clean(wd))
	}

	envKeys := make([]string, 0, len(req.Env))
	for key := range req.Env {
		envKeys = append(envKeys, key)
	}
	sort.Strings(envKeys)
	for _, key := range envKeys {
		args = append(args, "-e", fmt.Sprintf("%s=%s", key, req.Env[key]))
	}

	if len(req.Stdin) > 0 {
		args = append(args, "-i")
	}

	image := strings.TrimSpace(cfg.Docker.Image)
	if image == "" {
		image = DefaultConfig().Docker.Image
	}
	args = append(args, image, req.Command)
	args = append(args, req.Args...)

	return args
}

func (d *DockerSandbox) checkFilesystemAccess(path string) error {
	if path == "" {
		return nil
	}

	cleanPath := filepath.Clean(path)

	d.mu.RLock()
	cfg := d.config
	d.mu.RUnlock()

	for _, denied := range cfg.FilesystemAccess.DeniedPaths {
		if strings.HasPrefix(cleanPath, denied) {
			return fmt.Errorf("%w: %s", ErrFilesystemAccessDenied, path)
		}
	}

	if len(cfg.FilesystemAccess.AllowedPaths) == 0 {
		return nil
	}
	for _, allowed := range cfg.FilesystemAccess.AllowedPaths {
		if strings.HasPrefix(cleanPath, allowed) {
			return nil
		}
	}

	return fmt.Errorf("%w: %s", ErrFilesystemAccessDenied, path)
}
