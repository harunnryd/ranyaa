package browser

import (
	"fmt"
	"net/url"
	"strings"
)

// SecurityValidator validates URLs and enforces security policies
type SecurityValidator struct {
	config SecurityConfig
}

// NewSecurityValidator creates a new security validator
func NewSecurityValidator(config SecurityConfig) *SecurityValidator {
	return &SecurityValidator{
		config: config,
	}
}

// ValidateURL validates a URL and checks security policies
func (sv *SecurityValidator) ValidateURL(urlStr string) error {
	// Parse URL
	parsedURL, err := url.Parse(urlStr)
	if err != nil {
		return &BrowserError{
			Code:    ErrCodeValidation,
			Message: fmt.Sprintf("Invalid URL format: %s", urlStr),
		}
	}

	// Check for file:// URLs
	if parsedURL.Scheme == "file" && !sv.config.AllowFileUrls {
		sv.logSecurityViolation("file_url_blocked", urlStr)
		return &BrowserError{
			Code:    ErrCodeSecurity,
			Message: "file:// URLs are not allowed",
			Details: map[string]interface{}{
				"url": urlStr,
			},
		}
	}

	// Check for localhost URLs
	if sv.isLocalhostURL(parsedURL) && !sv.config.AllowLocalhostUrls {
		sv.logSecurityViolation("localhost_url_blocked", urlStr)
		return &BrowserError{
			Code:    ErrCodeSecurity,
			Message: "localhost URLs are not allowed",
			Details: map[string]interface{}{
				"url": urlStr,
			},
		}
	}

	// Check allowed domains
	if len(sv.config.AllowedDomains) > 0 {
		if !sv.isDomainAllowed(parsedURL.Host) {
			sv.logSecurityViolation("domain_not_allowed", urlStr)
			return &BrowserError{
				Code:    ErrCodeSecurity,
				Message: fmt.Sprintf("Domain not in allowed list: %s", parsedURL.Host),
				Details: map[string]interface{}{
					"url":    urlStr,
					"domain": parsedURL.Host,
				},
			}
		}
	}

	// Check blocked domains
	if len(sv.config.BlockedDomains) > 0 {
		if sv.isDomainBlocked(parsedURL.Host) {
			sv.logSecurityViolation("domain_blocked", urlStr)
			return &BrowserError{
				Code:    ErrCodeSecurity,
				Message: fmt.Sprintf("Domain is blocked: %s", parsedURL.Host),
				Details: map[string]interface{}{
					"url":    urlStr,
					"domain": parsedURL.Host,
				},
			}
		}
	}

	return nil
}

// isLocalhostURL checks if a URL points to localhost
func (sv *SecurityValidator) isLocalhostURL(parsedURL *url.URL) bool {
	host := strings.ToLower(parsedURL.Host)

	// Remove port if present
	if idx := strings.Index(host, ":"); idx != -1 {
		host = host[:idx]
	}

	return host == "localhost" ||
		host == "127.0.0.1" ||
		host == "::1" ||
		host == "0.0.0.0" ||
		strings.HasPrefix(host, "127.") ||
		strings.HasPrefix(host, "localhost.")
}

// isDomainAllowed checks if a domain is in the allowed list
func (sv *SecurityValidator) isDomainAllowed(host string) bool {
	// Remove port if present
	if idx := strings.Index(host, ":"); idx != -1 {
		host = host[:idx]
	}

	for _, allowed := range sv.config.AllowedDomains {
		if sv.matchDomain(host, allowed) {
			return true
		}
	}

	return false
}

// isDomainBlocked checks if a domain is in the blocked list
func (sv *SecurityValidator) isDomainBlocked(host string) bool {
	// Remove port if present
	if idx := strings.Index(host, ":"); idx != -1 {
		host = host[:idx]
	}

	for _, blocked := range sv.config.BlockedDomains {
		if sv.matchDomain(host, blocked) {
			return true
		}
	}

	return false
}

// matchDomain checks if a host matches a domain pattern
func (sv *SecurityValidator) matchDomain(host, pattern string) bool {
	// Exact match
	if host == pattern {
		return true
	}

	// Wildcard match (*.example.com)
	if strings.HasPrefix(pattern, "*.") {
		suffix := pattern[2:]
		return strings.HasSuffix(host, "."+suffix) || host == suffix
	}

	// Subdomain match (.example.com matches any subdomain)
	if strings.HasPrefix(pattern, ".") {
		return strings.HasSuffix(host, pattern) || host == pattern[1:]
	}

	return false
}

// logSecurityViolation logs a security violation
func (sv *SecurityValidator) logSecurityViolation(violationType, details string) {
	// In a real implementation, this would use a proper logger
	fmt.Printf("[SECURITY] Violation: %s - %s\n", violationType, details)
}

// ValidateNavigationTimeout validates navigation timeout bounds
func ValidateNavigationTimeout(timeout int) error {
	if timeout < 5 || timeout > 120 {
		return &BrowserError{
			Code:    ErrCodeValidation,
			Message: fmt.Sprintf("Navigation timeout must be between 5 and 120 seconds, got %d", timeout),
		}
	}
	return nil
}

// ValidateNavigationParams validates navigation parameters
func ValidateNavigationParams(params NavigateParams) error {
	// Validate wait condition
	if params.WaitUntil != "" {
		validWaitConditions := map[string]bool{
			"load":             true,
			"domcontentloaded": true,
			"networkidle":      true,
		}
		if !validWaitConditions[params.WaitUntil] {
			return &BrowserError{
				Code:    ErrCodeValidation,
				Message: fmt.Sprintf("Invalid wait condition: %s (must be 'load', 'domcontentloaded', or 'networkidle')", params.WaitUntil),
			}
		}
	}

	// Validate timeout
	if params.Timeout > 0 {
		if err := ValidateNavigationTimeout(params.Timeout); err != nil {
			return err
		}
	}

	return nil
}

// ValidateScreenshotParams validates screenshot parameters
func ValidateScreenshotParams(params ScreenshotParams) error {
	// Validate format
	if params.Type != "" && params.Type != "png" && params.Type != "jpeg" {
		return &BrowserError{
			Code:    ErrCodeValidation,
			Message: fmt.Sprintf("Invalid screenshot format: %s (must be 'png' or 'jpeg')", params.Type),
		}
	}

	// Validate quality (for JPEG)
	if params.Type == "jpeg" && params.Quality > 0 {
		if params.Quality < 0 || params.Quality > 100 {
			return &BrowserError{
				Code:    ErrCodeValidation,
				Message: fmt.Sprintf("JPEG quality must be between 0 and 100, got %d", params.Quality),
			}
		}
	}

	return nil
}

// ValidateExtractParams validates content extraction parameters
func ValidateExtractParams(params ExtractParams) error {
	validTypes := map[string]bool{
		"html":     true,
		"text":     true,
		"metadata": true,
		"links":    true,
		"selector": true,
	}

	if !validTypes[params.Type] {
		return &BrowserError{
			Code:    ErrCodeValidation,
			Message: fmt.Sprintf("Invalid extract type: %s", params.Type),
		}
	}

	// Selector is required for "selector" type
	if params.Type == "selector" && params.Selector == "" {
		return &BrowserError{
			Code:    ErrCodeValidation,
			Message: "Selector is required for extract type 'selector'",
		}
	}

	return nil
}

// ValidateExecuteParams validates JavaScript execution parameters
func ValidateExecuteParams(params ExecuteParams) error {
	if params.Script == "" {
		return &BrowserError{
			Code:    ErrCodeValidation,
			Message: "Script is required",
		}
	}

	// Validate timeout
	if params.Timeout > 0 && params.Timeout > 120 {
		return &BrowserError{
			Code:    ErrCodeValidation,
			Message: fmt.Sprintf("Script timeout must be <= 120 seconds, got %d", params.Timeout),
		}
	}

	// Security validation for script content
	if err := ValidateScriptSecurity(params.Script); err != nil {
		return err
	}

	return nil
}

// ValidateScriptSecurity validates JavaScript code for security concerns
func ValidateScriptSecurity(script string) error {
	lowerScript := strings.ToLower(script)

	// Check for dangerous patterns that could be used for malicious purposes
	dangerousPatterns := []struct {
		pattern string
		reason  string
	}{
		{"eval(", "eval() can execute arbitrary code"},
		{"function(", "Function constructor can execute arbitrary code"},
		{"settimeout", "setTimeout can be used for timing attacks"},
		{"setinterval", "setInterval can be used for timing attacks"},
		{"import(", "Dynamic imports can load external code"},
		{"<script", "Script tags are not allowed"},
		{"</script", "Script tags are not allowed"},
		{"javascript:", "javascript: protocol is not allowed"},
		{"data:", "data: URLs can contain executable code"},
		{"vbscript:", "vbscript: protocol is not allowed"},
		{"file:", "file: protocol is not allowed"},
	}

	for _, dp := range dangerousPatterns {
		if strings.Contains(lowerScript, dp.pattern) {
			return &BrowserError{
				Code:    ErrCodeSecurity,
				Message: fmt.Sprintf("Script contains potentially dangerous pattern: %s", dp.reason),
				Details: map[string]interface{}{
					"pattern": dp.pattern,
				},
			}
		}
	}

	// Check script length (prevent extremely large scripts)
	if len(script) > 100000 { // 100KB limit
		return &BrowserError{
			Code:    ErrCodeValidation,
			Message: fmt.Sprintf("Script is too large: %d bytes (max 100000)", len(script)),
		}
	}

	return nil
}

// ValidateInteractParams validates element interaction parameters
func ValidateInteractParams(params InteractParams) error {
	validActions := map[string]bool{
		"click":   true,
		"type":    true,
		"select":  true,
		"wait":    true,
		"visible": true,
	}

	if !validActions[params.Action] {
		return &BrowserError{
			Code:    ErrCodeValidation,
			Message: fmt.Sprintf("Invalid interaction action: %s", params.Action),
		}
	}

	if params.Selector == "" {
		return &BrowserError{
			Code:    ErrCodeValidation,
			Message: "Selector is required",
		}
	}

	// Value is required for "type" and "select" actions
	if (params.Action == "type" || params.Action == "select") && params.Value == "" {
		return &BrowserError{
			Code:    ErrCodeValidation,
			Message: fmt.Sprintf("Value is required for action '%s'", params.Action),
		}
	}

	return nil
}

// ValidateProfileConfig validates a profile configuration
func ValidateProfileConfig(profile *BrowserProfile) error {
	if profile.Name == "" {
		return &BrowserError{
			Code:    ErrCodeConfiguration,
			Message: "Profile name is required",
		}
	}

	if profile.CDPPort != 0 {
		if err := ValidateCDPPort(profile.CDPPort); err != nil {
			return err
		}
	}

	return nil
}

// SanitizeScript sanitizes a JavaScript script (basic sanitization)
func SanitizeScript(script string) string {
	// Remove potentially dangerous patterns
	// This is a basic implementation - in production, use a proper sanitizer
	script = strings.ReplaceAll(script, "<script>", "")
	script = strings.ReplaceAll(script, "</script>", "")
	return script
}

// IsValidSelector checks if a selector is valid
func IsValidSelector(selector string) bool {
	// Basic validation - check if selector is not empty and doesn't contain dangerous patterns
	if selector == "" {
		return false
	}

	// Check for script injection attempts
	dangerous := []string{"<script", "javascript:", "onerror=", "onload="}
	lowerSelector := strings.ToLower(selector)
	for _, pattern := range dangerous {
		if strings.Contains(lowerSelector, pattern) {
			return false
		}
	}

	return true
}

// ValidateCookieParams validates cookie parameters
func ValidateCookieParams(cookie Cookie) error {
	if cookie.Name == "" {
		return &BrowserError{
			Code:    ErrCodeValidation,
			Message: "Cookie name is required",
		}
	}

	if cookie.Domain == "" {
		return &BrowserError{
			Code:    ErrCodeValidation,
			Message: "Cookie domain is required",
		}
	}

	// Validate SameSite
	if cookie.SameSite != "" {
		validSameSite := map[string]bool{
			"Strict": true,
			"Lax":    true,
			"None":   true,
		}
		if !validSameSite[cookie.SameSite] {
			return &BrowserError{
				Code:    ErrCodeValidation,
				Message: fmt.Sprintf("Invalid SameSite value: %s", cookie.SameSite),
			}
		}
	}

	return nil
}
