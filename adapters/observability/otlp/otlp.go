package otlp

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/zenta-dev/zever/core/observability"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/propagation"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"
	"google.golang.org/grpc/credentials"
)

var (
	globalMu   sync.RWMutex
	globalTP   *sdktrace.TracerProvider
	globalProp propagation.TextMapPropagator
)

var hooksMu sync.RWMutex

var newTraceExporter = otlptracegrpc.New

var newMetricExporter = otlpmetricgrpc.New

var defaultPropagator = propagation.NewCompositeTextMapPropagator(
	propagation.TraceContext{},
	propagation.Baggage{},
)

var registerGlobals = func(tp *sdktrace.TracerProvider) {
	globalMu.Lock()
	defer globalMu.Unlock()

	globalTP = tp
	globalProp = otel.GetTextMapPropagator()

	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(defaultPropagator)
}

var unregisterGlobals = func(tp *sdktrace.TracerProvider) {
	globalMu.Lock()
	defer globalMu.Unlock()

	if globalTP == tp {
		globalTP = nil

		otel.SetTracerProvider(noop.NewTracerProvider())
		otel.SetTextMapPropagator(globalProp)
	}
}

type tracer struct {
	tp     *sdktrace.TracerProvider
	tracer trace.Tracer
}

func (t *tracer) Shutdown(ctx context.Context) error { return t.tp.Shutdown(ctx) }

type provider struct {
	tracer     *tracer
	metrics    *metrics
	setGlobals bool
}

func (p *provider) Tracer(string) observability.Tracer { return p.tracer }
func (p *provider) Meter(string) observability.Metrics { return p.metrics }

func (p *provider) Shutdown(ctx context.Context) error {
	var errs []error

	if p.tracer != nil {
		if err := p.tracer.tp.Shutdown(ctx); err != nil {
			errs = append(errs, err)
		}
	}
	if p.metrics != nil {
		if err := p.metrics.mp.Shutdown(ctx); err != nil {
			errs = append(errs, err)
		}
	}
	if p.setGlobals && p.tracer != nil {
		hooksMu.RLock()

		fn := unregisterGlobals

		hooksMu.RUnlock()

		fn(p.tracer.tp)
	}

	return errors.Join(errs...)
}

type otelSpan struct{ span trace.Span }

func (s *otelSpan) SetAttributes(attrs ...observability.Attr) {
	for _, a := range normalizeAttrs(attrs, 0) {
		s.span.SetAttributes(toAttribute(a.Key, a.Value))
	}
}

func (s *otelSpan) RecordError(err error) {
	s.span.RecordError(err)
}

func (s *otelSpan) End() { s.span.End() }

func toAttribute(key string, value observability.AttributeValue) attribute.KeyValue {
	switch v := value.(type) {
	case observability.StringValue:
		return attribute.String(key, v.Value)
	case observability.Int64Value:
		return attribute.Int64(key, v.Value)
	case observability.Float64Value:
		return attribute.Float64(key, v.Value)
	case observability.BoolValue:
		return attribute.Bool(key, v.Value)
	default:
		return attribute.String(key, fmt.Sprintf("%v", v))
	}
}

//nolint:spancheck // Start returns the span to the caller, who owns calling End() (see otelSpan.End); this is the standard tracer.Start contract, not a leak
func (t *tracer) Start(ctx context.Context, name string) (context.Context, observability.Span) {
	if ctx == nil {
		ctx = context.Background() //nolint:contextcheck // nil context treated as background for safe propagation
	}

	ctx, span := t.tracer.Start(ctx, name)

	return ctx, &otelSpan{span: span}
}

// TracerProvider extracts a TracerProvider from the given observability.Provider.
func TracerProvider(p observability.Provider) *sdktrace.TracerProvider {
	if pr, ok := p.(*provider); ok && pr.tracer != nil {
		return pr.tracer.tp
	}

	return nil
}

// MeterProvider extracts a MeterProvider from the given observability.Provider.
func MeterProvider(p observability.Provider) *sdkmetric.MeterProvider {
	if pr, ok := p.(*provider); ok && pr.metrics != nil {
		return pr.metrics.mp
	}

	return nil
}

type metrics struct {
	mp    *sdkmetric.MeterProvider
	scope string
	limit int
	cache *instrumentCache
}

// normalizeAttrs caps attribute count, truncates overlong strings, and redacts secrets.
func normalizeAttrs(attrs []observability.Attr, limit int) []observability.Attr {
	effective := limit
	if effective <= 0 {
		effective = observability.MaxValueLen
	}

	n := len(attrs)
	if n > observability.MaxAttrs {
		n = observability.MaxAttrs
	}
	out := make([]observability.Attr, 0, n)
	for i := 0; i < n; i++ {
		a := attrs[i]
		if isSensitiveKey(a.Key) {
			a.Value = observability.StringAttr("[redacted]")
			out = append(out, a)
			continue
		}
		if v, ok := a.Value.(observability.StringValue); ok {
			if len(v.Value) > effective {
				a.Value = observability.StringAttr(v.Value[:effective])
			}
		}
		out = append(out, a)
	}
	return out
}

func isSensitiveKey(key string) bool {
	lowered := strings.ToLower(key)
	for _, sub := range []string{"password", "secret", "token", "api_key", "apikey", "auth", "cookie", "session", "private_key", "privatekey"} {
		if strings.Contains(lowered, sub) {
			return true
		}
	}
	return false
}

func (m *metrics) Counter(ctx context.Context, name string, value float64, attrs ...observability.Attr) error {
	i, err := m.cache.counter(m.mp, m.scope, name)
	if err != nil {
		return err
	}

	i.Add(ctx, value, metric.WithAttributes(attrsToKeyValue(attrs, m.limit)...))

	return nil
}

func (m *metrics) Gauge(ctx context.Context, name string, value float64, attrs ...observability.Attr) error {
	i, err := m.cache.gauge(m.mp, m.scope, name)
	if err != nil {
		return err
	}

	i.Record(ctx, value, metric.WithAttributes(attrsToKeyValue(attrs, m.limit)...))

	return nil
}

func (m *metrics) Histogram(ctx context.Context, name string, value float64, attrs ...observability.Attr) error {
	i, err := m.cache.histogram(m.mp, m.scope, name)
	if err != nil {
		return err
	}

	i.Record(ctx, value, metric.WithAttributes(attrsToKeyValue(attrs, m.limit)...))

	return nil
}

func (m *metrics) Shutdown(ctx context.Context) error { return m.mp.Shutdown(ctx) }

func attrsToKeyValue(attrs []observability.Attr, limit int) []attribute.KeyValue {
	norm := normalizeAttrs(attrs, limit)

	out := make([]attribute.KeyValue, 0, len(norm))
	for _, a := range norm {
		out = append(out, toAttribute(a.Key, a.Value))
	}

	return out
}

type instrumentKind string

const (
	kindCounter   instrumentKind = "counter"
	kindGauge     instrumentKind = "gauge"
	kindHistogram instrumentKind = "histogram"
)

const maxInstruments = 1000

// DefaultGRPCTimeout bounds OTLP gRPC exporter dials and calls.
const DefaultGRPCTimeout = 10 * time.Second

// DefaultBatchTimeout bounds how long spans wait before export.
const DefaultBatchTimeout = 5 * time.Second

// DefaultExportTimeout bounds a single trace batch export.
const DefaultExportTimeout = 30 * time.Second

// DefaultMetricInterval is the periodic metric reader collection interval.
const DefaultMetricInterval = 60 * time.Second

// DefaultMetricTimeout bounds a single metric collection and export.
const DefaultMetricTimeout = 30 * time.Second

// KindMismatchError reports reuse of an instrument name for a different kind.
type KindMismatchError struct {
	// Name is the reused instrument name.
	Name string
	// Want is the requested kind.
	Want instrumentKind
	// Got is the already-registered kind.
	Got instrumentKind
}

// Error returns a human-readable description of the kind mismatch.
func (e KindMismatchError) Error() string {
	return fmt.Sprintf("otlp: instrument %q already registered as %s", e.Name, e.Got)
}

// Unwrap returns ErrInstrumentConflict for errors.Is matching.
func (e KindMismatchError) Unwrap() error { return observability.ErrInstrumentConflict }

type instrumentCache struct {
	mu         sync.RWMutex
	kinds      map[string]instrumentKind
	counters   map[string]metric.Float64Counter
	gauges     map[string]metric.Float64Gauge
	histograms map[string]metric.Float64Histogram
}

func kindMismatch(name string, want, got instrumentKind) error {
	return &KindMismatchError{Name: name, Want: want, Got: got}
}

func tooManyInstruments() error {
	return fmt.Errorf("otlp: instrument limit %d reached: %w", maxInstruments, observability.ErrTooManyInstruments)
}

func (c *instrumentCache) counter(mp *sdkmetric.MeterProvider, scope, name string) (metric.Float64Counter, error) { //nolint:dupl
	c.mu.RLock()

	if k, ok := c.kinds[name]; ok {
		if k != kindCounter {
			c.mu.RUnlock()
			return nil, kindMismatch(name, kindCounter, k)
		}

		ctr := c.counters[name]
		c.mu.RUnlock()

		return ctr, nil
	}

	c.mu.RUnlock()

	c.mu.Lock()
	defer c.mu.Unlock()

	if k, ok := c.kinds[name]; ok {
		if k != kindCounter {
			return nil, kindMismatch(name, kindCounter, k)
		}

		return c.counters[name], nil
	}

	if len(c.kinds) >= maxInstruments {
		return nil, tooManyInstruments()
	}

	i, err := mp.Meter(scope).Float64Counter(name)
	if err != nil {
		return nil, fmt.Errorf("otlp: create counter %q: %w", name, err)
	}

	c.kinds[name] = kindCounter
	c.counters[name] = i

	return i, nil
}

func (c *instrumentCache) gauge(mp *sdkmetric.MeterProvider, scope, name string) (metric.Float64Gauge, error) { //nolint:dupl
	c.mu.RLock()

	if k, ok := c.kinds[name]; ok {
		if k != kindGauge {
			c.mu.RUnlock()
			return nil, kindMismatch(name, kindGauge, k)
		}

		g := c.gauges[name]
		c.mu.RUnlock()

		return g, nil
	}

	c.mu.RUnlock()

	c.mu.Lock()
	defer c.mu.Unlock()

	if k, ok := c.kinds[name]; ok {
		if k != kindGauge {
			return nil, kindMismatch(name, kindGauge, k)
		}

		return c.gauges[name], nil
	}

	if len(c.kinds) >= maxInstruments {
		return nil, tooManyInstruments()
	}

	i, err := mp.Meter(scope).Float64Gauge(name)
	if err != nil {
		return nil, fmt.Errorf("otlp: create gauge %q: %w", name, err)
	}

	c.kinds[name] = kindGauge
	c.gauges[name] = i

	return i, nil
}

func (c *instrumentCache) histogram(mp *sdkmetric.MeterProvider, scope, name string) (metric.Float64Histogram, error) { //nolint:dupl
	c.mu.RLock()

	if k, ok := c.kinds[name]; ok {
		if k != kindHistogram {
			c.mu.RUnlock()
			return nil, kindMismatch(name, kindHistogram, k)
		}

		h := c.histograms[name]
		c.mu.RUnlock()

		return h, nil
	}

	c.mu.RUnlock()

	c.mu.Lock()
	defer c.mu.Unlock()

	if k, ok := c.kinds[name]; ok {
		if k != kindHistogram {
			return nil, kindMismatch(name, kindHistogram, k)
		}

		return c.histograms[name], nil
	}

	if len(c.kinds) >= maxInstruments {
		return nil, tooManyInstruments()
	}

	i, err := mp.Meter(scope).Float64Histogram(name)
	if err != nil {
		return nil, fmt.Errorf("otlp: create histogram %q: %w", name, err)
	}

	c.kinds[name] = kindHistogram
	c.histograms[name] = i

	return i, nil
}

// New creates an OTLP observability Provider from the given options.
func New(opts observability.Options) (observability.Provider, error) {
	if err := opts.Validate(); err != nil {
		return nil, err
	}

	endpoint := strings.TrimSpace(opts.Endpoint)
	if endpoint == "" {
		endpoint = "localhost:4317"
	}

	serviceName := strings.TrimSpace(opts.ServiceName)

	limit := opts.AttrValueLimit
	if limit <= 0 {
		limit = observability.MaxValueLen
	}

	// Validate already guarantees a usable service name, so a ServiceName-only resource build cannot fail.
	res, _ := resource.New(
		context.Background(),
		resource.WithAttributes(semconv.ServiceName(serviceName)),
	)

	traceOpts := []otlptracegrpc.Option{
		otlptracegrpc.WithEndpoint(endpoint),
		otlptracegrpc.WithTimeout(DefaultGRPCTimeout),
		otlptracegrpc.WithCompressor("gzip"),
	}
	metricOpts := []otlpmetricgrpc.Option{
		otlpmetricgrpc.WithEndpoint(endpoint),
		otlpmetricgrpc.WithTimeout(DefaultGRPCTimeout),
		otlpmetricgrpc.WithCompressor("gzip"),
	}

	if len(opts.Headers) > 0 {
		traceOpts = append(traceOpts, otlptracegrpc.WithHeaders(opts.Headers))
		metricOpts = append(metricOpts, otlpmetricgrpc.WithHeaders(opts.Headers))
	}

	switch {
	case opts.CertFile != "" || opts.KeyFile != "":
		cert, certErr := tls.LoadX509KeyPair(opts.CertFile, opts.KeyFile)
		if certErr != nil {
			return nil, fmt.Errorf("otlp: load client cert error: %w", certErr)
		}

		tlsCfg := &tls.Config{Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS13}
		if opts.CAFile != "" {
			// Validate already verified the CA file is readable with valid PEM contents.
			pemData, _ := os.ReadFile(opts.CAFile)
			pool := x509.NewCertPool()
			pool.AppendCertsFromPEM(pemData)
			tlsCfg.RootCAs = pool
		}

		creds := credentials.NewTLS(tlsCfg)
		traceOpts = append(traceOpts, otlptracegrpc.WithTLSCredentials(creds))
		metricOpts = append(metricOpts, otlpmetricgrpc.WithTLSCredentials(creds))
	case opts.CAFile != "":
		// Validate already verified the CA file is readable with valid PEM contents,
		// so NewClientTLSFromFile cannot fail here.
		creds, _ := credentials.NewClientTLSFromFile(opts.CAFile, "")

		traceOpts = append(traceOpts, otlptracegrpc.WithTLSCredentials(creds))
		metricOpts = append(metricOpts, otlpmetricgrpc.WithTLSCredentials(creds))
	case opts.Insecure:
		traceOpts = append(traceOpts, otlptracegrpc.WithInsecure())
		metricOpts = append(metricOpts, otlpmetricgrpc.WithInsecure())
	}

	hooksMu.RLock()

	traceFn := newTraceExporter
	metricFn := newMetricExporter

	hooksMu.RUnlock()

	traceExp, err := traceFn(
		context.Background(),
		traceOpts...,
	)
	if err != nil {
		return nil, fmt.Errorf("otlp: trace exporter error: %w", err)
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(
			traceExp,
			sdktrace.WithMaxQueueSize(2048),
			sdktrace.WithBatchTimeout(DefaultBatchTimeout),
			sdktrace.WithMaxExportBatchSize(512),
			sdktrace.WithExportTimeout(DefaultExportTimeout),
		),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sdktrace.ParentBased(sdktrace.TraceIDRatioBased(opts.SampleRatio))),
	)

	metricExp, err := metricFn(
		context.Background(),
		metricOpts...,
	)
	if err != nil {
		_ = tp.Shutdown(context.Background())

		return nil, fmt.Errorf("otlp: metric exporter error: %w", err)
	}

	if opts.SetGlobals {
		hooksMu.RLock()

		fn := registerGlobals

		hooksMu.RUnlock()

		fn(tp)
	}

	mp := sdkmetric.NewMeterProvider(
		sdkmetric.WithReader(sdkmetric.NewPeriodicReader(
			metricExp,
			sdkmetric.WithInterval(DefaultMetricInterval),
			sdkmetric.WithTimeout(DefaultMetricTimeout),
		)),
		sdkmetric.WithResource(res),
	)

	return &provider{
		tracer: &tracer{
			tp:     tp,
			tracer: tp.Tracer(serviceName),
		},
		metrics: &metrics{
			mp:    mp,
			scope: serviceName + "_app",
			limit: limit,
			cache: &instrumentCache{
				kinds:      make(map[string]instrumentKind),
				counters:   make(map[string]metric.Float64Counter),
				gauges:     make(map[string]metric.Float64Gauge),
				histograms: make(map[string]metric.Float64Histogram),
			},
		},
		setGlobals: opts.SetGlobals,
	}, nil
}
