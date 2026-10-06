package grpcclient

import (
	"context"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"

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
		ctx, span := c.startSpan(ctx, method)
		defer span.End()

		ctx = c.attachMetadata(ctx)

		if c.timeoutSet {
			var cancel context.CancelFunc
			ctx, cancel = context.WithTimeout(ctx, c.timeout)
			defer cancel()
		}

		if c.guard != nil {
			return c.guard.Execute(ctx, func(ctx context.Context) error {
				return invoker(ctx, method, req, reply, cc, opts...)
			})
		}

		return invoker(ctx, method, req, reply, cc, opts...)
	}
}

// streamInterceptor returns the chained stream client interceptor with the
// same ordering as unaryInterceptor. The guard wraps stream establishment
// only: messages exchanged after the stream is established are not
// guarded.
func (c *config) streamInterceptor() grpc.StreamClientInterceptor {
	return func(ctx context.Context, desc *grpc.StreamDesc, cc *grpc.ClientConn, method string, streamer grpc.Streamer, opts ...grpc.CallOption) (grpc.ClientStream, error) {
		ctx, span := c.startSpan(ctx, method)
		defer span.End()

		ctx = c.attachMetadata(ctx)

		if c.timeoutSet {
			var cancel context.CancelFunc
			ctx, cancel = context.WithTimeout(ctx, c.timeout)
			defer cancel()
		}

		if c.guard != nil {
			var stream grpc.ClientStream
			err := c.guard.Execute(ctx, func(ctx context.Context) error {
				var execErr error
				stream, execErr = streamer(ctx, desc, cc, method, opts...)
				return execErr
			})
			return stream, err
		}

		return streamer(ctx, desc, cc, method, opts...)
	}
}

// startSpan starts a client span when an observability Provider is
// configured; otherwise it returns ctx unchanged with a noopSpan.
func (c *config) startSpan(ctx context.Context, method string) (context.Context, observability.Span) {
	if c.observability == nil {
		return ctx, noopSpan{}
	}
	return c.observability.Tracer(scopeName).Start(ctx, method)
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
