package workspace

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveEnvVars(t *testing.T) {
	// Set up test environment variables
	os.Setenv("TEST_VAR", "test_value")
	os.Setenv("API_KEY", "secret123")
	defer func() {
		os.Unsetenv("TEST_VAR")
		os.Unsetenv("API_KEY")
	}()

	tests := []struct {
		name     string
		input    interface{}
		expected interface{}
	}{
		{
			name:     "simple string with env var",
			input:    "${TEST_VAR}",
			expected: "test_value",
		},
		{
			name:     "string with multiple env vars",
			input:    "prefix_${TEST_VAR}_${API_KEY}_suffix",
			expected: "prefix_test_value_secret123_suffix",
		},
		{
			name:     "undefined env var",
			input:    "${UNDEFINED_VAR}",
			expected: "",
		},
		{
			name:     "string without env vars",
			input:    "plain string",
			expected: "plain string",
		},
		{
			name: "map with env vars",
			input: map[string]interface{}{
				"key1": "${TEST_VAR}",
				"key2": "value2",
			},
			expected: map[string]interface{}{
				"key1": "test_value",
				"key2": "value2",
			},
		},
		{
			name: "array with env vars",
			input: []interface{}{
				"${TEST_VAR}",
				"plain",
				"${API_KEY}",
			},
			expected: []interface{}{
				"test_value",
				"plain",
				"secret123",
			},
		},
		{
			name: "nested structure",
			input: map[string]interface{}{
				"outer": map[string]interface{}{
					"inner": "${TEST_VAR}",
				},
				"array": []interface{}{
					"${API_KEY}",
				},
			},
			expected: map[string]interface{}{
				"outer": map[string]interface{}{
					"inner": "test_value",
				},
				"array": []interface{}{
					"secret123",
				},
			},
		},
		{
			name:     "non-string value",
			input:    123,
			expected: 123,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := resolveEnvVars(tt.input)

			// Deep comparison
			if !deepEqual(result, tt.expected) {
				t.Errorf("resolveEnvVars() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestResolveFileRefs(t *testing.T) {
	// Create temporary workspace
	tmpDir := t.TempDir()

	// Create test files
	testFile1 := filepath.Join(tmpDir, "test1.txt")
	testFile2 := filepath.Join(tmpDir, "test2.txt")

	os.WriteFile(testFile1, []byte("content from file 1"), 0644)
	os.WriteFile(testFile2, []byte("content from file 2"), 0644)

	loader := NewWorkspaceLoader(tmpDir)

	tests := []struct {
		name      string
		input     interface{}
		expected  interface{}
		expectErr bool
	}{
		{
			name:     "simple file reference",
			input:    "file://" + testFile1,
			expected: "content from file 1",
		},
		{
			name:     "non-existent file",
			input:    "file://" + filepath.Join(tmpDir, "nonexistent.txt"),
			expected: "",
		},
		{
			name:     "string without file reference",
			input:    "plain string",
			expected: "plain string",
		},
		{
			name: "map with file references",
			input: map[string]interface{}{
				"key1": "file://" + testFile1,
				"key2": "plain value",
			},
			expected: map[string]interface{}{
				"key1": "content from file 1",
				"key2": "plain value",
			},
		},
		{
			name: "array with file references",
			input: []interface{}{
				"file://" + testFile1,
				"plain",
				"file://" + testFile2,
			},
			expected: []interface{}{
				"content from file 1",
				"plain",
				"content from file 2",
			},
		},
		{
			name: "nested structure",
			input: map[string]interface{}{
				"outer": map[string]interface{}{
					"inner": "file://" + testFile1,
				},
				"array": []interface{}{
					"file://" + testFile2,
				},
			},
			expected: map[string]interface{}{
				"outer": map[string]interface{}{
					"inner": "content from file 1",
				},
				"array": []interface{}{
					"content from file 2",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := resolveFileRefs(tt.input, loader, 0)

			if tt.expectErr {
				if err == nil {
					t.Errorf("resolveFileRefs() expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("resolveFileRefs() unexpected error: %v", err)
				return
			}

			if !deepEqual(result, tt.expected) {
				t.Errorf("resolveFileRefs() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestResolveFileRefsDepthLimit(t *testing.T) {
	tmpDir := t.TempDir()
	loader := NewWorkspaceLoader(tmpDir)

	// Create deeply nested structure
	input := map[string]interface{}{"level": 0}
	current := input
	for i := 1; i <= 15; i++ {
		nested := map[string]interface{}{"level": i}
		current["nested"] = nested
		current = nested
	}

	_, err := resolveFileRefs(input, loader, 0)
	if err == nil {
		t.Error("resolveFileRefs() expected depth limit error, got nil")
	}
}

func TestResolveConfig(t *testing.T) {
	// Set up test environment
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "content.txt")
	os.WriteFile(testFile, []byte("file content"), 0644)

	os.Setenv("TEST_VAR", "env_value")
	defer os.Unsetenv("TEST_VAR")

	loader := NewWorkspaceLoader(tmpDir)

	tests := []struct {
		name     string
		input    interface{}
		expected interface{}
	}{
		{
			name: "combined env vars and file refs",
			input: map[string]interface{}{
				"env":  "${TEST_VAR}",
				"file": "file://" + testFile,
			},
			expected: map[string]interface{}{
				"env":  "env_value",
				"file": "file content",
			},
		},
		{
			name:     "env var in file reference path",
			input:    "prefix_${TEST_VAR}",
			expected: "prefix_env_value",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ResolveConfig(tt.input, loader)
			if err != nil {
				t.Errorf("ResolveConfig() unexpected error: %v", err)
				return
			}

			if !deepEqual(result, tt.expected) {
				t.Errorf("ResolveConfig() = %v, want %v", result, tt.expected)
			}
		})
	}
}

// deepEqual performs deep comparison of two values
func deepEqual(a, b interface{}) bool {
	switch va := a.(type) {
	case string:
		vb, ok := b.(string)
		return ok && va == vb

	case int:
		vb, ok := b.(int)
		return ok && va == vb

	case []interface{}:
		vb, ok := b.([]interface{})
		if !ok || len(va) != len(vb) {
			return false
		}
		for i := range va {
			if !deepEqual(va[i], vb[i]) {
				return false
			}
		}
		return true

	case map[string]interface{}:
		vb, ok := b.(map[string]interface{})
		if !ok || len(va) != len(vb) {
			return false
		}
		for key, val := range va {
			if !deepEqual(val, vb[key]) {
				return false
			}
		}
		return true

	default:
		return a == b
	}
}
