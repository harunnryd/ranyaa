package webhook

import (
	"sync"
	"time"
)

// MetricsTracker tracks webhook performance metrics
type MetricsTracker struct {
	metrics map[string]*WebhookMetrics
	mu      sync.RWMutex
}

// NewMetricsTracker creates a new metrics tracker
func NewMetricsTracker() *MetricsTracker {
	return &MetricsTracker{
		metrics: make(map[string]*WebhookMetrics),
	}
}

// Track records a webhook execution
func (mt *MetricsTracker) Track(path string, method string, success bool, durationMs float64) {
	mt.mu.Lock()
	defer mt.mu.Unlock()

	key := method + ":" + path

	m, exists := mt.metrics[key]
	if !exists {
		m = &WebhookMetrics{
			Path:                path,
			Method:              method,
			TotalRequests:       0,
			SuccessCount:        0,
			FailureCount:        0,
			AverageResponseTime: 0,
		}
		mt.metrics[key] = m
	}

	m.TotalRequests++
	if success {
		m.SuccessCount++
	} else {
		m.FailureCount++
	}

	// Update average response time (running average)
	m.AverageResponseTime = (m.AverageResponseTime*float64(m.TotalRequests-1) + durationMs) / float64(m.TotalRequests)
	m.LastRequestAt = time.Now().UnixMilli()
}

// GetMetrics returns all metrics
func (mt *MetricsTracker) GetMetrics() []WebhookMetrics {
	mt.mu.RLock()
	defer mt.mu.RUnlock()

	result := make([]WebhookMetrics, 0, len(mt.metrics))
	for _, m := range mt.metrics {
		result = append(result, *m)
	}
	return result
}

// GetMetricsForWebhook returns metrics for a specific webhook
func (mt *MetricsTracker) GetMetricsForWebhook(path string, method string) *WebhookMetrics {
	mt.mu.RLock()
	defer mt.mu.RUnlock()

	key := method + ":" + path
	m, exists := mt.metrics[key]
	if !exists {
		return nil
	}

	// Return a copy
	result := *m
	return &result
}
