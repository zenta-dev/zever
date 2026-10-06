package grpcclient

import (
	"context"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/zenta-dev/zever/core/observability"
	"github.com/zenta-dev/zever/shared/traceprop"
)

// scopeName is the observability scope for client spans started by the
// interceptors.
const scopeName = "github.com/zenta-dev/zever/shared/grpcclient"

// unaryInterceptor returns the chained unary client interceptor. Order,
// outermost to innermost: observability span, W3C trace injection,
// WithMetadata keys, WithTimeout deadline, WithGuard execution.
func (c *config) unaryInterceptor() grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		ctx, span := c.startSpan(ctx, method, dialTarget(cc))
		defer span.End()

		ctx = c.attachMetadata(ctx)

		if c.timeoutSet {
			var cancel context.CancelFunc
			ctx, cancel = context.WithTimeout(ctx, c.timeout)
			defer cancel()
		}

		err := c.execute(ctx, func(ctx context.Context) error {
			return invoker(ctx, method, req, reply, cc, opts...)
		})
		recordRPCStatus(span, err)

		return err
	}
}

// streamInterceptor returns the chained stream client interceptor with the
// same ordering as unaryInterceptor. The guard wraps stream establishment
// only: messages exchanged after the stream is established are not
// guarded.
func (c *config) streamInterceptor() grpc.StreamClientInterceptor {
	return func(ctx context.Context, desc *grpc.StreamDesc, cc *grpc.ClientConn, method string, streamer grpc.Streamer, opts ...grpc.CallOption) (grpc.ClientStream, error) {
		ctx, span := c.startSpan(ctx, method, dialTarget(cc))
		defer span.End()

		ctx = c.attachMetadata(ctx)

		if c.timeoutSet {
			var cancel context.CancelFunc
			ctx, cancel = context.WithTimeout(ctx, c.timeout)
			defer cancel()
		}

		var stream grpc.ClientStream

		err := c.execute(ctx, func(ctx context.Context) error {
			var execErr error
			stream, execErr = streamer(ctx, desc, cc, method, opts...)

			return execErr
		})
		recordRPCStatus(span, err)

		return stream, err
	}
}

// startSpan starts a SpanKindClient span when an observability Provider is
// configured; otherwise it returns ctx unchanged with a noopSpan. The span
// carries the OpenTelemetry semantic-convention RPC attributes:
// rpc.system, server.address, rpc.service, and rpc.method (see
// observability.RPCSystem, observability.ServerAddress,
// observability.RPCService, observability.RPCMethod). addr is the dial
// target and may be empty when the caller has no connection.
//
// The kind and the attributes are passed as start options so a Tracer
// implementing the optional observability.SpanStarter records them at span
// creation. observability.StartSpan drops the options entirely for a Tracer
// that does not implement SpanStarter, so the attribute set is also applied
// explicitly on the returned span; setting the same key twice is idempotent
// for the backends that keep both.
func (c *config) startSpan(ctx context.Context, method, addr string) (context.Context, observability.Span) {
	if c.observability == nil {
		return ctx, noopSpan{}
	}

	svc, m := splitFullMethod(method)

	attrs := []observability.Attr{
		observability.String(observability.RPCSystem, observability.SystemGRPC),
		observability.String(observability.RPCService, svc),
		observability.String(observability.RPCMethod, m),
	}
	if addr != "" {
		attrs = append(attrs, observability.String(observability.ServerAddress, addr))
	}

	ctx, span := observability.StartSpan(ctx, c.observability.Tracer(scopeName), method,
		observability.WithSpanKind(observability.SpanKindClient),
		observability.WithAttributes(attrs...),
	)
	span.SetAttributes(attrs...)

	return ctx, span
}

// dialTarget returns cc's dial target, or "" when cc is nil (tests and
// direct interceptor calls may pass no connection).
func dialTarget(cc *grpc.ClientConn) string {
	if cc == nil {
		return ""
	}

	return cc.Target()
}

// execute runs fn through the configured resilience.Guard, or directly
// when no guard is set.
func (c *config) execute(ctx context.Context, fn func(context.Context) error) error {
	if c.guard == nil {
		return fn(ctx)
	}

	return c.guard.Execute(ctx, fn)
}

// splitFullMethod parses a gRPC full method ("/pkg.Service/Method") into
// its service ("pkg.Service") and method ("Method") parts. A malformed
// value yields empty strings rather than panicking; the span name keeps
// the raw full method either way.
func splitFullMethod(full string) (service, method string) {
	trimmed := strings.TrimPrefix(full, "/")
	svc, m, found := strings.Cut(trimmed, "/")
	if !found {
		return "", svc
	}

	return svc, m
}

// recordRPCStatus adds rpc.grpc.status_code to the span when err is
// non-nil. A nil span or nil error is a no-op.
func recordRPCStatus(span observability.Span, err error) {
	if span == nil || err == nil {
		return
	}

	span.SetAttributes(observability.Int(observability.RPCGRPCStatusCode, int(status.Code(err))))
}

// attachMetadata injects the W3C trace context from ctx and the configured
// WithMetadata keys into the outgoing metadata. Keys already present in
// the outgoing context are never overwritten.
func (c *config) attachMetadata(ctx context.Context) context.Context {
	md := traceprop.Inject(ctx, nil)
	if md == nil {
		md = make(map[string]string, len(c.metadata))
	}
	for k, v := range c.metadata {
		if _, exists := md[k]; !exists {
			md[k] = v
		}
	}

	var existing map[string][]string
	if raw, ok := metadata.FromOutgoingContext(ctx); ok {
		existing = raw
	}

	for k, v := range md {
		if _, exists := existing[k]; exists {
			continue
		}
		ctx = metadata.AppendToOutgoingContext(ctx, k, v)
	}

	return ctx
}

// noopSpan is the observability.Span used when no Provider is configured.
type noopSpan struct{}

// SetAttributes discards attributes.
func (noopSpan) SetAttributes(...observability.Attr) {}

// RecordError discards the error.
func (noopSpan) RecordError(error) {}

// End is a no-op.
func (noopSpan) End() {}
