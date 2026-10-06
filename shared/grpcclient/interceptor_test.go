package grpcclient

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"go.opentelemetry.io/otel/trace"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/zenta-dev/zever/core/observability"
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

// attrStringValue returns the string value of key in attrs, or "" when the
// key is absent or not a string attribute.
func attrStringValue(attrs []observability.Attr, key string) string {
	for _, a := range attrs {
		if a.Key != key {
			continue
		}
		if v, ok := a.Value.(observability.StringValue); ok {
			return v.Value
		}
	}

	return ""
}

// attrIntValue returns the int value of key in attrs, and whether the key
// was present as an int attribute.
func attrIntValue(attrs []observability.Attr, key string) (int64, bool) {
	for _, a := range attrs {
		if a.Key != key {
			continue
		}
		if v, ok := a.Value.(observability.Int64Value); ok {
			return v.Value, true
		}
	}

	return 0, false
}

// TestUnaryInterceptorSpanSemconvAttributes pins the RPC attributes on the
// unary client span: rpc.system, server.address, rpc.service, rpc.method,
// and rpc.grpc.status_code on failure.
func TestUnaryInterceptorSpanSemconvAttributes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		err       error
		wantCode  codes.Code
		wantIsSet bool
	}{
		{name: "success omits status code"},
		{
			name:      "failure records status code",
			err:       status.Error(codes.Unavailable, "down"),
			wantCode:  codes.Unavailable,
			wantIsSet: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			tr := &stubTracer{}
			cfg := &config{observability: &stubProvider{tracer: tr}}
			invoker, _ := captureInvoker(tt.err)

			err := cfg.unaryInterceptor()(t.Context(), "/pkg.Svc/DoThing", nil, nil, nil, invoker)
			if !errors.Is(err, tt.err) && (err == nil) != (tt.err == nil) {
				t.Fatalf("err = %v, want %v", err, tt.err)
			}

			if len(tr.spans) != 1 {
				t.Fatalf("spans = %d, want 1", len(tr.spans))
			}

			attrs := tr.spans[0].attrs

			if got := attrStringValue(attrs, observability.RPCSystem); got != observability.SystemGRPC {
				t.Errorf("%s = %q, want %q", observability.RPCSystem, got, observability.SystemGRPC)
			}
			if got := attrStringValue(attrs, observability.RPCService); got != "pkg.Svc" {
				t.Errorf("%s = %q, want pkg.Svc", observability.RPCService, got)
			}
			if got := attrStringValue(attrs, observability.RPCMethod); got != "DoThing" {
				t.Errorf("%s = %q, want DoThing", observability.RPCMethod, got)
			}

			for _, a := range attrs {
				if a.Key == observability.ServerAddress {
					t.Error("server.address present without a connection, want absent")
				}
			}

			got, ok := attrIntValue(attrs, observability.RPCGRPCStatusCode)
			if ok != tt.wantIsSet {
				t.Fatalf("%s present = %v, want %v", observability.RPCGRPCStatusCode, ok, tt.wantIsSet)
			}
			if tt.wantIsSet && got != int64(tt.wantCode) {
				t.Errorf("%s = %d, want %d", observability.RPCGRPCStatusCode, got, tt.wantCode)
			}
		})
	}
}

// TestUnaryInterceptorSpanSemconvAttributesThroughRealConn pins that
// server.address comes from the connection's dial target.
func TestUnaryInterceptorSpanSemconvAttributesThroughRealConn(t *testing.T) {
	t.Parallel()

	conn, err := New(t.Context(), "dns:///localhost:50051", WithInsecure())
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer conn.Close()

	tr := &stubTracer{}
	cfg := &config{observability: &stubProvider{tracer: tr}}
	invoker, _ := captureInvoker(nil)

	if err := cfg.unaryInterceptor()(t.Context(), "/pkg.Svc/DoThing", nil, nil, conn, invoker); err != nil {
		t.Fatalf("interceptor: %v", err)
	}

	if len(tr.spans) != 1 {
		t.Fatalf("spans = %d, want 1", len(tr.spans))
	}

	if got := attrStringValue(tr.spans[0].attrs, observability.ServerAddress); got != conn.Target() {
		t.Errorf("%s = %q, want %q", observability.ServerAddress, got, conn.Target())
	}
}

// TestStreamInterceptorSpanSemconvAttributes pins the same attributes plus
// rpc.grpc.status_code on the stream path.
func TestStreamInterceptorSpanSemconvAttributes(t *testing.T) {
	t.Parallel()

	tr := &stubTracer{}
	cfg := &config{observability: &stubProvider{tracer: tr}}
	streamer := func(context.Context, *grpc.StreamDesc, *grpc.ClientConn, string, ...grpc.CallOption) (grpc.ClientStream, error) {
		return nil, status.Error(codes.Internal, "boom")
	}

	if _, err := cfg.streamInterceptor()(t.Context(), &grpc.StreamDesc{}, nil, "/pkg.Svc/Stream", streamer); err == nil {
		t.Fatal("streamer error = nil, want Internal")
	}

	if len(tr.spans) != 1 {
		t.Fatalf("spans = %d, want 1", len(tr.spans))
	}

	attrs := tr.spans[0].attrs

	if got := attrStringValue(attrs, observability.RPCService); got != "pkg.Svc" {
		t.Errorf("%s = %q, want pkg.Svc", observability.RPCService, got)
	}
	if got := attrStringValue(attrs, observability.RPCMethod); got != "Stream" {
		t.Errorf("%s = %q, want Stream", observability.RPCMethod, got)
	}
	for _, a := range attrs {
		if a.Key == observability.ServerAddress {
			t.Error("server.address present without a connection, want absent")
		}
	}
	if got, ok := attrIntValue(attrs, observability.RPCGRPCStatusCode); !ok || got != int64(codes.Internal) {
		t.Errorf("%s = (%d, %v), want (%d, true)", observability.RPCGRPCStatusCode, got, ok, codes.Internal)
	}
}

// TestSplitFullMethod covers the gRPC full-method parser, including the
// malformed shapes a non-gRPC caller can hand it.
func TestSplitFullMethod(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		full        string
		wantService string
		wantMethod  string
	}{
		{name: "package qualified", full: "/pkg.Svc/Method", wantService: "pkg.Svc", wantMethod: "Method"},
		{name: "bare service", full: "/Svc/Method", wantService: "Svc", wantMethod: "Method"},
		{name: "empty", full: "", wantService: "", wantMethod: ""},
		{name: "no slash", full: "Method", wantService: "", wantMethod: "Method"},
		{name: "bare slash", full: "/", wantService: "", wantMethod: ""},
		{name: "trailing slash", full: "/Svc/", wantService: "Svc", wantMethod: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			svc, method := splitFullMethod(tt.full)
			if svc != tt.wantService || method != tt.wantMethod {
				t.Errorf("splitFullMethod(%q) = (%q, %q), want (%q, %q)",
					tt.full, svc, method, tt.wantService, tt.wantMethod)
			}
		})
	}
}

// TestDialTargetNilConn pins that a nil connection yields an empty target
// rather than panicking.
func TestDialTargetNilConn(t *testing.T) {
	t.Parallel()

	if got := dialTarget(nil); got != "" {
		t.Errorf("dialTarget(nil) = %q, want empty", got)
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
