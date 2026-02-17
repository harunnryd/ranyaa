package browser

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/go-rod/rod"
)

// ProfileContext manages a single browser profile
type ProfileContext struct {
	profile        *ResolvedBrowserProfile
	processManager *ProcessManager
	session        *SessionManager
	security       *SecurityValidator
	commandQueue   interface{} // Reference to command queue (will be set during integration)
	mu             sync.RWMutex
	isAvailable    bool
	metrics        *ProfileMetrics
}

// NewProfileContext creates a new profile context
func NewProfileContext(profile *ResolvedBrowserProfile, poolConfig PoolConfig, securityConfig SecurityConfig) *ProfileContext {
	pc := &ProfileContext{
		profile:        profile,
		processManager: NewProcessManager(profile),
		security:       NewSecurityValidator(securityConfig),
		isAvailable:    false,
		metrics: &ProfileMetrics{
			ActivePages: 0,
			TotalOps:    0,
			Errors:      0,
		},
	}

	return pc
}

// EnsureBrowserAvailable starts the browser if not running
func (pc *ProfileContext) EnsureBrowserAvailable(ctx context.Context) error {
	pc.mu.Lock()
	defer pc.mu.Unlock()

	if pc.isAvailable && pc.session != nil {
		// Check health
		if err := pc.processManager.CheckHealth(ctx); err == nil {
			return nil
		}
		// Browser crashed, need to restart
		pc.isAvailable = false
		pc.session = nil
	}

	// Start browser
	var browser *rod.Browser
	var err error

	if pc.profile.AttachOnly {
		// Attach to existing browser
		browser, err = pc.processManager.AttachToExisting(ctx)
	} else {
		// Spawn new Chrome process
		if err := pc.processManager.SpawnChrome(ctx); err != nil {
			return err
		}

		// Connect to CDP
		browser, err = pc.processManager.ConnectCDP(ctx)
	}

	if err != nil {
		return err
	}

	// Create session manager
	pc.session = NewSessionManager(browser, pc.profile)

	pc.isAvailable = true

	return nil
}

// StopRunningBrowser stops the browser and cleans up
func (pc *ProfileContext) StopRunningBrowser() error {
	pc.mu.Lock()
	defer pc.mu.Unlock()

	if !pc.isAvailable {
		return nil
	}

	// Close session
	if pc.session != nil {
		pc.session.Close()
		pc.session = nil
	}

	// Kill Chrome process
	if err := pc.processManager.KillChrome(); err != nil {
		return err
	}

	pc.isAvailable = false

	return nil
}

// ResetProfile moves user data dir and creates fresh profile
func (pc *ProfileContext) ResetProfile(ctx context.Context) error {
	// Stop browser first
	if err := pc.StopRunningBrowser(); err != nil {
		return err
	}

	pc.mu.Lock()
	defer pc.mu.Unlock()

	// Move old user data directory to backup
	oldDir := pc.profile.UserDataDir
	if _, err := os.Stat(oldDir); err == nil {
		backupDir := fmt.Sprintf("%s.backup.%d", oldDir, time.Now().Unix())
		if err := os.Rename(oldDir, backupDir); err != nil {
			return &BrowserError{
				Code:    ErrCodeConfiguration,
				Message: fmt.Sprintf("Failed to backup user data directory: %v", err),
			}
		}
	}

	// Create new empty user data directory
	if err := os.MkdirAll(oldDir, 0755); err != nil {
		return &BrowserError{
			Code:    ErrCodeConfiguration,
			Message: fmt.Sprintf("Failed to create user data directory: %v", err),
		}
	}

	// Decorate profile
	if err := pc.processManager.DecorateProfile(); err != nil {
		return &BrowserError{
			Code:    ErrCodeConfiguration,
			Message: fmt.Sprintf("Failed to decorate profile: %v", err),
		}
	}

	return nil
}

// ListTabs returns all tabs for the profile
func (pc *ProfileContext) ListTabs(ctx context.Context) ([]BrowserTab, error) {
	if err := pc.EnsureBrowserAvailable(ctx); err != nil {
		return nil, err
	}

	return pc.session.ListPages()
}

// OpenTab creates a new tab with URL
func (pc *ProfileContext) OpenTab(ctx context.Context, url string) (*BrowserTab, error) {
	if err := pc.EnsureBrowserAvailable(ctx); err != nil {
		return nil, err
	}

	// Validate URL
	if url != "" {
		if err := pc.security.ValidateURL(url); err != nil {
			return nil, err
		}
	}

	page, err := pc.session.CreatePage(ctx, url)
	if err != nil {
		return nil, err
	}

	info, _ := page.Info()
	targetID := string(page.TargetID)

	pc.metrics.ActivePages++

	return &BrowserTab{
		TargetID: targetID,
		URL:      info.URL,
		Title:    info.Title,
	}, nil
}

// FocusTab brings a tab to the foreground
func (pc *ProfileContext) FocusTab(ctx context.Context, targetID string) error {
	if err := pc.EnsureBrowserAvailable(ctx); err != nil {
		return err
	}

	page, err := pc.session.GetPage(targetID)
	if err != nil {
		return err
	}

	// Activate the page
	page.MustActivate()
	return nil
}

// CloseTab closes a specific tab
func (pc *ProfileContext) CloseTab(ctx context.Context, targetID string) error {
	if err := pc.EnsureBrowserAvailable(ctx); err != nil {
		return err
	}

	if err := pc.session.ClosePage(targetID); err != nil {
		return err
	}

	pc.metrics.ActivePages--

	return nil
}

// EnsureTabAvailable gets or creates a tab
func (pc *ProfileContext) EnsureTabAvailable(ctx context.Context) (*rod.Page, string, error) {
	if err := pc.EnsureBrowserAvailable(ctx); err != nil {
		return nil, "", err
	}

	// Get existing pages
	tabs, err := pc.session.ListPages()
	if err != nil {
		return nil, "", err
	}

	// If we have pages, use the first one
	if len(tabs) > 0 {
		page, err := pc.session.GetPage(tabs[0].TargetID)
		if err != nil {
			return nil, "", err
		}
		return page, tabs[0].TargetID, nil
	}

	// Create a new page
	page, err := pc.session.CreatePage(ctx, "")
	if err != nil {
		return nil, "", err
	}

	targetID := string(page.TargetID)
	return page, targetID, nil
}

// Navigate navigates to a URL
func (pc *ProfileContext) Navigate(ctx context.Context, targetID string, params NavigateParams) (*NavigateResult, error) {
	if err := pc.EnsureBrowserAvailable(ctx); err != nil {
		return nil, err
	}

	// Validate URL
	if err := pc.security.ValidateURL(params.URL); err != nil {
		pc.metrics.Errors++
		return nil, err
	}

	// Validate timeout
	if params.Timeout > 0 {
		if err := ValidateNavigationTimeout(params.Timeout); err != nil {
			pc.metrics.Errors++
			return nil, err
		}
	}

	page, err := pc.session.GetPage(targetID)
	if err != nil {
		pc.metrics.Errors++
		return nil, err
	}

	result, err := pc.session.Navigate(ctx, page, params)
	if err != nil {
		pc.metrics.Errors++
		return nil, err
	}

	pc.metrics.TotalOps++

	return result, nil
}

// Screenshot captures a screenshot
func (pc *ProfileContext) Screenshot(ctx context.Context, targetID string, params ScreenshotParams) (*ScreenshotResult, error) {
	if err := pc.EnsureBrowserAvailable(ctx); err != nil {
		return nil, err
	}

	// Validate params
	if err := ValidateScreenshotParams(params); err != nil {
		pc.metrics.Errors++
		return nil, err
	}

	page, err := pc.session.GetPage(targetID)
	if err != nil {
		pc.metrics.Errors++
		return nil, err
	}

	result, err := pc.session.Screenshot(page, params)
	if err != nil {
		pc.metrics.Errors++
		return nil, err
	}

	pc.metrics.TotalOps++

	return result, nil
}

// Extract extracts content from the page
func (pc *ProfileContext) Extract(ctx context.Context, targetID string, params ExtractParams) (*ExtractResult, error) {
	if err := pc.EnsureBrowserAvailable(ctx); err != nil {
		return nil, err
	}

	// Validate params
	if err := ValidateExtractParams(params); err != nil {
		pc.metrics.Errors++
		return nil, err
	}

	page, err := pc.session.GetPage(targetID)
	if err != nil {
		pc.metrics.Errors++
		return nil, err
	}

	var data interface{}
	var extractErr error

	switch params.Type {
	case "html":
		data, extractErr = pc.session.ExtractHTML(page)
	case "text":
		data, extractErr = pc.session.ExtractText(page)
	case "metadata":
		data, extractErr = pc.session.ExtractMetadata(page)
	case "links":
		data, extractErr = pc.session.ExtractLinks(page)
	case "selector":
		data, extractErr = pc.session.ExtractBySelector(page, params.Selector)
	default:
		extractErr = &BrowserError{
			Code:    ErrCodeValidation,
			Message: fmt.Sprintf("Invalid extract type: %s", params.Type),
		}
	}

	if extractErr != nil {
		pc.metrics.Errors++
		return nil, extractErr
	}

	pc.metrics.TotalOps++

	return &ExtractResult{
		Type:     params.Type,
		Data:     data,
		Selector: params.Selector,
	}, nil
}

// Execute executes JavaScript
func (pc *ProfileContext) Execute(ctx context.Context, targetID string, params ExecuteParams) (*ExecuteResult, error) {
	if err := pc.EnsureBrowserAvailable(ctx); err != nil {
		return nil, err
	}

	// Validate params
	if err := ValidateExecuteParams(params); err != nil {
		pc.metrics.Errors++
		return nil, err
	}

	page, err := pc.session.GetPage(targetID)
	if err != nil {
		pc.metrics.Errors++
		return nil, err
	}

	result, err := pc.session.ExecuteScript(ctx, page, params)
	if err != nil {
		pc.metrics.Errors++
		return nil, err
	}

	pc.metrics.TotalOps++

	return result, nil
}

// Interact performs element interaction
func (pc *ProfileContext) Interact(ctx context.Context, targetID string, params InteractParams) (*InteractResult, error) {
	if err := pc.EnsureBrowserAvailable(ctx); err != nil {
		return nil, err
	}

	// Validate params
	if err := ValidateInteractParams(params); err != nil {
		pc.metrics.Errors++
		return nil, err
	}

	// Validate selector
	if !IsValidSelector(params.Selector) {
		pc.metrics.Errors++
		return nil, &BrowserError{
			Code:    ErrCodeValidation,
			Message: "Invalid selector",
		}
	}

	page, err := pc.session.GetPage(targetID)
	if err != nil {
		pc.metrics.Errors++
		return nil, err
	}

	var result *InteractResult
	var interactErr error

	switch params.Action {
	case "click":
		result, interactErr = pc.session.Click(page, params.Selector, params.Timeout)
	case "type":
		result, interactErr = pc.session.Type(page, params.Selector, params.Value, params.Timeout)
	case "select":
		result, interactErr = pc.session.Select(page, params.Selector, params.Value, params.Timeout)
	case "wait":
		result, interactErr = pc.session.WaitForElement(page, params.Selector, params.Timeout)
	case "visible":
		visible, err := pc.session.IsVisible(page, params.Selector)
		if err != nil {
			interactErr = err
		} else {
			result = &InteractResult{
				Action:   "visible",
				Selector: params.Selector,
				Success:  visible,
				Duration: 0,
			}
		}
	default:
		interactErr = &BrowserError{
			Code:    ErrCodeValidation,
			Message: fmt.Sprintf("Invalid interaction action: %s", params.Action),
		}
	}

	if interactErr != nil {
		pc.metrics.Errors++
		return nil, interactErr
	}

	pc.metrics.TotalOps++

	return result, nil
}

// GetCookies retrieves cookies
func (pc *ProfileContext) GetCookies(ctx context.Context, targetID string, params GetCookiesParams) ([]Cookie, error) {
	if err := pc.EnsureBrowserAvailable(ctx); err != nil {
		return nil, err
	}

	page, err := pc.session.GetPage(targetID)
	if err != nil {
		pc.metrics.Errors++
		return nil, err
	}

	cookies, err := pc.session.GetCookies(page, params.Names)
	if err != nil {
		pc.metrics.Errors++
		return nil, err
	}

	pc.metrics.TotalOps++

	return cookies, nil
}

// SetCookies sets cookies
func (pc *ProfileContext) SetCookies(ctx context.Context, targetID string, params SetCookiesParams) error {
	if err := pc.EnsureBrowserAvailable(ctx); err != nil {
		return err
	}

	// Validate cookies
	for _, cookie := range params.Cookies {
		if err := ValidateCookieParams(cookie); err != nil {
			pc.metrics.Errors++
			return err
		}
	}

	page, err := pc.session.GetPage(targetID)
	if err != nil {
		pc.metrics.Errors++
		return err
	}

	if err := pc.session.SetCookies(page, params.Cookies); err != nil {
		pc.metrics.Errors++
		return err
	}

	pc.metrics.TotalOps++

	return nil
}

// DeleteCookies deletes cookies
func (pc *ProfileContext) DeleteCookies(ctx context.Context, targetID string, params DeleteCookiesParams) error {
	if err := pc.EnsureBrowserAvailable(ctx); err != nil {
		return err
	}

	page, err := pc.session.GetPage(targetID)
	if err != nil {
		pc.metrics.Errors++
		return err
	}

	if err := pc.session.DeleteCookies(page, params.Names); err != nil {
		pc.metrics.Errors++
		return err
	}

	pc.metrics.TotalOps++

	return nil
}

// ClearCookies clears all cookies
func (pc *ProfileContext) ClearCookies(ctx context.Context, targetID string) error {
	if err := pc.EnsureBrowserAvailable(ctx); err != nil {
		return err
	}

	page, err := pc.session.GetPage(targetID)
	if err != nil {
		pc.metrics.Errors++
		return err
	}

	if err := pc.session.ClearCookies(page); err != nil {
		pc.metrics.Errors++
		return err
	}

	pc.metrics.TotalOps++

	return nil
}

// GetPageState retrieves page state
func (pc *ProfileContext) GetPageState(ctx context.Context, targetID string) (*PageState, error) {
	if err := pc.EnsureBrowserAvailable(ctx); err != nil {
		return nil, err
	}

	return pc.session.GetPageState(targetID)
}

// GetMetrics returns profile metrics
func (pc *ProfileContext) GetMetrics() *ProfileMetrics {
	pc.mu.RLock()
	defer pc.mu.RUnlock()

	metricsCopy := *pc.metrics
	return &metricsCopy
}

// GetProfile returns the profile configuration
func (pc *ProfileContext) GetProfile() *ResolvedBrowserProfile {
	return pc.profile
}

// IsAvailable returns whether the browser is available
func (pc *ProfileContext) IsAvailable() bool {
	pc.mu.RLock()
	defer pc.mu.RUnlock()
	return pc.isAvailable
}

// GetPoolStats returns page pool statistics
func (pc *ProfileContext) GetPoolStats() map[string]interface{} {
	// Pool not implemented yet - return basic stats
	return map[string]interface{}{
		"activePages": pc.metrics.ActivePages,
		"totalOps":    pc.metrics.TotalOps,
	}
}

// SetCommandQueue sets the command queue reference
func (pc *ProfileContext) SetCommandQueue(queue interface{}) {
	pc.commandQueue = queue
}

// EnqueueOperation enqueues an operation in the profile's command queue lane
func (pc *ProfileContext) EnqueueOperation(operation func() (interface{}, error)) (interface{}, error) {
	// This will be implemented during command queue integration
	// For now, execute directly
	return operation()
}

// GetLaneName returns the command queue lane name for this profile
func (pc *ProfileContext) GetLaneName() string {
	return fmt.Sprintf("browser-profile-%s", pc.profile.Name)
}

// Cleanup performs cleanup when shutting down
func (pc *ProfileContext) Cleanup() error {
	return pc.StopRunningBrowser()
}
