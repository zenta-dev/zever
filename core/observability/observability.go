package observability

import (
	"context"
)

// Span is a single unit of traced work.
type Span interface {
	// SetAttributes attaches attributes to the span.
	SetAttributes(attrs ...Attr)
	// RecordError records an error on the span.
	RecordError(err error)
	// End finishes the span.
	End()
}

// Tracer creates spans.
type Tracer interface {
	// Start begins a span with the given name, returning the updated context.
	Start(ctx context.Context, name string) (context.Context, Span)
	// Shutdown flushes any buffered spans.
	Shutdown(ctx context.Context) error
}

// Metrics records measurements.
type Metrics interface {
	// Counter adds value to the named counter.
	Counter(ctx context.Context, name string, value float64, attrs ...Attr) error
	// Gauge sets the named gauge.
	Gauge(ctx context.Context, name string, value float64, attrs ...Attr) error
	// Histogram records a value in the named histogram.
	Histogram(ctx context.Context, name string, value float64, attrs ...Attr) error
	// Shutdown flushes any buffered metrics.
	Shutdown(ctx context.Context) error
}

// Provider supplies tracers and meters.
type Provider interface {
	// Tracer returns a Tracer for the given scope.
	Tracer(scope string) Tracer
	// Meter returns a Metrics for the given scope.
	Meter(scope string) Metrics
	// Shutdown flushes telemetry and releases resources.
	Shutdown(ctx context.Context) error
}

// Factory creates a Provider from the given Options.
type Factory func(opts Options) (Provider, error)
