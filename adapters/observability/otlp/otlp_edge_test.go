package otlp

import (
	"errors"
	"strconv"
	"sync"
	"testing"

	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"

	"github.com/zenta-dev/zever/core/observability"
)

func TestInstrumentCache_concurrentSameName_singleInstrument(t *testing.T) {
	t.Parallel()

	mp := testMeterProvider()
	t.Cleanup(func() { _ = mp.Shutdown(t.Context()) })

	c := newTestCache()
	const workers = 32

	var wg sync.WaitGroup
	errs := make([]error, workers)
	for w := range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, errs[w] = c.counter(mp, "scope", "shared")
		}()
	}
	wg.Wait()
	for w, err := range errs {
		if err != nil {
			t.Errorf("worker %d counter() = %v, want nil", w, err)
		}
	}

	c.mu.RLock()
	defer c.mu.RUnlock()
	if len(c.kinds) != 1 {
		t.Errorf("kinds = %d, want 1 (single instrument for one name)", len(c.kinds))
	}
	if len(c.counters) != 1 {
		t.Errorf("counters = %d, want 1", len(c.counters))
	}
}

func TestAttrsToKeyValue_empty_nil(t *testing.T) {
	t.Parallel()

	if got := attrsToKeyValue(nil, 0); len(got) != 0 {
		t.Errorf("attrsToKeyValue(nil) len = %d, want 0", len(got))
	}
	if got := attrsToKeyValue([]observability.Attr{}, 0); len(got) != 0 {
		t.Errorf("attrsToKeyValue(empty) len = %d, want 0", len(got))
	}
}

func TestNormalizeAttrs_exactLimit_notTruncated(t *testing.T) {
	t.Parallel()

	s := make([]byte, observability.MaxValueLen)
	for i := range s {
		s[i] = 'x'
	}
	got := normalizeAttrs([]observability.Attr{observability.String("k", string(s))}, 0)
	if len(got) != 1 {
		t.Fatalf("normalizeAttrs() len = %d, want 1", len(got))
	}
	sv, ok := got[0].Value.(observability.StringValue)
	if !ok {
		t.Fatalf("value type = %T, want StringValue", got[0].Value)
	}
	if len(sv.Value) != observability.MaxValueLen {
		t.Errorf("value len = %d, want %d (no truncation at exact limit)", len(sv.Value), observability.MaxValueLen)
	}
}

func TestNormalizeAttrs_capsAtMaxAttrs(t *testing.T) {
	t.Parallel()

	attrs := make([]observability.Attr, observability.MaxAttrs+5)
	for i := range attrs {
		attrs[i] = observability.String("k"+strconv.Itoa(i), "v")
	}

	got := normalizeAttrs(attrs, 0)
	if len(got) != observability.MaxAttrs {
		t.Errorf("normalizeAttrs() len = %d, want %d (capped)", len(got), observability.MaxAttrs)
	}
}

func TestNormalizeAttrs_customLimitTruncates(t *testing.T) {
	t.Parallel()

	got := normalizeAttrs([]observability.Attr{observability.String("k", "abcdefgh")}, 4)
	if len(got) != 1 {
		t.Fatalf("normalizeAttrs() len = %d, want 1", len(got))
	}

	sv, ok := got[0].Value.(observability.StringValue)
	if !ok {
		t.Fatalf("value type = %T, want StringValue", got[0].Value)
	}

	if sv.Value != "abcd" {
		t.Errorf("value = %q, want abcd (truncated to custom limit 4)", sv.Value)
	}
}

func TestNormalizeAttrs_customLimitExact_notTruncated(t *testing.T) {
	t.Parallel()

	got := normalizeAttrs([]observability.Attr{observability.String("k", "abcd")}, 4)
	sv, ok := got[0].Value.(observability.StringValue)
	if !ok {
		t.Fatalf("value type = %T, want StringValue", got[0].Value)
	}

	if sv.Value != "abcd" {
		t.Errorf("value = %q, want abcd (no truncation at exact custom limit)", sv.Value)
	}
}

func TestInstrumentCache_cacheHitReturnsSame(t *testing.T) {
	t.Parallel()

	mp := testMeterProvider()
	t.Cleanup(func() { _ = mp.Shutdown(t.Context()) })

	c := newTestCache()

	first, err := c.counter(mp, "scope", "shared")
	if err != nil {
		t.Fatalf("counter() = %v, want nil", err)
	}

	second, err := c.counter(mp, "scope", "shared")
	if err != nil {
		t.Fatalf("second counter() = %v, want nil", err)
	}

	if first != second {
		t.Error("cache hit returned different instrument, want same")
	}

	g1, err := c.gauge(mp, "scope", "gshared")
	if err != nil {
		t.Fatalf("gauge() = %v, want nil", err)
	}

	g2, err := c.gauge(mp, "scope", "gshared")
	if err != nil {
		t.Fatalf("second gauge() = %v, want nil", err)
	}

	if g1 != g2 {
		t.Error("gauge cache hit returned different instrument, want same")
	}

	h1, err := c.histogram(mp, "scope", "hshared")
	if err != nil {
		t.Fatalf("histogram() = %v, want nil", err)
	}

	h2, err := c.histogram(mp, "scope", "hshared")
	if err != nil {
		t.Fatalf("second histogram() = %v, want nil", err)
	}

	if h1 != h2 {
		t.Error("histogram cache hit returned different instrument, want same")
	}
}

func TestMetrics_customLimitRespected(t *testing.T) {
	t.Parallel()

	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	t.Cleanup(func() { _ = mp.Shutdown(t.Context()) })

	m := &metrics{mp: mp, scope: "edge", limit: 4, cache: newTestCache()}

	if err := m.Counter(t.Context(), "c", 1, observability.String("k", "abcdefgh")); err != nil {
		t.Fatalf("Counter() = %v, want nil", err)
	}

	var rm metricdata.ResourceMetrics
	if err := reader.Collect(t.Context(), &rm); err != nil {
		t.Fatalf("Collect() = %v", err)
	}

	for _, sm := range rm.ScopeMetrics {
		for _, mm := range sm.Metrics {
			sum, ok := mm.Data.(metricdata.Sum[float64])
			if !ok {
				t.Fatalf("metric data = %T, want Sum[float64]", mm.Data)
			}
			for _, dp := range sum.DataPoints {
				for _, a := range dp.Attributes.ToSlice() {
					if a.Key == "k" {
						if got := a.Value.AsString(); got != "abcd" {
							t.Errorf("attr k = %q, want abcd (custom limit 4)", got)
						}
					}
				}
			}
		}
	}
}

func TestKindMismatch_allKindPairs(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		register func(*instrumentCache, *sdkmetric.MeterProvider) error
		request  func(*instrumentCache, *sdkmetric.MeterProvider) error
	}{
		{
			name: "counter then gauge",
			register: func(c *instrumentCache, mp *sdkmetric.MeterProvider) error {
				_, err := c.counter(mp, "s", "x")
				return err
			},
			request: func(c *instrumentCache, mp *sdkmetric.MeterProvider) error {
				_, err := c.gauge(mp, "s", "x")
				return err
			},
		},
		{
			name: "gauge then histogram",
			register: func(c *instrumentCache, mp *sdkmetric.MeterProvider) error {
				_, err := c.gauge(mp, "s", "x")
				return err
			},
			request: func(c *instrumentCache, mp *sdkmetric.MeterProvider) error {
				_, err := c.histogram(mp, "s", "x")
				return err
			},
		},
		{
			name: "histogram then counter",
			register: func(c *instrumentCache, mp *sdkmetric.MeterProvider) error {
				_, err := c.histogram(mp, "s", "x")
				return err
			},
			request: func(c *instrumentCache, mp *sdkmetric.MeterProvider) error {
				_, err := c.counter(mp, "s", "x")
				return err
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			mp := testMeterProvider()
			t.Cleanup(func() { _ = mp.Shutdown(t.Context()) })

			c := newTestCache()

			if err := tc.register(c, mp); err != nil {
				t.Fatalf("register = %v, want nil", err)
			}

			err := tc.request(c, mp)
			if err == nil {
				t.Fatal("request = nil, want kind mismatch error")
			}

			var kme *KindMismatchError
			if !errors.As(err, &kme) {
				t.Fatalf("request error = %T, want *KindMismatchError", err)
			}

			if !errors.Is(err, observability.ErrInstrumentConflict) {
				t.Errorf("request error = %v, want wrap of ErrInstrumentConflict", err)
			}
		})
	}
}
