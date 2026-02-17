package webhook

import (
	"time"
)

// WebhookConfig defines a webhook endpoint configuration
type WebhookConfig struct {
	Path               string         // URL path (e.g., "/webhook/github")
	Method             string         // HTTP method (POST, GET, PUT, DELETE)
	Handler            WebhookHandler // Processing function
	Secret             string         // Signature verification secret
	SignatureHeader    string         // Header for signature (e.g., "X-Hub-Signature-256")
	SignatureAlgorithm string         // Signature algorithm ("sha256" or "sha1")
	Timeout            time.Duration  // Handler timeout (default: 30s)
	Description        string         // Human-readable description
}

// WebhookHandler is a function that processes webhook requests
type WebhookHandler func(params WebhookParams) (WebhookResponse, error)

// WebhookParams contains parsed webhook request data
type WebhookParams struct {
	Body    interface{}       // Parsed request body
	Headers map[string]string // Request headers
	Query   map[string]string // Query parameters
}

// WebhookResponse defines the webhook response
type WebhookResponse struct {
	Status  int               // HTTP status code
	Body    interface{}       // Response body (will be JSON serialized)
	Headers map[string]string // Custom response headers
}

// WebhookRegistryEntry represents a persisted webhook configuration
type WebhookRegistryEntry struct {
	Path               string `json:"path"`
	Method             string `json:"method"`
	Secret             string `json:"secret,omitempty"`
	SignatureHeader    string `json:"signatureHeader,omitempty"`
	SignatureAlgorithm string `json:"signatureAlgorithm,omitempty"`
	Timeout            int64  `json:"timeout,omitempty"` // milliseconds
	Description        string `json:"description,omitempty"`
}

// WebhookRegistry is the persisted registry structure
type WebhookRegistry struct {
	Version     int                    `json:"version"`
	Webhooks    []WebhookRegistryEntry `json:"webhooks"`
	LastUpdated int64                  `json:"lastUpdated"`
}

// WebhookMetrics tracks webhook performance metrics
type WebhookMetrics struct {
	Path                string  `json:"path"`
	Method              string  `json:"method"`
	TotalRequests       int64   `json:"totalRequests"`
	SuccessCount        int64   `json:"successCount"`
	FailureCount        int64   `json:"failureCount"`
	AverageResponseTime float64 `json:"averageResponseTime"` // milliseconds
	LastRequestAt       int64   `json:"lastRequestAt,omitempty"`
}

// RateLimitState tracks rate limiting per IP
type RateLimitState struct {
	Requests    []int64 // Timestamps of requests
	WindowStart int64   // Start of current window
}

// ServerOptions configures the webhook server
type ServerOptions struct {
	Port               int           // Server port (default: 3001)
	Host               string        // Server host (default: "0.0.0.0")
	RegistryPath       string        // Path to webhook registry file
	RateLimitPerMinute int           // Requests per minute per IP (default: 100)
	DefaultTimeout     time.Duration // Default handler timeout (default: 30s)
}
