package observability

import (
	"context"
	"os"
	"sync"
	"time"

	"github.com/rs/zerolog"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// AuditEvent represents a structured event for the audit log
type AuditEvent struct {
	Type      string                 `json:"event_type"`
	Timestamp time.Time              `json:"timestamp"`
	Actor     string                 `json:"actor,omitempty"` // SessionKey or AgentID
	Action    string                 `json:"action"`          // e.g., "tool_executed", "command_enqueued"
	Status    string                 `json:"status"`          // "success", "failure", "pending"
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
	TraceID   string                 `json:"trace_id,omitempty"`
}

// AuditLogger handles recording and persisting audit events
type AuditLogger struct {
	logger zerolog.Logger
	mu     sync.Mutex
	file   *os.File
}

var (
	auditOnce sync.Once
	auditInst *AuditLogger
)

// GetAuditLogger returns the global audit logger instance
func GetAuditLogger() *AuditLogger {
	auditOnce.Do(func() {
		// Default to stderr if not initialized
		auditInst = &AuditLogger{
			logger: zerolog.New(os.Stderr).With().Timestamp().Logger(),
		}
	})
	return auditInst
}

// Initialize global audit logger with a specific file
func InitAuditLogger(path string) error {
	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}

	auditInst = &AuditLogger{
		logger: zerolog.New(file).With().Timestamp().Logger(),
		file:   file,
	}
	return nil
}

// Record emits an audit event to the log file and optionally to OpenTelemetry
func (a *AuditLogger) Record(ctx context.Context, event AuditEvent) {
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}

	// Extract tracing info if available
	span := trace.SpanFromContext(ctx)
	if span.SpanContext().IsValid() {
		event.TraceID = span.SpanContext().TraceID().String()

		// Also record as a span event for Otel
		span.AddEvent(event.Action, trace.WithAttributes(
			attribute.String("audit.type", event.Type),
			attribute.String("audit.status", event.Status),
			attribute.String("audit.actor", event.Actor),
		))
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	// Direct JSON logging to file/logger
	entry := a.logger.Log().
		Str("type", event.Type).
		Str("actor", event.Actor).
		Str("action", event.Action).
		Str("status", event.Status).
		Str("trace_id", event.TraceID)

	if event.Metadata != nil {
		entry.Interface("metadata", event.Metadata)
	}

	entry.Msg("")
}

// Close closes the audit logger's file handle
func (a *AuditLogger) Close() error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.file != nil {
		return a.file.Close()
	}
	return nil
}

// Helper methods for common events

func RecordToolAudit(ctx context.Context, toolName, actor, status string, metadata map[string]interface{}) {
	GetAuditLogger().Record(ctx, AuditEvent{
		Type:     "tool",
		Actor:    actor,
		Action:   "execute:" + toolName,
		Status:   status,
		Metadata: metadata,
	})
}

func RecordSecurityAudit(ctx context.Context, action, actor, status string, metadata map[string]interface{}) {
	GetAuditLogger().Record(ctx, AuditEvent{
		Type:     "security",
		Actor:    actor,
		Action:   action,
		Status:   status,
		Metadata: metadata,
	})
}

func RecordConfigAudit(ctx context.Context, action, actor string, metadata map[string]interface{}) {
	GetAuditLogger().Record(ctx, AuditEvent{
		Type:     "config",
		Actor:    actor,
		Action:   action,
		Status:   "success",
		Metadata: metadata,
	})
}
