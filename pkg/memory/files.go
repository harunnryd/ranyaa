package memory

import (
	"fmt"
	"os"
	"path/filepath"
)

// EnsureMemoryDirectory creates the memory directory if it doesn't exist
func EnsureMemoryDirectory(basePath string) (string, error) {
	memoryPath := filepath.Join(basePath, "memory")

	// Check if directory exists
	info, err := os.Stat(memoryPath)
	if err == nil {
		if !info.IsDir() {
			return "", fmt.Errorf("memory path exists but is not a directory: %s", memoryPath)
		}
		return memoryPath, nil
	}

	if !os.IsNotExist(err) {
		return "", fmt.Errorf("failed to stat memory directory: %w", err)
	}

	// Create directory
	if err := os.MkdirAll(memoryPath, 0755); err != nil {
		return "", fmt.Errorf("failed to create memory directory: %w", err)
	}

	return memoryPath, nil
}

// ValidateMemoryPath validates that a path is safe for memory operations
func ValidateMemoryPath(path string) error {
	if path == "" {
		return fmt.Errorf("path cannot be empty")
	}

	if filepath.IsAbs(path) {
		return fmt.Errorf("path must be relative, got absolute path: %s", path)
	}

	// Check for path traversal attempts
	cleanPath := filepath.Clean(path)
	if cleanPath != path {
		return fmt.Errorf("path contains invalid components: %s", path)
	}

	// Check for parent directory references
	if filepath.HasPrefix(cleanPath, "..") {
		return fmt.Errorf("path cannot reference parent directories: %s", path)
	}

	return nil
}

// GetMemoryFilePath constructs a full path for a memory file
func GetMemoryFilePath(basePath, relativePath string) (string, error) {
	if err := ValidateMemoryPath(relativePath); err != nil {
		return "", err
	}

	fullPath := filepath.Join(basePath, relativePath)

	// Ensure the path is still within basePath (defense in depth)
	absBase, err := filepath.Abs(basePath)
	if err != nil {
		return "", fmt.Errorf("failed to get absolute base path: %w", err)
	}

	absFull, err := filepath.Abs(fullPath)
	if err != nil {
		return "", fmt.Errorf("failed to get absolute full path: %w", err)
	}

	if !filepath.HasPrefix(absFull, absBase) {
		return "", fmt.Errorf("path escapes base directory: %s", relativePath)
	}

	return fullPath, nil
}

// FileExists checks if a file exists at the given path
func FileExists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}
