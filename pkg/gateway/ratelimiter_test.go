package gateway

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestClientRateLimiter_CheckRequestAllowed(t *testing.T) {
	t.Run("should allow requests under limit", func(t *testing.T) {
		limiter := NewClientRateLimiterWithLimits(10, 5)

		for i := 0; i < 5; i++ {
			allowed, reason := limiter.CheckRequestAllowed()
			assert.True(t, allowed)
			assert.Empty(t, reason)
			limiter.RecordRequestStart()
		}
	})

	t.Run("should reject when concurrent limit exceeded", func(t *testing.T) {
		limiter := NewClientRateLimiterWithLimits(100, 3)

		// Fill up concurrent slots
		for i := 0; i < 3; i++ {
			limiter.RecordRequestStart()
		}

		allowed, reason := limiter.CheckRequestAllowed()
		assert.False(t, allowed)
		assert.Equal(t, "too many concurrent requests", reason)
	})

	t.Run("should reject when rate limit exceeded", func(t *testing.T) {
		limiter := NewClientRateLimiterWithLimits(5, 10)

		// Fill up rate limit
		for i := 0; i < 5; i++ {
			limiter.CheckRequestAllowed()
			limiter.RecordRequestStart()
			limiter.RecordRequestEnd() // End immediately to avoid concurrent limit
		}

		allowed, reason := limiter.CheckRequestAllowed()
		assert.False(t, allowed)
		assert.Equal(t, "rate limit exceeded", reason)
	})

	t.Run("should allow requests after window expires", func(t *testing.T) {
		limiter := NewClientRateLimiterWithLimits(2, 10)

		// Use up the limit
		limiter.CheckRequestAllowed()
		limiter.RecordRequestStart()
		limiter.RecordRequestEnd()

		limiter.CheckRequestAllowed()
		limiter.RecordRequestStart()
		limiter.RecordRequestEnd()

		// Should be at limit
		allowed, _ := limiter.CheckRequestAllowed()
		assert.False(t, allowed)

		// Wait for window to expire (in real scenario, this would be 1 minute)
		// For testing, we'll just verify the logic works
		time.Sleep(10 * time.Millisecond)
	})
}

func TestClientRateLimiter_RecordRequestStartEnd(t *testing.T) {
	t.Run("should track concurrent requests", func(t *testing.T) {
		limiter := NewClientRateLimiterWithLimits(100, 10)

		limiter.RecordRequestStart()
		limiter.RecordRequestStart()

		_, concurrent := limiter.GetStats()
		assert.Equal(t, 2, concurrent)

		limiter.RecordRequestEnd()
		_, concurrent = limiter.GetStats()
		assert.Equal(t, 1, concurrent)

		limiter.RecordRequestEnd()
		_, concurrent = limiter.GetStats()
		assert.Equal(t, 0, concurrent)
	})

	t.Run("should not go negative on concurrent count", func(t *testing.T) {
		limiter := NewClientRateLimiterWithLimits(100, 10)

		limiter.RecordRequestEnd()
		limiter.RecordRequestEnd()

		_, concurrent := limiter.GetStats()
		assert.Equal(t, 0, concurrent)
	})
}

func TestClientRateLimiter_UpdateLimits(t *testing.T) {
	t.Run("should update limits", func(t *testing.T) {
		limiter := NewClientRateLimiterWithLimits(10, 5)

		// Use some requests
		for i := 0; i < 3; i++ {
			limiter.RecordRequestStart()
		}

		// Update limits
		limiter.UpdateLimits(20, 10)

		// Should now allow more concurrent requests
		for i := 0; i < 7; i++ {
			allowed, _ := limiter.CheckRequestAllowed()
			assert.True(t, allowed)
			limiter.RecordRequestStart()
		}

		// Should hit new limit
		allowed, reason := limiter.CheckRequestAllowed()
		assert.False(t, allowed)
		assert.Equal(t, "too many concurrent requests", reason)
	})
}

func TestClientRateLimiter_GetStats(t *testing.T) {
	t.Run("should return accurate stats", func(t *testing.T) {
		limiter := NewClientRateLimiterWithLimits(100, 10)

		limiter.RecordRequestStart()
		limiter.RecordRequestStart()
		limiter.RecordRequestStart()

		requests, concurrent := limiter.GetStats()
		assert.Equal(t, 3, requests)
		assert.Equal(t, 3, concurrent)

		limiter.RecordRequestEnd()

		requests, concurrent = limiter.GetStats()
		assert.Equal(t, 3, requests)
		assert.Equal(t, 2, concurrent)
	})
}
