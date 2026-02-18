package toolexecutor

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMCPServerHelper(t *testing.T) {
	if os.Getenv("MCP_SERVER_HELPER") != "1" {
		t.Skip("helper process")
	}

	scanner := bufio.NewScanner(os.Stdin)
	encoder := json.NewEncoder(os.Stdout)

	for scanner.Scan() {
		var req mcpRequest
		if err := json.Unmarshal(scanner.Bytes(), &req); err != nil {
			continue
		}

		switch req.Method {
		case "initialize":
			writeMCPResponse(encoder, req.ID, map[string]interface{}{"ok": true}, nil)
		case "tools/list":
			writeMCPResponse(encoder, req.ID, map[string]interface{}{
				"tools": []map[string]interface{}{
					{
						"name":        "calculator",
						"description": "adds two numbers",
						"inputSchema": map[string]interface{}{
							"type": "object",
							"properties": map[string]interface{}{
								"a": map[string]interface{}{"type": "number"},
								"b": map[string]interface{}{"type": "number"},
							},
							"required": []string{"a", "b"},
						},
					},
				},
			}, nil)
		case "tools/call":
			params, _ := req.Params.(map[string]interface{})
			name, _ := params["name"].(string)
			args, _ := params["arguments"].(map[string]interface{})
			if name == "calculator" {
				a, _ := args["a"].(float64)
				b, _ := args["b"].(float64)
				writeMCPResponse(encoder, req.ID, map[string]interface{}{"result": a + b}, nil)
				continue
			}
			writeMCPResponse(encoder, req.ID, nil, &mcpError{Code: -32601, Message: "tool not found"})
		case "resources/list":
			writeMCPResponse(encoder, req.ID, map[string]interface{}{
				"resources": []map[string]interface{}{
					{
						"uri":         "file://test.txt",
						"name":        "test",
						"description": "test resource",
						"mimeType":    "text/plain",
					},
				},
			}, nil)
		case "resources/read":
			writeMCPResponse(encoder, req.ID, map[string]interface{}{
				"content": "hello from mcp",
			}, nil)
		default:
			writeMCPResponse(encoder, req.ID, nil, &mcpError{Code: -32601, Message: "method not found"})
		}
	}
	_ = scanner.Err()
}

func writeMCPResponse(encoder *json.Encoder, id interface{}, result interface{}, err *mcpError) {
	resp := mcpResponse{JSONRPC: "2.0", ID: id, Error: err}
	if err == nil {
		payload, _ := json.Marshal(result)
		resp.Result = payload
	}
	_ = encoder.Encode(resp)
}

func TestMCPServerAdapter_RegistersToolsAndResources(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	os.Setenv("MCP_SERVER_HELPER", "1")
	defer os.Unsetenv("MCP_SERVER_HELPER")

	adapter := NewMCPServerAdapter("test", os.Args[0], []string{"-test.run", "TestMCPServerHelper"})
	defer func() {
		_ = adapter.Stop()
	}()

	executor := New()

	registered, err := executor.RegisterMCPServer(ctx, "test", adapter)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(registered), 3)

	tool := executor.GetTool("calculator")
	require.NotNil(t, tool)

	result := executor.ExecuteWithRetry(ctx, "calculator", map[string]interface{}{"a": 2.0, "b": 3.0}, &ExecutionContext{SessionKey: "mcp:test"})
	require.True(t, result.Success)
	outputMap, ok := result.Output.(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, float64(5), outputMap["result"])

	resourceListTool := "mcp_test_resources_list"
	if executor.GetTool(resourceListTool) == nil {
		resourceListTool = "test_mcp_test_resources_list"
	}
	listResult := executor.ExecuteWithRetry(ctx, resourceListTool, map[string]interface{}{}, &ExecutionContext{SessionKey: "mcp:test"})
	require.True(t, listResult.Success)
	listBytes, _ := json.Marshal(listResult.Output)
	assert.True(t, strings.Contains(string(listBytes), "file://test.txt"))

	resourceReadTool := "mcp_test_resource_read"
	if executor.GetTool(resourceReadTool) == nil {
		resourceReadTool = "test_mcp_test_resource_read"
	}
	readResult := executor.ExecuteWithRetry(ctx, resourceReadTool, map[string]interface{}{"uri": "file://test.txt"}, &ExecutionContext{SessionKey: "mcp:test"})
	require.True(t, readResult.Success)
	readBytes, _ := json.Marshal(readResult.Output)
	assert.True(t, strings.Contains(string(readBytes), "hello from mcp"))
}
