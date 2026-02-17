package toolexecutor

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestToolExecutor_RegisterTool(t *testing.T) {
	te := New()

	def := ToolDefinition{
		Name:        "test_tool",
		Description: "A test tool",
		Parameters: []ToolParameter{
			{
				Name:        "input",
				Type:        "string",
				Description: "Input parameter",
				Required:    true,
			},
		},
		Handler: func(ctx context.Context, params map[string]interface{}) (interface{}, error) {
			return "result", nil
		},
	}

	err := te.RegisterTool(def)
	assert.NoError(t, err)

	// Verify tool is registered
	tool := te.GetTool("test_tool")
	assert.NotNil(t, tool)
	assert.Equal(t, "test_tool", tool.Name)
}

func TestToolExecutor_RegisterTool_InvalidDefinition(t *testing.T) {
	te := New()

	tests := []struct {
		name string
		def  ToolDefinition
	}{
		{
			name: "empty name",
			def: ToolDefinition{
				Description: "Test",
				Handler:     func(ctx context.Context, params map[string]interface{}) (interface{}, error) { return nil, nil },
			},
		},
		{
			name: "empty description",
			def: ToolDefinition{
				Name:    "test",
				Handler: func(ctx context.Context, params map[string]interface{}) (interface{}, error) { return nil, nil },
			},
		},
		{
			name: "nil handler",
			def: ToolDefinition{
				Name:        "test",
				Description: "Test",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := te.RegisterTool(tt.def)
			assert.Error(t, err)
		})
	}
}

func TestToolExecutor_Execute_Success(t *testing.T) {
	te := New()

	def := ToolDefinition{
		Name:        "echo",
		Description: "Echo tool",
		Parameters: []ToolParameter{
			{
				Name:        "message",
				Type:        "string",
				Description: "Message to echo",
				Required:    true,
			},
		},
		Handler: func(ctx context.Context, params map[string]interface{}) (interface{}, error) {
			return params["message"], nil
		},
	}

	err := te.RegisterTool(def)
	require.NoError(t, err)

	result := te.Execute(context.Background(), "echo", map[string]interface{}{
		"message": "Hello, World!",
	}, nil)

	assert.True(t, result.Success)
	assert.Equal(t, "Hello, World!", result.Output)
	assert.Empty(t, result.Error)
}

func TestToolExecutor_Execute_ToolNotFound(t *testing.T) {
	te := New()

	result := te.Execute(context.Background(), "nonexistent", map[string]interface{}{}, nil)

	assert.False(t, result.Success)
	assert.Contains(t, result.Error, "tool not found")
}

func TestToolExecutor_Execute_ValidationError(t *testing.T) {
	te := New()

	def := ToolDefinition{
		Name:        "test",
		Description: "Test tool",
		Parameters: []ToolParameter{
			{
				Name:        "required_param",
				Type:        "string",
				Description: "Required parameter",
				Required:    true,
			},
		},
		Handler: func(ctx context.Context, params map[string]interface{}) (interface{}, error) {
			return nil, nil
		},
	}

	err := te.RegisterTool(def)
	require.NoError(t, err)

	// Execute without required parameter
	result := te.Execute(context.Background(), "test", map[string]interface{}{}, nil)

	assert.False(t, result.Success)
	assert.Contains(t, result.Error, "validation")
}

func TestToolExecutor_Execute_HandlerError(t *testing.T) {
	te := New()

	expectedErr := errors.New("handler error")
	def := ToolDefinition{
		Name:        "failing_tool",
		Description: "A tool that fails",
		Parameters:  []ToolParameter{},
		Handler: func(ctx context.Context, params map[string]interface{}) (interface{}, error) {
			return nil, expectedErr
		},
	}

	err := te.RegisterTool(def)
	require.NoError(t, err)

	result := te.Execute(context.Background(), "failing_tool", map[string]interface{}{}, nil)

	assert.False(t, result.Success)
	assert.Contains(t, result.Error, "handler error")
}

func TestToolExecutor_Execute_Timeout(t *testing.T) {
	te := New()

	def := ToolDefinition{
		Name:        "slow_tool",
		Description: "A slow tool",
		Parameters:  []ToolParameter{},
		Handler: func(ctx context.Context, params map[string]interface{}) (interface{}, error) {
			time.Sleep(2 * time.Second)
			return "done", nil
		},
	}

	err := te.RegisterTool(def)
	require.NoError(t, err)

	execCtx := &ExecutionContext{
		Timeout: 100 * time.Millisecond,
	}

	result := te.Execute(context.Background(), "slow_tool", map[string]interface{}{}, execCtx)

	assert.False(t, result.Success)
	assert.Contains(t, result.Error, "timeout")
}

func TestToolExecutor_ExecuteWithRetry_RetryableTransientError(t *testing.T) {
	te := New()
	te.SetRetryConfig(RetryConfig{
		Enabled:        true,
		MaxAttempts:    3,
		InitialBackoff: 5 * time.Millisecond,
		MaxBackoff:     10 * time.Millisecond,
	})

	var attempts int
	def := ToolDefinition{
		Name:        "flaky_tool",
		Description: "Fails once with timeout then succeeds",
		Parameters:  []ToolParameter{},
		Handler: func(ctx context.Context, params map[string]interface{}) (interface{}, error) {
			attempts++
			if attempts == 1 {
				return nil, errors.New("network timeout while calling remote endpoint")
			}
			return "ok", nil
		},
	}

	err := te.RegisterTool(def)
	require.NoError(t, err)

	result := te.ExecuteWithRetry(context.Background(), "flaky_tool", map[string]interface{}{}, nil)

	assert.True(t, result.Success)
	assert.Equal(t, "ok", result.Output)
	assert.Equal(t, 2, attempts)
	assert.Equal(t, 1, result.Metadata["retry_attempts"])
}

func TestToolExecutor_ExecuteWithRetry_DoesNotRetryPermanentError(t *testing.T) {
	te := New()
	te.SetRetryConfig(RetryConfig{
		Enabled:        true,
		MaxAttempts:    3,
		InitialBackoff: 5 * time.Millisecond,
		MaxBackoff:     10 * time.Millisecond,
	})

	var attempts int
	def := ToolDefinition{
		Name:        "permanent_failure",
		Description: "Always fails with a permanent error",
		Parameters:  []ToolParameter{},
		Handler: func(ctx context.Context, params map[string]interface{}) (interface{}, error) {
			attempts++
			return nil, errors.New("permission denied: operation blocked by policy")
		},
	}

	err := te.RegisterTool(def)
	require.NoError(t, err)

	result := te.ExecuteWithRetry(context.Background(), "permanent_failure", map[string]interface{}{}, nil)

	assert.False(t, result.Success)
	assert.Contains(t, result.Error, "permission denied")
	assert.Equal(t, 1, attempts)
}

func TestToolExecutor_Execute_OutputTruncation(t *testing.T) {
	te := New()

	// Create large output (> 10KB)
	largeOutput := make([]byte, 15*1024)
	for i := range largeOutput {
		largeOutput[i] = 'A'
	}

	def := ToolDefinition{
		Name:        "large_output",
		Description: "Tool with large output",
		Parameters:  []ToolParameter{},
		Handler: func(ctx context.Context, params map[string]interface{}) (interface{}, error) {
			return string(largeOutput), nil
		},
	}

	err := te.RegisterTool(def)
	require.NoError(t, err)

	result := te.Execute(context.Background(), "large_output", map[string]interface{}{}, nil)

	assert.True(t, result.Success)
	assert.True(t, result.Truncated)
	assert.Contains(t, result.Output.(string), "truncated")
}

func TestToolExecutor_ListTools(t *testing.T) {
	te := New()

	tools := []string{"tool1", "tool2", "tool3"}
	for _, name := range tools {
		def := ToolDefinition{
			Name:        name,
			Description: "Test tool",
			Parameters:  []ToolParameter{},
			Handler: func(ctx context.Context, params map[string]interface{}) (interface{}, error) {
				return nil, nil
			},
		}
		err := te.RegisterTool(def)
		require.NoError(t, err)
	}

	list := te.ListTools()
	assert.ElementsMatch(t, tools, list)
}

func TestToolExecutor_UnregisterTool(t *testing.T) {
	te := New()

	def := ToolDefinition{
		Name:        "test_tool",
		Description: "Test tool",
		Parameters:  []ToolParameter{},
		Handler: func(ctx context.Context, params map[string]interface{}) (interface{}, error) {
			return nil, nil
		},
	}

	err := te.RegisterTool(def)
	require.NoError(t, err)

	// Verify tool exists
	assert.NotNil(t, te.GetTool("test_tool"))

	// Unregister
	te.UnregisterTool("test_tool")

	// Verify tool is removed
	assert.Nil(t, te.GetTool("test_tool"))
}

func TestToolExecutor_GetToolCount(t *testing.T) {
	te := New()

	assert.Equal(t, 0, te.GetToolCount())

	for i := 0; i < 5; i++ {
		def := ToolDefinition{
			Name:        fmt.Sprintf("tool%d", i),
			Description: "Test tool",
			Parameters:  []ToolParameter{},
			Handler: func(ctx context.Context, params map[string]interface{}) (interface{}, error) {
				return nil, nil
			},
		}
		err := te.RegisterTool(def)
		require.NoError(t, err)
	}

	assert.Equal(t, 5, te.GetToolCount())
}

func TestToolExecutor_ParameterTypes(t *testing.T) {
	te := New()

	def := ToolDefinition{
		Name:        "multi_param",
		Description: "Tool with multiple parameter types",
		Parameters: []ToolParameter{
			{Name: "str", Type: "string", Description: "String param", Required: true},
			{Name: "num", Type: "number", Description: "Number param", Required: true},
			{Name: "bool", Type: "boolean", Description: "Boolean param", Required: true},
			{Name: "obj", Type: "object", Description: "Object param", Required: false},
			{Name: "arr", Type: "array", Description: "Array param", Required: false},
		},
		Handler: func(ctx context.Context, params map[string]interface{}) (interface{}, error) {
			return params, nil
		},
	}

	err := te.RegisterTool(def)
	require.NoError(t, err)

	result := te.Execute(context.Background(), "multi_param", map[string]interface{}{
		"str":  "test",
		"num":  42.5,
		"bool": true,
		"obj":  map[string]interface{}{"key": "value"},
		"arr":  []interface{}{1, 2, 3},
	}, nil)

	assert.True(t, result.Success)
}
