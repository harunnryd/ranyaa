package toolexecutor

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/xeipuuv/gojsonschema"
)

// ToolPolicy defines which tools an agent can use
type ToolPolicy struct {
	Allow []string `json:"allow"` // List of allowed tools (* for all)
	Deny  []string `json:"deny"`  // List of denied tools (overrides allow)
}

// IsToolAllowed checks if a tool is allowed by the policy
func (tp *ToolPolicy) IsToolAllowed(toolName string) bool {
	if tp == nil {
		// No policy means allow all
		return true
	}

	// Check deny list first (overrides allow list)
	for _, denied := range tp.Deny {
		if denied == toolName || denied == "*" {
			return false
		}
	}

	// Check allow list
	for _, allowed := range tp.Allow {
		if allowed == toolName || allowed == "*" {
			return true
		}
	}

	// If no explicit allow, deny by default
	return false
}

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
	PluginID    string          `json:"plugin_id,omitempty"`   // Plugin ID if this is a plugin tool
	Permissions []string        `json:"permissions,omitempty"` // Plugin permissions for approval decisions
}

// ToolHandler is the function signature for tool execution
type ToolHandler func(ctx context.Context, params map[string]interface{}) (interface{}, error)

// ExecutionContext provides runtime information for tool execution
type ExecutionContext struct {
	SessionKey    string
	WorkingDir    string
	Timeout       time.Duration
	SandboxPolicy map[string]interface{}
	AgentID       string // Agent ID for policy enforcement
	ToolPolicy    *ToolPolicy
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
	tools           map[string]*ToolDefinition
	schemas         map[string]*gojsonschema.Schema
	sandboxManager  *SandboxManager
	approvalManager *ApprovalManager
	pluginRuntime   PluginToolExecutor
	mu              sync.RWMutex
}

// New creates a new ToolExecutor
func New() *ToolExecutor {
	te := &ToolExecutor{
		tools:           make(map[string]*ToolDefinition),
		schemas:         make(map[string]*gojsonschema.Schema),
		sandboxManager:  nil, // Sandbox is optional
		approvalManager: nil, // Approval is optional
		pluginRuntime:   nil, // Plugin runtime is optional
	}

	log.Info().Msg("Tool executor initialized")

	return te
}

// PluginToolProvider is an interface for plugin tool execution
type PluginToolProvider interface {
	ExecuteTool(ctx context.Context, name string, params map[string]interface{}) (map[string]interface{}, error)
}

// PluginToolExecutor is an interface for executing plugin tools
type PluginToolExecutor interface {
	GetPlugin(pluginID string) (interface{}, error)
}

// PluginInfo provides metadata about a plugin for approval decisions
type PluginInfo interface {
	GetID() string
	GetPermissions() []string
}

// RegisterPluginTool registers a single plugin tool with conflict resolution
func (te *ToolExecutor) RegisterPluginTool(pluginID string, toolDef ToolDefinition, pluginRuntime PluginToolExecutor) error {
	// Check for name conflicts
	originalName := toolDef.Name
	toolName := originalName

	te.mu.Lock()
	defer te.mu.Unlock()

	// If tool name already exists, prefix with plugin name
	if _, exists := te.tools[toolName]; exists {
		toolName = pluginID + "_" + originalName
		log.Warn().
			Str("original_name", originalName).
			Str("prefixed_name", toolName).
			Str("plugin", pluginID).
			Msg("Tool name conflict resolved by prefixing with plugin name")
	}

	// Get plugin permissions for approval workflow
	var pluginPermissions []string
	plugin, err := pluginRuntime.GetPlugin(pluginID)
	if err == nil {
		if pluginInfo, ok := plugin.(PluginInfo); ok {
			pluginPermissions = pluginInfo.GetPermissions()
		}
	}

	// Create a handler that routes to the plugin through PluginRuntime
	handler := func(ctx context.Context, params map[string]interface{}) (interface{}, error) {
		log.Debug().
			Str("plugin", pluginID).
			Str("tool", originalName).
			Msg("Routing tool execution to plugin")

		// Get plugin provider from runtime
		plugin, err := pluginRuntime.GetPlugin(pluginID)
		if err != nil {
			return nil, fmt.Errorf("failed to get plugin %s: %w", pluginID, err)
		}

		// Type assert to PluginToolProvider interface
		provider, ok := plugin.(PluginToolProvider)
		if !ok {
			return nil, fmt.Errorf("plugin %s does not implement PluginToolProvider interface", pluginID)
		}

		// Execute tool through the plugin provider with timeout handling
		result, err := provider.ExecuteTool(ctx, originalName, params)
		if err != nil {
			// Check if error is due to context cancellation or timeout
			if ctx.Err() != nil {
				if ctx.Err() == context.DeadlineExceeded {
					return nil, fmt.Errorf("plugin tool execution timeout: %w", err)
				}
				return nil, fmt.Errorf("plugin tool execution cancelled: %w", err)
			}
			return nil, fmt.Errorf("plugin %s tool %s execution failed: %w", pluginID, originalName, err)
		}

		log.Debug().
			Str("plugin", pluginID).
			Str("tool", originalName).
			Msg("Plugin tool execution completed")

		return result, nil
	}

	// Create tool definition with the handler and plugin metadata
	wrappedDef := ToolDefinition{
		Name:        toolName,
		Description: fmt.Sprintf("[Plugin: %s] %s", pluginID, toolDef.Description),
		Parameters:  toolDef.Parameters,
		Handler:     handler,
		PluginID:    pluginID,
		Permissions: pluginPermissions,
	}

	// Validate and register
	if err := te.validateToolDefinition(wrappedDef); err != nil {
		return fmt.Errorf("invalid plugin tool definition: %w", err)
	}

	schema, err := te.generateJSONSchema(wrappedDef)
	if err != nil {
		return fmt.Errorf("failed to generate schema for plugin tool: %w", err)
	}

	te.tools[toolName] = &wrappedDef
	te.schemas[toolName] = schema

	log.Info().
		Str("tool", toolName).
		Str("plugin", pluginID).
		Strs("permissions", pluginPermissions).
		Msg("Plugin tool registered")

	return nil
}

// SetSandboxManager sets the sandbox manager for the tool executor
func (te *ToolExecutor) SetSandboxManager(manager *SandboxManager) {
	te.mu.Lock()
	defer te.mu.Unlock()
	te.sandboxManager = manager
}

// SetApprovalManager sets the approval manager for the tool executor
func (te *ToolExecutor) SetApprovalManager(manager *ApprovalManager) {
	te.mu.Lock()
	defer te.mu.Unlock()
	te.approvalManager = manager
	log.Info().Msg("Approval manager configured for tool executor")
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

	// Check tool policy if provided
	if execCtx != nil && execCtx.ToolPolicy != nil {
		if !execCtx.ToolPolicy.IsToolAllowed(toolName) {
			log.Warn().
				Str("tool", toolName).
				Str("agent_id", execCtx.AgentID).
				Msg("Tool execution blocked by policy")
			return ToolResult{
				Success: false,
				Error:   fmt.Sprintf("tool '%s' is not allowed by agent policy", toolName),
				Metadata: map[string]interface{}{
					"policy_violation": true,
					"agent_id":         execCtx.AgentID,
				},
			}
		}
	}

	// Check if sandbox is enabled
	te.mu.RLock()
	sandboxManager := te.sandboxManager
	te.mu.RUnlock()

	if sandboxManager != nil && execCtx != nil && execCtx.SandboxPolicy != nil {
		// Check if sandboxing is enabled
		mode, ok := execCtx.SandboxPolicy["mode"].(string)
		if ok && mode != "off" {
			// Use sandbox for execution
			return ExecuteWithSandbox(ctx, te, sandboxManager, toolName, params, execCtx)
		}
	}

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

// requiresApproval determines if a plugin tool requires approval based on its permissions
// Sensitive permissions that require approval:
// - filesystem:write - Can modify files
// - process:spawn - Can execute processes
// - database:write - Can modify database
// - network:http - Can make external HTTP requests
// - network:websocket - Can establish WebSocket connections
func (te *ToolExecutor) requiresApproval(tool *ToolDefinition) bool {
	if tool.PluginID == "" {
		// Not a plugin tool, no approval needed
		return false
	}

	sensitivePermissions := map[string]bool{
		"filesystem:write":  true,
		"process:spawn":     true,
		"database:write":    true,
		"network:http":      true,
		"network:websocket": true,
	}

	for _, perm := range tool.Permissions {
		if sensitivePermissions[perm] {
			return true
		}
	}

	return false
}
