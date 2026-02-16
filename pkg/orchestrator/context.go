package orchestrator

import (
	"fmt"
	"sync"
)

// ContextManager manages context passing between agents
type ContextManager struct {
	contexts map[string]*AgentContext
	mu       sync.RWMutex
	logger   Logger
}

// NewContextManager creates a new ContextManager instance
func NewContextManager(logger Logger) *ContextManager {
	return &ContextManager{
		contexts: make(map[string]*AgentContext),
		logger:   logger,
	}
}

// CreateContext creates a new agent context
func (cm *ContextManager) CreateContext(sessionKey string, ctx AgentContext) error {
	if sessionKey == "" {
		return fmt.Errorf("session key is required")
	}

	cm.mu.Lock()
	defer cm.mu.Unlock()

	// Check if context already exists
	if _, exists := cm.contexts[sessionKey]; exists {
		return fmt.Errorf("context already exists for session: %s", sessionKey)
	}

	// Create a copy to ensure isolation
	contextCopy := cm.copyContext(ctx)
	cm.contexts[sessionKey] = &contextCopy

	if cm.logger != nil {
		cm.logger.Debug("Context created",
			"session_key", sessionKey,
			"parent_session", ctx.ParentSessionKey,
			"trace_id", ctx.TraceID,
			"run_id", ctx.RunID)
	}

	return nil
}

// GetContext retrieves an agent context by session key
func (cm *ContextManager) GetContext(sessionKey string) (AgentContext, error) {
	if sessionKey == "" {
		return AgentContext{}, fmt.Errorf("session key is required")
	}

	cm.mu.RLock()
	defer cm.mu.RUnlock()

	ctx, exists := cm.contexts[sessionKey]
	if !exists {
		return AgentContext{}, fmt.Errorf("context not found for session: %s", sessionKey)
	}

	// Return a copy to ensure isolation
	return cm.copyContext(*ctx), nil
}

// UpdateContext updates an existing agent context
func (cm *ContextManager) UpdateContext(sessionKey string, ctx AgentContext) error {
	if sessionKey == "" {
		return fmt.Errorf("session key is required")
	}

	cm.mu.Lock()
	defer cm.mu.Unlock()

	if _, exists := cm.contexts[sessionKey]; !exists {
		return fmt.Errorf("context not found for session: %s", sessionKey)
	}

	// Create a copy to ensure isolation
	contextCopy := cm.copyContext(ctx)
	cm.contexts[sessionKey] = &contextCopy

	if cm.logger != nil {
		cm.logger.Debug("Context updated",
			"session_key", sessionKey,
			"trace_id", ctx.TraceID,
			"run_id", ctx.RunID)
	}

	return nil
}

// DeleteContext removes an agent context
func (cm *ContextManager) DeleteContext(sessionKey string) error {
	if sessionKey == "" {
		return fmt.Errorf("session key is required")
	}

	cm.mu.Lock()
	defer cm.mu.Unlock()

	if _, exists := cm.contexts[sessionKey]; !exists {
		return fmt.Errorf("context not found for session: %s", sessionKey)
	}

	delete(cm.contexts, sessionKey)

	if cm.logger != nil {
		cm.logger.Debug("Context deleted", "session_key", sessionKey)
	}

	return nil
}

// PassContext creates a child context from a parent context
func (cm *ContextManager) PassContext(parentSessionKey, childSessionKey string, instructions string) (AgentContext, error) {
	if parentSessionKey == "" {
		return AgentContext{}, fmt.Errorf("parent session key is required")
	}

	if childSessionKey == "" {
		return AgentContext{}, fmt.Errorf("child session key is required")
	}

	// Get parent context
	parentCtx, err := cm.GetContext(parentSessionKey)
	if err != nil {
		return AgentContext{}, fmt.Errorf("failed to get parent context: %w", err)
	}

	// Create child context with inherited state
	childCtx := AgentContext{
		ParentSessionKey: parentSessionKey,
		SharedState:      cm.copySharedState(parentCtx.SharedState),
		Files:            cm.copyFiles(parentCtx.Files),
		Instructions:     instructions,
		TraceID:          parentCtx.TraceID,
		RunID:            parentCtx.RunID,
	}

	// Store child context
	if err := cm.CreateContext(childSessionKey, childCtx); err != nil {
		return AgentContext{}, fmt.Errorf("failed to create child context: %w", err)
	}

	if cm.logger != nil {
		cm.logger.Info("Context passed to child",
			"parent_session", parentSessionKey,
			"child_session", childSessionKey,
			"trace_id", childCtx.TraceID)
	}

	return childCtx, nil
}

// MergeSharedState merges shared state from child back to parent
func (cm *ContextManager) MergeSharedState(parentSessionKey, childSessionKey string) error {
	if parentSessionKey == "" {
		return fmt.Errorf("parent session key is required")
	}

	if childSessionKey == "" {
		return fmt.Errorf("child session key is required")
	}

	// Get both contexts
	parentCtx, err := cm.GetContext(parentSessionKey)
	if err != nil {
		return fmt.Errorf("failed to get parent context: %w", err)
	}

	childCtx, err := cm.GetContext(childSessionKey)
	if err != nil {
		return fmt.Errorf("failed to get child context: %w", err)
	}

	// Merge child state into parent
	if parentCtx.SharedState == nil {
		parentCtx.SharedState = make(map[string]interface{})
	}

	for key, value := range childCtx.SharedState {
		parentCtx.SharedState[key] = value
	}

	// Update parent context
	if err := cm.UpdateContext(parentSessionKey, parentCtx); err != nil {
		return fmt.Errorf("failed to update parent context: %w", err)
	}

	if cm.logger != nil {
		cm.logger.Debug("Shared state merged",
			"parent_session", parentSessionKey,
			"child_session", childSessionKey)
	}

	return nil
}

// ListContexts returns all session keys with active contexts
func (cm *ContextManager) ListContexts() []string {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	keys := make([]string, 0, len(cm.contexts))
	for key := range cm.contexts {
		keys = append(keys, key)
	}

	return keys
}

// Clear removes all contexts
func (cm *ContextManager) Clear() {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	cm.contexts = make(map[string]*AgentContext)

	if cm.logger != nil {
		cm.logger.Debug("All contexts cleared")
	}
}

// copyContext creates a deep copy of an AgentContext to ensure isolation
func (cm *ContextManager) copyContext(ctx AgentContext) AgentContext {
	return AgentContext{
		ParentSessionKey: ctx.ParentSessionKey,
		SharedState:      cm.copySharedState(ctx.SharedState),
		Files:            cm.copyFiles(ctx.Files),
		Instructions:     ctx.Instructions,
		TraceID:          ctx.TraceID,
		RunID:            ctx.RunID,
	}
}

// copySharedState creates a deep copy of shared state map
func (cm *ContextManager) copySharedState(state map[string]interface{}) map[string]interface{} {
	if state == nil {
		return nil
	}

	copy := make(map[string]interface{}, len(state))
	for key, value := range state {
		copy[key] = value
	}

	return copy
}

// copyFiles creates a copy of files slice
func (cm *ContextManager) copyFiles(files []string) []string {
	if files == nil {
		return nil
	}

	copy := make([]string, len(files))
	for i, file := range files {
		copy[i] = file
	}

	return copy
}
