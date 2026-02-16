package plugin

import (
	"fmt"
	"sync"
	"time"
)

// PluginRegistry tracks loaded plugins and their state
type PluginRegistry struct {
	plugins map[string]*PluginRecord
	mu      sync.RWMutex
}

// NewPluginRegistry creates a new plugin registry
func NewPluginRegistry() *PluginRegistry {
	return &PluginRegistry{
		plugins: make(map[string]*PluginRecord),
	}
}

// Register registers a plugin
func (r *PluginRegistry) Register(plugin *LoadedPlugin) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.plugins[plugin.ID]; exists {
		return fmt.Errorf("plugin %s already registered", plugin.ID)
	}

	r.plugins[plugin.ID] = &PluginRecord{
		Plugin:                   plugin,
		RegisteredTools:          []string{},
		RegisteredHooks:          []string{},
		RegisteredChannels:       []string{},
		RegisteredProviders:      []string{},
		RegisteredGatewayMethods: []string{},
		LoadedAt:                 time.Now(),
		ErrorCount:               0,
	}

	return nil
}

// Get retrieves a plugin by ID
func (r *PluginRegistry) Get(pluginID string) (*PluginRecord, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	record, exists := r.plugins[pluginID]
	return record, exists
}

// GetAll returns all registered plugins
func (r *PluginRegistry) GetAll() []*PluginRecord {
	r.mu.RLock()
	defer r.mu.RUnlock()

	records := make([]*PluginRecord, 0, len(r.plugins))
	for _, record := range r.plugins {
		records = append(records, record)
	}

	return records
}

// GetByState returns all plugins in a specific state
func (r *PluginRegistry) GetByState(state PluginState) []*PluginRecord {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var records []*PluginRecord
	for _, record := range r.plugins {
		if record.Plugin.State == state {
			records = append(records, record)
		}
	}

	return records
}

// Update updates a plugin record
func (r *PluginRegistry) Update(pluginID string, updater func(*PluginRecord)) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	record, exists := r.plugins[pluginID]
	if !exists {
		return fmt.Errorf("plugin %s not found", pluginID)
	}

	updater(record)
	return nil
}

// UpdateState updates a plugin's state
func (r *PluginRegistry) UpdateState(pluginID string, state PluginState) error {
	return r.Update(pluginID, func(record *PluginRecord) {
		record.Plugin.State = state
	})
}

// Remove removes a plugin from the registry
func (r *PluginRegistry) Remove(pluginID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.plugins[pluginID]; !exists {
		return fmt.Errorf("plugin %s not found", pluginID)
	}

	delete(r.plugins, pluginID)
	return nil
}

// AddRegisteredTool adds a tool to the plugin's registered tools
func (r *PluginRegistry) AddRegisteredTool(pluginID, toolName string) error {
	return r.Update(pluginID, func(record *PluginRecord) {
		record.RegisteredTools = append(record.RegisteredTools, toolName)
	})
}

// AddRegisteredHook adds a hook to the plugin's registered hooks
func (r *PluginRegistry) AddRegisteredHook(pluginID, event string) error {
	return r.Update(pluginID, func(record *PluginRecord) {
		record.RegisteredHooks = append(record.RegisteredHooks, event)
	})
}

// RecordError records an error for a plugin
func (r *PluginRegistry) RecordError(pluginID string, err error) error {
	return r.Update(pluginID, func(record *PluginRecord) {
		record.ErrorCount++
		record.LastError = err
	})
}

// RecordReload records a plugin reload
func (r *PluginRegistry) RecordReload(pluginID string) error {
	return r.Update(pluginID, func(record *PluginRecord) {
		now := time.Now()
		record.LastReloadAt = &now
	})
}
