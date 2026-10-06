package middleware

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"

	"github.com/zenta-dev/zever/core/observability"
	"github.com/zenta-dev/zever/shared/traceprop"
)

type traceSpanCtxKey struct{}

type traceFakeSpan struct {
	kind         observability.SpanKind
	attrs        []observability.Attr
	errors       []error
	endCalls     int
	parentTrace  trace.TraceID
	parentSpan   trace.SpanID
	parentRemote bool
}

func (s *traceFakeSpan) SetAttributes(attrs ...observability.Attr) {
	s.attrs = append(s.attrs, attrs...)
}

func (s *traceFakeSpan) RecordError(err error) { s.errors = append(s.errors, err) }
func (s *traceFakeSpan) End()                  { s.endCalls++ }

type traceFakeTracer struct {
	spans []*traceFakeSpan
	names []string
	kinds []observability.SpanKind
}

func (t *traceFakeTracer) Start(ctx context.Context, name string) (context.Context, observability.Span) {
	return t.StartSpan(ctx, name)
}

func (t *traceFakeTracer) StartSpan(ctx context.Context, name string, opts ...observability.SpanStartOption) (context.Context, observability.Span) {
	cfg := observability.NewSpanConfig(opts...)

	parent := trace.SpanContextFromContext(ctx)
	span := &traceFakeSpan{
		kind:         cfg.Kind,
		attrs:        cfg.Attrs,
		parentTrace:  parent.TraceID(),
		parentSpan:   parent.SpanID(),
		parentRemote: parent.IsRemote(),
	}
	t.spans = append(t.spans, span)
	t.names = append(t.names, name)
	t.kinds = append(t.kinds, cfg.Kind)

	return context.WithValue(ctx, traceSpanCtxKey{}, span), span
}

func (t *traceFakeTracer) Shutdown(context.Context) error { return nil }

type traceCounterCall struct {
	name  string
	value float64
	attrs []observability.Attr
}

type traceFakeMetrics struct {
	calls      []traceCounterCall
	counterErr error
}

func (m *traceFakeMetrics) Counter(_ context.Context, name string, value float64, attrs ...observability.Attr) error {
	m.calls = append(m.calls, traceCounterCall{name: name, value: value, attrs: attrs})

	return m.counterErr
}

func (m *traceFakeMetrics) Gauge(context.Context, string, float64, ...observability.Attr) error {
	return nil
}

func (m *traceFakeMetrics) Histogram(context.Context, string, float64, ...observability.Attr) error {
	return nil
}

func (m *traceFakeMetrics) Shutdown(context.Context) error { return nil }

type traceFakeProvider struct {
	tracer      *traceFakeTracer
	meter       *traceFakeMetrics
	tracerScope string
	meterScope  string
}

func (p *traceFakeProvider) Tracer(scope string) observability.Tracer {
	p.tracerScope = scope

	return p.tracer
}

func (p *traceFakeProvider) Meter(scope string) observability.Metrics {
	p.meterScope = scope

	return p.meter
}

func (p *traceFakeProvider) Shutdown(context.Context) error { return nil }

func newTraceTestProvider() (*traceFakeProvider, *traceFakeTracer, *traceFakeMetrics) {
	tracer := &traceFakeTracer{}
	meter := &traceFakeMetrics{}

	return &traceFakeProvider{tracer: tracer, meter: meter}, tracer, meter
}

func findTraceAttr(attrs []observability.Attr, key string) (observability.AttributeValue, bool) {
	for _, a := range attrs {
		if a.Key == key {
			return a.Value, true
		}
	}

	return nil, false
}

func TestTracing_spanNameIsRequestPath(t *testing.T) {
	t.Parallel()

	provider, tracer, _ := newTraceTestProvider()

	handler := Tracing(provider)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/widgets?verbose=true", nil)
	handler.ServeHTTP(httptest.NewRecorder(), req)

	if len(tracer.names) != 1 || tracer.names[0] != "/widgets" {
		t.Fatalf("span names = %v, want [/widgets] (path only, no query)", tracer.names)
	}
}

func TestTracing_methodAndPathAttributes(t *testing.T) {
	t.Parallel()

	provider, tracer, _ := newTraceTestProvider()

	handler := Tracing(provider)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/orders", nil))

	if len(tracer.spans) != 1 {
		t.Fatalf("expected one span, got %d", len(tracer.spans))
	}

	span := tracer.spans[0]

	v, ok := findTraceAttr(span.attrs, "http.method")
	if !ok || v != observability.StringAttr("POST") {
		t.Errorf("http.method attr = %v (found=%v), want POST", v, ok)
	}

	v, ok = findTraceAttr(span.attrs, "http.path")
	if !ok || v != observability.StringAttr("/orders") {
		t.Errorf("http.path attr = %v (found=%v), want /orders", v, ok)
	}

	if span.endCalls != 1 {
		t.Errorf("span.End() called %d times, want 1", span.endCalls)
	}
}

func TestTracing_statusAttributeAfterHandler(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		status int
	}{
		{name: "ok", status: http.StatusOK},
		{name: "created", status: http.StatusCreated},
		{name: "not found", status: http.StatusNotFound},
		{name: "server error", status: http.StatusInternalServerError},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			provider, tracer, _ := newTraceTestProvider()

			handler := Tracing(provider)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tt.status)
			}))

			handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil))

			v, ok := findTraceAttr(tracer.spans[0].attrs, "http.status_code")
			if !ok || v != observability.IntAttr(tt.status) {
				t.Fatalf("http.status_code attr = %v (found=%v), want %d", v, ok, tt.status)
			}
		})
	}
}

func TestTracing_serverErrorRecordsSpanError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		status  int
		wantErr bool
	}{
		{name: "500 records", status: http.StatusInternalServerError, wantErr: true},
		{name: "503 records", status: http.StatusServiceUnavailable, wantErr: true},
		{name: "499 silent", status: 499, wantErr: false},
		{name: "400 silent", status: http.StatusBadRequest, wantErr: false},
		{name: "200 silent", status: http.StatusOK, wantErr: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			provider, tracer, _ := newTraceTestProvider()

			handler := Tracing(provider)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tt.status)
			}))

			handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil))

			got := len(tracer.spans[0].errors)
			if tt.wantErr && got != 1 {
				t.Fatalf("expected 1 recorded error for status %d, got %d", tt.status, got)
			}

			if !tt.wantErr && got != 0 {
				t.Fatalf("expected no recorded error for status %d, got %v", tt.status, tracer.spans[0].errors)
			}

			if tt.wantErr {
				var se statusError
				if !errors.As(tracer.spans[0].errors[0], &se) || se.status != tt.status {
					t.Fatalf("recorded error = %v, want statusError{%d}", tracer.spans[0].errors[0], tt.status)
				}
			}
		})
	}
}

func TestTracing_requestCounter(t *testing.T) {
	t.Parallel()

	provider, _, meter := newTraceTestProvider()

	handler := Tracing(provider)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	}))

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodDelete, "/cups", nil))

	if rec.Code != http.StatusTeapot {
		t.Fatalf("handler status = %d, want 418", rec.Code)
	}

	if len(meter.calls) != 1 {
		t.Fatalf("expected 1 counter call, got %v", meter.calls)
	}

	call := meter.calls[0]
	if call.name != "http.request" {
		t.Errorf("counter name = %q, want http.request", call.name)
	}

	if call.value != 1 {
		t.Errorf("counter value = %v, want 1", call.value)
	}

	v, ok := findTraceAttr(call.attrs, "method")
	if !ok || v != observability.StringAttr("DELETE") {
		t.Errorf("method tag = %v (found=%v), want DELETE", v, ok)
	}

	v, ok = findTraceAttr(call.attrs, "status")
	if !ok || v != observability.StringAttr(http.StatusText(http.StatusTeapot)) {
		t.Errorf("status tag = %v (found=%v), want %q", v, ok, http.StatusText(http.StatusTeapot))
	}

	if len(call.attrs) != 2 {
		t.Errorf("counter attrs = %v, want exactly 2 tags", call.attrs)
	}

	if provider.tracerScope != "middleware" || provider.meterScope != "middleware" {
		t.Errorf("scopes = tracer %q meter %q, want middleware", provider.tracerScope, provider.meterScope)
	}
}

func TestTracing_meterErrorTolerated(t *testing.T) {
	t.Parallel()

	provider, _, meter := newTraceTestProvider()
	meter.counterErr = errors.New("boom")

	handler := Tracing(provider)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("handler status = %d, want 200 despite meter error", rec.Code)
	}
}

func TestTracing_propagatesSpanContext(t *testing.T) {
	t.Parallel()

	provider, _, _ := newTraceTestProvider()

	var handlerCtx context.Context

	handler := Tracing(provider)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlerCtx = r.Context()
		w.WriteHeader(http.StatusOK)
	}))

	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil))

	if handlerCtx == nil {
		t.Fatal("handler never ran")
	}

	if handlerCtx.Value(traceSpanCtxKey{}) == nil {
		t.Fatal("handler context lacks span (tracer ctx not propagated)")
	}
}

func TestTracing_panicPropagates(t *testing.T) {
	t.Parallel()

	provider, tracer, _ := newTraceTestProvider()

	handler := Tracing(provider)(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		panic("kaboom")
	}))

	defer func() {
		r := recover()
		if r != "kaboom" {
			t.Fatalf("panic = %v, want kaboom (Tracing must not recover; order Recover outside Tracing)", r)
		}

		if tracer.spans[0].endCalls != 1 {
			t.Fatalf("span.End() called %d times after panic, want 1 via defer", tracer.spans[0].endCalls)
		}
	}()

	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil))
}

func TestTracingUnaryServerInterceptor_ok(t *testing.T) {
	t.Parallel()

	provider, tracer, meter := newTraceTestProvider()
	interceptor := TracingUnaryServerInterceptor(provider)

	handler := func(ctx context.Context, req any) (any, error) {
		if ctx.Value(traceSpanCtxKey{}) == nil {
			t.Error("handler context lacks span")
		}

		return req, nil
	}

	resp, err := interceptor(t.Context(), "payload", &grpc.UnaryServerInfo{FullMethod: "/svc/Method"}, handler)
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}

	if resp != "payload" {
		t.Fatalf("resp = %v, want payload passthrough", resp)
	}

	if len(tracer.names) != 1 || tracer.names[0] != "/svc/Method" {
		t.Fatalf("span names = %v, want [/svc/Method]", tracer.names)
	}

	if tracer.spans[0].endCalls != 1 {
		t.Fatalf("span.End() called %d times, want 1", tracer.spans[0].endCalls)
	}

	if len(tracer.spans[0].errors) != 0 {
		t.Fatalf("errors = %v, want none on ok", tracer.spans[0].errors)
	}

	if len(meter.calls) != 1 || meter.calls[0].name != "rpc.request" {
		t.Fatalf("counters = %v, want one rpc.request", meter.calls)
	}

	v, ok := findTraceAttr(meter.calls[0].attrs, "method")
	if !ok || v != observability.StringAttr("/svc/Method") {
		t.Errorf("method tag = %v (found=%v), want /svc/Method", v, ok)
	}

	v, ok = findTraceAttr(meter.calls[0].attrs, "outcome")
	if !ok || v != observability.StringAttr("ok") {
		t.Errorf("outcome tag = %v (found=%v), want ok", v, ok)
	}
}

func TestTracingUnaryServerInterceptor_error(t *testing.T) {
	t.Parallel()

	provider, tracer, meter := newTraceTestProvider()
	interceptor := TracingUnaryServerInterceptor(provider)

	sentinel := errors.New("backend down")

	handler := func(context.Context, any) (any, error) { return nil, sentinel }

	resp, err := interceptor(t.Context(), "req", &grpc.UnaryServerInfo{FullMethod: "/svc/Fail"}, handler)
	if !errors.Is(err, sentinel) {
		t.Fatalf("err = %v, want %v", err, sentinel)
	}

	if resp != nil {
		t.Fatalf("resp = %v, want nil passthrough", resp)
	}

	if len(tracer.spans[0].errors) != 1 || !errors.Is(tracer.spans[0].errors[0], sentinel) {
		t.Fatalf("span errors = %v, want [sentinel]", tracer.spans[0].errors)
	}

	v, ok := findTraceAttr(meter.calls[0].attrs, "outcome")
	if !ok || v != observability.StringAttr("error") {
		t.Fatalf("outcome tag = %v (found=%v), want error", v, ok)
	}
}

func TestTracing_extractsInboundTraceContext(t *testing.T) {
	t.Parallel()

	provider, tracer, _ := newTraceTestProvider()

	handler := Tracing(provider)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	traceID, err := trace.TraceIDFromHex("4bf92f3577b34da6a3ce929d0e0e4736")
	if err != nil {
		t.Fatalf("TraceIDFromHex: %v", err)
	}

	spanID, err := trace.SpanIDFromHex("00f067aa0ba902b7")
	if err != nil {
		t.Fatalf("SpanIDFromHex: %v", err)
	}

	ctx := trace.ContextWithSpanContext(t.Context(), trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    traceID,
		SpanID:     spanID,
		TraceFlags: trace.FlagsSampled,
	}))

	hdrs := traceprop.Inject(ctx, nil)

	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/widgets", nil)
	for k, v := range hdrs {
		req.Header.Set(k, v)
	}
	handler.ServeHTTP(rec, req)

	if len(tracer.spans) != 1 {
		t.Fatalf("spans = %d, want 1", len(tracer.spans))
	}

	span := tracer.spans[0]

	if span.parentTrace != traceID {
		t.Errorf("parent trace ID = %s, want %s (child of inbound span)", span.parentTrace, traceID)
	}

	if span.parentSpan != spanID {
		t.Errorf("parent span ID = %s, want %s", span.parentSpan, spanID)
	}

	if !span.parentRemote {
		t.Error("parent remote = false, want true (extracted context is remote)")
	}

	if span.kind != observability.SpanKindServer {
		t.Errorf("span kind = %v, want SpanKindServer", span.kind)
	}
}

func TestTracing_noInboundTraceStartsRoot(t *testing.T) {
	t.Parallel()

	provider, tracer, _ := newTraceTestProvider()

	handler := Tracing(provider)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil))

	if len(tracer.spans) != 1 {
		t.Fatalf("spans = %d, want 1", len(tracer.spans))
	}

	if tracer.spans[0].parentRemote {
		t.Error("parent remote = true, want false (no inbound traceparent)")
	}
}

func TestTracing_baggagePropagatedToHandler(t *testing.T) {
	t.Parallel()

	provider, _, _ := newTraceTestProvider()

	var handlerBaggage string

	handler := Tracing(provider)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlerBaggage = traceprop.Baggage(r.Context(), traceprop.BaggageTenantID)
		w.WriteHeader(http.StatusOK)
	}))

	traceID, err := trace.TraceIDFromHex("4bf92f3577b34da6a3ce929d0e0e4736")
	if err != nil {
		t.Fatalf("TraceIDFromHex: %v", err)
	}

	spanID, err := trace.SpanIDFromHex("00f067aa0ba902b7")
	if err != nil {
		t.Fatalf("SpanIDFromHex: %v", err)
	}

	ctx := trace.ContextWithSpanContext(t.Context(), trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    traceID,
		SpanID:     spanID,
		TraceFlags: trace.FlagsSampled,
	}))
	ctx = traceprop.WithBaggage(ctx, traceprop.BaggageTenantID, "acme")

	hdrs := traceprop.Inject(ctx, nil)

	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil)
	for k, v := range hdrs {
		req.Header.Set(k, v)
	}
	handler.ServeHTTP(rec, req)

	if handlerBaggage != "acme" {
		t.Errorf("handler baggage tenant.id = %q, want %q", handlerBaggage, "acme")
	}
}

func TestTracingUnaryServerInterceptor_extractsInboundTraceContext(t *testing.T) {
	t.Parallel()

	provider, tracer, _ := newTraceTestProvider()
	interceptor := TracingUnaryServerInterceptor(provider)

	traceID, err := trace.TraceIDFromHex("4bf92f3577b34da6a3ce929d0e0e4736")
	if err != nil {
		t.Fatalf("TraceIDFromHex: %v", err)
	}

	spanID, err := trace.SpanIDFromHex("00f067aa0ba902b7")
	if err != nil {
		t.Fatalf("SpanIDFromHex: %v", err)
	}

	ctx := metadata.NewIncomingContext(t.Context(), metadata.Pairs(
		"traceparent", "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01",
	))
	_ = traceID
	_ = spanID

	handler := func(context.Context, any) (any, error) { return "ok", nil }

	_, err = interceptor(ctx, "req", &grpc.UnaryServerInfo{FullMethod: "/svc/Method"}, handler)
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}

	if len(tracer.spans) != 1 {
		t.Fatalf("spans = %d, want 1", len(tracer.spans))
	}

	span := tracer.spans[0]

	if span.parentTrace != traceID {
		t.Errorf("parent trace ID = %s, want %s", span.parentTrace, traceID)
	}

	if !span.parentRemote {
		t.Error("parent remote = false, want true")
	}

	if span.kind != observability.SpanKindServer {
		t.Errorf("span kind = %v, want SpanKindServer", span.kind)
	}
}

func TestTracing_e2ePropagation(t *testing.T) {
	t.Parallel()

	provider, tracer, _ := newTraceTestProvider()

	srv := httptest.NewServer(Tracing(provider)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})))
	defer srv.Close()

	traceID, err := trace.TraceIDFromHex("4bf92f3577b34da6a3ce929d0e0e4736")
	if err != nil {
		t.Fatalf("TraceIDFromHex: %v", err)
	}

	spanID, err := trace.SpanIDFromHex("00f067aa0ba902b7")
	if err != nil {
		t.Fatalf("SpanIDFromHex: %v", err)
	}

	ctx := trace.ContextWithSpanContext(t.Context(), trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    traceID,
		SpanID:     spanID,
		TraceFlags: trace.FlagsSampled,
	}))

	hdrs := traceprop.Inject(ctx, nil)

	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, srv.URL+"/widgets", nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}

	for k, v := range hdrs {
		req.Header.Set(k, v)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	resp.Body.Close()

	if len(tracer.spans) != 1 {
		t.Fatalf("spans = %d, want 1", len(tracer.spans))
	}

	if tracer.spans[0].parentTrace != traceID {
		t.Errorf("server span parent trace ID = %s, want %s (e2e propagation)", tracer.spans[0].parentTrace, traceID)
	}
}

func TestStatusError_Error(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		status int
		want   string
	}{
		{name: "internal", status: http.StatusInternalServerError, want: "Internal Server Error"},
		{name: "bad gateway", status: http.StatusBadGateway, want: "Bad Gateway"},
		{name: "unknown empty", status: 599, want: ""},
		{name: "zero empty", status: 0, want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := (statusError{tt.status}).Error(); got != tt.want {
				t.Errorf("statusError{%d}.Error() = %q, want %q", tt.status, got, tt.want)
			}
		})
	}
}
