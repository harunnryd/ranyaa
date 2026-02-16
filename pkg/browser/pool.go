package browser

import (
	"context"
	"sync"
	"time"

	"github.com/go-rod/rod"
)

// PooledPage represents a page in the pool with metadata
type PooledPage struct {
	Page           *rod.Page
	TargetID       string
	CreatedAt      time.Time
	LastUsedAt     time.Time
	OperationCount int
	InUse          bool
}

// PagePool manages a pool of reusable pages
type PagePool struct {
	session     *SessionManager
	pool        map[string]*PooledPage
	maxSize     int
	idleTimeout time.Duration
	mu          sync.RWMutex
	stopChan    chan struct{}
}

// NewPagePool creates a new page pool
func NewPagePool(session *SessionManager, maxSize int, idleTimeout time.Duration) *PagePool {
	pool := &PagePool{
		session:     session,
		pool:        make(map[string]*PooledPage),
		maxSize:     maxSize,
		idleTimeout: idleTimeout,
		stopChan:    make(chan struct{}),
	}

	// Start idle timer
	pool.startIdleTimer()

	return pool
}

// Acquire gets a page from the pool or creates a new one
func (pp *PagePool) Acquire(ctx context.Context, url string) (*rod.Page, string, error) {
	pp.mu.Lock()
	defer pp.mu.Unlock()

	// Try to find an idle page
	for targetID, pooled := range pp.pool {
		if !pooled.InUse {
			// Reuse this page
			pooled.InUse = true
			pooled.LastUsedAt = time.Now()
			pooled.OperationCount++

			// Navigate to URL if provided
			if url != "" {
				if err := pooled.Page.Navigate(url); err != nil {
					return nil, "", &BrowserError{
						Code:    ErrCodeNavigation,
						Message: err.Error(),
					}
				}
				pooled.Page.WaitLoad()
			}

			return pooled.Page, targetID, nil
		}
	}

	// No idle page available, check if we can create a new one
	if len(pp.pool) >= pp.maxSize {
		// Pool is full, evict LRU page
		if err := pp.evictLRU(); err != nil {
			return nil, "", err
		}
	}

	// Create new page
	page, err := pp.session.CreatePage(ctx, url)
	if err != nil {
		return nil, "", err
	}

	targetID := string(page.TargetID)

	// Add to pool
	pooled := &PooledPage{
		Page:           page,
		TargetID:       targetID,
		CreatedAt:      time.Now(),
		LastUsedAt:     time.Now(),
		OperationCount: 1,
		InUse:          true,
	}
	pp.pool[targetID] = pooled

	return page, targetID, nil
}

// Release returns a page to the pool
func (pp *PagePool) Release(targetID string) error {
	pp.mu.Lock()
	defer pp.mu.Unlock()

	pooled, exists := pp.pool[targetID]
	if !exists {
		return &BrowserError{
			Code:    ErrCodeNotFound,
			Message: "Page not found in pool",
		}
	}

	pooled.InUse = false
	pooled.LastUsedAt = time.Now()

	return nil
}

// Evict removes a specific page from the pool
func (pp *PagePool) Evict(targetID string) error {
	pp.mu.Lock()
	defer pp.mu.Unlock()

	pooled, exists := pp.pool[targetID]
	if !exists {
		return &BrowserError{
			Code:    ErrCodeNotFound,
			Message: "Page not found in pool",
		}
	}

	// Close page
	if err := pooled.Page.Close(); err != nil {
		return &BrowserError{
			Code:    ErrCodeBrowserCrash,
			Message: err.Error(),
		}
	}

	delete(pp.pool, targetID)

	return nil
}

// evictLRU removes the least recently used page (must be called with lock held)
func (pp *PagePool) evictLRU() error {
	var lruTargetID string
	var lruTime time.Time

	// Find LRU page that is not in use
	for targetID, pooled := range pp.pool {
		if !pooled.InUse {
			if lruTargetID == "" || pooled.LastUsedAt.Before(lruTime) {
				lruTargetID = targetID
				lruTime = pooled.LastUsedAt
			}
		}
	}

	if lruTargetID == "" {
		return &BrowserError{
			Code:    ErrCodeValidation,
			Message: "No pages available for eviction",
		}
	}

	// Evict the LRU page
	pooled := pp.pool[lruTargetID]
	if err := pooled.Page.Close(); err != nil {
		return &BrowserError{
			Code:    ErrCodeBrowserCrash,
			Message: err.Error(),
		}
	}

	delete(pp.pool, lruTargetID)

	return nil
}

// EvictIdlePages removes pages that have been idle beyond the timeout
func (pp *PagePool) EvictIdlePages() int {
	pp.mu.Lock()
	defer pp.mu.Unlock()

	now := time.Now()
	evicted := 0

	for targetID, pooled := range pp.pool {
		if !pooled.InUse && now.Sub(pooled.LastUsedAt) > pp.idleTimeout {
			if err := pooled.Page.Close(); err == nil {
				delete(pp.pool, targetID)
				evicted++
			}
		}
	}

	return evicted
}

// startIdleTimer periodically checks for idle pages
func (pp *PagePool) startIdleTimer() {
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				pp.EvictIdlePages()
			case <-pp.stopChan:
				return
			}
		}
	}()
}

// Clear closes all pages and empties the pool
func (pp *PagePool) Clear() error {
	pp.mu.Lock()
	defer pp.mu.Unlock()

	for targetID, pooled := range pp.pool {
		pooled.Page.Close()
		delete(pp.pool, targetID)
	}

	return nil
}

// GetStats returns pool metrics
func (pp *PagePool) GetStats() map[string]interface{} {
	pp.mu.RLock()
	defer pp.mu.RUnlock()

	totalOps := 0
	inUse := 0

	for _, pooled := range pp.pool {
		totalOps += pooled.OperationCount
		if pooled.InUse {
			inUse++
		}
	}

	return map[string]interface{}{
		"totalPages":  len(pp.pool),
		"inUse":       inUse,
		"idle":        len(pp.pool) - inUse,
		"maxSize":     pp.maxSize,
		"totalOps":    totalOps,
		"idleTimeout": pp.idleTimeout.Seconds(),
	}
}

// GetPage retrieves a pooled page by target ID
func (pp *PagePool) GetPage(targetID string) (*PooledPage, error) {
	pp.mu.RLock()
	defer pp.mu.RUnlock()

	pooled, exists := pp.pool[targetID]
	if !exists {
		return nil, &BrowserError{
			Code:    ErrCodeNotFound,
			Message: "Page not found in pool",
		}
	}

	return pooled, nil
}

// IncrementOperationCount increments the operation count for a page
func (pp *PagePool) IncrementOperationCount(targetID string) {
	pp.mu.Lock()
	defer pp.mu.Unlock()

	if pooled, exists := pp.pool[targetID]; exists {
		pooled.OperationCount++
		pooled.LastUsedAt = time.Now()
	}
}

// Close stops the pool and closes all pages
func (pp *PagePool) Close() error {
	close(pp.stopChan)
	return pp.Clear()
}

// Size returns the current number of pages in the pool
func (pp *PagePool) Size() int {
	pp.mu.RLock()
	defer pp.mu.RUnlock()
	return len(pp.pool)
}

// InUseCount returns the number of pages currently in use
func (pp *PagePool) InUseCount() int {
	pp.mu.RLock()
	defer pp.mu.RUnlock()

	count := 0
	for _, pooled := range pp.pool {
		if pooled.InUse {
			count++
		}
	}
	return count
}

// IdleCount returns the number of idle pages
func (pp *PagePool) IdleCount() int {
	pp.mu.RLock()
	defer pp.mu.RUnlock()

	count := 0
	for _, pooled := range pp.pool {
		if !pooled.InUse {
			count++
		}
	}
	return count
}
