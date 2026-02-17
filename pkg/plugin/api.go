package plugin

import (
	"context"
	"fmt"
	"sync"

	"github.com/rs/zerolog"
)

// PluginAPIImpl implements the PluginAPI interface
type PluginAPIImpl struct {
	pluginID     string
	config       map[string]any
	sandbox      *SandboxContext
	toolRegistry *ToolRegistry
	hookRegistry *HookRegistry
	logger       zerolog.Logger
	mu           sync.RWMutex
}

// NewPluginAPI creates a new plugin API instance
func NewPluginAPI(
	pluginID string,
	config map[string]any,
	sandbox *SandboxContext,
	toolRegistry *ToolRegistry,
	hookRegistry *HookRegistry,
	logger zerolog.Logger,
) *PluginAPIImpl {
	return &PluginAPIImpl{
		pluginID:     pluginID,
		config:       config,
		sandbox:      sandbox,
		toolRegistry: toolRegistry,
		hookRegistry: hookRegistry,
		logger:       logger.With().Str("plugin", pluginID).Logger(),
	}
}

func (api *PluginAPIImpl) RegisterTool(ctx context.Context, definition ToolDefinition) error {
	api.mu.Lock()
	defer api.mu.Unlock()

	// Validate tool definition
	if definition.Name == "" {
		return fmt.Errorf("tool name cannot be empty")
	}
	if definition.Description == "" {
		return fmt.Errorf("tool description cannot be empty")
	}
	if definition.Parameters == nil {
		return fmt.Errorf("tool parameters cannot be nil")
	}

	// Register tool
	if err := api.toolRegistry.Register(api.pluginID, definition); err != nil {
		return err
	}

	api.logger.Debug().Str("tool", definition.Name).Msg("Registered tool")
	return nil
}

func (api *PluginAPIImpl) RegisterHook(ctx context.Context, definition HookDefinition) error {
	api.mu.Lock()
	defer api.mu.Unlock()

	// Validate hook definition
	if definition.Event == "" {
		return fmt.Errorf("hook event cannot be empty")
	}

	// Register hook
	if err := api.hookRegistry.Register(api.pluginID, definition); err != nil {
		return err
	}

	api.logger.Debug().Str("event", definition.Event).Msg("Registered hook")
	return nil
}

func (api *PluginAPIImpl) RegisterChannel(ctx context.Context, definition ChannelDefinition) error {
	api.mu.Lock()
	defer api.mu.Unlock()

	// Validate channel definition
	if definition.Protocol == "" {
		return fmt.Errorf("channel protocol cannot be empty")
	}
	if definition.Name == "" {
		return fmt.Errorf("channel name cannot be empty")
	}

	api.logger.Debug().
		Str("protocol", definition.Protocol).
		Str("name", definition.Name).
		Msg("Registered channel")
	return nil
}

func (api *PluginAPIImpl) RegisterProvider(ctx context.Context, definition ProviderDefinition) error {
	api.mu.Lock()
	defer api.mu.Unlock()

	// Validate provider definition
	if definition.Type == "" {
		return fmt.Errorf("provider type cannot be empty")
	}
	if definition.Name == "" {
		return fmt.Errorf("provider name cannot be empty")
	}

	api.logger.Debug().
		Str("type", definition.Type).
		Str("name", definition.Name).
		Msg("Registered provider")
	return nil
}

func (api *PluginAPIImpl) RegisterGatewayMethod(ctx context.Context, definition GatewayMethodDefinition) error {
	api.mu.Lock()
	defer api.mu.Unlock()

	// Check permission
	if err := api.sandbox.RequirePermission(PermissionGatewayRegister); err != nil {
		return err
	}

	// Validate gateway method definition
	if definition.Name == "" {
		return fmt.Errorf("gateway method name cannot be empty")
	}

	api.logger.Debug().Str("method", definition.Name).Msg("Registered gateway method")
	return nil
}

func (api *PluginAPIImpl) GetConfig(ctx context.Context) (map[string]any, error) {
	api.mu.RLock()
	defer api.mu.RUnlock()
	return api.config, nil
}

func (api *PluginAPIImpl) GetPluginID(ctx context.Context) (string, error) {
	return api.pluginID, nil
}

// ToolRegistry manages registered tools
type ToolRegistry struct {
	tools map[string]*RegisteredTool
	mu    sync.RWMutex
}

// RegisteredTool represents a registered tool
type RegisteredTool struct {
	PluginID   string
	Definition ToolDefinition
}

// NewToolRegistry creates a new tool registry
func NewToolRegistry() *ToolRegistry {
	return &ToolRegistry{
		tools: make(map[string]*RegisteredTool),
	}
}

// Register registers a tool
func (r *ToolRegistry) Register(pluginID string, definition ToolDefinition) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.tools[definition.Name]; exists {
		return fmt.Errorf("tool '%s' already registered", definition.Name)
	}

	r.tools[definition.Name] = &RegisteredTool{
		PluginID:   pluginID,
		Definition: definition,
	}

	return nil
}

// Unregister removes a tool
func (r *ToolRegistry) Unregister(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.tools, name)
}

// UnregisterByPlugin removes all tools registered by a plugin
func (r *ToolRegistry) UnregisterByPlugin(pluginID string) []string {
	r.mu.Lock()
	defer r.mu.Unlock()

	var removed []string
	for name, tool := range r.tools {
		if tool.PluginID == pluginID {
			delete(r.tools, name)
			removed = append(removed, name)
		}
	}

	return removed
}

// Get retrieves a tool by name
func (r *ToolRegistry) Get(name string) (*RegisteredTool, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	tool, exists := r.tools[name]
	return tool, exists
}

// GetByPlugin retrieves all tools registered by a plugin
func (r *ToolRegistry) GetByPlugin(pluginID string) []ToolDefinition {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var tools []ToolDefinition
	for _, tool := range r.tools {
		if tool.PluginID == pluginID {
			tools = append(tools, tool.Definition)
		}
	}

	return tools
}

// GetAll retrieves all registered tools
func (r *ToolRegistry) GetAll() []*RegisteredTool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	tools := make([]*RegisteredTool, 0, len(r.tools))
	for _, tool := range r.tools {
		tools = append(tools, tool)
	}

	return tools
}

// HookRegistry manages registered hooks
type HookRegistry struct {
	hooks map[string][]*RegisteredHook
	mu    sync.RWMutex
}

// RegisteredHook represents a registered hook
type RegisteredHook struct {
	PluginID   string
	Definition HookDefinition
}

// NewHookRegistry creates a new hook registry
func NewHookRegistry() *HookRegistry {
	return &HookRegistry{
		hooks: make(map[string][]*RegisteredHook),
	}
}

// Register registers a hook
func (r *HookRegistry) Register(pluginID string, definition HookDefinition) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.hooks[definition.Event] == nil {
		r.hooks[definition.Event] = []*RegisteredHook{}
	}

	r.hooks[definition.Event] = append(r.hooks[definition.Event], &RegisteredHook{
		PluginID:   pluginID,
		Definition: definition,
	})

	return nil
}

// UnregisterByPlugin removes all hooks registered by a plugin
func (r *HookRegistry) UnregisterByPlugin(pluginID string) []string {
	r.mu.Lock()
	defer r.mu.Unlock()

	var removed []string
	for event, hooks := range r.hooks {
		filtered := []*RegisteredHook{}
		for _, hook := range hooks {
			if hook.PluginID != pluginID {
				filtered = append(filtered, hook)
			} else {
				removed = append(removed, event)
			}
		}
		r.hooks[event] = filtered
	}

	return removed
}

// GetHooks retrieves all hooks for an event
func (r *HookRegistry) GetHooks(event string) []*RegisteredHook {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.hooks[event]
}

// InvokeHooks invokes all hooks for an event
func (r *HookRegistry) InvokeHooks(event HookEvent, logger zerolog.Logger) {
	hooks := r.GetHooks(event.Type)

	for _, hook := range hooks {
		// In a real implementation, this would call the plugin's ExecuteHook method
		// For now, we just log
		logger.Debug().
			Str("plugin", hook.PluginID).
			Str("event", event.Type).
			Msg("Would invoke hook")
	}
}
