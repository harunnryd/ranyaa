package commandqueue

import (
	"context"
	"sync"
	"time"
)

// dedupEntry stores cached result for idempotency
type dedupEntry struct {
	result    taskResult
	timestamp time.Time
}

// dedupCache provides time-bounded LRU cache for request idempotency
type dedupCache struct {
	entries map[string]*dedupEntry
	ttl     time.Duration
	mu      sync.RWMutex
	ctx     context.Context
	cancel  context.CancelFunc
}

// newDedupCache creates a new deduplication cache
func newDedupCache(ctx context.Context, ttl time.Duration) *dedupCache {
	if ttl <= 0 {
		ttl = 5 * time.Minute
	}

	ctx, cancel := context.WithCancel(ctx)
	cache := &dedupCache{
		entries: make(map[string]*dedupEntry),
		ttl:     ttl,
		ctx:     ctx,
		cancel:  cancel,
	}

	// Start cleanup goroutine
	go cache.cleanup()

	return cache
}

func (dc *dedupCache) Stop() {
	if dc.cancel != nil {
		dc.cancel()
	}
}

// Get retrieves a cached result if it exists and is not expired
func (dc *dedupCache) Get(requestID string) (taskResult, bool) {
	dc.mu.RLock()
	defer dc.mu.RUnlock()

	entry, exists := dc.entries[requestID]
	if !exists {
		return taskResult{}, false
	}

	// Check if expired
	if time.Since(entry.timestamp) > dc.ttl {
		return taskResult{}, false
	}

	return entry.result, true
}

// Set stores a result in the cache
func (dc *dedupCache) Set(requestID string, result taskResult) {
	dc.mu.Lock()
	defer dc.mu.Unlock()

	dc.entries[requestID] = &dedupEntry{
		result:    result,
		timestamp: time.Now(),
	}
}

// cleanup periodically removes expired entries
func (dc *dedupCache) cleanup() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-dc.ctx.Done():
			return
		case <-ticker.C:
			dc.mu.Lock()
			now := time.Now()
			for requestID, entry := range dc.entries {
				if now.Sub(entry.timestamp) > dc.ttl {
					delete(dc.entries, requestID)
				}
			}
			dc.mu.Unlock()
		}
	}
}

// Size returns the number of entries in the cache
func (dc *dedupCache) Size() int {
	dc.mu.RLock()
	defer dc.mu.RUnlock()
	return len(dc.entries)
}

// Clear removes all entries from the cache
func (dc *dedupCache) Clear() {
	dc.mu.Lock()
	defer dc.mu.Unlock()
	dc.entries = make(map[string]*dedupEntry)
}
