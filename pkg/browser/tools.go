package browser

import (
	"context"
	"fmt"

	"github.com/harun/ranya/pkg/toolexecutor"
)

// RegisterBrowserTools registers all browser tools with the tool executor
func RegisterBrowserTools(executor *toolexecutor.ToolExecutor, browserContext *BrowserServerContext) error {
	tools := []toolexecutor.ToolDefinition{
		createNavigateTool(browserContext),
		createScreenshotTool(browserContext),
		createExtractTool(browserContext),
		createInteractTool(browserContext),
		createExecuteTool(browserContext),
		createCookiesTool(browserContext),
	}

	for _, tool := range tools {
		if err := executor.RegisterTool(tool); err != nil {
			return fmt.Errorf("failed to register tool %s: %w", tool.Name, err)
		}
	}

	return nil
}

// createNavigateTool creates the browser_navigate tool definition
func createNavigateTool(browserContext *BrowserServerContext) toolexecutor.ToolDefinition {
	return toolexecutor.ToolDefinition{
		Name:        "browser_navigate",
		Description: "Navigate to a URL in a browser profile. Validates URL security before navigation.",
		Parameters: []toolexecutor.ToolParameter{
			{
				Name:        "profile",
				Type:        "string",
				Description: "Browser profile name to use",
				Required:    true,
			},
			{
				Name:        "url",
				Type:        "string",
				Description: "URL to navigate to",
				Required:    true,
			},
			{
				Name:        "targetId",
				Type:        "string",
				Description: "Target ID of the tab to navigate (optional, will use or create a tab if not provided)",
				Required:    false,
			},
			{
				Name:        "timeout",
				Type:        "integer",
				Description: "Navigation timeout in seconds (default: 30)",
				Required:    false,
				Default:     30,
			},
			{
				Name:        "waitUntil",
				Type:        "string",
				Description: "Wait condition: 'load', 'domcontentloaded', or 'networkidle' (default: 'load')",
				Required:    false,
				Default:     "load",
			},
		},
		Handler: handleNavigate(browserContext),
	}
}

// createScreenshotTool creates the browser_screenshot tool definition
func createScreenshotTool(browserContext *BrowserServerContext) toolexecutor.ToolDefinition {
	return toolexecutor.ToolDefinition{
		Name:        "browser_screenshot",
		Description: "Capture a screenshot of a browser page. Returns base64-encoded image data.",
		Parameters: []toolexecutor.ToolParameter{
			{
				Name:        "profile",
				Type:        "string",
				Description: "Browser profile name to use",
				Required:    true,
			},
			{
				Name:        "targetId",
				Type:        "string",
				Description: "Target ID of the tab to screenshot",
				Required:    true,
			},
			{
				Name:        "type",
				Type:        "string",
				Description: "Image format: 'png' or 'jpeg' (default: 'png')",
				Required:    false,
				Default:     "png",
			},
			{
				Name:        "quality",
				Type:        "integer",
				Description: "JPEG quality 0-100 (only for jpeg format, default: 80)",
				Required:    false,
				Default:     80,
			},
			{
				Name:        "fullPage",
				Type:        "boolean",
				Description: "Capture full page screenshot (default: false)",
				Required:    false,
				Default:     false,
			},
			{
				Name:        "selector",
				Type:        "string",
				Description: "CSS selector to screenshot specific element (optional)",
				Required:    false,
			},
		},
		Handler: handleScreenshot(browserContext),
	}
}

// createExtractTool creates the browser_extract tool definition
func createExtractTool(browserContext *BrowserServerContext) toolexecutor.ToolDefinition {
	return toolexecutor.ToolDefinition{
		Name:        "browser_extract",
		Description: "Extract content from a browser page. Supports text, HTML, metadata, links, or selector-based extraction.",
		Parameters: []toolexecutor.ToolParameter{
			{
				Name:        "profile",
				Type:        "string",
				Description: "Browser profile name to use",
				Required:    true,
			},
			{
				Name:        "targetId",
				Type:        "string",
				Description: "Target ID of the tab to extract from",
				Required:    true,
			},
			{
				Name:        "type",
				Type:        "string",
				Description: "Extraction type: 'html', 'text', 'metadata', 'links', or 'selector'",
				Required:    true,
			},
			{
				Name:        "selector",
				Type:        "string",
				Description: "CSS selector for 'selector' type extraction",
				Required:    false,
			},
		},
		Handler: handleExtract(browserContext),
	}
}

// createInteractTool creates the browser_interact tool definition
func createInteractTool(browserContext *BrowserServerContext) toolexecutor.ToolDefinition {
	return toolexecutor.ToolDefinition{
		Name:        "browser_interact",
		Description: "Interact with elements on a browser page. Supports click, type, select, wait, and visibility checks.",
		Parameters: []toolexecutor.ToolParameter{
			{
				Name:        "profile",
				Type:        "string",
				Description: "Browser profile name to use",
				Required:    true,
			},
			{
				Name:        "targetId",
				Type:        "string",
				Description: "Target ID of the tab to interact with",
				Required:    true,
			},
			{
				Name:        "action",
				Type:        "string",
				Description: "Interaction action: 'click', 'type', 'select', 'wait', or 'visible'",
				Required:    true,
			},
			{
				Name:        "selector",
				Type:        "string",
				Description: "CSS selector of the element to interact with",
				Required:    true,
			},
			{
				Name:        "value",
				Type:        "string",
				Description: "Value for 'type' or 'select' actions",
				Required:    false,
			},
			{
				Name:        "timeout",
				Type:        "integer",
				Description: "Timeout in seconds (default: 30)",
				Required:    false,
				Default:     30,
			},
		},
		Handler: handleInteract(browserContext),
	}
}

// createExecuteTool creates the browser_execute tool definition
func createExecuteTool(browserContext *BrowserServerContext) toolexecutor.ToolDefinition {
	return toolexecutor.ToolDefinition{
		Name:        "browser_execute",
		Description: "Execute JavaScript code in a browser page context. Use with caution.",
		Parameters: []toolexecutor.ToolParameter{
			{
				Name:        "profile",
				Type:        "string",
				Description: "Browser profile name to use",
				Required:    true,
			},
			{
				Name:        "targetId",
				Type:        "string",
				Description: "Target ID of the tab to execute in",
				Required:    true,
			},
			{
				Name:        "script",
				Type:        "string",
				Description: "JavaScript code to execute",
				Required:    true,
			},
			{
				Name:        "args",
				Type:        "array",
				Description: "Arguments to pass to the script (optional)",
				Required:    false,
			},
			{
				Name:        "timeout",
				Type:        "integer",
				Description: "Execution timeout in seconds (default: 30)",
				Required:    false,
				Default:     30,
			},
		},
		Handler: handleExecute(browserContext),
	}
}

// createCookiesTool creates the browser_cookies tool definition
func createCookiesTool(browserContext *BrowserServerContext) toolexecutor.ToolDefinition {
	return toolexecutor.ToolDefinition{
		Name:        "browser_cookies",
		Description: "Manage browser cookies. Supports get, set, delete, and clear operations.",
		Parameters: []toolexecutor.ToolParameter{
			{
				Name:        "profile",
				Type:        "string",
				Description: "Browser profile name to use",
				Required:    true,
			},
			{
				Name:        "targetId",
				Type:        "string",
				Description: "Target ID of the tab",
				Required:    true,
			},
			{
				Name:        "action",
				Type:        "string",
				Description: "Cookie action: 'get', 'set', 'delete', or 'clear'",
				Required:    true,
			},
			{
				Name:        "names",
				Type:        "array",
				Description: "Cookie names for 'get' or 'delete' actions",
				Required:    false,
			},
			{
				Name:        "cookies",
				Type:        "array",
				Description: "Cookies to set for 'set' action (array of cookie objects)",
				Required:    false,
			},
		},
		Handler: handleCookies(browserContext),
	}
}

// Handler implementations

// handleNavigate creates the handler for browser_navigate tool
func handleNavigate(browserContext *BrowserServerContext) toolexecutor.ToolHandler {
	return func(ctx context.Context, params map[string]interface{}) (interface{}, error) {
		// Extract parameters
		profileName, ok := params["profile"].(string)
		if !ok {
			return nil, &BrowserError{Code: ErrCodeValidation, Message: "profile parameter is required"}
		}

		url, ok := params["url"].(string)
		if !ok {
			return nil, &BrowserError{Code: ErrCodeValidation, Message: "url parameter is required"}
		}

		// Get profile
		profile, err := browserContext.GetProfile(profileName)
		if err != nil {
			return nil, err
		}

		// Get or create target
		targetID, _ := params["targetId"].(string)
		if targetID == "" {
			// Ensure tab available
			_, newTargetID, err := profile.EnsureTabAvailable(ctx)
			if err != nil {
				return nil, err
			}
			targetID = newTargetID
		}

		// Build navigation params
		navParams := NavigateParams{
			URL: url,
		}

		if timeout, ok := params["timeout"].(float64); ok {
			navParams.Timeout = int(timeout)
		}

		if waitUntil, ok := params["waitUntil"].(string); ok {
			navParams.WaitUntil = waitUntil
		}

		// Execute navigation
		result, err := profile.Navigate(ctx, targetID, navParams)
		if err != nil {
			return nil, err
		}

		return map[string]interface{}{
			"targetId": targetID,
			"url":      result.URL,
			"title":    result.Title,
			"status":   result.Status,
			"duration": result.Duration,
		}, nil
	}
}

// handleScreenshot creates the handler for browser_screenshot tool
func handleScreenshot(browserContext *BrowserServerContext) toolexecutor.ToolHandler {
	return func(ctx context.Context, params map[string]interface{}) (interface{}, error) {
		// Extract parameters
		profileName, ok := params["profile"].(string)
		if !ok {
			return nil, &BrowserError{Code: ErrCodeValidation, Message: "profile parameter is required"}
		}

		targetID, ok := params["targetId"].(string)
		if !ok {
			return nil, &BrowserError{Code: ErrCodeValidation, Message: "targetId parameter is required"}
		}

		// Get profile
		profile, err := browserContext.GetProfile(profileName)
		if err != nil {
			return nil, err
		}

		// Build screenshot params
		screenshotParams := ScreenshotParams{}

		if imgType, ok := params["type"].(string); ok {
			screenshotParams.Type = imgType
		}

		if quality, ok := params["quality"].(float64); ok {
			screenshotParams.Quality = int(quality)
		}

		if fullPage, ok := params["fullPage"].(bool); ok {
			screenshotParams.FullPage = fullPage
		}

		if selector, ok := params["selector"].(string); ok {
			screenshotParams.Selector = selector
		}

		// Execute screenshot
		result, err := profile.Screenshot(ctx, targetID, screenshotParams)
		if err != nil {
			return nil, err
		}

		return result, nil
	}
}

// handleExtract creates the handler for browser_extract tool
func handleExtract(browserContext *BrowserServerContext) toolexecutor.ToolHandler {
	return func(ctx context.Context, params map[string]interface{}) (interface{}, error) {
		// Extract parameters
		profileName, ok := params["profile"].(string)
		if !ok {
			return nil, &BrowserError{Code: ErrCodeValidation, Message: "profile parameter is required"}
		}

		targetID, ok := params["targetId"].(string)
		if !ok {
			return nil, &BrowserError{Code: ErrCodeValidation, Message: "targetId parameter is required"}
		}

		extractType, ok := params["type"].(string)
		if !ok {
			return nil, &BrowserError{Code: ErrCodeValidation, Message: "type parameter is required"}
		}

		// Get profile
		profile, err := browserContext.GetProfile(profileName)
		if err != nil {
			return nil, err
		}

		// Build extract params
		extractParams := ExtractParams{
			Type: extractType,
		}

		if selector, ok := params["selector"].(string); ok {
			extractParams.Selector = selector
		}

		// Execute extraction
		result, err := profile.Extract(ctx, targetID, extractParams)
		if err != nil {
			return nil, err
		}

		return result, nil
	}
}

// handleInteract creates the handler for browser_interact tool
func handleInteract(browserContext *BrowserServerContext) toolexecutor.ToolHandler {
	return func(ctx context.Context, params map[string]interface{}) (interface{}, error) {
		// Extract parameters
		profileName, ok := params["profile"].(string)
		if !ok {
			return nil, &BrowserError{Code: ErrCodeValidation, Message: "profile parameter is required"}
		}

		targetID, ok := params["targetId"].(string)
		if !ok {
			return nil, &BrowserError{Code: ErrCodeValidation, Message: "targetId parameter is required"}
		}

		action, ok := params["action"].(string)
		if !ok {
			return nil, &BrowserError{Code: ErrCodeValidation, Message: "action parameter is required"}
		}

		selector, ok := params["selector"].(string)
		if !ok {
			return nil, &BrowserError{Code: ErrCodeValidation, Message: "selector parameter is required"}
		}

		// Get profile
		profile, err := browserContext.GetProfile(profileName)
		if err != nil {
			return nil, err
		}

		// Build interact params
		interactParams := InteractParams{
			Action:   action,
			Selector: selector,
		}

		if value, ok := params["value"].(string); ok {
			interactParams.Value = value
		}

		if timeout, ok := params["timeout"].(float64); ok {
			interactParams.Timeout = int(timeout)
		}

		// Execute interaction
		result, err := profile.Interact(ctx, targetID, interactParams)
		if err != nil {
			return nil, err
		}

		return result, nil
	}
}

// handleExecute creates the handler for browser_execute tool
func handleExecute(browserContext *BrowserServerContext) toolexecutor.ToolHandler {
	return func(ctx context.Context, params map[string]interface{}) (interface{}, error) {
		// Extract parameters
		profileName, ok := params["profile"].(string)
		if !ok {
			return nil, &BrowserError{Code: ErrCodeValidation, Message: "profile parameter is required"}
		}

		targetID, ok := params["targetId"].(string)
		if !ok {
			return nil, &BrowserError{Code: ErrCodeValidation, Message: "targetId parameter is required"}
		}

		script, ok := params["script"].(string)
		if !ok {
			return nil, &BrowserError{Code: ErrCodeValidation, Message: "script parameter is required"}
		}

		// Get profile
		profile, err := browserContext.GetProfile(profileName)
		if err != nil {
			return nil, err
		}

		// Build execute params
		executeParams := ExecuteParams{
			Script: script,
		}

		if args, ok := params["args"].([]interface{}); ok {
			executeParams.Args = args
		}

		if timeout, ok := params["timeout"].(float64); ok {
			executeParams.Timeout = int(timeout)
		}

		// Execute script
		result, err := profile.Execute(ctx, targetID, executeParams)
		if err != nil {
			return nil, err
		}

		return result, nil
	}
}

// handleCookies creates the handler for browser_cookies tool
func handleCookies(browserContext *BrowserServerContext) toolexecutor.ToolHandler {
	return func(ctx context.Context, params map[string]interface{}) (interface{}, error) {
		// Extract parameters
		profileName, ok := params["profile"].(string)
		if !ok {
			return nil, &BrowserError{Code: ErrCodeValidation, Message: "profile parameter is required"}
		}

		targetID, ok := params["targetId"].(string)
		if !ok {
			return nil, &BrowserError{Code: ErrCodeValidation, Message: "targetId parameter is required"}
		}

		action, ok := params["action"].(string)
		if !ok {
			return nil, &BrowserError{Code: ErrCodeValidation, Message: "action parameter is required"}
		}

		// Validate action before getting profile
		validActions := map[string]bool{
			"get":    true,
			"set":    true,
			"delete": true,
			"clear":  true,
		}
		if !validActions[action] {
			return nil, &BrowserError{
				Code:    ErrCodeValidation,
				Message: fmt.Sprintf("invalid cookie action: %s (must be 'get', 'set', 'delete', or 'clear')", action),
			}
		}

		// Validate action-specific parameters before getting profile
		if action == "delete" {
			namesParam, ok := params["names"].([]interface{})
			if !ok || len(namesParam) == 0 {
				return nil, &BrowserError{Code: ErrCodeValidation, Message: "names parameter is required for 'delete' action"}
			}
		}

		if action == "set" {
			_, ok := params["cookies"].([]interface{})
			if !ok {
				return nil, &BrowserError{Code: ErrCodeValidation, Message: "cookies parameter is required for 'set' action"}
			}
		}

		// Get profile
		profile, err := browserContext.GetProfile(profileName)
		if err != nil {
			return nil, err
		}

		// Execute cookie operation based on action
		switch action {
		case "get":
			var names []string
			if namesParam, ok := params["names"].([]interface{}); ok {
				for _, name := range namesParam {
					if nameStr, ok := name.(string); ok {
						names = append(names, nameStr)
					}
				}
			}

			cookies, err := profile.GetCookies(ctx, targetID, GetCookiesParams{Names: names})
			if err != nil {
				return nil, err
			}
			return map[string]interface{}{"cookies": cookies}, nil

		case "set":
			cookiesParam, ok := params["cookies"].([]interface{})
			if !ok {
				return nil, &BrowserError{Code: ErrCodeValidation, Message: "cookies parameter is required for 'set' action"}
			}

			var cookies []Cookie
			for _, c := range cookiesParam {
				cookieMap, ok := c.(map[string]interface{})
				if !ok {
					continue
				}

				cookie := Cookie{}
				if name, ok := cookieMap["name"].(string); ok {
					cookie.Name = name
				}
				if value, ok := cookieMap["value"].(string); ok {
					cookie.Value = value
				}
				if domain, ok := cookieMap["domain"].(string); ok {
					cookie.Domain = domain
				}
				if path, ok := cookieMap["path"].(string); ok {
					cookie.Path = path
				}
				if httpOnly, ok := cookieMap["httpOnly"].(bool); ok {
					cookie.HTTPOnly = httpOnly
				}
				if secure, ok := cookieMap["secure"].(bool); ok {
					cookie.Secure = secure
				}
				if sameSite, ok := cookieMap["sameSite"].(string); ok {
					cookie.SameSite = sameSite
				}

				cookies = append(cookies, cookie)
			}

			err := profile.SetCookies(ctx, targetID, SetCookiesParams{Cookies: cookies})
			if err != nil {
				return nil, err
			}
			return map[string]interface{}{"success": true}, nil

		case "delete":
			var names []string
			if namesParam, ok := params["names"].([]interface{}); ok {
				for _, name := range namesParam {
					if nameStr, ok := name.(string); ok {
						names = append(names, nameStr)
					}
				}
			}

			err := profile.DeleteCookies(ctx, targetID, DeleteCookiesParams{Names: names})
			if err != nil {
				return nil, err
			}
			return map[string]interface{}{"success": true}, nil

		case "clear":
			err := profile.ClearCookies(ctx, targetID)
			if err != nil {
				return nil, err
			}
			return map[string]interface{}{"success": true}, nil
		}

		return nil, nil
	}
}
