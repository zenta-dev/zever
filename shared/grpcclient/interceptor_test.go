package grpcclient

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"go.opentelemetry.io/otel/trace"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"

	"github.com/zenta-dev/zever/core/resilience"
	"github.com/zenta-dev/zever/shared/traceprop"
)

// stubGuard is a test double resilience.Guard recording Execute calls.
type stubGuard struct {
	calls int
	err   error
}

// Execute records the call and runs fn, returning its error.
func (g *stubGuard) Execute(ctx context.Context, fn func(context.Context) error) error {
	g.calls++
	if g.err != nil {
		return g.err
	}
	return fn(ctx)
}

// State reports StateClosed.
func (g *stubGuard) State() resilience.State { return resilience.StateClosed }

// Name returns the stub name.
func (g *stubGuard) Name() string { return "stub" }

// Close is a no-op.
func (g *stubGuard) Close() error { return nil }

// ctxWithSpanContext returns ctx carrying a valid sampled span context.
func ctxWithSpanContext(ctx context.Context) context.Context {
	return trace.ContextWithSpanContext(ctx, trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    trace.TraceID{0x01},
		SpanID:     trace.SpanID{0x01},
		TraceFlags: trace.FlagsSampled,
	}))
}

// captureInvoker returns a UnaryInvoker that records its ctx.
func captureInvoker(err error) (grpc.UnaryInvoker, *context.Context) {
	var captured context.Context
	invoker := func(ctx context.Context, _ string, _, _ any, _ *grpc.ClientConn, _ ...grpc.CallOption) error {
		captured = ctx
		return err
	}
	return invoker, &captured
}

// stubClientStream is a test double grpc.ClientStream.
type stubClientStream struct{}

// Header returns nil.
func (stubClientStream) Header() (metadata.MD, error) { return metadata.MD{}, nil }

// Trailer returns nil.
func (stubClientStream) Trailer() metadata.MD { return nil }

// CloseSend is a no-op.
func (stubClientStream) CloseSend() error { return nil }

// Context returns a background context.
func (stubClientStream) Context() context.Context { return context.Background() }

// SendMsg is a no-op.
func (stubClientStream) SendMsg(any) error { return nil }

// RecvMsg is a no-op.
func (stubClientStream) RecvMsg(any) error { return nil }

func TestUnaryInterceptorInjectsTraceParent(t *testing.T) {
	t.Parallel()

	invoker, captured := captureInvoker(nil)
	cfg := &config{}

	err := cfg.unaryInterceptor()(ctxWithSpanContext(t.Context()), "/svc/Method", nil, nil, nil, invoker)
	if err != nil {
		t.Fatalf("interceptor: %v", err)
	}

	md, ok := metadata.FromOutgoingContext(*captured)
	if !ok {
		t.Fatal("no outgoing metadata, want traceparent injected")
	}
	tp := md.Get(traceprop.TraceParentHeader)
	if len(tp) != 1 {
		t.Fatalf("traceparent values = %v, want exactly 1", tp)
	}
	want := "00-01000000000000000000000000000000-0100000000000000-01"
	if tp[0] != want {
		t.Fatalf("traceparent = %q, want %q", tp[0], want)
	}
}

func TestUnaryInterceptorNoSpanNoTraceParent(t *testing.T) {
	t.Parallel()

	invoker, captured := captureInvoker(nil)
	cfg := &config{}

	if err := cfg.unaryInterceptor()(t.Context(), "/svc/Method", nil, nil, nil, invoker); err != nil {
		t.Fatalf("interceptor: %v", err)
	}

	if md, ok := metadata.FromOutgoingContext(*captured); ok {
		if _, exists := md[traceprop.TraceParentHeader]; exists {
			t.Fatal("traceparent injected without a span in ctx")
		}
	}
}

func TestUnaryInterceptorPropagatesMetadata(t *testing.T) {
	t.Parallel()

	invoker, captured := captureInvoker(nil)
	cfg := &config{metadata: map[string]string{"tenant": "acme", "version": "v2"}}

	if err := cfg.unaryInterceptor()(t.Context(), "/svc/Method", nil, nil, nil, invoker); err != nil {
		t.Fatalf("interceptor: %v", err)
	}

	md, ok := metadata.FromOutgoingContext(*captured)
	if !ok {
		t.Fatal("no outgoing metadata, want WithMetadata keys")
	}
	if got := md.Get("tenant"); len(got) != 1 || got[0] != "acme" {
		t.Errorf("tenant = %v, want [acme]", got)
	}
	if got := md.Get("version"); len(got) != 1 || got[0] != "v2" {
		t.Errorf("version = %v, want [v2]", got)
	}
}

func TestUnaryInterceptorMetadataDoesNotOverwrite(t *testing.T) {
	t.Parallel()

	invoker, captured := captureInvoker(nil)
	ctx := metadata.AppendToOutgoingContext(t.Context(), "tenant", "original")
	cfg := &config{metadata: map[string]string{"tenant": "override", "other": "x"}}

	if err := cfg.unaryInterceptor()(ctx, "/svc/Method", nil, nil, nil, invoker); err != nil {
		t.Fatalf("interceptor: %v", err)
	}

	md, _ := metadata.FromOutgoingContext(*captured)
	if got := md.Get("tenant"); len(got) != 1 || got[0] != "original" {
		t.Errorf("tenant = %v, want [original] (not overwritten)", got)
	}
	if got := md.Get("other"); len(got) != 1 || got[0] != "x" {
		t.Errorf("other = %v, want [x]", got)
	}
}

func TestUnaryInterceptorAppliesTimeout(t *testing.T) {
	t.Parallel()

	cfg := &config{timeout: 25 * time.Millisecond, timeoutSet: true}
	invoker := func(ctx context.Context, _ string, _, _ any, _ *grpc.ClientConn, _ ...grpc.CallOption) error {
		<-ctx.Done()
		return ctx.Err()
	}

	err := cfg.unaryInterceptor()(t.Context(), "/svc/Method", nil, nil, nil, invoker)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("error = %v, want context.DeadlineExceeded", err)
	}
}

func TestUnaryInterceptorNoTimeoutWithoutOption(t *testing.T) {
	t.Parallel()

	invoker, captured := captureInvoker(nil)
	cfg := &config{}

	if err := cfg.unaryInterceptor()(t.Context(), "/svc/Method", nil, nil, nil, invoker); err != nil {
		t.Fatalf("interceptor: %v", err)
	}
	if _, deadline := (*captured).Deadline(); deadline {
		t.Fatal("deadline set without WithTimeout")
	}
}

func TestUnaryInterceptorRoutesThroughGuard(t *testing.T) {
	t.Parallel()

	g := &stubGuard{}
	cfg := &config{guard: g}
	wantErr := errors.New("boom")
	invoker, _ := captureInvoker(wantErr)

	err := cfg.unaryInterceptor()(t.Context(), "/svc/Method", nil, nil, nil, invoker)
	if g.calls != 1 {
		t.Fatalf("guard Execute calls = %d, want 1", g.calls)
	}
	if !errors.Is(err, wantErr) {
		t.Fatalf("error = %v, want %v", err, wantErr)
	}
}

func TestUnaryInterceptorGuardErrorShortCircuits(t *testing.T) {
	t.Parallel()

	g := &stubGuard{err: errors.New("breaker open")}
	cfg := &config{guard: g}
	called := false
	invoker := func(_ context.Context, _ string, _, _ any, _ *grpc.ClientConn, _ ...grpc.CallOption) error {
		called = true
		return nil
	}

	err := cfg.unaryInterceptor()(t.Context(), "/svc/Method", nil, nil, nil, invoker)
	if g.calls != 1 {
		t.Fatalf("guard Execute calls = %d, want 1", g.calls)
	}
	if called {
		t.Fatal("invoker called despite guard error")
	}
	if err == nil || !strings.Contains(err.Error(), "breaker open") {
		t.Fatalf("error = %v, want breaker open", err)
	}
}

func TestUnaryInterceptorNilGuard(t *testing.T) {
	t.Parallel()

	cfg := &config{}
	invoker, _ := captureInvoker(nil)

	if err := cfg.unaryInterceptor()(t.Context(), "/svc/Method", nil, nil, nil, invoker); err != nil {
		t.Fatalf("interceptor: %v", err)
	}
}

func TestStreamInterceptorGuardsEstablishment(t *testing.T) {
	t.Parallel()

	g := &stubGuard{}
	cfg := &config{guard: g}
	wantErr := errors.New("boom")
	streamer := func(_ context.Context, _ *grpc.StreamDesc, _ *grpc.ClientConn, _ string, _ ...grpc.CallOption) (grpc.ClientStream, error) {
		return nil, wantErr
	}

	_, err := cfg.streamInterceptor()(t.Context(), &grpc.StreamDesc{}, nil, "/svc/Method", streamer)
	if g.calls != 1 {
		t.Fatalf("guard Execute calls = %d, want 1", g.calls)
	}
	if !errors.Is(err, wantErr) {
		t.Fatalf("error = %v, want %v", err, wantErr)
	}
}

func TestStreamInterceptorMetadataAndTrace(t *testing.T) {
	t.Parallel()

	var captured context.Context
	streamer := func(ctx context.Context, _ *grpc.StreamDesc, _ *grpc.ClientConn, _ string, _ ...grpc.CallOption) (grpc.ClientStream, error) {
		captured = ctx
		return stubClientStream{}, nil
	}
	cfg := &config{metadata: map[string]string{"tenant": "acme"}}

	if _, err := cfg.streamInterceptor()(ctxWithSpanContext(t.Context()), &grpc.StreamDesc{}, nil, "/svc/Method", streamer); err != nil {
		t.Fatalf("interceptor: %v", err)
	}

	md, ok := metadata.FromOutgoingContext(captured)
	if !ok {
		t.Fatal("no outgoing metadata")
	}
	if got := md.Get("traceparent"); len(got) != 1 {
		t.Errorf("traceparent = %v, want 1 value", got)
	}
	if got := md.Get("tenant"); len(got) != 1 || got[0] != "acme" {
		t.Errorf("tenant = %v, want [acme]", got)
	}
}

func TestStreamInterceptorNilGuard(t *testing.T) {
	t.Parallel()

	called := false
	streamer := func(_ context.Context, _ *grpc.StreamDesc, _ *grpc.ClientConn, _ string, _ ...grpc.CallOption) (grpc.ClientStream, error) {
		called = true
		return stubClientStream{}, nil
	}
	cfg := &config{}

	if _, err := cfg.streamInterceptor()(t.Context(), &grpc.StreamDesc{}, nil, "/svc/Method", streamer); err != nil {
		t.Fatalf("interceptor: %v", err)
	}
	if !called {
		t.Fatal("streamer not called with nil guard")
	}
}
