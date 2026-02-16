package browser

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
)

// ProcessManager manages Chrome browser processes
type ProcessManager struct {
	profile   *ResolvedBrowserProfile
	cmd       *exec.Cmd
	launcher  *launcher.Launcher
	mu        sync.RWMutex
	isRunning bool
}

// NewProcessManager creates a new process manager for a profile
func NewProcessManager(profile *ResolvedBrowserProfile) *ProcessManager {
	return &ProcessManager{
		profile: profile,
	}
}

// SpawnChrome spawns a Chrome process with the profile's configuration
func (pm *ProcessManager) SpawnChrome(ctx context.Context) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	if pm.isRunning {
		return nil
	}

	// Ensure user data directory exists
	if err := pm.ensureUserDataDir(); err != nil {
		return &BrowserError{
			Code:    ErrCodeConfiguration,
			Message: fmt.Sprintf("Failed to create user data directory: %v", err),
		}
	}

	// Build Chrome launcher
	l := launcher.New().
		Headless(pm.profile.Headless).
		UserDataDir(pm.profile.UserDataDir).
		RemoteDebuggingPort(pm.profile.CDPPort)

	// Add no-sandbox flag if configured
	if pm.profile.NoSandbox {
		l = l.NoSandbox(true)
	}

	// Set custom Chrome path if specified
	if pm.profile.ChromePath != "" {
		l = l.Bin(pm.profile.ChromePath)
	}

	// Custom arguments are handled by Rod's launcher automatically

	// Launch Chrome
	url, err := l.Launch()
	if err != nil {
		return &BrowserError{
			Code:    ErrCodeBrowserCrash,
			Message: fmt.Sprintf("Failed to launch Chrome: %v", err),
		}
	}

	pm.launcher = l
	pm.isRunning = true

	// Update CDP URL
	pm.profile.CDPUrl = url

	return nil
}

// ConnectCDP connects to the Chrome DevTools Protocol endpoint
func (pm *ProcessManager) ConnectCDP(ctx context.Context) (*rod.Browser, error) {
	pm.mu.RLock()
	cdpURL := pm.profile.CDPUrl
	pm.mu.RUnlock()

	if cdpURL == "" {
		return nil, &BrowserError{
			Code:    ErrCodeConfiguration,
			Message: "CDP URL not set",
		}
	}

	// Wait for CDP to be available
	if err := pm.waitForCDP(ctx, cdpURL); err != nil {
		return nil, err
	}

	// Connect to browser
	browser := rod.New().ControlURL(cdpURL)
	if err := browser.Connect(); err != nil {
		return nil, &BrowserError{
			Code:    ErrCodeBrowserCrash,
			Message: fmt.Sprintf("Failed to connect to CDP: %v", err),
		}
	}

	return browser, nil
}

// waitForCDP waits for the CDP endpoint to become available
func (pm *ProcessManager) waitForCDP(ctx context.Context, cdpURL string) error {
	// Extract port from CDP URL
	port := pm.profile.CDPPort

	timeout := 10 * time.Second
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// Try to connect to the port
		conn, err := net.DialTimeout("tcp", fmt.Sprintf("localhost:%d", port), time.Second)
		if err == nil {
			conn.Close()
			return nil
		}

		time.Sleep(100 * time.Millisecond)
	}

	return &BrowserError{
		Code:    ErrCodeTimeout,
		Message: fmt.Sprintf("CDP endpoint not available after %v", timeout),
	}
}

// KillChrome terminates the Chrome process
func (pm *ProcessManager) KillChrome() error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	if !pm.isRunning {
		return nil
	}

	if pm.launcher != nil {
		pm.launcher.Kill()
		pm.launcher = nil
	}

	pm.isRunning = false
	return nil
}

// IsRunning checks if the Chrome process is running
func (pm *ProcessManager) IsRunning() bool {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	return pm.isRunning
}

// CheckHealth verifies the CDP connection is healthy
func (pm *ProcessManager) CheckHealth(ctx context.Context) error {
	if !pm.IsRunning() {
		return &BrowserError{
			Code:    ErrCodeBrowserCrash,
			Message: "Chrome process not running",
		}
	}

	// Try to connect to CDP
	port := pm.profile.CDPPort
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("localhost:%d", port), 2*time.Second)
	if err != nil {
		pm.mu.Lock()
		pm.isRunning = false
		pm.mu.Unlock()
		return &BrowserError{
			Code:    ErrCodeBrowserCrash,
			Message: "CDP endpoint not responding",
		}
	}
	conn.Close()

	return nil
}

// ensureUserDataDir creates the user data directory if it doesn't exist
func (pm *ProcessManager) ensureUserDataDir() error {
	if pm.profile.UserDataDir == "" {
		// Generate default user data directory
		tmpDir := os.TempDir()
		pm.profile.UserDataDir = filepath.Join(tmpDir, "browser-profiles", pm.profile.Name)
	}

	// Create directory
	if err := os.MkdirAll(pm.profile.UserDataDir, 0755); err != nil {
		return err
	}

	return nil
}

// DecorateProfile sets up profile preferences
func (pm *ProcessManager) DecorateProfile() error {
	// Create preferences file if needed
	prefsPath := filepath.Join(pm.profile.UserDataDir, "Default", "Preferences")
	prefsDir := filepath.Dir(prefsPath)

	if err := os.MkdirAll(prefsDir, 0755); err != nil {
		return err
	}

	// Basic preferences (can be extended)
	prefs := `{
		"profile": {
			"name": "` + pm.profile.Name + `"
		}
	}`

	if err := os.WriteFile(prefsPath, []byte(prefs), 0644); err != nil {
		return err
	}

	return nil
}

// AttachToExisting attaches to an existing Chrome instance
func (pm *ProcessManager) AttachToExisting(ctx context.Context) (*rod.Browser, error) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	if pm.profile.CDPUrl == "" {
		// Build CDP URL from port
		pm.profile.CDPUrl = fmt.Sprintf("ws://localhost:%d", pm.profile.CDPPort)
	}

	// Wait for CDP to be available
	if err := pm.waitForCDP(ctx, pm.profile.CDPUrl); err != nil {
		return nil, err
	}

	// Connect to browser
	browser := rod.New().ControlURL(pm.profile.CDPUrl)
	if err := browser.Connect(); err != nil {
		return nil, &BrowserError{
			Code:    ErrCodeBrowserCrash,
			Message: fmt.Sprintf("Failed to attach to existing browser: %v", err),
		}
	}

	pm.isRunning = true
	return browser, nil
}

// GetCDPPort returns the CDP port for this profile
func (pm *ProcessManager) GetCDPPort() int {
	return pm.profile.CDPPort
}

// GetUserDataDir returns the user data directory path
func (pm *ProcessManager) GetUserDataDir() string {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	return pm.profile.UserDataDir
}

// findAvailablePort finds an available port starting from the given port
func findAvailablePort(startPort int) (int, error) {
	for port := startPort; port < startPort+100; port++ {
		ln, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
		if err == nil {
			ln.Close()
			return port, nil
		}
	}
	return 0, fmt.Errorf("no available port found in range %d-%d", startPort, startPort+100)
}

// EnsureUniquePort ensures the profile has a unique CDP port
func EnsureUniquePort(profile *BrowserProfile, usedPorts map[int]bool) error {
	if profile.CDPPort == 0 {
		profile.CDPPort = 9222
	}

	// Check if port is already used
	if usedPorts[profile.CDPPort] {
		// Find next available port
		port, err := findAvailablePort(profile.CDPPort + 1)
		if err != nil {
			return err
		}
		profile.CDPPort = port
	}

	usedPorts[profile.CDPPort] = true
	return nil
}

// ResolveProfile resolves a profile configuration with computed paths
func ResolveProfile(profile *BrowserProfile, baseDir string) (*ResolvedBrowserProfile, error) {
	resolved := &ResolvedBrowserProfile{
		BrowserProfile: *profile,
	}

	// Compute user data directory
	if profile.UserDataDir == "" {
		resolved.UserDataDir = filepath.Join(baseDir, "profiles", profile.Name)
	} else {
		// Make absolute
		if !filepath.IsAbs(profile.UserDataDir) {
			resolved.UserDataDir = filepath.Join(baseDir, profile.UserDataDir)
		} else {
			resolved.UserDataDir = profile.UserDataDir
		}
	}

	// Validate CDP port
	if profile.CDPPort == 0 {
		resolved.CDPPort = 9222
	}

	// Build CDP URL if not provided
	if profile.CDPUrl == "" && !profile.AttachOnly {
		resolved.CDPUrl = fmt.Sprintf("ws://localhost:%d", resolved.CDPPort)
	}

	return resolved, nil
}

// ParseCDPPort extracts the port number from a CDP URL
func ParseCDPPort(cdpURL string) (int, error) {
	// Extract port from ws://localhost:PORT format
	var port int
	_, err := fmt.Sscanf(cdpURL, "ws://localhost:%d", &port)
	if err != nil {
		return 0, fmt.Errorf("invalid CDP URL format: %s", cdpURL)
	}
	return port, nil
}

// ValidateCDPPort checks if a CDP port is valid
func ValidateCDPPort(port int) error {
	if port < 1024 || port > 65535 {
		return fmt.Errorf("CDP port must be between 1024 and 65535, got %d", port)
	}
	return nil
}

// GetChromeVersion returns the Chrome version
func GetChromeVersion(chromePath string) (string, error) {
	if chromePath == "" {
		chromePath = launcher.NewBrowser().MustGet()
	}

	cmd := exec.Command(chromePath, "--version")
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}

	return string(output), nil
}

// IsChromeInstalled checks if Chrome is installed
func IsChromeInstalled() bool {
	_, err := launcher.NewBrowser().Get()
	return err == nil
}

// PortToString converts a port number to string
func PortToString(port int) string {
	return strconv.Itoa(port)
}
