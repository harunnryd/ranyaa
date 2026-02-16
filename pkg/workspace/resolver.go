package workspace

import (
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/rs/zerolog/log"
)

const (
	// MaxResolutionDepth prevents infinite recursion in file reference resolution
	MaxResolutionDepth = 10
)

// resolveEnvVars recursively resolves environment variables in a value
// Replaces ${VAR_NAME} patterns with environment variable values
// Only resolves string values, not keys or non-string values
func resolveEnvVars(value interface{}) interface{} {
	switch v := value.(type) {
	case string:
		// Replace ${VAR_NAME} with environment variable
		re := regexp.MustCompile(`\$\{([^}]+)\}`)
		return re.ReplaceAllStringFunc(v, func(match string) string {
			// Extract variable name
			varName := match[2 : len(match)-1] // Remove ${ and }

			// Get environment variable
			envValue := os.Getenv(varName)
			if envValue == "" {
				log.Warn().
					Str("variable", varName).
					Msg("Environment variable not found, using empty string")
			}

			return envValue
		})

	case []interface{}:
		// Recursively resolve array elements
		result := make([]interface{}, len(v))
		for i, item := range v {
			result[i] = resolveEnvVars(item)
		}
		return result

	case map[string]interface{}:
		// Recursively resolve object values (not keys)
		result := make(map[string]interface{})
		for key, val := range v {
			result[key] = resolveEnvVars(val)
		}
		return result

	default:
		// Return non-string values as-is
		return value
	}
}

// resolveFileRefs recursively resolves file references in a value
// Replaces file://FILENAME patterns with file content
// Prevents circular references by tracking resolution depth
func resolveFileRefs(value interface{}, loader *WorkspaceLoader, depth int) (interface{}, error) {
	// Prevent infinite recursion
	if depth > MaxResolutionDepth {
		return nil, fmt.Errorf("maximum file reference depth (%d) exceeded", MaxResolutionDepth)
	}

	switch v := value.(type) {
	case string:
		// Check for file:// reference
		if strings.HasPrefix(v, "file://") {
			filename := strings.TrimPrefix(v, "file://")

			// Load referenced file
			content, err := loader.readFile(filename)
			if err != nil {
				log.Warn().
					Err(err).
					Str("filename", filename).
					Msg("Referenced file not found, using empty string")
				return "", nil
			}

			return content, nil
		}
		return v, nil

	case []interface{}:
		// Recursively resolve array elements
		result := make([]interface{}, len(v))
		for i, item := range v {
			resolved, err := resolveFileRefs(item, loader, depth+1)
			if err != nil {
				return nil, err
			}
			result[i] = resolved
		}
		return result, nil

	case map[string]interface{}:
		// Recursively resolve object values
		result := make(map[string]interface{})
		for key, val := range v {
			resolved, err := resolveFileRefs(val, loader, depth+1)
			if err != nil {
				return nil, err
			}
			result[key] = resolved
		}
		return result, nil

	default:
		// Return non-string values as-is
		return value, nil
	}
}

// ResolveConfig resolves both environment variables and file references in config
// Environment variables are resolved first, then file references
func ResolveConfig(config interface{}, loader *WorkspaceLoader) (interface{}, error) {
	// First resolve environment variables
	resolved := resolveEnvVars(config)

	// Then resolve file references
	resolved, err := resolveFileRefs(resolved, loader, 0)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve file references: %w", err)
	}

	return resolved, nil
}
