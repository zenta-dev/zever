package noop

import (
	"context"

	"github.com/zenta-dev/zever/core/observability"
)

type noopProvider struct{}

type noopTracer struct{}

type noopSpan struct {
	kind  observability.SpanKind
	attrs []observability.Attr
	links []observability.SpanLink
}

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

func (noopTracer) Start(ctx context.Context, name string) (context.Context, observability.Span) {
	return noopTracer{}.StartSpan(ctx, name)
}

func (noopTracer) StartSpan(ctx context.Context, _ string, opts ...observability.SpanStartOption) (context.Context, observability.Span) {
	cfg := observability.NewSpanConfig(opts...)
	return ctx, noopSpan{kind: cfg.Kind, attrs: cfg.Attrs, links: cfg.Links}
}

// Kind returns the span kind recorded at StartSpan.
func (s noopSpan) Kind() observability.SpanKind { return s.kind }

// Attrs returns the span attributes recorded at StartSpan.
func (s noopSpan) Attrs() []observability.Attr { return s.attrs }

var _ observability.SpanStarter = noopTracer{}
