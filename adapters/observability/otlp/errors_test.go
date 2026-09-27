package otlp

import (
	"errors"
	"testing"

	"go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"

	"github.com/zenta-dev/zever/core/observability"
)

func TestKindMismatchIsAndAs(t *testing.T) {
	t.Parallel()

	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	t.Cleanup(func() { _ = mp.Shutdown(t.Context()) })

	c := &instrumentCache{
		kinds:      make(map[string]instrumentKind),
		counters:   make(map[string]metric.Float64Counter),
		gauges:     make(map[string]metric.Float64Gauge),
		histograms: make(map[string]metric.Float64Histogram),
	}

	if _, err := c.counter(mp, "scope", "shared"); err != nil {
		t.Fatalf("counter() error = %v", err)
	}

	_, err := c.gauge(mp, "scope", "shared")
	if err == nil {
		t.Fatal("gauge() error = nil, want kind mismatch")
	}

	if !errors.Is(err, observability.ErrInstrumentConflict) {
		t.Errorf("errors.Is(err, ErrInstrumentConflict) = false (err = %v)", err)
	}

	var km *KindMismatchError
	if !errors.As(err, &km) {
		t.Fatalf("errors.As(err, KindMismatchError) = false (err = %T %v)", err, err)
	}

	if km.Name != "shared" {
		t.Errorf("KindMismatchError.Name = %q, want shared", km.Name)
	}
}
