package otlp

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"

	"github.com/zenta-dev/zever/observability"
)

func validOptions() observability.Options {
	return observability.Options{ServiceName: "svc", SampleRatio: 1}
}

func TestNew_invalidOptions_returnsError(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		mut  func(*observability.Options)
	}{
		{name: "empty service", mut: func(o *observability.Options) { o.ServiceName = "" }},
		{name: "bad endpoint scheme", mut: func(o *observability.Options) {
			o.Endpoint = "https://host:4317"
		}},
		{name: "bad endpoint slash", mut: func(o *observability.Options) {
			o.Endpoint = "host:4317/v1"
		}},
		{name: "insecure non-loopback", mut: func(o *observability.Options) {
			o.Endpoint = "collector.example.com:4317"
			o.Insecure = true
		}},
		{name: "bad ratio negative", mut: func(o *observability.Options) { o.SampleRatio = -0.1 }},
		{name: "bad ratio above one", mut: func(o *observability.Options) { o.SampleRatio = 1.5 }},
		{name: "cert without key", mut: func(o *observability.Options) { o.CertFile = "cert.pem" }},
		{name: "bad attr limit", mut: func(o *observability.Options) { o.AttrValueLimit = 10 }},
		{name: "auth header insecure", mut: func(o *observability.Options) {
			o.Endpoint = "localhost:4317"
			o.Insecure = true
			o.Headers = map[string]string{"Authorization": "x"}
		}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			opts := validOptions()
			tc.mut(&opts)

			if _, err := New(opts); err == nil {
				t.Fatal("New() error = nil, want non-nil")
			} else if !errors.Is(err, observability.ErrInvalidOptions) {
				t.Errorf("errors.Is(err, ErrInvalidOptions) = false (err = %v)", err)
			}
		})
	}
}

func TestNew_badCAFile_returnsError(t *testing.T) {
	t.Parallel()

	opts := validOptions()
	opts.CAFile = "/nonexistent/ca.pem"

	if _, err := New(opts); err == nil {
		t.Fatal("New() error = nil, want ca error")
	}
}

func TestNew_metricExporterError_shutsDownTracer(t *testing.T) {
	// Not parallel: mutates global exporter hooks.

	hooksMu.Lock()
	oldTrace := newTraceExporter
	oldMetric := newMetricExporter
	newMetricExporter = func(context.Context, ...otlpmetricgrpc.Option) (*otlpmetricgrpc.Exporter, error) {
		return nil, errors.New("metric exporter down")
	}
	hooksMu.Unlock()

	t.Cleanup(func() {
		hooksMu.Lock()
		newTraceExporter = oldTrace
		newMetricExporter = oldMetric
		hooksMu.Unlock()
	})

	if _, err := New(validOptions()); err == nil {
		t.Fatal("New() error = nil, want metric exporter error")
	} else if !strings.Contains(err.Error(), "metric") {
		t.Errorf("error = %v, want metric exporter error", err)
	}
}

func TestNew_traceExporterError_returnsError(t *testing.T) {
	// Not parallel: mutates global exporter hooks.

	hooksMu.Lock()
	oldTrace := newTraceExporter
	oldMetric := newMetricExporter
	newTraceExporter = func(context.Context, ...otlptracegrpc.Option) (*otlptrace.Exporter, error) {
		return nil, errors.New("trace exporter down")
	}
	hooksMu.Unlock()

	t.Cleanup(func() {
		hooksMu.Lock()
		newTraceExporter = oldTrace
		newMetricExporter = oldMetric
		hooksMu.Unlock()
	})

	if _, err := New(validOptions()); err == nil {
		t.Fatal("New() error = nil, want trace exporter error")
	} else if !strings.Contains(err.Error(), "trace") {
		t.Errorf("error = %v, want trace exporter error", err)
	}
}

func TestShutdown_emptyProvider_nilError(t *testing.T) {
	t.Parallel()

	var p provider

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := p.Shutdown(ctx); err != nil {
		t.Errorf("Shutdown() error = %v, want nil", err)
	}
}

func TestShutdown_joinsErrors(t *testing.T) {
	t.Parallel()

	tp := sdktrace.NewTracerProvider()
	t.Cleanup(func() { _ = tp.Shutdown(context.Background()) })

	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	t.Cleanup(func() { _ = mp.Shutdown(context.Background()) })

	p := &provider{tracer: &tracer{tp: tp, tracer: tp.Tracer("test")}, metrics: &metrics{mp: mp}}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := p.Shutdown(ctx); err != nil {
		t.Errorf("Shutdown() error = %v, want nil", err)
	}
}

func TestInstrumentCache_kindMismatch_typedError(t *testing.T) {
	t.Parallel()

	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	t.Cleanup(func() { _ = mp.Shutdown(context.Background()) })

	c := &instrumentCache{
		kinds:      make(map[string]instrumentKind),
		counters:   make(map[string]metric.Float64Counter),
		gauges:     make(map[string]metric.Float64Gauge),
		histograms: make(map[string]metric.Float64Histogram),
	}

	ctx := context.Background()
	if _, err := c.counter(mp, "scope", "shared"); err != nil {
		t.Fatalf("counter() error = %v", err)
	}

	if _, err := c.gauge(mp, "scope", "shared"); err == nil {
		t.Fatal("gauge() error = nil, want kind mismatch")
	} else {
		var km *KindMismatchError
		if !errors.As(err, &km) {
			t.Fatalf("errors.As(err, KindMismatchError) = false (err = %T %v)", err, err)
		}
		if km.Name != "shared" {
			t.Errorf("KindMismatchError.Name = %q, want shared", km.Name)
		}
	}

	if _, err := c.histogram(mp, "scope", "shared"); err == nil {
		t.Fatal("histogram() error = nil, want kind mismatch")
	}

	// Original kind still usable.
	if _, err := c.counter(mp, "scope", "shared"); err != nil {
		t.Errorf("counter() re-fetch error = %v, want nil", err)
	}

	_ = ctx
}

func TestInstrumentCache_limit_returnsTooMany(t *testing.T) {
	t.Parallel()

	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	t.Cleanup(func() { _ = mp.Shutdown(context.Background()) })

	c := &instrumentCache{
		kinds:      make(map[string]instrumentKind),
		counters:   make(map[string]metric.Float64Counter),
		gauges:     make(map[string]metric.Float64Gauge),
		histograms: make(map[string]metric.Float64Histogram),
	}

	// Fill to the limit with distinct names.
	for i := 0; i < maxInstruments; i++ {
		name := "counter_" + strings.Repeat("a", 8) + string(rune('0'+i%10)) + strings.Repeat("b", 4) + string(rune('A'+i/10%26)) + strings.Repeat("c", 4) + string(rune('a'+i/260%26)) + strings.Repeat("d", 2) + string(rune('0'+(i/6760)%10))
		if _, err := c.counter(mp, "scope", name); err != nil {
			t.Fatalf("counter(%q) error = %v", name, err)
		}
	}

	if _, err := c.counter(mp, "scope", "one-too-many"); err == nil {
		t.Fatal("counter() error = nil, want too many instruments")
	} else if !errors.Is(err, observability.ErrTooManyInstruments) {
		t.Errorf("errors.Is(err, ErrTooManyInstruments) = false (err = %v)", err)
	}
}

func TestNormalizeAttrs_capsTruncatesRedacts(t *testing.T) {
	t.Parallel()

	// Cap.
	in := make([]observability.Attr, 0, observability.MaxAttrs+5)
	for i := 0; i < observability.MaxAttrs+5; i++ {
		in = append(in, observability.Int("k", i))
	}
	if got := normalizeAttrs(in, 0); len(got) != observability.MaxAttrs {
		t.Errorf("cap len = %d, want %d", len(got), observability.MaxAttrs)
	}

	// Redact.
	got := normalizeAttrs([]observability.Attr{observability.String("password", "hunter2")}, 0)
	if len(got) != 1 {
		t.Fatalf("redact len = %d, want 1", len(got))
	}
	if sv, ok := got[0].Value.(observability.StringValue); !ok || sv.Value != "[redacted]" {
		t.Errorf("redact value = %+v, want [redacted]", got[0].Value)
	}

	// Truncate custom limit.
	got = normalizeAttrs([]observability.Attr{observability.String("k", "abcdef")}, 3)
	if sv, ok := got[0].Value.(observability.StringValue); !ok || sv.Value != "abc" {
		t.Errorf("truncate value = %+v, want abc", got[0].Value)
	}
}
