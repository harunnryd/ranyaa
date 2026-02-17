package gateway

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

// RPCRouter handles RPC method registration and request routing
type RPCRouter struct {
	mu               sync.RWMutex
	methods          map[string]RequestHandler
	idempotencyTTL   time.Duration
	idempotencyCache map[string]cachedRPCResponse
}

type cachedRPCResponse struct {
	response  RPCResponse
	expiresAt time.Time
}

// NewRPCRouter creates a new RPC router
func NewRPCRouter() *RPCRouter {
	return &RPCRouter{
		methods:          make(map[string]RequestHandler),
		idempotencyTTL:   5 * time.Minute,
		idempotencyCache: make(map[string]cachedRPCResponse),
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
	if req == nil {
		return &RPCResponse{
			ID:      "",
			JSONRPC: "2.0",
			Error: &RPCError{
				Code:    InvalidRequest,
				Message: "invalid request",
			},
		}
	}

	cacheKey := idempotencyCacheKey(req.Method, req.IdempotencyKey)
	if cacheKey != "" {
		if cached, ok := r.getCachedResponse(cacheKey); ok {
			cached.ID = req.ID
			return &cached
		}
	}

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
	var response *RPCResponse
	if err != nil {
		response = &RPCResponse{
			ID:      req.ID,
			JSONRPC: "2.0",
			Error: &RPCError{
				Code:    InternalError,
				Message: err.Error(),
			},
		}
	} else {
		response = &RPCResponse{
			ID:      req.ID,
			JSONRPC: "2.0",
			Result:  result,
		}
	}

	if cacheKey != "" {
		r.cacheResponse(cacheKey, *response)
	}

	return response
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

func idempotencyCacheKey(method string, idempotencyKey string) string {
	if idempotencyKey == "" {
		return ""
	}
	return method + ":" + idempotencyKey
}

func (r *RPCRouter) getCachedResponse(key string) (RPCResponse, bool) {
	r.mu.RLock()
	entry, exists := r.idempotencyCache[key]
	r.mu.RUnlock()
	if !exists {
		return RPCResponse{}, false
	}

	now := time.Now()
	if now.After(entry.expiresAt) {
		r.mu.Lock()
		if current, ok := r.idempotencyCache[key]; ok && now.After(current.expiresAt) {
			delete(r.idempotencyCache, key)
		}
		r.mu.Unlock()
		return RPCResponse{}, false
	}

	return cloneRPCResponse(entry.response), true
}

func (r *RPCRouter) cacheResponse(key string, response RPCResponse) {
	now := time.Now()

	r.mu.Lock()
	r.idempotencyCache[key] = cachedRPCResponse{
		response:  cloneRPCResponse(response),
		expiresAt: now.Add(r.idempotencyTTL),
	}
	for cacheKey, entry := range r.idempotencyCache {
		if now.After(entry.expiresAt) {
			delete(r.idempotencyCache, cacheKey)
		}
	}
	r.mu.Unlock()
}

func cloneRPCResponse(src RPCResponse) RPCResponse {
	cloned := RPCResponse{
		ID:      src.ID,
		Result:  src.Result,
		JSONRPC: src.JSONRPC,
	}
	if src.Error != nil {
		errCopy := *src.Error
		cloned.Error = &errCopy
	}
	return cloned
}
