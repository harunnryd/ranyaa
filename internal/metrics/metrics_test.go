package metrics

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNewMetrics(t *testing.T) {
	m := NewMetrics()

	if m == nil {
		t.Fatal("NewMetrics returned nil")
	}

	if m.registry == nil {
		t.Error("Registry is nil")
	}

	// Verify agent metrics
	if m.AgentExecutionsTotal == nil {
		t.Error("AgentExecutionsTotal is nil")
	}
	if m.AgentExecutionDuration == nil {
		t.Error("AgentExecutionDuration is nil")
	}
	if m.AgentExecutionErrorsTotal == nil {
		t.Error("AgentExecutionErrorsTotal is nil")
	}

	// Verify tool metrics
	if m.ToolExecutionsTotal == nil {
		t.Error("ToolExecutionsTotal is nil")
	}
	if m.ToolExecutionDuration == nil {
		t.Error("ToolExecutionDuration is nil")
	}
	if m.ToolExecutionErrorsTotal == nil {
		t.Error("ToolExecutionErrorsTotal is nil")
	}

	// Verify session metrics
	if m.SessionsActive == nil {
		t.Error("SessionsActive is nil")
	}
	if m.SessionsTotal == nil {
		t.Error("SessionsTotal is nil")
	}
	if m.SessionsArchived == nil {
		t.Error("SessionsArchived is nil")
	}

	// Verify Telegram metrics
	if m.TelegramMessagesSentTotal == nil {
		t.Error("TelegramMessagesSentTotal is nil")
	}
	if m.TelegramMessagesReceivedTotal == nil {
		t.Error("TelegramMessagesReceivedTotal is nil")
	}
	if m.TelegramErrorsTotal == nil {
		t.Error("TelegramErrorsTotal is nil")
	}
}

func TestMetricsHandler(t *testing.T) {
	m := NewMetrics()

	// Record some sample metrics so they appear in output
	m.AgentExecutionsTotal.WithLabelValues("test", "success").Inc()
	m.AgentExecutionDuration.WithLabelValues("test").Observe(1.0)
	m.AgentExecutionErrorsTotal.WithLabelValues("test", "error").Inc()
	m.ToolExecutionsTotal.WithLabelValues("test", "success").Inc()
	m.ToolExecutionDuration.WithLabelValues("test").Observe(0.5)
	m.ToolExecutionErrorsTotal.WithLabelValues("test", "error").Inc()

	handler := m.Handler()
	if handler == nil {
		t.Fatal("Handler returned nil")
	}

	// Test HTTP endpoint
	req := httptest.NewRequest("GET", "/metrics", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	body := w.Body.String()

	// Verify metrics are exposed
	expectedMetrics := []string{
		"agent_executions_total",
		"agent_execution_duration_seconds",
		"agent_execution_errors_total",
		"tool_executions_total",
		"tool_execution_duration_seconds",
		"tool_execution_errors_total",
		"sessions_active",
		"sessions_total",
		"sessions_archived",
		"telegram_messages_sent_total",
		"telegram_messages_received_total",
		"telegram_errors_total",
	}

	for _, metric := range expectedMetrics {
		if !strings.Contains(body, metric) {
			t.Errorf("Metrics output missing: %s", metric)
		}
	}
}

func TestMetricsRegistry(t *testing.T) {
	m := NewMetrics()

	registry := m.Registry()
	if registry == nil {
		t.Fatal("Registry returned nil")
	}

	// Record some sample metrics so they appear in gather
	m.AgentExecutionsTotal.WithLabelValues("test", "success").Inc()
	m.AgentExecutionDuration.WithLabelValues("test").Observe(1.0)
	m.AgentExecutionErrorsTotal.WithLabelValues("test", "error").Inc()
	m.ToolExecutionsTotal.WithLabelValues("test", "success").Inc()
	m.ToolExecutionDuration.WithLabelValues("test").Observe(0.5)
	m.ToolExecutionErrorsTotal.WithLabelValues("test", "error").Inc()

	// Verify metrics are registered
	metricFamilies, err := registry.Gather()
	if err != nil {
		t.Fatalf("Failed to gather metrics: %v", err)
	}

	if len(metricFamilies) == 0 {
		t.Error("No metrics registered")
	}

	// Count registered metrics
	metricNames := make(map[string]bool)
	for _, mf := range metricFamilies {
		metricNames[*mf.Name] = true
	}

	expectedCount := 12 // Total number of metrics
	if len(metricNames) != expectedCount {
		t.Errorf("Expected %d metrics, got %d", expectedCount, len(metricNames))
	}
}

func TestAgentMetrics(t *testing.T) {
	m := NewMetrics()

	t.Run("increment agent executions", func(t *testing.T) {
		m.AgentExecutionsTotal.WithLabelValues("test-agent", "success").Inc()

		// Verify metric was incremented
		metricFamilies, _ := m.registry.Gather()
		found := false
		for _, mf := range metricFamilies {
			if *mf.Name == "agent_executions_total" {
				found = true
				if len(mf.Metric) == 0 {
					t.Error("No metrics recorded")
				}
			}
		}
		if !found {
			t.Error("agent_executions_total metric not found")
		}
	})

	t.Run("record agent execution duration", func(t *testing.T) {
		m.AgentExecutionDuration.WithLabelValues("test-agent").Observe(1.5)

		metricFamilies, _ := m.registry.Gather()
		found := false
		for _, mf := range metricFamilies {
			if *mf.Name == "agent_execution_duration_seconds" {
				found = true
			}
		}
		if !found {
			t.Error("agent_execution_duration_seconds metric not found")
		}
	})

	t.Run("increment agent errors", func(t *testing.T) {
		m.AgentExecutionErrorsTotal.WithLabelValues("test-agent", "timeout").Inc()

		metricFamilies, _ := m.registry.Gather()
		found := false
		for _, mf := range metricFamilies {
			if *mf.Name == "agent_execution_errors_total" {
				found = true
			}
		}
		if !found {
			t.Error("agent_execution_errors_total metric not found")
		}
	})
}

func TestToolMetrics(t *testing.T) {
	m := NewMetrics()

	t.Run("increment tool executions", func(t *testing.T) {
		m.ToolExecutionsTotal.WithLabelValues("test-tool", "success").Inc()

		metricFamilies, _ := m.registry.Gather()
		found := false
		for _, mf := range metricFamilies {
			if *mf.Name == "tool_executions_total" {
				found = true
			}
		}
		if !found {
			t.Error("tool_executions_total metric not found")
		}
	})

	t.Run("record tool execution duration", func(t *testing.T) {
		m.ToolExecutionDuration.WithLabelValues("test-tool").Observe(0.5)

		metricFamilies, _ := m.registry.Gather()
		found := false
		for _, mf := range metricFamilies {
			if *mf.Name == "tool_execution_duration_seconds" {
				found = true
			}
		}
		if !found {
			t.Error("tool_execution_duration_seconds metric not found")
		}
	})

	t.Run("increment tool errors", func(t *testing.T) {
		m.ToolExecutionErrorsTotal.WithLabelValues("test-tool", "permission_denied").Inc()

		metricFamilies, _ := m.registry.Gather()
		found := false
		for _, mf := range metricFamilies {
			if *mf.Name == "tool_execution_errors_total" {
				found = true
			}
		}
		if !found {
			t.Error("tool_execution_errors_total metric not found")
		}
	})
}

func TestSessionMetrics(t *testing.T) {
	m := NewMetrics()

	t.Run("set active sessions", func(t *testing.T) {
		m.SessionsActive.Set(5)

		metricFamilies, _ := m.registry.Gather()
		found := false
		for _, mf := range metricFamilies {
			if *mf.Name == "sessions_active" {
				found = true
				if len(mf.Metric) > 0 && *mf.Metric[0].Gauge.Value != 5 {
					t.Errorf("Expected value 5, got %f", *mf.Metric[0].Gauge.Value)
				}
			}
		}
		if !found {
			t.Error("sessions_active metric not found")
		}
	})

	t.Run("increment total sessions", func(t *testing.T) {
		m.SessionsTotal.Inc()

		metricFamilies, _ := m.registry.Gather()
		found := false
		for _, mf := range metricFamilies {
			if *mf.Name == "sessions_total" {
				found = true
			}
		}
		if !found {
			t.Error("sessions_total metric not found")
		}
	})

	t.Run("increment archived sessions", func(t *testing.T) {
		m.SessionsArchived.Inc()

		metricFamilies, _ := m.registry.Gather()
		found := false
		for _, mf := range metricFamilies {
			if *mf.Name == "sessions_archived" {
				found = true
			}
		}
		if !found {
			t.Error("sessions_archived metric not found")
		}
	})
}

func TestTelegramMetrics(t *testing.T) {
	m := NewMetrics()

	t.Run("increment messages sent", func(t *testing.T) {
		m.TelegramMessagesSentTotal.Inc()

		metricFamilies, _ := m.registry.Gather()
		found := false
		for _, mf := range metricFamilies {
			if *mf.Name == "telegram_messages_sent_total" {
				found = true
			}
		}
		if !found {
			t.Error("telegram_messages_sent_total metric not found")
		}
	})

	t.Run("increment messages received", func(t *testing.T) {
		m.TelegramMessagesReceivedTotal.Inc()

		metricFamilies, _ := m.registry.Gather()
		found := false
		for _, mf := range metricFamilies {
			if *mf.Name == "telegram_messages_received_total" {
				found = true
			}
		}
		if !found {
			t.Error("telegram_messages_received_total metric not found")
		}
	})

	t.Run("increment telegram errors", func(t *testing.T) {
		m.TelegramErrorsTotal.Inc()

		metricFamilies, _ := m.registry.Gather()
		found := false
		for _, mf := range metricFamilies {
			if *mf.Name == "telegram_errors_total" {
				found = true
			}
		}
		if !found {
			t.Error("telegram_errors_total metric not found")
		}
	})
}

func TestMetricsIsolation(t *testing.T) {
	// Create two separate metrics instances
	m1 := NewMetrics()
	m2 := NewMetrics()

	// Increment metrics in m1
	m1.SessionsTotal.Inc()
	m1.SessionsTotal.Inc()

	// Increment metrics in m2
	m2.SessionsTotal.Inc()

	// Verify m1 has 2
	metricFamilies1, _ := m1.registry.Gather()
	for _, mf := range metricFamilies1 {
		if *mf.Name == "sessions_total" {
			if len(mf.Metric) > 0 && *mf.Metric[0].Counter.Value != 2 {
				t.Errorf("m1: Expected value 2, got %f", *mf.Metric[0].Counter.Value)
			}
		}
	}

	// Verify m2 has 1
	metricFamilies2, _ := m2.registry.Gather()
	for _, mf := range metricFamilies2 {
		if *mf.Name == "sessions_total" {
			if len(mf.Metric) > 0 && *mf.Metric[0].Counter.Value != 1 {
				t.Errorf("m2: Expected value 1, got %f", *mf.Metric[0].Counter.Value)
			}
		}
	}
}
