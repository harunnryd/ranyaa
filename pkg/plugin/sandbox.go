package plugin

import (
	"fmt"
)

// SandboxContext enforces permission-based access control for plugins
type SandboxContext struct {
	permissions map[Permission]bool
}

// NewSandboxContext creates a new sandbox context with the given permissions
func NewSandboxContext(permissions []Permission) *SandboxContext {
	permMap := make(map[Permission]bool)
	for _, perm := range permissions {
		permMap[perm] = true
	}
	return &SandboxContext{
		permissions: permMap,
	}
}

// CheckPermission checks if the plugin has the required permission
func (s *SandboxContext) CheckPermission(permission Permission) bool {
	return s.permissions[permission]
}

// RequirePermission throws an error if the plugin lacks the required permission
func (s *SandboxContext) RequirePermission(permission Permission) error {
	if !s.CheckPermission(permission) {
		return fmt.Errorf("permission denied: %s", permission)
	}
	return nil
}

// GetPermissions returns all permissions granted to the plugin
func (s *SandboxContext) GetPermissions() []Permission {
	perms := make([]Permission, 0, len(s.permissions))
	for perm := range s.permissions {
		perms = append(perms, perm)
	}
	return perms
}

// HasAnyPermission checks if the plugin has at least one of the given permissions
func (s *SandboxContext) HasAnyPermission(permissions ...Permission) bool {
	for _, perm := range permissions {
		if s.CheckPermission(perm) {
			return true
		}
	}
	return false
}

// HasAllPermissions checks if the plugin has all of the given permissions
func (s *SandboxContext) HasAllPermissions(permissions ...Permission) bool {
	for _, perm := range permissions {
		if !s.CheckPermission(perm) {
			return false
		}
	}
	return true
}
