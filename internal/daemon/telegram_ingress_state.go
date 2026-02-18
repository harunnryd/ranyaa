package daemon

import (
	"sync"
	"time"
)

type messageDedupeCache struct {
	ttl time.Duration

	mu      sync.RWMutex
	entries map[string]time.Time

	startOnce sync.Once
	stopOnce  sync.Once
	stopCh    chan struct{}
}

func newMessageDedupeCache(ttl time.Duration) *messageDedupeCache {
	if ttl <= 0 {
		ttl = 5 * time.Minute
	}
	return &messageDedupeCache{
		ttl:     ttl,
		entries: make(map[string]time.Time),
		stopCh:  make(chan struct{}),
	}
}

func (c *messageDedupeCache) IsDuplicate(key string) bool {
	c.mu.RLock()
	ts, exists := c.entries[key]
	c.mu.RUnlock()
	if !exists {
		return false
	}

	if time.Since(ts) > c.ttl {
		c.mu.Lock()
		delete(c.entries, key)
		c.mu.Unlock()
		return false
	}

	return true
}

func (c *messageDedupeCache) Mark(key string) {
	c.mu.Lock()
	c.entries[key] = time.Now()
	c.mu.Unlock()
}

func (c *messageDedupeCache) Start() {
	if c == nil {
		return
	}

	c.startOnce.Do(func() {
		interval := c.ttl / 2
		if interval <= 0 {
			interval = time.Minute
		}
		if interval > 30*time.Second {
			interval = 30 * time.Second
		}

		ticker := time.NewTicker(interval)
		go func() {
			defer ticker.Stop()
			for {
				select {
				case <-ticker.C:
					c.cleanupExpired()
				case <-c.stopCh:
					return
				}
			}
		}()
	})
}

func (c *messageDedupeCache) Stop() {
	if c == nil {
		return
	}
	c.stopOnce.Do(func() {
		close(c.stopCh)
	})
}

func (c *messageDedupeCache) cleanupExpired() {
	now := time.Now()

	c.mu.Lock()
	for key, ts := range c.entries {
		if now.Sub(ts) > c.ttl {
			delete(c.entries, key)
		}
	}
	c.mu.Unlock()
}
