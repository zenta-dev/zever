package noop

import (
	"errors"
	"sync"
	"testing"

	"github.com/zenta-dev/zever/core/observability"
)

func TestStart_nilContext_returnsNil(t *testing.T) {
	t.Parallel()

	tr := New().Tracer("scope")
	ctx, span := tr.Start(nil, "op") //nolint:staticcheck // deliberately exercises nil-context handling
	if ctx != nil {
		t.Errorf("Start(nil) ctx = %v, want nil (noop passes through)", ctx)
	}
	if span == nil {
		t.Fatal("Start(nil) span = nil, want non-nil span")
	}
	span.End()
}

func TestSpan_setAttributes_emptyAndNil(t *testing.T) {
	t.Parallel()

	_, span := New().Tracer("scope").Start(t.Context(), "op")
	span.SetAttributes()
	span.SetAttributes(nil...)
	span.RecordError(errors.New("boom"))
	span.End()
}

func TestMetrics_zeroNegativeNaN_nilError(t *testing.T) {
	t.Parallel()

	m := New().Meter("scope")
	values := []float64{0, -1, 1e308, -1e308}
	for _, v := range values {
		if err := m.Counter(t.Context(), "c", v); err != nil {
			t.Errorf("Counter(%v) = %v, want nil", v, err)
		}
		if err := m.Gauge(t.Context(), "g", v); err != nil {
			t.Errorf("Gauge(%v) = %v, want nil", v, err)
		}
		if err := m.Histogram(t.Context(), "h", v); err != nil {
			t.Errorf("Histogram(%v) = %v, want nil", v, err)
		}
	}
}

func TestStart_concurrentSafe(t *testing.T) {
	t.Parallel()

	tr := New().Tracer("scope")
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, span := tr.Start(t.Context(), "op")
			span.SetAttributes(observability.String("k", "v"))
			span.End()
		}()
	}
	wg.Wait()
}
