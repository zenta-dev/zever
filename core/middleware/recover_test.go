package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/zenta-dev/zever/core/log"
)

// panicEvent is a minimal log.Event fake recording fields and the message a
// test's log call carried.
type panicEvent struct {
	logger *panicLogger
	level  log.Level
	fields map[string]any
}

func newPanicEvent(l *panicLogger, level log.Level) *panicEvent {
	return &panicEvent{logger: l, level: level, fields: map[string]any{}}
}

func (e *panicEvent) Str(key, val string) log.Event { e.fields[key] = val; return e }
func (e *panicEvent) Int(key string, val int) log.Event {
	e.fields[key] = val
	return e
}
func (e *panicEvent) Int64(key string, val int64) log.Event {
	e.fields[key] = val
	return e
}
func (e *panicEvent) Float64(key string, val float64) log.Event {
	e.fields[key] = val
	return e
}
func (e *panicEvent) Bool(key string, val bool) log.Event {
	e.fields[key] = val
	return e
}
func (e *panicEvent) Dur(key string, val time.Duration) log.Event {
	e.fields[key] = val
	return e
}
func (e *panicEvent) Time(key string, val time.Time) log.Event {
	e.fields[key] = val
	return e
}
func (e *panicEvent) Err(err error) log.Event { e.fields["error"] = err; return e }
func (e *panicEvent) AnErr(key string, err error) log.Event {
	e.fields[key] = err
	return e
}
func (e *panicEvent) Any(key string, val any) log.Event {
	e.fields[key] = val
	return e
}
func (e *panicEvent) Msg(msg string) {
	e.logger.events = append(e.logger.events, panicLogCall{level: e.level, msg: msg, fields: e.fields})
}
func (e *panicEvent) Msgf(format string, _ ...any) {
	e.logger.events = append(e.logger.events, panicLogCall{level: e.level, msg: format, fields: e.fields})
}
func (e *panicEvent) Send() {
	e.logger.events = append(e.logger.events, panicLogCall{level: e.level, fields: e.fields})
}

type panicLogCall struct {
	level  log.Level
	msg    string
	fields map[string]any
}

// panicLogger implements the full log.Logger interface, recording every event
// per level for tests to assert on.
type panicLogger struct {
	events []panicLogCall
}

func (l *panicLogger) Debug() log.Event { return newPanicEvent(l, log.LevelDebug) }
func (l *panicLogger) Info() log.Event  { return newPanicEvent(l, log.LevelInfo) }
func (l *panicLogger) Warn() log.Event  { return newPanicEvent(l, log.LevelWarn) }
func (l *panicLogger) Error() log.Event { return newPanicEvent(l, log.LevelError) }
func (l *panicLogger) Fatal() log.Event { return newPanicEvent(l, log.LevelFatal) }

func (l *panicLogger) With() log.Context { return nil }
func (l *panicLogger) WithContext(context.Context) log.Logger {
	return l
}
func (l *panicLogger) Enabled(log.Level) bool { return true }
func (l *panicLogger) Sync() error            { return nil }
func (l *panicLogger) Name() string           { return "fake" }

var _ log.Logger = (*panicLogger)(nil)
var _ log.Event = (*panicEvent)(nil)

func (l *panicLogger) errorCalls() []panicLogCall {
	var out []panicLogCall
	for _, e := range l.events {
		if e.level == log.LevelError {
			out = append(out, e)
		}
	}
	return out
}

func TestRecover_panicReturns500ExactJSON(t *testing.T) {
	logger := &panicLogger{}

	handler := Recover(logger)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic("boom")
	}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}

	if got := rec.Body.String(); got != "{\"error\":\"internal error\"}\n" {
		t.Fatalf("body = %q, want exact fixed-shape 500 JSON", got)
	}

	if got := rec.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", got)
	}

	errs := logger.errorCalls()
	if len(errs) != 1 {
		t.Fatalf("expected exactly one error log, got %d", len(logger.events))
	}
	if errs[0].msg != "panic recovered" {
		t.Fatalf("log msg = %q, want %q", errs[0].msg, "panic recovered")
	}
	panStr, ok := errs[0].fields["panic"].(string)
	if !ok || !strings.Contains(panStr, "boom") {
		t.Fatalf("log fields missing panic value, got %v", errs[0].fields)
	}
	stStr, ok := errs[0].fields["stack"].(string)
	if !ok || stStr == "" || !strings.Contains(stStr, "goroutine") {
		t.Fatalf("log fields missing stack trace, got %v", errs[0].fields)
	}
}

func TestRecover_noPanicPassthroughKeepsStatusAndBody(t *testing.T) {
	logger := &panicLogger{}

	handler := Recover(logger)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTeapot)
		_, _ = w.Write([]byte("hi"))
	}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusTeapot {
		t.Fatalf("status = %d, want 418", rec.Code)
	}
	if got := rec.Body.String(); got != "hi" {
		t.Fatalf("body = %q, want %q", got, "hi")
	}
	if len(logger.events) != 0 {
		t.Fatalf("expected no logs for a non-panicking handler, got %d", len(logger.events))
	}
}

func TestRecover_committedResponseLogsOnlyWithoutRewrite(t *testing.T) {
	logger := &panicLogger{}

	handler := Recover(logger)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("partial"))
		panic("late boom")
	}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil)
	handler.ServeHTTP(rec, req)

	// The header is already committed: recovery must not attempt a second
	// WriteHeader or append a truncated JSON body.
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want committed 200 left untouched", rec.Code)
	}
	if got := rec.Body.String(); got != "partial" {
		t.Fatalf("body = %q, want only what the handler wrote", got)
	}
	if len(logger.errorCalls()) != 1 {
		t.Fatalf("expected exactly one error log, got %d", len(logger.events))
	}
}

func TestRecover_nilPanicValueStillHandled(t *testing.T) {
	// Since Go 1.21 panic(nil) recovers as a non-nil *runtime.PanicNilError,
	// so a nil panic still takes the recovery path: 500 + one error log.
	logger := &panicLogger{}

	handler := Recover(logger)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic(nil)
	}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
	if got := rec.Body.String(); got != "{\"error\":\"internal error\"}\n" {
		t.Fatalf("body = %q, want exact fixed-shape 500 JSON", got)
	}
	if len(logger.errorCalls()) != 1 {
		t.Fatalf("expected exactly one error log, got %d", len(logger.events))
	}
}

func TestRecoverUnaryServerInterceptor_recoversWithInternal(t *testing.T) {
	logger := &panicLogger{}
	interceptor := RecoverUnaryServerInterceptor(logger)

	handler := func(context.Context, any) (any, error) {
		panic("boom")
	}

	_, err := interceptor(t.Context(), nil, &grpc.UnaryServerInfo{FullMethod: "/svc/Method"}, handler)

	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("err = %v, want a gRPC status error", err)
	}
	if st.Code() != codes.Internal {
		t.Fatalf("code = %v, want codes.Internal", st.Code())
	}
	if st.Message() != "internal error" {
		t.Fatalf("message = %q, want fixed client-safe message", st.Message())
	}

	errs := logger.errorCalls()
	if len(errs) != 1 {
		t.Fatalf("expected exactly one error log, got %d", len(logger.events))
	}
	if errs[0].fields["method"] != "/svc/Method" {
		t.Fatalf("log fields missing method, got %v", errs[0].fields)
	}
	if _, ok := errs[0].fields["panic"]; !ok {
		t.Fatalf("log fields missing panic value, got %v", errs[0].fields)
	}
	if _, ok := errs[0].fields["stack"]; !ok {
		t.Fatalf("log fields missing stack trace, got %v", errs[0].fields)
	}
}

func TestRecoverUnaryServerInterceptor_passthrough(t *testing.T) {
	logger := &panicLogger{}
	interceptor := RecoverUnaryServerInterceptor(logger)

	handler := func(_ context.Context, req any) (any, error) { return req, nil }

	resp, err := interceptor(t.Context(), "ok", &grpc.UnaryServerInfo{FullMethod: "/svc/Method"}, handler)
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if resp != "ok" {
		t.Fatalf("resp = %v, want %q", resp, "ok")
	}
	if len(logger.events) != 0 {
		t.Fatalf("expected no logs for a non-panicking handler, got %d", len(logger.events))
	}
}
