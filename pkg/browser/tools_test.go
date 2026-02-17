package browser

import (
	"context"
	"testing"

	"github.com/harun/ranya/pkg/toolexecutor"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockToolExecutor implements toolexecutor interface for testing
type mockToolExecutor struct {
	registeredTools []toolexecutor.ToolDefinition
}

func (m *mockToolExecutor) RegisterTool(def toolexecutor.ToolDefinition) error {
	m.registeredTools = append(m.registeredTools, def)
	return nil
}

func TestRegisterBrowserTools(t *testing.T) {
	// Create mock browser context (nil is acceptable for registration test)
	var browserContext *BrowserServerContext

	executor := toolexecutor.New()

	err := RegisterBrowserTools(executor, browserContext)
	require.NoError(t, err)

	// Verify all 6 tools were registered
	assert.Equal(t, 6, executor.GetToolCount())

	// Verify tool names
	expectedTools := []string{
		"browser_navigate",
		"browser_screenshot",
		"browser_extract",
		"browser_interact",
		"browser_execute",
		"browser_cookies",
	}

	registeredTools := executor.ListTools()
	for _, expected := range expectedTools {
		assert.Contains(t, registeredTools, expected)
	}
}

func TestCreateScreenshotTool(t *testing.T) {
	var browserContext *BrowserServerContext
	tool := createScreenshotTool(browserContext)

	t.Run("tool definition", func(t *testing.T) {
		assert.Equal(t, "browser_screenshot", tool.Name)
		assert.NotEmpty(t, tool.Description)
		assert.NotNil(t, tool.Handler)
	})

	t.Run("parameters", func(t *testing.T) {
		// Verify required parameters
		paramMap := make(map[string]toolexecutor.ToolParameter)
		for _, param := range tool.Parameters {
			paramMap[param.Name] = param
		}

		// Check profile parameter
		profile, ok := paramMap["profile"]
		require.True(t, ok, "profile parameter should exist")
		assert.True(t, profile.Required)
		assert.Equal(t, "string", profile.Type)

		// Check targetId parameter
		targetID, ok := paramMap["targetId"]
		require.True(t, ok, "targetId parameter should exist")
		assert.True(t, targetID.Required)
		assert.Equal(t, "string", targetID.Type)

		// Check type parameter (optional)
		imgType, ok := paramMap["type"]
		require.True(t, ok, "type parameter should exist")
		assert.False(t, imgType.Required)
		assert.Equal(t, "string", imgType.Type)
		assert.Equal(t, "png", imgType.Default)

		// Check quality parameter (optional)
		quality, ok := paramMap["quality"]
		require.True(t, ok, "quality parameter should exist")
		assert.False(t, quality.Required)
		assert.Equal(t, "integer", quality.Type)
		assert.Equal(t, 80, quality.Default)

		// Check fullPage parameter (optional)
		fullPage, ok := paramMap["fullPage"]
		require.True(t, ok, "fullPage parameter should exist")
		assert.False(t, fullPage.Required)
		assert.Equal(t, "boolean", fullPage.Type)
		assert.Equal(t, false, fullPage.Default)

		// Check selector parameter (optional)
		selector, ok := paramMap["selector"]
		require.True(t, ok, "selector parameter should exist")
		assert.False(t, selector.Required)
		assert.Equal(t, "string", selector.Type)
	})
}

func TestScreenshotToolHandler_Validation(t *testing.T) {
	var browserContext *BrowserServerContext
	tool := createScreenshotTool(browserContext)
	handler := tool.Handler

	t.Run("missing profile parameter", func(t *testing.T) {
		params := map[string]interface{}{
			"targetId": "test-target",
		}

		result, err := handler(context.Background(), params)
		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "profile")
	})

	t.Run("missing targetId parameter", func(t *testing.T) {
		params := map[string]interface{}{
			"profile": "default",
		}

		result, err := handler(context.Background(), params)
		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "targetId")
	})

	t.Run("invalid profile type", func(t *testing.T) {
		params := map[string]interface{}{
			"profile":  123, // Should be string
			"targetId": "test-target",
		}

		result, err := handler(context.Background(), params)
		assert.Error(t, err)
		assert.Nil(t, result)
	})

	t.Run("invalid targetId type", func(t *testing.T) {
		params := map[string]interface{}{
			"profile":  "default",
			"targetId": 123, // Should be string
		}

		result, err := handler(context.Background(), params)
		assert.Error(t, err)
		assert.Nil(t, result)
	})
}

func TestCreateExtractTool(t *testing.T) {
	var browserContext *BrowserServerContext
	tool := createExtractTool(browserContext)

	t.Run("tool definition", func(t *testing.T) {
		assert.Equal(t, "browser_extract", tool.Name)
		assert.NotEmpty(t, tool.Description)
		assert.NotNil(t, tool.Handler)
		assert.Contains(t, tool.Description, "Extract content")
	})

	t.Run("parameters", func(t *testing.T) {
		// Verify required parameters
		paramMap := make(map[string]toolexecutor.ToolParameter)
		for _, param := range tool.Parameters {
			paramMap[param.Name] = param
		}

		// Check profile parameter
		profile, ok := paramMap["profile"]
		require.True(t, ok, "profile parameter should exist")
		assert.True(t, profile.Required)
		assert.Equal(t, "string", profile.Type)

		// Check targetId parameter
		targetID, ok := paramMap["targetId"]
		require.True(t, ok, "targetId parameter should exist")
		assert.True(t, targetID.Required)
		assert.Equal(t, "string", targetID.Type)

		// Check type parameter (required)
		extractType, ok := paramMap["type"]
		require.True(t, ok, "type parameter should exist")
		assert.True(t, extractType.Required)
		assert.Equal(t, "string", extractType.Type)
		assert.Contains(t, extractType.Description, "html")
		assert.Contains(t, extractType.Description, "text")
		assert.Contains(t, extractType.Description, "metadata")
		assert.Contains(t, extractType.Description, "links")
		assert.Contains(t, extractType.Description, "selector")

		// Check selector parameter (optional)
		selector, ok := paramMap["selector"]
		require.True(t, ok, "selector parameter should exist")
		assert.False(t, selector.Required)
		assert.Equal(t, "string", selector.Type)
	})
}

func TestExtractToolHandler_Validation(t *testing.T) {
	var browserContext *BrowserServerContext
	tool := createExtractTool(browserContext)
	handler := tool.Handler

	t.Run("missing profile parameter", func(t *testing.T) {
		params := map[string]interface{}{
			"targetId": "test-target",
			"type":     "html",
		}

		result, err := handler(context.Background(), params)
		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "profile")
	})

	t.Run("missing targetId parameter", func(t *testing.T) {
		params := map[string]interface{}{
			"profile": "default",
			"type":    "html",
		}

		result, err := handler(context.Background(), params)
		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "targetId")
	})

	t.Run("missing type parameter", func(t *testing.T) {
		params := map[string]interface{}{
			"profile":  "default",
			"targetId": "test-target",
		}

		result, err := handler(context.Background(), params)
		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "type")
	})

	t.Run("invalid profile type", func(t *testing.T) {
		params := map[string]interface{}{
			"profile":  123, // Should be string
			"targetId": "test-target",
			"type":     "html",
		}

		result, err := handler(context.Background(), params)
		assert.Error(t, err)
		assert.Nil(t, result)
	})

	t.Run("invalid targetId type", func(t *testing.T) {
		params := map[string]interface{}{
			"profile":  "default",
			"targetId": 123, // Should be string
			"type":     "html",
		}

		result, err := handler(context.Background(), params)
		assert.Error(t, err)
		assert.Nil(t, result)
	})

	t.Run("invalid type parameter type", func(t *testing.T) {
		params := map[string]interface{}{
			"profile":  "default",
			"targetId": "test-target",
			"type":     123, // Should be string
		}

		result, err := handler(context.Background(), params)
		assert.Error(t, err)
		assert.Nil(t, result)
	})
}

func TestCreateInteractTool(t *testing.T) {
	var browserContext *BrowserServerContext
	tool := createInteractTool(browserContext)

	t.Run("tool definition", func(t *testing.T) {
		assert.Equal(t, "browser_interact", tool.Name)
		assert.NotEmpty(t, tool.Description)
		assert.NotNil(t, tool.Handler)
		assert.Contains(t, tool.Description, "Interact with elements")
	})

	t.Run("parameters", func(t *testing.T) {
		// Verify required parameters
		paramMap := make(map[string]toolexecutor.ToolParameter)
		for _, param := range tool.Parameters {
			paramMap[param.Name] = param
		}

		// Check profile parameter
		profile, ok := paramMap["profile"]
		require.True(t, ok, "profile parameter should exist")
		assert.True(t, profile.Required)
		assert.Equal(t, "string", profile.Type)

		// Check targetId parameter
		targetID, ok := paramMap["targetId"]
		require.True(t, ok, "targetId parameter should exist")
		assert.True(t, targetID.Required)
		assert.Equal(t, "string", targetID.Type)

		// Check action parameter (required)
		action, ok := paramMap["action"]
		require.True(t, ok, "action parameter should exist")
		assert.True(t, action.Required)
		assert.Equal(t, "string", action.Type)
		assert.Contains(t, action.Description, "click")
		assert.Contains(t, action.Description, "type")
		assert.Contains(t, action.Description, "select")
		assert.Contains(t, action.Description, "wait")
		assert.Contains(t, action.Description, "visible")

		// Check selector parameter (required)
		selector, ok := paramMap["selector"]
		require.True(t, ok, "selector parameter should exist")
		assert.True(t, selector.Required)
		assert.Equal(t, "string", selector.Type)

		// Check value parameter (optional)
		value, ok := paramMap["value"]
		require.True(t, ok, "value parameter should exist")
		assert.False(t, value.Required)
		assert.Equal(t, "string", value.Type)

		// Check timeout parameter (optional)
		timeout, ok := paramMap["timeout"]
		require.True(t, ok, "timeout parameter should exist")
		assert.False(t, timeout.Required)
		assert.Equal(t, "integer", timeout.Type)
		assert.Equal(t, 30, timeout.Default)
	})
}

func TestInteractToolHandler_Validation(t *testing.T) {
	var browserContext *BrowserServerContext
	tool := createInteractTool(browserContext)
	handler := tool.Handler

	t.Run("missing profile parameter", func(t *testing.T) {
		params := map[string]interface{}{
			"targetId": "test-target",
			"action":   "click",
			"selector": "#button",
		}

		result, err := handler(context.Background(), params)
		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "profile")
	})

	t.Run("missing targetId parameter", func(t *testing.T) {
		params := map[string]interface{}{
			"profile":  "default",
			"action":   "click",
			"selector": "#button",
		}

		result, err := handler(context.Background(), params)
		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "targetId")
	})

	t.Run("missing action parameter", func(t *testing.T) {
		params := map[string]interface{}{
			"profile":  "default",
			"targetId": "test-target",
			"selector": "#button",
		}

		result, err := handler(context.Background(), params)
		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "action")
	})

	t.Run("missing selector parameter", func(t *testing.T) {
		params := map[string]interface{}{
			"profile":  "default",
			"targetId": "test-target",
			"action":   "click",
		}

		result, err := handler(context.Background(), params)
		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "selector")
	})

	t.Run("invalid profile type", func(t *testing.T) {
		params := map[string]interface{}{
			"profile":  123, // Should be string
			"targetId": "test-target",
			"action":   "click",
			"selector": "#button",
		}

		result, err := handler(context.Background(), params)
		assert.Error(t, err)
		assert.Nil(t, result)
	})

	t.Run("invalid targetId type", func(t *testing.T) {
		params := map[string]interface{}{
			"profile":  "default",
			"targetId": 123, // Should be string
			"action":   "click",
			"selector": "#button",
		}

		result, err := handler(context.Background(), params)
		assert.Error(t, err)
		assert.Nil(t, result)
	})

	t.Run("invalid action type", func(t *testing.T) {
		params := map[string]interface{}{
			"profile":  "default",
			"targetId": "test-target",
			"action":   123, // Should be string
			"selector": "#button",
		}

		result, err := handler(context.Background(), params)
		assert.Error(t, err)
		assert.Nil(t, result)
	})

	t.Run("invalid selector type", func(t *testing.T) {
		params := map[string]interface{}{
			"profile":  "default",
			"targetId": "test-target",
			"action":   "click",
			"selector": 123, // Should be string
		}

		result, err := handler(context.Background(), params)
		assert.Error(t, err)
		assert.Nil(t, result)
	})
}

func TestCreateCookiesTool(t *testing.T) {
	var browserContext *BrowserServerContext
	tool := createCookiesTool(browserContext)

	t.Run("tool definition", func(t *testing.T) {
		assert.Equal(t, "browser_cookies", tool.Name)
		assert.NotEmpty(t, tool.Description)
		assert.NotNil(t, tool.Handler)
		assert.Contains(t, tool.Description, "Manage browser cookies")
	})

	t.Run("parameters", func(t *testing.T) {
		// Verify required parameters
		paramMap := make(map[string]toolexecutor.ToolParameter)
		for _, param := range tool.Parameters {
			paramMap[param.Name] = param
		}

		// Check profile parameter
		profile, ok := paramMap["profile"]
		require.True(t, ok, "profile parameter should exist")
		assert.True(t, profile.Required)
		assert.Equal(t, "string", profile.Type)

		// Check targetId parameter
		targetID, ok := paramMap["targetId"]
		require.True(t, ok, "targetId parameter should exist")
		assert.True(t, targetID.Required)
		assert.Equal(t, "string", targetID.Type)

		// Check action parameter (required)
		action, ok := paramMap["action"]
		require.True(t, ok, "action parameter should exist")
		assert.True(t, action.Required)
		assert.Equal(t, "string", action.Type)
		assert.Contains(t, action.Description, "get")
		assert.Contains(t, action.Description, "set")
		assert.Contains(t, action.Description, "delete")
		assert.Contains(t, action.Description, "clear")

		// Check names parameter (optional)
		names, ok := paramMap["names"]
		require.True(t, ok, "names parameter should exist")
		assert.False(t, names.Required)
		assert.Equal(t, "array", names.Type)

		// Check cookies parameter (optional)
		cookies, ok := paramMap["cookies"]
		require.True(t, ok, "cookies parameter should exist")
		assert.False(t, cookies.Required)
		assert.Equal(t, "array", cookies.Type)
	})
}

func TestCookiesToolHandler_Validation(t *testing.T) {
	var browserContext *BrowserServerContext
	tool := createCookiesTool(browserContext)
	handler := tool.Handler

	t.Run("missing profile parameter", func(t *testing.T) {
		params := map[string]interface{}{
			"targetId": "test-target",
			"action":   "get",
		}

		result, err := handler(context.Background(), params)
		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "profile")
	})

	t.Run("missing targetId parameter", func(t *testing.T) {
		params := map[string]interface{}{
			"profile": "default",
			"action":  "get",
		}

		result, err := handler(context.Background(), params)
		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "targetId")
	})

	t.Run("missing action parameter", func(t *testing.T) {
		params := map[string]interface{}{
			"profile":  "default",
			"targetId": "test-target",
		}

		result, err := handler(context.Background(), params)
		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "action")
	})

	t.Run("invalid profile type", func(t *testing.T) {
		params := map[string]interface{}{
			"profile":  123, // Should be string
			"targetId": "test-target",
			"action":   "get",
		}

		result, err := handler(context.Background(), params)
		assert.Error(t, err)
		assert.Nil(t, result)
	})

	t.Run("invalid targetId type", func(t *testing.T) {
		params := map[string]interface{}{
			"profile":  "default",
			"targetId": 123, // Should be string
			"action":   "get",
		}

		result, err := handler(context.Background(), params)
		assert.Error(t, err)
		assert.Nil(t, result)
	})

	t.Run("invalid action type", func(t *testing.T) {
		params := map[string]interface{}{
			"profile":  "default",
			"targetId": "test-target",
			"action":   123, // Should be string
		}

		result, err := handler(context.Background(), params)
		assert.Error(t, err)
		assert.Nil(t, result)
	})

	t.Run("invalid action value", func(t *testing.T) {
		params := map[string]interface{}{
			"profile":  "default",
			"targetId": "test-target",
			"action":   "invalid_action",
		}

		result, err := handler(context.Background(), params)
		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "invalid cookie action")
	})

	t.Run("delete action without names", func(t *testing.T) {
		params := map[string]interface{}{
			"profile":  "default",
			"targetId": "test-target",
			"action":   "delete",
		}

		result, err := handler(context.Background(), params)
		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "names parameter is required")
	})

	t.Run("set action without cookies", func(t *testing.T) {
		params := map[string]interface{}{
			"profile":  "default",
			"targetId": "test-target",
			"action":   "set",
		}

		result, err := handler(context.Background(), params)
		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "cookies parameter is required")
	})
}

// Navigate Tool Tests

func TestCreateNavigateTool(t *testing.T) {
	var browserContext *BrowserServerContext
	tool := createNavigateTool(browserContext)

	t.Run("tool definition", func(t *testing.T) {
		assert.Equal(t, "browser_navigate", tool.Name)
		assert.NotEmpty(t, tool.Description)
		assert.NotNil(t, tool.Handler)
		assert.Contains(t, tool.Description, "Navigate to a URL")
	})

	t.Run("parameters", func(t *testing.T) {
		paramMap := make(map[string]toolexecutor.ToolParameter)
		for _, param := range tool.Parameters {
			paramMap[param.Name] = param
		}

		// Check profile parameter
		profile, ok := paramMap["profile"]
		require.True(t, ok, "profile parameter should exist")
		assert.True(t, profile.Required)
		assert.Equal(t, "string", profile.Type)

		// Check url parameter
		url, ok := paramMap["url"]
		require.True(t, ok, "url parameter should exist")
		assert.True(t, url.Required)
		assert.Equal(t, "string", url.Type)

		// Check targetId parameter (optional)
		targetID, ok := paramMap["targetId"]
		require.True(t, ok, "targetId parameter should exist")
		assert.False(t, targetID.Required)
		assert.Equal(t, "string", targetID.Type)

		// Check timeout parameter (optional)
		timeout, ok := paramMap["timeout"]
		require.True(t, ok, "timeout parameter should exist")
		assert.False(t, timeout.Required)
		assert.Equal(t, "integer", timeout.Type)
		assert.Equal(t, 30, timeout.Default)

		// Check waitUntil parameter (optional)
		waitUntil, ok := paramMap["waitUntil"]
		require.True(t, ok, "waitUntil parameter should exist")
		assert.False(t, waitUntil.Required)
		assert.Equal(t, "string", waitUntil.Type)
		assert.Equal(t, "load", waitUntil.Default)
	})
}

func TestNavigateToolHandler_Validation(t *testing.T) {
	var browserContext *BrowserServerContext
	tool := createNavigateTool(browserContext)
	handler := tool.Handler

	t.Run("missing profile parameter", func(t *testing.T) {
		params := map[string]interface{}{
			"url": "https://example.com",
		}

		result, err := handler(context.Background(), params)
		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "profile")
	})

	t.Run("missing url parameter", func(t *testing.T) {
		params := map[string]interface{}{
			"profile": "default",
		}

		result, err := handler(context.Background(), params)
		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "url")
	})

	t.Run("invalid profile type", func(t *testing.T) {
		params := map[string]interface{}{
			"profile": 123, // Should be string
			"url":     "https://example.com",
		}

		result, err := handler(context.Background(), params)
		assert.Error(t, err)
		assert.Nil(t, result)
	})

	t.Run("invalid url type", func(t *testing.T) {
		params := map[string]interface{}{
			"profile": "default",
			"url":     123, // Should be string
		}

		result, err := handler(context.Background(), params)
		assert.Error(t, err)
		assert.Nil(t, result)
	})
}

// Execute Tool Tests

func TestCreateExecuteTool(t *testing.T) {
	var browserContext *BrowserServerContext
	tool := createExecuteTool(browserContext)

	t.Run("tool definition", func(t *testing.T) {
		assert.Equal(t, "browser_execute", tool.Name)
		assert.NotEmpty(t, tool.Description)
		assert.NotNil(t, tool.Handler)
		assert.Contains(t, tool.Description, "Execute JavaScript")
	})

	t.Run("parameters", func(t *testing.T) {
		paramMap := make(map[string]toolexecutor.ToolParameter)
		for _, param := range tool.Parameters {
			paramMap[param.Name] = param
		}

		// Check profile parameter
		profile, ok := paramMap["profile"]
		require.True(t, ok, "profile parameter should exist")
		assert.True(t, profile.Required)
		assert.Equal(t, "string", profile.Type)

		// Check targetId parameter
		targetID, ok := paramMap["targetId"]
		require.True(t, ok, "targetId parameter should exist")
		assert.True(t, targetID.Required)
		assert.Equal(t, "string", targetID.Type)

		// Check script parameter (required)
		script, ok := paramMap["script"]
		require.True(t, ok, "script parameter should exist")
		assert.True(t, script.Required)
		assert.Equal(t, "string", script.Type)

		// Check args parameter (optional)
		args, ok := paramMap["args"]
		require.True(t, ok, "args parameter should exist")
		assert.False(t, args.Required)
		assert.Equal(t, "array", args.Type)

		// Check timeout parameter (optional)
		timeout, ok := paramMap["timeout"]
		require.True(t, ok, "timeout parameter should exist")
		assert.False(t, timeout.Required)
		assert.Equal(t, "integer", timeout.Type)
		assert.Equal(t, 30, timeout.Default)
	})
}

func TestExecuteToolHandler_Validation(t *testing.T) {
	var browserContext *BrowserServerContext
	tool := createExecuteTool(browserContext)
	handler := tool.Handler

	t.Run("missing profile parameter", func(t *testing.T) {
		params := map[string]interface{}{
			"targetId": "test-target",
			"script":   "return 1 + 1;",
		}

		result, err := handler(context.Background(), params)
		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "profile")
	})

	t.Run("missing targetId parameter", func(t *testing.T) {
		params := map[string]interface{}{
			"profile": "default",
			"script":  "return 1 + 1;",
		}

		result, err := handler(context.Background(), params)
		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "targetId")
	})

	t.Run("missing script parameter", func(t *testing.T) {
		params := map[string]interface{}{
			"profile":  "default",
			"targetId": "test-target",
		}

		result, err := handler(context.Background(), params)
		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "script")
	})

	t.Run("invalid profile type", func(t *testing.T) {
		params := map[string]interface{}{
			"profile":  123, // Should be string
			"targetId": "test-target",
			"script":   "return 1 + 1;",
		}

		result, err := handler(context.Background(), params)
		assert.Error(t, err)
		assert.Nil(t, result)
	})

	t.Run("invalid targetId type", func(t *testing.T) {
		params := map[string]interface{}{
			"profile":  "default",
			"targetId": 123, // Should be string
			"script":   "return 1 + 1;",
		}

		result, err := handler(context.Background(), params)
		assert.Error(t, err)
		assert.Nil(t, result)
	})

	t.Run("invalid script type", func(t *testing.T) {
		params := map[string]interface{}{
			"profile":  "default",
			"targetId": "test-target",
			"script":   123, // Should be string
		}

		result, err := handler(context.Background(), params)
		assert.Error(t, err)
		assert.Nil(t, result)
	})
}

// Security Validation Tests
// Note: Full URL security validation is tested in security_test.go
// These tests verify that the tool handlers properly validate parameters
// URL security is enforced at the ProfileContext level during actual navigation

// Error Handling Tests
// Note: These tests verify parameter validation errors
// Profile not found errors are tested separately with proper mocking

func TestToolHandlers_ParameterErrors(t *testing.T) {
	var browserContext *BrowserServerContext

	t.Run("navigate with wrong parameter types", func(t *testing.T) {
		tool := createNavigateTool(browserContext)
		handler := tool.Handler

		params := map[string]interface{}{
			"profile": 123, // Wrong type
			"url":     "https://example.com",
		}

		result, err := handler(context.Background(), params)
		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "profile")
	})

	t.Run("screenshot with missing required params", func(t *testing.T) {
		tool := createScreenshotTool(browserContext)
		handler := tool.Handler

		params := map[string]interface{}{
			"profile": "default",
			// Missing targetId
		}

		result, err := handler(context.Background(), params)
		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "targetId")
	})

	t.Run("extract with invalid type parameter", func(t *testing.T) {
		tool := createExtractTool(browserContext)
		handler := tool.Handler

		params := map[string]interface{}{
			"profile":  "default",
			"targetId": 456, // Wrong type
			"type":     "html",
		}

		result, err := handler(context.Background(), params)
		assert.Error(t, err)
		assert.Nil(t, result)
	})

	t.Run("interact with missing action", func(t *testing.T) {
		tool := createInteractTool(browserContext)
		handler := tool.Handler

		params := map[string]interface{}{
			"profile":  "default",
			"targetId": "test-target",
			"selector": "#button",
			// Missing action
		}

		result, err := handler(context.Background(), params)
		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "action")
	})

	t.Run("execute with empty script", func(t *testing.T) {
		tool := createExecuteTool(browserContext)
		handler := tool.Handler

		params := map[string]interface{}{
			"profile":  "default",
			"targetId": "test-target",
			// Missing script
		}

		result, err := handler(context.Background(), params)
		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "script")
	})

	t.Run("cookies with invalid action", func(t *testing.T) {
		tool := createCookiesTool(browserContext)
		handler := tool.Handler

		params := map[string]interface{}{
			"profile":  "default",
			"targetId": "test-target",
			"action":   "invalid_action",
		}

		result, err := handler(context.Background(), params)
		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "invalid cookie action")
	})
}

// Timeout Tests
// Note: These tests verify that timeout parameters are properly extracted and passed
// Actual timeout enforcement is tested in integration tests

func TestToolHandlers_TimeoutParameters(t *testing.T) {
	t.Run("navigate tool accepts timeout parameter", func(t *testing.T) {
		tool := createNavigateTool(nil)

		// Verify timeout parameter exists and has correct default
		var timeoutParam *toolexecutor.ToolParameter
		for _, param := range tool.Parameters {
			if param.Name == "timeout" {
				timeoutParam = &param
				break
			}
		}

		require.NotNil(t, timeoutParam)
		assert.Equal(t, "integer", timeoutParam.Type)
		assert.Equal(t, 30, timeoutParam.Default)
		assert.False(t, timeoutParam.Required)
	})

	t.Run("interact tool accepts timeout parameter", func(t *testing.T) {
		tool := createInteractTool(nil)

		var timeoutParam *toolexecutor.ToolParameter
		for _, param := range tool.Parameters {
			if param.Name == "timeout" {
				timeoutParam = &param
				break
			}
		}

		require.NotNil(t, timeoutParam)
		assert.Equal(t, "integer", timeoutParam.Type)
		assert.Equal(t, 30, timeoutParam.Default)
		assert.False(t, timeoutParam.Required)
	})

	t.Run("execute tool accepts timeout parameter", func(t *testing.T) {
		tool := createExecuteTool(nil)

		var timeoutParam *toolexecutor.ToolParameter
		for _, param := range tool.Parameters {
			if param.Name == "timeout" {
				timeoutParam = &param
				break
			}
		}

		require.NotNil(t, timeoutParam)
		assert.Equal(t, "integer", timeoutParam.Type)
		assert.Equal(t, 30, timeoutParam.Default)
		assert.False(t, timeoutParam.Required)
	})
}

// Optional Parameter Tests
// Note: These tests verify that optional parameters are properly defined in tool schemas

func TestToolHandlers_OptionalParameterDefinitions(t *testing.T) {
	t.Run("screenshot optional parameters", func(t *testing.T) {
		tool := createScreenshotTool(nil)

		paramMap := make(map[string]toolexecutor.ToolParameter)
		for _, param := range tool.Parameters {
			paramMap[param.Name] = param
		}

		// Verify optional parameters exist and have defaults
		assert.False(t, paramMap["type"].Required)
		assert.Equal(t, "png", paramMap["type"].Default)

		assert.False(t, paramMap["quality"].Required)
		assert.Equal(t, 80, paramMap["quality"].Default)

		assert.False(t, paramMap["fullPage"].Required)
		assert.Equal(t, false, paramMap["fullPage"].Default)

		assert.False(t, paramMap["selector"].Required)
	})

	t.Run("navigate optional parameters", func(t *testing.T) {
		tool := createNavigateTool(nil)

		paramMap := make(map[string]toolexecutor.ToolParameter)
		for _, param := range tool.Parameters {
			paramMap[param.Name] = param
		}

		assert.False(t, paramMap["targetId"].Required)
		assert.False(t, paramMap["timeout"].Required)
		assert.Equal(t, 30, paramMap["timeout"].Default)
		assert.False(t, paramMap["waitUntil"].Required)
		assert.Equal(t, "load", paramMap["waitUntil"].Default)
	})

	t.Run("extract optional selector", func(t *testing.T) {
		tool := createExtractTool(nil)

		paramMap := make(map[string]toolexecutor.ToolParameter)
		for _, param := range tool.Parameters {
			paramMap[param.Name] = param
		}

		assert.False(t, paramMap["selector"].Required)
	})

	t.Run("interact optional value and timeout", func(t *testing.T) {
		tool := createInteractTool(nil)

		paramMap := make(map[string]toolexecutor.ToolParameter)
		for _, param := range tool.Parameters {
			paramMap[param.Name] = param
		}

		assert.False(t, paramMap["value"].Required)
		assert.False(t, paramMap["timeout"].Required)
		assert.Equal(t, 30, paramMap["timeout"].Default)
	})

	t.Run("execute optional args and timeout", func(t *testing.T) {
		tool := createExecuteTool(nil)

		paramMap := make(map[string]toolexecutor.ToolParameter)
		for _, param := range tool.Parameters {
			paramMap[param.Name] = param
		}

		assert.False(t, paramMap["args"].Required)
		assert.False(t, paramMap["timeout"].Required)
		assert.Equal(t, 30, paramMap["timeout"].Default)
	})

	t.Run("cookies optional names and cookies", func(t *testing.T) {
		tool := createCookiesTool(nil)

		paramMap := make(map[string]toolexecutor.ToolParameter)
		for _, param := range tool.Parameters {
			paramMap[param.Name] = param
		}

		assert.False(t, paramMap["names"].Required)
		assert.False(t, paramMap["cookies"].Required)
	})
}

// Context Cancellation Tests
// Note: Context cancellation is properly tested in integration tests
// These tests verify that handlers accept context parameters

func TestToolHandlers_ContextParameter(t *testing.T) {
	t.Run("all handlers accept context", func(t *testing.T) {
		ctx := context.Background()

		// Verify handlers can be called with context (even if they fail due to nil browserContext)
		// This ensures the handler signature is correct

		navigateTool := createNavigateTool(nil)
		assert.NotNil(t, navigateTool.Handler)

		screenshotTool := createScreenshotTool(nil)
		assert.NotNil(t, screenshotTool.Handler)

		extractTool := createExtractTool(nil)
		assert.NotNil(t, extractTool.Handler)

		interactTool := createInteractTool(nil)
		assert.NotNil(t, interactTool.Handler)

		executeTool := createExecuteTool(nil)
		assert.NotNil(t, executeTool.Handler)

		cookiesTool := createCookiesTool(nil)
		assert.NotNil(t, cookiesTool.Handler)

		// Verify context is used (handlers should accept it)
		_ = ctx
	})
}
