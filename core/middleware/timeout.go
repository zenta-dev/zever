package middleware

import (
	"bytes"
	"context"
	"net/http"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Timeout returns HTTP middleware that bounds a handler's execution to d,
// responding 504 with the fixed error shape if the handler hasn't finished
// by then -- instead of a slow or hung downstream call blocking the request
// indefinitely, which is what happens with no timeout middleware in the
// path. The handler's ctx (r.Context()) carries the deadline, so downstream
// calls using it (db.QueryContext, an outbound http.Client, ...) are
// canceled too; a handler that ignores ctx keeps running in the background
// after the client gets the timeout response, but its writes are then
// discarded rather than racing the timeout response on the wire.
//
// This deliberately does not use the stdlib http.TimeoutHandler: it writes
// its own response body on timeout, which would fight this package's fixed
// errorBody envelope and Recover's own response writing. The concurrency-
// safety technique (buffer the handler's output until it finishes or times
// out, so exactly one goroutine ever writes to the real ResponseWriter) is
// the same one http.TimeoutHandler uses internally.
//
// Timeout should run inside Recover (Recover outermost) so a panic in a
// still-running, timed-out handler is still caught, and outside RateLimit
// so a denied request never starts the timer.
func Timeout(d time.Duration) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx, cancel := context.WithTimeout(r.Context(), d)
			defer cancel()

			tw := &timeoutWriter{header: make(http.Header)}
			done := make(chan struct{})

			go func() {
				defer close(done)
				next.ServeHTTP(tw, r.WithContext(ctx))
			}()

			select {
			case <-done:
				tw.mu.Lock()
				defer tw.mu.Unlock()

				for k, v := range tw.header {
					w.Header()[k] = v
				}

				if tw.code == 0 {
					tw.code = http.StatusOK
				}

				w.WriteHeader(tw.code)
				_, _ = w.Write(tw.buf.Bytes())
			case <-ctx.Done():
				tw.mu.Lock()
				tw.timedOut = true
				tw.mu.Unlock()

				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusGatewayTimeout)

				if data, encErr := errorBodyCodec.Encode(errorBody{Error: "request timed out"}); encErr == nil {
					_, _ = w.Write(append(data, '\n'))
				}
			}
		})
	}
}

// timeoutWriter buffers a handler's response instead of writing it straight
// to the real http.ResponseWriter, so Timeout can discard it cleanly if the
// handler is still running past the deadline -- the real ResponseWriter is
// then only ever touched by Timeout's own goroutine, never concurrently by
// the (possibly still-running) handler goroutine.
type timeoutWriter struct {
	mu       sync.Mutex
	header   http.Header
	buf      bytes.Buffer
	code     int
	timedOut bool
}

func (tw *timeoutWriter) Header() http.Header {
	tw.mu.Lock()
	defer tw.mu.Unlock()

	return tw.header
}

func (tw *timeoutWriter) WriteHeader(code int) {
	tw.mu.Lock()
	defer tw.mu.Unlock()

	if tw.timedOut || tw.code != 0 {
		return
	}

	tw.code = code
}

func (tw *timeoutWriter) Write(b []byte) (int, error) {
	tw.mu.Lock()
	defer tw.mu.Unlock()

	if tw.timedOut {
		return len(b), nil
	}

	if tw.code == 0 {
		tw.code = http.StatusOK
	}

	return tw.buf.Write(b)
}

// TimeoutUnaryServerInterceptor is Timeout's gRPC counterpart: it bounds a
// handler's execution to d, returning codes.DeadlineExceeded if the handler
// hasn't finished by then. ctx already carries the deadline into handler,
// so downstream calls using it are canceled too; a handler that ignores ctx
// keeps running in the background, its eventual result simply discarded.
func TimeoutUnaryServerInterceptor(d time.Duration) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		ctx, cancel := context.WithTimeout(ctx, d)
		defer cancel()

		type result struct {
			resp any
			err  error
		}

		done := make(chan result, 1)

		go func() {
			resp, err := handler(ctx, req)
			done <- result{resp: resp, err: err}
		}()

		select {
		case res := <-done:
			return res.resp, res.err
		case <-ctx.Done():
			return nil, status.Error(codes.DeadlineExceeded, "request timed out")
		}
	}
}
