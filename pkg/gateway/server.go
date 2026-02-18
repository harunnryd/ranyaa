package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/harun/ranya/internal/observability"
	"github.com/harun/ranya/internal/tracing"
	"github.com/harun/ranya/pkg/agent"
	"github.com/harun/ranya/pkg/commandqueue"
	"github.com/harun/ranya/pkg/memory"
	"github.com/harun/ranya/pkg/session"
	gonanoid "github.com/matoous/go-nanoid/v2"
	"github.com/rs/zerolog"
)

// Server is the main Gateway Server
type Server struct {
	port            int
	sharedSecret    string
	tickInterval    time.Duration
	server          *http.Server
	upgrader        websocket.Upgrader
	clients         *ClientRegistry
	router          *RPCRouter
	authHandler     *AuthHandler
	broadcaster     *EventBroadcaster
	commandQueue    *commandqueue.CommandQueue
	agentRunner     *agent.Runner
	agentDispatcher AgentDispatcher
	sessionManager  *session.SessionManager
	memoryManager   *memory.Manager
	runtimeEventHub RuntimeEventHub
	logger          zerolog.Logger
	isShuttingDown  bool
	shutdownMu      sync.RWMutex
	inFlightReqs    sync.WaitGroup
	tickCancel      context.CancelFunc
	tickWG          sync.WaitGroup
}

// Config holds server configuration
type Config struct {
	Port            int
	SharedSecret    string
	TickInterval    time.Duration
	CommandQueue    *commandqueue.CommandQueue
	AgentRunner     *agent.Runner
	AgentDispatcher AgentDispatcher
	SessionManager  *session.SessionManager
	MemoryManager   *memory.Manager
	RuntimeEventHub RuntimeEventHub
	Logger          zerolog.Logger
}

// RuntimeEventHub provides subscription to runtime events for streaming
type RuntimeEventHub interface {
	Subscribe(sessionKey string, buffer int) (<-chan agent.RuntimeEvent, func())
	Publish(sessionKey string, evt agent.RuntimeEvent)
}

// AgentDispatchRequest carries an ingress request into the canonical runtime flow.
type AgentDispatchRequest struct {
	Prompt     string
	SessionKey string
	Source     string
	AgentID    string
	Config     agent.AgentConfig
	CWD        string
	Metadata   map[string]interface{}
}

// AgentDispatcher routes requests into the daemon runtime pipeline.
type AgentDispatcher func(ctx context.Context, req AgentDispatchRequest) (agent.AgentResult, error)

// NewServer creates a new Gateway Server
func NewServer(cfg Config) (*Server, error) {
	if cfg.Port <= 0 {
		return nil, fmt.Errorf("invalid port: %d", cfg.Port)
	}
	if cfg.SharedSecret == "" {
		return nil, fmt.Errorf("shared secret is required")
	}
	if cfg.CommandQueue == nil {
		return nil, fmt.Errorf("command queue is required")
	}
	if cfg.AgentRunner == nil {
		return nil, fmt.Errorf("agent runner is required")
	}
	if cfg.AgentDispatcher == nil {
		return nil, fmt.Errorf("agent dispatcher is required")
	}
	if cfg.SessionManager == nil {
		return nil, fmt.Errorf("session manager is required")
	}
	if cfg.TickInterval <= 0 {
		cfg.TickInterval = 30 * time.Second
	}

	clients := NewClientRegistry()
	router := NewRPCRouter()
	authHandler := NewAuthHandler(cfg.SharedSecret)
	broadcaster := NewEventBroadcaster(clients, cfg.Logger)

	s := &Server{
		port:            cfg.Port,
		sharedSecret:    cfg.SharedSecret,
		tickInterval:    cfg.TickInterval,
		clients:         clients,
		router:          router,
		authHandler:     authHandler,
		broadcaster:     broadcaster,
		commandQueue:    cfg.CommandQueue,
		agentRunner:     cfg.AgentRunner,
		agentDispatcher: cfg.AgentDispatcher,
		sessionManager:  cfg.SessionManager,
		memoryManager:   cfg.MemoryManager,
		runtimeEventHub: cfg.RuntimeEventHub,
		logger:          cfg.Logger,
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				return true // Allow all origins for now
			},
		},
	}

	// Register built-in methods
	s.registerBuiltinMethods()

	return s, nil
}

// Start starts the Gateway Server
func (s *Server) Start() error {
	mux := http.NewServeMux()
	mux.HandleFunc("/ws", s.handleWebSocket)
	mux.HandleFunc("/rpc", s.handleRPC)
	mux.HandleFunc("/events", s.handleSSE)
	mux.HandleFunc("/api/agent/stream", s.handleAgentStream)
	mux.Handle("/metrics", observability.MetricsHandler())
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})

	s.server = &http.Server{
		Addr:    fmt.Sprintf(":%d", s.port),
		Handler: mux,
	}

	s.logger.Info().Int("port", s.port).Msg("Starting Gateway Server")

	// Start server in goroutine so it doesn't block
	go func() {
		if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			s.logger.Error().Err(err).Msg("Gateway server error")
		}
	}()

	// Give the server a moment to start
	time.Sleep(50 * time.Millisecond)
	s.startTickEmitter()

	return nil
}

// Stop gracefully stops the Gateway Server
func (s *Server) Stop() error {
	s.shutdownMu.Lock()
	s.isShuttingDown = true
	s.shutdownMu.Unlock()

	s.logger.Info().Msg("Shutting down Gateway Server")
	s.stopTickEmitter()

	// Broadcast shutdown event
	s.broadcaster.Broadcast("server.shutdown", map[string]interface{}{
		"message": "Server is shutting down",
	})

	// Wait for in-flight requests with timeout
	done := make(chan struct{})
	go func() {
		s.inFlightReqs.Wait()
		close(done)
	}()

	select {
	case <-done:
		s.logger.Info().Msg("All in-flight requests completed")
	case <-time.After(30 * time.Second):
		s.logger.Warn().Msg("Shutdown timeout reached, forcing close")
	}

	// Close all client connections
	clients := s.clients.GetAll()
	for _, client := range clients {
		client.Conn.Close()
	}

	// Shutdown HTTP server
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := s.server.Shutdown(ctx); err != nil {
		return fmt.Errorf("failed to shutdown server: %w", err)
	}

	s.logger.Info().Msg("Gateway Server stopped")
	return nil
}

func (s *Server) startTickEmitter() {
	if s.tickInterval <= 0 {
		return
	}

	tickCtx, cancel := context.WithCancel(context.Background())
	s.tickCancel = cancel
	s.tickWG.Add(1)

	go func() {
		defer s.tickWG.Done()

		ticker := time.NewTicker(s.tickInterval)
		defer ticker.Stop()

		for {
			select {
			case <-tickCtx.Done():
				return
			case <-ticker.C:
				s.broadcaster.BroadcastTyped(EventMessage{
					Event:  "tick",
					Stream: StreamTypeLifecycle,
					Phase:  "tick",
					Data: map[string]interface{}{
						"status": "alive",
					},
				})
			}
		}
	}()
}

func (s *Server) stopTickEmitter() {
	if s.tickCancel != nil {
		s.tickCancel()
		s.tickCancel = nil
	}
	s.tickWG.Wait()
}

// handleWebSocket handles WebSocket connections
func (s *Server) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	// Check if shutting down
	s.shutdownMu.RLock()
	if s.isShuttingDown {
		s.shutdownMu.RUnlock()
		http.Error(w, "Server is shutting down", http.StatusServiceUnavailable)
		return
	}
	s.shutdownMu.RUnlock()

	// Upgrade connection
	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		s.logger.Error().Err(err).Msg("Failed to upgrade connection")
		return
	}

	// Create client
	clientID, _ := gonanoid.New()

	// Set read dead-line for initial authentication
	_ = conn.SetReadDeadline(time.Now().Add(s.tickInterval * 2))

	// Configure pong handler to reset read deadline
	conn.SetPongHandler(func(string) error {
		_ = conn.SetReadDeadline(time.Now().Add(s.tickInterval * 2))
		s.clients.UpdateActivity(clientID)
		return nil
	})

	client := &Client{
		ID:            clientID,
		Conn:          conn,
		Authenticated: false,
		ConnectedAt:   time.Now(),
		LastActivity:  time.Now(),
		IPAddress:     r.RemoteAddr,
		AuthAttempts:  0,
		RateLimiter:   NewClientRateLimiter(),
		State:         StateConnecting,
	}

	// Add to registry
	s.clients.Add(client)

	s.logger.Info().
		Str("clientId", clientID).
		Str("ip", r.RemoteAddr).
		Msg("Client connected")

	// Send authentication challenge
	if err := s.sendAuthChallenge(client); err != nil {
		s.logger.Error().Err(err).Str("clientId", clientID).Msg("Failed to send auth challenge")
		conn.Close()
		s.clients.Remove(clientID)
		return
	}

	// Handle client messages
	go s.handleClient(client)
}

// sendAuthChallenge sends an authentication challenge to a client
func (s *Server) sendAuthChallenge(client *Client) error {
	challenge, err := s.authHandler.GenerateChallenge()
	if err != nil {
		return err
	}

	client.Challenge = challenge
	client.State = StateAuthenticating

	msg := AuthChallenge{
		Event:     "auth.challenge",
		Challenge: challenge,
	}

	return client.WriteJSON(msg)
}

// handleClient handles messages from a client
func (s *Server) handleClient(client *Client) {
	defer func() {
		client.Conn.Close()
		s.clients.Remove(client.ID)
		s.logger.Info().Str("clientId", client.ID).Msg("Client disconnected")
	}()

	pingTicker := time.NewTicker(s.tickInterval)
	defer pingTicker.Stop()

	stopPing := make(chan struct{})
	defer close(stopPing)

	// Start ping goroutine
	go func() {
		for {
			select {
			case <-pingTicker.C:
				if err := client.Conn.WriteControl(websocket.PingMessage, []byte{}, time.Now().Add(10*time.Second)); err != nil {
					return
				}
			case <-stopPing:
				return
			}
		}
	}()

	for {
		_, message, err := client.Conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				s.logger.Error().Err(err).Str("clientId", client.ID).Msg("WebSocket error")
			}
			break
		}

		// Update activity
		s.clients.UpdateActivity(client.ID)

		// Handle message
		s.handleMessage(client, message)
	}
}

// handleMessage handles a single message from a client
func (s *Server) handleMessage(client *Client, message []byte) {
	// Try to parse as auth response first
	var authResp AuthResponse
	if err := json.Unmarshal(message, &authResp); err == nil && authResp.Method == "auth.response" {
		s.handleAuthMessage(client, authResp)
		return
	}

	// Check authentication
	if !client.IsAuthenticated() {
		s.sendError(client, "", AuthenticationRequired, "Authentication required")
		return
	}

	// Parse as RPC request
	req, err := s.router.ParseRequest(message)
	if err != nil {
		if rpcErr, ok := err.(*RPCError); ok {
			s.sendError(client, "", rpcErr.Code, rpcErr.Message)
		} else {
			s.sendError(client, "", ParseError, err.Error())
		}
		return
	}

	// Check rate limits
	allowed, reason := client.RateLimiter.CheckRequestAllowed()
	if !allowed {
		code := RateLimitExceeded
		if reason == "too many concurrent requests" {
			code = TooManyConcurrent
		}
		s.sendError(client, req.ID, code, reason)
		return
	}

	// Record request start
	client.RateLimiter.RecordRequestStart()
	s.inFlightReqs.Add(1)

	// Handle request asynchronously
	go func() {
		defer client.RateLimiter.RecordRequestEnd()
		defer s.inFlightReqs.Done()

		// Create context with client ID
		ctx := withClientID(context.Background(), client.ID)

		response := s.router.RouteRequest(ctx, req)
		if err := client.WriteJSON(response); err != nil {
			s.logger.Error().
				Err(err).
				Str("clientId", client.ID).
				Str("requestId", req.ID).
				Msg("Failed to send response")
		}
	}()
}

// handleRPC handles single-shot HTTP JSON-RPC requests.
func (s *Server) handleRPC(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if s.sharedSecret != "" {
		secret := r.Header.Get("X-Ranya-Secret")
		if secret != s.sharedSecret {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "failed to read request body", http.StatusBadRequest)
		return
	}

	req, err := s.router.ParseRequest(body)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(RPCResponse{
			ID:      "",
			JSONRPC: "2.0",
			Error: &RPCError{
				Code:    ParseError,
				Message: err.Error(),
			},
		})
		return
	}

	traceID := r.Header.Get("X-Trace-Id")
	if traceID == "" {
		traceID = tracing.NewTraceID()
	}
	ctx := tracing.WithTraceID(context.Background(), traceID)
	logger := tracing.LoggerFromContext(ctx, s.logger)
	logger.Info().
		Str("trace_id", traceID).
		Str("request_id", req.ID).
		Str("method", req.Method).
		Msg("Gateway received HTTP RPC request")

	resp := s.router.RouteRequest(ctx, req)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		logger.Error().Err(err).Msg("Failed to encode RPC response")
	}
}

// handleAuthMessage handles authentication messages
func (s *Server) handleAuthMessage(client *Client, authResp AuthResponse) {
	result := s.authHandler.HandleAuthResponse(client, authResp.Signature)

	if err := client.WriteJSON(result); err != nil {
		s.logger.Error().Err(err).Str("clientId", client.ID).Msg("Failed to send auth result")
		return
	}

	if !result.Success {
		s.logger.Warn().
			Str("clientId", client.ID).
			Str("reason", result.Message).
			Msg("Authentication failed")

		// Close connection after 3 failed attempts
		if client.AuthAttempts >= 3 {
			client.Conn.Close()
		}
	} else {
		s.logger.Info().Str("clientId", client.ID).Msg("Client authenticated")
	}
}

// sendError sends an error response to a client
func (s *Server) sendError(client *Client, requestID string, code int, message string) {
	response := RPCResponse{
		ID:      requestID,
		JSONRPC: "2.0",
		Error: &RPCError{
			Code:    code,
			Message: message,
		},
	}

	if err := client.WriteJSON(response); err != nil {
		s.logger.Error().
			Err(err).
			Str("clientId", client.ID).
			Msg("Failed to send error response")
	}
}

// Broadcast broadcasts an event to all authenticated clients
func (s *Server) Broadcast(event string, data interface{}) {
	s.broadcaster.Broadcast(event, data)
}

// BroadcastTyped broadcasts a typed stream event to authenticated clients.
func (s *Server) BroadcastTyped(msg EventMessage) {
	s.broadcaster.BroadcastTyped(msg)
}

// RegisterMethod registers an RPC method handler
func (s *Server) RegisterMethod(name string, handler RequestHandler) error {
	return s.router.RegisterMethod(name, handler)
}

// UnregisterMethod unregisters an RPC method handler
func (s *Server) UnregisterMethod(name string) {
	s.router.UnregisterMethod(name)
}

// handleSSE handles Server-Sent Events for streaming runtime events
func (s *Server) handleSSE(w http.ResponseWriter, r *http.Request) {
	// Authenticate
	secret := r.Header.Get("X-Ranya-Secret")
	if secret == "" {
		secret = r.URL.Query().Get("access_token")
	}

	if s.sharedSecret != "" && secret != s.sharedSecret {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	sessionKey := r.URL.Query().Get("session_key")
	if sessionKey == "" {
		http.Error(w, "session_key query parameter is required", http.StatusBadRequest)
		return
	}

	// Set headers for SSE
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	// Subscribe to events
	events, unsubscribe := s.runtimeEventHub.Subscribe(sessionKey, 64)
	defer unsubscribe()

	s.logger.Info().Str("session_key", sessionKey).Msg("SSE client subscribed")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
		return
	}

	// Send initial connection event
	_, _ = fmt.Fprintf(w, "event: connected\ndata: {\"session_key\":\"%s\"}\n\n", sessionKey)
	flusher.Flush()

	// Keep-alive ticker
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-r.Context().Done():
			s.logger.Info().Str("session_key", sessionKey).Msg("SSE client disconnected")
			return
		case <-ticker.C:
			// Send heartbeat
			_, _ = fmt.Fprintf(w, ": heartbeat\n\n")
			flusher.Flush()
		case evt, ok := <-events:
			if !ok {
				return
			}
			data, err := json.Marshal(evt)
			if err != nil {
				continue
			}
			_, _ = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", evt.Phase, string(data))
			flusher.Flush()
		}
	}
}

// handleAgentStream handles streaming agent responses via SSE.
func (s *Server) handleAgentStream(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	accept := r.Header.Get("Accept")
	if !strings.Contains(accept, "text/event-stream") {
		http.Error(w, "Accept: text/event-stream is required", http.StatusNotAcceptable)
		return
	}

	// Authenticate
	secret := r.Header.Get("X-Ranya-Secret")
	if secret == "" {
		secret = r.URL.Query().Get("access_token")
	}

	if s.sharedSecret != "" && secret != s.sharedSecret {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var params map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&params); err != nil {
		http.Error(w, "invalid JSON body", http.StatusBadRequest)
		return
	}

	prompt, ok := params["prompt"].(string)
	if !ok || strings.TrimSpace(prompt) == "" {
		http.Error(w, "prompt is required", http.StatusBadRequest)
		return
	}

	sessionKey, ok := params["sessionKey"].(string)
	if !ok || strings.TrimSpace(sessionKey) == "" {
		http.Error(w, "sessionKey is required", http.StatusBadRequest)
		return
	}

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
	if !config.Streaming {
		config.Streaming = true
	}

	cwd := ""
	if cwdParam, ok := params["cwd"].(string); ok {
		cwd = cwdParam
	}

	agentID := ""
	if requestedAgentID, ok := params["agentId"].(string); ok {
		agentID = strings.TrimSpace(requestedAgentID)
	}

	if s.runtimeEventHub == nil {
		http.Error(w, "runtime event hub is not available", http.StatusInternalServerError)
		return
	}

	// Set headers for SSE
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	ctx := tracing.NewRequestContext(r.Context())
	ctx = tracing.WithSessionKey(ctx, sessionKey)
	ctx = tracing.WithRunID(ctx, tracing.NewRunID())

	events, unsubscribe := s.runtimeEventHub.Subscribe(sessionKey, 64)
	defer unsubscribe()

	_, _ = fmt.Fprintf(w, "event: connected\ndata: {\"session_key\":\"%s\"}\n\n", sessionKey)
	flusher.Flush()

	resultCh := make(chan agent.AgentResult, 1)
	errCh := make(chan error, 1)

	go func() {
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
			errCh <- err
			return
		}
		resultCh <- result
	}()

	for {
		select {
		case <-r.Context().Done():
			return
		case evt, ok := <-events:
			if !ok {
				return
			}
			eventName, payload := runtimeEventToSSE(evt)
			if eventName == "" {
				continue
			}
			writeSSEEvent(w, flusher, eventName, payload)
		case err := <-errCh:
			writeSSEEvent(w, flusher, "error", map[string]interface{}{"error": err.Error()})
			return
		case result := <-resultCh:
			writeSSEEvent(w, flusher, "complete", result)
			return
		}
	}
}

func runtimeEventToSSE(evt agent.RuntimeEvent) (string, interface{}) {
	switch evt.Stream {
	case agent.RuntimeStreamAssistant:
		if evt.Phase == "output" && strings.TrimSpace(evt.Content) != "" {
			return "token", map[string]interface{}{
				"content": evt.Content,
				"phase":   evt.Phase,
			}
		}
	case agent.RuntimeStreamTool:
		return "tool_call", map[string]interface{}{
			"tool":      evt.ToolName,
			"tool_call": evt.ToolCall,
			"phase":     evt.Phase,
			"metadata":  evt.Metadata,
			"event":     evt.Event,
		}
	case agent.RuntimeStreamLifecycle:
		if evt.Phase == "error" {
			var errMsg interface{}
			if evt.Metadata != nil {
				errMsg = evt.Metadata["error"]
			}
			return "error", map[string]interface{}{
				"error":    errMsg,
				"metadata": evt.Metadata,
			}
		}
	}

	return "event", map[string]interface{}{
		"event":    evt.Event,
		"stream":   evt.Stream,
		"phase":    evt.Phase,
		"content":  evt.Content,
		"metadata": evt.Metadata,
	}
}

func writeSSEEvent(w http.ResponseWriter, flusher http.Flusher, name string, payload interface{}) {
	data, err := json.Marshal(payload)
	if err != nil {
		return
	}
	_, _ = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", name, string(data))
	flusher.Flush()
}

// GetConnectedClients returns information about all connected clients
func (s *Server) GetConnectedClients() []ClientInfo {
	return s.clients.GetConnectedClients()
}
