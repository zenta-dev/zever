package middleware

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/zenta-dev/zever/core/log"
	"github.com/zenta-dev/zever/core/observability"
)

// fakeEvent captures builder fields for assertions.
type fakeEvent struct {
	strs   map[string]string
	ints   map[string]int
	int64s map[string]int64
	msg    string
}

func newFakeEvent() *fakeEvent {
	return &fakeEvent{
		strs:   make(map[string]string),
		ints:   make(map[string]int),
		int64s: make(map[string]int64),
	}
}

func (e *fakeEvent) Str(key, val string) log.Event {
	e.strs[key] = val
	return e
}

func (e *fakeEvent) Int(key string, val int) log.Event {
	e.ints[key] = val
	return e
}

func (e *fakeEvent) Int64(key string, val int64) log.Event {
	e.int64s[key] = val
	return e
}

func (e *fakeEvent) Float64(string, float64) log.Event { return e }

func (e *fakeEvent) Bool(string, bool) log.Event { return e }

func (e *fakeEvent) Dur(string, time.Duration) log.Event { return e }

func (e *fakeEvent) Time(string, time.Time) log.Event { return e }

func (e *fakeEvent) Err(error) log.Event { return e }

func (e *fakeEvent) AnErr(string, error) log.Event { return e }

func (e *fakeEvent) Any(string, any) log.Event { return e }

func (e *fakeEvent) Msg(msg string) { e.msg = msg }

func (e *fakeEvent) Msgf(format string, _ ...any) { e.msg = format }

func (e *fakeEvent) Send() {}

// fakeContext is a minimal log.Context for the fake logger.
type fakeContext struct{}

func (c *fakeContext) Str(string, string) log.Context { return c }

func (c *fakeContext) Int(string, int) log.Context { return c }

func (c *fakeContext) Int64(string, int64) log.Context { return c }

func (c *fakeContext) Float64(string, float64) log.Context { return c }

func (c *fakeContext) Bool(string, bool) log.Context { return c }

func (c *fakeContext) Dur(string, time.Duration) log.Context { return c }

func (c *fakeContext) Time(string, time.Time) log.Context { return c }

func (c *fakeContext) Err(error) log.Context { return c }

func (c *fakeContext) AnErr(string, error) log.Context { return c }

func (c *fakeContext) Any(string, any) log.Context { return c }

func (c *fakeContext) Logger() log.Logger { return nil }

// fakeLogger captures the single Info event per request.
type fakeLogger struct {
	event *fakeEvent
	calls int
}

func (l *fakeLogger) Debug() log.Event { return newFakeEvent() }

func (l *fakeLogger) Info() log.Event {
	l.calls++
	l.event = newFakeEvent()

	return l.event
}

func (l *fakeLogger) Warn() log.Event { return newFakeEvent() }

func (l *fakeLogger) Error() log.Event { return newFakeEvent() }

func (l *fakeLogger) Fatal() log.Event { return newFakeEvent() }

func (l *fakeLogger) With() log.Context { return &fakeContext{} }

func (l *fakeLogger) WithContext(context.Context) log.Logger { return l }

func (l *fakeLogger) Enabled(log.Level) bool { return true }

func (l *fakeLogger) Sync() error { return nil }

func (l *fakeLogger) Name() string { return "fake" }

func doLoggedRequest(ctx context.Context, t *testing.T, logger *fakeLogger, handler http.Handler) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequestWithContext(ctx, http.MethodGet, "/items?secret=1", nil)
	rec := httptest.NewRecorder()

	RequestLogger(logger)(handler).ServeHTTP(rec, req)

	return rec
}

func TestRequestLoggerLogsFields(t *testing.T) {
	t.Parallel()

	logger := &fakeLogger{}
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(w, "ok")
	})

	rec := doLoggedRequest(t.Context(), t, logger, handler)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusCreated)
	}

	if rec.Body.String() != "ok" {
		t.Fatalf("body = %q, want %q", rec.Body.String(), "ok")
	}

	if logger.calls != 1 {
		t.Fatalf("Info calls = %d, want 1", logger.calls)
	}

	ev := logger.event

	if got := ev.strs["method"]; got != http.MethodGet {
		t.Errorf("method = %q, want %q", got, http.MethodGet)
	}

	if got := ev.strs["path"]; got != "/items" {
		t.Errorf("path = %q, want %q", got, "/items")
	}

	if got := ev.ints["status"]; got != http.StatusCreated {
		t.Errorf("status = %d, want %d", got, http.StatusCreated)
	}

	ms, ok := ev.int64s["duration_ms"]
	if !ok {
		t.Fatal("duration_ms missing")
	}

	if ms < 0 {
		t.Errorf("duration_ms = %d, want >= 0", ms)
	}

	if _, ok := ev.strs["request_id"]; ok {
		t.Error("request_id must be absent without observability.WithRequestID")
	}

	if ev.msg != "request" {
		t.Errorf("msg = %q, want %q", ev.msg, "request")
	}
}

func TestRequestLoggerIncludesRequestID(t *testing.T) {
	t.Parallel()

	logger := &fakeLogger{}
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "ok")
	})

	ctx := observability.WithRequestID(t.Context(), "req-123")
	doLoggedRequest(ctx, t, logger, handler)

	if got := logger.event.strs["request_id"]; got != "req-123" {
		t.Errorf("request_id = %q, want %q", got, "req-123")
	}
}

func TestRequestLoggerImplicit200(t *testing.T) {
	t.Parallel()

	logger := &fakeLogger{}
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "ok")
	})

	doLoggedRequest(t.Context(), t, logger, handler)

	if got := logger.event.ints["status"]; got != http.StatusOK {
		t.Errorf("status = %d, want %d", got, http.StatusOK)
	}
}

func TestRequestLoggerChainedDedup(t *testing.T) {
	t.Parallel()

	logger := &fakeLogger{}
	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusAccepted)
		_, _ = io.WriteString(w, "ok")
	})

	// Wrap twice: idempotent newStatusRecorder must reuse a single recorder.
	chained := RequestLogger(logger)(RequestLogger(logger)(inner))

	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/chain", nil)
	rec := httptest.NewRecorder()
	chained.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusAccepted)
	}

	if logger.calls != 2 {
		t.Fatalf("Info calls = %d, want 2 (one per layer)", logger.calls)
	}

	if got := logger.event.ints["status"]; got != http.StatusAccepted {
		t.Errorf("status = %d, want %d", got, http.StatusAccepted)
	}

	if got := logger.event.strs["path"]; got != "/chain" {
		t.Errorf("path = %q, want %q", got, "/chain")
	}
}

func TestRequestLoggerNoPanicOnHandlerWithoutExplicitHeader(t *testing.T) {
	t.Parallel()

	logger := &fakeLogger{}
	handler := http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		// No Write/WriteHeader at all.
	})

	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/empty", nil)
	rec := httptest.NewRecorder()

	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("RequestLogger panicked: %v", r)
			}
		}()
		RequestLogger(logger)(handler).ServeHTTP(rec, req)
	}()

	if got := logger.event.ints["status"]; got != http.StatusOK {
		t.Errorf("status = %d, want %d", got, http.StatusOK)
	}
}
