package workspace

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestWorkspaceValidator_ValidateMarkdownFile(t *testing.T) {
	validator := NewWorkspaceValidator()

	file := &LoadedFile{
		Path:    "/workspace/AGENTS.md",
		Content: "# Agent Instructions\n\nYou are an AI assistant.",
		Type:    FileTypeAgents,
		Size:    100,
	}

	result := validator.Validate(file)
	assert.True(t, result.Valid)
	assert.Empty(t, result.Errors)
}

func TestWorkspaceValidator_ValidateEmptyMarkdown(t *testing.T) {
	validator := NewWorkspaceValidator()

	file := &LoadedFile{
		Path:    "/workspace/AGENTS.md",
		Content: "",
		Type:    FileTypeAgents,
		Size:    0,
	}

	result := validator.Validate(file)
	assert.True(t, result.Valid) // Still valid, just a warning
	assert.Len(t, result.Warnings, 1)
	assert.Equal(t, "File is empty", result.Warnings[0].Message)
}

func TestWorkspaceValidator_ValidateLargeMarkdown(t *testing.T) {
	validator := NewWorkspaceValidator()

	file := &LoadedFile{
		Path:    "/workspace/AGENTS.md",
		Content: GenerateLargeFile(MaxRecommendedFileSize + 1000),
		Type:    FileTypeAgents,
		Size:    MaxRecommendedFileSize + 1000,
	}

	result := validator.Validate(file)
	assert.True(t, result.Valid) // Still valid, just a warning
	assert.Len(t, result.Warnings, 1)
	assert.Contains(t, result.Warnings[0].Message, "exceeds recommended maximum")
}

func TestWorkspaceValidator_ValidateConfigFile(t *testing.T) {
	validator := NewWorkspaceValidator()

	file := &LoadedFile{
		Path:          "/workspace/config/test.yaml",
		Content:       "key: value\n",
		ParsedContent: map[string]interface{}{"key": "value"},
		Type:          FileTypeConfig,
		Size:          100,
	}

	result := validator.Validate(file)
	assert.True(t, result.Valid)
	assert.Empty(t, result.Errors)
}

func TestWorkspaceValidator_ValidateInvalidConfig(t *testing.T) {
	validator := NewWorkspaceValidator()

	file := &LoadedFile{
		Path:          "/workspace/config/test.yaml",
		Content:       "invalid yaml",
		ParsedContent: nil, // Failed to parse
		Type:          FileTypeConfig,
		Size:          100,
	}

	result := validator.Validate(file)
	assert.False(t, result.Valid)
	assert.Len(t, result.Errors, 1)
	assert.Equal(t, "Failed to parse config file", result.Errors[0].Message)
}

func TestWorkspaceValidator_ValidatePluginManifest(t *testing.T) {
	validator := NewWorkspaceValidator()

	file := &LoadedFile{
		Path:    "/workspace/plugins/test/plugin.json",
		Content: `{"id": "test", "name": "Test", "version": "1.0.0"}`,
		ParsedContent: map[string]interface{}{
			"id":      "test",
			"name":    "Test",
			"version": "1.0.0",
		},
		Type: FileTypePluginManifest,
		Size: 100,
	}

	result := validator.Validate(file)
	assert.True(t, result.Valid)
	assert.Empty(t, result.Errors)
}

func TestWorkspaceValidator_ValidatePluginManifestMissingFields(t *testing.T) {
	validator := NewWorkspaceValidator()

	file := &LoadedFile{
		Path:    "/workspace/plugins/test/plugin.json",
		Content: `{"name": "Test"}`,
		ParsedContent: map[string]interface{}{
			"name": "Test",
		},
		Type: FileTypePluginManifest,
		Size: 100,
	}

	result := validator.Validate(file)
	assert.False(t, result.Valid)
	assert.Len(t, result.Errors, 2) // Missing id and version
}

func TestWorkspaceValidator_SanitizeContent(t *testing.T) {
	validator := NewWorkspaceValidator()

	tests := []struct {
		name            string
		content         string
		expectWarning   bool
		warningContains string
	}{
		{
			name:          "Clean content",
			content:       "This is clean content",
			expectWarning: false,
		},
		{
			name:            "Script tag",
			content:         "Some text <script>alert('xss')</script>",
			expectWarning:   true,
			warningContains: "<script",
		},
		{
			name:            "JavaScript protocol",
			content:         "Link: javascript:alert('xss')",
			expectWarning:   true,
			warningContains: "javascript:",
		},
		{
			name:            "Eval function",
			content:         "Code: eval('malicious')",
			expectWarning:   true,
			warningContains: "eval(",
		},
		{
			name:            "Function constructor",
			content:         "Code: new function('return 1')",
			expectWarning:   true,
			warningContains: "function(",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			file := &LoadedFile{
				Path:    "/workspace/test.md",
				Content: tt.content,
				Type:    FileTypeOther,
				Size:    int64(len(tt.content)),
			}

			result := validator.Validate(file)

			if tt.expectWarning {
				assert.NotEmpty(t, result.Warnings)
				found := false
				for _, warning := range result.Warnings {
					if assert.Contains(t, warning.Message, tt.warningContains) {
						found = true
						break
					}
				}
				assert.True(t, found, "Expected warning containing '%s'", tt.warningContains)
			} else {
				assert.Empty(t, result.Warnings)
			}
		})
	}
}
