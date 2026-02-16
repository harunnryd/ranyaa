package agent

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/harun/ranya/pkg/commandqueue"
	"github.com/harun/ranya/pkg/session"
	"github.com/harun/ranya/pkg/toolexecutor"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupTestRunner(t *testing.T) (*Runner, string, func()) {
	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "agent-test-*")
	require.NoError(t, err)

	// Create session manager
	sm, err := session.New(tmpDir)
	require.NoError(t, err)

	// Create tool executor
	te := toolexecutor.New()

	// Register test tool
	err = te.RegisterTool(toolexecutor.ToolDefinition{
		Name:        "test_tool",
		Description: "A test tool",
		Parameters: []toolexecutor.ToolParameter{
			{
				Name:        "input",
				Type:        "string",
				Description: "Test input",
				Required:    true,
			},
		},
		Handler: func(ctx context.Context, params map[string]interface{}) (interface{}, error) {
			return "test result", nil
		},
	})
	require.NoError(t, err)

	// Create command queue
	cq := commandqueue.New()

	// Create logger
	logger := zerolog.New(os.Stdout).Level(zerolog.ErrorLevel)

	// Create runner with mock auth profile
	runner, err := NewRunner(Config{
		SessionManager: sm,
		ToolExecutor:   te,
		CommandQueue:   cq,
		Logger:         logger,
		AuthProfiles: []AuthProfile{
			{
				ID:       "test",
				Provider: "anthropic",
				APIKey:   "test-key",
				Priority: 1,
			},
		},
	})
	require.NoError(t, err)

	cleanup := func() {
		sm.Close()
		cq.Close()
		os.RemoveAll(tmpDir)
	}

	return runner, tmpDir, cleanup
}

func TestNewRunner(t *testing.T) {
	t.Run("should create runner with valid config", func(t *testing.T) {
		runner, _, cleanup := setupTestRunner(t)
		defer cleanup()

		assert.NotNil(t, runner)
		assert.NotNil(t, runner.sessionManager)
		assert.NotNil(t, runner.toolExecutor)
		assert.NotNil(t, runner.commandQueue)
	})

	t.Run("should fail without session manager", func(t *testing.T) {
		_, err := NewRunner(Config{
			ToolExecutor: toolexecutor.New(),
			CommandQueue: commandqueue.New(),
			Logger:       zerolog.New(os.Stdout),
			AuthProfiles: []AuthProfile{{ID: "test", Provider: "anthropic", APIKey: "key", Priority: 1}},
		})

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "session manager")
	})

	t.Run("should fail without auth profiles", func(t *testing.T) {
		tmpDir, _ := os.MkdirTemp("", "test-*")
		defer os.RemoveAll(tmpDir)

		sm, _ := session.New(tmpDir)

		_, err := NewRunner(Config{
			SessionManager: sm,
			ToolExecutor:   toolexecutor.New(),
			CommandQueue:   commandqueue.New(),
			Logger:         zerolog.New(os.Stdout),
			AuthProfiles:   []AuthProfile{},
		})

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "auth profile")
	})
}

func TestValidateConfig(t *testing.T) {
	runner, _, cleanup := setupTestRunner(t)
	defer cleanup()

	t.Run("should accept valid config", func(t *testing.T) {
		config := AgentConfig{
			Model:       "claude-3-5-sonnet-20241022",
			Temperature: 0.7,
			MaxTokens:   4096,
			MaxRetries:  3,
		}

		err := runner.validateConfig(config)
		assert.NoError(t, err)
	})

	t.Run("should reject empty model", func(t *testing.T) {
		config := AgentConfig{
			Model: "",
		}

		err := runner.validateConfig(config)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "model")
	})

	t.Run("should reject invalid temperature", func(t *testing.T) {
		config := AgentConfig{
			Model:       "claude-3-5-sonnet-20241022",
			Temperature: 1.5,
		}

		err := runner.validateConfig(config)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "temperature")
	})

	t.Run("should reject negative max tokens", func(t *testing.T) {
		config := AgentConfig{
			Model:     "claude-3-5-sonnet-20241022",
			MaxTokens: -1,
		}

		err := runner.validateConfig(config)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "max tokens")
	})
}

func TestLoadSessionHistory(t *testing.T) {
	runner, tmpDir, cleanup := setupTestRunner(t)
	defer cleanup()

	t.Run("should load empty session", func(t *testing.T) {
		sessionKey := "test-session"
		err := runner.sessionManager.CreateSession(sessionKey)
		require.NoError(t, err)

		history, err := runner.loadSessionHistory(sessionKey)
		assert.NoError(t, err)
		assert.Empty(t, history)
	})

	t.Run("should load session with messages", func(t *testing.T) {
		sessionKey := "test-session-2"
		err := runner.sessionManager.CreateSession(sessionKey)
		require.NoError(t, err)

		err = runner.sessionManager.AppendMessage(sessionKey, session.Message{
			Role:    "user",
			Content: "Hello",
		})
		require.NoError(t, err)

		history, err := runner.loadSessionHistory(sessionKey)
		assert.NoError(t, err)
		assert.Len(t, history, 1)
		assert.Equal(t, "user", history[0].Message.Role)
		assert.Equal(t, "Hello", history[0].Message.Content)
	})

	t.Run("should validate message fields", func(t *testing.T) {
		sessionKey := "test-session-3"

		// Manually create invalid session file
		sessionPath := filepath.Join(tmpDir, sessionKey+".jsonl")
		invalidEntry := `{"sessionKey":"test-session-3","message":{"role":"","content":"test"}}`
		err := os.WriteFile(sessionPath, []byte(invalidEntry+"\n"), 0600)
		require.NoError(t, err)

		// Session manager should skip invalid entries gracefully
		history, err := runner.loadSessionHistory(sessionKey)
		assert.NoError(t, err)
		assert.Empty(t, history) // Invalid entry should be skipped
	})
}

func TestBuildMessages(t *testing.T) {
	runner, _, cleanup := setupTestRunner(t)
	defer cleanup()

	t.Run("should build messages with system prompt", func(t *testing.T) {
		params := AgentRunParams{
			Prompt:     "Test prompt",
			SessionKey: "test",
			Config: AgentConfig{
				Model:        "claude-3-5-sonnet-20241022",
				SystemPrompt: "You are a test assistant",
			},
		}

		messages, err := runner.buildMessages(context.Background(), []session.SessionEntry{}, params)
		assert.NoError(t, err)
		assert.Len(t, messages, 2) // system + user
		assert.Equal(t, "system", messages[0].Role)
		assert.Equal(t, "You are a test assistant", messages[0].Content)
		assert.Equal(t, "user", messages[1].Role)
		assert.Equal(t, "Test prompt", messages[1].Content)
	})

	t.Run("should use default system prompt", func(t *testing.T) {
		params := AgentRunParams{
			Prompt:     "Test prompt",
			SessionKey: "test",
			Config: AgentConfig{
				Model: "claude-3-5-sonnet-20241022",
			},
		}

		messages, err := runner.buildMessages(context.Background(), []session.SessionEntry{}, params)
		assert.NoError(t, err)
		assert.Contains(t, messages[0].Content, "helpful assistant")
	})

	t.Run("should include conversation history", func(t *testing.T) {
		history := []session.SessionEntry{
			{
				SessionKey: "test",
				Message: session.Message{
					Role:    "user",
					Content: "Previous message",
				},
			},
		}

		params := AgentRunParams{
			Prompt:     "New message",
			SessionKey: "test",
			Config: AgentConfig{
				Model: "claude-3-5-sonnet-20241022",
			},
		}

		messages, err := runner.buildMessages(context.Background(), history, params)
		assert.NoError(t, err)
		assert.Len(t, messages, 3) // system + previous + current
		assert.Equal(t, "Previous message", messages[1].Content)
		assert.Equal(t, "New message", messages[2].Content)
	})
}

func TestCompactIfNeeded(t *testing.T) {
	runner, _, cleanup := setupTestRunner(t)
	defer cleanup()

	t.Run("should not compact if under limit", func(t *testing.T) {
		messages := []AgentMessage{
			{Role: "system", Content: "System"},
			{Role: "user", Content: "Hello"},
		}

		result := runner.compactIfNeeded(messages, 1000)
		assert.Len(t, result, 2)
	})

	t.Run("should compact if over limit", func(t *testing.T) {
		messages := []AgentMessage{
			{Role: "system", Content: "System"},
		}

		// Add many messages to exceed limit
		for i := 0; i < 30; i++ {
			messages = append(messages, AgentMessage{
				Role:    "user",
				Content: "Message with some content to increase token count",
			})
		}

		result := runner.compactIfNeeded(messages, 100)

		// Should have system + summary + recent messages
		assert.Less(t, len(result), len(messages))

		// Should preserve system message
		assert.Equal(t, "system", result[0].Role)

		// Should have summary
		foundSummary := false
		for _, msg := range result {
			if msg.Role == "system" && len(msg.Content) > len("System") {
				foundSummary = true
				break
			}
		}
		assert.True(t, foundSummary)
	})
}

func TestBuildTools(t *testing.T) {
	runner, _, cleanup := setupTestRunner(t)
	defer cleanup()

	t.Run("should return nil for empty tool list", func(t *testing.T) {
		tools, err := runner.buildTools([]string{})
		assert.NoError(t, err)
		assert.Nil(t, tools)
	})

	t.Run("should build tools from registered tools", func(t *testing.T) {
		tools, err := runner.buildTools([]string{"test_tool"})
		assert.NoError(t, err)
		assert.Len(t, tools, 1)
	})

	t.Run("should fail for unknown tool", func(t *testing.T) {
		_, err := runner.buildTools([]string{"unknown_tool"})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "tool not found")
	})
}

func TestAbort(t *testing.T) {
	runner, _, cleanup := setupTestRunner(t)
	defer cleanup()

	t.Run("should handle abort on non-existent session", func(t *testing.T) {
		err := runner.Abort("non-existent")
		assert.NoError(t, err)
	})

	t.Run("should abort running execution", func(t *testing.T) {
		sessionKey := "test-abort"

		// Register a mock cancel function
		called := false
		runner.runsMu.Lock()
		runner.activeRuns[sessionKey] = func() {
			called = true
		}
		runner.runsMu.Unlock()

		err := runner.Abort(sessionKey)
		assert.NoError(t, err)
		assert.True(t, called)

		// Should be removed from active runs
		assert.False(t, runner.IsRunning(sessionKey))
	})
}

func TestIsRunning(t *testing.T) {
	runner, _, cleanup := setupTestRunner(t)
	defer cleanup()

	t.Run("should return false for non-existent session", func(t *testing.T) {
		assert.False(t, runner.IsRunning("non-existent"))
	})

	t.Run("should return true for active session", func(t *testing.T) {
		sessionKey := "test-running"

		runner.runsMu.Lock()
		runner.activeRuns[sessionKey] = func() {}
		runner.runsMu.Unlock()

		assert.True(t, runner.IsRunning(sessionKey))
	})
}

func TestEstimateTokens(t *testing.T) {
	t.Run("should estimate tokens correctly", func(t *testing.T) {
		messages := []AgentMessage{
			{Role: "user", Content: "Hello"},
			{Role: "assistant", Content: "Hi there"},
		}

		tokens := EstimateTokens(messages)
		assert.Greater(t, tokens, 0)
		assert.Less(t, tokens, 100)
	})

	t.Run("should handle empty messages", func(t *testing.T) {
		tokens := EstimateTokens([]AgentMessage{})
		assert.Equal(t, 0, tokens)
	})
}

func TestIsRetryableError(t *testing.T) {
	t.Run("should identify retryable errors", func(t *testing.T) {
		assert.True(t, IsRetryableError(fmt.Errorf("ECONNRESET")))
		assert.True(t, IsRetryableError(fmt.Errorf("ETIMEDOUT")))
		assert.True(t, IsRetryableError(fmt.Errorf("429 rate limit")))
		assert.True(t, IsRetryableError(fmt.Errorf("500 server error")))
	})

	t.Run("should identify non-retryable errors", func(t *testing.T) {
		assert.False(t, IsRetryableError(fmt.Errorf("invalid API key")))
		assert.False(t, IsRetryableError(fmt.Errorf("validation failed")))
		assert.False(t, IsRetryableError(nil))
	})
}

func TestSortProfilesByPriority(t *testing.T) {
	t.Run("should sort profiles by priority", func(t *testing.T) {
		profiles := []AuthProfile{
			{ID: "low", Priority: 3},
			{ID: "high", Priority: 1},
			{ID: "medium", Priority: 2},
		}

		sortProfilesByPriority(profiles)

		assert.Equal(t, "high", profiles[0].ID)
		assert.Equal(t, "medium", profiles[1].ID)
		assert.Equal(t, "low", profiles[2].ID)
	})
}
