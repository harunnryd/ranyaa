package daemon

import (
	"context"
	"os"
	"sync"
	"testing"

	"github.com/harun/ranya/internal/config"
	"github.com/harun/ranya/internal/logger"
	"github.com/harun/ranya/pkg/agent"
	"github.com/harun/ranya/pkg/routing"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewRouter(t *testing.T) {
	tmpDir := t.TempDir()

	cfg := config.DefaultConfig()
	cfg.DataDir = tmpDir
	cfg.WorkspacePath = tmpDir + "/workspace"
	cfg.AI.Profiles = []config.AIProfile{{ID: "test-profile", Provider: "anthropic", APIKey: "sk-ant-test123", Priority: 1}}
	cfg.Channels.Telegram.Enabled = false
	cfg.Gateway.SharedSecret = "test-secret"

	// Create workspace directory
	err := os.MkdirAll(cfg.WorkspacePath, 0755)
	require.NoError(t, err)

	logCfg := logger.Config{
		Level:   "info",
		Console: false,
	}
	log, err := logger.New(logCfg)
	require.NoError(t, err)
	defer log.Close()

	daemon, err := New(cfg, log)
	require.NoError(t, err)

	router := NewRouter(daemon)
	assert.NotNil(t, router)
	assert.Equal(t, daemon, router.daemon)
}

func TestRouteMessage(t *testing.T) {
	origNewAgentRunner := newAgentRunner
	newAgentRunner = func(cfg agent.Config) (*agent.Runner, error) {
		cfg.ProviderFactory = fixedProviderFactory{
			provider: &scriptedProvider{
				responses: []*agent.LLMResponse{{Content: "ok"}},
			},
		}
		return agent.NewRunner(cfg)
	}
	defer func() {
		newAgentRunner = origNewAgentRunner
	}()

	tmpDir := t.TempDir()

	cfg := config.DefaultConfig()
	cfg.DataDir = tmpDir
	cfg.WorkspacePath = tmpDir + "/workspace"
	cfg.AI.Profiles = []config.AIProfile{{ID: "test-profile", Provider: "anthropic", APIKey: "sk-ant-test123", Priority: 1}}
	cfg.Channels.Telegram.Enabled = false
	cfg.Gateway.SharedSecret = "test-secret"

	// Create workspace directory
	err := os.MkdirAll(cfg.WorkspacePath, 0755)
	require.NoError(t, err)

	logCfg := logger.Config{
		Level:   "info",
		Console: false,
	}
	log, err := logger.New(logCfg)
	require.NoError(t, err)
	defer log.Close()

	daemon, err := New(cfg, log)
	require.NoError(t, err)

	router := NewRouter(daemon)

	msg := Message{
		SessionKey: "test-session",
		Source:     "telegram",
		Content:    "Hello, world!",
		Metadata:   map[string]interface{}{},
	}

	err = router.RouteMessage(context.Background(), msg)
	assert.NoError(t, err)
}

func TestRouteMessage_RoutingBindingsResolveAgent(t *testing.T) {
	origNewAgentRunner := newAgentRunner

	var (
		mu         sync.Mutex
		calledWith []string
	)
	provider := &scriptedProvider{
		responses: []*agent.LLMResponse{{Content: "ok"}},
		onCall: func(req agent.LLMRequest) {
			mu.Lock()
			calledWith = append(calledWith, req.Model)
			mu.Unlock()
		},
	}

	newAgentRunner = func(cfg agent.Config) (*agent.Runner, error) {
		cfg.ProviderFactory = fixedProviderFactory{
			provider: provider,
		}
		return agent.NewRunner(cfg)
	}
	defer func() {
		newAgentRunner = origNewAgentRunner
	}()

	tmpDir := t.TempDir()

	cfg := config.DefaultConfig()
	cfg.DataDir = tmpDir
	cfg.WorkspacePath = tmpDir + "/workspace"
	cfg.AI.Profiles = []config.AIProfile{{ID: "test-profile", Provider: "anthropic", APIKey: "sk-ant-test123", Priority: 1}}
	cfg.Channels.Telegram.Enabled = false
	cfg.Gateway.SharedSecret = "test-secret"
	cfg.Agents = []config.AgentConfig{
		{
			ID:          "default",
			Name:        "Default Agent",
			Role:        "general",
			Model:       "model-default",
			Temperature: 0.7,
			MaxTokens:   4096,
			Tools: config.ToolPolicyConfig{
				Allow: []string{"*"},
				Deny:  []string{},
			},
			Sandbox: config.SandboxConfig{
				Mode:  "tools",
				Scope: "session",
			},
			MaxConcurrentSubAgents: 5,
			AllowedSubAgents:       []string{"*"},
		},
		{
			ID:          "routed-agent",
			Name:        "Routed Agent",
			Role:        "general",
			Model:       "model-routed",
			Temperature: 0.7,
			MaxTokens:   4096,
			Tools: config.ToolPolicyConfig{
				Allow: []string{"*"},
				Deny:  []string{},
			},
			Sandbox: config.SandboxConfig{
				Mode:  "tools",
				Scope: "session",
			},
			MaxConcurrentSubAgents: 5,
			AllowedSubAgents:       []string{"*"},
		},
	}

	// Create workspace directory
	err := os.MkdirAll(cfg.WorkspacePath, 0755)
	require.NoError(t, err)

	logCfg := logger.Config{
		Level:   "info",
		Console: false,
	}
	log, err := logger.New(logCfg)
	require.NoError(t, err)
	defer log.Close()

	daemon, err := New(cfg, log)
	require.NoError(t, err)

	route := &routing.Route{
		ID:       "peer-route",
		Name:     "Peer Route",
		Handler:  "routed-agent",
		Priority: 100,
		Enabled:  true,
		Patterns: []routing.RoutePattern{
			{Type: routing.PatternTypeWildcard, Value: "*"},
		},
		Metadata: map[string]interface{}{
			"binding": map[string]interface{}{
				"peer": "peer-42",
			},
		},
	}
	require.NoError(t, daemon.GetRoutingService().AddRoute(route))

	router := NewRouter(daemon)
	msg := Message{
		SessionKey: "test-session",
		Source:     "telegram",
		Content:    "Hello, route me",
		Metadata: map[string]interface{}{
			"peer": "peer-42",
		},
	}

	err = router.RouteMessage(context.Background(), msg)
	require.NoError(t, err)

	mu.Lock()
	defer mu.Unlock()
	require.NotEmpty(t, calledWith)
	assert.Equal(t, "model-routed", calledWith[0])
}
