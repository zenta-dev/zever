package middleware

import (
	"net/http"
	"time"

	"github.com/zenta-dev/zever/log"
	"github.com/zenta-dev/zever/observability"
)

// RequestLogger returns HTTP middleware that logs one structured line per
// request via logger, after the handler has run: method, path, status, and
// duration. Only r.URL.Path is logged (never the query string), and the
// request ID is attached when one is present in the request's context.
func RequestLogger(logger log.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			rec := newStatusRecorder(w)

			next.ServeHTTP(rec, r)

			ms := time.Since(start).Milliseconds()

			ev := logger.Info().
				Str("method", r.Method).
				Str("path", r.URL.Path).
				Int("status", rec.status).
				Int64("duration_ms", ms)

			if id := observability.RequestIDFromContext(r.Context()); id != "" {
				ev = ev.Str("request_id", id)
			}

			ev.Msg("request")
		})
	}
}
