package sandbox

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	assert.Equal(t, RuntimeHost, cfg.Runtime)
	assert.Equal(t, ModeOff, cfg.Mode)
	assert.Equal(t, ScopeAgent, cfg.Scope)
	assert.Equal(t, 50, cfg.ResourceLimits.MaxCPU)
	assert.Equal(t, 512, cfg.ResourceLimits.MaxMemoryMB)
	assert.Equal(t, 10, cfg.ResourceLimits.MaxProcesses)
	assert.Equal(t, 30*time.Second, cfg.ResourceLimits.Timeout)
	assert.False(t, cfg.NetworkAccess.Enabled)
}

func TestValidateConfig_ValidConfig(t *testing.T) {
	tests := []struct {
		name string
		cfg  Config
	}{
		{
			name: "default config",
			cfg:  DefaultConfig(),
		},
		{
			name: "mode all",
			cfg: Config{
				Mode:  ModeAll,
				Scope: ScopeAgent,
				ResourceLimits: ResourceLimits{
					MaxCPU:       100,
					MaxMemoryMB:  1024,
					MaxProcesses: 20,
					Timeout:      60 * time.Second,
				},
			},
		},
		{
			name: "mode tools",
			cfg: Config{
				Mode:  ModeTools,
				Scope: ScopeSession,
				ResourceLimits: ResourceLimits{
					MaxCPU:       0,
					MaxMemoryMB:  0,
					MaxProcesses: 0,
					Timeout:      0,
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateConfig(tt.cfg)
			require.NoError(t, err)
		})
	}
}

func TestValidateConfig_InvalidMode(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Mode = Mode("invalid")

	err := ValidateConfig(cfg)
	assert.ErrorIs(t, err, ErrInvalidMode)
}

func TestValidateConfig_InvalidRuntime(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Runtime = Runtime("invalid")

	err := ValidateConfig(cfg)
	assert.ErrorIs(t, err, ErrInvalidRuntime)
}

func TestValidateConfig_DockerRuntimeRequiresImage(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Runtime = RuntimeDocker
	cfg.Docker.Image = ""

	err := ValidateConfig(cfg)
	assert.ErrorIs(t, err, ErrDockerImageRequired)
}

func TestValidateConfig_InvalidScope(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Scope = Scope("invalid")

	err := ValidateConfig(cfg)
	assert.ErrorIs(t, err, ErrInvalidScope)
}

func TestValidateConfig_InvalidCPULimit(t *testing.T) {
	tests := []struct {
		name   string
		maxCPU int
	}{
		{"negative", -1},
		{"too high", 101},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := DefaultConfig()
			cfg.ResourceLimits.MaxCPU = tt.maxCPU

			err := ValidateConfig(cfg)
			assert.ErrorIs(t, err, ErrInvalidCPULimit)
		})
	}
}

func TestValidateConfig_InvalidMemoryLimit(t *testing.T) {
	cfg := DefaultConfig()
	cfg.ResourceLimits.MaxMemoryMB = -1

	err := ValidateConfig(cfg)
	assert.ErrorIs(t, err, ErrInvalidMemoryLimit)
}

func TestValidateConfig_InvalidProcessLimit(t *testing.T) {
	cfg := DefaultConfig()
	cfg.ResourceLimits.MaxProcesses = -1

	err := ValidateConfig(cfg)
	assert.ErrorIs(t, err, ErrInvalidProcessLimit)
}

func TestValidateConfig_InvalidTimeout(t *testing.T) {
	cfg := DefaultConfig()
	cfg.ResourceLimits.Timeout = -1 * time.Second

	err := ValidateConfig(cfg)
	assert.ErrorIs(t, err, ErrInvalidTimeout)
}

func TestMode_Constants(t *testing.T) {
	assert.Equal(t, Mode("off"), ModeOff)
	assert.Equal(t, Mode("all"), ModeAll)
	assert.Equal(t, Mode("tools"), ModeTools)
}

func TestScope_Constants(t *testing.T) {
	assert.Equal(t, Scope("agent"), ScopeAgent)
	assert.Equal(t, Scope("session"), ScopeSession)
}

func TestRuntime_Constants(t *testing.T) {
	assert.Equal(t, Runtime("host"), RuntimeHost)
	assert.Equal(t, Runtime("docker"), RuntimeDocker)
}
