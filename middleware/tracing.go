package middleware

import (
	"context"
	"net/http"

	"google.golang.org/grpc"

	"github.com/zenta-dev/zever/observability"
)

// scopeName is the observability scope all middleware tracers and meters
// share. One fixed scope keeps dashboards/queries simple: every
// middleware span and counter is queryable under "middleware".
const scopeName = "middleware"

// Tracing returns HTTP middleware that starts a span (named by the request
// path) around each request, records the handler's status on the span, and
// emits an "http.request" counter tagged by method/status.
//
// Only r.URL.Path is used for the span name and path attribute — never the
// query string or raw URI — so high-cardinality/sensitive query values stay
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
			ctx, span := provider.Tracer(scopeName).Start(r.Context(), r.URL.Path)
			defer span.End()

			span.SetAttributes(
				observability.String("http.method", r.Method),
				observability.String("http.path", r.URL.Path),
			)

			rec := newStatusRecorder(w)

			next.ServeHTTP(rec, r.WithContext(ctx))

			span.SetAttributes(observability.Int("http.status_code", rec.status))

			if rec.status >= http.StatusInternalServerError {
				span.RecordError(statusError{rec.status})
			}

			_ = provider.Meter(scopeName).Counter(ctx, "http.request", 1,
				observability.String("method", r.Method),
				observability.String("status", http.StatusText(rec.status)),
			)
		})
	}
}

// TracingUnaryServerInterceptor is Tracing's gRPC counterpart: it starts a
// span named by the full gRPC method around each call and records an
// "rpc.request" counter tagged by method/outcome ("ok" or "error").
// Handler errors are recorded on the span; the counter is best-effort.
func TracingUnaryServerInterceptor(provider observability.Provider) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		ctx, span := provider.Tracer(scopeName).Start(ctx, info.FullMethod)
		defer span.End()

		resp, err := handler(ctx, req)

		outcome := "ok"

		if err != nil {
			span.RecordError(err)

			outcome = "error"
		}

		_ = provider.Meter(scopeName).Counter(ctx, "rpc.request", 1,
			observability.String("method", info.FullMethod),
			observability.String("outcome", outcome),
		)

		return resp, err
	}
}

// statusError adapts an HTTP status code to an error so Tracing can record a
// server-error response on the span via the same RecordError(err) path a real
// handler error would use. Unknown codes have no StatusText and yield "".
type statusError struct{ status int }

func (e statusError) Error() string {
	return http.StatusText(e.status)
}
