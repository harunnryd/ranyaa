package webhook

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMetricsTrackerTrack(t *testing.T) {
	mt := NewMetricsTracker()

	path := "/webhook/test"
	method := "POST"

	// Track successful request
	mt.Track(path, method, true, 100.0)

	// Get metrics
	metrics := mt.GetMetricsForWebhook(path, method)
	assert.NotNil(t, metrics)
	assert.Equal(t, path, metrics.Path)
	assert.Equal(t, method, metrics.Method)
	assert.Equal(t, int64(1), metrics.TotalRequests)
	assert.Equal(t, int64(1), metrics.SuccessCount)
	assert.Equal(t, int64(0), metrics.FailureCount)
	assert.Equal(t, 100.0, metrics.AverageResponseTime)
	assert.Greater(t, metrics.LastRequestAt, int64(0))
}

func TestMetricsTrackerTrackMultiple(t *testing.T) {
	mt := NewMetricsTracker()

	path := "/webhook/test"
	method := "POST"

	// Track multiple requests
	mt.Track(path, method, true, 100.0)
	mt.Track(path, method, true, 200.0)
	mt.Track(path, method, false, 150.0)

	// Get metrics
	metrics := mt.GetMetricsForWebhook(path, method)
	assert.NotNil(t, metrics)
	assert.Equal(t, int64(3), metrics.TotalRequests)
	assert.Equal(t, int64(2), metrics.SuccessCount)
	assert.Equal(t, int64(1), metrics.FailureCount)
	assert.Equal(t, 150.0, metrics.AverageResponseTime) // (100 + 200 + 150) / 3
}

func TestMetricsTrackerGetMetrics(t *testing.T) {
	mt := NewMetricsTracker()

	// Track requests for multiple webhooks
	mt.Track("/webhook/test1", "POST", true, 100.0)
	mt.Track("/webhook/test2", "GET", true, 200.0)
	mt.Track("/webhook/test1", "POST", false, 150.0)

	// Get all metrics
	allMetrics := mt.GetMetrics()
	assert.Len(t, allMetrics, 2)

	// Verify metrics exist for both webhooks
	found1 := false
	found2 := false
	for _, m := range allMetrics {
		if m.Path == "/webhook/test1" && m.Method == "POST" {
			found1 = true
			assert.Equal(t, int64(2), m.TotalRequests)
		}
		if m.Path == "/webhook/test2" && m.Method == "GET" {
			found2 = true
			assert.Equal(t, int64(1), m.TotalRequests)
		}
	}
	assert.True(t, found1)
	assert.True(t, found2)
}

func TestMetricsTrackerGetMetricsForWebhookNotFound(t *testing.T) {
	mt := NewMetricsTracker()

	// Get metrics for non-existent webhook
	metrics := mt.GetMetricsForWebhook("/webhook/notfound", "POST")
	assert.Nil(t, metrics)
}

func TestMetricsTrackerAverageResponseTime(t *testing.T) {
	mt := NewMetricsTracker()

	path := "/webhook/test"
	method := "POST"

	// Track requests with different durations
	durations := []float64{100.0, 200.0, 300.0, 400.0, 500.0}
	for _, duration := range durations {
		mt.Track(path, method, true, duration)
	}

	// Get metrics
	metrics := mt.GetMetricsForWebhook(path, method)
	assert.NotNil(t, metrics)

	// Average should be 300.0
	expectedAvg := 300.0
	assert.Equal(t, expectedAvg, metrics.AverageResponseTime)
}

func TestMetricsTrackerConcurrency(t *testing.T) {
	mt := NewMetricsTracker()

	path := "/webhook/test"
	method := "POST"

	// Track requests concurrently
	done := make(chan bool)
	for i := 0; i < 100; i++ {
		go func() {
			mt.Track(path, method, true, 100.0)
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 100; i++ {
		<-done
	}

	// Get metrics
	metrics := mt.GetMetricsForWebhook(path, method)
	assert.NotNil(t, metrics)
	assert.Equal(t, int64(100), metrics.TotalRequests)
	assert.Equal(t, int64(100), metrics.SuccessCount)
}
