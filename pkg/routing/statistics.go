package routing

import (
	"sort"
	"sync"
	"time"
)

// StatisticsTracker tracks routing metrics and statistics
type StatisticsTracker struct {
	stats             map[string]*RouteStatistics
	globalStats       *GlobalStatistics
	latencyHistograms map[string][]int64
	mu                sync.RWMutex
}

// NewStatisticsTracker creates a new statistics tracker
func NewStatisticsTracker() *StatisticsTracker {
	return &StatisticsTracker{
		stats:             make(map[string]*RouteStatistics),
		globalStats:       &GlobalStatistics{},
		latencyHistograms: make(map[string][]int64),
	}
}

// RecordMatch records a pattern match
func (st *StatisticsTracker) RecordMatch(routeID string, routeName string) {
	st.mu.Lock()
	defer st.mu.Unlock()

	stats := st.getOrCreateStats(routeID, routeName)
	stats.MatchCount++
	stats.LastUsed = time.Now().UnixMilli()

	if stats.FirstUsed == 0 {
		stats.FirstUsed = stats.LastUsed
	}
}

// RecordSuccess records a successful routing
func (st *StatisticsTracker) RecordSuccess(routeID string, routeName string, latency int64) {
	st.mu.Lock()
	defer st.mu.Unlock()

	stats := st.getOrCreateStats(routeID, routeName)
	stats.SuccessCount++
	st.updateLatencyMetrics(routeID, latency)

	st.globalStats.SuccessfulRoutings++
	st.globalStats.TotalMessages++
	st.updateGlobalLatency(latency)
}

// RecordFailure records a failed routing
func (st *StatisticsTracker) RecordFailure(routeID string, routeName string, latency int64, err error) {
	st.mu.Lock()
	defer st.mu.Unlock()

	stats := st.getOrCreateStats(routeID, routeName)
	stats.FailureCount++
	st.updateLatencyMetrics(routeID, latency)

	// Track error type
	if stats.ErrorTypes == nil {
		stats.ErrorTypes = make(map[string]int64)
	}
	errorType := "unknown"
	if routingErr, ok := err.(*RoutingError); ok {
		errorType = routingErr.Code
	}
	stats.ErrorTypes[errorType]++

	st.globalStats.FailedRoutings++
	st.globalStats.TotalMessages++
	st.updateGlobalLatency(latency)
}

// RecordFallback records a fallback route usage
func (st *StatisticsTracker) RecordFallback(routeID string, routeName string) {
	st.mu.Lock()
	defer st.mu.Unlock()

	stats := st.getOrCreateStats(routeID, routeName)
	stats.FallbackCount++
}

// GetRouteStatistics returns statistics for a specific route
func (st *StatisticsTracker) GetRouteStatistics(routeID string) *RouteStatistics {
	st.mu.RLock()
	defer st.mu.RUnlock()

	if stats, exists := st.stats[routeID]; exists {
		// Return a copy
		statsCopy := *stats
		statsCopy.ErrorTypes = make(map[string]int64)
		for k, v := range stats.ErrorTypes {
			statsCopy.ErrorTypes[k] = v
		}
		return &statsCopy
	}
	return nil
}

// GetAllStatistics returns statistics for all routes
func (st *StatisticsTracker) GetAllStatistics() []*RouteStatistics {
	st.mu.RLock()
	defer st.mu.RUnlock()

	result := make([]*RouteStatistics, 0, len(st.stats))
	for _, stats := range st.stats {
		statsCopy := *stats
		statsCopy.ErrorTypes = make(map[string]int64)
		for k, v := range stats.ErrorTypes {
			statsCopy.ErrorTypes[k] = v
		}
		result = append(result, &statsCopy)
	}
	return result
}

// GetGlobalStatistics returns aggregated statistics
func (st *StatisticsTracker) GetGlobalStatistics() *GlobalStatistics {
	st.mu.RLock()
	defer st.mu.RUnlock()

	globalCopy := *st.globalStats
	return &globalCopy
}

// ResetStatistics resets statistics for a specific route or all routes
func (st *StatisticsTracker) ResetStatistics(routeID string) {
	st.mu.Lock()
	defer st.mu.Unlock()

	if routeID == "" {
		// Reset all
		st.stats = make(map[string]*RouteStatistics)
		st.latencyHistograms = make(map[string][]int64)
		st.globalStats = &GlobalStatistics{}
	} else {
		// Reset specific route
		delete(st.stats, routeID)
		delete(st.latencyHistograms, routeID)
	}
}

// UpdateRouteCounts updates route counts in global stats
func (st *StatisticsTracker) UpdateRouteCounts(total, enabled int) {
	st.mu.Lock()
	defer st.mu.Unlock()

	st.globalStats.TotalRoutes = total
	st.globalStats.EnabledRoutes = enabled
}

// getOrCreateStats gets or creates statistics for a route
func (st *StatisticsTracker) getOrCreateStats(routeID string, routeName string) *RouteStatistics {
	if stats, exists := st.stats[routeID]; exists {
		return stats
	}

	stats := &RouteStatistics{
		RouteID:    routeID,
		RouteName:  routeName,
		ErrorTypes: make(map[string]int64),
	}
	st.stats[routeID] = stats
	return stats
}

// updateLatencyMetrics updates latency metrics for a route
func (st *StatisticsTracker) updateLatencyMetrics(routeID string, latency int64) {
	stats := st.stats[routeID]

	// Update histogram
	if _, exists := st.latencyHistograms[routeID]; !exists {
		st.latencyHistograms[routeID] = make([]int64, 0)
	}
	st.latencyHistograms[routeID] = append(st.latencyHistograms[routeID], latency)

	// Update min/max
	if stats.MinLatency == 0 || latency < stats.MinLatency {
		stats.MinLatency = latency
	}
	if latency > stats.MaxLatency {
		stats.MaxLatency = latency
	}

	// Calculate average
	totalOps := stats.SuccessCount + stats.FailureCount
	if totalOps > 0 {
		stats.AvgLatency = (stats.AvgLatency*float64(totalOps-1) + float64(latency)) / float64(totalOps)
	}

	// Calculate percentiles
	st.calculatePercentiles(routeID)
}

// calculatePercentiles calculates latency percentiles
func (st *StatisticsTracker) calculatePercentiles(routeID string) {
	histogram := st.latencyHistograms[routeID]
	if len(histogram) == 0 {
		return
	}

	// Sort histogram
	sorted := make([]int64, len(histogram))
	copy(sorted, histogram)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })

	stats := st.stats[routeID]
	stats.P50Latency = st.percentile(sorted, 50)
	stats.P95Latency = st.percentile(sorted, 95)
	stats.P99Latency = st.percentile(sorted, 99)
}

// percentile calculates a percentile from sorted values
func (st *StatisticsTracker) percentile(sorted []int64, p int) int64 {
	if len(sorted) == 0 {
		return 0
	}

	index := (len(sorted) * p) / 100
	if index >= len(sorted) {
		index = len(sorted) - 1
	}
	return sorted[index]
}

// updateGlobalLatency updates global latency metrics
func (st *StatisticsTracker) updateGlobalLatency(latency int64) {
	totalMessages := st.globalStats.TotalMessages
	if totalMessages > 0 {
		st.globalStats.AvgRoutingLatency = (st.globalStats.AvgRoutingLatency*float64(totalMessages-1) + float64(latency)) / float64(totalMessages)
	}
}
