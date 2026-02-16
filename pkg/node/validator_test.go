package node

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCapabilityValidator_ValidateCapabilities(t *testing.T) {
	validator := NewCapabilityValidator()

	tests := []struct {
		name         string
		capabilities []NodeCapability
		expectError  bool
	}{
		{
			name:         "valid capabilities",
			capabilities: []NodeCapability{CapabilityCameraSnap, CapabilityLocationGet},
			expectError:  false,
		},
		{
			name:         "empty capabilities",
			capabilities: []NodeCapability{},
			expectError:  true,
		},
		{
			name:         "invalid capability",
			capabilities: []NodeCapability{"invalid.capability"},
			expectError:  true,
		},
		{
			name:         "mixed valid and invalid",
			capabilities: []NodeCapability{CapabilityCameraSnap, "invalid.capability"},
			expectError:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validator.ValidateCapabilities(tt.capabilities)
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestCapabilityValidator_GetPermissionForCapability(t *testing.T) {
	validator := NewCapabilityValidator()

	tests := []struct {
		capability         NodeCapability
		expectedPermission PermissionType
		expectError        bool
	}{
		{CapabilityCameraSnap, PermissionCamera, false},
		{CapabilityCameraRecord, PermissionCamera, false},
		{CapabilityLocationGet, PermissionLocation, false},
		{CapabilityLocationStream, PermissionLocation, false},
		{CapabilityNotificationSend, PermissionNotifications, false},
		{CapabilityNotificationList, PermissionNotifications, false},
		{CapabilityAutomationClick, PermissionAutomation, false},
		{CapabilityAutomationInput, PermissionAutomation, false},
		{"unknown.capability", "", true},
	}

	for _, tt := range tests {
		t.Run(string(tt.capability), func(t *testing.T) {
			permission, err := validator.GetPermissionForCapability(tt.capability)
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedPermission, permission)
			}
		})
	}
}

func TestCapabilityValidator_ValidateInvocationParameters(t *testing.T) {
	validator := NewCapabilityValidator()

	tests := []struct {
		name        string
		capability  NodeCapability
		parameters  map[string]interface{}
		expectError bool
		errorMsg    string
	}{
		{
			name:        "camera.snap - valid",
			capability:  CapabilityCameraSnap,
			parameters:  map[string]interface{}{"quality": float64(80)},
			expectError: false,
		},
		{
			name:        "camera.snap - invalid quality type",
			capability:  CapabilityCameraSnap,
			parameters:  map[string]interface{}{"quality": "high"},
			expectError: true,
			errorMsg:    "quality parameter must be a number",
		},
		{
			name:        "notification.send - valid",
			capability:  CapabilityNotificationSend,
			parameters:  map[string]interface{}{"title": "Test", "message": "Hello"},
			expectError: false,
		},
		{
			name:        "notification.send - missing title",
			capability:  CapabilityNotificationSend,
			parameters:  map[string]interface{}{"message": "Hello"},
			expectError: true,
			errorMsg:    "title parameter is required",
		},
		{
			name:        "notification.send - missing message",
			capability:  CapabilityNotificationSend,
			parameters:  map[string]interface{}{"title": "Test"},
			expectError: true,
			errorMsg:    "message parameter is required",
		},
		{
			name:        "automation.click - valid",
			capability:  CapabilityAutomationClick,
			parameters:  map[string]interface{}{"x": float64(100), "y": float64(200)},
			expectError: false,
		},
		{
			name:        "automation.click - missing x",
			capability:  CapabilityAutomationClick,
			parameters:  map[string]interface{}{"y": float64(200)},
			expectError: true,
			errorMsg:    "x parameter is required",
		},
		{
			name:        "automation.input - valid",
			capability:  CapabilityAutomationInput,
			parameters:  map[string]interface{}{"text": "Hello World"},
			expectError: false,
		},
		{
			name:        "automation.input - missing text",
			capability:  CapabilityAutomationInput,
			parameters:  map[string]interface{}{},
			expectError: true,
			errorMsg:    "text parameter is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validator.ValidateInvocationParameters(tt.capability, tt.parameters)
			if tt.expectError {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
