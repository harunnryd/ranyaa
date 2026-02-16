package webhook

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/harun/ranya/pkg/agent"
	"github.com/harun/ranya/pkg/commandqueue"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func createTestServer(t *testing.T) *Server {
	logger := zerolog.New(os.Stdout).Level(zerolog.Disabled)
	cq := commandqueue.New()

	// Create temp directory for registry
	tempDir := t.TempDir()
	registryPath := filepath.Join(tempDir, "registry.json")

	options := ServerOptions{
		Port:               3001,
		Host:               "0.0.0.0",
		RegistryPath:       registryPath,
		RateLimitPerMinute: 100,
		DefaultTimeout:     30 * time.Second,
	}

	// Create mock agent runner (minimal implementation)
	runner := &agent.Runner{}

	server, err := NewServer(options, cq, runner, nil, logger)
	require.NoError(t, err)

	return server
}

func TestNewServer(t *testing.T) {
	server := createTestServer(t)
	assert.NotNil(t, server)
	assert.NotNil(t, server.webhooks)
	assert.NotNil(t, server.rateLimiter)
	assert.NotNil(t, server.metricsTracker)
}

func TestNewServerDefaults(t *testing.T) {
	logger := zerolog.New(os.Stdout).Level(zerolog.Disabled)
	cq := commandqueue.New()
	runner := &agent.Runner{}

	options := ServerOptions{}
	server, err := NewServer(options, cq, runner, nil, logger)
	require.NoError(t, err)

	assert.Equal(t, 3001, server.options.Port)
	assert.Equal(t, "0.0.0.0", server.options.Host)
	assert.Equal(t, 100, server.options.RateLimitPerMinute)
	assert.Equal(t, 30*time.Second, server.options.DefaultTimeout)
}

func TestNewServerRequiredDependencies(t *testing.T) {
	logger := zerolog.New(os.Stdout).Level(zerolog.Disabled)
	options := ServerOptions{}

	// Missing command queue
	_, err := NewServer(options, nil, &agent.Runner{}, nil, logger)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "command queue is required")

	// Missing agent runner
	cq := commandqueue.New()
	_, err = NewServer(options, cq, nil, nil, logger)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "agent runner is required")
}

func TestRegisterWebhook(t *testing.T) {
	server := createTestServer(t)

	config := WebhookConfig{
		Path:   "/webhook/test",
		Method: http.MethodPost,
		Handler: func(params WebhookParams) (WebhookResponse, error) {
			return WebhookResponse{Status: http.StatusOK}, nil
		},
	}

	err := server.RegisterWebhook(config)
	assert.NoError(t, err)

	// Verify webhook is registered
	webhook := server.getWebhook("/webhook/test", http.MethodPost)
	assert.NotNil(t, webhook)
	assert.Equal(t, "/webhook/test", webhook.Path)
	assert.Equal(t, http.MethodPost, webhook.Method)
}

func TestRegisterWebhookInvalidPath(t *testing.T) {
	server := createTestServer(t)

	config := WebhookConfig{
		Path:   "invalid-path", // Missing leading slash
		Method: http.MethodPost,
		Handler: func(params WebhookParams) (WebhookResponse, error) {
			return WebhookResponse{Status: http.StatusOK}, nil
		},
	}

	err := server.RegisterWebhook(config)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "must start with /")
}

func TestRegisterWebhookInvalidMethod(t *testing.T) {
	server := createTestServer(t)

	config := WebhookConfig{
		Path:   "/webhook/test",
		Method: "INVALID",
		Handler: func(params WebhookParams) (WebhookResponse, error) {
			return WebhookResponse{Status: http.StatusOK}, nil
		},
	}

	err := server.RegisterWebhook(config)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid HTTP method")
}

func TestRegisterWebhookMissingHandler(t *testing.T) {
	server := createTestServer(t)

	config := WebhookConfig{
		Path:    "/webhook/test",
		Method:  http.MethodPost,
		Handler: nil,
	}

	err := server.RegisterWebhook(config)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "handler is required")
}

func TestUnregisterWebhook(t *testing.T) {
	server := createTestServer(t)

	// Register webhook
	config := WebhookConfig{
		Path:   "/webhook/test",
		Method: http.MethodPost,
		Handler: func(params WebhookParams) (WebhookResponse, error) {
			return WebhookResponse{Status: http.StatusOK}, nil
		},
	}
	server.RegisterWebhook(config)

	// Unregister webhook
	removed := server.UnregisterWebhook("/webhook/test", http.MethodPost)
	assert.True(t, removed)

	// Verify webhook is removed
	webhook := server.getWebhook("/webhook/test", http.MethodPost)
	assert.Nil(t, webhook)
}

func TestUnregisterWebhookNotFound(t *testing.T) {
	server := createTestServer(t)

	// Unregister non-existent webhook
	removed := server.UnregisterWebhook("/webhook/notfound", http.MethodPost)
	assert.False(t, removed)
}

func TestListWebhooks(t *testing.T) {
	server := createTestServer(t)

	// Register multiple webhooks
	configs := []WebhookConfig{
		{
			Path:   "/webhook/test1",
			Method: http.MethodPost,
			Handler: func(params WebhookParams) (WebhookResponse, error) {
				return WebhookResponse{Status: http.StatusOK}, nil
			},
		},
		{
			Path:   "/webhook/test2",
			Method: http.MethodGet,
			Handler: func(params WebhookParams) (WebhookResponse, error) {
				return WebhookResponse{Status: http.StatusOK}, nil
			},
		},
	}

	for _, config := range configs {
		server.RegisterWebhook(config)
	}

	// List webhooks
	webhooks := server.ListWebhooks()
	assert.Len(t, webhooks, 2)
}

func TestListWebhooksRedactsSecrets(t *testing.T) {
	server := createTestServer(t)

	// Register webhook with secret
	config := WebhookConfig{
		Path:   "/webhook/test",
		Method: http.MethodPost,
		Secret: "my-secret-key",
		Handler: func(params WebhookParams) (WebhookResponse, error) {
			return WebhookResponse{Status: http.StatusOK}, nil
		},
	}
	server.RegisterWebhook(config)

	// List webhooks
	webhooks := server.ListWebhooks()
	assert.Len(t, webhooks, 1)
	assert.Equal(t, "[REDACTED]", webhooks[0].Secret)
}

func TestHandleHealth(t *testing.T) {
	server := createTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()

	server.handleHealth(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "application/json", w.Header().Get("Content-Type"))

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Equal(t, "ok", response["status"])
	assert.NotNil(t, response["uptime"])
	assert.NotNil(t, response["webhookCount"])
}

func TestHandleHealthInvalidMethod(t *testing.T) {
	server := createTestServer(t)

	req := httptest.NewRequest(http.MethodPost, "/health", nil)
	w := httptest.NewRecorder()

	server.handleHealth(w, req)

	assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
}

func TestHandleWebhookSuccess(t *testing.T) {
	server := createTestServer(t)

	// Register webhook
	called := false
	config := WebhookConfig{
		Path:   "/webhook/test",
		Method: http.MethodPost,
		Handler: func(params WebhookParams) (WebhookResponse, error) {
			called = true
			return WebhookResponse{
				Status: http.StatusOK,
				Body:   map[string]string{"message": "success"},
			}, nil
		},
	}
	server.RegisterWebhook(config)

	// Make request
	body := map[string]string{"test": "data"}
	bodyBytes, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/webhook/test", bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	server.handleWebhook(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.True(t, called)

	var response map[string]string
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Equal(t, "success", response["message"])
}

func TestHandleWebhookNotFound(t *testing.T) {
	server := createTestServer(t)

	req := httptest.NewRequest(http.MethodPost, "/webhook/notfound", nil)
	w := httptest.NewRecorder()

	server.handleWebhook(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestHandleWebhookSignatureVerification(t *testing.T) {
	server := createTestServer(t)

	// Register webhook with secret
	called := false
	config := WebhookConfig{
		Path:               "/webhook/test",
		Method:             http.MethodPost,
		Secret:             "my-secret",
		SignatureHeader:    "X-Webhook-Signature",
		SignatureAlgorithm: "sha256",
		Handler: func(params WebhookParams) (WebhookResponse, error) {
			called = true
			return WebhookResponse{Status: http.StatusOK}, nil
		},
	}
	server.RegisterWebhook(config)

	// Make request with valid signature
	body := `{"test":"data"}`
	signature := computeHMACSHA256(body, "my-secret")
	req := httptest.NewRequest(http.MethodPost, "/webhook/test", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Webhook-Signature", signature)
	w := httptest.NewRecorder()

	server.handleWebhook(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.True(t, called)

	// Wait for async operations to complete
	time.Sleep(10 * time.Millisecond)
}

func TestHandleWebhookInvalidSignature(t *testing.T) {
	server := createTestServer(t)

	// Register webhook with secret
	config := WebhookConfig{
		Path:               "/webhook/test",
		Method:             http.MethodPost,
		Secret:             "my-secret",
		SignatureHeader:    "X-Webhook-Signature",
		SignatureAlgorithm: "sha256",
		Handler: func(params WebhookParams) (WebhookResponse, error) {
			return WebhookResponse{Status: http.StatusOK}, nil
		},
	}
	server.RegisterWebhook(config)

	// Make request with invalid signature
	body := `{"test":"data"}`
	req := httptest.NewRequest(http.MethodPost, "/webhook/test", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Webhook-Signature", "sha256=invalid")
	w := httptest.NewRecorder()

	server.handleWebhook(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestHandleWebhookMissingSignature(t *testing.T) {
	server := createTestServer(t)

	// Register webhook with secret
	config := WebhookConfig{
		Path:   "/webhook/test",
		Method: http.MethodPost,
		Secret: "my-secret",
		Handler: func(params WebhookParams) (WebhookResponse, error) {
			return WebhookResponse{Status: http.StatusOK}, nil
		},
	}
	server.RegisterWebhook(config)

	// Make request without signature
	body := `{"test":"data"}`
	req := httptest.NewRequest(http.MethodPost, "/webhook/test", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	server.handleWebhook(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestHandleWebhookRateLimit(t *testing.T) {
	logger := zerolog.New(os.Stdout).Level(zerolog.Disabled)
	cq := commandqueue.New()
	runner := &agent.Runner{}

	options := ServerOptions{
		RateLimitPerMinute: 2, // Very low limit for testing
	}

	server, err := NewServer(options, cq, runner, nil, logger)
	require.NoError(t, err)

	// Register webhook
	config := WebhookConfig{
		Path:   "/webhook/test",
		Method: http.MethodPost,
		Handler: func(params WebhookParams) (WebhookResponse, error) {
			return WebhookResponse{Status: http.StatusOK}, nil
		},
	}
	server.RegisterWebhook(config)

	// Make requests until rate limited
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest(http.MethodPost, "/webhook/test", nil)
		req.RemoteAddr = "192.168.1.1:12345"
		w := httptest.NewRecorder()
		server.handleWebhook(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
	}

	// Next request should be rate limited
	req := httptest.NewRequest(http.MethodPost, "/webhook/test", nil)
	req.RemoteAddr = "192.168.1.1:12345"
	w := httptest.NewRecorder()
	server.handleWebhook(w, req)
	assert.Equal(t, http.StatusTooManyRequests, w.Code)
	assert.NotEmpty(t, w.Header().Get("Retry-After"))
}

func TestGetClientIP(t *testing.T) {
	server := createTestServer(t)

	// Test X-Forwarded-For
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Forwarded-For", "192.168.1.1, 10.0.0.1")
	ip := server.getClientIP(req)
	assert.Equal(t, "192.168.1.1", ip)

	// Test X-Real-IP
	req = httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Real-IP", "192.168.1.2")
	ip = server.getClientIP(req)
	assert.Equal(t, "192.168.1.2", ip)

	// Test RemoteAddr
	req = httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "192.168.1.3:12345"
	ip = server.getClientIP(req)
	assert.Equal(t, "192.168.1.3", ip)
}

func TestParseRequestJSON(t *testing.T) {
	server := createTestServer(t)

	body := map[string]interface{}{
		"key": "value",
		"num": 123,
	}
	bodyBytes, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/webhook/test", bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Custom-Header", "custom-value")
	req.URL.RawQuery = "param1=value1&param2=value2"

	params, err := server.parseRequest(req, bodyBytes)
	assert.NoError(t, err)
	assert.NotNil(t, params.Body)
	assert.Equal(t, "custom-value", params.Headers["X-Custom-Header"])
	assert.Equal(t, "value1", params.Query["param1"])
	assert.Equal(t, "value2", params.Query["param2"])
}

func TestParseRequestFormData(t *testing.T) {
	server := createTestServer(t)

	body := "key1=value1&key2=value2"
	req := httptest.NewRequest(http.MethodPost, "/webhook/test", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	params, err := server.parseRequest(req, []byte(body))
	assert.NoError(t, err)
	assert.NotNil(t, params.Body)

	formData, ok := params.Body.(map[string]string)
	assert.True(t, ok)
	assert.Equal(t, "value1", formData["key1"])
	assert.Equal(t, "value2", formData["key2"])
}

func TestExecuteHandlerTimeout(t *testing.T) {
	server := createTestServer(t)

	// Handler that takes too long
	handler := func(params WebhookParams) (WebhookResponse, error) {
		time.Sleep(2 * time.Second)
		return WebhookResponse{Status: http.StatusOK}, nil
	}

	params := WebhookParams{}
	response, err := server.executeHandler(handler, params, 100*time.Millisecond)

	assert.NoError(t, err)
	assert.Equal(t, http.StatusGatewayTimeout, response.Status)
}

func TestGetMetrics(t *testing.T) {
	server := createTestServer(t)

	// Register and call webhook
	config := WebhookConfig{
		Path:   "/webhook/test",
		Method: http.MethodPost,
		Handler: func(params WebhookParams) (WebhookResponse, error) {
			return WebhookResponse{Status: http.StatusOK}, nil
		},
	}
	server.RegisterWebhook(config)

	req := httptest.NewRequest(http.MethodPost, "/webhook/test", nil)
	w := httptest.NewRecorder()
	server.handleWebhook(w, req)

	// Get metrics
	metrics := server.GetMetrics()
	assert.Len(t, metrics, 1)
	assert.Equal(t, "/webhook/test", metrics[0].Path)
	assert.Equal(t, "POST", metrics[0].Method)
	assert.Equal(t, int64(1), metrics[0].TotalRequests)
}
