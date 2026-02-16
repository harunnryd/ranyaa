package gateway

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRPCRouter_RegisterMethod(t *testing.T) {
	router := NewRPCRouter()

	t.Run("should register method successfully", func(t *testing.T) {
		handler := func(params map[string]interface{}) (interface{}, error) {
			return "result", nil
		}

		err := router.RegisterMethod("test.method", handler)
		assert.NoError(t, err)
		assert.True(t, router.HasMethod("test.method"))
	})

	t.Run("should replace existing method", func(t *testing.T) {
		handler1 := func(params map[string]interface{}) (interface{}, error) {
			return "result1", nil
		}
		handler2 := func(params map[string]interface{}) (interface{}, error) {
			return "result2", nil
		}

		router.RegisterMethod("test.replace", handler1)
		router.RegisterMethod("test.replace", handler2)

		assert.True(t, router.HasMethod("test.replace"))
	})

	t.Run("should reject nil handler", func(t *testing.T) {
		err := router.RegisterMethod("test.nil", nil)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "handler cannot be nil")
	})
}

func TestRPCRouter_UnregisterMethod(t *testing.T) {
	router := NewRPCRouter()

	t.Run("should unregister method", func(t *testing.T) {
		handler := func(params map[string]interface{}) (interface{}, error) {
			return "result", nil
		}

		router.RegisterMethod("test.method", handler)
		assert.True(t, router.HasMethod("test.method"))

		router.UnregisterMethod("test.method")
		assert.False(t, router.HasMethod("test.method"))
	})

	t.Run("should handle unregistering non-existent method", func(t *testing.T) {
		router.UnregisterMethod("non.existent")
		// Should not panic
	})
}

func TestRPCRouter_ParseRequest(t *testing.T) {
	router := NewRPCRouter()

	t.Run("should parse valid request", func(t *testing.T) {
		data := []byte(`{"id":"1","method":"test.method","params":{"key":"value"}}`)

		req, err := router.ParseRequest(data)
		require.NoError(t, err)
		assert.Equal(t, "1", req.ID)
		assert.Equal(t, "test.method", req.Method)
		assert.Equal(t, "value", req.Params["key"])
		assert.Equal(t, "2.0", req.JSONRPC)
	})

	t.Run("should reject malformed JSON", func(t *testing.T) {
		data := []byte(`{invalid json}`)

		_, err := router.ParseRequest(data)
		require.Error(t, err)

		rpcErr, ok := err.(*RPCError)
		require.True(t, ok)
		assert.Equal(t, ParseError, rpcErr.Code)
	})

	t.Run("should reject request without id", func(t *testing.T) {
		data := []byte(`{"method":"test.method"}`)

		_, err := router.ParseRequest(data)
		require.Error(t, err)

		rpcErr, ok := err.(*RPCError)
		require.True(t, ok)
		assert.Equal(t, InvalidRequest, rpcErr.Code)
		assert.Contains(t, rpcErr.Message, "missing id")
	})

	t.Run("should reject request without method", func(t *testing.T) {
		data := []byte(`{"id":"1"}`)

		_, err := router.ParseRequest(data)
		require.Error(t, err)

		rpcErr, ok := err.(*RPCError)
		require.True(t, ok)
		assert.Equal(t, InvalidRequest, rpcErr.Code)
		assert.Contains(t, rpcErr.Message, "missing method")
	})
}

func TestRPCRouter_RouteRequest(t *testing.T) {
	router := NewRPCRouter()

	t.Run("should route to registered handler", func(t *testing.T) {
		handler := func(params map[string]interface{}) (interface{}, error) {
			return map[string]interface{}{
				"echo": params["input"],
			}, nil
		}

		router.RegisterMethod("test.echo", handler)

		req := &RPCRequest{
			ID:     "1",
			Method: "test.echo",
			Params: map[string]interface{}{
				"input": "hello",
			},
		}

		resp := router.RouteRequest(req)
		assert.Equal(t, "1", resp.ID)
		assert.Nil(t, resp.Error)
		assert.NotNil(t, resp.Result)

		result := resp.Result.(map[string]interface{})
		assert.Equal(t, "hello", result["echo"])
	})

	t.Run("should return error for unknown method", func(t *testing.T) {
		req := &RPCRequest{
			ID:     "1",
			Method: "unknown.method",
		}

		resp := router.RouteRequest(req)
		assert.Equal(t, "1", resp.ID)
		assert.Nil(t, resp.Result)
		assert.NotNil(t, resp.Error)
		assert.Equal(t, MethodNotFound, resp.Error.Code)
	})

	t.Run("should return error when handler fails", func(t *testing.T) {
		handler := func(params map[string]interface{}) (interface{}, error) {
			return nil, fmt.Errorf("handler error")
		}

		router.RegisterMethod("test.error", handler)

		req := &RPCRequest{
			ID:     "1",
			Method: "test.error",
		}

		resp := router.RouteRequest(req)
		assert.Equal(t, "1", resp.ID)
		assert.Nil(t, resp.Result)
		assert.NotNil(t, resp.Error)
		assert.Equal(t, InternalError, resp.Error.Code)
		assert.Contains(t, resp.Error.Message, "handler error")
	})

	t.Run("should preserve request ID in response", func(t *testing.T) {
		handler := func(params map[string]interface{}) (interface{}, error) {
			return "ok", nil
		}

		router.RegisterMethod("test.id", handler)

		req := &RPCRequest{
			ID:     "unique-id-123",
			Method: "test.id",
		}

		resp := router.RouteRequest(req)
		assert.Equal(t, "unique-id-123", resp.ID)
	})
}

func TestRPCRouter_GetMethods(t *testing.T) {
	router := NewRPCRouter()

	t.Run("should return all registered methods", func(t *testing.T) {
		handler := func(params map[string]interface{}) (interface{}, error) {
			return nil, nil
		}

		router.RegisterMethod("method1", handler)
		router.RegisterMethod("method2", handler)
		router.RegisterMethod("method3", handler)

		methods := router.GetMethods()
		assert.Len(t, methods, 3)
		assert.Contains(t, methods, "method1")
		assert.Contains(t, methods, "method2")
		assert.Contains(t, methods, "method3")
	})

	t.Run("should return empty list when no methods registered", func(t *testing.T) {
		router := NewRPCRouter()
		methods := router.GetMethods()
		assert.Empty(t, methods)
	})
}
