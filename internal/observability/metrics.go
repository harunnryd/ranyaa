package observability

import (
	"net/http"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type moduleMetrics struct {
	queueSize    *prometheus.GaugeVec
	enqueueTotal *prometheus.CounterVec
	dequeueTotal *prometheus.CounterVec
	taskDuration *prometheus.HistogramVec

	activeSessions       prometheus.Gauge
	sessionLoadDuration  prometheus.Histogram
	sessionSaveDuration  prometheus.Histogram
	memorySearchDuration prometheus.Histogram
	memoryWriteDuration  prometheus.Histogram
	memoryEntriesTotal   prometheus.Gauge

	toolExecutionTotal    *prometheus.CounterVec
	toolExecutionDuration *prometheus.HistogramVec
	toolErrorsTotal       *prometheus.CounterVec

	agentRunTotal    *prometheus.CounterVec
	agentRunDuration *prometheus.HistogramVec
	agentErrorsTotal *prometheus.CounterVec
	providerCooldown *prometheus.GaugeVec
}

var (
	metricsOnce sync.Once
	metricsInst *moduleMetrics
)

func getMetrics() *moduleMetrics {
	metricsOnce.Do(func() {
		m := &moduleMetrics{
			queueSize: prometheus.NewGaugeVec(
				prometheus.GaugeOpts{
					Name: "queue_size",
					Help: "Current queue size by lane.",
				},
				[]string{"lane"},
			),
			enqueueTotal: prometheus.NewCounterVec(
				prometheus.CounterOpts{
					Name: "enqueue_total",
					Help: "Total enqueue operations by lane.",
				},
				[]string{"lane"},
			),
			dequeueTotal: prometheus.NewCounterVec(
				prometheus.CounterOpts{
					Name: "dequeue_total",
					Help: "Total dequeue/completion operations by lane and status.",
				},
				[]string{"lane", "status"},
			),
			taskDuration: prometheus.NewHistogramVec(
				prometheus.HistogramOpts{
					Name:    "task_duration_seconds",
					Help:    "Task execution duration in seconds by lane.",
					Buckets: prometheus.DefBuckets,
				},
				[]string{"lane"},
			),
			activeSessions: prometheus.NewGauge(
				prometheus.GaugeOpts{
					Name: "active_sessions",
					Help: "Current active session count.",
				},
			),
			sessionLoadDuration: prometheus.NewHistogram(
				prometheus.HistogramOpts{
					Name:    "session_load_duration_seconds",
					Help:    "Session load duration in seconds.",
					Buckets: prometheus.DefBuckets,
				},
			),
			sessionSaveDuration: prometheus.NewHistogram(
				prometheus.HistogramOpts{
					Name:    "session_save_duration_seconds",
					Help:    "Session save duration in seconds.",
					Buckets: prometheus.DefBuckets,
				},
			),
			memorySearchDuration: prometheus.NewHistogram(
				prometheus.HistogramOpts{
					Name:    "memory_search_duration_seconds",
					Help:    "Memory search duration in seconds.",
					Buckets: prometheus.DefBuckets,
				},
			),
			memoryWriteDuration: prometheus.NewHistogram(
				prometheus.HistogramOpts{
					Name:    "memory_write_duration_seconds",
					Help:    "Memory write/sync duration in seconds.",
					Buckets: prometheus.DefBuckets,
				},
			),
			memoryEntriesTotal: prometheus.NewGauge(
				prometheus.GaugeOpts{
					Name: "memory_entries_total",
					Help: "Total memory entries/chunks indexed.",
				},
			),
			toolExecutionTotal: prometheus.NewCounterVec(
				prometheus.CounterOpts{
					Name: "tool_execution_total",
					Help: "Total tool executions by tool and status.",
				},
				[]string{"tool", "status"},
			),
			toolExecutionDuration: prometheus.NewHistogramVec(
				prometheus.HistogramOpts{
					Name:    "tool_execution_duration_seconds",
					Help:    "Tool execution duration in seconds by tool.",
					Buckets: prometheus.DefBuckets,
				},
				[]string{"tool"},
			),
			toolErrorsTotal: prometheus.NewCounterVec(
				prometheus.CounterOpts{
					Name: "tool_errors_total",
					Help: "Total tool execution errors by tool.",
				},
				[]string{"tool"},
			),
			agentRunTotal: prometheus.NewCounterVec(
				prometheus.CounterOpts{
					Name: "agent_run_total",
					Help: "Total agent runs by provider and status.",
				},
				[]string{"provider", "status"},
			),
			agentRunDuration: prometheus.NewHistogramVec(
				prometheus.HistogramOpts{
					Name:    "agent_run_duration_seconds",
					Help:    "Agent run duration in seconds by provider.",
					Buckets: prometheus.DefBuckets,
				},
				[]string{"provider"},
			),
			agentErrorsTotal: prometheus.NewCounterVec(
				prometheus.CounterOpts{
					Name: "agent_errors_total",
					Help: "Total agent errors by provider.",
				},
				[]string{"provider"},
			),
			providerCooldown: prometheus.NewGaugeVec(
				prometheus.GaugeOpts{
					Name: "provider_cooldown_active",
					Help: "Provider cooldown active state (1 active, 0 inactive).",
				},
				[]string{"provider"},
			),
		}

		prometheus.MustRegister(
			m.queueSize,
			m.enqueueTotal,
			m.dequeueTotal,
			m.taskDuration,
			m.activeSessions,
			m.sessionLoadDuration,
			m.sessionSaveDuration,
			m.memorySearchDuration,
			m.memoryWriteDuration,
			m.memoryEntriesTotal,
			m.toolExecutionTotal,
			m.toolExecutionDuration,
			m.toolErrorsTotal,
			m.agentRunTotal,
			m.agentRunDuration,
			m.agentErrorsTotal,
			m.providerCooldown,
		)

		metricsInst = m
	})

	return metricsInst
}

// EnsureRegistered initializes and registers metrics the first time it is called.
func EnsureRegistered() {
	_ = getMetrics()
}

func MetricsHandler() http.Handler {
	EnsureRegistered()
	return promhttp.Handler()
}

func RecordQueueEnqueue(lane string, queueSize int) {
	m := getMetrics()
	m.enqueueTotal.WithLabelValues(lane).Inc()
	m.queueSize.WithLabelValues(lane).Set(float64(queueSize))
}

func SetQueueSize(lane string, queueSize int) {
	m := getMetrics()
	m.queueSize.WithLabelValues(lane).Set(float64(queueSize))
}

func RecordQueueCompletion(lane string, duration time.Duration, success bool, queueSize int) {
	m := getMetrics()
	status := "error"
	if success {
		status = "success"
	}
	m.dequeueTotal.WithLabelValues(lane, status).Inc()
	m.taskDuration.WithLabelValues(lane).Observe(duration.Seconds())
	m.queueSize.WithLabelValues(lane).Set(float64(queueSize))
}

func SetActiveSessions(count int) {
	m := getMetrics()
	m.activeSessions.Set(float64(count))
}

func RecordSessionLoad(duration time.Duration) {
	m := getMetrics()
	m.sessionLoadDuration.Observe(duration.Seconds())
}

func RecordSessionSave(duration time.Duration) {
	m := getMetrics()
	m.sessionSaveDuration.Observe(duration.Seconds())
}

func RecordMemorySearch(duration time.Duration) {
	m := getMetrics()
	m.memorySearchDuration.Observe(duration.Seconds())
}

func RecordMemoryWrite(duration time.Duration) {
	m := getMetrics()
	m.memoryWriteDuration.Observe(duration.Seconds())
}

func SetMemoryEntries(total int) {
	m := getMetrics()
	m.memoryEntriesTotal.Set(float64(total))
}

func RecordToolExecution(tool string, duration time.Duration, success bool) {
	m := getMetrics()
	status := "error"
	if success {
		status = "success"
	}
	m.toolExecutionTotal.WithLabelValues(tool, status).Inc()
	m.toolExecutionDuration.WithLabelValues(tool).Observe(duration.Seconds())
	if !success {
		m.toolErrorsTotal.WithLabelValues(tool).Inc()
	}
}

func RecordAgentRun(provider string, duration time.Duration, success bool) {
	m := getMetrics()
	status := "error"
	if success {
		status = "success"
	}
	m.agentRunTotal.WithLabelValues(provider, status).Inc()
	m.agentRunDuration.WithLabelValues(provider).Observe(duration.Seconds())
	if !success {
		m.agentErrorsTotal.WithLabelValues(provider).Inc()
	}
}

func SetProviderCooldown(provider string, active bool) {
	m := getMetrics()
	value := 0.0
	if active {
		value = 1.0
	}
	m.providerCooldown.WithLabelValues(provider).Set(value)
}
