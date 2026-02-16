package routing

import (
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"
)

// ConditionEvaluator evaluates routing conditions
type ConditionEvaluator struct {
	conditionCache   *LRUCache
	customConditions map[string]ConditionFunction
	config           *RoutingConfig
	mu               sync.RWMutex
}

// NewConditionEvaluator creates a new condition evaluator
func NewConditionEvaluator(config *RoutingConfig) *ConditionEvaluator {
	return &ConditionEvaluator{
		conditionCache:   NewLRUCache(config.ConditionCacheSize),
		customConditions: make(map[string]ConditionFunction),
		config:           config,
	}
}

// Evaluate evaluates a single condition
func (ce *ConditionEvaluator) Evaluate(condition map[string]interface{}, context *RoutingContext) bool {
	// Check cache
	cacheKey := ce.getCacheKey(condition, context)
	if cached, found := ce.conditionCache.Get(cacheKey); found {
		return cached.(bool)
	}

	// Evaluate based on type
	condType, ok := condition["type"].(string)
	if !ok {
		return false
	}

	var result bool
	switch ConditionType(condType) {
	case ConditionTypeTime:
		result = ce.evaluateTimeCondition(condition, context)
	case ConditionTypeContent:
		result = ce.evaluateContentCondition(condition, context)
	case ConditionTypeMetadata:
		result = ce.evaluateMetadataCondition(condition, context)
	case ConditionTypeCustom:
		result = ce.evaluateCustomCondition(condition, context)
	default:
		result = false
	}

	// Cache result
	ce.conditionCache.Put(cacheKey, result)

	return result
}

// EvaluateAll evaluates all conditions (AND logic)
func (ce *ConditionEvaluator) EvaluateAll(conditions []map[string]interface{}, context *RoutingContext) bool {
	for _, condition := range conditions {
		if !ce.Evaluate(condition, context) {
			return false
		}
	}
	return true
}

// RegisterCustomCondition registers a custom condition function
func (ce *ConditionEvaluator) RegisterCustomCondition(name string, fn ConditionFunction) {
	ce.mu.Lock()
	defer ce.mu.Unlock()
	ce.customConditions[name] = fn
}

// UnregisterCustomCondition unregisters a custom condition function
func (ce *ConditionEvaluator) UnregisterCustomCondition(name string) {
	ce.mu.Lock()
	defer ce.mu.Unlock()
	delete(ce.customConditions, name)
}

// ClearCache clears the condition cache
func (ce *ConditionEvaluator) ClearCache() {
	ce.conditionCache.Clear()
}

// evaluateTimeCondition evaluates a time-based condition
func (ce *ConditionEvaluator) evaluateTimeCondition(condition map[string]interface{}, context *RoutingContext) bool {
	// Extract time range
	timeRange, ok := condition["timeRange"].(map[string]interface{})
	if !ok {
		return false
	}

	start, _ := timeRange["start"].(string)
	end, _ := timeRange["end"].(string)
	timezone, _ := condition["timezone"].(string)

	// Parse timezone
	loc, err := time.LoadLocation(timezone)
	if err != nil {
		loc = time.UTC
	}

	// Get current time in timezone
	now := context.Timestamp.In(loc)

	// Parse time range
	startTime, err := parseTime(start)
	if err != nil {
		return false
	}
	endTime, err := parseTime(end)
	if err != nil {
		return false
	}

	// Check if current time is within range
	currentMinutes := now.Hour()*60 + now.Minute()
	if currentMinutes < startTime || currentMinutes > endTime {
		return false
	}

	// Check day of week if specified
	if daysOfWeek, ok := condition["daysOfWeek"].([]interface{}); ok && len(daysOfWeek) > 0 {
		currentDay := int(now.Weekday())
		dayMatch := false
		for _, day := range daysOfWeek {
			if dayInt, ok := day.(float64); ok && int(dayInt) == currentDay {
				dayMatch = true
				break
			}
		}
		if !dayMatch {
			return false
		}
	}

	return true
}

// evaluateContentCondition evaluates a content-based condition
func (ce *ConditionEvaluator) evaluateContentCondition(condition map[string]interface{}, context *RoutingContext) bool {
	field, _ := condition["field"].(string)
	operator, _ := condition["operator"].(string)
	value, _ := condition["value"].(string)

	// Get content field
	var content string
	if field == "content" {
		content = context.Content
	} else {
		// Try to get from context metadata
		if val, ok := context.Metadata[field]; ok {
			content = fmt.Sprintf("%v", val)
		}
	}

	// Apply operator
	switch operator {
	case "contains":
		return strings.Contains(content, value)
	case "equals":
		return content == value
	case "startsWith":
		return strings.HasPrefix(content, value)
	case "endsWith":
		return strings.HasSuffix(content, value)
	case "matches":
		matched, _ := regexp.MatchString(value, content)
		return matched
	default:
		return false
	}
}

// evaluateMetadataCondition evaluates a metadata-based condition
func (ce *ConditionEvaluator) evaluateMetadataCondition(condition map[string]interface{}, context *RoutingContext) bool {
	field, _ := condition["field"].(string)
	operator, _ := condition["operator"].(string)
	value := condition["value"]

	// Get metadata field
	metadataValue, exists := context.Metadata[field]
	if !exists {
		return false
	}

	// Apply operator
	switch operator {
	case "equals":
		return fmt.Sprintf("%v", metadataValue) == fmt.Sprintf("%v", value)
	case "notEquals":
		return fmt.Sprintf("%v", metadataValue) != fmt.Sprintf("%v", value)
	case "in":
		if arr, ok := value.([]interface{}); ok {
			for _, item := range arr {
				if fmt.Sprintf("%v", metadataValue) == fmt.Sprintf("%v", item) {
					return true
				}
			}
		}
		return false
	case "notIn":
		if arr, ok := value.([]interface{}); ok {
			for _, item := range arr {
				if fmt.Sprintf("%v", metadataValue) == fmt.Sprintf("%v", item) {
					return false
				}
			}
			return true
		}
		return false
	default:
		return false
	}
}

// evaluateCustomCondition evaluates a custom condition
func (ce *ConditionEvaluator) evaluateCustomCondition(condition map[string]interface{}, context *RoutingContext) bool {
	name, _ := condition["name"].(string)
	params, _ := condition["params"].(map[string]interface{})

	ce.mu.RLock()
	fn, exists := ce.customConditions[name]
	ce.mu.RUnlock()

	if !exists {
		return false
	}

	// Execute custom function
	return fn(context, params)
}

// getCacheKey generates a cache key for condition-context pair
func (ce *ConditionEvaluator) getCacheKey(condition map[string]interface{}, context *RoutingContext) string {
	// Simple cache key based on condition type and message ID
	condType, _ := condition["type"].(string)
	return fmt.Sprintf("%s:%s:%d", condType, context.MessageID, context.Timestamp.Unix())
}

// parseTime parses HH:MM format to minutes since midnight
func parseTime(timeStr string) (int, error) {
	parts := strings.Split(timeStr, ":")
	if len(parts) != 2 {
		return 0, fmt.Errorf("invalid time format")
	}

	var hours, minutes int
	fmt.Sscanf(parts[0], "%d", &hours)
	fmt.Sscanf(parts[1], "%d", &minutes)

	if hours < 0 || hours > 23 || minutes < 0 || minutes > 59 {
		return 0, fmt.Errorf("invalid time values")
	}

	return hours*60 + minutes, nil
}
