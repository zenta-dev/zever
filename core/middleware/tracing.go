package middleware

import (
	"context"
	"net/http"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/zenta-dev/zever/core/observability"
	"github.com/zenta-dev/zever/shared/traceprop"
)

// scopeName is the observability scope all middleware tracers and meters
// share. One fixed scope keeps dashboards/queries simple: every
// middleware span and counter is queryable under "middleware".
const scopeName = "middleware"

// traceHeaders are the W3C propagation headers extracted from inbound
// requests before a server span is started.
var traceHeaders = []string{
	traceprop.TraceParentHeader,
	traceprop.TraceStateHeader,
	traceprop.BaggageHeader,
}

// extractTraceHeaders builds a lowercase header map from the request's W3C
// propagation headers. Only headers with a non-empty value are included.
func extractTraceHeaders(r *http.Request) map[string]string {
	hdrs := make(map[string]string, len(traceHeaders))
	for _, k := range traceHeaders {
		if v := r.Header.Get(k); v != "" {
			hdrs[k] = v
		}
	}

	return hdrs
}

// extractTraceMetadata builds a lowercase header map from the incoming gRPC
// metadata's W3C propagation headers. Only headers with a non-empty first
// value are included.
func extractTraceMetadata(ctx context.Context) map[string]string {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return nil
	}

	hdrs := make(map[string]string, len(traceHeaders))
	for _, k := range traceHeaders {
		if vals := md.Get(k); len(vals) > 0 && vals[0] != "" {
			hdrs[k] = vals[0]
		}
	}

	return hdrs
}

// Tracing returns HTTP middleware that starts a server span (named by the
// request path) around each request, records the handler's status on the
// span, and emits an "http.request" counter tagged by
// http.request.method/http.response.status_code. Span and counter
// attributes follow the OpenTelemetry semantic conventions
// (observability.HTTPRequestMethod, observability.URLPath,
// observability.HTTPResponseStatusCode).
//
// Inbound W3C traceparent/tracestate/baggage headers are extracted before
// the span starts, so the server span joins the caller's trace instead of
// starting a new root. Baggage is propagated to the handler context.
//
// Only r.URL.Path is used for the span name and path attribute - never the
// query string or raw URI - so high-cardinality/sensitive query values stay
// out of traces and metrics.
//
// Tracing does not recover panics: the deferred span.End still runs on panic,
// then the panic propagates. Wire Recover outside Tracing so panics are
// converted to 500s before the outer logging layer observes them.
//
// The metrics counter is best-effort: its error is discarded (`_ =`) so a
// metrics backend outage never fails a served request.
func Tracing(provider observability.Provider) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := traceprop.Extract(r.Context(), extractTraceHeaders(r))

			ctx, span := observability.StartSpan(ctx, provider.Tracer(scopeName), r.URL.Path,
				observability.WithSpanKind(observability.SpanKindServer),
				observability.WithAttributes(
					observability.String(observability.HTTPRequestMethod, r.Method),
					observability.String(observability.URLPath, r.URL.Path),
				),
			)
			defer span.End()

			rec := newStatusRecorder(w)

			next.ServeHTTP(rec, r.WithContext(ctx))

			span.SetAttributes(observability.Int(observability.HTTPResponseStatusCode, rec.status))

			if rec.status >= http.StatusInternalServerError {
				span.RecordError(statusError{rec.status})
			}

			_ = provider.Meter(scopeName).Counter(ctx, "http.request", 1,
				observability.String(observability.HTTPRequestMethod, r.Method),
				observability.Int(observability.HTTPResponseStatusCode, rec.status),
			)
		})
	}
}

// TracingUnaryServerInterceptor is Tracing's gRPC counterpart: it starts a
// server span named by the full gRPC method around each call and records an
// "rpc.request" counter tagged by rpc.service/rpc.method and the gRPC
// status code. Inbound W3C traceparent/tracestate/baggage metadata is
// extracted before the span starts, so the server span joins the caller's
// trace. Handler errors are recorded on the span; the counter is
// best-effort.
//
// The span name stays the low-cardinality full method while rpc.system,
// rpc.service, and rpc.method carry the parsed pieces
// (observability.RPCSystem, observability.RPCService,
// observability.RPCMethod). observability.RPCGRPCStatusCode is added
// after the handler returns, carrying codes.OK on success.
func TracingUnaryServerInterceptor(provider observability.Provider) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		ctx = traceprop.Extract(ctx, extractTraceMetadata(ctx))

		svc, method := splitFullMethod(info.FullMethod)

		ctx, span := observability.StartSpan(ctx, provider.Tracer(scopeName), info.FullMethod,
			observability.WithSpanKind(observability.SpanKindServer),
			observability.WithAttributes(
				observability.String(observability.RPCSystem, observability.SystemGRPC),
				observability.String(observability.RPCService, svc),
				observability.String(observability.RPCMethod, method),
			),
		)
		defer span.End()

		resp, err := handler(ctx, req)

		code := codes.OK

		if err != nil {
			span.RecordError(err)

			code = status.Code(err)
		}

		span.SetAttributes(observability.Int(observability.RPCGRPCStatusCode, int(code)))

		_ = provider.Meter(scopeName).Counter(ctx, "rpc.request", 1,
			observability.String(observability.RPCService, svc),
			observability.String(observability.RPCMethod, method),
			observability.Int(observability.RPCGRPCStatusCode, int(code)),
		)

		return resp, err
	}
}

// splitFullMethod parses a gRPC full method ("/pkg.Service/Method") into
// its service ("pkg.Service") and method ("Method") parts. A malformed
// value yields empty strings rather than panicking; the caller still sees
// the raw full method in the span name.
func splitFullMethod(full string) (service, method string) {
	trimmed := strings.TrimPrefix(full, "/")
	svc, m, found := strings.Cut(trimmed, "/")
	if !found {
		return "", svc
	}

	return svc, m
}

// statusError adapts an HTTP status code to an error so Tracing can record a
// server-error response on the span via the same RecordError(err) path a real
// handler error would use. Unknown codes have no StatusText and yield "".
type statusError struct{ status int }

// Error returns the status text for the recorded HTTP status code.
func (e statusError) Error() string {
	return http.StatusText(e.status)
}
