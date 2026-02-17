package daemon

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/harun/ranya/internal/config"
	"github.com/harun/ranya/internal/logger"
	"github.com/harun/ranya/internal/tracing"
	"github.com/harun/ranya/pkg/agent"
	"github.com/harun/ranya/pkg/browser"
	"github.com/harun/ranya/pkg/channels"
	"github.com/harun/ranya/pkg/commandqueue"
	"github.com/harun/ranya/pkg/cron"
	"github.com/harun/ranya/pkg/gateway"
	"github.com/harun/ranya/pkg/plugin"
	"github.com/harun/ranya/pkg/routing"
	"github.com/harun/ranya/pkg/session"
	"github.com/harun/ranya/pkg/toolexecutor"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type scriptedProvider struct {
	providerName string
	responses    []*agent.LLMResponse
	delay        time.Duration
	onCall       func(req agent.LLMRequest)

	mu    sync.Mutex
	calls int
}

func (p *scriptedProvider) Call(ctx context.Context, req agent.LLMRequest) (*agent.LLMResponse, error) {
	if p.onCall != nil {
		p.onCall(req)
	}

	if p.delay > 0 {
		select {
		case <-time.After(p.delay):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	p.mu.Lock()
	idx := p.calls
	p.calls++
	p.mu.Unlock()

	if idx < len(p.responses) {
		resp := p.responses[idx]
		copyResp := *resp
		if resp.ToolCalls != nil {
			copyResp.ToolCalls = make([]agent.ToolCall, len(resp.ToolCalls))
			copy(copyResp.ToolCalls, resp.ToolCalls)
		}
		return &copyResp, nil
	}

	return &agent.LLMResponse{Content: "ok"}, nil
}

func (p *scriptedProvider) Provider() string {
	if p.providerName == "" {
		return "anthropic"
	}
	return p.providerName
}

type fixedProviderFactory struct {
	provider agent.LLMProvider
}

func (f fixedProviderFactory) NewProvider(_ agent.AuthProfile) (agent.LLMProvider, error) {
	return f.provider, nil
}

func reservePort(t *testing.T) int {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer listener.Close()
	return listener.Addr().(*net.TCPAddr).Port
}

func writeBrowserConfig(t *testing.T, dataDir string) {
	t.Helper()

	cfg := browser.BrowserConfig{
		Profiles: []browser.BrowserProfile{
			{
				Name:      "default",
				CDPPort:   reservePort(t),
				Headless:  true,
				NoSandbox: true,
			},
		},
		Pool: browser.PoolConfig{
			MaxPages:           2,
			MaxMemoryMB:        256,
			IdlePageTimeout:    60,
			IdleBrowserTimeout: 60,
		},
		Security: browser.SecurityConfig{
			AllowFileUrls:      false,
			AllowLocalhostUrls: true,
		},
	}

	data, err := json.Marshal(cfg)
	require.NoError(t, err)
	err = os.WriteFile(filepath.Join(dataDir, "browser.json"), data, 0600)
	require.NoError(t, err)
}

func createIntegrationDaemon(t *testing.T, provider agent.LLMProvider, withBrowserProfile bool) *Daemon {
	return createIntegrationDaemonWithLogFile(t, provider, withBrowserProfile, "")
}

func createIntegrationDaemonWithLogFile(t *testing.T, provider agent.LLMProvider, withBrowserProfile bool, logFile string) *Daemon {
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

	tmpDir := t.TempDir()
	workspacePath := filepath.Join(tmpDir, "workspace")
	require.NoError(t, os.MkdirAll(workspacePath, 0755))

	cfg := config.DefaultConfig()
	cfg.DataDir = tmpDir
	cfg.WorkspacePath = workspacePath
	cfg.Channels.Telegram.Enabled = false
	cfg.Webhook.Enabled = false
	cfg.Gateway.Port = reservePort(t)
	cfg.Gateway.SharedSecret = "integration-secret"
	cfg.Gateway.TickInterval = 200
	cfg.AI.Profiles = []config.AIProfile{{
		ID:       "integration-profile",
		Provider: "anthropic",
		APIKey:   "sk-ant-integration",
		Priority: 1,
	}}

	if withBrowserProfile {
		writeBrowserConfig(t, tmpDir)
	}

	log, err := logger.New(logger.Config{Level: "debug", Console: false, File: logFile})
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = log.Close()
	})

	d, err := New(cfg, log)
	if err != nil {
		if strings.Contains(err.Error(), "no such module: fts5") {
			t.Skip("SQLite FTS5 not available in test environment")
		}
		require.NoError(t, err)
	}

	require.NoError(t, d.Start())
	t.Cleanup(func() {
		_ = d.Stop()
	})

	return d
}

func rpcCall(t *testing.T, d *Daemon, traceID string, method string, params map[string]interface{}) gateway.RPCResponse {
	t.Helper()

	body, err := json.Marshal(gateway.RPCRequest{
		ID:      "req-1",
		Method:  method,
		Params:  params,
		JSONRPC: "2.0",
	})
	require.NoError(t, err)

	url := fmt.Sprintf("http://127.0.0.1:%d/rpc", d.config.Gateway.Port)
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Ranya-Secret", d.config.Gateway.SharedSecret)
	if traceID != "" {
		req.Header.Set("X-Trace-Id", traceID)
	}

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	var rpcResp gateway.RPCResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&rpcResp))
	return rpcResp
}

func connectAuthenticatedGatewaySocket(t *testing.T, d *Daemon) *websocket.Conn {
	t.Helper()

	wsURL := fmt.Sprintf("ws://127.0.0.1:%d/ws", d.config.Gateway.Port)
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	require.NoError(t, err)

	var challenge gateway.AuthChallenge
	require.NoError(t, conn.SetReadDeadline(time.Now().Add(2*time.Second)))
	require.NoError(t, conn.ReadJSON(&challenge))
	require.Equal(t, "auth.challenge", challenge.Event)
	require.NotEmpty(t, challenge.Challenge)

	signature := signGatewayChallenge(d.config.Gateway.SharedSecret, challenge.Challenge)
	require.NoError(t, conn.WriteJSON(gateway.AuthResponse{
		Method:    "auth.response",
		Signature: signature,
	}))

	var authResult gateway.AuthResult
	require.NoError(t, conn.SetReadDeadline(time.Now().Add(2*time.Second)))
	require.NoError(t, conn.ReadJSON(&authResult))
	require.True(t, authResult.Success, "gateway websocket authentication failed")

	return conn
}

func signGatewayChallenge(secret string, challenge string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(challenge))
	return hex.EncodeToString(mac.Sum(nil))
}

func waitForCondition(t *testing.T, timeout time.Duration, description string, fn func() bool) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if fn() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}

	t.Fatalf("timed out waiting for condition: %s", description)
}

func readStructuredLogs(t *testing.T, logPath string) []map[string]interface{} {
	t.Helper()

	raw, err := os.ReadFile(logPath)
	require.NoError(t, err)

	lines := strings.Split(string(raw), "\n")
	events := make([]map[string]interface{}, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		entry := map[string]interface{}{}
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue
		}
		events = append(events, entry)
	}

	return events
}

func findEventIndex(events []map[string]interface{}, eventName string, toolName string) int {
	for idx, event := range events {
		value, _ := event["event"].(string)
		if value != eventName {
			continue
		}
		if toolName == "" {
			return idx
		}
		tool, _ := event["tool"].(string)
		if tool == toolName {
			return idx
		}
	}

	return -1
}

func TestIntegrationCLIMessageFlow(t *testing.T) {
	provider := &scriptedProvider{
		responses: []*agent.LLMResponse{
			{ToolCalls: []agent.ToolCall{{ID: "tool-1", Name: "integration_trace", Parameters: map[string]interface{}{}}}},
			{Content: "integration-step-1"},
			{Content: "integration-ok"},
		},
	}
	d := createIntegrationDaemon(t, provider, false)

	var toolTraceID string
	var toolSessionKey string
	require.NoError(t, d.toolExecutor.RegisterTool(toolexecutor.ToolDefinition{
		Name:        "integration_trace",
		Description: "Capture trace/session context",
		Parameters:  []toolexecutor.ToolParameter{},
		Handler: func(ctx context.Context, _ map[string]interface{}) (interface{}, error) {
			toolTraceID = tracing.GetTraceID(ctx)
			toolSessionKey = tracing.GetSessionKey(ctx)
			return map[string]interface{}{"trace_id": toolTraceID, "session_key": toolSessionKey}, nil
		},
	}))

	var eventMu sync.Mutex
	var events []commandqueue.Event
	d.queue.On("enqueued", func(evt commandqueue.Event) {
		eventMu.Lock()
		defer eventMu.Unlock()
		events = append(events, evt)
	})
	d.queue.On("completed", func(evt commandqueue.Event) {
		eventMu.Lock()
		defer eventMu.Unlock()
		events = append(events, evt)
	})

	sessionKey := "cli:integration"
	traceID := tracing.NewTraceID()
	rpcResp := rpcCall(t, d, traceID, "agent.wait", map[string]interface{}{
		"prompt":     "run integration flow",
		"sessionKey": sessionKey,
		"config": map[string]interface{}{
			"model": "integration-model",
			"tools": []interface{}{"integration_trace"},
		},
	})

	require.Nil(t, rpcResp.Error)
	resultMap, ok := rpcResp.Result.(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "integration-ok", resultMap["response"])

	assert.NotEmpty(t, toolTraceID)
	assert.Equal(t, sessionKey, toolSessionKey)

	waitForCondition(t, 2*time.Second, "queue events", func() bool {
		eventMu.Lock()
		defer eventMu.Unlock()
		return len(events) >= 2
	})

	entries, err := d.sessionMgr.LoadSession(sessionKey)
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(entries), 2)
	assert.Equal(t, "user", entries[0].Message.Role)
	assert.Equal(t, "assistant", entries[len(entries)-1].Message.Role)
}

func TestIntegrationPlannerMandatoryBeforeTools(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "planner-mandatory.log")
	provider := &scriptedProvider{
		responses: []*agent.LLMResponse{
			{ToolCalls: []agent.ToolCall{{ID: "tool-1", Name: "planner_probe_tool", Parameters: map[string]interface{}{}}}},
			{Content: "tool-complete"},
		},
	}
	d := createIntegrationDaemonWithLogFile(t, provider, false, logPath)

	require.NoError(t, d.toolExecutor.RegisterTool(toolexecutor.ToolDefinition{
		Name:        "planner_probe_tool",
		Description: "Test planner ordering",
		Category:    toolexecutor.CategoryRead,
		Parameters:  []toolexecutor.ToolParameter{},
		Handler: func(ctx context.Context, _ map[string]interface{}) (interface{}, error) {
			return map[string]interface{}{"ok": true}, nil
		},
	}))

	rpcResp := rpcCall(t, d, tracing.NewTraceID(), "agent.wait", map[string]interface{}{
		"prompt":     "read a file and summarize it",
		"sessionKey": "planner:mandatory",
		"config": map[string]interface{}{
			"model": "integration-model",
			"tools": []interface{}{"planner_probe_tool"},
		},
	})
	require.Nil(t, rpcResp.Error)

	waitForCondition(t, 3*time.Second, "planner and tool events in log", func() bool {
		events := readStructuredLogs(t, logPath)
		return findEventIndex(events, "planner:plan_created", "") >= 0 &&
			findEventIndex(events, "tool:execute", "planner_probe_tool") >= 0
	})

	events := readStructuredLogs(t, logPath)
	planIdx := findEventIndex(events, "planner:plan_created", "")
	toolIdx := findEventIndex(events, "tool:execute", "planner_probe_tool")
	require.NotEqual(t, -1, planIdx, "expected planner:plan_created event")
	require.NotEqual(t, -1, toolIdx, "expected tool:execute event")
	assert.Less(t, planIdx, toolIdx, "planner must run before any tool execution")
}

func TestIntegrationPolicyDenyBlocksShellWithoutSandbox(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "policy-deny.log")
	provider := &scriptedProvider{
		responses: []*agent.LLMResponse{
			{ToolCalls: []agent.ToolCall{{ID: "tool-1", Name: "policy_blocked_shell", Parameters: map[string]interface{}{}}}},
			{Content: "done"},
		},
	}
	d := createIntegrationDaemonWithLogFile(t, provider, false, logPath)

	var commandExecuted atomic.Bool
	require.NoError(t, d.toolExecutor.RegisterTool(toolexecutor.ToolDefinition{
		Name:        "policy_blocked_shell",
		Description: "Shell tool blocked by policy",
		Category:    toolexecutor.CategoryShell,
		Parameters:  []toolexecutor.ToolParameter{},
		Handler: func(ctx context.Context, _ map[string]interface{}) (interface{}, error) {
			commandExecuted.Store(true)
			return "should-not-run", nil
		},
	}))

	d.config.Agents[0].Tools.Allow = []string{"*"}
	d.config.Agents[0].Tools.Deny = []string{"policy_blocked_shell"}

	rpcResp := rpcCall(t, d, tracing.NewTraceID(), "agent.wait", map[string]interface{}{
		"prompt":     "run the shell command",
		"sessionKey": "policy:deny",
		"config": map[string]interface{}{
			"model": "integration-model",
			"tools": []interface{}{"policy_blocked_shell"},
		},
	})
	require.Nil(t, rpcResp.Error)
	assert.False(t, commandExecuted.Load(), "shell tool handler must not execute when denied")

	waitForCondition(t, 3*time.Second, "policy denial event in log", func() bool {
		events := readStructuredLogs(t, logPath)
		return findEventIndex(events, "tool:denied", "policy_blocked_shell") >= 0
	})

	events := readStructuredLogs(t, logPath)
	denyIdx := findEventIndex(events, "tool:denied", "policy_blocked_shell")
	sandboxIdx := findEventIndex(events, "sandbox:execute", "policy_blocked_shell")
	require.NotEqual(t, -1, denyIdx, "expected tool:denied audit event")
	assert.Equal(t, -1, sandboxIdx, "sandbox must not execute for denied tools")
}

func TestIntegrationSandboxMandatoryForRiskyTools(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "sandbox-mandatory.log")
	provider := &scriptedProvider{
		responses: []*agent.LLMResponse{
			{ToolCalls: []agent.ToolCall{{ID: "tool-1", Name: "sandbox_risky_shell", Parameters: map[string]interface{}{}}}},
			{Content: "done"},
		},
	}
	d := createIntegrationDaemonWithLogFile(t, provider, false, logPath)

	var riskyExecuted atomic.Bool
	require.NoError(t, d.toolExecutor.RegisterTool(toolexecutor.ToolDefinition{
		Name:        "sandbox_risky_shell",
		Description: "Risky shell tool",
		Category:    toolexecutor.CategoryShell,
		Parameters:  []toolexecutor.ToolParameter{},
		Handler: func(ctx context.Context, _ map[string]interface{}) (interface{}, error) {
			riskyExecuted.Store(true)
			return "executed", nil
		},
	}))

	d.config.Agents[0].Tools.Allow = []string{"*"}
	d.config.Agents[0].Tools.Deny = []string{}

	rpcResp := rpcCall(t, d, tracing.NewTraceID(), "agent.wait", map[string]interface{}{
		"prompt":     "execute risky shell task",
		"sessionKey": "sandbox:mandatory",
		"config": map[string]interface{}{
			"model": "integration-model",
			"tools": []interface{}{"sandbox_risky_shell"},
		},
	})
	require.Nil(t, rpcResp.Error)
	assert.True(t, riskyExecuted.Load(), "risky tool should execute when policy allows")

	waitForCondition(t, 3*time.Second, "sandbox execution event in log", func() bool {
		events := readStructuredLogs(t, logPath)
		return findEventIndex(events, "sandbox:execute", "sandbox_risky_shell") >= 0
	})

	events := readStructuredLogs(t, logPath)
	sandboxIdx := findEventIndex(events, "sandbox:execute", "sandbox_risky_shell")
	require.NotEqual(t, -1, sandboxIdx, "expected sandbox:execute event for risky tools")

	sandboxEvent := events[sandboxIdx]
	traceID, traceOK := sandboxEvent["trace_id"].(string)
	runID, runOK := sandboxEvent["run_id"].(string)
	assert.True(t, traceOK && traceID != "", "sandbox event must include trace_id")
	assert.True(t, runOK && runID != "", "sandbox event must include run_id")
}

func TestIntegrationGatewayTypedStreamingEvents(t *testing.T) {
	provider := &scriptedProvider{
		responses: []*agent.LLMResponse{
			{ToolCalls: []agent.ToolCall{{ID: "tool-1", Name: "typed_stream_tool", Parameters: map[string]interface{}{}}}},
			{Content: "typed-stream-complete"},
		},
	}
	d := createIntegrationDaemon(t, provider, false)

	require.NoError(t, d.toolExecutor.RegisterTool(toolexecutor.ToolDefinition{
		Name:        "typed_stream_tool",
		Description: "Tool for typed stream event tests",
		Category:    toolexecutor.CategoryRead,
		Parameters:  []toolexecutor.ToolParameter{},
		Handler: func(context.Context, map[string]interface{}) (interface{}, error) {
			return map[string]interface{}{"ok": true}, nil
		},
	}))

	wsConn := connectAuthenticatedGatewaySocket(t, d)
	defer wsConn.Close()

	traceID := tracing.NewTraceID()
	rpcResp := rpcCall(t, d, traceID, "agent.wait", map[string]interface{}{
		"prompt":     "use a tool and finish",
		"sessionKey": "typed:streaming",
		"config": map[string]interface{}{
			"model": "integration-model",
			"tools": []interface{}{"typed_stream_tool"},
		},
	})
	require.Nil(t, rpcResp.Error)

	var events []gateway.EventMessage
	deadline := time.Now().Add(4 * time.Second)
	for time.Now().Before(deadline) {
		var evt gateway.EventMessage
		_ = wsConn.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
		err := wsConn.ReadJSON(&evt)
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				if hasTypedEvent(events, gateway.StreamTypeLifecycle, "start") &&
					hasTypedEvent(events, gateway.StreamTypeTool, "start") &&
					hasTypedEvent(events, gateway.StreamTypeTool, "end") {
					break
				}
				continue
			}
			require.NoError(t, err)
		}

		if evt.Type == "event" && evt.Stream != "" {
			events = append(events, evt)
			if hasTypedEvent(events, gateway.StreamTypeLifecycle, "start") &&
				hasTypedEvent(events, gateway.StreamTypeTool, "start") &&
				hasTypedEvent(events, gateway.StreamTypeTool, "end") {
				break
			}
		}
	}

	require.NotEmpty(t, events, "expected typed gateway events")
	assert.True(t, hasTypedEvent(events, gateway.StreamTypeLifecycle, "start"), "missing lifecycle:start event")
	assert.True(t, hasTypedEvent(events, gateway.StreamTypeTool, "start"), "missing tool:start event")
	assert.True(t, hasTypedEvent(events, gateway.StreamTypeTool, "end"), "missing tool:end event")

	var prevSeq int64
	for _, evt := range events {
		assert.Equal(t, "event", evt.Type)
		assert.NotEmpty(t, evt.TraceID)
		assert.NotEmpty(t, evt.RunID)
		assert.NotZero(t, evt.Seq)
		assert.Greater(t, evt.Seq, prevSeq)
		prevSeq = evt.Seq
	}
}

func TestIntegrationGatewayLifecycleTickEvents(t *testing.T) {
	d := createIntegrationDaemon(t, nil, false)

	wsConn := connectAuthenticatedGatewaySocket(t, d)
	defer wsConn.Close()

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		var evt gateway.EventMessage
		_ = wsConn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
		err := wsConn.ReadJSON(&evt)
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				continue
			}
			require.NoError(t, err)
		}

		if evt.Event == "tick" {
			assert.Equal(t, "event", evt.Type)
			assert.Equal(t, gateway.StreamTypeLifecycle, evt.Stream)
			assert.Equal(t, "tick", evt.Phase)
			assert.NotZero(t, evt.Seq)
			return
		}
	}

	t.Fatalf("timed out waiting for gateway tick event")
}

func TestIntegrationMultiChannelRoutingByChannelBinding(t *testing.T) {
	origNewAgentRunner := newAgentRunner

	var (
		mu         sync.Mutex
		calledWith []string
	)
	provider := &scriptedProvider{
		responses: []*agent.LLMResponse{
			{Content: "gateway-ok"},
			{Content: "telegram-ok"},
		},
		onCall: func(req agent.LLMRequest) {
			mu.Lock()
			calledWith = append(calledWith, req.Model)
			mu.Unlock()
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
	cfg.Channels.Telegram.Enabled = false
	cfg.Webhook.Enabled = false
	cfg.Gateway.Port = reservePort(t)
	cfg.Gateway.SharedSecret = "integration-secret"
	cfg.Gateway.TickInterval = 200
	cfg.AI.Profiles = []config.AIProfile{{
		ID:       "integration-profile",
		Provider: "anthropic",
		APIKey:   "sk-ant-integration",
		Priority: 1,
	}}

	defaultAgent := cfg.Agents[0]
	defaultAgent.Model = "model-gateway"
	defaultAgent.Sandbox.Mode = "tools"
	defaultAgent.Sandbox.Scope = "session"

	telegramAgent := defaultAgent
	telegramAgent.ID = "telegram-agent"
	telegramAgent.Name = "Telegram Agent"
	telegramAgent.Model = "model-telegram"

	cfg.Agents = []config.AgentConfig{defaultAgent, telegramAgent}

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

	require.NoError(t, d.GetRoutingService().AddRoute(&routing.Route{
		ID:       "route-telegram-channel",
		Name:     "route-telegram-channel",
		Handler:  "telegram-agent",
		Priority: 90,
		Enabled:  true,
		Patterns: []routing.RoutePattern{
			{Type: routing.PatternTypeWildcard, Value: "*"},
		},
		Metadata: map[string]interface{}{
			"binding": map[string]interface{}{"channel": "telegram"},
		},
	}))

	require.NoError(t, d.GetRoutingService().AddRoute(&routing.Route{
		ID:       "route-gateway-channel",
		Name:     "route-gateway-channel",
		Handler:  "default",
		Priority: 80,
		Enabled:  true,
		Patterns: []routing.RoutePattern{
			{Type: routing.PatternTypeWildcard, Value: "*"},
		},
		Metadata: map[string]interface{}{
			"binding": map[string]interface{}{"channel": "gateway"},
		},
	}))

	rpcResp := rpcCall(t, d, tracing.NewTraceID(), "agent.wait", map[string]interface{}{
		"prompt":     "gateway route check",
		"sessionKey": "gateway:channel-check",
	})
	require.Nil(t, rpcResp.Error)

	_, err = d.GetChannelRegistry().Dispatch(context.Background(), channels.InboundMessage{
		Channel:    "telegram",
		SessionKey: "telegram:999",
		Content:    "telegram route check",
		Metadata: map[string]interface{}{
			"peer": "999",
		},
	})
	require.NoError(t, err)

	waitForCondition(t, 2*time.Second, "both channel routes dispatched", func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(calledWith) >= 2
	})

	mu.Lock()
	defer mu.Unlock()
	assert.Contains(t, calledWith, "model-gateway")
	assert.Contains(t, calledWith, "model-telegram")
}

func hasTypedEvent(events []gateway.EventMessage, stream gateway.StreamType, phase string) bool {
	for _, evt := range events {
		if evt.Stream == stream && evt.Phase == phase {
			return true
		}
	}
	return false
}

func TestIntegrationCLIMessageOrdering(t *testing.T) {
	provider := &scriptedProvider{
		delay: 250 * time.Millisecond,
		responses: []*agent.LLMResponse{
			{Content: "ok-1"},
			{Content: "ok-2"},
			{Content: "ok-3"},
			{Content: "ok-4"},
		},
	}
	d := createIntegrationDaemon(t, provider, false)

	run := func(sessionKey string, prompt string) {
		_, err := d.agentRunner.RunWithContext(context.Background(), agent.AgentRunParams{
			Prompt:     prompt,
			SessionKey: sessionKey,
			Config:     agent.DefaultConfig(),
		})
		require.NoError(t, err)
	}

	startSame := time.Now()
	var sameWG sync.WaitGroup
	sameWG.Add(2)
	go func() {
		defer sameWG.Done()
		run("session-order", "first")
	}()
	go func() {
		defer sameWG.Done()
		run("session-order", "second")
	}()
	sameWG.Wait()
	sameElapsed := time.Since(startSame)
	assert.GreaterOrEqual(t, sameElapsed, 450*time.Millisecond)

	startDiff := time.Now()
	var diffWG sync.WaitGroup
	diffWG.Add(2)
	go func() {
		defer diffWG.Done()
		run("session-a", "alpha")
	}()
	go func() {
		defer diffWG.Done()
		run("session-b", "beta")
	}()
	diffWG.Wait()
	diffElapsed := time.Since(startDiff)
	assert.Less(t, diffElapsed, 420*time.Millisecond)
}

func TestIntegrationTelegramMessageFlow(t *testing.T) {
	d := createIntegrationDaemon(t, nil, false)

	var gotEnqueued atomic.Bool
	d.queue.On("enqueued", func(evt commandqueue.Event) {
		if evt.Lane == "telegram:12345" {
			gotEnqueued.Store(true)
		}
	})

	err := d.router.RouteMessage(context.Background(), Message{
		SessionKey: "telegram:12345",
		Source:     "telegram",
		Content:    "hello from telegram",
		Metadata:   map[string]interface{}{"chat_id": 12345},
	})
	require.NoError(t, err)

	waitForCondition(t, time.Second, "telegram queue enqueue", func() bool {
		return gotEnqueued.Load()
	})
}

func TestIntegrationTelegramSessionIsolation(t *testing.T) {
	d := createIntegrationDaemon(t, nil, false)

	start := time.Now()
	var wg sync.WaitGroup
	wg.Add(3)

	go func() {
		defer wg.Done()
		_, err := d.queue.Enqueue("telegram:1", func(ctx context.Context) (interface{}, error) {
			select {
			case <-time.After(200 * time.Millisecond):
				return "ok", nil
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}, nil)
		require.NoError(t, err)
	}()

	go func() {
		defer wg.Done()
		_, err := d.queue.Enqueue("telegram:1", func(ctx context.Context) (interface{}, error) {
			select {
			case <-time.After(200 * time.Millisecond):
				return "ok", nil
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}, nil)
		require.NoError(t, err)
	}()

	go func() {
		defer wg.Done()
		_, err := d.queue.Enqueue("telegram:2", func(ctx context.Context) (interface{}, error) {
			select {
			case <-time.After(200 * time.Millisecond):
				return "ok", nil
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}, nil)
		require.NoError(t, err)
	}()

	wg.Wait()
	elapsed := time.Since(start)
	assert.GreaterOrEqual(t, elapsed, 380*time.Millisecond)
	assert.Less(t, elapsed, 620*time.Millisecond)
}

func TestIntegrationCronJobExecution(t *testing.T) {
	provider := &scriptedProvider{responses: []*agent.LLMResponse{{Content: "cron-ok"}}}
	d := createIntegrationDaemon(t, provider, false)

	var laneSeen atomic.Bool

	job, err := d.cronService.AddJob(cron.AddParams{
		Name:    "integration-cron-job",
		Enabled: true,
		Schedule: cron.Schedule{
			Kind:    cron.ScheduleKindEvery,
			EveryMs: int64(time.Hour / time.Millisecond),
		},
		SessionTarget: cron.SessionTargetIsolated,
		WakeMode:      cron.WakeModeNow,
		Payload: cron.Payload{
			Kind:    cron.PayloadKindAgentTurn,
			Message: "cron integration prompt",
		},
	})
	require.NoError(t, err)
	d.queue.On("enqueued", func(evt commandqueue.Event) {
		if evt.Lane == "session-cron:"+job.ID {
			laneSeen.Store(true)
		}
	})

	require.NoError(t, d.cronService.RunJob(job.ID, cron.RunModeForce))

	waitForCondition(t, 3*time.Second, "cron job completion", func() bool {
		j := d.cronService.GetJob(job.ID)
		return j != nil && j.State.LastRunAtMs != nil && j.State.LastStatus != ""
	})

	assert.True(t, laneSeen.Load())
	jobState := d.cronService.GetJob(job.ID)
	require.NotNil(t, jobState)
	assert.Equal(t, "ok", jobState.State.LastStatus)

	entries, err := d.sessionMgr.LoadSession("cron:" + job.ID)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(entries), 2)
}

func TestIntegrationCronSchedulingAndPersistence(t *testing.T) {
	storePath := filepath.Join(t.TempDir(), "cron", "jobs.json")
	var runCount atomic.Int32

	svc, err := cron.NewService(cron.ServiceOptions{
		StorePath:          storePath,
		EnqueueSystemEvent: func(_ string, _ string) {},
		RunIsolatedAgentJob: func(_ *cron.Job, _ string) error {
			runCount.Add(1)
			return nil
		},
		RequestHeartbeatNow: func() {},
		OnEvent:             func(_ cron.Event) {},
	})
	require.NoError(t, err)
	defer func() { _ = svc.Stop() }()

	job, err := svc.AddJob(cron.AddParams{
		Name:    "schedule-test",
		Enabled: true,
		Schedule: cron.Schedule{
			Kind:    cron.ScheduleKindEvery,
			EveryMs: 120,
		},
		SessionTarget: cron.SessionTargetIsolated,
		WakeMode:      cron.WakeModeNow,
		Payload: cron.Payload{
			Kind:    cron.PayloadKindAgentTurn,
			Message: "scheduled",
		},
	})
	require.NoError(t, err)

	waitForCondition(t, 3*time.Second, "cron job executes multiple times", func() bool {
		return runCount.Load() >= 2
	})
	assert.GreaterOrEqual(t, runCount.Load(), int32(2))

	require.NoError(t, svc.Stop())

	svc2, err := cron.NewService(cron.ServiceOptions{
		StorePath:           storePath,
		EnqueueSystemEvent:  func(_ string, _ string) {},
		RunIsolatedAgentJob: func(_ *cron.Job, _ string) error { return nil },
		RequestHeartbeatNow: func() {},
		OnEvent:             func(_ cron.Event) {},
	})
	require.NoError(t, err)
	defer func() { _ = svc2.Stop() }()

	loaded := svc2.GetJob(job.ID)
	require.NotNil(t, loaded)
	assert.Equal(t, job.ID, loaded.ID)
}

func TestIntegrationMemoryToolExecution(t *testing.T) {
	d := createIntegrationDaemon(t, nil, false)

	ctx := tracing.NewRequestContext(context.Background())
	ctx = tracing.WithSessionKey(ctx, "memory:integration")
	execCtx := &toolexecutor.ExecutionContext{SessionKey: "memory:integration", Timeout: 5 * time.Second}

	write := d.toolExecutor.Execute(ctx, "memory_write", map[string]interface{}{
		"path":    "integration/memory.md",
		"content": "integration memory token",
	}, execCtx)
	require.True(t, write.Success, "memory_write failed: %s", write.Error)

	search := d.toolExecutor.Execute(ctx, "memory_search", map[string]interface{}{
		"query": "integration memory token",
		"limit": 5,
	}, execCtx)
	require.True(t, search.Success, "memory_search failed: %s", search.Error)

	raw, err := json.Marshal(search.Output)
	require.NoError(t, err)
	assert.Contains(t, string(raw), "integration memory token")
}

func TestIntegrationBrowserToolExecution(t *testing.T) {
	d := createIntegrationDaemon(t, nil, true)

	ctx := tracing.NewRequestContext(context.Background())
	ctx = tracing.WithSessionKey(ctx, "browser:integration")
	execCtx := &toolexecutor.ExecutionContext{SessionKey: "browser:integration", Timeout: 30 * time.Second}

	navigate := d.toolExecutor.Execute(ctx, "browser_navigate", map[string]interface{}{
		"profile": "default",
		"url":     "https://example.com",
	}, execCtx)
	if !navigate.Success {
		if strings.Contains(strings.ToLower(navigate.Error), "failed to launch chrome") ||
			strings.Contains(strings.ToLower(navigate.Error), "cdp") ||
			strings.Contains(strings.ToLower(navigate.Error), "browser") {
			t.Skipf("chrome not available in environment: %s", navigate.Error)
		}
		require.True(t, navigate.Success, "browser_navigate failed: %s", navigate.Error)
	}

	navMap, ok := navigate.Output.(map[string]interface{})
	require.True(t, ok)
	targetID, ok := navMap["targetId"].(string)
	require.True(t, ok)
	require.NotEmpty(t, targetID)

	extract := d.toolExecutor.Execute(ctx, "browser_extract", map[string]interface{}{
		"profile":  "default",
		"targetId": targetID,
		"type":     "text",
	}, execCtx)
	require.True(t, extract.Success, "browser_extract failed: %s", extract.Error)

	screenshot := d.toolExecutor.Execute(ctx, "browser_screenshot", map[string]interface{}{
		"profile":  "default",
		"targetId": targetID,
		"type":     "png",
	}, execCtx)
	require.True(t, screenshot.Success, "browser_screenshot failed: %s", screenshot.Error)

	raw, err := json.Marshal(extract.Output)
	require.NoError(t, err)
	assert.Contains(t, strings.ToLower(string(raw)), "example")
}

type fakePluginRuntime struct {
	plugins map[string]*fakePlugin
}

func (r *fakePluginRuntime) GetPlugin(pluginID string) (interface{}, error) {
	p, ok := r.plugins[pluginID]
	if !ok {
		return nil, fmt.Errorf("plugin %s not found", pluginID)
	}
	return p, nil
}

type fakePlugin struct {
	id          string
	permissions []string
	fail        bool
}

func (p *fakePlugin) ExecuteTool(_ context.Context, name string, params map[string]interface{}) (map[string]interface{}, error) {
	if p.fail {
		return nil, fmt.Errorf("plugin tool %s failed", name)
	}
	return map[string]interface{}{"name": name, "echo": params["input"]}, nil
}

func (p *fakePlugin) GetID() string {
	return p.id
}

func (p *fakePlugin) GetPermissions() []string {
	return p.permissions
}

func TestIntegrationPluginToolExecution(t *testing.T) {
	d := createIntegrationDaemon(t, nil, false)

	runtime := &fakePluginRuntime{plugins: map[string]*fakePlugin{
		"good": {id: "good", permissions: []string{"filesystem:read"}},
		"bad":  {id: "bad", permissions: []string{"filesystem:read"}, fail: true},
	}}

	require.NoError(t, d.toolExecutor.RegisterPluginTool("good", toolexecutor.ToolDefinition{
		Name:        "plugin_echo",
		Description: "Echo input",
		Parameters: []toolexecutor.ToolParameter{{
			Name:        "input",
			Type:        "string",
			Description: "Input",
			Required:    true,
		}},
	}, runtime))

	require.NoError(t, d.toolExecutor.RegisterPluginTool("bad", toolexecutor.ToolDefinition{
		Name:        "plugin_fail",
		Description: "Failing tool",
		Parameters:  []toolexecutor.ToolParameter{},
	}, runtime))

	ctx := tracing.NewRequestContext(context.Background())
	execCtx := &toolexecutor.ExecutionContext{SessionKey: "plugin:integration", Timeout: 5 * time.Second}

	okResult := d.toolExecutor.Execute(ctx, "plugin_echo", map[string]interface{}{"input": "hello"}, execCtx)
	require.True(t, okResult.Success, "plugin_echo failed: %s", okResult.Error)

	failResult := d.toolExecutor.Execute(ctx, "plugin_fail", map[string]interface{}{}, execCtx)
	require.False(t, failResult.Success)
	assert.Contains(t, failResult.Error, "failed")
	assert.True(t, d.Status().Running)
}

func TestIntegrationTimeoutPropagation(t *testing.T) {
	provider := &scriptedProvider{
		responses: []*agent.LLMResponse{
			{ToolCalls: []agent.ToolCall{{ID: "slow-1", Name: "slow_tool", Parameters: map[string]interface{}{}}}},
			{Content: "done"},
		},
	}
	d := createIntegrationDaemon(t, provider, false)

	require.NoError(t, d.toolExecutor.RegisterTool(toolexecutor.ToolDefinition{
		Name:        "slow_tool",
		Description: "Sleeps until cancelled",
		Parameters:  []toolexecutor.ToolParameter{},
		Handler: func(ctx context.Context, _ map[string]interface{}) (interface{}, error) {
			select {
			case <-time.After(2 * time.Second):
				return "slow-finished", nil
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		},
	}))

	runCtx, cancel := context.WithTimeout(context.Background(), 120*time.Millisecond)
	defer cancel()

	start := time.Now()
	result, err := d.agentRunner.RunWithContext(runCtx, agent.AgentRunParams{
		Prompt:     "trigger timeout",
		SessionKey: "timeout:integration",
		Config: agent.AgentConfig{
			Model:      "integration-model",
			MaxRetries: 1,
			Tools:      []string{"slow_tool"},
		},
	})
	require.NoError(t, err)
	assert.True(t, result.Aborted)
	assert.Less(t, time.Since(start), time.Second)
}

func TestIntegrationCancellationPropagation(t *testing.T) {
	provider := &scriptedProvider{
		responses: []*agent.LLMResponse{
			{ToolCalls: []agent.ToolCall{{ID: "cancel-1", Name: "cancel_tool", Parameters: map[string]interface{}{}}}},
			{Content: "done"},
		},
	}
	d := createIntegrationDaemon(t, provider, false)

	toolCancelled := make(chan struct{}, 1)
	require.NoError(t, d.toolExecutor.RegisterTool(toolexecutor.ToolDefinition{
		Name:        "cancel_tool",
		Description: "Waits for cancellation",
		Parameters:  []toolexecutor.ToolParameter{},
		Handler: func(ctx context.Context, _ map[string]interface{}) (interface{}, error) {
			<-ctx.Done()
			select {
			case toolCancelled <- struct{}{}:
			default:
			}
			return nil, ctx.Err()
		},
	}))

	runCtx, cancel := context.WithCancel(context.Background())
	resultCh := make(chan agent.AgentResult, 1)
	errCh := make(chan error, 1)

	go func() {
		result, err := d.agentRunner.RunWithContext(runCtx, agent.AgentRunParams{
			Prompt:     "trigger cancellation",
			SessionKey: "cancel:integration",
			Config: agent.AgentConfig{
				Model:      "integration-model",
				MaxRetries: 1,
				Tools:      []string{"cancel_tool"},
			},
		})
		if err != nil {
			errCh <- err
			return
		}
		resultCh <- result
	}()

	time.Sleep(100 * time.Millisecond)
	cancel()

	select {
	case err := <-errCh:
		require.NoError(t, err)
	case result := <-resultCh:
		assert.True(t, result.Aborted)
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for cancellation result")
	}

	select {
	case <-toolCancelled:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("expected cancel_tool to observe context cancellation")
	}
}

func TestIntegrationModuleBypassDetection(t *testing.T) {
	routerSrc, err := os.ReadFile("router.go")
	require.NoError(t, err)
	assert.Contains(t, string(routerSrc), "EnqueueWithContext")

	runnerSrc, err := os.ReadFile(filepath.Join("..", "..", "pkg", "agent", "runner.go"))
	require.NoError(t, err)
	assert.Contains(t, string(runnerSrc), "toolExecutor.Execute")
	assert.Contains(t, string(runnerSrc), "loadSessionHistory")
	assert.NotContains(t, string(runnerSrc), "database/sql")
}

func TestIntegrationModuleInvariants(t *testing.T) {
	d := createIntegrationDaemon(t, nil, false)

	require.NoError(t, d.sessionMgr.AppendMessage("inv:one", sessionMessage("user", "hello one")))
	require.NoError(t, d.sessionMgr.AppendMessage("inv:two", sessionMessage("user", "hello two")))
	sessions, err := d.sessionMgr.ListSessions()
	require.NoError(t, err)
	assert.Contains(t, sessions, "inv:one")
	assert.Contains(t, sessions, "inv:two")

	var orderMu sync.Mutex
	var order []int
	for i := 1; i <= 3; i++ {
		i := i
		go func() {
			_, err := d.queue.Enqueue("fifo-lane", func(_ context.Context) (interface{}, error) {
				orderMu.Lock()
				defer orderMu.Unlock()
				order = append(order, i)
				return nil, nil
			}, nil)
			require.NoError(t, err)
		}()
		time.Sleep(15 * time.Millisecond)
	}

	waitForCondition(t, time.Second, "fifo order", func() bool {
		orderMu.Lock()
		defer orderMu.Unlock()
		return len(order) == 3
	})

	orderMu.Lock()
	assert.Equal(t, []int{1, 2, 3}, order)
	orderMu.Unlock()

	resolver := plugin.NewDependencyResolver(zerolog.Nop())
	graph := &plugin.DependencyGraph{
		Nodes: map[string]*plugin.PluginManifest{
			"plugin-a": {ID: "plugin-a", Version: "1.0.0", Dependencies: []plugin.PluginDependency{{PluginID: "plugin-b"}}},
			"plugin-b": {ID: "plugin-b", Version: "1.0.0", Dependencies: []plugin.PluginDependency{}},
		},
		Edges: map[string][]string{
			"plugin-a": {"plugin-b"},
			"plugin-b": {},
		},
	}
	orderPlugins, err := resolver.TopologicalSort(graph)
	require.NoError(t, err)

	indexA := -1
	indexB := -1
	for i, id := range orderPlugins {
		if id == "plugin-a" {
			indexA = i
		}
		if id == "plugin-b" {
			indexB = i
		}
	}
	require.NotEqual(t, -1, indexA)
	require.NotEqual(t, -1, indexB)
	assert.Less(t, indexB, indexA)
}

func sessionMessage(role, content string) session.Message {
	return session.Message{Role: role, Content: content}
}
