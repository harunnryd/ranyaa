package daemon

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/harun/ranya/internal/config"
	"github.com/harun/ranya/internal/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWorkspaceAgentsHotReload(t *testing.T) {
	tmpDir := t.TempDir()
	workspacePath := filepath.Join(tmpDir, "workspace")
	require.NoError(t, os.MkdirAll(workspacePath, 0755))

	agentsPath := filepath.Join(workspacePath, "AGENTS.md")
	initial := "## agent-one\n- name: Agent One\n- role: general\n- model: model-one\n- systemprompt: first prompt\n"
	require.NoError(t, os.WriteFile(agentsPath, []byte(initial), 0644))

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
	require.NoError(t, d.Start())
	t.Cleanup(func() {
		_ = d.Stop()
	})

	waitForCondition(t, 2*time.Second, "initial agents load", func() bool {
		d.mu.RLock()
		defer d.mu.RUnlock()
		return len(d.config.Agents) == 1 && d.config.Agents[0].ID == "agent-one"
	})

	updated := "## agent-two\n- name: Agent Two\n- role: general\n- model: model-two\n- systemprompt: updated prompt\n"
	require.NoError(t, os.WriteFile(agentsPath, []byte(updated), 0644))

	waitForCondition(t, 3*time.Second, "agents hot reload", func() bool {
		d.mu.RLock()
		defer d.mu.RUnlock()
		return len(d.config.Agents) == 1 && d.config.Agents[0].ID == "agent-two"
	})

	d.mu.RLock()
	defer d.mu.RUnlock()
	assert.Equal(t, "agent-two", d.config.Agents[0].ID)
	assert.Equal(t, "model-two", d.config.Agents[0].Model)
}
