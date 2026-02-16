package browser

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
)

// SessionManager manages Rod browser sessions and pages
type SessionManager struct {
	browser    *rod.Browser
	profile    *ResolvedBrowserProfile
	pages      map[string]*rod.Page
	pageStates map[string]*PageState
	mu         sync.RWMutex
}

// NewSessionManager creates a new session manager
func NewSessionManager(browser *rod.Browser, profile *ResolvedBrowserProfile) *SessionManager {
	return &SessionManager{
		browser:    browser,
		profile:    profile,
		pages:      make(map[string]*rod.Page),
		pageStates: make(map[string]*PageState),
	}
}

// GetPage retrieves a page by target ID
func (sm *SessionManager) GetPage(targetID string) (*rod.Page, error) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	page, exists := sm.pages[targetID]
	if !exists {
		return nil, &BrowserError{
			Code:    ErrCodeNotFound,
			Message: fmt.Sprintf("Page not found: %s", targetID),
		}
	}

	return page, nil
}

// CreatePage creates a new page with optional URL
func (sm *SessionManager) CreatePage(ctx context.Context, url string) (*rod.Page, error) {
	page, err := sm.browser.Page(proto.TargetCreateTarget{})
	if err != nil {
		return nil, &BrowserError{
			Code:    ErrCodeBrowserCrash,
			Message: fmt.Sprintf("Failed to create page: %v", err),
		}
	}

	// Get target ID
	targetID := string(page.TargetID)

	// Store page
	sm.mu.Lock()
	sm.pages[targetID] = page
	sm.pageStates[targetID] = &PageState{
		TargetID:        targetID,
		ConsoleMessages: make([]ConsoleMessage, 0),
		Errors:          make([]PageError, 0),
		NetworkRequests: make([]NetworkRequest, 0),
	}
	sm.mu.Unlock()

	// Initialize page observers
	sm.initializePageObservers(page, targetID)

	// Navigate to URL if provided
	if url != "" {
		if err := page.Navigate(url); err != nil {
			return nil, &BrowserError{
				Code:    ErrCodeNavigation,
				Message: fmt.Sprintf("Failed to navigate to %s: %v", url, err),
			}
		}
		if err := page.WaitLoad(); err != nil {
			return nil, &BrowserError{
				Code:    ErrCodeTimeout,
				Message: fmt.Sprintf("Page load timeout: %v", err),
			}
		}
	}

	return page, nil
}

// ClosePage closes a page and cleans up state
func (sm *SessionManager) ClosePage(targetID string) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	page, exists := sm.pages[targetID]
	if !exists {
		return &BrowserError{
			Code:    ErrCodeNotFound,
			Message: fmt.Sprintf("Page not found: %s", targetID),
		}
	}

	// Close page
	if err := page.Close(); err != nil {
		return &BrowserError{
			Code:    ErrCodeBrowserCrash,
			Message: fmt.Sprintf("Failed to close page: %v", err),
		}
	}

	// Clean up
	delete(sm.pages, targetID)
	delete(sm.pageStates, targetID)

	return nil
}

// ListPages returns all pages as BrowserTab array
func (sm *SessionManager) ListPages() ([]BrowserTab, error) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	tabs := make([]BrowserTab, 0, len(sm.pages))
	for targetID, page := range sm.pages {
		info, err := page.Info()
		if err != nil {
			continue
		}

		tabs = append(tabs, BrowserTab{
			TargetID: targetID,
			URL:      info.URL,
			Title:    info.Title,
		})
	}

	return tabs, nil
}

// Navigate navigates to a URL with retry logic
func (sm *SessionManager) Navigate(ctx context.Context, page *rod.Page, params NavigateParams) (*NavigateResult, error) {
	start := time.Now()

	// Default timeout
	timeout := 30
	if params.Timeout > 0 {
		timeout = params.Timeout
	}

	// Validate timeout bounds (5-120 seconds)
	if timeout < 5 || timeout > 120 {
		return nil, &BrowserError{
			Code:    ErrCodeValidation,
			Message: fmt.Sprintf("Timeout must be between 5 and 120 seconds, got %d", timeout),
		}
	}

	// Default wait until
	waitUntil := "load"
	if params.WaitUntil != "" {
		waitUntil = params.WaitUntil
	}

	// Retry logic (up to 3 attempts)
	var lastErr error
	for attempt := 1; attempt <= 3; attempt++ {
		// Set timeout
		page = page.Timeout(time.Duration(timeout) * time.Second)

		// Navigate
		err := page.Navigate(params.URL)
		if err != nil {
			lastErr = err
			if attempt < 3 {
				// Exponential backoff
				time.Sleep(time.Duration(1<<uint(attempt)) * time.Second)
				continue
			}
			return nil, &BrowserError{
				Code:    ErrCodeNavigation,
				Message: fmt.Sprintf("Failed to navigate to %s after 3 attempts: %v", params.URL, err),
			}
		}

		// Wait for load state
		switch waitUntil {
		case "load":
			err = page.WaitLoad()
		case "domcontentloaded":
			err = page.WaitIdle(time.Second)
		case "networkidle":
			err = page.WaitIdle(2 * time.Second)
		}

		if err != nil {
			lastErr = err
			if attempt < 3 {
				time.Sleep(time.Duration(1<<uint(attempt)) * time.Second)
				continue
			}
			return nil, &BrowserError{
				Code:    ErrCodeTimeout,
				Message: fmt.Sprintf("Page load timeout: %v", err),
			}
		}

		// Success
		info, _ := page.Info()
		duration := time.Since(start).Milliseconds()

		return &NavigateResult{
			URL:      info.URL,
			Title:    info.Title,
			Status:   200, // Rod doesn't expose status code directly
			Duration: duration,
		}, nil
	}

	return nil, &BrowserError{
		Code:    ErrCodeNavigation,
		Message: fmt.Sprintf("Navigation failed: %v", lastErr),
	}
}

// GoBack navigates back in history
func (sm *SessionManager) GoBack(page *rod.Page) error {
	page.MustNavigateBack()
	return nil
}

// GoForward navigates forward in history
func (sm *SessionManager) GoForward(page *rod.Page) error {
	page.MustNavigateForward()
	return nil
}

// Reload reloads the current page
func (sm *SessionManager) Reload(page *rod.Page) error {
	page.MustReload()
	return nil
}

// ExtractHTML extracts page HTML content
func (sm *SessionManager) ExtractHTML(page *rod.Page) (string, error) {
	html, err := page.HTML()
	if err != nil {
		return "", &BrowserError{
			Code:    ErrCodeScriptExecution,
			Message: fmt.Sprintf("Failed to extract HTML: %v", err),
		}
	}
	return html, nil
}

// ExtractText extracts visible text without tags
func (sm *SessionManager) ExtractText(page *rod.Page) (string, error) {
	text, err := page.Eval(`() => document.body.innerText`)
	if err != nil {
		return "", &BrowserError{
			Code:    ErrCodeScriptExecution,
			Message: fmt.Sprintf("Failed to extract text: %v", err),
		}
	}
	return text.Value.String(), nil
}

// ExtractMetadata extracts page metadata
func (sm *SessionManager) ExtractMetadata(page *rod.Page) (*Metadata, error) {
	result, err := page.Eval(`() => {
		const title = document.title;
		const description = document.querySelector('meta[name="description"]')?.content || '';
		const canonical = document.querySelector('link[rel="canonical"]')?.href || '';
		return { title, description, canonicalUrl: canonical };
	}`)
	if err != nil {
		return nil, &BrowserError{
			Code:    ErrCodeScriptExecution,
			Message: fmt.Sprintf("Failed to extract metadata: %v", err),
		}
	}

	metadata := &Metadata{}
	result.Value.Unmarshal(metadata)
	return metadata, nil
}

// ExtractLinks extracts all anchor elements
func (sm *SessionManager) ExtractLinks(page *rod.Page) ([]Link, error) {
	result, err := page.Eval(`() => {
		const links = Array.from(document.querySelectorAll('a'));
		return links.map(a => ({ href: a.href, text: a.textContent.trim() }));
	}`)
	if err != nil {
		return nil, &BrowserError{
			Code:    ErrCodeScriptExecution,
			Message: fmt.Sprintf("Failed to extract links: %v", err),
		}
	}

	var links []Link
	result.Value.Unmarshal(&links)
	return links, nil
}

// ExtractBySelector extracts content by CSS selector
func (sm *SessionManager) ExtractBySelector(page *rod.Page, selector string) (string, error) {
	elem, err := page.Element(selector)
	if err != nil {
		return "", &BrowserError{
			Code:    ErrCodeElementNotFound,
			Message: fmt.Sprintf("Element not found: %s", selector),
		}
	}

	text, err := elem.Text()
	if err != nil {
		return "", &BrowserError{
			Code:    ErrCodeScriptExecution,
			Message: fmt.Sprintf("Failed to extract text from element: %v", err),
		}
	}

	return text, nil
}

// Screenshot captures a screenshot
func (sm *SessionManager) Screenshot(page *rod.Page, params ScreenshotParams) (*ScreenshotResult, error) {
	// Default format
	format := "png"
	if params.Type != "" {
		format = params.Type
	}

	var data []byte
	var err error

	if params.Selector != "" {
		// Element screenshot
		elem, err := page.Element(params.Selector)
		if err != nil {
			return nil, &BrowserError{
				Code:    ErrCodeElementNotFound,
				Message: fmt.Sprintf("Element not found: %s", params.Selector),
			}
		}
		data, err = elem.Screenshot(proto.PageCaptureScreenshotFormatPng, 0)
		if err != nil {
			return nil, &BrowserError{
				Code:    ErrCodeScriptExecution,
				Message: fmt.Sprintf("Failed to capture element screenshot: %v", err),
			}
		}
	} else if params.FullPage {
		// Full page screenshot
		data, err = page.Screenshot(params.FullPage, nil)
		if err != nil {
			return nil, &BrowserError{
				Code:    ErrCodeScriptExecution,
				Message: fmt.Sprintf("Failed to capture full page screenshot: %v", err),
			}
		}
	} else {
		// Viewport screenshot
		data, err = page.Screenshot(false, nil)
		if err != nil {
			return nil, &BrowserError{
				Code:    ErrCodeScriptExecution,
				Message: fmt.Sprintf("Failed to capture screenshot: %v", err),
			}
		}
	}

	// Encode to base64
	encoded := base64.StdEncoding.EncodeToString(data)

	// Get dimensions (approximate)
	result := &ScreenshotResult{
		Image:  encoded,
		Format: format,
		Size: struct {
			Width  int `json:"width"`
			Height int `json:"height"`
		}{
			Width:  1920, // Default, Rod doesn't expose actual dimensions easily
			Height: 1080,
		},
	}

	return result, nil
}

// ExecuteScript executes JavaScript with arguments
func (sm *SessionManager) ExecuteScript(ctx context.Context, page *rod.Page, params ExecuteParams) (*ExecuteResult, error) {
	start := time.Now()

	// Default timeout
	timeout := 30
	if params.Timeout > 0 {
		timeout = params.Timeout
	}

	// Set timeout
	page = page.Timeout(time.Duration(timeout) * time.Second)

	// Build function with arguments
	script := params.Script
	if len(params.Args) > 0 {
		// Wrap script to accept arguments
		script = fmt.Sprintf("(function(...args) { %s })", params.Script)
	}

	// Execute
	result, err := page.Eval(script, params.Args...)
	if err != nil {
		return nil, &BrowserError{
			Code:    ErrCodeScriptExecution,
			Message: fmt.Sprintf("Script execution failed: %v", err),
			Details: map[string]interface{}{
				"script": params.Script[:min(100, len(params.Script))],
			},
		}
	}

	duration := time.Since(start).Milliseconds()

	return &ExecuteResult{
		Result:   result.Value.Val(),
		Duration: duration,
	}, nil
}

// Click clicks an element by selector
func (sm *SessionManager) Click(page *rod.Page, selector string, timeout int) (*InteractResult, error) {
	start := time.Now()

	// Default timeout
	if timeout == 0 {
		timeout = 30
	}

	page = page.Timeout(time.Duration(timeout) * time.Second)

	elem, err := page.Element(selector)
	if err != nil {
		return nil, &BrowserError{
			Code:    ErrCodeElementNotFound,
			Message: fmt.Sprintf("Element not found: %s", selector),
			Details: map[string]interface{}{
				"selector": selector,
				"timeout":  timeout,
			},
		}
	}

	if err := elem.Click(proto.InputMouseButtonLeft, 1); err != nil {
		return nil, &BrowserError{
			Code:    ErrCodeScriptExecution,
			Message: fmt.Sprintf("Failed to click element: %v", err),
		}
	}

	duration := time.Since(start).Milliseconds()

	return &InteractResult{
		Action:   "click",
		Selector: selector,
		Success:  true,
		Duration: duration,
	}, nil
}

// Type types text into an input element
func (sm *SessionManager) Type(page *rod.Page, selector string, value string, timeout int) (*InteractResult, error) {
	start := time.Now()

	if timeout == 0 {
		timeout = 30
	}

	page = page.Timeout(time.Duration(timeout) * time.Second)

	elem, err := page.Element(selector)
	if err != nil {
		return nil, &BrowserError{
			Code:    ErrCodeElementNotFound,
			Message: fmt.Sprintf("Element not found: %s", selector),
		}
	}

	if err := elem.Input(value); err != nil {
		return nil, &BrowserError{
			Code:    ErrCodeScriptExecution,
			Message: fmt.Sprintf("Failed to type into element: %v", err),
		}
	}

	duration := time.Since(start).Milliseconds()

	return &InteractResult{
		Action:   "type",
		Selector: selector,
		Success:  true,
		Duration: duration,
	}, nil
}

// Select selects an option from a dropdown
func (sm *SessionManager) Select(page *rod.Page, selector string, value string, timeout int) (*InteractResult, error) {
	start := time.Now()

	if timeout == 0 {
		timeout = 30
	}

	page = page.Timeout(time.Duration(timeout) * time.Second)

	elem, err := page.Element(selector)
	if err != nil {
		return nil, &BrowserError{
			Code:    ErrCodeElementNotFound,
			Message: fmt.Sprintf("Element not found: %s", selector),
		}
	}

	if err := elem.Select([]string{value}, true, rod.SelectorTypeText); err != nil {
		return nil, &BrowserError{
			Code:    ErrCodeScriptExecution,
			Message: fmt.Sprintf("Failed to select option: %v", err),
		}
	}

	duration := time.Since(start).Milliseconds()

	return &InteractResult{
		Action:   "select",
		Selector: selector,
		Success:  true,
		Duration: duration,
	}, nil
}

// WaitForElement waits for an element to appear
func (sm *SessionManager) WaitForElement(page *rod.Page, selector string, timeout int) (*InteractResult, error) {
	start := time.Now()

	if timeout == 0 {
		timeout = 30
	}

	page = page.Timeout(time.Duration(timeout) * time.Second)

	_, err := page.Element(selector)
	if err != nil {
		return nil, &BrowserError{
			Code:    ErrCodeElementNotFound,
			Message: fmt.Sprintf("Element not found: %s", selector),
			Details: map[string]interface{}{
				"selector": selector,
				"timeout":  timeout,
			},
		}
	}

	duration := time.Since(start).Milliseconds()

	return &InteractResult{
		Action:   "wait",
		Selector: selector,
		Success:  true,
		Duration: duration,
	}, nil
}

// IsVisible checks if an element is visible
func (sm *SessionManager) IsVisible(page *rod.Page, selector string) (bool, error) {
	elem, err := page.Element(selector)
	if err != nil {
		return false, nil
	}

	visible, err := elem.Visible()
	if err != nil {
		return false, err
	}

	return visible, nil
}

// GetCookies retrieves cookies for the domain
func (sm *SessionManager) GetCookies(page *rod.Page, names []string) ([]Cookie, error) {
	cookies, err := page.Cookies([]string{})
	if err != nil {
		return nil, &BrowserError{
			Code:    ErrCodeScriptExecution,
			Message: fmt.Sprintf("Failed to get cookies: %v", err),
		}
	}

	result := make([]Cookie, 0)
	for _, c := range cookies {
		// Filter by names if provided
		if len(names) > 0 {
			found := false
			for _, name := range names {
				if c.Name == name {
					found = true
					break
				}
			}
			if !found {
				continue
			}
		}

		result = append(result, Cookie{
			Name:     c.Name,
			Value:    c.Value,
			Domain:   c.Domain,
			Path:     c.Path,
			Expires:  time.Unix(int64(c.Expires), 0),
			HTTPOnly: c.HTTPOnly,
			Secure:   c.Secure,
			SameSite: string(c.SameSite),
		})
	}

	return result, nil
}

// SetCookies sets cookies
func (sm *SessionManager) SetCookies(page *rod.Page, cookies []Cookie) error {
	protoCookies := make([]*proto.NetworkCookieParam, len(cookies))
	for i, c := range cookies {
		protoCookies[i] = &proto.NetworkCookieParam{
			Name:     c.Name,
			Value:    c.Value,
			Domain:   c.Domain,
			Path:     c.Path,
			Expires:  proto.TimeSinceEpoch(c.Expires.Unix()),
			HTTPOnly: c.HTTPOnly,
			Secure:   c.Secure,
			SameSite: proto.NetworkCookieSameSite(c.SameSite),
		}
	}

	if err := page.SetCookies(protoCookies); err != nil {
		return &BrowserError{
			Code:    ErrCodeScriptExecution,
			Message: fmt.Sprintf("Failed to set cookies: %v", err),
		}
	}

	return nil
}

// DeleteCookies deletes cookies by name
func (sm *SessionManager) DeleteCookies(page *rod.Page, names []string) error {
	// Get all cookies first
	cookies, err := page.Cookies([]string{})
	if err != nil {
		return &BrowserError{
			Code:    ErrCodeScriptExecution,
			Message: fmt.Sprintf("Failed to get cookies: %v", err),
		}
	}

	// Delete matching cookies
	for _, cookie := range cookies {
		for _, name := range names {
			if cookie.Name == name {
				if err := page.SetCookies([]*proto.NetworkCookieParam{
					{
						Name:    cookie.Name,
						Value:   "",
						Domain:  cookie.Domain,
						Path:    cookie.Path,
						Expires: proto.TimeSinceEpoch(0),
					},
				}); err != nil {
					return &BrowserError{
						Code:    ErrCodeScriptExecution,
						Message: fmt.Sprintf("Failed to delete cookie %s: %v", name, err),
					}
				}
			}
		}
	}
	return nil
}

// ClearCookies removes all cookies
func (sm *SessionManager) ClearCookies(page *rod.Page) error {
	cookies, err := page.Cookies([]string{})
	if err != nil {
		return &BrowserError{
			Code:    ErrCodeScriptExecution,
			Message: fmt.Sprintf("Failed to get cookies: %v", err),
		}
	}

	// Delete all cookies by setting them with expired time
	for _, c := range cookies {
		if err := page.SetCookies([]*proto.NetworkCookieParam{
			{
				Name:    c.Name,
				Value:   "",
				Domain:  c.Domain,
				Path:    c.Path,
				Expires: proto.TimeSinceEpoch(0),
			},
		}); err != nil {
			return &BrowserError{
				Code:    ErrCodeScriptExecution,
				Message: fmt.Sprintf("Failed to delete cookie: %v", err),
			}
		}
	}

	return nil
}

// initializePageObservers sets up event listeners for page state tracking
func (sm *SessionManager) initializePageObservers(page *rod.Page, targetID string) {
	// Console messages
	go page.EachEvent(func(e *proto.RuntimeConsoleAPICalled) {
		sm.trackConsoleMessage(targetID, e)
	})()

	// Page errors
	go page.EachEvent(func(e *proto.RuntimeExceptionThrown) {
		sm.trackPageError(targetID, e)
	})()

	// Network requests
	go page.EachEvent(func(e *proto.NetworkResponseReceived) {
		sm.trackNetworkRequest(targetID, e)
	})()
}

// trackConsoleMessage captures console log messages
func (sm *SessionManager) trackConsoleMessage(targetID string, event *proto.RuntimeConsoleAPICalled) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	state, exists := sm.pageStates[targetID]
	if !exists {
		return
	}

	// Build message text
	var text strings.Builder
	for i, arg := range event.Args {
		if i > 0 {
			text.WriteString(" ")
		}
		text.WriteString(fmt.Sprintf("%v", arg.Value))
	}

	msg := ConsoleMessage{
		Type:      string(event.Type),
		Text:      text.String(),
		Timestamp: time.Now(),
	}

	// Add to buffer (limit 100)
	state.ConsoleMessages = append(state.ConsoleMessages, msg)
	if len(state.ConsoleMessages) > 100 {
		state.ConsoleMessages = state.ConsoleMessages[1:]
	}
}

// trackPageError captures page errors
func (sm *SessionManager) trackPageError(targetID string, event *proto.RuntimeExceptionThrown) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	state, exists := sm.pageStates[targetID]
	if !exists {
		return
	}

	err := PageError{
		Message:   event.ExceptionDetails.Text,
		Timestamp: time.Now(),
	}

	// Add to buffer (limit 50)
	state.Errors = append(state.Errors, err)
	if len(state.Errors) > 50 {
		state.Errors = state.Errors[1:]
	}
}

// trackNetworkRequest captures network requests
func (sm *SessionManager) trackNetworkRequest(targetID string, event *proto.NetworkResponseReceived) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	state, exists := sm.pageStates[targetID]
	if !exists {
		return
	}

	req := NetworkRequest{
		URL:       event.Response.URL,
		Method:    "GET", // Default, Rod doesn't expose method easily
		Status:    event.Response.Status,
		Timestamp: time.Now(),
		Duration:  0, // Rod doesn't expose timing easily
	}

	// Add to buffer (limit 100)
	state.NetworkRequests = append(state.NetworkRequests, req)
	if len(state.NetworkRequests) > 100 {
		state.NetworkRequests = state.NetworkRequests[1:]
	}
}

// GetPageState retrieves page state
func (sm *SessionManager) GetPageState(targetID string) (*PageState, error) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	state, exists := sm.pageStates[targetID]
	if !exists {
		return nil, &BrowserError{
			Code:    ErrCodeNotFound,
			Message: fmt.Sprintf("Page state not found: %s", targetID),
		}
	}

	// Return copy
	stateCopy := *state
	return &stateCopy, nil
}

// GetCDPSession gets CDP session for a page
func (sm *SessionManager) GetCDPSession(page *rod.Page) interface{} {
	// Rod doesn't expose CDP session directly, but we can use the page's browser
	return page
}

// SendCDPCommand sends a CDP command
func (sm *SessionManager) SendCDPCommand(page *rod.Page, method string, params interface{}) (interface{}, error) {
	// Use Rod's Eval to send CDP commands
	result, err := page.Eval(fmt.Sprintf("() => { return %v }", params))
	if err != nil {
		return nil, &BrowserError{
			Code:    ErrCodeScriptExecution,
			Message: fmt.Sprintf("CDP command failed: %v", err),
		}
	}
	return result.Value.Val(), nil
}

// Close closes the session and all pages
func (sm *SessionManager) Close() error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	for targetID := range sm.pages {
		if page, exists := sm.pages[targetID]; exists {
			page.Close()
		}
	}

	sm.pages = make(map[string]*rod.Page)
	sm.pageStates = make(map[string]*PageState)

	return nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
