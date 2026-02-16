package workspace

import "time"

// WorkspaceFileType represents the category of a workspace file
type WorkspaceFileType string

const (
	FileTypeAgents         WorkspaceFileType = "agents"          // AGENTS.md
	FileTypeSoul           WorkspaceFileType = "soul"            // SOUL.md
	FileTypeTools          WorkspaceFileType = "tools"           // TOOLS.md
	FileTypeMemory         WorkspaceFileType = "memory"          // MEMORY.md
	FileTypeWebhooks       WorkspaceFileType = "webhooks"        // WEBHOOKS.md
	FileTypeCron           WorkspaceFileType = "cron"            // CRON.md
	FileTypePlugins        WorkspaceFileType = "plugins"         // PLUGINS.md
	FileTypeSkill          WorkspaceFileType = "skill"           // SKILL.md
	FileTypeConfig         WorkspaceFileType = "config"          // YAML/JSON config files
	FileTypePluginManifest WorkspaceFileType = "plugin-manifest" // plugin.json
	FileTypePluginCode     WorkspaceFileType = "plugin-code"     // .go in plugins/
	FileTypeOther          WorkspaceFileType = "other"           // Unknown files
)

// WorkspaceFile represents a loaded workspace file with metadata
type WorkspaceFile struct {
	Path          string            // Absolute path
	RelativePath  string            // Path relative to workspace root
	Content       string            // Raw file content
	ParsedContent interface{}       // Parsed content (for YAML/JSON)
	Hash          string            // SHA-256 hash of content
	PreviousHash  string            // Previous hash (for change tracking)
	LoadedAt      time.Time         // Timestamp when loaded
	Type          WorkspaceFileType // File category
	Size          int64             // File size in bytes
	IsCritical    bool              // Whether this is a critical file
}

// LoadedFile represents a file that has been loaded from disk
type LoadedFile struct {
	Path          string
	Content       string
	ParsedContent interface{}
	Hash          string
	Type          WorkspaceFileType
	Size          int64
}

// WorkspaceConfig holds configuration for the workspace manager
type WorkspaceConfig struct {
	WorkspacePath      string                    // Path to workspace directory
	MaxFileSize        int64                     // Maximum file size in bytes (default: 10MB)
	StabilityThreshold time.Duration             // Time to wait after file write (default: 100ms)
	EnableValidation   bool                      // Enable file validation (default: true)
	OnReload           func(file *WorkspaceFile) // Callback for file reload
}

// ValidationError represents a validation error
type ValidationError struct {
	Field   string
	Message string
}

// ValidationWarning represents a validation warning
type ValidationWarning struct {
	Field   string
	Message string
}

// ValidationResult holds the result of file validation
type ValidationResult struct {
	Valid    bool
	Errors   []ValidationError
	Warnings []ValidationWarning
}

// WorkspaceEvent represents event types emitted by the workspace manager
type WorkspaceEvent string

const (
	EventInitialized WorkspaceEvent = "workspace.initialized"
	EventFileAdded   WorkspaceEvent = "workspace.file.added"
	EventFileChanged WorkspaceEvent = "workspace.file.changed"
	EventFileDeleted WorkspaceEvent = "workspace.file.deleted"
	EventError       WorkspaceEvent = "workspace.error"
)

// EventPayload represents the base event payload
type EventPayload struct {
	Timestamp time.Time
}

// InitializedPayload is emitted when workspace initialization completes
type InitializedPayload struct {
	EventPayload
	FileCount int
}

// FileEventPayload is emitted for file operations
type FileEventPayload struct {
	EventPayload
	Path         string
	RelativePath string
	Type         WorkspaceFileType
}

// FileChangedPayload is emitted when a file changes
type FileChangedPayload struct {
	FileEventPayload
	HasChanges bool
}

// ErrorPayload is emitted when an error occurs
type ErrorPayload struct {
	EventPayload
	Error   error
	Context map[string]interface{}
}
