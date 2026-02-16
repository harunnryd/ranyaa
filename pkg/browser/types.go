package browser

import (
	"time"
)

// BrowserProfile represents a browser profile configuration
type BrowserProfile struct {
	Name        string   `json:"name"`
	CDPPort     int      `json:"cdpPort"`
	CDPUrl      string   `json:"cdpUrl,omitempty"`
	Headless    bool     `json:"headless"`
	NoSandbox   bool     `json:"noSandbox"`
	AttachOnly  bool     `json:"attachOnly"`
	UserDataDir string   `json:"userDataDir,omitempty"`
	Color       string   `json:"color,omitempty"`
	ChromePath  string   `json:"chromePath,omitempty"`
	Args        []string `json:"args,omitempty"`
}

// ResolvedBrowserProfile represents a profile with computed paths
type ResolvedBrowserProfile struct {
	BrowserProfile
	UserDataDir string `json:"userDataDir"` // Computed absolute path
}

// BrowserTab represents an open browser tab
type BrowserTab struct {
	TargetID string `json:"targetId"`
	URL      string `json:"url"`
	Title    string `json:"title"`
}

// PageState tracks console messages, errors, and network requests for a page
type PageState struct {
	TargetID        string           `json:"targetId"`
	ConsoleMessages []ConsoleMessage `json:"consoleMessages"`
	Errors          []PageError      `json:"errors"`
	NetworkRequests []NetworkRequest `json:"networkRequests"`
}

// ConsoleMessage represents a console log message
type ConsoleMessage struct {
	Type      string    `json:"type"`
	Text      string    `json:"text"`
	Timestamp time.Time `json:"timestamp"`
}

// PageError represents a page error
type PageError struct {
	Message   string    `json:"message"`
	Timestamp time.Time `json:"timestamp"`
}

// NetworkRequest represents a network request
type NetworkRequest struct {
	URL       string    `json:"url"`
	Method    string    `json:"method"`
	Status    int       `json:"status"`
	Timestamp time.Time `json:"timestamp"`
	Duration  int64     `json:"duration"` // milliseconds
}

// Cookie represents a browser cookie
type Cookie struct {
	Name     string    `json:"name"`
	Value    string    `json:"value"`
	Domain   string    `json:"domain"`
	Path     string    `json:"path"`
	Expires  time.Time `json:"expires,omitempty"`
	HTTPOnly bool      `json:"httpOnly"`
	Secure   bool      `json:"secure"`
	SameSite string    `json:"sameSite,omitempty"`
}

// BrowserConfig represents the browser system configuration
type BrowserConfig struct {
	Profiles []BrowserProfile `json:"profiles"`
	Pool     PoolConfig       `json:"pool"`
	Security SecurityConfig   `json:"security"`
}

// PoolConfig represents page pool configuration
type PoolConfig struct {
	MaxPages           int `json:"maxPages"`
	MaxMemoryMB        int `json:"maxMemoryMB"`
	IdlePageTimeout    int `json:"idlePageTimeout"`    // seconds
	IdleBrowserTimeout int `json:"idleBrowserTimeout"` // seconds
}

// SecurityConfig represents security configuration
type SecurityConfig struct {
	AllowFileUrls      bool     `json:"allowFileUrls"`
	AllowLocalhostUrls bool     `json:"allowLocalhostUrls"`
	AllowedDomains     []string `json:"allowedDomains,omitempty"`
	BlockedDomains     []string `json:"blockedDomains,omitempty"`
}

// NavigateParams represents navigation parameters
type NavigateParams struct {
	URL       string `json:"url"`
	Timeout   int    `json:"timeout,omitempty"`   // seconds
	WaitUntil string `json:"waitUntil,omitempty"` // "load", "domcontentloaded", "networkidle"
}

// ScreenshotParams represents screenshot parameters
type ScreenshotParams struct {
	Type     string `json:"type,omitempty"`    // "png" or "jpeg"
	Quality  int    `json:"quality,omitempty"` // 0-100 for jpeg
	FullPage bool   `json:"fullPage,omitempty"`
	Selector string `json:"selector,omitempty"`
}

// ExtractParams represents content extraction parameters
type ExtractParams struct {
	Type     string `json:"type"`               // "html", "text", "metadata", "links", "selector"
	Selector string `json:"selector,omitempty"` // for "selector" type
}

// ExecuteParams represents JavaScript execution parameters
type ExecuteParams struct {
	Script  string        `json:"script"`
	Args    []interface{} `json:"args,omitempty"`
	Timeout int           `json:"timeout,omitempty"` // seconds
}

// InteractParams represents element interaction parameters
type InteractParams struct {
	Action   string `json:"action"` // "click", "type", "select", "wait", "visible"
	Selector string `json:"selector"`
	Value    string `json:"value,omitempty"`
	Timeout  int    `json:"timeout,omitempty"` // seconds
}

// GetCookiesParams represents cookie retrieval parameters
type GetCookiesParams struct {
	Names []string `json:"names,omitempty"`
}

// SetCookiesParams represents cookie setting parameters
type SetCookiesParams struct {
	Cookies []Cookie `json:"cookies"`
}

// DeleteCookiesParams represents cookie deletion parameters
type DeleteCookiesParams struct {
	Names []string `json:"names"`
}

// NavigateResult represents navigation result
type NavigateResult struct {
	URL      string `json:"url"`
	Title    string `json:"title"`
	Status   int    `json:"status"`
	Duration int64  `json:"duration"` // milliseconds
}

// ScreenshotResult represents screenshot result
type ScreenshotResult struct {
	Image  string `json:"image"` // Base64 encoded
	Format string `json:"format"`
	Size   struct {
		Width  int `json:"width"`
		Height int `json:"height"`
	} `json:"size"`
}

// ExtractResult represents content extraction result
type ExtractResult struct {
	Type     string      `json:"type"`
	Data     interface{} `json:"data"`
	Selector string      `json:"selector,omitempty"`
}

// ExecuteResult represents JavaScript execution result
type ExecuteResult struct {
	Result   interface{} `json:"result"`
	Duration int64       `json:"duration"` // milliseconds
}

// InteractResult represents element interaction result
type InteractResult struct {
	Action   string `json:"action"`
	Selector string `json:"selector"`
	Success  bool   `json:"success"`
	Duration int64  `json:"duration"` // milliseconds
}

// Metadata represents page metadata
type Metadata struct {
	Title        string `json:"title"`
	Description  string `json:"description,omitempty"`
	CanonicalURL string `json:"canonicalUrl,omitempty"`
}

// Link represents an anchor element
type Link struct {
	Href string `json:"href"`
	Text string `json:"text"`
}

// BrowserMetrics represents browser system metrics
type BrowserMetrics struct {
	ActiveProfiles int                       `json:"activeProfiles"`
	ActivePages    int                       `json:"activePages"`
	TotalOps       int64                     `json:"totalOps"`
	Latency        LatencyMetrics            `json:"latency"`
	Errors         map[string]int64          `json:"errors"`
	ProfileMetrics map[string]ProfileMetrics `json:"profileMetrics"`
}

// LatencyMetrics represents operation latency metrics
type LatencyMetrics struct {
	Avg int64 `json:"avg"` // milliseconds
	Min int64 `json:"min"`
	Max int64 `json:"max"`
	P95 int64 `json:"p95"`
	P99 int64 `json:"p99"`
}

// ProfileMetrics represents per-profile metrics
type ProfileMetrics struct {
	ActivePages int   `json:"activePages"`
	TotalOps    int64 `json:"totalOps"`
	Errors      int64 `json:"errors"`
}

// Error types
type BrowserError struct {
	Code    string      `json:"code"`
	Message string      `json:"message"`
	Details interface{} `json:"details,omitempty"`
}

func (e *BrowserError) Error() string {
	return e.Message
}

// Error codes
const (
	ErrCodeValidation      = "VALIDATION_ERROR"
	ErrCodeNavigation      = "NAVIGATION_ERROR"
	ErrCodeTimeout         = "TIMEOUT_ERROR"
	ErrCodeElementNotFound = "ELEMENT_NOT_FOUND"
	ErrCodeScriptExecution = "SCRIPT_EXECUTION_ERROR"
	ErrCodeSecurity        = "SECURITY_ERROR"
	ErrCodeBrowserCrash    = "BROWSER_CRASH"
	ErrCodeConfiguration   = "CONFIGURATION_ERROR"
	ErrCodeNotFound        = "NOT_FOUND"
)
