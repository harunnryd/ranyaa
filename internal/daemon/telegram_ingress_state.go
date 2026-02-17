package daemon

import (
	"strconv"
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

type telegramPairingStore struct {
	mu        sync.RWMutex
	allowlist map[string]bool
	paired    map[string]bool
}

func newTelegramPairingStore(allowlist []int64) *telegramPairingStore {
	store := &telegramPairingStore{
		allowlist: make(map[string]bool, len(allowlist)),
		paired:    make(map[string]bool),
	}
	for _, chatID := range allowlist {
		store.allowlist[strconv.FormatInt(chatID, 10)] = true
	}
	return store
}

func (s *telegramPairingStore) IsAllowlisted(peerID string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.allowlist[peerID]
}

func (s *telegramPairingStore) IsPaired(peerID string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.paired[peerID]
}

func (s *telegramPairingStore) IsAllowed(peerID string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.allowlist[peerID] || s.paired[peerID]
}

func (s *telegramPairingStore) Pair(peerID string) {
	s.mu.Lock()
	s.paired[peerID] = true
	s.mu.Unlock()
}
