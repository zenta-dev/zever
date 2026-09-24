package otlp

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"math/big"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"github.com/zenta-dev/zever/observability"
)

type foreignProvider struct{}

func (foreignProvider) Tracer(string) observability.Tracer { return nil }
func (foreignProvider) Meter(string) observability.Metrics { return nil }
func (foreignProvider) Shutdown(context.Context) error     { return nil }

type errSpanExporter struct{ err error }

func (e errSpanExporter) ExportSpans(context.Context, []sdktrace.ReadOnlySpan) error {
	return e.err
}

func (e errSpanExporter) Shutdown(context.Context) error { return e.err }

type errReader struct {
	sdkmetric.ManualReader
	shutdownErr error
}

func (r *errReader) Shutdown(context.Context) error { return r.shutdownErr }

func newTestCache() *instrumentCache {
	return &instrumentCache{
		kinds:      make(map[string]instrumentKind),
		counters:   make(map[string]metric.Float64Counter),
		gauges:     make(map[string]metric.Float64Gauge),
		histograms: make(map[string]metric.Float64Histogram),
	}
}

func testMeterProvider() *sdkmetric.MeterProvider {
	return sdkmetric.NewMeterProvider(sdkmetric.WithReader(sdkmetric.NewManualReader()))
}

func callKind(kind string, c *instrumentCache, mp *sdkmetric.MeterProvider, scope, name string) error {
	switch kind {
	case "counter":
		_, err := c.counter(mp, scope, name)
		return err
	case "gauge":
		_, err := c.gauge(mp, scope, name)
		return err
	default:
		_, err := c.histogram(mp, scope, name)
		return err
	}
}

func callOtherKind(kind string, c *instrumentCache, mp *sdkmetric.MeterProvider, scope, name string) error {
	switch kind {
	case "counter":
		_, err := c.gauge(mp, scope, name)
		return err
	default:
		_, err := c.counter(mp, scope, name)
		return err
	}
}

func TestCoverTracerShutdown(t *testing.T) {
	t.Parallel()

	tp := sdktrace.NewTracerProvider()
	t.Cleanup(func() { _ = tp.Shutdown(t.Context()) })

	tr := &tracer{tp: tp, tracer: tp.Tracer("scope")}

	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()

	if err := tr.Shutdown(ctx); err != nil {
		t.Errorf("tracer.Shutdown() error = %v, want nil", err)
	}
}

func TestCoverProviderAccessorsAndExtractors(t *testing.T) {
	t.Parallel()

	tp := sdktrace.NewTracerProvider()
	t.Cleanup(func() { _ = tp.Shutdown(t.Context()) })

	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	t.Cleanup(func() { _ = mp.Shutdown(t.Context()) })

	p := &provider{
		tracer:  &tracer{tp: tp, tracer: tp.Tracer("scope")},
		metrics: &metrics{mp: mp},
	}

	if p.Tracer("scope") == nil {
		t.Error("Tracer() = nil, want non-nil")
	}

	if p.Meter("scope") == nil {
		t.Error("Meter() = nil, want non-nil")
	}

	if TracerProvider(p) != tp {
		t.Error("TracerProvider() did not return provider tracer")
	}

	if MeterProvider(p) != mp {
		t.Error("MeterProvider() did not return provider meter")
	}

	empty := &provider{}
	if tr := empty.Tracer("scope"); tr != nil {
		if ttr, ok := tr.(*tracer); !ok || ttr != nil {
			t.Error("empty Tracer() = non-nil instrument, want nil *tracer")
		}
	}

	if mm := empty.Meter("scope"); mm != nil {
		if tmm, ok := mm.(*metrics); !ok || tmm != nil {
			t.Error("empty Meter() = non-nil instrument, want nil *metrics")
		}
	}

	if TracerProvider(empty) != nil {
		t.Error("TracerProvider(empty) = non-nil, want nil")
	}

	if MeterProvider(empty) != nil {
		t.Error("MeterProvider(empty) = non-nil, want nil")
	}

	if TracerProvider(foreignProvider{}) != nil {
		t.Error("TracerProvider(foreign) = non-nil, want nil")
	}

	if MeterProvider(foreignProvider{}) != nil {
		t.Error("MeterProvider(foreign) = non-nil, want nil")
	}
}

func TestCoverShutdownJoinErrors(t *testing.T) {
	t.Parallel()

	traceErr := errors.New("trace-boom")
	metricErr := errors.New("metric-boom")

	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(errSpanExporter{err: traceErr}))
	t.Cleanup(func() { _ = tp.Shutdown(t.Context()) })

	reader := &errReader{shutdownErr: metricErr}
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	t.Cleanup(func() { _ = mp.Shutdown(t.Context()) })

	p := &provider{
		tracer:  &tracer{tp: tp, tracer: tp.Tracer("scope")},
		metrics: &metrics{mp: mp},
	}

	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()

	err := p.Shutdown(ctx)
	if err == nil {
		t.Fatal("Shutdown() error = nil, want joined errors")
	}

	if !strings.Contains(err.Error(), "trace-boom") {
		t.Errorf("Shutdown() error = %v, want trace substring", err)
	}

	if !strings.Contains(err.Error(), "metric-boom") {
		t.Errorf("Shutdown() error = %v, want metric substring", err)
	}
}

func TestCoverShutdownPartialNils(t *testing.T) {
	t.Parallel()

	tp := sdktrace.NewTracerProvider()
	t.Cleanup(func() { _ = tp.Shutdown(t.Context()) })

	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	t.Cleanup(func() { _ = mp.Shutdown(t.Context()) })

	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()

	tracerOnly := &provider{tracer: &tracer{tp: tp, tracer: tp.Tracer("scope")}}
	if err := tracerOnly.Shutdown(ctx); err != nil {
		t.Errorf("tracer-only Shutdown() error = %v, want nil", err)
	}

	reader2 := sdkmetric.NewManualReader()
	mp2 := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader2))
	t.Cleanup(func() { _ = mp2.Shutdown(t.Context()) })

	metricsOnly := &provider{metrics: &metrics{mp: mp2}}
	if err := metricsOnly.Shutdown(ctx); err != nil {
		t.Errorf("metrics-only Shutdown() error = %v, want nil", err)
	}

	reader3 := sdkmetric.NewManualReader()
	mp3 := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader3))
	t.Cleanup(func() { _ = mp3.Shutdown(t.Context()) })

	globalsNoTracer := &provider{metrics: &metrics{mp: mp3}, setGlobals: true}
	if err := globalsNoTracer.Shutdown(ctx); err != nil {
		t.Errorf("setGlobals-no-tracer Shutdown() error = %v, want nil", err)
	}
}

func TestCoverTracerStartAndSpan(t *testing.T) {
	t.Parallel()

	sr := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(sr))
	t.Cleanup(func() { _ = tp.Shutdown(t.Context()) })

	tr := &tracer{tp: tp, tracer: tp.Tracer("scope")}

	ctx, sp := tr.Start(t.Context(), "op")
	if ctx == nil {
		t.Error("Start() ctx = nil, want non-nil")
	}

	long := strings.Repeat("v", observability.MaxValueLen+10)
	sp.SetAttributes(
		observability.String("password", "hunter2"),
		observability.String("big", long),
		observability.String("ok", "fine"),
		observability.Int("n", 7),
	)
	sp.RecordError(errors.New("boom"))
	sp.End()

	var nilCtx context.Context

	ctx2, sp2 := tr.Start(nilCtx, "op2")
	if ctx2 == nil {
		t.Error("Start(nil) ctx = nil, want background")
	}

	sp2.End()

	ended := sr.Ended()
	if len(ended) != 2 {
		t.Fatalf("ended spans = %d, want 2", len(ended))
	}

	found := map[string]attribute.Value{}
	for _, kv := range ended[0].Attributes() {
		found[string(kv.Key)] = kv.Value
	}

	redacted, ok := found["password"]
	if !ok {
		t.Fatal("span missing password attribute")
	}

	if redacted.AsString() != "[redacted]" {
		t.Errorf("password = %q, want [redacted]", redacted.AsString())
	}

	bigAttr, ok := found["big"]
	if !ok {
		t.Fatal("span missing big attribute")
	}

	if len(bigAttr.AsString()) != observability.MaxValueLen {
		t.Errorf("big len = %d, want %d", len(bigAttr.AsString()), observability.MaxValueLen)
	}

	if got := found["ok"].AsString(); got != "fine" {
		t.Errorf("ok = %q, want fine", got)
	}

	if got := found["n"].AsInt64(); got != 7 {
		t.Errorf("n = %d, want 7", got)
	}

	if len(ended[0].Events()) == 0 {
		t.Error("span events = 0, want recorded error event")
	}
}

func TestCoverToAttribute(t *testing.T) {
	t.Parallel()

	if got := toAttribute("s", observability.StringAttr("v")); got.Value.AsString() != "v" {
		t.Errorf("string attr = %q, want v", got.Value.AsString())
	}

	if got := toAttribute("i", observability.Int64Attr(42)); got.Value.AsInt64() != 42 {
		t.Errorf("int attr = %d, want 42", got.Value.AsInt64())
	}

	if got := toAttribute("f", observability.Float64Attr(1.5)); got.Value.AsFloat64() != 1.5 {
		t.Errorf("float attr = %v, want 1.5", got.Value.AsFloat64())
	}

	if got := toAttribute("b", observability.BoolAttr(true)); !got.Value.AsBool() {
		t.Error("bool attr = false, want true")
	}

	if got := toAttribute("d", nil); got.Value.AsString() != "<nil>" {
		t.Errorf("default attr = %q, want <nil>", got.Value.AsString())
	}
}

func TestCoverKindMismatchErrorString(t *testing.T) {
	t.Parallel()

	err := kindMismatch("shared", kindGauge, kindCounter)
	want := `otlp: instrument "shared" already registered as counter`

	if err.Error() != want {
		t.Errorf("Error() = %q, want %q", err.Error(), want)
	}

	if tooManyInstruments() == nil {
		t.Error("tooManyInstruments() = nil, want non-nil")
	}
}

func TestCoverIsSensitiveKeyTable(t *testing.T) {
	t.Parallel()

	sensitive := []string{
		"password", "PASSWORD", "db-secret", "api_token", "apiToken",
		"my-apikey", "x-api_key-y", "auth", "Authorization", "cookie",
		"session", "private_key", "privatekey",
	}
	for _, key := range sensitive {
		if !isSensitiveKey(key) {
			t.Errorf("isSensitiveKey(%q) = false, want true", key)
		}
	}

	benign := []string{"user", "request-id", "count", "", "public"}
	for _, key := range benign {
		if isSensitiveKey(key) {
			t.Errorf("isSensitiveKey(%q) = true, want false", key)
		}
	}
}

func TestCoverNormalizeAttrsGaps(t *testing.T) {
	t.Parallel()

	assertStringValue := func(got []observability.Attr, want string) {
		t.Helper()

		if len(got) != 1 {
			t.Fatalf("attrs len = %d, want 1", len(got))
		}

		sv, ok := got[0].Value.(observability.StringValue)
		if !ok || sv.Value != want {
			t.Errorf("value = %+v, want %q", got[0].Value, want)
		}
	}

	assertStringValue(normalizeAttrs([]observability.Attr{observability.String("k", "v")}, 0), "v")
	assertStringValue(normalizeAttrs([]observability.Attr{observability.String("k", "abcdef")}, 0), "abcdef")
	assertStringValue(normalizeAttrs([]observability.Attr{observability.String("k", "abcdef")}, 256), "abcdef")
	assertStringValue(normalizeAttrs([]observability.Attr{observability.Int("token-count", 9)}, -5), "[redacted]")

	got := normalizeAttrs([]observability.Attr{observability.Int("n", 3)}, 0)
	if len(got) != 1 {
		t.Fatalf("attrs len = %d, want 1", len(got))
	}

	iv, ok := got[0].Value.(observability.Int64Value)
	if !ok || iv.Value != 3 {
		t.Errorf("int passthrough = %+v, want 3", got[0].Value)
	}
}

func TestCoverAttrsToKeyValue(t *testing.T) {
	t.Parallel()

	long := strings.Repeat("x", 300)
	kvs := attrsToKeyValue([]observability.Attr{
		observability.String("auth", "s3cret"),
		observability.String("big", long),
		observability.Int("n", 4),
	}, 256)

	if len(kvs) != 3 {
		t.Fatalf("attrsToKeyValue len = %d, want 3", len(kvs))
	}

	if kvs[0].Value.AsString() != "[redacted]" {
		t.Errorf("redacted = %q, want [redacted]", kvs[0].Value.AsString())
	}

	if len(kvs[1].Value.AsString()) != 256 {
		t.Errorf("truncated len = %d, want 256", len(kvs[1].Value.AsString()))
	}

	if kvs[2].Value.AsInt64() != 4 {
		t.Errorf("int = %d, want 4", kvs[2].Value.AsInt64())
	}
}

func TestCoverMetricsSuccessMismatchShutdown(t *testing.T) {
	t.Parallel()

	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	m := &metrics{mp: mp, scope: "covscope", cache: newTestCache()}

	ctx := t.Context()

	if err := m.Counter(ctx, "cov.counter", 1, observability.String("k", "v")); err != nil {
		t.Fatalf("Counter() error = %v", err)
	}

	if err := m.Gauge(ctx, "cov.gauge", 2, observability.String("k", "v")); err != nil {
		t.Fatalf("Gauge() error = %v", err)
	}

	if err := m.Histogram(ctx, "cov.hist", 3, observability.String("k", "v")); err != nil {
		t.Fatalf("Histogram() error = %v", err)
	}

	var rm metricdata.ResourceMetrics
	if err := reader.Collect(ctx, &rm); err != nil {
		t.Fatalf("Collect() error = %v", err)
	}

	total := 0

	for _, sm := range rm.ScopeMetrics {
		total += len(sm.Metrics)
	}

	if total != 3 {
		t.Errorf("collected metrics = %d, want 3", total)
	}

	mismatches := []func() error{
		func() error { return m.Gauge(ctx, "cov.counter", 1) },
		func() error { return m.Histogram(ctx, "cov.counter", 1) },
		func() error { return m.Counter(ctx, "cov.gauge", 1) },
		func() error { return m.Histogram(ctx, "cov.gauge", 1) },
		func() error { return m.Counter(ctx, "cov.hist", 1) },
		func() error { return m.Gauge(ctx, "cov.hist", 1) },
	}

	for i, fn := range mismatches {
		if err := fn(); err == nil {
			t.Errorf("mismatch[%d] error = nil, want kind mismatch", i)
		} else if !errors.Is(err, observability.ErrInstrumentConflict) {
			t.Errorf("mismatch[%d] errors.Is conflict = false (err = %v)", i, err)
		}
	}

	errCache := newTestCache()
	errCache.kinds["taken"] = kindGauge
	errMetrics := &metrics{mp: mp, scope: "covscope", cache: errCache}

	if err := errMetrics.Counter(ctx, "taken", 1); err == nil {
		t.Error("Counter(taken) error = nil, want kind mismatch")
	}

	if err := errMetrics.Histogram(ctx, "taken", 1); err == nil {
		t.Error("Histogram(taken) error = nil, want kind mismatch")
	}

	errCache2 := newTestCache()
	errCache2.kinds["taken2"] = kindCounter
	errMetrics2 := &metrics{mp: mp, scope: "covscope", cache: errCache2}

	if err := errMetrics2.Gauge(ctx, "taken2", 1); err == nil {
		t.Error("Gauge(taken2) error = nil, want kind mismatch")
	}

	shutdownCtx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()

	if err := m.Shutdown(shutdownCtx); err != nil {
		t.Errorf("metrics.Shutdown() error = %v, want nil", err)
	}
}

type cacheKindCase struct {
	name  string
	other instrumentKind
}

func cacheKindCases() []cacheKindCase {
	return []cacheKindCase{
		{name: "counter", other: kindGauge},
		{name: "gauge", other: kindCounter},
		{name: "histogram", other: kindCounter},
	}
}

func TestCoverCacheFullMatrix(t *testing.T) {
	t.Parallel()

	for _, tc := range cacheKindCases() {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			mp := testMeterProvider()
			t.Cleanup(func() { _ = mp.Shutdown(t.Context()) })

			fast := newTestCache()
			if err := callKind(tc.name, fast, mp, "scope", "fast"); err != nil {
				t.Fatalf("create error = %v", err)
			}

			if err := callKind(tc.name, fast, mp, "scope", "fast"); err != nil {
				t.Fatalf("fast-hit error = %v", err)
			}

			readMismatch := newTestCache()
			readMismatch.kinds["shared"] = tc.other

			if err := callKind(tc.name, readMismatch, mp, "scope", "shared"); err == nil {
				t.Fatal("read-lock mismatch error = nil, want mismatch")
			} else if !errors.Is(err, observability.ErrInstrumentConflict) {
				t.Fatalf("errors.Is conflict = false (err = %v)", err)
			}

			for i := 0; i < 300; i++ {
				raceSameKind(tc.name, mp, "scope", "race-hit")
				raceMixedKind(tc.name, mp, "scope", "race-mismatch")
			}

			full := newTestCache()
			for i := 0; i < maxInstruments; i++ {
				full.kinds["k"+strconv.Itoa(i)] = kindGauge
			}

			if err := callKind(tc.name, full, mp, "scope", "one-too-many"); err == nil {
				t.Fatal("limit error = nil, want too many")
			} else if !errors.Is(err, observability.ErrTooManyInstruments) {
				t.Fatalf("errors.Is too-many = false (err = %v)", err)
			}

			if err := callKind(tc.name, newTestCache(), mp, "scope", ""); err == nil {
				t.Fatal("create-error = nil, want SDK error")
			} else if !strings.Contains(err.Error(), "create "+tc.name) {
				t.Fatalf("create error = %v, want create %s prefix", err, tc.name)
			}
		})
	}
}

func raceSameKind(kind string, mp *sdkmetric.MeterProvider, scope, name string) {
	race := newTestCache()

	start := make(chan struct{})

	var wg sync.WaitGroup

	for g := 0; g < 16; g++ {
		wg.Add(1)

		go func() {
			defer wg.Done()
			<-start
			_ = callKind(kind, race, mp, scope, name)
		}()
	}

	close(start)
	wg.Wait()
}

func raceMixedKind(kind string, mp *sdkmetric.MeterProvider, scope, name string) {
	race := newTestCache()

	start := make(chan struct{})

	var wg sync.WaitGroup

	for g := 0; g < 8; g++ {
		wg.Add(1)

		go func() {
			defer wg.Done()
			<-start
			_ = callKind(kind, race, mp, scope, name)
		}()

		wg.Add(1)

		go func() {
			defer wg.Done()
			<-start
			_ = callOtherKind(kind, race, mp, scope, name)
		}()
	}

	close(start)
	wg.Wait()
}

func TestCoverNewEndpointDefault(t *testing.T) {
	t.Parallel()

	opts := validOptions()
	opts.Endpoint = ""

	p, err := New(opts)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()

	_ = p.Shutdown(ctx)
}

func TestCoverNewHeadersAndLimit(t *testing.T) {
	t.Parallel()

	opts := validOptions()
	opts.Endpoint = "127.0.0.1:1"
	opts.Insecure = true
	opts.Headers = map[string]string{"x-test": "v"}
	opts.AttrValueLimit = 512

	p, err := New(opts)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()

	_ = p.Shutdown(ctx)
}

func TestCoverNewInsecureLoopback(t *testing.T) {
	t.Parallel()

	opts := validOptions()
	opts.Endpoint = "127.0.0.1:1"
	opts.Insecure = true

	p, err := New(opts)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()

	_ = p.Shutdown(ctx)
}

func TestCoverNewBadClientCert(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	missing := filepath.Join(dir, "nope.pem")

	opts := validOptions()
	opts.CertFile = missing
	opts.KeyFile = missing

	if _, err := New(opts); err == nil {
		t.Fatal("New() error = nil, want client cert error")
	} else if !strings.Contains(err.Error(), "client cert") {
		t.Fatalf("New() error = %v, want client cert error", err)
	}
}

func TestCoverNewCertKey(t *testing.T) {
	t.Parallel()

	caFile, certFile, keyFile := writeTestCertFiles(t)

	withCA := validOptions()
	withCA.CertFile = certFile
	withCA.KeyFile = keyFile
	withCA.CAFile = caFile

	p, err := New(withCA)
	if err != nil {
		t.Fatalf("New(cert+key+ca) error = %v", err)
	}

	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()

	_ = p.Shutdown(ctx)

	withoutCA := validOptions()
	withoutCA.CertFile = certFile
	withoutCA.KeyFile = keyFile

	p2, err := New(withoutCA)
	if err != nil {
		t.Fatalf("New(cert+key) error = %v", err)
	}

	ctx2, cancel2 := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel2()

	_ = p2.Shutdown(ctx2)
}

func TestCoverNewCAOnly(t *testing.T) {
	t.Parallel()

	caFile, _, _ := writeTestCertFiles(t)

	opts := validOptions()
	opts.CAFile = caFile

	p, err := New(opts)
	if err != nil {
		t.Fatalf("New(ca) error = %v", err)
	}

	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()

	_ = p.Shutdown(ctx)
}

func TestCoverNewSetGlobalsFalseKeepsGlobals(t *testing.T) {
	t.Parallel()

	before := otel.GetTracerProvider()

	opts := validOptions()
	opts.Endpoint = "127.0.0.1:1"
	opts.Insecure = true

	p, err := New(opts)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if otel.GetTracerProvider() != before {
		t.Error("New(SetGlobals=false) changed global tracer provider")
	}

	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()

	_ = p.Shutdown(ctx)

	if otel.GetTracerProvider() != before {
		t.Error("Shutdown(SetGlobals=false) changed global tracer provider")
	}
}

func TestCoverGlobalsSetAndUnregister(t *testing.T) {
	// Not parallel: mutates otel globals.
	savedTP := otel.GetTracerProvider()
	savedProp := otel.GetTextMapPropagator()

	t.Cleanup(func() {
		otel.SetTracerProvider(savedTP)
		otel.SetTextMapPropagator(savedProp)
	})

	opts := validOptions()
	opts.Endpoint = "127.0.0.1:1"
	opts.Insecure = true
	opts.SetGlobals = true

	p, err := New(opts)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	tp := TracerProvider(p)
	if tp == nil {
		t.Fatal("TracerProvider() = nil, want non-nil")
	}

	got, ok := otel.GetTracerProvider().(*sdktrace.TracerProvider)
	if !ok {
		t.Fatalf("global provider = %T, want *sdktrace.TracerProvider", otel.GetTracerProvider())
	}

	if got != tp {
		t.Error("global provider is not the created tracer provider")
	}

	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()

	_ = p.Shutdown(ctx)

	if after, ok := otel.GetTracerProvider().(*sdktrace.TracerProvider); ok && after == tp {
		t.Error("Shutdown() did not unregister global tracer provider")
	}
}

func TestCoverUnregisterGlobalsNoop(t *testing.T) {
	// Not parallel: mutates otel globals.
	savedTP := otel.GetTracerProvider()
	savedProp := otel.GetTextMapPropagator()

	t.Cleanup(func() {
		otel.SetTracerProvider(savedTP)
		otel.SetTextMapPropagator(savedProp)
	})

	owned := sdktrace.NewTracerProvider()
	t.Cleanup(func() { _ = owned.Shutdown(t.Context()) })

	other := sdktrace.NewTracerProvider()
	t.Cleanup(func() { _ = other.Shutdown(t.Context()) })

	registerGlobals(owned)
	unregisterGlobals(other)

	got, ok := otel.GetTracerProvider().(*sdktrace.TracerProvider)
	if !ok || got != owned {
		t.Error("unregisterGlobals(other) changed globals, want no-op")
	}

	unregisterGlobals(owned)
}

func writeTestCertFiles(t *testing.T) (caFile, certFile, keyFile string) {
	t.Helper()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("GenerateKey() error = %v", err)
	}

	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "test-ca"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
		IsCA:                  true,
	}

	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("CreateCertificate() error = %v", err)
	}

	caPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})

	dir := t.TempDir()
	caFile = filepath.Join(dir, "ca.pem")
	certFile = filepath.Join(dir, "cert.pem")
	keyFile = filepath.Join(dir, "key.pem")

	if err := os.WriteFile(caFile, caPEM, 0o600); err != nil {
		t.Fatalf("WriteFile(ca) error = %v", err)
	}

	if err := os.WriteFile(certFile, caPEM, 0o600); err != nil {
		t.Fatalf("WriteFile(cert) error = %v", err)
	}

	if err := os.WriteFile(keyFile, keyPEM, 0o600); err != nil {
		t.Fatalf("WriteFile(key) error = %v", err)
	}

	return caFile, certFile, keyFile
}
