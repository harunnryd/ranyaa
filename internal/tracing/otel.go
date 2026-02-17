package tracing

import (
	"context"
	"sync"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"
)

var (
	providerOnce sync.Once
	providerMu   sync.RWMutex
	provider     *sdktrace.TracerProvider
	providerErr  error
)

// InitOpenTelemetry initializes a process-wide OpenTelemetry tracer provider.
// It is safe to call multiple times.
func InitOpenTelemetry(serviceName string) error {
	providerOnce.Do(func() {
		res, err := resource.New(
			context.Background(),
			resource.WithAttributes(
				semconv.ServiceName(serviceName),
			),
		)
		if err != nil {
			providerErr = err
			return
		}

		tp := sdktrace.NewTracerProvider(
			sdktrace.WithSampler(sdktrace.ParentBased(sdktrace.TraceIDRatioBased(1))),
			sdktrace.WithResource(res),
		)

		providerMu.Lock()
		provider = tp
		providerMu.Unlock()

		otel.SetTracerProvider(tp)
	})

	return providerErr
}

// ShutdownOpenTelemetry flushes and shuts down the global tracer provider.
func ShutdownOpenTelemetry(ctx context.Context) error {
	providerMu.RLock()
	tp := provider
	providerMu.RUnlock()
	if tp == nil {
		return nil
	}
	return tp.Shutdown(ctx)
}

// StartSpan starts a span and ensures trace_id is propagated in the tracing context package.
func StartSpan(ctx context.Context, tracerName, spanName string, attrs ...attribute.KeyValue) (context.Context, trace.Span) {
	if ctx == nil {
		ctx = context.Background()
	}

	tracer := otel.Tracer(tracerName)
	ctx, span := tracer.Start(ctx, spanName, trace.WithAttributes(attrs...))

	if GetTraceID(ctx) == "" {
		sc := span.SpanContext()
		if sc.IsValid() {
			ctx = WithTraceID(ctx, sc.TraceID().String())
		}
	}

	return ctx, span
}
