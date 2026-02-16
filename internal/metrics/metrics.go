package metrics

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Metrics holds all Prometheus metrics for the application
type Metrics struct {
	registry *prometheus.Registry

	// Agent metrics
	AgentExecutionsTotal      *prometheus.CounterVec
	AgentExecutionDuration    *prometheus.HistogramVec
	AgentExecutionErrorsTotal *prometheus.CounterVec

	// Tool metrics
	ToolExecutionsTotal      *prometheus.CounterVec
	ToolExecutionDuration    *prometheus.HistogramVec
	ToolExecutionErrorsTotal *prometheus.CounterVec

	// Session metrics
	SessionsActive   prometheus.Gauge
	SessionsTotal    prometheus.Counter
	SessionsArchived prometheus.Counter

	// Telegram metrics
	TelegramMessagesSentTotal     prometheus.Counter
	TelegramMessagesReceivedTotal prometheus.Counter
	TelegramErrorsTotal           prometheus.Counter
}

// NewMetrics creates and registers all metrics
func NewMetrics() *Metrics {
	registry := prometheus.NewRegistry()

	m := &Metrics{
		registry: registry,

		// Agent metrics
		AgentExecutionsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "agent_executions_total",
				Help: "Total number of agent executions",
			},
			[]string{"agent_id", "status"},
		),
		AgentExecutionDuration: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "agent_execution_duration_seconds",
				Help:    "Duration of agent executions in seconds",
				Buckets: prometheus.DefBuckets,
			},
			[]string{"agent_id"},
		),
		AgentExecutionErrorsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "agent_execution_errors_total",
				Help: "Total number of agent execution errors",
			},
			[]string{"agent_id", "error_type"},
		),

		// Tool metrics
		ToolExecutionsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "tool_executions_total",
				Help: "Total number of tool executions",
			},
			[]string{"tool_name", "status"},
		),
		ToolExecutionDuration: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "tool_execution_duration_seconds",
				Help:    "Duration of tool executions in seconds",
				Buckets: prometheus.DefBuckets,
			},
			[]string{"tool_name"},
		),
		ToolExecutionErrorsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "tool_execution_errors_total",
				Help: "Total number of tool execution errors",
			},
			[]string{"tool_name", "error_type"},
		),

		// Session metrics
		SessionsActive: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Name: "sessions_active",
				Help: "Number of currently active sessions",
			},
		),
		SessionsTotal: prometheus.NewCounter(
			prometheus.CounterOpts{
				Name: "sessions_total",
				Help: "Total number of sessions created",
			},
		),
		SessionsArchived: prometheus.NewCounter(
			prometheus.CounterOpts{
				Name: "sessions_archived",
				Help: "Total number of sessions archived",
			},
		),

		// Telegram metrics
		TelegramMessagesSentTotal: prometheus.NewCounter(
			prometheus.CounterOpts{
				Name: "telegram_messages_sent_total",
				Help: "Total number of Telegram messages sent",
			},
		),
		TelegramMessagesReceivedTotal: prometheus.NewCounter(
			prometheus.CounterOpts{
				Name: "telegram_messages_received_total",
				Help: "Total number of Telegram messages received",
			},
		),
		TelegramErrorsTotal: prometheus.NewCounter(
			prometheus.CounterOpts{
				Name: "telegram_errors_total",
				Help: "Total number of Telegram errors",
			},
		),
	}

	// Register all metrics
	m.registerMetrics()

	return m
}

// registerMetrics registers all metrics with the registry
func (m *Metrics) registerMetrics() {
	// Agent metrics
	m.registry.MustRegister(m.AgentExecutionsTotal)
	m.registry.MustRegister(m.AgentExecutionDuration)
	m.registry.MustRegister(m.AgentExecutionErrorsTotal)

	// Tool metrics
	m.registry.MustRegister(m.ToolExecutionsTotal)
	m.registry.MustRegister(m.ToolExecutionDuration)
	m.registry.MustRegister(m.ToolExecutionErrorsTotal)

	// Session metrics
	m.registry.MustRegister(m.SessionsActive)
	m.registry.MustRegister(m.SessionsTotal)
	m.registry.MustRegister(m.SessionsArchived)

	// Telegram metrics
	m.registry.MustRegister(m.TelegramMessagesSentTotal)
	m.registry.MustRegister(m.TelegramMessagesReceivedTotal)
	m.registry.MustRegister(m.TelegramErrorsTotal)
}

// Handler returns an HTTP handler for the metrics endpoint
func (m *Metrics) Handler() http.Handler {
	return promhttp.HandlerFor(m.registry, promhttp.HandlerOpts{
		EnableOpenMetrics: true,
	})
}

// Registry returns the Prometheus registry
func (m *Metrics) Registry() *prometheus.Registry {
	return m.registry
}
