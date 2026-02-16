package webhook

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestRateLimiterCheckLimit(t *testing.T) {
	rl := NewRateLimiter(5) // 5 requests per minute
	defer rl.Stop()

	ip := "192.168.1.1"

	// First 5 requests should be allowed
	for i := 0; i < 5; i++ {
		assert.True(t, rl.CheckLimit(ip), "Request %d should be allowed", i+1)
	}

	// 6th request should be denied
	assert.False(t, rl.CheckLimit(ip), "6th request should be denied")
}

func TestRateLimiterMultipleIPs(t *testing.T) {
	rl := NewRateLimiter(3)
	defer rl.Stop()

	ip1 := "192.168.1.1"
	ip2 := "192.168.1.2"

	// Each IP should have independent limits
	for i := 0; i < 3; i++ {
		assert.True(t, rl.CheckLimit(ip1))
		assert.True(t, rl.CheckLimit(ip2))
	}

	// Both IPs should be rate limited independently
	assert.False(t, rl.CheckLimit(ip1))
	assert.False(t, rl.CheckLimit(ip2))
}

func TestRateLimiterGetRetryAfter(t *testing.T) {
	rl := NewRateLimiter(2)
	defer rl.Stop()

	ip := "192.168.1.1"

	// Make requests to hit limit
	rl.CheckLimit(ip)
	rl.CheckLimit(ip)
	rl.CheckLimit(ip) // This should be denied

	// Should have retry-after value
	retryAfter := rl.GetRetryAfter(ip)
	assert.Greater(t, retryAfter, 0)
	assert.LessOrEqual(t, retryAfter, 60)
}

func TestRateLimiterGetRetryAfterNoRequests(t *testing.T) {
	rl := NewRateLimiter(5)
	defer rl.Stop()

	ip := "192.168.1.1"

	// No requests made, should return 0
	retryAfter := rl.GetRetryAfter(ip)
	assert.Equal(t, 0, retryAfter)
}

func TestRateLimiterSlidingWindow(t *testing.T) {
	rl := NewRateLimiter(2)
	defer rl.Stop()

	ip := "192.168.1.1"

	// Make 2 requests
	assert.True(t, rl.CheckLimit(ip))
	time.Sleep(100 * time.Millisecond)
	assert.True(t, rl.CheckLimit(ip))

	// 3rd request should be denied
	assert.False(t, rl.CheckLimit(ip))

	// Manually expire old requests (simulate time passing)
	rl.mu.Lock()
	state := rl.limits[ip]
	// Set first request to 61 seconds ago
	state.Requests[0] = time.Now().UnixMilli() - 61000
	rl.mu.Unlock()

	// Now should be allowed again
	assert.True(t, rl.CheckLimit(ip))
}

func TestRateLimiterCleanup(t *testing.T) {
	rl := NewRateLimiter(5)
	defer rl.Stop()

	ip := "192.168.1.1"

	// Make a request
	rl.CheckLimit(ip)

	// Verify entry exists
	rl.mu.RLock()
	_, exists := rl.limits[ip]
	rl.mu.RUnlock()
	assert.True(t, exists)

	// Manually expire all requests
	rl.mu.Lock()
	state := rl.limits[ip]
	for i := range state.Requests {
		state.Requests[i] = time.Now().UnixMilli() - 61000
	}
	rl.mu.Unlock()

	// Run cleanup
	rl.cleanup()

	// Entry should be removed
	rl.mu.RLock()
	_, exists = rl.limits[ip]
	rl.mu.RUnlock()
	assert.False(t, exists)
}

func TestRateLimiterStop(t *testing.T) {
	rl := NewRateLimiter(5)

	// Stop should not panic
	assert.NotPanics(t, func() {
		rl.Stop()
	})
}
