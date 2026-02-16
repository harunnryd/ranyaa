package gateway

import (
	"encoding/json"
	"fmt"
	"sync"
)

// RPCRouter handles RPC method registration and request routing
type RPCRouter struct {
	mu      sync.RWMutex
	methods map[string]RequestHandler
}

// NewRPCRouter creates a new RPC router
func NewRPCRouter() *RPCRouter {
	return &RPCRouter{
		methods: make(map[string]RequestHandler),
	}
}

// RegisterMethod registers an RPC method handler
func (r *RPCRouter) RegisterMethod(name string, handler RequestHandler) error {
	if handler == nil {
		return fmt.Errorf("handler cannot be nil")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	r.methods[name] = handler
	return nil
}

// UnregisterMethod removes an RPC method handler
func (r *RPCRouter) UnregisterMethod(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.methods, name)
}

// ParseRequest parses and validates a JSON-RPC request
func (r *RPCRouter) ParseRequest(data []byte) (*RPCRequest, error) {
	var req RPCRequest
	if err := json.Unmarshal(data, &req); err != nil {
		return nil, &RPCError{
			Code:    ParseError,
			Message: "Parse error",
			Data:    err.Error(),
		}
	}

	// Validate required fields
	if req.ID == "" {
		return nil, &RPCError{
			Code:    InvalidRequest,
			Message: "Invalid request: missing id field",
		}
	}

	if req.Method == "" {
		return nil, &RPCError{
			Code:    InvalidRequest,
			Message: "Invalid request: missing method field",
		}
	}

	// Set JSONRPC version if not provided
	if req.JSONRPC == "" {
		req.JSONRPC = "2.0"
	}

	return &req, nil
}

// RouteRequest routes a request to the appropriate handler
func (r *RPCRouter) RouteRequest(req *RPCRequest) *RPCResponse {
	r.mu.RLock()
	handler, exists := r.methods[req.Method]
	r.mu.RUnlock()

	if !exists {
		return &RPCResponse{
			ID:      req.ID,
			JSONRPC: "2.0",
			Error: &RPCError{
				Code:    MethodNotFound,
				Message: fmt.Sprintf("Method not found: %s", req.Method),
			},
		}
	}

	// Execute handler
	result, err := handler(req.Params)
	if err != nil {
		return &RPCResponse{
			ID:      req.ID,
			JSONRPC: "2.0",
			Error: &RPCError{
				Code:    InternalError,
				Message: err.Error(),
			},
		}
	}

	return &RPCResponse{
		ID:      req.ID,
		JSONRPC: "2.0",
		Result:  result,
	}
}

// HasMethod checks if a method is registered
func (r *RPCRouter) HasMethod(name string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	_, exists := r.methods[name]
	return exists
}

// GetMethods returns all registered method names
func (r *RPCRouter) GetMethods() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	methods := make([]string, 0, len(r.methods))
	for name := range r.methods {
		methods = append(methods, name)
	}
	return methods
}
