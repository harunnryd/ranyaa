package gateway

import (
	"context"
	"fmt"

	"github.com/harun/ranya/internal/tracing"
	"github.com/harun/ranya/pkg/agent"
	"github.com/harun/ranya/pkg/memory"
	"github.com/harun/ranya/pkg/session"
)

// registerBuiltinMethods registers all built-in RPC methods
func (s *Server) registerBuiltinMethods() {
	_ = s.router.RegisterMethod("agent.wait", s.handleAgentWait)
	_ = s.router.RegisterMethod("agent.abort", s.handleAgentAbort)
	_ = s.router.RegisterMethod("sessions.send", s.handleSessionsSend)
	_ = s.router.RegisterMethod("sessions.list", s.handleSessionsList)
	_ = s.router.RegisterMethod("sessions.get", s.handleSessionsGet)
	_ = s.router.RegisterMethod("sessions.delete", s.handleSessionsDelete)

	if s.memoryManager != nil {
		_ = s.router.RegisterMethod("memory.search", s.handleMemorySearch)
	}
}

// handleAgentWait handles agent.wait RPC method
func (s *Server) handleAgentWait(params map[string]interface{}) (interface{}, error) {
	// Extract parameters
	prompt, ok := params["prompt"].(string)
	if !ok {
		return nil, fmt.Errorf("prompt parameter is required and must be a string")
	}

	sessionKey, ok := params["sessionKey"].(string)
	if !ok {
		return nil, fmt.Errorf("sessionKey parameter is required and must be a string")
	}

	// Extract config (optional)
	config := agent.AgentConfig{
		Model:       "claude-3-5-sonnet-20241022",
		Temperature: 0.7,
		MaxTokens:   4096,
		MaxRetries:  3,
	}

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

	ctx := tracing.NewRequestContext(context.Background())
	ctx = tracing.WithSessionKey(ctx, sessionKey)
	ctx = tracing.WithRunID(ctx, tracing.NewRunID())

	result, err := s.agentDispatcher(ctx, AgentDispatchRequest{
		Prompt:     prompt,
		SessionKey: sessionKey,
		Source:     "gateway",
		AgentID:    "default",
		Config:     config,
		CWD:        cwd,
		Metadata:   params,
	})

	if err != nil {
		return nil, fmt.Errorf("agent execution failed: %w", err)
	}

	// Convert result to map
	return map[string]interface{}{
		"response":   result.Response,
		"toolCalls":  result.ToolCalls,
		"usage":      result.Usage,
		"sessionKey": result.SessionKey,
		"aborted":    result.Aborted,
	}, nil
}

// handleAgentAbort handles agent.abort RPC method
func (s *Server) handleAgentAbort(params map[string]interface{}) (interface{}, error) {
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
func (s *Server) handleSessionsSend(params map[string]interface{}) (interface{}, error) {
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
	ctx := tracing.NewRequestContext(context.Background())
	ctx = tracing.WithSessionKey(ctx, targetSessionKey)
	if err := s.sessionManager.AppendMessageWithContext(ctx, targetSessionKey, session.Message{
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
func (s *Server) handleSessionsList(params map[string]interface{}) (interface{}, error) {
	sessions, err := s.sessionManager.ListSessions()
	if err != nil {
		return nil, fmt.Errorf("failed to list sessions: %w", err)
	}

	return map[string]interface{}{
		"sessions": sessions,
	}, nil
}

// handleSessionsGet handles sessions.get RPC method
func (s *Server) handleSessionsGet(params map[string]interface{}) (interface{}, error) {
	sessionKey, ok := params["sessionKey"].(string)
	if !ok {
		return nil, fmt.Errorf("sessionKey parameter is required and must be a string")
	}

	ctx := tracing.NewRequestContext(context.Background())
	ctx = tracing.WithSessionKey(ctx, sessionKey)
	entries, err := s.sessionManager.LoadSessionWithContext(ctx, sessionKey)
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
func (s *Server) handleSessionsDelete(params map[string]interface{}) (interface{}, error) {
	sessionKey, ok := params["sessionKey"].(string)
	if !ok {
		return nil, fmt.Errorf("sessionKey parameter is required and must be a string")
	}

	ctx := tracing.NewRequestContext(context.Background())
	ctx = tracing.WithSessionKey(ctx, sessionKey)
	if err := s.sessionManager.DeleteSessionWithContext(ctx, sessionKey); err != nil {
		return nil, fmt.Errorf("failed to delete session: %w", err)
	}

	return map[string]interface{}{
		"success": true,
	}, nil
}

// handleMemorySearch handles memory.search RPC method
func (s *Server) handleMemorySearch(params map[string]interface{}) (interface{}, error) {
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
	ctx := tracing.NewRequestContext(context.Background())
	results, err := s.memoryManager.SearchWithContext(ctx, query, opts)
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
