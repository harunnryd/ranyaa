package plugin

import (
	"time"
)

// PluginState represents the current state of a plugin
type PluginState string

const (
	StateLoading  PluginState = "loading"
	StateEnabled  PluginState = "enabled"
	StateDisabled PluginState = "disabled"
	StateFailed   PluginState = "failed"
	StateUnloaded PluginState = "unloaded"
)

// Permission represents a capability that a plugin can request
type Permission string

const (
	PermissionFilesystemRead   Permission = "filesystem:read"
	PermissionFilesystemWrite  Permission = "filesystem:write"
	PermissionNetworkHTTP      Permission = "network:http"
	PermissionNetworkWebSocket Permission = "network:websocket"
	PermissionProcessSpawn     Permission = "process:spawn"
	PermissionDatabaseRead     Permission = "database:read"
	PermissionDatabaseWrite    Permission = "database:write"
	PermissionGatewayRegister  Permission = "gateway:register"
)

// ValidPermissions is a set of all valid permissions
var ValidPermissions = map[Permission]bool{
	PermissionFilesystemRead:   true,
	PermissionFilesystemWrite:  true,
	PermissionNetworkHTTP:      true,
	PermissionNetworkWebSocket: true,
	PermissionProcessSpawn:     true,
	PermissionDatabaseRead:     true,
	PermissionDatabaseWrite:    true,
	PermissionGatewayRegister:  true,
}

// PluginManifest represents the plugin.json file structure
type PluginManifest struct {
	ID           string             `json:"id"`
	Name         string             `json:"name"`
	Version      string             `json:"version"`
	Description  string             `json:"description,omitempty"`
	Author       string             `json:"author,omitempty"`
	Main         string             `json:"main"`
	Dependencies []PluginDependency `json:"dependencies,omitempty"`
	Permissions  []Permission       `json:"permissions,omitempty"`
	Config       map[string]any     `json:"config,omitempty"`
	Exports      *PluginExports     `json:"exports,omitempty"`
}

// PluginDependency represents a dependency on another plugin
type PluginDependency struct {
	PluginID string `json:"pluginId"`
	Version  string `json:"version,omitempty"` // Semver constraint
}

// PluginExports declares what the plugin exports
type PluginExports struct {
	Tools          []string `json:"tools,omitempty"`
	Hooks          []string `json:"hooks,omitempty"`
	Channels       []string `json:"channels,omitempty"`
	Providers      []string `json:"providers,omitempty"`
	GatewayMethods []string `json:"gatewayMethods,omitempty"`
}

// DiscoveredPlugin represents a plugin found during discovery
type DiscoveredPlugin struct {
	ID           string
	Path         string
	Source       PluginSource
	ManifestPath string
}

// PluginSource indicates where a plugin was discovered
type PluginSource string

const (
	SourceBuiltin   PluginSource = "builtin"
	SourceWorkspace PluginSource = "workspace"
	SourceExtra     PluginSource = "extra"
)

// LoadedPlugin represents a fully loaded plugin
type LoadedPlugin struct {
	ID       string
	Manifest PluginManifest
	State    PluginState
	Sandbox  *SandboxContext
	Client   PluginClient // go-plugin client
	Config   map[string]any
}

// PluginRecord tracks a plugin and its registered extensions
type PluginRecord struct {
	Plugin                   *LoadedPlugin
	RegisteredTools          []string
	RegisteredHooks          []string
	RegisteredChannels       []string
	RegisteredProviders      []string
	RegisteredGatewayMethods []string
	LoadedAt                 time.Time
	LastReloadAt             *time.Time
	ErrorCount               int
	LastError                error
}

// ToolDefinition represents a tool that can be registered
type ToolDefinition struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"` // JSON Schema
}

// HookDefinition represents a hook registration
type HookDefinition struct {
	Event string `json:"event"`
}

// HookEvent represents an event passed to hooks
type HookEvent struct {
	Type      string         `json:"type"`
	Payload   map[string]any `json:"payload"`
	Timestamp int64          `json:"timestamp"`
}

// ChannelDefinition represents a messaging channel
type ChannelDefinition struct {
	Protocol string `json:"protocol"`
	Name     string `json:"name"`
}

// ProviderDefinition represents a service provider
type ProviderDefinition struct {
	Type string `json:"type"`
	Name string `json:"name"`
}

// GatewayMethodDefinition represents a gateway RPC method
type GatewayMethodDefinition struct {
	Name string `json:"name"`
}

// LoadResult contains the results of loading plugins
type LoadResult struct {
	Loaded  []string         // Successfully loaded plugin IDs
	Failed  []string         // Failed plugin IDs
	Skipped []string         // Skipped plugin IDs (dependency issues)
	Errors  map[string]error // Errors by plugin ID
}

// PluginInfo contains plugin metadata with tool definitions
type PluginInfo struct {
	ID         string
	Manifest   PluginManifest
	State      PluginState
	Tools      []ToolDefinition
	LoadedAt   time.Time
	LastReload *time.Time
	ErrorCount int
	LastError  error
}

// PluginDiscoveryConfig configures plugin discovery
type PluginDiscoveryConfig struct {
	BuiltinDir   string
	WorkspaceDir string
	ExtraDirs    []string
}

// PluginRuntimeConfig configures the plugin runtime
type PluginRuntimeConfig struct {
	BuiltinDir    string
	WorkspaceDir  string
	ExtraDirs     []string
	PluginConfigs map[string]map[string]any
}

// DependencyGraph represents plugin dependencies
type DependencyGraph struct {
	Nodes map[string]*PluginManifest
	Edges map[string][]string // pluginId -> dependencies
}
