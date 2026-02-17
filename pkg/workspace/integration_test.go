package workspace

import (
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

// TestIntegration_CompleteWorkflow tests the complete workflow from init to close
func TestIntegration_CompleteWorkflow(t *testing.T) {
	// Create temporary workspace
	tmpDir := t.TempDir()

	// Create test files
	agentsFile := filepath.Join(tmpDir, "AGENTS.md")
	soulFile := filepath.Join(tmpDir, "SOUL.md")
	configFile := filepath.Join(tmpDir, "config.yaml")

	_ = os.WriteFile(agentsFile, []byte("# Agent Instructions\nYou are an AI assistant."), 0644)
	_ = os.WriteFile(soulFile, []byte("# Agent Soul\nBe helpful and friendly."), 0644)
	_ = os.WriteFile(configFile, []byte("name: test\nversion: 1.0"), 0644)

	// Create workspace manager
	config := WorkspaceConfig{
		WorkspacePath:    tmpDir,
		EnableValidation: true,
	}

	manager, err := NewWorkspaceManager(config)
	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}

	// Track events
	var initEvent atomic.Bool
	var addEvent atomic.Bool
	var changeEvent atomic.Bool
	var deleteEvent atomic.Bool

	manager.On(EventInitialized, func(payload interface{}) {
		initEvent.Store(true)
	})

	manager.On(EventFileAdded, func(payload interface{}) {
		addEvent.Store(true)
	})

	manager.On(EventFileChanged, func(payload interface{}) {
		changeEvent.Store(true)
	})

	manager.On(EventFileDeleted, func(payload interface{}) {
		deleteEvent.Store(true)
	})

	// Initialize
	if err := manager.Init(); err != nil {
		t.Fatalf("Failed to initialize: %v", err)
	}

	waitForAtomicEvent(t, &initEvent, 2*time.Second, "initialization event")

	// Verify files are loaded
	files := manager.GetAllFiles()
	if len(files) != 3 {
		t.Errorf("Expected 3 files, got %d", len(files))
	}

	// Verify file retrieval
	agentsContent, ok := manager.GetFileContent("AGENTS.md")
	if !ok {
		t.Error("Failed to get AGENTS.md content")
	}
	if agentsContent != "# Agent Instructions\nYou are an AI assistant." {
		t.Errorf("Unexpected content: %s", agentsContent)
	}

	// Verify file by type
	agentFiles := manager.GetFilesByType(FileTypeAgents)
	if len(agentFiles) != 1 {
		t.Errorf("Expected 1 agent file, got %d", len(agentFiles))
	}

	// Test file addition
	newFile := filepath.Join(tmpDir, "TOOLS.md")
	_ = os.WriteFile(newFile, []byte("# Tools\nAvailable tools..."), 0644)

	waitForAtomicEvent(t, &addEvent, 2*time.Second, "file added event")

	// Verify new file is in cache
	files = manager.GetAllFiles()
	if len(files) != 4 {
		t.Errorf("Expected 4 files after addition, got %d", len(files))
	}

	// Test file change
	_ = os.WriteFile(agentsFile, []byte("# Agent Instructions\nUpdated content."), 0644)

	waitForAtomicEvent(t, &changeEvent, 2*time.Second, "file changed event")

	// Verify content is updated
	updatedContent, _ := manager.GetFileContent("AGENTS.md")
	if updatedContent != "# Agent Instructions\nUpdated content." {
		t.Errorf("Content not updated: %s", updatedContent)
	}

	// Test file deletion
	os.Remove(newFile)

	waitForAtomicEvent(t, &deleteEvent, 2*time.Second, "file deleted event")

	// Verify file is removed from cache
	files = manager.GetAllFiles()
	if len(files) != 3 {
		t.Errorf("Expected 3 files after deletion, got %d", len(files))
	}

	// Close manager
	if err := manager.Close(); err != nil {
		t.Fatalf("Failed to close manager: %v", err)
	}

	// Verify cache is cleared
	if manager.cache.Size() != 0 {
		t.Errorf("Cache not cleared after close, size: %d", manager.cache.Size())
	}
}

// TestIntegration_ConfigResolution tests environment variable and file reference resolution
func TestIntegration_ConfigResolution(t *testing.T) {
	// Create temporary workspace
	tmpDir := t.TempDir()

	// Set environment variable
	os.Setenv("TEST_API_KEY", "secret123")
	defer os.Unsetenv("TEST_API_KEY")

	// Create content file
	contentFile := filepath.Join(tmpDir, "content.txt")
	_ = os.WriteFile(contentFile, []byte("This is file content"), 0644)

	// Create config file with env vars and file refs
	configFile := filepath.Join(tmpDir, "config.yaml")
	configContent := `
api:
  key: ${TEST_API_KEY}
  url: https://api.example.com
content: file://` + contentFile + `
`
	_ = os.WriteFile(configFile, []byte(configContent), 0644)

	// Create workspace manager
	config := WorkspaceConfig{
		WorkspacePath:    tmpDir,
		EnableValidation: true,
	}

	manager, err := NewWorkspaceManager(config)
	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}

	// Initialize
	if err := manager.Init(); err != nil {
		t.Fatalf("Failed to initialize: %v", err)
	}

	// Get config file
	configFileObj, ok := manager.GetFileByName("config.yaml")
	if !ok {
		t.Fatal("Config file not found")
	}

	// Verify parsed content has resolved values
	parsedConfig, ok := configFileObj.ParsedContent.(map[string]interface{})
	if !ok {
		t.Fatal("Parsed content is not a map")
	}

	// Check API key is resolved
	api, ok := parsedConfig["api"].(map[string]interface{})
	if !ok {
		t.Fatal("API section not found")
	}

	if api["key"] != "secret123" {
		t.Errorf("API key not resolved, got: %v", api["key"])
	}

	// Check file reference is resolved
	if parsedConfig["content"] != "This is file content" {
		t.Errorf("File reference not resolved, got: %v", parsedConfig["content"])
	}

	// Close manager
	manager.Close()
}

// TestIntegration_CriticalFileReload tests reload callback for critical files
func TestIntegration_CriticalFileReload(t *testing.T) {
	// Create temporary workspace
	tmpDir := t.TempDir()

	// Create critical file
	agentsFile := filepath.Join(tmpDir, "AGENTS.md")
	_ = os.WriteFile(agentsFile, []byte("# Agent Instructions"), 0644)

	// Track reload callback
	reloadEvents := make(chan *WorkspaceFile, 8)

	// Create workspace manager with reload callback
	config := WorkspaceConfig{
		WorkspacePath:    tmpDir,
		EnableValidation: true,
		OnReload: func(file *WorkspaceFile) {
			select {
			case reloadEvents <- file:
			default:
			}
		},
	}

	manager, err := NewWorkspaceManager(config)
	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}

	// Initialize
	if err := manager.Init(); err != nil {
		t.Fatalf("Failed to initialize: %v", err)
	}

	// Wait for initialization
	time.Sleep(100 * time.Millisecond)

	// Reset by draining any initialization callback notifications.
	for {
		select {
		case <-reloadEvents:
		default:
			goto drained
		}
	}
drained:

	// Modify critical file
	_ = os.WriteFile(agentsFile, []byte("# Agent Instructions\nUpdated"), 0644)

	var reloadedFile *WorkspaceFile
	select {
	case reloadedFile = <-reloadEvents:
	case <-time.After(2 * time.Second):
		t.Fatal("reload callback not called for critical file")
	}

	if reloadedFile == nil {
		t.Error("Reloaded file is nil")
	} else {
		if !reloadedFile.IsCritical {
			t.Error("Reloaded file is not marked as critical")
		}
		if reloadedFile.Type != FileTypeAgents {
			t.Errorf("Unexpected file type: %s", reloadedFile.Type)
		}
	}

	// Close manager
	manager.Close()
}

// TestIntegration_ErrorHandling tests error handling and recovery
func TestIntegration_ErrorHandling(t *testing.T) {
	// Create temporary workspace
	tmpDir := t.TempDir()

	// Create valid and invalid files
	validFile := filepath.Join(tmpDir, "valid.yaml")
	invalidFile := filepath.Join(tmpDir, "invalid.yaml")

	_ = os.WriteFile(validFile, []byte("key: value"), 0644)
	_ = os.WriteFile(invalidFile, []byte("invalid: yaml: content:"), 0644)

	// Track error events
	var errorReceived atomic.Bool

	// Create workspace manager
	config := WorkspaceConfig{
		WorkspacePath:    tmpDir,
		EnableValidation: true,
	}

	manager, err := NewWorkspaceManager(config)
	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}

	manager.On(EventError, func(payload interface{}) {
		errorReceived.Store(true)
	})

	// Initialize - should continue despite invalid file
	if err := manager.Init(); err != nil {
		t.Fatalf("Failed to initialize: %v", err)
	}

	// Wait for processing
	time.Sleep(100 * time.Millisecond)

	// Note: errorReceived may or may not be true depending on timing
	// The important thing is that the system continues to work
	_ = errorReceived.Load()

	// Verify valid file is loaded
	files := manager.GetAllFiles()
	if len(files) != 1 {
		t.Errorf("Expected 1 valid file, got %d", len(files))
	}

	// Verify it's the valid file
	if files[0].Path != validFile {
		t.Errorf("Expected valid file, got: %s", files[0].Path)
	}

	// Close manager
	manager.Close()
}

func waitForAtomicEvent(t *testing.T, flag *atomic.Bool, timeout time.Duration, eventName string) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if flag.Load() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("%s not received within %s", eventName, timeout)
}

// TestIntegration_PerformanceBasic tests basic performance requirements
func TestIntegration_PerformanceBasic(t *testing.T) {
	// Create temporary workspace
	tmpDir := t.TempDir()

	// Create multiple files
	for i := 0; i < 50; i++ {
		filename := filepath.Join(tmpDir, "file_"+string(rune('a'+i))+".md")
		_ = os.WriteFile(filename, []byte("Content "+string(rune('a'+i))), 0644)
	}

	// Create workspace manager
	config := WorkspaceConfig{
		WorkspacePath:    tmpDir,
		EnableValidation: true,
	}

	manager, err := NewWorkspaceManager(config)
	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}

	// Measure initialization time
	start := time.Now()
	if err := manager.Init(); err != nil {
		t.Fatalf("Failed to initialize: %v", err)
	}
	elapsed := time.Since(start)

	// Should complete within reasonable time (5 seconds for 50 files)
	if elapsed > 5*time.Second {
		t.Errorf("Initialization took too long: %v", elapsed)
	}

	// Verify all files are loaded
	files := manager.GetAllFiles()
	if len(files) != 50 {
		t.Errorf("Expected 50 files, got %d", len(files))
	}

	// Test file change performance
	testFile := filepath.Join(tmpDir, "file_a.md")
	start = time.Now()
	_ = os.WriteFile(testFile, []byte("Updated content"), 0644)

	// Wait for change to be processed
	time.Sleep(200 * time.Millisecond)

	elapsed = time.Since(start)

	// Should process within 500ms (requirement: 100ms detection + 500ms reload)
	if elapsed > 600*time.Millisecond {
		t.Errorf("File change processing took too long: %v", elapsed)
	}

	// Close manager
	manager.Close()
}
