package gateway

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/harun/ranya/internal/tracing"
	"github.com/harun/ranya/pkg/agent"
	"github.com/harun/ranya/pkg/memory"
	"github.com/harun/ranya/pkg/session"
)

// registerBuiltinMethods registers all built-in RPC methods
func (s *Server) registerBuiltinMethods() {
	_ = s.RegisterMethodWithScopes("agent.wait", []string{"operator.write"}, s.handleAgentWait)
	_ = s.RegisterMethodWithScopes("agent.abort", []string{"operator.write"}, s.handleAgentAbort)
	_ = s.RegisterMethodWithScopes("chat.send", []string{"operator.write"}, s.handleChatSend)
	_ = s.RegisterMethodWithScopes("sessions.send", []string{"operator.write"}, s.handleSessionsSend)
	_ = s.RegisterMethodWithScopes("sessions.list", []string{"operator.read"}, s.handleSessionsList)
	_ = s.RegisterMethodWithScopes("sessions.get", []string{"operator.read"}, s.handleSessionsGet)
	_ = s.RegisterMethodWithScopes("sessions.delete", []string{"operator.admin"}, s.handleSessionsDelete)

	if s.memoryManager != nil {
		_ = s.RegisterMethodWithScopes("memory.search", []string{"operator.read"}, s.handleMemorySearch)
	}
}

// handleAgentWait handles agent.wait RPC method
func (s *Server) handleAgentWait(ctx context.Context, params map[string]interface{}) (interface{}, error) {
	// Extract parameters
	prompt, ok := params["prompt"].(string)
	if !ok {
		return nil, fmt.Errorf("prompt parameter is required and must be a string")
	}

	sessionKey, ok := params["sessionKey"].(string)
	if !ok {
		return nil, fmt.Errorf("sessionKey parameter is required and must be a string")
	}

	// Extract config overrides (optional).
	// Empty values defer to agent-level defaults resolved by runtime pipeline.
	config := agent.AgentConfig{}

	if configMap, ok := params["config"].(map[string]interface{}); ok {
		if model, ok := configMap["model"].(string); ok {
			config.Model = model
		}
		if temp, ok := configMap["temperature"].(float64); ok {
			config.Temperature = temp
		}
		if maxTokens, ok := configMap["maxTokens"].(float64); ok {
			config.MaxTokens = int(maxTokens)
		}
		if systemPrompt, ok := configMap["systemPrompt"].(string); ok {
			config.SystemPrompt = systemPrompt
		}
		if tools, ok := configMap["tools"].([]interface{}); ok {
			toolNames := make([]string, 0, len(tools))
			for _, t := range tools {
				if toolName, ok := t.(string); ok {
					toolNames = append(toolNames, toolName)
				}
			}
			config.Tools = toolNames
		}
		if useMemory, ok := configMap["useMemory"].(bool); ok {
			config.UseMemory = useMemory
		}
		if streaming, ok := configMap["streaming"].(bool); ok {
			config.Streaming = streaming
		}
	}

	// Extract cwd (optional)
	cwd := ""
	if cwdParam, ok := params["cwd"].(string); ok {
		cwd = cwdParam
	}

	agentID := ""
	if requestedAgentID, ok := params["agentId"].(string); ok {
		agentID = strings.TrimSpace(requestedAgentID)
	}

	// Setup tracing context
	ctx = tracing.NewRequestContext(ctx)
	ctx = tracing.WithSessionKey(ctx, sessionKey)
	ctx = tracing.WithRunID(ctx, tracing.NewRunID())

	// Generate request ID for idempotency
	requestID := tracing.NewTraceID()
	// ctx = tracing.WithRequestID(ctx, requestID)

	// Subscribe to runtime events for streaming
	if s.runtimeEventHub != nil {
		events, cancel := s.runtimeEventHub.Subscribe(sessionKey, 64)
		defer cancel()

		// Start streaming goroutine
		go s.streamRuntimeEvents(ctx, sessionKey, events)
	}

	result, err := s.agentDispatcher(ctx, AgentDispatchRequest{
		Prompt:     prompt,
		SessionKey: sessionKey,
		Source:     "gateway",
		AgentID:    agentID,
		Config:     config,
		CWD:        cwd,
		Metadata:   params,
	})

	if err != nil {
		return nil, fmt.Errorf("agent execution failed: %w", err)
	}

	// Send completion event
	if s.broadcaster != nil {
		clientID := clientIDFromContext(ctx)
		if clientID != "" {
			s.broadcaster.BroadcastToClient(clientID, EventMessage{
				Event:     "chat.complete",
				Stream:    StreamTypeLifecycle,
				Phase:     "complete",
				Session:   sessionKey,
				Data:      result,
				Timestamp: time.Now().UnixMilli(),
			})
		}
	}

	// Convert result to map
	return map[string]interface{}{
		"response":   result.Response,
		"toolCalls":  result.ToolCalls,
		"usage":      result.Usage,
		"sessionKey": result.SessionKey,
		"aborted":    result.Aborted,
		"requestId":  requestID,
	}, nil
}

// handleChatSend handles chat.send RPC method (send message + run agent)
func (s *Server) handleChatSend(ctx context.Context, params map[string]interface{}) (interface{}, error) {
	sessionKey, ok := params["sessionKey"].(string)
	if !ok {
		return nil, fmt.Errorf("sessionKey parameter is required and must be a string")
	}

	messageContent, ok := params["message"].(string) // "message" as input text
	if !ok {
		// Fallback to "prompt" if message not set, to match agent.wait
		if prompt, ok := params["prompt"].(string); ok {
			messageContent = prompt
		} else {
			return nil, fmt.Errorf("message (or prompt) parameter is required")
		}
	}

	// Ensure prompt param is set for agent execution
	params["prompt"] = messageContent

	// Append user message
	reqCtx := tracing.NewRequestContext(ctx)
	reqCtx = tracing.WithSessionKey(reqCtx, sessionKey)

	if err := s.sessionManager.AppendMessageWithContext(reqCtx, sessionKey, session.Message{
		Role:    "user",
		Content: messageContent,
	}); err != nil {
		return nil, fmt.Errorf("failed to append message: %w", err)
	}

	// Broadcast session.message event
	if s.broadcaster != nil {
		// This one might be broadcast to everyone observing the session?
		// For now, let's keep it broad or restrict?
		// Existing behavior was global broadcast for session.message.
		// We'll stick to global for session updates if that was the intent, or maybe restrict.
		// Given the gap is about "gateway streaming", let's leave session updates global for now
		// (assuming multiple clients might watch a session), but ensure runtime events are private.
		s.broadcaster.Broadcast("session.message", map[string]interface{}{
			"sessionKey": sessionKey,
			"message": map[string]interface{}{
				"role":    "user",
				"content": messageContent,
			},
		})
	}

	return s.handleAgentWait(ctx, params)
}

// streamRuntimeEvents streams runtime events to gateway clients
func (s *Server) streamRuntimeEvents(ctx context.Context, sessionKey string, events <-chan agent.RuntimeEvent) {
	clientID := clientIDFromContext(ctx)

	// If no client ID (e.g. HTTP request), we do NOT stream to prevent leakage.
	if clientID == "" {
		return
	}

	for evt := range events {
		if s.broadcaster == nil {
			continue
		}

		// Map agent runtime events to gateway event messages
		stream := StreamTypeLifecycle
		switch evt.Stream {
		case agent.RuntimeStreamTool:
			stream = StreamTypeTool
		case agent.RuntimeStreamAssistant:
			stream = StreamTypeAssistant
		case agent.RuntimeStreamReasoning:
			stream = StreamTypeReasoning
		case agent.RuntimeStreamLifecycle:
			stream = StreamTypeLifecycle
		}

		// Construct data payload
		data := make(map[string]interface{})
		if evt.Content != "" {
			data["content"] = evt.Content
		}
		if evt.ToolName != "" {
			data["tool_name"] = evt.ToolName
		}
		if evt.ToolCall != "" {
			data["tool_call"] = evt.ToolCall
		}
		if evt.Metadata != nil {
			for k, v := range evt.Metadata {
				data[k] = v
			}
		}

		s.broadcaster.BroadcastToClient(clientID, EventMessage{
			Event:     evt.Event,
			Stream:    stream,
			Phase:     evt.Phase,
			Session:   sessionKey,
			Data:      data,
			TraceID:   tracing.GetTraceID(ctx),
			RunID:     tracing.GetRunID(ctx),
			AgentID:   tracing.GetAgentID(ctx),
			Timestamp: time.Now().UnixMilli(),
		})
	}
}

// handleAgentAbort handles agent.abort RPC method
func (s *Server) handleAgentAbort(ctx context.Context, params map[string]interface{}) (interface{}, error) {
	sessionKey, ok := params["sessionKey"].(string)
	if !ok {
		return nil, fmt.Errorf("sessionKey parameter is required and must be a string")
	}

	if err := s.agentRunner.Abort(sessionKey); err != nil {
		return nil, fmt.Errorf("failed to abort agent: %w", err)
	}

	return map[string]interface{}{
		"success": true,
		"message": "Agent execution aborted",
	}, nil
}

// handleSessionsSend handles sessions.send RPC method
func (s *Server) handleSessionsSend(ctx context.Context, params map[string]interface{}) (interface{}, error) {
	targetSessionKey, ok := params["targetSessionKey"].(string)
	if !ok {
		return nil, fmt.Errorf("targetSessionKey parameter is required and must be a string")
	}

	messageContent, ok := params["message"].(string)
	if !ok {
		return nil, fmt.Errorf("message parameter is required and must be a string")
	}

	// Extract role (optional, defaults to "user")
	role := "user"
	if roleParam, ok := params["role"].(string); ok {
		role = roleParam
	}

	// Append message to session
	reqCtx := tracing.NewRequestContext(ctx)
	reqCtx = tracing.WithSessionKey(reqCtx, targetSessionKey)
	if err := s.sessionManager.AppendMessageWithContext(reqCtx, targetSessionKey, session.Message{
		Role:    role,
		Content: messageContent,
	}); err != nil {
		return nil, fmt.Errorf("failed to append message: %w", err)
	}

	// Broadcast session.message event
	s.broadcaster.Broadcast("session.message", map[string]interface{}{
		"sessionKey": targetSessionKey,
		"message": map[string]interface{}{
			"role":    role,
			"content": messageContent,
		},
	})

	return map[string]interface{}{
		"success": true,
	}, nil
}

// handleSessionsList handles sessions.list RPC method
func (s *Server) handleSessionsList(ctx context.Context, params map[string]interface{}) (interface{}, error) {
	sessions, err := s.sessionManager.ListSessions()
	if err != nil {
		return nil, fmt.Errorf("failed to list sessions: %w", err)
	}

	return map[string]interface{}{
		"sessions": sessions,
	}, nil
}

// handleSessionsGet handles sessions.get RPC method
func (s *Server) handleSessionsGet(ctx context.Context, params map[string]interface{}) (interface{}, error) {
	sessionKey, ok := params["sessionKey"].(string)
	if !ok {
		return nil, fmt.Errorf("sessionKey parameter is required and must be a string")
	}

	reqCtx := tracing.NewRequestContext(ctx)
	reqCtx = tracing.WithSessionKey(reqCtx, sessionKey)
	entries, err := s.sessionManager.LoadSessionWithContext(reqCtx, sessionKey)
	if err != nil {
		return nil, fmt.Errorf("failed to load session: %w", err)
	}

	// Convert entries to map format
	messages := make([]map[string]interface{}, 0, len(entries))
	for _, entry := range entries {
		messages = append(messages, map[string]interface{}{
			"role":      entry.Message.Role,
			"content":   entry.Message.Content,
			"timestamp": entry.Message.Timestamp,
			"metadata":  entry.Message.Metadata,
		})
	}

	return map[string]interface{}{
		"sessionKey": sessionKey,
		"messages":   messages,
	}, nil
}

// handleSessionsDelete handles sessions.delete RPC method
func (s *Server) handleSessionsDelete(ctx context.Context, params map[string]interface{}) (interface{}, error) {
	sessionKey, ok := params["sessionKey"].(string)
	if !ok {
		return nil, fmt.Errorf("sessionKey parameter is required and must be a string")
	}

	reqCtx := tracing.NewRequestContext(ctx)
	reqCtx = tracing.WithSessionKey(reqCtx, sessionKey)
	if err := s.sessionManager.DeleteSessionWithContext(reqCtx, sessionKey); err != nil {
		return nil, fmt.Errorf("failed to delete session: %w", err)
	}

	return map[string]interface{}{
		"success": true,
	}, nil
}

// handleMemorySearch handles memory.search RPC method
func (s *Server) handleMemorySearch(ctx context.Context, params map[string]interface{}) (interface{}, error) {
	if s.memoryManager == nil {
		return nil, fmt.Errorf("memory manager is not available")
	}

	query, ok := params["query"].(string)
	if !ok {
		return nil, fmt.Errorf("query parameter is required and must be a string")
	}

	// Extract options (optional)
	opts := &memory.SearchOptions{
		Limit:    10,
		MinScore: 0.5,
	}

	if limit, ok := params["limit"].(float64); ok {
		opts.Limit = int(limit)
	}

	if minScore, ok := params["minScore"].(float64); ok {
		opts.MinScore = minScore
	}

	// Perform search
	reqCtx := tracing.NewRequestContext(ctx)
	results, err := s.memoryManager.SearchWithContext(reqCtx, query, opts)
	if err != nil {
		return nil, fmt.Errorf("memory search failed: %w", err)
	}

	// Convert results to map format
	resultMaps := make([]map[string]interface{}, 0, len(results))
	for _, result := range results {
		resultMaps = append(resultMaps, map[string]interface{}{
			"chunkId":      result.ChunkID,
			"filePath":     result.FilePath,
			"content":      result.Content,
			"score":        result.Score,
			"vectorScore":  result.VectorScore,
			"keywordScore": result.KeywordScore,
			"metadata":     result.Metadata,
		})
	}

	return map[string]interface{}{
		"results": resultMaps,
	}, nil
}
