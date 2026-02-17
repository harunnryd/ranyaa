package agent

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/harun/ranya/internal/observability"
	"github.com/harun/ranya/internal/tracing"
	"github.com/harun/ranya/pkg/commandqueue"
	"github.com/harun/ranya/pkg/memory"
	"github.com/harun/ranya/pkg/session"
	"github.com/harun/ranya/pkg/toolexecutor"
	"github.com/rs/zerolog"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
)

// Runner orchestrates AI agent execution
type Runner struct {
	sessionManager  *session.SessionManager
	toolExecutor    *toolexecutor.ToolExecutor
	commandQueue    *commandqueue.CommandQueue
	memoryManager   *memory.Manager
	logger          zerolog.Logger
	providerFactory ProviderCreator

	// Auth profiles
	authProfiles []AuthProfile
	authMu       sync.RWMutex

	// Active runs for abort capability
	activeRuns map[string]context.CancelFunc
	runsMu     sync.RWMutex
}

// Config holds runner configuration
type Config struct {
	SessionManager  *session.SessionManager
	ToolExecutor    *toolexecutor.ToolExecutor
	CommandQueue    *commandqueue.CommandQueue
	MemoryManager   *memory.Manager
	Logger          zerolog.Logger
	AuthProfiles    []AuthProfile
	ProviderFactory ProviderCreator
}

// ProviderCreator creates LLM providers from auth profiles.
type ProviderCreator interface {
	NewProvider(profile AuthProfile) (LLMProvider, error)
}

// NewRunner creates a new agent runner
func NewRunner(cfg Config) (*Runner, error) {
	observability.EnsureRegistered()

	if cfg.SessionManager == nil {
		return nil, fmt.Errorf("session manager is required")
	}
	if cfg.ToolExecutor == nil {
		return nil, fmt.Errorf("tool executor is required")
	}
	if cfg.CommandQueue == nil {
		return nil, fmt.Errorf("command queue is required")
	}
	if len(cfg.AuthProfiles) == 0 {
		return nil, fmt.Errorf("at least one auth profile is required")
	}

	providerFactory := cfg.ProviderFactory
	if providerFactory == nil {
		providerFactory = &ProviderFactory{}
	}

	return &Runner{
		sessionManager:  cfg.SessionManager,
		toolExecutor:    cfg.ToolExecutor,
		commandQueue:    cfg.CommandQueue,
		memoryManager:   cfg.MemoryManager,
		logger:          cfg.Logger,
		providerFactory: providerFactory,
		authProfiles:    cfg.AuthProfiles,
		activeRuns:      make(map[string]context.CancelFunc),
	}, nil
}

// Run executes an agent with the given parameters
func (r *Runner) Run(params AgentRunParams) (AgentResult, error) {
	return r.RunWithContext(context.Background(), params)
}

// RunWithContext executes an agent with a caller-provided context.
func (r *Runner) RunWithContext(ctx context.Context, params AgentRunParams) (AgentResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if tracing.GetTraceID(ctx) == "" {
		ctx = tracing.NewRequestContext(ctx)
	}
	ctx = tracing.WithSessionKey(ctx, params.SessionKey)
	ctx, span := tracing.StartSpan(
		ctx,
		"ranya.agent",
		"agent.run",
		attribute.String("session_key", params.SessionKey),
	)
	defer span.End()
	logger := tracing.LoggerFromContext(ctx, r.logger).With().Str("session_key", params.SessionKey).Logger()

	// Validate configuration
	if err := r.validateConfig(params.Config); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return AgentResult{}, fmt.Errorf("invalid configuration: %w", err)
	}

	// Enqueue in command queue (session-specific lane)
	lane := fmt.Sprintf("session-%s", params.SessionKey)

	result, err := r.commandQueue.EnqueueWithContext(ctx, lane, func(taskCtx context.Context) (interface{}, error) {
		return r.executeAgent(taskCtx, params)
	}, nil)

	if err != nil {
		logger.Error().Err(err).Msg("Agent run failed before execution")
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return AgentResult{}, err
	}

	return result.(AgentResult), nil
}

// Abort cancels a running agent execution
func (r *Runner) Abort(sessionKey string) error {
	r.runsMu.Lock()
	defer r.runsMu.Unlock()

	cancel, exists := r.activeRuns[sessionKey]
	if !exists {
		r.logger.Debug().Str("sessionKey", sessionKey).Msg("No active run to abort")
		return nil
	}

	r.logger.Info().Str("sessionKey", sessionKey).Msg("Aborting agent execution")
	cancel()
	delete(r.activeRuns, sessionKey)

	return nil
}

// IsRunning checks if an agent is currently running for a session
func (r *Runner) IsRunning(sessionKey string) bool {
	r.runsMu.RLock()
	defer r.runsMu.RUnlock()

	_, exists := r.activeRuns[sessionKey]
	return exists
}

// executeAgent performs the actual agent execution
func (r *Runner) executeAgent(ctx context.Context, params AgentRunParams) (AgentResult, error) {
	ctx = tracing.WithSessionKey(ctx, params.SessionKey)
	logger := tracing.LoggerFromContext(ctx, r.logger).With().Str("session_key", params.SessionKey).Logger()

	// Create cancellable context
	execCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Register abort controller
	r.runsMu.Lock()
	r.activeRuns[params.SessionKey] = cancel
	r.runsMu.Unlock()

	defer func() {
		r.runsMu.Lock()
		delete(r.activeRuns, params.SessionKey)
		r.runsMu.Unlock()
	}()

	// Check if already aborted
	select {
	case <-execCtx.Done():
		return AgentResult{
			SessionKey: params.SessionKey,
			Aborted:    true,
		}, nil
	default:
	}

	// Load session history
	history, err := r.loadSessionHistory(execCtx, params.SessionKey)
	if err != nil {
		logger.Error().Err(err).Msg("Failed to load session history")
		return AgentResult{}, fmt.Errorf("failed to load session history: %w", err)
	}

	// Build messages
	messages, err := r.buildMessages(execCtx, history, params)
	if err != nil {
		return AgentResult{}, fmt.Errorf("failed to build messages: %w", err)
	}

	// Build tools
	tools, err := r.buildTools(params.Config.Tools)
	if err != nil {
		return AgentResult{}, fmt.Errorf("failed to build tools: %w", err)
	}

	// Save user message
	if err := r.sessionManager.AppendMessageWithContext(execCtx, params.SessionKey, session.Message{
		Role:    "user",
		Content: params.Prompt,
	}); err != nil {
		logger.Error().Err(err).Msg("Failed to persist user message")
		return AgentResult{}, fmt.Errorf("failed to save user message: %w", err)
	}

	// Execute with auth failover and tool loop
	result, err := r.executeWithFailover(execCtx, messages, tools, params)
	if err != nil {
		return AgentResult{}, err
	}

	// Save assistant response
	if err := r.sessionManager.AppendMessageWithContext(execCtx, params.SessionKey, session.Message{
		Role:    "assistant",
		Content: result.Response,
		Metadata: map[string]interface{}{
			"model": params.Config.Model,
			"usage": result.Usage,
		},
	}); err != nil {
		logger.Error().Err(err).Msg("Failed to persist assistant message")
		return AgentResult{}, fmt.Errorf("failed to save assistant message: %w", err)
	}

	result.SessionKey = params.SessionKey
	return result, nil
}

// validateConfig validates agent configuration
func (r *Runner) validateConfig(config AgentConfig) error {
	if config.Model == "" {
		return fmt.Errorf("model cannot be empty")
	}
	if config.Temperature < 0 || config.Temperature > 1 {
		return fmt.Errorf("temperature must be between 0 and 1")
	}
	if config.MaxTokens < 0 {
		return fmt.Errorf("max tokens cannot be negative")
	}
	if config.MaxRetries < 0 {
		return fmt.Errorf("max retries cannot be negative")
	}
	return nil
}

// loadSessionHistory loads and validates session history
func (r *Runner) loadSessionHistory(ctx context.Context, sessionKey string) ([]session.SessionEntry, error) {
	entries, err := r.sessionManager.LoadSessionWithContext(ctx, sessionKey)
	if err != nil {
		return nil, err
	}

	// Validate entries
	for i, entry := range entries {
		if entry.Message.Role == "" {
			return nil, fmt.Errorf("message %d has empty role", i)
		}
		if entry.Message.Content == "" {
			return nil, fmt.Errorf("message %d has empty content", i)
		}
	}

	return entries, nil
}

// buildMessages constructs the message array for LLM
func (r *Runner) buildMessages(ctx context.Context, history []session.SessionEntry, params AgentRunParams) ([]AgentMessage, error) {
	messages := []AgentMessage{}

	// Add system prompt with optional memory context
	systemPrompt := params.Config.SystemPrompt
	if systemPrompt == "" {
		systemPrompt = "You are a helpful assistant."
	}

	// Add memory context if enabled
	if params.Config.UseMemory && r.memoryManager != nil {
		memoryContext, err := r.getMemoryContext(ctx, params.Prompt)
		if err != nil {
			logger := tracing.LoggerFromContext(ctx, r.logger)
			logger.Warn().Err(err).Msg("Failed to load memory context")
		} else if memoryContext != "" {
			systemPrompt = fmt.Sprintf("%s\n\n# Relevant Context from Memory\n\n%s", systemPrompt, memoryContext)
		}
	}

	messages = append(messages, AgentMessage{
		Role:    "system",
		Content: systemPrompt,
	})

	// Add conversation history
	for _, entry := range history {
		messages = append(messages, AgentMessage{
			Role:    entry.Message.Role,
			Content: entry.Message.Content,
		})
	}

	// Add current user message
	messages = append(messages, AgentMessage{
		Role:    "user",
		Content: params.Prompt,
	})

	// Compact if needed
	messages = r.compactIfNeeded(messages, params.Config.MaxTokens)

	return messages, nil
}

// getMemoryContext retrieves relevant context from memory
func (r *Runner) getMemoryContext(ctx context.Context, prompt string) (string, error) {
	results, err := r.memoryManager.SearchWithContext(ctx, prompt, &memory.SearchOptions{
		Limit:    3,
		MinScore: 0.5,
	})
	if err != nil {
		return "", err
	}

	if len(results) == 0 {
		return "", nil
	}

	context := ""
	for i, result := range results {
		context += fmt.Sprintf("## Memory %d (relevance: %.2f)\nSource: %s\n\n%s\n\n---\n\n",
			i+1, result.Score, result.FilePath, result.Content)
	}

	return context, nil
}

// compactIfNeeded compacts messages if they exceed token limit
func (r *Runner) compactIfNeeded(messages []AgentMessage, maxTokens int) []AgentMessage {
	if maxTokens <= 0 {
		maxTokens = 4096
	}

	tokenCount := EstimateTokens(messages)
	if tokenCount <= maxTokens {
		return messages
	}

	r.logger.Info().
		Int("tokenCount", tokenCount).
		Int("maxTokens", maxTokens).
		Msg("Compacting context")

	// Keep system messages
	systemMessages := []AgentMessage{}
	conversationMessages := []AgentMessage{}

	for _, msg := range messages {
		if msg.Role == "system" {
			systemMessages = append(systemMessages, msg)
		} else {
			conversationMessages = append(conversationMessages, msg)
		}
	}

	// Keep last 20 messages
	recentCount := 20
	if len(conversationMessages) <= recentCount {
		return messages
	}

	recentMessages := conversationMessages[len(conversationMessages)-recentCount:]
	olderCount := len(conversationMessages) - recentCount

	// Create summary
	summary := AgentMessage{
		Role:    "system",
		Content: fmt.Sprintf("[Previous conversation summary: %d messages exchanged]", olderCount),
	}

	result := append(systemMessages, summary)
	result = append(result, recentMessages...)

	return result
}

// buildTools converts tool names to tool definitions
func (r *Runner) buildTools(toolNames []string) ([]interface{}, error) {
	if len(toolNames) == 0 {
		return nil, nil
	}

	tools := []interface{}{}

	for _, name := range toolNames {
		toolDef := r.toolExecutor.GetTool(name)
		if toolDef == nil {
			return nil, fmt.Errorf("tool not found: %s", name)
		}

		// Convert to tool format
		inputSchema := map[string]interface{}{
			"type":       "object",
			"properties": make(map[string]interface{}),
		}

		required := []string{}
		properties := inputSchema["properties"].(map[string]interface{})

		for _, param := range toolDef.Parameters {
			properties[param.Name] = map[string]interface{}{
				"type":        param.Type,
				"description": param.Description,
			}
			if param.Required {
				required = append(required, param.Name)
			}
		}

		if len(required) > 0 {
			inputSchema["required"] = required
		}

		tools = append(tools, map[string]interface{}{
			"name":         toolDef.Name,
			"description":  toolDef.Description,
			"input_schema": inputSchema,
		})
	}

	return tools, nil
}

// executeWithFailover executes with auth profile failover
func (r *Runner) executeWithFailover(ctx context.Context, messages []AgentMessage, tools []interface{}, params AgentRunParams) (AgentResult, error) {
	r.authMu.RLock()
	profiles := make([]AuthProfile, len(r.authProfiles))
	copy(profiles, r.authProfiles)
	r.authMu.RUnlock()
	logger := tracing.LoggerFromContext(ctx, r.logger).With().Str("session_key", params.SessionKey).Logger()

	// Sort by priority
	sortProfilesByPriority(profiles)

	var lastErr error

	for _, profile := range profiles {
		profileStart := time.Now()
		// Skip profiles in cooldown
		if profile.CooldownUntil != nil && time.Now().UnixMilli() < *profile.CooldownUntil {
			observability.SetProviderCooldown(profile.Provider, true)
			logger.Debug().
				Str("profileId", profile.ID).
				Msg("Skipping profile in cooldown")
			continue
		}

		observability.SetProviderCooldown(profile.Provider, false)
		logger.Info().Str("profileId", profile.ID).Msg("Trying auth profile")

		// Create provider
		provider, err := r.providerFactory.NewProvider(profile)
		if err != nil {
			observability.RecordAgentRun(profile.Provider, time.Since(profileStart), false)
			logger.Warn().
				Str("profileId", profile.ID).
				Err(err).
				Msg("Failed to create provider")
			continue
		}

		result, err := r.executeWithProvider(ctx, provider, messages, tools, params)
		if err == nil {
			// Success - reset failure count
			r.updateProfileSuccess(profile.ID)
			observability.RecordAgentRun(profile.Provider, time.Since(profileStart), true)
			observability.SetProviderCooldown(profile.Provider, false)
			return result, nil
		}

		lastErr = err
		observability.RecordAgentRun(profile.Provider, time.Since(profileStart), false)
		logger.Warn().
			Str("profileId", profile.ID).
			Err(err).
			Msg("Auth profile failed")

		// Mark failure
		r.updateProfileFailure(profile.ID)

		// Don't retry on permanent errors
		if !IsRetryableError(err) {
			return AgentResult{}, err
		}
	}

	if lastErr != nil {
		logger.Error().Err(lastErr).Msg("All auth profiles failed")
	}
	return AgentResult{}, fmt.Errorf("all auth profiles failed: %w", lastErr)
}

// executeWithProvider executes with a specific LLM provider
func (r *Runner) executeWithProvider(ctx context.Context, provider LLMProvider, messages []AgentMessage, tools []interface{}, params AgentRunParams) (AgentResult, error) {
	ctx, span := tracing.StartSpan(
		ctx,
		"ranya.agent",
		"agent.execute_with_provider",
		attribute.String("provider", provider.Provider()),
	)
	defer span.End()

	// Execute with tool loop
	result, err := r.executeWithTools(ctx, provider, messages, tools, params)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}
	return result, err
}

// executeWithTools handles the tool execution loop
func (r *Runner) executeWithTools(ctx context.Context, provider LLMProvider, messages []AgentMessage, tools []interface{}, params AgentRunParams) (AgentResult, error) {
	currentMessages := messages
	allToolCalls := []ToolCall{}

	// Get system prompt
	systemPrompt := ""
	for _, msg := range messages {
		if msg.Role == "system" {
			systemPrompt = msg.Content
			break
		}
	}

	// Maximum 10 turns to prevent infinite loops
	for turn := 0; turn < 10; turn++ {
		// Check for abort
		select {
		case <-ctx.Done():
			return AgentResult{Aborted: true}, nil
		default:
		}

		// Call LLM with retry
		response, err := r.callLLMWithRetry(ctx, provider, currentMessages, tools, systemPrompt, params)
		if err != nil {
			return AgentResult{}, err
		}

		// No tool calls - we're done
		if len(response.ToolCalls) == 0 {
			return AgentResult{
				Response:  response.Content,
				ToolCalls: allToolCalls,
				Usage:     response.Usage,
			}, nil
		}

		// Execute tool calls
		toolResults := []ToolResult{}
		for _, toolCall := range response.ToolCalls {
			result := r.toolExecutor.Execute(
				ctx,
				toolCall.Name,
				toolCall.Parameters,
				&toolexecutor.ExecutionContext{
					SessionKey: params.SessionKey,
					WorkingDir: params.CWD,
					Timeout:    30 * time.Second,
				},
			)

			toolResults = append(toolResults, ToolResult{
				ToolCallID: toolCall.ID,
				Output:     fmt.Sprintf("%v", result.Output),
				Error:      result.Error,
			})
		}

		// Add assistant message with tool calls
		currentMessages = append(currentMessages, AgentMessage{
			Role:      "assistant",
			Content:   response.Content,
			ToolCalls: response.ToolCalls,
		})

		// Add tool results
		for _, result := range toolResults {
			content := result.Output
			if result.Error != "" {
				content = result.Error
			}
			currentMessages = append(currentMessages, AgentMessage{
				Role:       "tool",
				Content:    content,
				ToolCallID: result.ToolCallID,
			})
		}

		allToolCalls = append(allToolCalls, response.ToolCalls...)
	}

	return AgentResult{}, fmt.Errorf("maximum tool execution turns exceeded")
}

// callLLMWithRetry calls LLM with exponential backoff retry
func (r *Runner) callLLMWithRetry(ctx context.Context, provider LLMProvider, messages []AgentMessage, tools []interface{}, systemPrompt string, params AgentRunParams) (*LLMResponse, error) {
	maxRetries := params.Config.MaxRetries
	if maxRetries <= 0 {
		maxRetries = 3
	}

	var lastErr error

	for attempt := 0; attempt < maxRetries; attempt++ {
		response, err := r.callLLM(ctx, provider, messages, tools, systemPrompt, params)
		if err == nil {
			return response, nil
		}

		lastErr = err

		// Don't retry on permanent errors
		if !IsRetryableError(err) {
			return nil, err
		}

		// Last attempt - don't wait
		if attempt == maxRetries-1 {
			break
		}

		// Exponential backoff: 1s, 2s, 4s
		delayMs := 1000 * (1 << attempt)
		r.logger.Info().
			Int("attempt", attempt+1).
			Int("delayMs", delayMs).
			Msg("Retrying after error")

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(time.Duration(delayMs) * time.Millisecond):
		}
	}

	return nil, fmt.Errorf("max retries (%d) exceeded: %w", maxRetries, lastErr)
}

// callLLM makes a single LLM API call
func (r *Runner) callLLM(ctx context.Context, provider LLMProvider, messages []AgentMessage, tools []interface{}, systemPrompt string, params AgentRunParams) (*LLMResponse, error) {
	// Build request
	request := LLMRequest{
		Model:        params.Config.Model,
		Messages:     messages,
		Tools:        tools,
		Temperature:  params.Config.Temperature,
		MaxTokens:    params.Config.MaxTokens,
		SystemPrompt: systemPrompt,
	}

	// Make API call
	return provider.Call(ctx, request)
}

// updateProfileSuccess resets failure count for a profile
func (r *Runner) updateProfileSuccess(profileID string) {
	r.authMu.Lock()
	defer r.authMu.Unlock()

	for i := range r.authProfiles {
		if r.authProfiles[i].ID == profileID {
			r.authProfiles[i].FailureCount = 0
			r.authProfiles[i].CooldownUntil = nil
			observability.SetProviderCooldown(r.authProfiles[i].Provider, false)
			break
		}
	}
}

// updateProfileFailure marks a profile as failed
func (r *Runner) updateProfileFailure(profileID string) {
	r.authMu.Lock()
	defer r.authMu.Unlock()

	for i := range r.authProfiles {
		if r.authProfiles[i].ID == profileID {
			r.authProfiles[i].FailureCount++
			cooldownMs := time.Now().UnixMilli() + int64(60000*r.authProfiles[i].FailureCount)
			r.authProfiles[i].CooldownUntil = &cooldownMs
			observability.SetProviderCooldown(r.authProfiles[i].Provider, true)
			break
		}
	}
}

// sortProfilesByPriority sorts profiles by priority (lower = higher priority)
func sortProfilesByPriority(profiles []AuthProfile) {
	for i := 0; i < len(profiles)-1; i++ {
		for j := i + 1; j < len(profiles); j++ {
			if profiles[j].Priority < profiles[i].Priority {
				profiles[i], profiles[j] = profiles[j], profiles[i]
			}
		}
	}
}
