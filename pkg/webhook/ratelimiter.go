package webhook

import (
	"sync"
	"time"
)

// RateLimiter implements per-IP rate limiting with sliding window
type RateLimiter struct {
	limits            map[string]*RateLimitState
	maxRequestsPerMin int
	mu                sync.RWMutex
	cleanupInterval   time.Duration
	stopCleanup       chan struct{}
	cleanupRunning    bool
}

// NewRateLimiter creates a new rate limiter
func NewRateLimiter(maxRequestsPerMinute int) *RateLimiter {
	rl := &RateLimiter{
		limits:            make(map[string]*RateLimitState),
		maxRequestsPerMin: maxRequestsPerMinute,
		cleanupInterval:   5 * time.Minute,
		stopCleanup:       make(chan struct{}),
	}

	// Start cleanup goroutine
	go rl.startCleanup()

	return rl
}

// CheckLimit checks if a request from the given IP is allowed
func (rl *RateLimiter) CheckLimit(ip string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now().UnixMilli()

	state, exists := rl.limits[ip]
	if !exists {
		state = &RateLimitState{
			Requests:    make([]int64, 0),
			WindowStart: now,
		}
		rl.limits[ip] = state
	}

	// Remove requests older than 1 minute (sliding window)
	validRequests := make([]int64, 0)
	for _, reqTime := range state.Requests {
		if now-reqTime < 60000 { // 60 seconds in milliseconds
			validRequests = append(validRequests, reqTime)
		}
	}
	state.Requests = validRequests

	// Check if limit exceeded
	if len(state.Requests) >= rl.maxRequestsPerMin {
		return false
	}

	// Add current request
	state.Requests = append(state.Requests, now)
	return true
}

// GetRetryAfter returns the number of seconds until the rate limit resets
func (rl *RateLimiter) GetRetryAfter(ip string) int {
	rl.mu.RLock()
	defer rl.mu.RUnlock()

	state, exists := rl.limits[ip]
	if !exists || len(state.Requests) == 0 {
		return 0
	}

	now := time.Now().UnixMilli()
	oldestRequest := state.Requests[0]

	// Calculate when the oldest request will expire
	retryAfterMs := 60000 - (now - oldestRequest)
	if retryAfterMs < 0 {
		return 0
	}

	// Convert to seconds and round up
	retryAfterSec := (retryAfterMs + 999) / 1000
	return int(retryAfterSec)
}

// startCleanup periodically removes old entries
func (rl *RateLimiter) startCleanup() {
	rl.mu.Lock()
	if rl.cleanupRunning {
		rl.mu.Unlock()
		return
	}
	rl.cleanupRunning = true
	rl.mu.Unlock()

	ticker := time.NewTicker(rl.cleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			rl.cleanup()
		case <-rl.stopCleanup:
			return
		}
	}
}

// cleanup removes old entries from the rate limiter
func (rl *RateLimiter) cleanup() {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now().UnixMilli()

	for ip, state := range rl.limits {
		// Remove requests older than 1 minute
		validRequests := make([]int64, 0)
		for _, reqTime := range state.Requests {
			if now-reqTime < 60000 {
				validRequests = append(validRequests, reqTime)
			}
		}

		// Remove IP if no recent requests
		if len(validRequests) == 0 {
			delete(rl.limits, ip)
		} else {
			state.Requests = validRequests
		}
	}
}

// Stop stops the cleanup goroutine
func (rl *RateLimiter) Stop() {
	close(rl.stopCleanup)
}
