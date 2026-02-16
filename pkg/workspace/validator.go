package workspace

import (
	"fmt"
	"strings"
)

const (
	// MaxRecommendedFileSize is the recommended maximum file size (100KB)
	MaxRecommendedFileSize = 100 * 1024
)

// WorkspaceValidator validates workspace file content
type WorkspaceValidator struct{}

// NewWorkspaceValidator creates a new workspace validator
func NewWorkspaceValidator() *WorkspaceValidator {
	return &WorkspaceValidator{}
}

// Validate validates a loaded file
func (v *WorkspaceValidator) Validate(file *LoadedFile) ValidationResult {
	result := ValidationResult{
		Valid:    true,
		Errors:   []ValidationError{},
		Warnings: []ValidationWarning{},
	}

	// Validate based on file type
	switch file.Type {
	case FileTypeAgents, FileTypeSoul, FileTypeTools, FileTypeMemory,
		FileTypeWebhooks, FileTypeCron, FileTypePlugins, FileTypeSkill:
		v.validateMarkdownFile(file, &result)

	case FileTypeConfig:
		v.validateConfigFile(file, &result)

	case FileTypePluginManifest:
		v.validatePluginManifest(file, &result)
	}

	// Sanitize content (basic check for suspicious patterns)
	v.sanitizeContent(file, &result)

	return result
}

// validateMarkdownFile validates markdown files
func (v *WorkspaceValidator) validateMarkdownFile(file *LoadedFile, result *ValidationResult) {
	// Check if empty
	if len(strings.TrimSpace(file.Content)) == 0 {
		result.Warnings = append(result.Warnings, ValidationWarning{
			Field:   "content",
			Message: "File is empty",
		})
	}

	// Warn if file is too large
	if file.Size > MaxRecommendedFileSize {
		result.Warnings = append(result.Warnings, ValidationWarning{
			Field:   "size",
			Message: fmt.Sprintf("File size %d exceeds recommended maximum %d (may impact performance)", file.Size, MaxRecommendedFileSize),
		})
	}
}

// validateConfigFile validates config files (YAML/JSON)
func (v *WorkspaceValidator) validateConfigFile(file *LoadedFile, result *ValidationResult) {
	// Check if parsed content is nil
	if file.ParsedContent == nil {
		result.Errors = append(result.Errors, ValidationError{
			Field:   "content",
			Message: "Failed to parse config file",
		})
		result.Valid = false
		return
	}

	// Warn if file is too large
	if file.Size > MaxRecommendedFileSize {
		result.Warnings = append(result.Warnings, ValidationWarning{
			Field:   "size",
			Message: fmt.Sprintf("File size %d exceeds recommended maximum %d", file.Size, MaxRecommendedFileSize),
		})
	}
}

// validatePluginManifest validates plugin.json files
func (v *WorkspaceValidator) validatePluginManifest(file *LoadedFile, result *ValidationResult) {
	// Check if parsed content is nil
	if file.ParsedContent == nil {
		result.Errors = append(result.Errors, ValidationError{
			Field:   "content",
			Message: "Failed to parse plugin manifest",
		})
		result.Valid = false
		return
	}

	// Check for required fields
	manifest, ok := file.ParsedContent.(map[string]interface{})
	if !ok {
		result.Errors = append(result.Errors, ValidationError{
			Field:   "content",
			Message: "Plugin manifest must be a JSON object",
		})
		result.Valid = false
		return
	}

	// Check required fields
	requiredFields := []string{"id", "name", "version"}
	for _, field := range requiredFields {
		if _, ok := manifest[field]; !ok {
			result.Errors = append(result.Errors, ValidationError{
				Field:   field,
				Message: fmt.Sprintf("Required field '%s' is missing", field),
			})
			result.Valid = false
		}
	}
}

// sanitizeContent performs basic content sanitization checks
func (v *WorkspaceValidator) sanitizeContent(file *LoadedFile, result *ValidationResult) {
	// Check for suspicious patterns that might indicate code injection attempts
	suspiciousPatterns := []string{
		"<script",
		"javascript:",
		"eval(",
		"function(",
	}

	contentLower := strings.ToLower(file.Content)
	for _, pattern := range suspiciousPatterns {
		if strings.Contains(contentLower, pattern) {
			result.Warnings = append(result.Warnings, ValidationWarning{
				Field:   "content",
				Message: fmt.Sprintf("Suspicious pattern detected: %s", pattern),
			})
		}
	}
}
