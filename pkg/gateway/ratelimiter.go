package gateway

import (
	"sync"
	"time"
)

// ClientRateLimiter implements sliding window rate limiting per client
type ClientRateLimiter struct {
	mu                 sync.Mutex
	requestsPerMinute  int
	maxConcurrent      int
	requests           []time.Time
	concurrentRequests int
	windowStart        time.Time
}

// NewClientRateLimiter creates a new rate limiter with default limits
func NewClientRateLimiter() *ClientRateLimiter {
	return &ClientRateLimiter{
		requestsPerMinute:  60,
		maxConcurrent:      10,
		requests:           make([]time.Time, 0),
		concurrentRequests: 0,
		windowStart:        time.Now(),
	}
}

// NewClientRateLimiterWithLimits creates a rate limiter with custom limits
func NewClientRateLimiterWithLimits(requestsPerMinute, maxConcurrent int) *ClientRateLimiter {
	return &ClientRateLimiter{
		requestsPerMinute:  requestsPerMinute,
		maxConcurrent:      maxConcurrent,
		requests:           make([]time.Time, 0),
		concurrentRequests: 0,
		windowStart:        time.Now(),
	}
}

// CheckRequestAllowed checks if a request is allowed under rate limits
func (r *ClientRateLimiter) CheckRequestAllowed() (bool, string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now()

	// Check concurrent requests limit
	if r.concurrentRequests >= r.maxConcurrent {
		return false, "too many concurrent requests"
	}

	// Clean up old requests (older than 1 minute)
	cutoff := now.Add(-time.Minute)
	validRequests := make([]time.Time, 0)
	for _, reqTime := range r.requests {
		if reqTime.After(cutoff) {
			validRequests = append(validRequests, reqTime)
		}
	}
	r.requests = validRequests

	// Check requests per minute limit
	if len(r.requests) >= r.requestsPerMinute {
		return false, "rate limit exceeded"
	}

	return true, ""
}

// RecordRequestStart records the start of a request
func (r *ClientRateLimiter) RecordRequestStart() {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.requests = append(r.requests, time.Now())
	r.concurrentRequests++
}

// RecordRequestEnd records the end of a request
func (r *ClientRateLimiter) RecordRequestEnd() {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.concurrentRequests > 0 {
		r.concurrentRequests--
	}
}

// UpdateLimits updates the rate limits
func (r *ClientRateLimiter) UpdateLimits(requestsPerMinute, maxConcurrent int) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.requestsPerMinute = requestsPerMinute
	r.maxConcurrent = maxConcurrent
}

// GetStats returns current rate limiter statistics
func (r *ClientRateLimiter) GetStats() (requestCount, concurrentCount int) {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Clean up old requests
	now := time.Now()
	cutoff := now.Add(-time.Minute)
	validRequests := make([]time.Time, 0)
	for _, reqTime := range r.requests {
		if reqTime.After(cutoff) {
			validRequests = append(validRequests, reqTime)
		}
	}
	r.requests = validRequests

	return len(r.requests), r.concurrentRequests
}
