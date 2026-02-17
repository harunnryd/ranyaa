package sandbox

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDockerSandbox_StartStop(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Runtime = RuntimeDocker
	cfg.Docker.Image = "alpine:3.20"

	sb, err := NewDockerSandbox(cfg)
	require.NoError(t, err)
	assert.False(t, sb.IsRunning())

	require.NoError(t, sb.Start(context.Background()))
	assert.True(t, sb.IsRunning())

	require.NoError(t, sb.Stop(context.Background()))
	assert.False(t, sb.IsRunning())
}

func TestDockerSandbox_BuildDockerRunArgs(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Runtime = RuntimeDocker
	cfg.Docker.Image = "alpine:3.20"
	cfg.ResourceLimits.MaxCPU = 50
	cfg.ResourceLimits.MaxMemoryMB = 256
	cfg.ResourceLimits.MaxProcesses = 32
	cfg.FilesystemAccess.ReadOnly = true
	cfg.FilesystemAccess.AllowedPaths = []string{"/tmp/workspace"}
	cfg.NetworkAccess.Enabled = false

	sb, err := NewDockerSandbox(cfg)
	require.NoError(t, err)

	req := ExecuteRequest{
		Command:    "echo",
		Args:       []string{"hello"},
		WorkingDir: "/tmp/workspace",
		Env: map[string]string{
			"FOO": "bar",
		},
		Timeout: 5 * time.Second,
	}

	args := sb.buildDockerRunArgs(req)

	assert.Contains(t, args, "run")
	assert.Contains(t, args, "--rm")
	assert.Contains(t, args, "--network")
	assert.Contains(t, args, "none")
	assert.Contains(t, args, "--cpus")
	assert.Contains(t, args, "--memory")
	assert.Contains(t, args, "--pids-limit")
	assert.Contains(t, args, "--read-only")
	assert.Contains(t, args, "-v")
	assert.Contains(t, args, "/tmp/workspace:/tmp/workspace:ro")
	assert.Contains(t, args, "-w")
	assert.Contains(t, args, "/tmp/workspace")
	assert.Contains(t, args, "-e")
	assert.Contains(t, args, "FOO=bar")
	assert.Contains(t, args, "alpine:3.20")
	assert.Contains(t, args, "echo")
	assert.Contains(t, args, "hello")
}
