package noop

import (
	"context"

	"github.com/zenta-dev/zever/core/observability"
)

type noopProvider struct{}

type noopTracer struct{}

type noopSpan struct{}

type noopMetrics struct{}

// New returns a Provider that discards all spans and metrics.
func New() observability.Provider { return noopProvider{} }

func (noopProvider) Tracer(string) observability.Tracer { return noopTracer{} }
func (noopProvider) Meter(string) observability.Metrics { return noopMetrics{} }
func (noopProvider) Shutdown(context.Context) error     { return nil }
func (noopTracer) Shutdown(context.Context) error       { return nil }
func (noopMetrics) Shutdown(context.Context) error      { return nil }
func (noopSpan) SetAttributes(...observability.Attr)    {}
func (noopSpan) RecordError(error)                      {}
func (noopSpan) End()                                   {}
func (noopMetrics) Counter(context.Context, string, float64, ...observability.Attr) error {
	return nil
}
func (noopMetrics) Gauge(context.Context, string, float64, ...observability.Attr) error {
	return nil
}
func (noopMetrics) Histogram(context.Context, string, float64, ...observability.Attr) error {
	return nil
}

func (noopTracer) Start(ctx context.Context, _ string) (context.Context, observability.Span) {
	return ctx, noopSpan{}
}
