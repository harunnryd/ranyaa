package daemon

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/harun/ranya/internal/config"
	"github.com/harun/ranya/internal/logger"
	"github.com/harun/ranya/internal/tracing"
	"github.com/harun/ranya/pkg/agent"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func createHookIntegrationDaemon(t *testing.T, cfg *config.Config, provider agent.LLMProvider) *Daemon {
	t.Helper()

	if provider == nil {
		provider = &scriptedProvider{
			responses: []*agent.LLMResponse{{Content: "ok"}},
		}
	}

	origNewAgentRunner := newAgentRunner
	newAgentRunner = func(cfg agent.Config) (*agent.Runner, error) {
		cfg.ProviderFactory = fixedProviderFactory{provider: provider}
		return agent.NewRunner(cfg)
	}
	t.Cleanup(func() {
		newAgentRunner = origNewAgentRunner
	})

	log, err := logger.New(logger.Config{Level: "debug", Console: false})
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = log.Close()
	})

	d, err := New(cfg, log)
	require.NoError(t, err)
	require.NoError(t, d.Start())
	t.Cleanup(func() {
		_ = d.Stop()
	})

	return d
}

func TestIntegrationHooksDaemonStartup(t *testing.T) {
	tmpDir := t.TempDir()
	workspacePath := filepath.Join(tmpDir, "workspace")
	require.NoError(t, os.MkdirAll(workspacePath, 0755))

	startupFile := filepath.Join(tmpDir, "startup_hook.txt")
	cfg := config.DefaultConfig()
	cfg.DataDir = tmpDir
	cfg.WorkspacePath = workspacePath
	cfg.Channels.Telegram.Enabled = false
	cfg.Webhook.Enabled = false
	cfg.Gateway.Port = reservePort(t)
	cfg.Gateway.SharedSecret = "hook-secret"
	cfg.AI.Profiles = []config.AIProfile{{
		ID:       "hook-profile",
		Provider: "anthropic",
		APIKey:   "sk-ant-hook",
		Priority: 1,
	}}
	cfg.Hooks.Enabled = true
	cfg.Hooks.Entries = []config.HookConfig{
		{
			ID:      "startup-hook",
			Event:   "daemon:startup",
			Script:  "echo startup > " + strconv.Quote(startupFile),
			Enabled: true,
		},
	}

	_ = createHookIntegrationDaemon(t, cfg, nil)

	waitForCondition(t, 2*time.Second, "startup hook output", func() bool {
		_, err := os.Stat(startupFile)
		return err == nil
	})

	content, err := os.ReadFile(startupFile)
	require.NoError(t, err)
	assert.Equal(t, "startup\n", string(content))
}

func TestIntegrationHooksCommandNewAndAgentBootstrap(t *testing.T) {
	tmpDir := t.TempDir()
	workspacePath := filepath.Join(tmpDir, "workspace")
	require.NoError(t, os.MkdirAll(workspacePath, 0755))

	hookFile := filepath.Join(tmpDir, "runtime_hooks.txt")
	cfg := config.DefaultConfig()
	cfg.DataDir = tmpDir
	cfg.WorkspacePath = workspacePath
	cfg.Channels.Telegram.Enabled = false
	cfg.Webhook.Enabled = false
	cfg.Gateway.Port = reservePort(t)
	cfg.Gateway.SharedSecret = "hook-secret"
	cfg.AI.Profiles = []config.AIProfile{{
		ID:       "hook-profile",
		Provider: "anthropic",
		APIKey:   "sk-ant-hook",
		Priority: 1,
	}}
	cfg.Hooks.Enabled = true
	cfg.Hooks.Entries = []config.HookConfig{
		{
			ID:      "command-hook",
			Event:   "command:new",
			Script:  "echo command:new >> " + strconv.Quote(hookFile),
			Enabled: true,
		},
		{
			ID:      "bootstrap-hook",
			Event:   "agent:bootstrap",
			Script:  "echo agent:bootstrap >> " + strconv.Quote(hookFile),
			Enabled: true,
		},
	}

	provider := &scriptedProvider{
		responses: []*agent.LLMResponse{
			{Content: "hooked"},
		},
	}
	d := createHookIntegrationDaemon(t, cfg, provider)

	resp := rpcCall(t, d, tracing.NewTraceID(), "agent.wait", map[string]interface{}{
		"prompt":     "trigger hooks",
		"sessionKey": "hooks:runtime",
		"config": map[string]interface{}{
			"model": "hook-model",
		},
	})
	require.Nil(t, resp.Error)

	waitForCondition(t, 3*time.Second, "command and bootstrap hooks", func() bool {
		raw, err := os.ReadFile(hookFile)
		if err != nil {
			return false
		}
		text := string(raw)
		return strings.Contains(text, "command:new") && strings.Contains(text, "agent:bootstrap")
	})

	content, err := os.ReadFile(hookFile)
	require.NoError(t, err)
	text := string(content)
	assert.Contains(t, text, "command:new")
	assert.Contains(t, text, "agent:bootstrap")
}
