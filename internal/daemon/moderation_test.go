package daemon

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/harun/ranya/internal/config"
	"github.com/harun/ranya/internal/logger"
	"github.com/harun/ranya/pkg/agent"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestModerationBlocksPromptBeforeExecution(t *testing.T) {
	origNewAgentRunner := newAgentRunner
	var callCount int32

	provider := &scriptedProvider{
		responses: []*agent.LLMResponse{{Content: "ok"}},
		onCall: func(agent.LLMRequest) {
			atomic.AddInt32(&callCount, 1)
		},
	}

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
	cfg.Moderation.Enabled = true
	cfg.Moderation.BlockedKeywords = []string{"forbidden"}

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

	result, _, err := d.executeRuntimeFlow(context.Background(), RuntimeRequest{
		Prompt:     "this contains forbidden content",
		SessionKey: "moderation:pre",
		Source:     "gateway",
	})

	require.Error(t, err)
	assert.True(t, result.Aborted)
	assert.Contains(t, err.Error(), "policy violation")
	assert.Equal(t, int32(0), atomic.LoadInt32(&callCount))
}

func TestModerationRedactsResponseAndAudits(t *testing.T) {
	origNewAgentRunner := newAgentRunner
	provider := &scriptedProvider{
		responses: []*agent.LLMResponse{{Content: "this has secret data"}},
	}
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
	cfg.Moderation.Enabled = true
	cfg.Moderation.BlockedKeywords = []string{"secret"}

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

	result, _, err := d.executeRuntimeFlow(context.Background(), RuntimeRequest{
		Prompt:     "hello",
		SessionKey: "moderation:post",
		Source:     "gateway",
	})

	require.NoError(t, err)
	assert.True(t, result.Aborted)
	assert.Equal(t, "[Content Redacted by Moderation Policy]", result.Response)

	auditPath := filepath.Join(tmpDir, "audit.log")
	waitForCondition(t, 2*time.Second, "moderation event audit", func() bool {
		data, readErr := os.ReadFile(auditPath)
		if readErr != nil {
			return false
		}
		return strings.Contains(string(data), "moderation_event")
	})
}
