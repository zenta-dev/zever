// Package observabilitytest provides the conformance kit third-party observability adapters run to prove backend parity.
package observabilitytest

import (
	"errors"
	"testing"

	"github.com/zenta-dev/zever/core/observability"
)

// Conformance verifies factory-built providers implement the
// observability.Provider contract: open/register round-trip, tracer
// spans with attributes and error recording, metrics
// counter/gauge/histogram, and Shutdown cleanup. Each subtest takes
// a fresh instance from factory so cases stay isolated. Tests never
// call time.Sleep and never touch the network.
//
// Cleanup is Shutdown(ctx), not Close: the Provider interface has
// no Close method, so the kit asserts Shutdown returns nil instead.
// Documented no-op exemption: the noop adapter discards every span
// and measurement; the kit only requires the calls not to panic and
// to return nil, never asserting export happened.
func Conformance(t *testing.T, factory func(t *testing.T) observability.Provider) {
	t.Helper()

	t.Run("OpenRegister", func(t *testing.T) { conformanceOpenRegister(t) })
	t.Run("Tracing", func(t *testing.T) { conformanceTracing(t, factory) })
	t.Run("Metrics", func(t *testing.T) { conformanceMetrics(t, factory) })
	t.Run("Shutdown", func(t *testing.T) { conformanceShutdown(t, factory) })
}

func conformanceOpenRegister(t *testing.T) {
	t.Helper()

	if _, err := observability.Open(observability.Adapter("conformance-missing-adapter"), observability.Options{ServiceName: "kit"}); !errors.Is(err, observability.ErrUnknownAdapter) {
		t.Fatalf("Open(missing) err = %v, want ErrUnknownAdapter", err)
	}

	probe := observability.Adapter("conformance-probe-observability")

	if err := observability.Register(probe, nil); !errors.Is(err, observability.ErrNilFactory) {
		t.Fatalf("Register(nil) err = %v, want ErrNilFactory", err)
	}

	stub := func(observability.Options) (observability.Provider, error) {
		return nil, errors.New("observabilitytest: probe factory must not run")
	}

	_ = observability.Register(probe, stub)

	if err := observability.Register(probe, stub); !errors.Is(err, observability.ErrDuplicate) {
		t.Fatalf("Register(duplicate) err = %v, want ErrDuplicate", err)
	}
}

func conformanceTracing(t *testing.T, factory func(t *testing.T) observability.Provider) {
	t.Helper()

	ctx := t.Context()
	p := factory(t)

	tracer := p.Tracer("kit-scope")
	if tracer == nil {
		t.Fatal("Tracer() = nil")
	}

	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("span flow panicked: %v", r)
			}
		}()

		spanCtx, span := tracer.Start(ctx, "kit-operation")
		if span == nil {
			t.Fatal("Start() span = nil")
		}

		if spanCtx == nil {
			t.Error("Start() ctx = nil")
		}

		span.SetAttributes(
			observability.String("k", "v"),
			observability.Int("n", 1),
			observability.Float64("f", 1.5),
			observability.Bool("b", true),
		)
		span.RecordError(errors.New("kit-error"))
		span.End()
	}()

	if err := tracer.Shutdown(ctx); err != nil {
		t.Errorf("Tracer.Shutdown() error = %v, want nil", err)
	}
}

func conformanceMetrics(t *testing.T, factory func(t *testing.T) observability.Provider) {
	t.Helper()

	ctx := t.Context()
	p := factory(t)

	m := p.Meter("kit-scope")
	if m == nil {
		t.Fatal("Meter() = nil")
	}

	attrs := []observability.Attr{observability.String("k", "v")}

	if err := m.Counter(ctx, "kit.counter", 1, attrs...); err != nil {
		t.Errorf("Counter() error = %v, want nil", err)
	}

	if err := m.Gauge(ctx, "kit.gauge", 2.5, attrs...); err != nil {
		t.Errorf("Gauge() error = %v, want nil", err)
	}

	if err := m.Histogram(ctx, "kit.histogram", 0.5, attrs...); err != nil {
		t.Errorf("Histogram() error = %v, want nil", err)
	}

	if err := m.Shutdown(ctx); err != nil {
		t.Errorf("Metrics.Shutdown() error = %v, want nil", err)
	}
}

func conformanceShutdown(t *testing.T, factory func(t *testing.T) observability.Provider) {
	t.Helper()

	ctx := t.Context()
	p := factory(t)

	if err := p.Shutdown(ctx); err != nil {
		t.Fatalf("Shutdown() error = %v", err)
	}

	if err := p.Shutdown(ctx); err != nil {
		t.Errorf("Shutdown() second error = %v, want nil", err)
	}
}
