package gateway

import (
	"time"

	"github.com/gorilla/websocket"
)

// StreamType identifies typed runtime streams delivered to gateway clients.
type StreamType string

const (
	StreamTypeTool      StreamType = "tool"
	StreamTypeAssistant StreamType = "assistant"
	StreamTypeLifecycle StreamType = "lifecycle"
)

// RPCRequest represents a JSON-RPC 2.0 request
type RPCRequest struct {
	ID             string                 `json:"id"`
	Method         string                 `json:"method"`
	Params         map[string]interface{} `json:"params,omitempty"`
	JSONRPC        string                 `json:"jsonrpc"`
	IdempotencyKey string                 `json:"idempotencyKey,omitempty"`
}

// RPCResponse represents a JSON-RPC 2.0 response
type RPCResponse struct {
	ID      string      `json:"id"`
	Result  interface{} `json:"result,omitempty"`
	Error   *RPCError   `json:"error,omitempty"`
	JSONRPC string      `json:"jsonrpc"`
}

// RPCError represents a JSON-RPC 2.0 error
type RPCError struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

// Error implements the error interface
func (e *RPCError) Error() string {
	return e.Message
}

// EventMessage represents a server-initiated event
type EventMessage struct {
	Type      string      `json:"type,omitempty"`
	Event     string      `json:"event"`
	Stream    StreamType  `json:"stream,omitempty"`
	Phase     string      `json:"phase,omitempty"`
	Seq       int64       `json:"seq,omitempty"`
	Data      interface{} `json:"data"`
	Timestamp int64       `json:"timestamp"`
	TraceID   string      `json:"trace_id,omitempty"`
	RunID     string      `json:"run_id,omitempty"`
	Session   string      `json:"session_key,omitempty"`
	AgentID   string      `json:"agent_id,omitempty"`
}

// AuthChallenge represents an authentication challenge message
type AuthChallenge struct {
	Event     string `json:"event"`
	Challenge string `json:"challenge"`
}

// AuthResponse represents a client's authentication response
type AuthResponse struct {
	Method    string `json:"method"`
	Signature string `json:"signature"`
}

// AuthResult represents the result of authentication
type AuthResult struct {
	Event   string `json:"event"`
	Success bool   `json:"success,omitempty"`
	Message string `json:"message,omitempty"`
}

// ClientInfo represents information about a connected client
type ClientInfo struct {
	ID            string    `json:"id"`
	Authenticated bool      `json:"authenticated"`
	ConnectedAt   time.Time `json:"connectedAt"`
	LastActivity  time.Time `json:"lastActivity"`
	IPAddress     string    `json:"ipAddress"`
	Idle          bool      `json:"idle"`
}

// ClientState represents the state of a client connection
type ClientState int

const (
	StateConnecting ClientState = iota
	StateAuthenticating
	StateAuthenticated
	StateDisconnected
)

// RequestHandler is a function that handles RPC requests
type RequestHandler func(params map[string]interface{}) (interface{}, error)

// RPC error codes
const (
	ParseError             = -32700
	InvalidRequest         = -32600
	MethodNotFound         = -32601
	InvalidParams          = -32602
	InternalError          = -32603
	AuthenticationRequired = -32001
	RateLimitExceeded      = -32005
	TooManyConcurrent      = -32006
)

// Client represents a connected WebSocket client
type Client struct {
	ID            string
	Conn          *websocket.Conn
	Authenticated bool
	Challenge     string
	ConnectedAt   time.Time
	LastActivity  time.Time
	IPAddress     string
	AuthAttempts  int
	RateLimiter   *ClientRateLimiter
	State         ClientState
}
