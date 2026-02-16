package node

import (
	"time"
)

// NodePlatform represents the platform type of a node
type NodePlatform string

const (
	PlatformMacOS   NodePlatform = "macos"
	PlatformWindows NodePlatform = "windows"
	PlatformLinux   NodePlatform = "linux"
	PlatformAndroid NodePlatform = "android"
	PlatformIOS     NodePlatform = "ios"
	PlatformWeb     NodePlatform = "web"
)

// NodeCapability represents a capability that a node can provide
type NodeCapability string

const (
	// Camera capabilities
	CapabilityCameraSnap   NodeCapability = "camera.snap"
	CapabilityCameraRecord NodeCapability = "camera.record"

	// Notification capabilities
	CapabilityNotificationSend NodeCapability = "notification.send"
	CapabilityNotificationList NodeCapability = "notification.list"

	// Location capabilities
	CapabilityLocationGet    NodeCapability = "location.get"
	CapabilityLocationStream NodeCapability = "location.stream"

	// Automation capabilities
	CapabilityAutomationClick NodeCapability = "automation.click"
	CapabilityAutomationInput NodeCapability = "automation.input"
)

// PermissionType represents a permission category
type PermissionType string

const (
	PermissionCamera        PermissionType = "camera"
	PermissionLocation      PermissionType = "location"
	PermissionNotifications PermissionType = "notifications"
	PermissionAutomation    PermissionType = "automation"
)

// NodeErrorCode represents error codes for node operations
type NodeErrorCode string

const (
	ErrNodeNotFound       NodeErrorCode = "NODE_NOT_FOUND"
	ErrNodeOffline        NodeErrorCode = "NODE_OFFLINE"
	ErrNodeDegraded       NodeErrorCode = "NODE_DEGRADED"
	ErrCapabilityNotFound NodeErrorCode = "CAPABILITY_NOT_FOUND"
	ErrPermissionDenied   NodeErrorCode = "PERMISSION_DENIED"
	ErrInvocationTimeout  NodeErrorCode = "INVOCATION_TIMEOUT"
	ErrInvocationFailed   NodeErrorCode = "INVOCATION_FAILED"
	ErrInvalidParameters  NodeErrorCode = "INVALID_PARAMETERS"
	ErrInvalidCapability  NodeErrorCode = "INVALID_CAPABILITY"
	ErrDuplicateNode      NodeErrorCode = "DUPLICATE_NODE"
	ErrHeartbeatMissed    NodeErrorCode = "HEARTBEAT_MISSED"
	ErrDefaultNodeNotSet  NodeErrorCode = "DEFAULT_NODE_NOT_SET"
	ErrCircularFallback   NodeErrorCode = "CIRCULAR_FALLBACK"
)

// Node represents a registered node in the system
type Node struct {
	ID               string                  `json:"id"`
	Name             string                  `json:"name"`
	Platform         NodePlatform            `json:"platform"`
	Capabilities     []NodeCapability        `json:"capabilities"`
	Permissions      map[PermissionType]bool `json:"permissions"`
	LastSeen         time.Time               `json:"lastSeen"`
	Online           bool                    `json:"online"`
	Metadata         map[string]interface{}  `json:"metadata"`
	CreatedAt        time.Time               `json:"createdAt"`
	MissedHeartbeats int                     `json:"missedHeartbeats"`
	TimeoutCount     int                     `json:"timeoutCount"`
	Degraded         bool                    `json:"degraded"`
}

// InvocationRequest represents a request to invoke a node capability
type InvocationRequest struct {
	InvocationID string                 `json:"invocationId"`
	NodeID       string                 `json:"nodeId"`
	Capability   NodeCapability         `json:"capability"`
	Parameters   map[string]interface{} `json:"parameters"`
	Timeout      int                    `json:"timeout"` // milliseconds
	Timestamp    time.Time              `json:"timestamp"`
}

// InvocationResponse represents a response from a node capability invocation
type InvocationResponse struct {
	InvocationID string                 `json:"invocationId"`
	NodeID       string                 `json:"nodeId"`
	Success      bool                   `json:"success"`
	Result       map[string]interface{} `json:"result,omitempty"`
	Error        *InvocationError       `json:"error,omitempty"`
	Timestamp    time.Time              `json:"timestamp"`
}

// InvocationError represents an error from a node capability invocation
type InvocationError struct {
	Code    NodeErrorCode `json:"code"`
	Message string        `json:"message"`
}

// NodeEvent represents an event emitted by the node system
type NodeEvent struct {
	Type      string                 `json:"type"`
	NodeID    string                 `json:"nodeId"`
	Timestamp time.Time              `json:"timestamp"`
	Data      map[string]interface{} `json:"data,omitempty"`
}

// NodeFilter represents filtering criteria for listing nodes
type NodeFilter struct {
	Platform   *NodePlatform   `json:"platform,omitempty"`
	Online     *bool           `json:"online,omitempty"`
	Capability *NodeCapability `json:"capability,omitempty"`
	Degraded   *bool           `json:"degraded,omitempty"`
}

// NodeStatistics represents statistics for a node
type NodeStatistics struct {
	NodeID                string        `json:"nodeId"`
	TotalInvocations      int64         `json:"totalInvocations"`
	SuccessfulInvocations int64         `json:"successfulInvocations"`
	FailedInvocations     int64         `json:"failedInvocations"`
	TimeoutInvocations    int64         `json:"timeoutInvocations"`
	AverageLatency        float64       `json:"averageLatency"` // milliseconds
	LastInvocation        time.Time     `json:"lastInvocation"`
	Uptime                time.Duration `json:"uptime"`
}

// NodeConfig represents configuration for the node system
type NodeConfig struct {
	HeartbeatInterval   time.Duration `json:"heartbeatInterval"`
	HeartbeatTimeout    time.Duration `json:"heartbeatTimeout"`
	MaxMissedHeartbeats int           `json:"maxMissedHeartbeats"`
	InvocationTimeout   time.Duration `json:"invocationTimeout"`
	DegradedThreshold   int           `json:"degradedThreshold"`
	StoragePath         string        `json:"storagePath"`
	AutoSaveInterval    time.Duration `json:"autoSaveInterval"`
}

// DefaultNodeConfig returns default configuration
func DefaultNodeConfig() NodeConfig {
	return NodeConfig{
		HeartbeatInterval:   30 * time.Second,
		HeartbeatTimeout:    90 * time.Second,
		MaxMissedHeartbeats: 3,
		InvocationTimeout:   30 * time.Second,
		DegradedThreshold:   5,
		StoragePath:         ".kiro/data/nodes.json",
		AutoSaveInterval:    5 * time.Minute,
	}
}

// InvocationTracker tracks active invocations
type invocationTracker struct {
	InvocationID string
	NodeID       string
	Capability   NodeCapability
	StartTime    time.Time
	ResponseChan chan *InvocationResponse
}

// EventHandler is a function that handles node events
type EventHandler func(event NodeEvent)
