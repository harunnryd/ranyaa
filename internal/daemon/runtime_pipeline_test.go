package daemon

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/harun/ranya/internal/config"
	"github.com/harun/ranya/internal/logger"
	"github.com/harun/ranya/pkg/agent"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDispatchPlannedStep_SkipsOrchestratorForSingleStep(t *testing.T) {
	origNewAgentRunner := newAgentRunner
	provider := &scriptedProvider{responses: []*agent.LLMResponse{{Content: "ok"}}}
	newAgentRunner = func(cfg agent.Config) (*agent.Runner, error) {
		cfg.ProviderFactory = fixedProviderFactory{provider: provider}
		return agent.NewRunner(cfg)
	}
	t.Cleanup(func() {
		newAgentRunner = origNewAgentRunner
	})

	tmpDir := t.TempDir()
	workspacePath := filepath.Join(tmpDir, "workspace")
	require.NoError(t, os.MkdirAll(workspacePath, 0755))

	cfg := config.DefaultConfig()
	cfg.DataDir = tmpDir
	cfg.WorkspacePath = workspacePath
	cfg.Gateway.SharedSecret = "test-secret"
	cfg.Gateway.Port = reservePort(t)
	cfg.Channels.Telegram.Enabled = false
	cfg.Webhook.Enabled = false
	cfg.AI.Profiles = []config.AIProfile{{
		ID:       "test-profile",
		Provider: "anthropic",
		APIKey:   "sk-test",
		Priority: 1,
	}}

	log, err := logger.New(logger.Config{Level: "debug", Console: false})
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = log.Close()
	})

	d, err := New(cfg, log)
	require.NoError(t, err)
	d.orchestrator = nil
	defer func() {
		_ = d.Stop()
	}()

	result, _, err := d.executeRuntimeFlow(context.Background(), RuntimeRequest{
		Prompt:     "hello",
		SessionKey: "single-step",
		Source:     "gateway",
	})
	require.NoError(t, err)
	assert.Equal(t, "ok", result.Response)
}

func TestValidateStepResult_UsesCriticAgent(t *testing.T) {
	origNewAgentRunner := newAgentRunner
	provider := &scriptedProvider{responses: []*agent.LLMResponse{{Content: "REJECTED: missing requirements"}}}
	newAgentRunner = func(cfg agent.Config) (*agent.Runner, error) {
		cfg.ProviderFactory = fixedProviderFactory{provider: provider}
		return agent.NewRunner(cfg)
	}
	t.Cleanup(func() {
		newAgentRunner = origNewAgentRunner
	})

	tmpDir := t.TempDir()
	workspacePath := filepath.Join(tmpDir, "workspace")
	require.NoError(t, os.MkdirAll(workspacePath, 0755))

	cfg := config.DefaultConfig()
	cfg.DataDir = tmpDir
	cfg.WorkspacePath = workspacePath
	cfg.Gateway.SharedSecret = "test-secret"
	cfg.Gateway.Port = reservePort(t)
	cfg.Channels.Telegram.Enabled = false
	cfg.Webhook.Enabled = false
	cfg.AI.Profiles = []config.AIProfile{{
		ID:       "test-profile",
		Provider: "anthropic",
		APIKey:   "sk-test",
		Priority: 1,
	}}
	cfg.Agents = []config.AgentConfig{
		cfg.Agents[0],
		{
			ID:     "critic",
			Name:   "Critic",
			Role:   "critic",
			Model:  "model-critic",
			Tools:  config.ToolPolicyConfig{Allow: []string{"*"}},
			Sandbox: config.SandboxConfig{Mode: "tools", Scope: "session", Runtime: "host"},
		},
	}

	log, err := logger.New(logger.Config{Level: "debug", Console: false})
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = log.Close()
	})

	d, err := New(cfg, log)
	require.NoError(t, err)
	defer func() {
		_ = d.Stop()
	}()

	valid, reason, err := d.validateStepResult(context.Background(), RuntimeRequest{
		Prompt:     "do the thing",
		SessionKey: "critic-check",
		AgentID:    "default",
	}, "do the thing", "bad response", agent.DefaultConfig())

	require.NoError(t, err)
	assert.False(t, valid)
	assert.Contains(t, reason, "REJECTED")
	assert.GreaterOrEqual(t, provider.calls, 1)
}
