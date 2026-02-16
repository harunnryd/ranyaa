package routing

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"
)

// PatternMatcher evaluates patterns against message content with caching
type PatternMatcher struct {
	patternCache map[string]*CompiledPattern
	resultCache  *LRUCache
	config       *RoutingConfig
	mu           sync.RWMutex
}

// NewPatternMatcher creates a new pattern matcher
func NewPatternMatcher(config *RoutingConfig) *PatternMatcher {
	return &PatternMatcher{
		patternCache: make(map[string]*CompiledPattern),
		resultCache:  NewLRUCache(config.ResultCacheSize),
		config:       config,
	}
}

// Match evaluates a pattern against a message
func (pm *PatternMatcher) Match(pattern *RoutePattern, message *Message) bool {
	// Check result cache first
	cacheKey := fmt.Sprintf("%s:%s:%s", pattern.Type, pattern.Value, message.Content)
	if cached, found := pm.resultCache.Get(cacheKey); found {
		return cached.(bool)
	}

	// Compile pattern if needed
	compiled, err := pm.compilePattern(pattern)
	if err != nil {
		return false
	}

	// Evaluate pattern
	var result bool
	switch pattern.Type {
	case PatternTypeExact:
		result = pm.matchExact(pattern.Value, message.Content)
	case PatternTypeWildcard:
		result = pm.matchWildcard(compiled, message.Content)
	case PatternTypeRegex:
		result = pm.matchRegex(compiled, message.Content)
	case PatternTypePrefix:
		result = pm.matchPrefix(pattern.Value, message.Content)
	case PatternTypeSuffix:
		result = pm.matchSuffix(pattern.Value, message.Content)
	default:
		result = false
	}

	// Cache result
	pm.resultCache.Put(cacheKey, result)

	return result
}

// MatchMultiple evaluates multiple patterns and returns matching route IDs
func (pm *PatternMatcher) MatchMultiple(patterns map[string]*RoutePattern, message *Message) []string {
	matches := make([]string, 0)
	for routeID, pattern := range patterns {
		if pm.Match(pattern, message) {
			matches = append(matches, routeID)
		}
	}
	return matches
}

// compilePattern compiles a pattern and caches it
func (pm *PatternMatcher) compilePattern(pattern *RoutePattern) (*CompiledPattern, error) {
	pm.mu.RLock()
	if compiled, exists := pm.patternCache[pattern.Value]; exists {
		pm.mu.RUnlock()
		return compiled, nil
	}
	pm.mu.RUnlock()

	// Compile pattern
	compiled := &CompiledPattern{
		Type:     pattern.Type,
		Original: pattern.Value,
	}

	switch pattern.Type {
	case PatternTypeRegex:
		// Validate regex safety
		if err := pm.validateRegexSafety(pattern.Value); err != nil {
			return nil, err
		}
		regex, err := regexp.Compile(pattern.Value)
		if err != nil {
			return nil, &RoutingError{
				Code:    ErrCodeValidation,
				Message: fmt.Sprintf("Invalid regex pattern: %v", err),
			}
		}
		compiled.Regex = regex

	case PatternTypeWildcard:
		// Convert wildcard to regex
		regexPattern := wildcardToRegex(pattern.Value)
		regex, err := regexp.Compile(regexPattern)
		if err != nil {
			return nil, &RoutingError{
				Code:    ErrCodeValidation,
				Message: fmt.Sprintf("Invalid wildcard pattern: %v", err),
			}
		}
		compiled.WildcardRegex = regex
	}

	// Cache compiled pattern
	pm.mu.Lock()
	// Check cache size and evict if needed
	if len(pm.patternCache) >= pm.config.PatternCacheSize {
		pm.evictLRUPattern()
	}
	pm.patternCache[pattern.Value] = compiled
	pm.mu.Unlock()

	return compiled, nil
}

// matchExact performs exact string matching
func (pm *PatternMatcher) matchExact(pattern, content string) bool {
	return pattern == content
}

// matchWildcard performs wildcard matching
func (pm *PatternMatcher) matchWildcard(compiled *CompiledPattern, content string) bool {
	if compiled.WildcardRegex == nil {
		return false
	}
	return compiled.WildcardRegex.MatchString(content)
}

// matchRegex performs regex matching with timeout
func (pm *PatternMatcher) matchRegex(compiled *CompiledPattern, content string) bool {
	if compiled.Regex == nil {
		return false
	}

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(pm.config.RegexTimeout)*time.Millisecond)
	defer cancel()

	// Run regex matching in goroutine
	resultChan := make(chan bool, 1)
	go func() {
		resultChan <- compiled.Regex.MatchString(content)
	}()

	// Wait for result or timeout
	select {
	case result := <-resultChan:
		return result
	case <-ctx.Done():
		// Timeout - skip this route
		return false
	}
}

// matchPrefix performs prefix matching
func (pm *PatternMatcher) matchPrefix(pattern, content string) bool {
	return strings.HasPrefix(content, pattern)
}

// matchSuffix performs suffix matching
func (pm *PatternMatcher) matchSuffix(pattern, content string) bool {
	return strings.HasSuffix(content, pattern)
}

// validateRegexSafety validates regex pattern safety
func (pm *PatternMatcher) validateRegexSafety(pattern string) error {
	// Check pattern length
	if len(pattern) > 1000 {
		return &RoutingError{
			Code:    ErrCodeValidation,
			Message: "Regex pattern exceeds 1000 characters",
		}
	}

	// Check for nested quantifiers (e.g., (a+)+)
	if hasNestedQuantifiers(pattern) {
		return &RoutingError{
			Code:    ErrCodeValidation,
			Message: "Regex pattern contains nested quantifiers",
		}
	}

	// Check alternation branches
	if countAlternations(pattern) > 100 {
		return &RoutingError{
			Code:    ErrCodeValidation,
			Message: "Regex pattern exceeds 100 alternation branches",
		}
	}

	// Check quantifier repetitions
	if hasExcessiveQuantifiers(pattern) {
		return &RoutingError{
			Code:    ErrCodeValidation,
			Message: "Regex pattern exceeds quantifier repetition limits",
		}
	}

	return nil
}

// evictLRUPattern evicts the least recently used pattern from cache
func (pm *PatternMatcher) evictLRUPattern() {
	// Simple eviction: remove first entry
	// In production, track access times for true LRU
	for key := range pm.patternCache {
		delete(pm.patternCache, key)
		break
	}
}

// ClearCache clears all caches
func (pm *PatternMatcher) ClearCache() {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	pm.patternCache = make(map[string]*CompiledPattern)
	pm.resultCache.Clear()
}

// wildcardToRegex converts wildcard pattern to regex
func wildcardToRegex(pattern string) string {
	// Escape special regex characters except * and ?
	escaped := regexp.QuoteMeta(pattern)
	// Replace escaped wildcards with regex equivalents
	escaped = strings.ReplaceAll(escaped, `\*`, ".*")
	escaped = strings.ReplaceAll(escaped, `\?`, ".")
	return "^" + escaped + "$"
}

// hasNestedQuantifiers checks for nested quantifiers
func hasNestedQuantifiers(pattern string) bool {
	// Simple check for patterns like (a+)+ or (a*)*
	nestedPatterns := []string{
		`\([^)]*[+*]\)[+*]`,
		`\([^)]*\{[^}]+\}\)[+*]`,
	}
	for _, np := range nestedPatterns {
		if matched, _ := regexp.MatchString(np, pattern); matched {
			return true
		}
	}
	return false
}

// countAlternations counts alternation branches
func countAlternations(pattern string) int {
	count := 1 // Start with 1 branch
	inGroup := 0
	for i := 0; i < len(pattern); i++ {
		switch pattern[i] {
		case '(':
			inGroup++
		case ')':
			inGroup--
		case '|':
			if inGroup == 0 {
				count++
			}
		case '\\':
			i++ // Skip next character
		}
	}
	return count
}

// hasExcessiveQuantifiers checks for excessive quantifier repetitions
func hasExcessiveQuantifiers(pattern string) bool {
	// Check for {n,m} where n or m > 1000
	quantifierRegex := regexp.MustCompile(`\{(\d+)(?:,(\d+))?\}`)
	matches := quantifierRegex.FindAllStringSubmatch(pattern, -1)
	for _, match := range matches {
		if len(match) > 1 {
			if n := parseInt(match[1]); n > 1000 {
				return true
			}
		}
		if len(match) > 2 && match[2] != "" {
			if m := parseInt(match[2]); m > 1000 {
				return true
			}
		}
	}
	return false
}

// parseInt safely parses an integer
func parseInt(s string) int {
	var n int
	fmt.Sscanf(s, "%d", &n)
	return n
}

// LRUCache is a simple LRU cache implementation
type LRUCache struct {
	capacity int
	cache    map[string]*cacheEntry
	head     *cacheEntry
	tail     *cacheEntry
	mu       sync.RWMutex
}

type cacheEntry struct {
	key   string
	value interface{}
	prev  *cacheEntry
	next  *cacheEntry
}

// NewLRUCache creates a new LRU cache
func NewLRUCache(capacity int) *LRUCache {
	return &LRUCache{
		capacity: capacity,
		cache:    make(map[string]*cacheEntry),
	}
}

// Get retrieves a value from cache
func (c *LRUCache) Get(key string) (interface{}, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if entry, exists := c.cache[key]; exists {
		c.moveToFront(entry)
		return entry.value, true
	}
	return nil, false
}

// Put adds a value to cache
func (c *LRUCache) Put(key string, value interface{}) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if entry, exists := c.cache[key]; exists {
		entry.value = value
		c.moveToFront(entry)
		return
	}

	// Create new entry
	entry := &cacheEntry{key: key, value: value}
	c.cache[key] = entry
	c.addToFront(entry)

	// Evict if over capacity
	if len(c.cache) > c.capacity {
		c.evictLRU()
	}
}

// Clear clears the cache
func (c *LRUCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cache = make(map[string]*cacheEntry)
	c.head = nil
	c.tail = nil
}

// moveToFront moves an entry to the front of the list
func (c *LRUCache) moveToFront(entry *cacheEntry) {
	if entry == c.head {
		return
	}

	// Remove from current position
	if entry.prev != nil {
		entry.prev.next = entry.next
	}
	if entry.next != nil {
		entry.next.prev = entry.prev
	}
	if entry == c.tail {
		c.tail = entry.prev
	}

	// Add to front
	entry.prev = nil
	entry.next = c.head
	if c.head != nil {
		c.head.prev = entry
	}
	c.head = entry
	if c.tail == nil {
		c.tail = entry
	}
}

// addToFront adds a new entry to the front
func (c *LRUCache) addToFront(entry *cacheEntry) {
	entry.next = c.head
	entry.prev = nil
	if c.head != nil {
		c.head.prev = entry
	}
	c.head = entry
	if c.tail == nil {
		c.tail = entry
	}
}

// evictLRU evicts the least recently used entry
func (c *LRUCache) evictLRU() {
	if c.tail == nil {
		return
	}

	delete(c.cache, c.tail.key)
	if c.tail.prev != nil {
		c.tail.prev.next = nil
	}
	c.tail = c.tail.prev
	if c.tail == nil {
		c.head = nil
	}
}
