package browser

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewProcessManager(t *testing.T) {
	profile := &ResolvedBrowserProfile{
		BrowserProfile: BrowserProfile{
			Name:    "test",
			CDPPort: 9222,
		},
	}

	pm := NewProcessManager(profile)
	assert.NotNil(t, pm)
	assert.Equal(t, profile, pm.profile)
	assert.False(t, pm.IsRunning())
}

func TestEnsureUserDataDir(t *testing.T) {
	tmpDir := t.TempDir()
	profile := &ResolvedBrowserProfile{
		BrowserProfile: BrowserProfile{
			Name:        "test",
			CDPPort:     9222,
			UserDataDir: filepath.Join(tmpDir, "test-profile"),
		},
	}

	pm := NewProcessManager(profile)
	err := pm.ensureUserDataDir()
	require.NoError(t, err)

	// Check directory was created
	_, err = os.Stat(profile.UserDataDir)
	assert.NoError(t, err)
}

func TestResolveProfile(t *testing.T) {
	baseDir := "/tmp/browser"

	tests := []struct {
		name     string
		profile  *BrowserProfile
		expected string
	}{
		{
			name: "relative path",
			profile: &BrowserProfile{
				Name:        "test",
				CDPPort:     9222,
				UserDataDir: "profiles/test",
			},
			expected: filepath.Join(baseDir, "profiles/test"),
		},
		{
			name: "absolute path",
			profile: &BrowserProfile{
				Name:        "test",
				CDPPort:     9222,
				UserDataDir: "/absolute/path",
			},
			expected: "/absolute/path",
		},
		{
			name: "empty path",
			profile: &BrowserProfile{
				Name:    "test",
				CDPPort: 9222,
			},
			expected: filepath.Join(baseDir, "profiles", "test"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resolved, err := ResolveProfile(tt.profile, baseDir)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, resolved.UserDataDir)
		})
	}
}

func TestEnsureUniquePort(t *testing.T) {
	usedPorts := make(map[int]bool)

	// First profile
	profile1 := &BrowserProfile{
		Name:    "test1",
		CDPPort: 9222,
	}
	err := EnsureUniquePort(profile1, usedPorts)
	require.NoError(t, err)
	assert.Equal(t, 9222, profile1.CDPPort)
	assert.True(t, usedPorts[9222])

	// Second profile with same port
	profile2 := &BrowserProfile{
		Name:    "test2",
		CDPPort: 9222,
	}
	err = EnsureUniquePort(profile2, usedPorts)
	require.NoError(t, err)
	assert.NotEqual(t, 9222, profile2.CDPPort)
	assert.True(t, usedPorts[profile2.CDPPort])
}

func TestValidateCDPPort(t *testing.T) {
	tests := []struct {
		name    string
		port    int
		wantErr bool
	}{
		{"valid port", 9222, false},
		{"valid high port", 50000, false},
		{"too low", 1023, true},
		{"too high", 65536, true},
		{"negative", -1, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateCDPPort(tt.port)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestParseCDPPort(t *testing.T) {
	tests := []struct {
		name     string
		cdpURL   string
		expected int
		wantErr  bool
	}{
		{"valid URL", "ws://localhost:9222", 9222, false},
		{"different port", "ws://localhost:9333", 9333, false},
		{"invalid format", "http://localhost:9222", 0, true},
		{"no port", "ws://localhost", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			port, err := ParseCDPPort(tt.cdpURL)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, port)
			}
		})
	}
}

func TestProcessManagerLifecycle(t *testing.T) {
	if !IsChromeInstalled() {
		t.Skip("Chrome not installed")
	}

	tmpDir := t.TempDir()
	profile := &ResolvedBrowserProfile{
		BrowserProfile: BrowserProfile{
			Name:        "test",
			CDPPort:     9333,
			Headless:    true,
			NoSandbox:   true,
			UserDataDir: filepath.Join(tmpDir, "test-profile"),
		},
	}

	pm := NewProcessManager(profile)
	ctx := context.Background()

	// Spawn Chrome
	err := pm.SpawnChrome(ctx)
	require.NoError(t, err)
	assert.True(t, pm.IsRunning())

	// Wait a bit for Chrome to start
	time.Sleep(2 * time.Second)

	// Check health
	err = pm.CheckHealth(ctx)
	assert.NoError(t, err)

	// Connect to CDP
	browser, err := pm.ConnectCDP(ctx)
	require.NoError(t, err)
	assert.NotNil(t, browser)

	// Close browser
	browser.Close()

	// Kill Chrome
	err = pm.KillChrome()
	assert.NoError(t, err)
	assert.False(t, pm.IsRunning())
}

func TestValidateProfileConfig(t *testing.T) {
	tests := []struct {
		name    string
		profile *BrowserProfile
		wantErr bool
	}{
		{
			name: "valid profile",
			profile: &BrowserProfile{
				Name:    "test",
				CDPPort: 9222,
			},
			wantErr: false,
		},
		{
			name: "missing name",
			profile: &BrowserProfile{
				CDPPort: 9222,
			},
			wantErr: true,
		},
		{
			name: "invalid port",
			profile: &BrowserProfile{
				Name:    "test",
				CDPPort: 100,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateProfileConfig(tt.profile)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
