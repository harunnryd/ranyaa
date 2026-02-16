package toolexecutor

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/xeipuuv/gojsonschema"
)

// ToolParameter defines a parameter for a tool
type ToolParameter struct {
	Name        string      `json:"name"`
	Type        string      `json:"type"`
	Description string      `json:"description"`
	Required    bool        `json:"required"`
	Default     interface{} `json:"default,omitempty"`
}

// ToolDefinition defines a tool's metadata and handler
type ToolDefinition struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  []ToolParameter `json:"parameters"`
	Handler     ToolHandler     `json:"-"`
}

// ToolHandler is the function signature for tool execution
type ToolHandler func(ctx context.Context, params map[string]interface{}) (interface{}, error)

// ExecutionContext provides runtime information for tool execution
type ExecutionContext struct {
	SessionKey    string
	WorkingDir    string
	Timeout       time.Duration
	SandboxPolicy map[string]interface{}
}

// ToolResult represents the result of a tool execution
type ToolResult struct {
	Success   bool                   `json:"success"`
	Output    interface{}            `json:"output,omitempty"`
	Error     string                 `json:"error,omitempty"`
	Truncated bool                   `json:"truncated,omitempty"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
}

// ToolExecutor manages and executes tools
type ToolExecutor struct {
	tools   map[string]*ToolDefinition
	schemas map[string]*gojsonschema.Schema
	mu      sync.RWMutex
}

// New creates a new ToolExecutor
func New() *ToolExecutor {
	te := &ToolExecutor{
		tools:   make(map[string]*ToolDefinition),
		schemas: make(map[string]*gojsonschema.Schema),
	}

	log.Info().Msg("Tool executor initialized")

	return te
}

// RegisterTool registers a new tool
func (te *ToolExecutor) RegisterTool(def ToolDefinition) error {
	// Validate tool definition
	if err := te.validateToolDefinition(def); err != nil {
		return fmt.Errorf("invalid tool definition: %w", err)
	}

	// Generate JSON Schema
	schema, err := te.generateJSONSchema(def)
	if err != nil {
		return fmt.Errorf("failed to generate schema: %w", err)
	}

	te.mu.Lock()
	defer te.mu.Unlock()

	te.tools[def.Name] = &def
	te.schemas[def.Name] = schema

	log.Info().Str("tool", def.Name).Msg("Tool registered")

	return nil
}

// UnregisterTool removes a tool
func (te *ToolExecutor) UnregisterTool(name string) {
	te.mu.Lock()
	defer te.mu.Unlock()

	delete(te.tools, name)
	delete(te.schemas, name)

	log.Info().Str("tool", name).Msg("Tool unregistered")
}

// GetTool returns a tool definition by name
func (te *ToolExecutor) GetTool(name string) *ToolDefinition {
	te.mu.RLock()
	defer te.mu.RUnlock()

	return te.tools[name]
}

// ListTools returns all registered tool names
func (te *ToolExecutor) ListTools() []string {
	te.mu.RLock()
	defer te.mu.RUnlock()

	tools := make([]string, 0, len(te.tools))
	for name := range te.tools {
		tools = append(tools, name)
	}

	return tools
}

// GetToolCount returns the number of registered tools
func (te *ToolExecutor) GetToolCount() int {
	te.mu.RLock()
	defer te.mu.RUnlock()

	return len(te.tools)
}

// Execute executes a tool with the given parameters
func (te *ToolExecutor) Execute(ctx context.Context, toolName string, params map[string]interface{}, execCtx *ExecutionContext) ToolResult {
	startTime := time.Now()

	// Get tool
	te.mu.RLock()
	tool := te.tools[toolName]
	schema := te.schemas[toolName]
	te.mu.RUnlock()

	if tool == nil {
		log.Error().Str("tool", toolName).Msg("Tool not found")
		return ToolResult{
			Success: false,
			Error:   fmt.Sprintf("tool not found: %s", toolName),
		}
	}

	// Validate parameters
	if err := te.validateParameters(schema, params); err != nil {
		log.Error().Str("tool", toolName).Err(err).Msg("Parameter validation failed")
		return ToolResult{
			Success: false,
			Error:   fmt.Sprintf("parameter validation failed: %v", err),
		}
	}

	log.Debug().Str("tool", toolName).Msg("Executing tool")

	// Apply timeout
	timeout := 30 * time.Second
	if execCtx != nil && execCtx.Timeout > 0 {
		timeout = execCtx.Timeout
	}

	timeoutCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Execute tool
	resultChan := make(chan interface{}, 1)
	errChan := make(chan error, 1)

	go func() {
		result, err := tool.Handler(timeoutCtx, params)
		if err != nil {
			errChan <- err
		} else {
			resultChan <- result
		}
	}()

	// Wait for result or timeout
	select {
	case result := <-resultChan:
		duration := time.Since(startTime)

		// Truncate output if too large
		output, truncated := te.truncateOutput(result)

		log.Debug().
			Str("tool", toolName).
			Dur("duration", duration).
			Bool("truncated", truncated).
			Msg("Tool execution completed")

		return ToolResult{
			Success:   true,
			Output:    output,
			Truncated: truncated,
			Metadata: map[string]interface{}{
				"duration": duration.Milliseconds(),
			},
		}

	case err := <-errChan:
		duration := time.Since(startTime)

		log.Error().
			Str("tool", toolName).
			Dur("duration", duration).
			Err(err).
			Msg("Tool execution failed")

		return ToolResult{
			Success: false,
			Error:   err.Error(),
			Metadata: map[string]interface{}{
				"duration": duration.Milliseconds(),
			},
		}

	case <-timeoutCtx.Done():
		duration := time.Since(startTime)

		log.Error().
			Str("tool", toolName).
			Dur("duration", duration).
			Msg("Tool execution timeout")

		return ToolResult{
			Success: false,
			Error:   fmt.Sprintf("tool execution timeout after %v", timeout),
			Metadata: map[string]interface{}{
				"duration": duration.Milliseconds(),
			},
		}
	}
}

// validateToolDefinition validates a tool definition
func (te *ToolExecutor) validateToolDefinition(def ToolDefinition) error {
	if def.Name == "" {
		return fmt.Errorf("tool name cannot be empty")
	}
	if def.Description == "" {
		return fmt.Errorf("tool description cannot be empty")
	}
	if def.Handler == nil {
		return fmt.Errorf("tool handler cannot be nil")
	}

	// Validate parameters
	for _, param := range def.Parameters {
		if param.Name == "" {
			return fmt.Errorf("parameter name cannot be empty")
		}
		if param.Type == "" {
			return fmt.Errorf("parameter type cannot be empty for %s", param.Name)
		}
		if param.Description == "" {
			return fmt.Errorf("parameter description cannot be empty for %s", param.Name)
		}

		// Validate type
		validTypes := map[string]bool{
			"string": true, "number": true, "boolean": true,
			"object": true, "array": true, "integer": true,
		}
		if !validTypes[param.Type] {
			return fmt.Errorf("invalid parameter type %s for %s", param.Type, param.Name)
		}
	}

	return nil
}

// generateJSONSchema generates a JSON Schema from tool parameters
func (te *ToolExecutor) generateJSONSchema(def ToolDefinition) (*gojsonschema.Schema, error) {
	schemaMap := map[string]interface{}{
		"type":                 "object",
		"additionalProperties": false,
		"properties":           make(map[string]interface{}),
	}

	properties := schemaMap["properties"].(map[string]interface{})
	required := []string{}

	for _, param := range def.Parameters {
		paramSchema := map[string]interface{}{
			"type":        param.Type,
			"description": param.Description,
		}

		if param.Default != nil {
			paramSchema["default"] = param.Default
		}

		properties[param.Name] = paramSchema

		if param.Required {
			required = append(required, param.Name)
		}
	}

	if len(required) > 0 {
		schemaMap["required"] = required
	}

	schemaLoader := gojsonschema.NewGoLoader(schemaMap)
	schema, err := gojsonschema.NewSchema(schemaLoader)
	if err != nil {
		return nil, err
	}

	return schema, nil
}

// validateParameters validates parameters against a JSON Schema
func (te *ToolExecutor) validateParameters(schema *gojsonschema.Schema, params map[string]interface{}) error {
	if schema == nil {
		return nil
	}

	paramsLoader := gojsonschema.NewGoLoader(params)
	result, err := schema.Validate(paramsLoader)
	if err != nil {
		return err
	}

	if !result.Valid() {
		errors := []string{}
		for _, err := range result.Errors() {
			errors = append(errors, err.String())
		}
		return fmt.Errorf("validation errors: %v", errors)
	}

	return nil
}

// truncateOutput truncates output if it exceeds the size limit
func (te *ToolExecutor) truncateOutput(output interface{}) (interface{}, bool) {
	const maxSize = 10 * 1024 // 10KB

	// Convert to string for size check
	str := fmt.Sprintf("%v", output)

	if len(str) <= maxSize {
		return output, false
	}

	truncated := str[:maxSize] + "\n... [output truncated]"
	log.Warn().
		Int("original", len(str)).
		Int("truncated", maxSize).
		Msg("Output truncated")

	return truncated, true
}
