package workspace_test

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/harun/ranya/pkg/workspace"
)

// Example_basic demonstrates basic usage of the workspace manager
func Example_basic() {
	// Create a temporary workspace directory
	tmpDir, _ := os.MkdirTemp("", "workspace-example-")
	defer os.RemoveAll(tmpDir)

	// Create some workspace files
	_ = os.WriteFile(filepath.Join(tmpDir, "AGENTS.md"), []byte("# Agent Instructions\nYou are an AI assistant."), 0644)
	_ = os.WriteFile(filepath.Join(tmpDir, "SOUL.md"), []byte("# Agent Soul\nBe helpful and friendly."), 0644)

	// Create workspace manager
	config := workspace.WorkspaceConfig{
		WorkspacePath:    tmpDir,
		EnableValidation: true,
	}

	manager, err := workspace.NewWorkspaceManager(config)
	if err != nil {
		log.Fatal(err)
	}

	// Initialize and load files
	if err := manager.Init(); err != nil {
		log.Fatal(err)
	}

	// Get file content
	content, ok := manager.GetFileContent("AGENTS.md")
	if ok {
		fmt.Println("Found AGENTS.md")
		fmt.Println(len(content) > 0)
	}

	// Get all files
	files := manager.GetAllFiles()
	fmt.Printf("Loaded %d files\n", len(files))

	// Close manager
	manager.Close()

	// Output:
	// Found AGENTS.md
	// true
	// Loaded 2 files
}

// Example_events demonstrates event handling
func Example_events() {
	// Create a temporary workspace directory
	tmpDir, _ := os.MkdirTemp("", "workspace-example-")
	defer os.RemoveAll(tmpDir)

	// Create workspace manager
	config := workspace.WorkspaceConfig{
		WorkspacePath:    tmpDir,
		EnableValidation: true,
	}

	manager, err := workspace.NewWorkspaceManager(config)
	if err != nil {
		log.Fatal(err)
	}

	// Register event handlers
	manager.On(workspace.EventInitialized, func(payload interface{}) {
		p := payload.(workspace.InitializedPayload)
		fmt.Printf("Workspace initialized with %d files\n", p.FileCount)
	})

	manager.On(workspace.EventFileAdded, func(payload interface{}) {
		p := payload.(workspace.FileEventPayload)
		fmt.Printf("File added: %s\n", p.RelativePath)
	})

	// Initialize
	_ = manager.Init()

	// Wait for events
	time.Sleep(100 * time.Millisecond)

	// Add a file
	_ = os.WriteFile(filepath.Join(tmpDir, "TOOLS.md"), []byte("# Tools"), 0644)

	// Wait for file added event
	time.Sleep(200 * time.Millisecond)

	// Close manager
	manager.Close()

	// Output:
	// Workspace initialized with 0 files
	// File added: TOOLS.md
}

// Example_configResolution demonstrates environment variable and file reference resolution
func Example_configResolution() {
	// Create a temporary workspace directory
	tmpDir, _ := os.MkdirTemp("", "workspace-example-")
	defer os.RemoveAll(tmpDir)

	// Set environment variable
	os.Setenv("API_KEY", "secret123")
	defer os.Unsetenv("API_KEY")

	// Create content file
	contentFile := filepath.Join(tmpDir, "prompt.txt")
	os.WriteFile(contentFile, []byte("You are a helpful assistant"), 0644)

	// Create config file with env vars and file refs
	configContent := `
api:
  key: ${API_KEY}
  url: https://api.example.com
prompt: file://` + contentFile + `
`
	os.WriteFile(filepath.Join(tmpDir, "config.yaml"), []byte(configContent), 0644)

	// Create workspace manager
	config := workspace.WorkspaceConfig{
		WorkspacePath:    tmpDir,
		EnableValidation: true,
	}

	manager, err := workspace.NewWorkspaceManager(config)
	if err != nil {
		log.Fatal(err)
	}

	// Initialize
	if err := manager.Init(); err != nil {
		log.Fatal(err)
	}

	// Get config file
	configFile, ok := manager.GetFileByName("config.yaml")
	if ok {
		parsedConfig := configFile.ParsedContent.(map[string]interface{})
		api := parsedConfig["api"].(map[string]interface{})

		fmt.Println("API key resolved:", api["key"] == "secret123")
		fmt.Println("Prompt resolved:", parsedConfig["prompt"] == "You are a helpful assistant")
	}

	// Close manager
	manager.Close()

	// Output:
	// API key resolved: true
	// Prompt resolved: true
}

// Example_criticalFileReload demonstrates reload callback for critical files
func Example_criticalFileReload() {
	// Create a temporary workspace directory
	tmpDir, _ := os.MkdirTemp("", "workspace-example-")
	defer os.RemoveAll(tmpDir)

	// Create critical file
	agentsFile := filepath.Join(tmpDir, "AGENTS.md")
	os.WriteFile(agentsFile, []byte("# Agent Instructions v1"), 0644)

	// Create workspace manager with reload callback
	config := workspace.WorkspaceConfig{
		WorkspacePath:    tmpDir,
		EnableValidation: true,
		OnReload: func(file *workspace.WorkspaceFile) {
			fmt.Printf("Critical file reloaded: %s\n", file.RelativePath)
		},
	}

	manager, err := workspace.NewWorkspaceManager(config)
	if err != nil {
		log.Fatal(err)
	}

	// Initialize
	_ = manager.Init()

	// Wait for initialization
	time.Sleep(100 * time.Millisecond)

	// Modify critical file
	os.WriteFile(agentsFile, []byte("# Agent Instructions v2"), 0644)

	// Wait for change detection
	time.Sleep(200 * time.Millisecond)

	// Close manager
	manager.Close()

	// Output:
	// Critical file reloaded: AGENTS.md
}
