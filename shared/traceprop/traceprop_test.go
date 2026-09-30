package traceprop

import (
	"context"
	"strings"
	"sync"
	"testing"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
)

func mustTraceID(t *testing.T) trace.TraceID {
	t.Helper()

	const hex = "4bf92f3577b34da6a3ce929d0e0e4736"

	id, err := trace.TraceIDFromHex(hex)
	if err != nil {
		t.Fatalf("TraceIDFromHex(%q): %v", hex, err)
	}

	return id
}

func mustSpanID(t *testing.T) trace.SpanID {
	t.Helper()

	const hex = "00f067aa0ba902b7"

	id, err := trace.SpanIDFromHex(hex)
	if err != nil {
		t.Fatalf("SpanIDFromHex(%q): %v", hex, err)
	}

	return id
}

func ctxWithSpanContext(
	ctx context.Context,
	traceID trace.TraceID,
	spanID trace.SpanID,
	sampled bool,
) context.Context {
	var flags byte
	if sampled {
		flags = 0x01
	}

	return trace.ContextWithSpanContext(ctx, trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    traceID,
		SpanID:     spanID,
		TraceFlags: trace.TraceFlags(flags),
	}))
}

func spanContextOf(ctx context.Context) trace.SpanContext {
	return trace.SpanContextFromContext(ctx)
}

func TestInjectExtract_RoundTrip(t *testing.T) {
	t.Parallel()

	traceID := mustTraceID(t)
	spanID := mustSpanID(t)

	ctx := ctxWithSpanContext(t.Context(), traceID, spanID, true)
	out := Inject(ctx, map[string]string{"k": "v"})

	if out["k"] != "v" {
		t.Errorf("Inject dropped existing header: %q", out)
	}

	got, want := out[TraceParentHeader], "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01"
	if got != want {
		t.Errorf("traceparent = %q, want %q", got, want)
	}

	back := spanContextOf(Extract(t.Context(), out))
	if back.TraceID() != traceID {
		t.Errorf("TraceID = %s, want %s", back.TraceID(), traceID)
	}
	if back.SpanID() != spanID {
		t.Errorf("SpanID = %s, want %s", back.SpanID(), spanID)
	}
	if !back.IsSampled() {
		t.Error("IsSampled = false, want true")
	}
	if !back.IsRemote() {
		t.Error("IsRemote = false, want true (extracted context is remote)")
	}
}

func TestInjectExtract_SampledFlag(t *testing.T) {
	t.Parallel()

	traceID := mustTraceID(t)
	spanID := mustSpanID(t)

	tests := []struct {
		name    string
		sampled bool
		flag    string
	}{
		{"sampled", true, "-01"},
		{"unsampled", false, "-00"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx := ctxWithSpanContext(t.Context(), traceID, spanID, tc.sampled)
			out := Inject(ctx, nil)

			if !strings.HasSuffix(out[TraceParentHeader], tc.flag) {
				t.Errorf("traceparent = %q, want suffix %q", out[TraceParentHeader], tc.flag)
			}

			if got := spanContextOf(Extract(t.Context(), out)).IsSampled(); got != tc.sampled {
				t.Errorf("IsSampled = %v, want %v", got, tc.sampled)
			}
		})
	}
}

func TestInjectExtract_Tracestate(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	ts, err := trace.ParseTraceState("rojo=00f067aa0ba902b7")
	if err != nil {
		t.Fatalf("ParseTraceState: %v", err)
	}

	ctx = trace.ContextWithSpanContext(ctx, trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    mustTraceID(t),
		SpanID:     mustSpanID(t),
		TraceFlags: 0x01,
		TraceState: ts,
	}))

	out := Inject(ctx, nil)
	if out[TraceStateHeader] == "" {
		t.Fatalf("tracestate missing in %q", out)
	}

	back := spanContextOf(Extract(t.Context(), out))
	if got := back.TraceState().Get("rojo"); got != "00f067aa0ba902b7" {
		t.Errorf("tracestate rojo = %q, want %q", got, "00f067aa0ba902b7")
	}
}

func TestInject_NoSpan(t *testing.T) {
	t.Parallel()

	in := map[string]string{"k": "v"}
	out := Inject(t.Context(), in)

	if _, ok := out[TraceParentHeader]; ok {
		t.Errorf("traceparent injected without span: %q", out)
	}
	if len(out) != 1 || out["k"] != "v" {
		t.Errorf("Inject altered headers: %q", out)
	}
	if out == nil {
		t.Error("Inject returned nil for non-nil input")
	}

	if out := Inject(t.Context(), nil); out != nil {
		t.Errorf("Inject(nil) = %q, want nil", out)
	}
}

func TestInject_DoesNotMutateInput(t *testing.T) {
	t.Parallel()

	traceID := mustTraceID(t)
	spanID := mustSpanID(t)

	in := map[string]string{"k": "v"}
	Inject(ctxWithSpanContext(t.Context(), traceID, spanID, true), in)

	if len(in) != 1 {
		t.Errorf("input mutated: %q", in)
	}
}

func TestExtract_EmptyAndInvalid(t *testing.T) {
	t.Parallel()

	for _, headers := range []map[string]string{nil, {}, {"traceparent": "bogus"}} {
		back := spanContextOf(Extract(t.Context(), headers))
		if back.IsValid() {
			t.Errorf("Extract(%q) valid = %s, want invalid", headers, back.TraceID())
		}
	}
}

// stubSpan records the parent context it was started from.
type stubSpan struct {
	trace.Span
	parent trace.SpanContext
	self   trace.SpanContext
	mu     sync.Mutex
	ended  bool
}

// SpanContext returns the stub's own span context.
func (s *stubSpan) SpanContext() trace.SpanContext { return s.self }

func (s *stubSpan) End(...trace.SpanEndOption) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ended = true
}

type stubTracer struct {
	trace.Tracer
	last *stubSpan
}

func (tr *stubTracer) Start(ctx context.Context, _ string, _ ...trace.SpanStartOption) (context.Context, trace.Span) {
	parent := spanContextOf(ctx)
	span := &stubSpan{
		parent: parent,
		self: trace.NewSpanContext(trace.SpanContextConfig{
			TraceID:    parent.TraceID(),
			SpanID:     trace.SpanID{0x1},
			TraceFlags: parent.TraceFlags(),
		}),
	}
	tr.last = span

	return trace.ContextWithSpan(ctx, span), span
}

func (tr *stubTracer) Enabled() bool { return true }

type stubProvider struct {
	trace.TracerProvider
	tracer *stubTracer
}

func (p *stubProvider) Tracer(_ string, _ ...trace.TracerOption) trace.Tracer {
	return p.tracer
}

func TestStartConsumeSpan_LinksRemoteParent(t *testing.T) {
	t.Parallel()

	traceID := mustTraceID(t)
	spanID := mustSpanID(t)

	headers := Inject(ctxWithSpanContext(t.Context(), traceID, spanID, true), nil)

	st := &stubTracer{}
	ctx, span := startConsumeSpanWithTracer(t.Context(), headers, st, "test.consume")
	defer span.End()

	stub, ok := span.(*stubSpan)
	if !ok {
		t.Fatalf("span = %T, want *stubSpan", span)
	}

	if stub.parent.TraceID() != traceID || stub.parent.SpanID() != spanID {
		t.Errorf("parent = %s/%s, want %s/%s", stub.parent.TraceID(), stub.parent.SpanID(), traceID, spanID)
	}

	if got := spanContextOf(ctx); !got.IsValid() {
		t.Error("returned ctx holds invalid span context")
	}
}

// startConsumeSpanWithTracer starts a consume span with an explicit tracer.
// It is test-only: it lets tests observe the started span without relying
// on the global tracer provider.
func startConsumeSpanWithTracer(
	ctx context.Context,
	headers map[string]string,
	tracer trace.Tracer,
	spanName string,
	opts ...trace.SpanStartOption,
) (context.Context, trace.Span) {
	ctx = Extract(ctx, headers)
	opts = append([]trace.SpanStartOption{trace.WithSpanKind(trace.SpanKindConsumer)}, opts...)

	//nolint:spancheck // helper returns the span to the test, which owns calling End(); standard tracer.Start contract, not a leak
	return tracer.Start(ctx, spanName, opts...)
}

func TestStartConsumeSpan_EndsCleanly(t *testing.T) {
	t.Parallel()

	prev := otel.GetTracerProvider()
	otel.SetTracerProvider(&stubProvider{tracer: &stubTracer{}})
	t.Cleanup(func() { otel.SetTracerProvider(prev) })

	_, span := StartConsumeSpan(t.Context(), nil, "test.noop")
	span.End()
}
