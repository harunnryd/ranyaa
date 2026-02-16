package workspace

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	// MaxFileSize is the maximum allowed file size (10MB)
	MaxFileSize = 10 * 1024 * 1024
)

// WorkspaceLoader handles loading and parsing workspace files
type WorkspaceLoader struct {
	workspacePath string
}

// NewWorkspaceLoader creates a new workspace loader
func NewWorkspaceLoader(workspacePath string) *WorkspaceLoader {
	return &WorkspaceLoader{
		workspacePath: workspacePath,
	}
}

// LoadFile loads a file from the workspace
func (l *WorkspaceLoader) LoadFile(filePath string) (*LoadedFile, error) {
	// Validate path
	if err := l.validatePath(filePath); err != nil {
		return nil, fmt.Errorf("invalid path: %w", err)
	}

	// Read file content
	content, err := l.readFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	// Get file info for size
	info, err := os.Stat(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to stat file: %w", err)
	}

	// Validate file size
	if info.Size() > MaxFileSize {
		return nil, fmt.Errorf("file size %d exceeds maximum %d", info.Size(), MaxFileSize)
	}

	// Detect file type
	fileType := l.detectFileType(filePath)

	// Parse content
	parsedContent, err := l.parseContent(content, fileType)
	if err != nil {
		return nil, fmt.Errorf("failed to parse content: %w", err)
	}

	// Compute hash
	hash := l.computeHash(content)

	return &LoadedFile{
		Path:          filePath,
		Content:       content,
		ParsedContent: parsedContent,
		Hash:          hash,
		Type:          fileType,
		Size:          info.Size(),
	}, nil
}

// readFile reads the content of a file
func (l *WorkspaceLoader) readFile(filePath string) (string, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// parseContent parses file content based on file type
func (l *WorkspaceLoader) parseContent(content string, fileType WorkspaceFileType) (interface{}, error) {
	switch fileType {
	case FileTypeConfig:
		// Try YAML first, then JSON
		var result interface{}
		err := yaml.Unmarshal([]byte(content), &result)
		if err != nil {
			// Try JSON
			err = json.Unmarshal([]byte(content), &result)
			if err != nil {
				return nil, fmt.Errorf("failed to parse as YAML or JSON: %w", err)
			}
		}
		return result, nil

	case FileTypePluginManifest:
		// Parse as JSON
		var result interface{}
		err := json.Unmarshal([]byte(content), &result)
		if err != nil {
			return nil, fmt.Errorf("failed to parse JSON: %w", err)
		}
		return result, nil

	case FileTypeAgents, FileTypeSoul, FileTypeTools, FileTypeMemory,
		FileTypeWebhooks, FileTypeCron, FileTypePlugins, FileTypeSkill:
		// Return raw markdown content
		return content, nil

	default:
		// Return raw content for other types
		return content, nil
	}
}

// detectFileType detects the file type based on path and name
func (l *WorkspaceLoader) detectFileType(filePath string) WorkspaceFileType {
	basename := filepath.Base(filePath)
	ext := filepath.Ext(filePath)
	dir := filepath.Dir(filePath)

	// Core workspace files
	switch basename {
	case "AGENTS.md":
		return FileTypeAgents
	case "SOUL.md":
		return FileTypeSoul
	case "TOOLS.md":
		return FileTypeTools
	case "MEMORY.md":
		return FileTypeMemory
	case "WEBHOOKS.md":
		return FileTypeWebhooks
	case "CRON.md":
		return FileTypeCron
	case "PLUGINS.md":
		return FileTypePlugins
	case "SKILL.md":
		return FileTypeSkill
	case "plugin.json":
		return FileTypePluginManifest
	}

	// Extension-based detection
	switch ext {
	case ".yaml", ".yml":
		return FileTypeConfig
	case ".json":
		if basename != "plugin.json" {
			return FileTypeConfig
		}
	case ".go":
		// Check if in plugins directory
		if strings.Contains(dir, "plugins") {
			return FileTypePluginCode
		}
	}

	return FileTypeOther
}

// computeHash computes SHA-256 hash of content
func (l *WorkspaceLoader) computeHash(content string) string {
	hash := sha256.Sum256([]byte(content))
	return hex.EncodeToString(hash[:])
}

// validatePath validates that the file path is within the workspace directory
func (l *WorkspaceLoader) validatePath(filePath string) error {
	// Get absolute paths
	absWorkspace, err := filepath.Abs(l.workspacePath)
	if err != nil {
		return fmt.Errorf("failed to get absolute workspace path: %w", err)
	}

	absFile, err := filepath.Abs(filePath)
	if err != nil {
		return fmt.Errorf("failed to get absolute file path: %w", err)
	}

	// Check for directory traversal
	if strings.Contains(filePath, "..") {
		return fmt.Errorf("path contains directory traversal: %s", filePath)
	}

	// Check if file is within workspace
	relPath, err := filepath.Rel(absWorkspace, absFile)
	if err != nil {
		return fmt.Errorf("failed to get relative path: %w", err)
	}

	if strings.HasPrefix(relPath, "..") {
		return fmt.Errorf("path is outside workspace: %s", filePath)
	}

	return nil
}
