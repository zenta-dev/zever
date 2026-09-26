package noop

import (
	"context"
	"testing"

	"github.com/zenta-dev/zever/observability"
)

var (
	_ observability.Provider = noopProvider{}
	_ observability.Tracer   = noopTracer{}
	_ observability.Span     = noopSpan{}
	_ observability.Metrics  = noopMetrics{}
)

func TestNew_implementsProvider(t *testing.T) {
	t.Parallel()

	p := New()
	if p == nil {
		t.Fatal("New() = nil, want provider")
	}

	if p.Tracer("scope") == nil {
		t.Error("Tracer() = nil, want non-nil tracer")
	}

	if p.Meter("scope") == nil {
		t.Error("Meter() = nil, want non-nil metrics")
	}
}

func TestSpan_startEnd_doesNotPanic(t *testing.T) {
	t.Parallel()

	p := New()
	tr := p.Tracer("scope")

	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("span chain panicked: %v", r)
			}
		}()

		ctx, span := tr.Start(t.Context(), "op")
		if span == nil {
			t.Fatal("Start() span = nil, want non-nil span")
		}
		if ctx == nil {
			t.Fatal("Start() ctx = nil, want non-nil ctx")
		}

		span.SetAttributes(
			observability.String("s", "v"),
			observability.Int("i", 1),
			observability.Int64("i64", -2),
			observability.Float64("f", 1.5),
			observability.Bool("b", true),
		)
		span.RecordError(nil)
		span.End()
	}()
}

func TestMetrics_allInstruments_nilError(t *testing.T) {
	t.Parallel()

	p := New()
	m := p.Meter("scope")
	ctx := t.Context()

	if err := m.Counter(ctx, "c", 1, observability.String("k", "v")); err != nil {
		t.Errorf("Counter() error = %v, want nil", err)
	}

	if err := m.Gauge(ctx, "g", 2.5); err != nil {
		t.Errorf("Gauge() error = %v, want nil", err)
	}

	if err := m.Histogram(ctx, "h", 3); err != nil {
		t.Errorf("Histogram() error = %v, want nil", err)
	}
}

func TestShutdown_all_nilError(t *testing.T) {
	t.Parallel()

	p := New()
	ctx := t.Context()

	if err := p.Shutdown(ctx); err != nil {
		t.Errorf("Provider.Shutdown() error = %v, want nil", err)
	}

	if err := p.Tracer("scope").Shutdown(ctx); err != nil {
		t.Errorf("Tracer.Shutdown() error = %v, want nil", err)
	}

	if err := p.Meter("scope").Shutdown(ctx); err != nil {
		t.Errorf("Metrics.Shutdown() error = %v, want nil", err)
	}
}

type ctxRoundtripKey struct{}

func TestStart_context_roundtripsValues(t *testing.T) {
	t.Parallel()

	p := New()
	tr := p.Tracer("scope")

	ctx := context.WithValue(t.Context(), ctxRoundtripKey{}, "v")

	got, span := tr.Start(ctx, "op")
	defer span.End()

	if got.Value(ctxRoundtripKey{}) != "v" {
		t.Error("Start() ctx lost parent value, want roundtrip")
	}
}
