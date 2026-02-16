package browser

import (
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValidateURL(t *testing.T) {
	tests := []struct {
		name    string
		config  SecurityConfig
		url     string
		wantErr bool
	}{
		{
			name:    "valid http URL",
			config:  SecurityConfig{},
			url:     "https://example.com",
			wantErr: false,
		},
		{
			name: "file URL blocked",
			config: SecurityConfig{
				AllowFileUrls: false,
			},
			url:     "file:///etc/passwd",
			wantErr: true,
		},
		{
			name: "file URL allowed",
			config: SecurityConfig{
				AllowFileUrls: true,
			},
			url:     "file:///tmp/test.html",
			wantErr: false,
		},
		{
			name: "localhost blocked",
			config: SecurityConfig{
				AllowLocalhostUrls: false,
			},
			url:     "http://localhost:8080",
			wantErr: true,
		},
		{
			name: "localhost allowed",
			config: SecurityConfig{
				AllowLocalhostUrls: true,
			},
			url:     "http://localhost:8080",
			wantErr: false,
		},
		{
			name: "127.0.0.1 blocked",
			config: SecurityConfig{
				AllowLocalhostUrls: false,
			},
			url:     "http://127.0.0.1:8080",
			wantErr: true,
		},
		{
			name: "domain in allowed list",
			config: SecurityConfig{
				AllowedDomains: []string{"example.com"},
			},
			url:     "https://example.com/page",
			wantErr: false,
		},
		{
			name: "domain not in allowed list",
			config: SecurityConfig{
				AllowedDomains: []string{"example.com"},
			},
			url:     "https://other.com/page",
			wantErr: true,
		},
		{
			name: "domain in blocked list",
			config: SecurityConfig{
				BlockedDomains: []string{"blocked.com"},
			},
			url:     "https://blocked.com/page",
			wantErr: true,
		},
		{
			name: "wildcard allowed domain",
			config: SecurityConfig{
				AllowedDomains: []string{"*.example.com"},
			},
			url:     "https://sub.example.com/page",
			wantErr: false,
		},
		{
			name:    "invalid URL format",
			config:  SecurityConfig{},
			url:     "://invalid",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sv := NewSecurityValidator(tt.config)
			err := sv.ValidateURL(tt.url)
			if tt.wantErr {
				assert.Error(t, err)
				if err != nil {
					browserErr, ok := err.(*BrowserError)
					assert.True(t, ok)
					// Can be either SECURITY_ERROR or VALIDATION_ERROR depending on the issue
					assert.Contains(t, []string{ErrCodeSecurity, ErrCodeValidation}, browserErr.Code)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestIsLocalhostURL(t *testing.T) {
	sv := NewSecurityValidator(SecurityConfig{})

	tests := []struct {
		url      string
		expected bool
	}{
		{"http://localhost", true},
		{"http://localhost:8080", true},
		{"http://127.0.0.1", true},
		{"http://127.0.0.1:8080", true},
		{"http://127.1.2.3", true},
		{"http://0.0.0.0", true},
		{"http://example.com", false},
		{"http://192.168.1.1", false},
	}

	for _, tt := range tests {
		t.Run(tt.url, func(t *testing.T) {
			parsedURL, _ := parseURL(tt.url)
			result := sv.isLocalhostURL(parsedURL)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestMatchDomain(t *testing.T) {
	sv := NewSecurityValidator(SecurityConfig{})

	tests := []struct {
		host     string
		pattern  string
		expected bool
	}{
		{"example.com", "example.com", true},
		{"sub.example.com", "*.example.com", true},
		{"example.com", "*.example.com", true},
		{"sub.example.com", ".example.com", true},
		{"example.com", ".example.com", true},
		{"other.com", "example.com", false},
		{"other.com", "*.example.com", false},
	}

	for _, tt := range tests {
		t.Run(tt.host+"_"+tt.pattern, func(t *testing.T) {
			result := sv.matchDomain(tt.host, tt.pattern)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestValidateNavigationTimeout(t *testing.T) {
	tests := []struct {
		timeout int
		wantErr bool
	}{
		{5, false},
		{30, false},
		{120, false},
		{4, true},
		{121, true},
		{0, true},
		{-1, true},
	}

	for _, tt := range tests {
		t.Run(string(rune(tt.timeout)), func(t *testing.T) {
			err := ValidateNavigationTimeout(tt.timeout)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateScreenshotParams(t *testing.T) {
	tests := []struct {
		name    string
		params  ScreenshotParams
		wantErr bool
	}{
		{
			name:    "valid png",
			params:  ScreenshotParams{Type: "png"},
			wantErr: false,
		},
		{
			name:    "valid jpeg",
			params:  ScreenshotParams{Type: "jpeg", Quality: 80},
			wantErr: false,
		},
		{
			name:    "invalid format",
			params:  ScreenshotParams{Type: "gif"},
			wantErr: true,
		},
		{
			name:    "invalid quality",
			params:  ScreenshotParams{Type: "jpeg", Quality: 150},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateScreenshotParams(tt.params)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateExtractParams(t *testing.T) {
	tests := []struct {
		name    string
		params  ExtractParams
		wantErr bool
	}{
		{
			name:    "valid html",
			params:  ExtractParams{Type: "html"},
			wantErr: false,
		},
		{
			name:    "valid selector with selector",
			params:  ExtractParams{Type: "selector", Selector: "div.content"},
			wantErr: false,
		},
		{
			name:    "selector without selector",
			params:  ExtractParams{Type: "selector"},
			wantErr: true,
		},
		{
			name:    "invalid type",
			params:  ExtractParams{Type: "invalid"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateExtractParams(tt.params)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateExecuteParams(t *testing.T) {
	tests := []struct {
		name    string
		params  ExecuteParams
		wantErr bool
	}{
		{
			name:    "valid script",
			params:  ExecuteParams{Script: "return 1 + 1"},
			wantErr: false,
		},
		{
			name:    "empty script",
			params:  ExecuteParams{Script: ""},
			wantErr: true,
		},
		{
			name:    "timeout too high",
			params:  ExecuteParams{Script: "return 1", Timeout: 200},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateExecuteParams(tt.params)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateInteractParams(t *testing.T) {
	tests := []struct {
		name    string
		params  InteractParams
		wantErr bool
	}{
		{
			name:    "valid click",
			params:  InteractParams{Action: "click", Selector: "button"},
			wantErr: false,
		},
		{
			name:    "valid type with value",
			params:  InteractParams{Action: "type", Selector: "input", Value: "text"},
			wantErr: false,
		},
		{
			name:    "type without value",
			params:  InteractParams{Action: "type", Selector: "input"},
			wantErr: true,
		},
		{
			name:    "invalid action",
			params:  InteractParams{Action: "invalid", Selector: "div"},
			wantErr: true,
		},
		{
			name:    "missing selector",
			params:  InteractParams{Action: "click"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateInteractParams(tt.params)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestIsValidSelector(t *testing.T) {
	tests := []struct {
		selector string
		expected bool
	}{
		{"div.content", true},
		{"#main", true},
		{"button[type='submit']", true},
		{"", false},
		{"<script>alert('xss')</script>", false},
		{"javascript:alert(1)", false},
		{"onerror=alert(1)", false},
	}

	for _, tt := range tests {
		t.Run(tt.selector, func(t *testing.T) {
			result := IsValidSelector(tt.selector)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestValidateCookieParams(t *testing.T) {
	tests := []struct {
		name    string
		cookie  Cookie
		wantErr bool
	}{
		{
			name: "valid cookie",
			cookie: Cookie{
				Name:   "session",
				Value:  "abc123",
				Domain: "example.com",
			},
			wantErr: false,
		},
		{
			name: "missing name",
			cookie: Cookie{
				Value:  "abc123",
				Domain: "example.com",
			},
			wantErr: true,
		},
		{
			name: "missing domain",
			cookie: Cookie{
				Name:  "session",
				Value: "abc123",
			},
			wantErr: true,
		},
		{
			name: "invalid SameSite",
			cookie: Cookie{
				Name:     "session",
				Value:    "abc123",
				Domain:   "example.com",
				SameSite: "Invalid",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateCookieParams(tt.cookie)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// Helper function to parse URL for testing
func parseURL(urlStr string) (*url.URL, error) {
	return url.Parse(urlStr)
}
