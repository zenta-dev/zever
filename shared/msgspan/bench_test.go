package msgspan

import (
	"context"
	"testing"

	"github.com/zenta-dev/zever/core/observability"
)

// discardSpan drops everything so benchmarks measure the msgspan wrapper, not
// a recorder's growth.
type discardSpan struct{}

func (discardSpan) SetAttributes(...observability.Attr) {}
func (discardSpan) RecordError(error)                   {}
func (discardSpan) End()                                {}

// discardTracer implements SpanStarter and discards every span.
type discardTracer struct{}

func (discardTracer) Start(ctx context.Context, _ string) (context.Context, observability.Span) {
	return ctx, discardSpan{}
}

func (discardTracer) StartSpan(ctx context.Context, _ string, opts ...observability.SpanStartOption) (context.Context, observability.Span) {
	_ = observability.NewSpanConfig(opts...)

	return ctx, discardSpan{}
}

func (discardTracer) Shutdown(context.Context) error { return nil }

type discardProvider struct{}

func (discardProvider) Tracer(string) observability.Tracer { return discardTracer{} }
func (discardProvider) Meter(string) observability.Metrics { return nil }
func (discardProvider) Shutdown(context.Context) error     { return nil }

// BenchmarkProducerUntraced is the zero-alloc fast path with no provider.
func BenchmarkProducerUntraced(b *testing.B) {
	ctx := context.Background()

	b.ReportAllocs()

	for b.Loop() {
		_, finish := Producer(ctx, nil, "queue", "low", "msg-1")
		finish(OutcomeOK)
	}
}

// BenchmarkProducerTraced measures a producer span against a discarding tracer.
func BenchmarkProducerTraced(b *testing.B) {
	ctx := context.Background()

	var p observability.Provider = discardProvider{}

	b.ReportAllocs()

	for b.Loop() {
		_, finish := Producer(ctx, p, "queue", "low", "msg-1")
		finish(OutcomeOK)
	}
}

// BenchmarkConsumerTraced measures a consumer span against a discarding tracer.
func BenchmarkConsumerTraced(b *testing.B) {
	ctx := context.Background()

	var p observability.Provider = discardProvider{}

	b.ReportAllocs()

	for b.Loop() {
		_, finish := Consumer(ctx, p, "eventbus", "orders", "msg-1")
		finish(OutcomeOK)
	}
}
