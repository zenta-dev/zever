package middleware

import (
	"bufio"
	"io"
	"net"
	"net/http"
)

// statusRecorder wraps an http.ResponseWriter to capture the status code a
// handler actually wrote. WriteHeader is not always called explicitly, so
// wroteHeader tracks whether a real call happened.
type statusRecorder struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
}

// newStatusRecorder wraps w to capture its status code, unless w is already
// a *statusRecorder - in that case it returns w unchanged instead of
// double-wrapping. Chained status-reading middleware reuses one recorder per
// request: a single alloc, no stacked wrappers.
func newStatusRecorder(w http.ResponseWriter) *statusRecorder {
	if rec, ok := w.(*statusRecorder); ok {
		return rec
	}

	return &statusRecorder{ResponseWriter: w, status: http.StatusOK}
}

// WriteHeader records status on first call and forwards every call to the wrapped writer.
func (r *statusRecorder) WriteHeader(status int) {
	if !r.wroteHeader {
		r.status = status
		r.wroteHeader = true
	}

	r.ResponseWriter.WriteHeader(status)
}

// Write marks the header as written and forwards the bytes to the wrapped writer.
func (r *statusRecorder) Write(b []byte) (int, error) {
	r.wroteHeader = true

	return r.ResponseWriter.Write(b)
}

// Flush forwards to the wrapped writer when it supports http.Flusher, and is
// a no-op otherwise.
func (r *statusRecorder) Flush() {
	if f, ok := r.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// Hijack forwards to the wrapped writer when it supports http.Hijacker,
// returning http.ErrNotSupported otherwise.
func (r *statusRecorder) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if h, ok := r.ResponseWriter.(http.Hijacker); ok {
		return h.Hijack()
	}

	return nil, nil, http.ErrNotSupported
}

// Push forwards to the wrapped writer when it supports http.Pusher,
// returning http.ErrNotSupported otherwise.
func (r *statusRecorder) Push(target string, opts *http.PushOptions) error {
	if p, ok := r.ResponseWriter.(http.Pusher); ok {
		return p.Push(target, opts)
	}

	return http.ErrNotSupported
}

// ReadFrom forwards to the wrapped writer when it supports io.ReaderFrom,
// returning http.ErrNotSupported otherwise.
func (r *statusRecorder) ReadFrom(src io.Reader) (int64, error) {
	if rf, ok := r.ResponseWriter.(io.ReaderFrom); ok {
		return rf.ReadFrom(src)
	}

	return 0, http.ErrNotSupported
}

// Unwrap returns the wrapped http.ResponseWriter. http.ResponseController
// (Go 1.20+) walks Unwrap chains to reach optional features on the inner
// writer, so without this a wrapped writer would hide them as unsupported.
func (r *statusRecorder) Unwrap() http.ResponseWriter {
	return r.ResponseWriter
}
