package webhook

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/harun/ranya/pkg/agent"
	"github.com/harun/ranya/pkg/commandqueue"
	"github.com/harun/ranya/pkg/gateway"
	"github.com/rs/zerolog"
)

// Server is the webhook HTTP server
type Server struct {
	options        ServerOptions
	server         *http.Server
	webhooks       map[string]*WebhookConfig // key: method:path
	rateLimiter    *RateLimiter
	metricsTracker *MetricsTracker
	commandQueue   *commandqueue.CommandQueue
	agentRunner    *agent.Runner
	gatewayServer  *gateway.Server
	logger         zerolog.Logger
	startTime      time.Time
	isShuttingDown bool
	shutdownMu     sync.RWMutex
	inFlightReqs   sync.WaitGroup
	webhooksMu     sync.RWMutex
}

// NewServer creates a new webhook server
func NewServer(options ServerOptions, commandQueue *commandqueue.CommandQueue, agentRunner *agent.Runner, gatewayServer *gateway.Server, logger zerolog.Logger) (*Server, error) {
	// Set defaults
	if options.Port == 0 {
		options.Port = 3001
	}
	if options.Host == "" {
		options.Host = "0.0.0.0"
	}
	if options.RateLimitPerMinute == 0 {
		options.RateLimitPerMinute = 100
	}
	if options.DefaultTimeout == 0 {
		options.DefaultTimeout = 30 * time.Second
	}
	if options.RegistryPath == "" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("failed to get home directory: %w", err)
		}
		options.RegistryPath = filepath.Join(homeDir, ".openclaw", "webhooks", "registry.json")
	}

	if commandQueue == nil {
		return nil, fmt.Errorf("command queue is required")
	}
	if agentRunner == nil {
		return nil, fmt.Errorf("agent runner is required")
	}

	s := &Server{
		options:        options,
		webhooks:       make(map[string]*WebhookConfig),
		rateLimiter:    NewRateLimiter(options.RateLimitPerMinute),
		metricsTracker: NewMetricsTracker(),
		commandQueue:   commandQueue,
		agentRunner:    agentRunner,
		gatewayServer:  gatewayServer,
		logger:         logger,
		startTime:      time.Now(),
	}

	// Load registry
	s.loadRegistry()

	return s, nil
}

// Start starts the webhook server
func (s *Server) Start() error {
	mux := http.NewServeMux()

	// Health check endpoint
	mux.HandleFunc("/health", s.handleHealth)

	// Webhook handler (catch-all)
	mux.HandleFunc("/", s.handleWebhook)

	s.server = &http.Server{
		Addr:    fmt.Sprintf("%s:%d", s.options.Host, s.options.Port),
		Handler: mux,
	}

	s.logger.Info().
		Str("host", s.options.Host).
		Int("port", s.options.Port).
		Msg("Starting webhook server")

	if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("failed to start webhook server: %w", err)
	}

	return nil
}

// Stop gracefully stops the webhook server
func (s *Server) Stop() error {
	s.shutdownMu.Lock()
	s.isShuttingDown = true
	s.shutdownMu.Unlock()

	s.logger.Info().Msg("Shutting down webhook server")

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

	// Stop rate limiter cleanup
	s.rateLimiter.Stop()

	// Shutdown HTTP server
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := s.server.Shutdown(ctx); err != nil {
		return fmt.Errorf("failed to shutdown webhook server: %w", err)
	}

	s.logger.Info().Msg("Webhook server stopped")
	return nil
}

// handleHealth handles health check requests
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	uptime := time.Since(s.startTime).Seconds()
	webhookCount := len(s.webhooks)

	response := map[string]interface{}{
		"status":       "ok",
		"uptime":       uptime,
		"webhookCount": webhookCount,
		"timestamp":    time.Now().UnixMilli(),
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}

// handleWebhook handles webhook requests
func (s *Server) handleWebhook(w http.ResponseWriter, r *http.Request) {
	startTime := time.Now()

	// Check if shutting down
	s.shutdownMu.RLock()
	if s.isShuttingDown {
		s.shutdownMu.RUnlock()
		http.Error(w, "Server is shutting down", http.StatusServiceUnavailable)
		return
	}
	s.shutdownMu.RUnlock()

	// Track in-flight request
	s.inFlightReqs.Add(1)
	defer s.inFlightReqs.Done()

	// Get client IP
	ip := s.getClientIP(r)

	// Check rate limit
	if !s.rateLimiter.CheckLimit(ip) {
		retryAfter := s.rateLimiter.GetRetryAfter(ip)
		s.logger.Warn().
			Str("ip", ip).
			Str("path", r.URL.Path).
			Int("retryAfter", retryAfter).
			Msg("Rate limit exceeded")

		w.Header().Set("Retry-After", fmt.Sprintf("%d", retryAfter))
		http.Error(w, "Too Many Requests", http.StatusTooManyRequests)
		return
	}

	// Read and store raw body for signature verification
	rawBody, err := io.ReadAll(r.Body)
	if err != nil {
		s.logger.Error().Err(err).Str("path", r.URL.Path).Msg("Failed to read request body")
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}
	r.Body = io.NopCloser(bytes.NewBuffer(rawBody))

	// Find webhook
	webhook := s.getWebhook(r.URL.Path, r.Method)
	if webhook == nil {
		s.logger.Debug().
			Str("path", r.URL.Path).
			Str("method", r.Method).
			Msg("Webhook not found")
		http.Error(w, "Not Found", http.StatusNotFound)
		return
	}

	// Verify signature if secret is configured
	if webhook.Secret != "" {
		signatureHeader := webhook.SignatureHeader
		if signatureHeader == "" {
			signatureHeader = "X-Webhook-Signature"
		}

		signature := r.Header.Get(signatureHeader)
		if signature == "" {
			s.logger.Warn().
				Str("path", webhook.Path).
				Str("ip", ip).
				Msg("Missing webhook signature")
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		algorithm := webhook.SignatureAlgorithm
		if algorithm == "" {
			algorithm = "sha256"
		}

		if !verifySignature(string(rawBody), signature, webhook.Secret, algorithm) {
			s.logger.Warn().
				Str("path", webhook.Path).
				Str("ip", ip).
				Msg("Invalid webhook signature")
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
	}

	// Parse request
	params, err := s.parseRequest(r, rawBody)
	if err != nil {
		s.logger.Error().
			Err(err).
			Str("path", webhook.Path).
			Msg("Failed to parse request")
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}

	// Execute handler with timeout
	timeout := webhook.Timeout
	if timeout == 0 {
		timeout = s.options.DefaultTimeout
	}

	response, err := s.executeHandler(webhook.Handler, params, timeout)

	duration := time.Since(startTime).Milliseconds()
	success := err == nil

	// Track metrics
	s.metricsTracker.Track(webhook.Path, webhook.Method, success, float64(duration))

	// Log request
	if success {
		s.logger.Info().
			Str("method", r.Method).
			Str("path", r.URL.Path).
			Str("ip", ip).
			Int64("duration", duration).
			Int("status", response.Status).
			Msg("Webhook request completed")
	} else {
		s.logger.Error().
			Err(err).
			Str("method", r.Method).
			Str("path", r.URL.Path).
			Str("ip", ip).
			Int64("duration", duration).
			Msg("Webhook request failed")
	}

	// Send response
	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	s.sendResponse(w, response)

	// Broadcast event to gateway (async, non-blocking)
	go s.broadcastEvent(webhook.Path, r.Method, params)

	// Enqueue to command queue (async, non-blocking)
	go s.enqueueSystemEvent(webhook.Path, r.Method, params)
}

// parseRequest parses the webhook request
func (s *Server) parseRequest(r *http.Request, rawBody []byte) (WebhookParams, error) {
	params := WebhookParams{
		Headers: make(map[string]string),
		Query:   make(map[string]string),
	}

	// Extract headers
	for key, values := range r.Header {
		if len(values) > 0 {
			params.Headers[key] = values[0]
		}
	}

	// Extract query parameters
	for key, values := range r.URL.Query() {
		if len(values) > 0 {
			params.Query[key] = values[0]
		}
	}

	// Parse body based on Content-Type
	contentType := r.Header.Get("Content-Type")

	if strings.Contains(contentType, "application/json") {
		var body interface{}
		if len(rawBody) > 0 {
			if err := json.Unmarshal(rawBody, &body); err != nil {
				return params, fmt.Errorf("failed to parse JSON body: %w", err)
			}
		}
		params.Body = body
	} else if strings.Contains(contentType, "application/x-www-form-urlencoded") {
		// Parse form data
		if err := r.ParseForm(); err != nil {
			return params, fmt.Errorf("failed to parse form data: %w", err)
		}
		formData := make(map[string]string)
		for key, values := range r.Form {
			if len(values) > 0 {
				formData[key] = values[0]
			}
		}
		params.Body = formData
	} else {
		// Raw body as string
		params.Body = string(rawBody)
	}

	return params, nil
}

// executeHandler executes a webhook handler with timeout
func (s *Server) executeHandler(handler WebhookHandler, params WebhookParams, timeout time.Duration) (WebhookResponse, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	resultChan := make(chan struct {
		response WebhookResponse
		err      error
	}, 1)

	go func() {
		response, err := handler(params)
		resultChan <- struct {
			response WebhookResponse
			err      error
		}{response, err}
	}()

	select {
	case result := <-resultChan:
		return result.response, result.err
	case <-ctx.Done():
		s.logger.Error().
			Dur("timeout", timeout).
			Msg("Webhook handler timed out")
		return WebhookResponse{
			Status: http.StatusGatewayTimeout,
			Body:   map[string]string{"error": "Gateway Timeout"},
		}, nil
	}
}

// sendResponse sends the webhook response
func (s *Server) sendResponse(w http.ResponseWriter, response WebhookResponse) {
	// Set custom headers
	for key, value := range response.Headers {
		w.Header().Set(key, value)
	}

	// Set content type if body is present
	if response.Body != nil {
		w.Header().Set("Content-Type", "application/json")
	}

	// Set status code
	w.WriteHeader(response.Status)

	// Write body
	if response.Body != nil {
		json.NewEncoder(w).Encode(response.Body)
	}
}

// broadcastEvent broadcasts webhook event to gateway
func (s *Server) broadcastEvent(path string, method string, params WebhookParams) {
	if s.gatewayServer == nil {
		return
	}

	defer func() {
		if r := recover(); r != nil {
			s.logger.Error().
				Interface("panic", r).
				Msg("Panic in broadcast event")
		}
	}()

	event := map[string]interface{}{
		"path":      path,
		"method":    method,
		"timestamp": time.Now().UnixMilli(),
		"headers":   params.Headers,
		"query":     params.Query,
	}

	s.gatewayServer.Broadcast("webhook.received", event)
}

// enqueueSystemEvent enqueues webhook event to command queue
func (s *Server) enqueueSystemEvent(path string, method string, params WebhookParams) {
	defer func() {
		if r := recover(); r != nil {
			s.logger.Error().
				Interface("panic", r).
				Msg("Panic in enqueue system event")
		}
	}()

	_, err := s.commandQueue.Enqueue("webhook", func(ctx context.Context) (interface{}, error) {
		s.logger.Debug().
			Str("path", path).
			Str("method", method).
			Msg("Processing webhook event")
		return nil, nil
	}, nil)

	if err != nil {
		s.logger.Error().
			Err(err).
			Str("path", path).
			Str("method", method).
			Msg("Failed to enqueue webhook event")
	}
}

// getClientIP extracts the client IP from the request
func (s *Server) getClientIP(r *http.Request) string {
	// Check X-Forwarded-For header
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		ips := strings.Split(xff, ",")
		if len(ips) > 0 {
			return strings.TrimSpace(ips[0])
		}
	}

	// Check X-Real-IP header
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return xri
	}

	// Use RemoteAddr
	ip := r.RemoteAddr
	if idx := strings.LastIndex(ip, ":"); idx != -1 {
		ip = ip[:idx]
	}
	return ip
}

// getWebhook retrieves a webhook by path and method
func (s *Server) getWebhook(path string, method string) *WebhookConfig {
	s.webhooksMu.RLock()
	defer s.webhooksMu.RUnlock()

	key := method + ":" + path
	return s.webhooks[key]
}

// RegisterWebhook registers a new webhook endpoint
func (s *Server) RegisterWebhook(config WebhookConfig) error {
	// Validate path
	if !strings.HasPrefix(config.Path, "/") {
		return fmt.Errorf("webhook path must start with /")
	}

	// Validate method
	validMethods := map[string]bool{
		http.MethodPost:   true,
		http.MethodGet:    true,
		http.MethodPut:    true,
		http.MethodDelete: true,
	}
	if !validMethods[config.Method] {
		return fmt.Errorf("invalid HTTP method: %s", config.Method)
	}

	// Validate handler
	if config.Handler == nil {
		return fmt.Errorf("webhook handler is required")
	}

	s.webhooksMu.Lock()
	key := config.Method + ":" + config.Path
	s.webhooks[key] = &config
	s.webhooksMu.Unlock()

	s.logger.Info().
		Str("path", config.Path).
		Str("method", config.Method).
		Msg("Webhook registered")

	// Save registry
	go s.saveRegistry()

	return nil
}

// UnregisterWebhook removes a webhook endpoint
func (s *Server) UnregisterWebhook(path string, method string) bool {
	s.webhooksMu.Lock()
	key := method + ":" + path
	_, exists := s.webhooks[key]
	if exists {
		delete(s.webhooks, key)
	}
	s.webhooksMu.Unlock()

	if exists {
		s.logger.Info().
			Str("path", path).
			Str("method", method).
			Msg("Webhook unregistered")

		// Save registry
		go s.saveRegistry()
	}

	return exists
}

// ListWebhooks returns all registered webhooks
func (s *Server) ListWebhooks() []WebhookRegistryEntry {
	s.webhooksMu.RLock()
	defer s.webhooksMu.RUnlock()

	entries := make([]WebhookRegistryEntry, 0, len(s.webhooks))
	for _, webhook := range s.webhooks {
		entry := WebhookRegistryEntry{
			Path:               webhook.Path,
			Method:             webhook.Method,
			Secret:             "[REDACTED]", // Don't expose secrets
			SignatureHeader:    webhook.SignatureHeader,
			SignatureAlgorithm: webhook.SignatureAlgorithm,
			Timeout:            webhook.Timeout.Milliseconds(),
			Description:        webhook.Description,
		}
		if webhook.Secret == "" {
			entry.Secret = ""
		}
		entries = append(entries, entry)
	}

	return entries
}

// GetMetrics returns all webhook metrics
func (s *Server) GetMetrics() []WebhookMetrics {
	return s.metricsTracker.GetMetrics()
}

// GetMetricsForWebhook returns metrics for a specific webhook
func (s *Server) GetMetricsForWebhook(path string, method string) *WebhookMetrics {
	return s.metricsTracker.GetMetricsForWebhook(path, method)
}

// loadRegistry loads webhook configurations from disk
func (s *Server) loadRegistry() {
	data, err := os.ReadFile(s.options.RegistryPath)
	if err != nil {
		if os.IsNotExist(err) {
			s.logger.Info().Msg("No existing webhook registry, starting with empty registry")
		} else {
			s.logger.Error().
				Err(err).
				Msg("Failed to load webhook registry, starting with empty registry")
		}
		return
	}

	var registry WebhookRegistry
	if err := json.Unmarshal(data, &registry); err != nil {
		s.logger.Error().
			Err(err).
			Msg("Failed to parse webhook registry, starting with empty registry")
		return
	}

	// Note: Handlers must be re-registered after loading
	for _, entry := range registry.Webhooks {
		s.logger.Info().
			Str("path", entry.Path).
			Str("method", entry.Method).
			Msg("Loaded webhook from registry (handler must be re-registered)")
	}
}

// saveRegistry saves webhook configurations to disk
func (s *Server) saveRegistry() {
	s.webhooksMu.RLock()
	entries := make([]WebhookRegistryEntry, 0, len(s.webhooks))
	for _, webhook := range s.webhooks {
		entry := WebhookRegistryEntry{
			Path:               webhook.Path,
			Method:             webhook.Method,
			Secret:             webhook.Secret,
			SignatureHeader:    webhook.SignatureHeader,
			SignatureAlgorithm: webhook.SignatureAlgorithm,
			Timeout:            webhook.Timeout.Milliseconds(),
			Description:        webhook.Description,
		}
		entries = append(entries, entry)
	}
	s.webhooksMu.RUnlock()

	registry := WebhookRegistry{
		Version:     1,
		Webhooks:    entries,
		LastUpdated: time.Now().UnixMilli(),
	}

	// Ensure directory exists
	dir := filepath.Dir(s.options.RegistryPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		s.logger.Error().
			Err(err).
			Str("dir", dir).
			Msg("Failed to create registry directory")
		return
	}

	// Write to temp file first (atomic write)
	tempFile := s.options.RegistryPath + ".tmp"
	data, err := json.MarshalIndent(registry, "", "  ")
	if err != nil {
		s.logger.Error().
			Err(err).
			Msg("Failed to marshal webhook registry")
		return
	}

	if err := os.WriteFile(tempFile, data, 0644); err != nil {
		s.logger.Error().
			Err(err).
			Str("file", tempFile).
			Msg("Failed to write webhook registry temp file")
		return
	}

	// Rename temp file to actual file (atomic operation)
	if err := os.Rename(tempFile, s.options.RegistryPath); err != nil {
		s.logger.Error().
			Err(err).
			Str("file", s.options.RegistryPath).
			Msg("Failed to rename webhook registry file")
		return
	}

	s.logger.Debug().Msg("Saved webhook registry")
}
