package node

import (
	"fmt"
	"strings"
)

// CapabilityValidator validates node capabilities
type CapabilityValidator struct {
	validCapabilities map[NodeCapability]bool
}

// NewCapabilityValidator creates a new CapabilityValidator
func NewCapabilityValidator() *CapabilityValidator {
	return &CapabilityValidator{
		validCapabilities: map[NodeCapability]bool{
			// Camera capabilities
			CapabilityCameraSnap:   true,
			CapabilityCameraRecord: true,
			// Notification capabilities
			CapabilityNotificationSend: true,
			CapabilityNotificationList: true,
			// Location capabilities
			CapabilityLocationGet:    true,
			CapabilityLocationStream: true,
			// Automation capabilities
			CapabilityAutomationClick: true,
			CapabilityAutomationInput: true,
		},
	}
}

// ValidateCapabilities validates a list of capabilities
func (cv *CapabilityValidator) ValidateCapabilities(capabilities []NodeCapability) error {
	if len(capabilities) == 0 {
		return fmt.Errorf("at least one capability is required")
	}

	for _, cap := range capabilities {
		if !cv.IsValidCapability(cap) {
			return fmt.Errorf("invalid capability: %s", cap)
		}
	}

	return nil
}

// IsValidCapability checks if a capability is valid
func (cv *CapabilityValidator) IsValidCapability(capability NodeCapability) bool {
	return cv.validCapabilities[capability]
}

// GetPermissionForCapability returns the permission type required for a capability
func (cv *CapabilityValidator) GetPermissionForCapability(capability NodeCapability) (PermissionType, error) {
	capStr := string(capability)

	if strings.HasPrefix(capStr, "camera.") {
		return PermissionCamera, nil
	}
	if strings.HasPrefix(capStr, "notification.") {
		return PermissionNotifications, nil
	}
	if strings.HasPrefix(capStr, "location.") {
		return PermissionLocation, nil
	}
	if strings.HasPrefix(capStr, "automation.") {
		return PermissionAutomation, nil
	}

	return "", fmt.Errorf("unknown capability: %s", capability)
}

// ValidateInvocationParameters validates parameters for a capability invocation
func (cv *CapabilityValidator) ValidateInvocationParameters(capability NodeCapability, parameters map[string]interface{}) error {
	// Basic validation based on capability type
	switch capability {
	case CapabilityCameraSnap:
		// Optional: quality parameter
		if quality, ok := parameters["quality"]; ok {
			if _, ok := quality.(float64); !ok {
				return fmt.Errorf("quality parameter must be a number")
			}
		}

	case CapabilityCameraRecord:
		// Optional: duration parameter
		if duration, ok := parameters["duration"]; ok {
			if _, ok := duration.(float64); !ok {
				return fmt.Errorf("duration parameter must be a number")
			}
		}

	case CapabilityNotificationSend:
		// Required: title and message
		if _, ok := parameters["title"]; !ok {
			return fmt.Errorf("title parameter is required")
		}
		if _, ok := parameters["message"]; !ok {
			return fmt.Errorf("message parameter is required")
		}

	case CapabilityLocationGet:
		// Optional: accuracy parameter
		if accuracy, ok := parameters["accuracy"]; ok {
			if _, ok := accuracy.(string); !ok {
				return fmt.Errorf("accuracy parameter must be a string")
			}
		}

	case CapabilityLocationStream:
		// Optional: interval parameter
		if interval, ok := parameters["interval"]; ok {
			if _, ok := interval.(float64); !ok {
				return fmt.Errorf("interval parameter must be a number")
			}
		}

	case CapabilityAutomationClick:
		// Required: x and y coordinates
		if _, ok := parameters["x"]; !ok {
			return fmt.Errorf("x parameter is required")
		}
		if _, ok := parameters["y"]; !ok {
			return fmt.Errorf("y parameter is required")
		}

	case CapabilityAutomationInput:
		// Required: text parameter
		if _, ok := parameters["text"]; !ok {
			return fmt.Errorf("text parameter is required")
		}
	}

	return nil
}
