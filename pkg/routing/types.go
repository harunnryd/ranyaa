package routing

import (
	"regexp"
	"time"
)

// PatternType represents the type of pattern matching
type PatternType string

const (
	PatternTypeExact    PatternType = "exact"
	PatternTypeWildcard PatternType = "wildcard"
	PatternTypeRegex    PatternType = "regex"
	PatternTypePrefix   PatternType = "prefix"
	PatternTypeSuffix   PatternType = "suffix"
)

// ConditionType represents the type of routing condition
type ConditionType string

const (
	ConditionTypeTime     ConditionType = "time"
	ConditionTypeContent  ConditionType = "content"
	ConditionTypeMetadata ConditionType = "metadata"
	ConditionTypeCustom   ConditionType = "custom"
)

// RoutePattern defines a pattern for matching messages
type RoutePattern struct {
	Type     PatternType    `json:"type"`
	Value    string         `json:"value"`
	Compiled *regexp.Regexp `json:"-"` // For regex patterns
}

// RouteCondition defines a condition for route eligibility
type RouteCondition struct {
	Type           string         `json:"type"`
	TimeRange      *TimeRange     `json:"timeRange,omitempty"`
	ContentMatch   *ContentMatch  `json:"contentMatch,omitempty"`
	MetadataMatch  *MetadataMatch `json:"metadataMatch,omitempty"`
	CustomFunction string         `json:"customFunction,omitempty"`
}

// TimeRange represents a time range
type TimeRange struct {
	Start string `json:"start"` // HH:MM format
	End   string `json:"end"`   // HH:MM format
}

// ContentMatch represents content matching criteria
type ContentMatch struct {
	Field   string `json:"field"`
	Pattern string `json:"pattern"`
}

// MetadataMatch represents metadata matching criteria
type MetadataMatch struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// Route represents a routing configuration
type Route struct {
	// Identity
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`

	// Pattern matching
	Patterns []RoutePattern `json:"patterns"`

	// Handler
	Handler string `json:"handler"`

	// Priority
	Priority int `json:"priority"` // 0-100, higher = more important

	// Conditions
	Conditions []RouteCondition `json:"conditions,omitempty"`

	// Fallback
	FallbackRoutes []string `json:"fallbackRoutes,omitempty"`

	// State
	Enabled bool `json:"enabled"`

	// Metadata
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
	CreatedAt int64                  `json:"createdAt"`
	UpdatedAt int64                  `json:"updatedAt"`
}

// Message represents a message to be routed
type Message struct {
	ID        string                 `json:"id"`
	Content   string                 `json:"content"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
	Timestamp int64                  `json:"timestamp"`
}

// RoutingContext contains the complete context for routing decisions
type RoutingContext struct {
	MessageID string                 `json:"messageId"`
	Content   string                 `json:"content"`
	Metadata  map[string]interface{} `json:"metadata"`
	Timestamp time.Time              `json:"timestamp"`
	Source    string                 `json:"source,omitempty"`
}

// RoutingResult represents the result of a routing operation
type RoutingResult struct {
	Success   bool          `json:"success"`
	RouteID   string        `json:"routeId,omitempty"`
	HandlerID string        `json:"handlerId,omitempty"`
	Latency   time.Duration `json:"latency"`
	Error     string        `json:"error,omitempty"`
	Timestamp time.Time     `json:"timestamp"`
}

// RouteStatistics represents statistics for a route
type RouteStatistics struct {
	RouteID       string           `json:"routeId"`
	RouteName     string           `json:"routeName"`
	MatchCount    int64            `json:"matchCount"`
	SuccessCount  int64            `json:"successCount"`
	FailureCount  int64            `json:"failureCount"`
	FallbackCount int64            `json:"fallbackCount"`
	AvgLatency    float64          `json:"avgLatency"`
	MinLatency    int64            `json:"minLatency"`
	MaxLatency    int64            `json:"maxLatency"`
	P50Latency    int64            `json:"p50Latency"`
	P95Latency    int64            `json:"p95Latency"`
	P99Latency    int64            `json:"p99Latency"`
	FirstUsed     int64            `json:"firstUsed"`
	LastUsed      int64            `json:"lastUsed"`
	ErrorTypes    map[string]int64 `json:"errorTypes"`
}

// GlobalStatistics represents aggregated statistics
type GlobalStatistics struct {
	TotalRoutes        int     `json:"totalRoutes"`
	EnabledRoutes      int     `json:"enabledRoutes"`
	TotalMessages      int64   `json:"totalMessages"`
	SuccessfulRoutings int64   `json:"successfulRoutings"`
	FailedRoutings     int64   `json:"failedRoutings"`
	AvgRoutingLatency  float64 `json:"avgRoutingLatency"`
}

// RoutingConfig represents the routing system configuration
type RoutingConfig struct {
	Enabled             bool    `json:"enabled"`
	StorageDir          string  `json:"storageDir"`
	AutoSave            bool    `json:"autoSave"`
	SaveInterval        int     `json:"saveInterval"` // milliseconds
	MaxRoutes           int     `json:"maxRoutes"`
	PatternCacheSize    int     `json:"patternCacheSize"`
	ResultCacheSize     int     `json:"resultCacheSize"`
	ConditionCacheSize  int     `json:"conditionCacheSize"`
	RegexTimeout        int     `json:"regexTimeout"` // milliseconds
	MaxRegexComplexity  int     `json:"maxRegexComplexity"`
	MaxFallbackAttempts int     `json:"maxFallbackAttempts"`
	PriorityAgingFactor float64 `json:"priorityAgingFactor"`
}

// DefaultConfig returns the default routing configuration
func DefaultConfig() *RoutingConfig {
	return &RoutingConfig{
		Enabled:             true,
		StorageDir:          "./data/routing",
		AutoSave:            true,
		SaveInterval:        60000,
		MaxRoutes:           1000,
		PatternCacheSize:    1000,
		ResultCacheSize:     10000,
		ConditionCacheSize:  1000,
		RegexTimeout:        100,
		MaxRegexComplexity:  1000,
		MaxFallbackAttempts: 10,
		PriorityAgingFactor: 0.1,
	}
}

// CompiledPattern represents a compiled pattern for caching
type CompiledPattern struct {
	Type          PatternType
	Original      string
	Regex         *regexp.Regexp
	WildcardRegex *regexp.Regexp
}

// QueueEntry represents an entry in the priority queue
type QueueEntry struct {
	Route      *Route
	EnqueuedAt time.Time
}

// ConditionFunction is a custom condition evaluation function
type ConditionFunction func(context *RoutingContext, params map[string]interface{}) bool

// ValidationResult represents the result of route validation
type ValidationResult struct {
	Valid  bool
	Errors []string
}

// RouteFilter represents filters for listing routes
type RouteFilter struct {
	Enabled  *bool
	Priority *int
	Handler  *string
}

// Error types
type RoutingError struct {
	Code    string
	Message string
	Details interface{}
}

func (e *RoutingError) Error() string {
	return e.Message
}

// Error codes
const (
	ErrCodeValidation   = "VALIDATION_ERROR"
	ErrCodeNotFound     = "NOT_FOUND"
	ErrCodeTimeout      = "TIMEOUT"
	ErrCodeHandlerError = "HANDLER_ERROR"
	ErrCodeStorage      = "STORAGE_ERROR"
	ErrCodeCircular     = "CIRCULAR_FALLBACK"
)

// PatternMatcherConfig holds configuration for PatternMatcher
type PatternMatcherConfig struct {
	PatternCacheSize int
	ResultCacheSize  int
	RegexTimeout     time.Duration
}

// ConditionEvaluatorConfig holds configuration for ConditionEvaluator
type ConditionEvaluatorConfig struct {
	ConditionCacheSize int
}

// PriorityQueueConfig holds configuration for PriorityQueue
type PriorityQueueConfig struct {
	AgingFactor float64
	MaxWaitTime time.Duration
}

// DefaultPatternMatcherConfig returns default configuration
func DefaultPatternMatcherConfig() PatternMatcherConfig {
	return PatternMatcherConfig{
		PatternCacheSize: 1000,
		ResultCacheSize:  10000,
		RegexTimeout:     100 * time.Millisecond,
	}
}

// DefaultConditionEvaluatorConfig returns default configuration
func DefaultConditionEvaluatorConfig() ConditionEvaluatorConfig {
	return ConditionEvaluatorConfig{
		ConditionCacheSize: 1000,
	}
}

// DefaultPriorityQueueConfig returns default configuration
func DefaultPriorityQueueConfig() PriorityQueueConfig {
	return PriorityQueueConfig{
		AgingFactor: 0.1,
		MaxWaitTime: 5 * time.Minute,
	}
}
