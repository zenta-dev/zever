package serve

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/zenta-dev/zever/container"
	"github.com/zenta-dev/zever/core/auth"
	"github.com/zenta-dev/zever/core/db"
	"github.com/zenta-dev/zever/core/log"
	"github.com/zenta-dev/zever/core/observability"
	"github.com/zenta-dev/zever/core/permission"
	"github.com/zenta-dev/zever/core/queue"
	"github.com/zenta-dev/zever/core/ratelimit"
	"github.com/zenta-dev/zever/core/router"
	"github.com/zenta-dev/zever/core/scheduler"
)

// stubRouter is an http.ServeMux-backed router.Router for tests. Method
// mismatches answer 405; middleware stacks in registration order.
type stubRouter struct {
	mu     sync.Mutex
	mux    *http.ServeMux
	mws    []func(http.Handler) http.Handler
	closed atomic.Bool
}

func newStubRouter() *stubRouter {
	return &stubRouter{mux: http.NewServeMux()}
}

// Handle registers handler for method and pattern.
func (r *stubRouter) Handle(method string, pattern string, handler http.HandlerFunc) {
	r.mu.Lock()
	defer r.mu.Unlock()

	mux := r.mux

	mux.HandleFunc(pattern, func(w http.ResponseWriter, req *http.Request) {
		if !strings.EqualFold(req.Method, method) {
			w.WriteHeader(http.StatusMethodNotAllowed)

			return
		}

		handler(w, req)
	})
}

// Group returns a route group joining prefix with sub-patterns.
func (r *stubRouter) Group(prefix string, middlewares ...func(http.Handler) http.Handler) router.Group {
	return &stubGroup{parent: r, prefix: prefix, mws: middlewares}
}

// Use appends middlewares to the router.
func (r *stubRouter) Use(middlewares ...func(http.Handler) http.Handler) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.mws = append(r.mws, middlewares...)
}

// ServeHTTP serves requests through the middleware stack, then the mux.
func (r *stubRouter) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	r.mu.Lock()
	mws := append([]func(http.Handler) http.Handler(nil), r.mws...)
	mux := r.mux
	r.mu.Unlock()

	var h http.Handler = mux
	for i := len(mws) - 1; i >= 0; i-- {
		h = mws[i](h)
	}

	h.ServeHTTP(w, req)
}

// Close records shutdown for Container.Close assertions.
func (r *stubRouter) Close(_ context.Context) error {
	r.closed.Store(true)

	return nil
}

// stubGroup joins prefix with sub-patterns on the parent stubRouter.
type stubGroup struct {
	parent *stubRouter
	prefix string
	mws    []func(http.Handler) http.Handler
}

// Handle registers handler for method and prefix-joined pattern.
func (g *stubGroup) Handle(method string, pattern string, handler http.HandlerFunc) {
	h := http.Handler(handler)

	for i := len(g.mws) - 1; i >= 0; i-- {
		h = g.mws[i](h)
	}

	g.parent.Handle(method, g.prefix+pattern, h.ServeHTTP)
}

// Group returns a nested route group.
func (g *stubGroup) Group(prefix string, middlewares ...func(http.Handler) http.Handler) router.Group {
	return &stubGroup{parent: g.parent, prefix: g.prefix + prefix, mws: append(append([]func(http.Handler) http.Handler(nil), g.mws...), middlewares...)}
}

// Use appends middlewares to the group.
func (g *stubGroup) Use(middlewares ...func(http.Handler) http.Handler) {
	g.mws = append(g.mws, middlewares...)
}

// stubEvent is a no-op log.Event recording nothing.
type stubEvent struct{}

func (stubEvent) Str(_, _ string) log.Event     { return stubEvent{} }
func (stubEvent) Int(_ string, _ int) log.Event { return stubEvent{} }
func (stubEvent) Int64(_ string, _ int64) log.Event {
	return stubEvent{}
}
func (stubEvent) Float64(_ string, _ float64) log.Event { return stubEvent{} }
func (stubEvent) Bool(_ string, _ bool) log.Event       { return stubEvent{} }
func (stubEvent) Dur(_ string, _ time.Duration) log.Event {
	return stubEvent{}
}
func (stubEvent) Time(_ string, _ time.Time) log.Event { return stubEvent{} }
func (stubEvent) Err(_ error) log.Event                { return stubEvent{} }
func (stubEvent) AnErr(_ string, _ error) log.Event    { return stubEvent{} }
func (stubEvent) Any(_ string, _ any) log.Event        { return stubEvent{} }
func (stubEvent) Msg(_ string)                         {}
func (stubEvent) Msgf(_ string, _ ...any)              {}
func (stubEvent) Send()                                {}

// stubContext is a no-op log.Context.
type stubContext struct{}

func (stubContext) Str(_, _ string) log.Context     { return stubContext{} }
func (stubContext) Int(_ string, _ int) log.Context { return stubContext{} }
func (stubContext) Int64(_ string, _ int64) log.Context {
	return stubContext{}
}
func (stubContext) Float64(_ string, _ float64) log.Context {
	return stubContext{}
}
func (stubContext) Bool(_ string, _ bool) log.Context { return stubContext{} }
func (stubContext) Dur(_ string, _ time.Duration) log.Context {
	return stubContext{}
}
func (stubContext) Time(_ string, _ time.Time) log.Context { return stubContext{} }
func (stubContext) Err(_ error) log.Context                { return stubContext{} }
func (stubContext) AnErr(_ string, _ error) log.Context    { return stubContext{} }
func (stubContext) Any(_ string, _ any) log.Context        { return stubContext{} }
func (stubContext) Logger() log.Logger                     { return stubLogger{} }

// stubLogger is a no-op log.Logger.
type stubLogger struct{}

func (stubLogger) Debug() log.Event                         { return stubEvent{} }
func (stubLogger) Info() log.Event                          { return stubEvent{} }
func (stubLogger) Warn() log.Event                          { return stubEvent{} }
func (stubLogger) Error() log.Event                         { return stubEvent{} }
func (stubLogger) Fatal() log.Event                         { return stubEvent{} }
func (stubLogger) With() log.Context                        { return stubContext{} }
func (stubLogger) WithContext(_ context.Context) log.Logger { return stubLogger{} }
func (stubLogger) Enabled(_ log.Level) bool                 { return false }
func (stubLogger) Sync() error                              { return nil }
func (stubLogger) Name() string                             { return "stub" }

// stubSpan is a no-op observability.Span.
type stubSpan struct{}

func (stubSpan) SetAttributes(_ ...observability.Attr) {}
func (stubSpan) RecordError(_ error)                   {}
func (stubSpan) End()                                  {}

// stubTracer is a no-op observability.Tracer.
type stubTracer struct{}

func (stubTracer) Start(ctx context.Context, _ string) (context.Context, observability.Span) {
	return ctx, stubSpan{}
}
func (stubTracer) Shutdown(_ context.Context) error { return nil }

// stubMetrics is a no-op observability.Metrics.
type stubMetrics struct{}

func (stubMetrics) Counter(_ context.Context, _ string, _ float64, _ ...observability.Attr) error {
	return nil
}
func (stubMetrics) Gauge(_ context.Context, _ string, _ float64, _ ...observability.Attr) error {
	return nil
}
func (stubMetrics) Histogram(_ context.Context, _ string, _ float64, _ ...observability.Attr) error {
	return nil
}
func (stubMetrics) Shutdown(_ context.Context) error { return nil }

// stubProvider is a no-op observability.Provider.
type stubProvider struct{}

func (stubProvider) Tracer(_ string) observability.Tracer { return stubTracer{} }
func (stubProvider) Meter(_ string) observability.Metrics { return stubMetrics{} }
func (stubProvider) Shutdown(_ context.Context) error     { return nil }

// stubRows is an empty db.Rows.
type stubRows struct{}

func (stubRows) Next() bool                 { return false }
func (stubRows) Scan(_ ...any) error        { return nil }
func (stubRows) Close() error               { return nil }
func (stubRows) Columns() ([]string, error) { return nil, nil }
func (stubRows) Err() error                 { return nil }

// stubDB is a configurable db.DB double: pingErr surfaces from Ping.
type stubDB struct {
	pingErr error
	closed  atomic.Bool
}

func (d *stubDB) Query(_ context.Context, _ string, _ ...any) (db.Rows, error) {
	return stubRows{}, nil
}
func (d *stubDB) Exec(_ context.Context, _ string, _ ...any) (int64, error) {
	return 0, nil
}
func (d *stubDB) Ping(_ context.Context) error { return d.pingErr }
func (d *stubDB) Close(_ context.Context) error {
	d.closed.Store(true)

	return nil
}
func (d *stubDB) Dialect() string { return "stub" }

// stubLimiter is an allow-all ratelimit.Limiter.
type stubLimiter struct{}

func (stubLimiter) Allow(_ context.Context, _ string, _ float64) (ratelimit.Decision, error) {
	return ratelimit.Decision{Allowed: true}, nil
}
func (stubLimiter) Reset(_ context.Context, _ string) error { return nil }
func (stubLimiter) Close() error                            { return nil }
func (stubLimiter) Name() string                            { return "stub" }

// stubQueue is a no-op queue.Queue recording Close.
type stubQueue struct {
	closed atomic.Bool
}

func (q *stubQueue) Push(_ context.Context, _ string, _ queue.Payload, _ queue.Headers) error {
	return nil
}
func (q *stubQueue) PushDelayed(_ context.Context, _ string, _ queue.Payload, _ queue.Headers, _ time.Duration) error {
	return nil
}
func (q *stubQueue) Pop(_ context.Context, _ string) (queue.Message, error) {
	return queue.Message{}, queue.ErrEmpty
}
func (q *stubQueue) Ack(_ context.Context, _ queue.Message) error { return nil }
func (q *stubQueue) Nack(_ context.Context, _ queue.Message, _ bool) error {
	return nil
}
func (q *stubQueue) Length(_ context.Context, _ string) (int64, error) {
	return 0, nil
}
func (q *stubQueue) IsEmpty(_ context.Context, _ string) (bool, error) {
	return true, nil
}
func (q *stubQueue) Close() error {
	q.closed.Store(true)

	return nil
}
func (q *stubQueue) Name() string { return "stub" }

// stubScheduler is a scheduler.Scheduler recording Start.
type stubScheduler struct {
	started atomic.Bool
	stopped atomic.Bool
	onStart func()
	onStop  func()
}

func (s *stubScheduler) Schedule(_ context.Context, _, _ string, _ any) (scheduler.EntryID, error) {
	return scheduler.EntryID(0), nil
}
func (s *stubScheduler) Remove(_ scheduler.EntryID) error { return nil }
func (s *stubScheduler) Entries() []scheduler.EntryID     { return nil }
func (s *stubScheduler) Name() string                     { return "stub" }
func (s *stubScheduler) Start() error {
	s.started.Store(true)
	if s.onStart != nil {
		s.onStart()
	}

	return nil
}
func (s *stubScheduler) Stop() error {
	s.stopped.Store(true)
	if s.onStop != nil {
		s.onStop()
	}

	return nil
}

// stubAuth is a no-op auth.Auth.
type stubAuth struct{}

func (stubAuth) Issue(_ context.Context, _ string, _ map[string]any, _ time.Duration) (auth.Token, error) {
	return auth.Token{}, nil
}
func (stubAuth) Verify(_ context.Context, _ string) (auth.Claims, error) {
	return auth.Claims{}, nil
}
func (stubAuth) Revoke(_ context.Context, _ string) error { return nil }
func (stubAuth) Close() error                             { return nil }

// stubPermission is a deny-by-default permission.Checker.
type stubPermission struct{}

func (stubPermission) Can(_ context.Context, _ permission.Subject, _ string, _ permission.Resource) (permission.Decision, error) {
	return permission.Decision{Allowed: false, Reason: "implicit_deny"}, nil
}

// recordEvent is a log.Event appending the message to its logger.
type recordEvent struct {
	logger *recordLogger
	level  string
}

func (e recordEvent) Str(_, _ string) log.Event     { return e }
func (e recordEvent) Int(_ string, _ int) log.Event { return e }
func (e recordEvent) Int64(_ string, _ int64) log.Event {
	return e
}
func (e recordEvent) Float64(_ string, _ float64) log.Event { return e }
func (e recordEvent) Bool(_ string, _ bool) log.Event       { return e }
func (e recordEvent) Dur(_ string, _ time.Duration) log.Event {
	return e
}
func (e recordEvent) Time(_ string, _ time.Time) log.Event { return e }
func (e recordEvent) Err(_ error) log.Event                { return e }
func (e recordEvent) AnErr(_ string, _ error) log.Event    { return e }
func (e recordEvent) Any(_ string, _ any) log.Event        { return e }
func (e recordEvent) Msg(msg string) {
	e.logger.mu.Lock()
	defer e.logger.mu.Unlock()

	e.logger.msgs = append(e.logger.msgs, e.level+": "+msg)
}
func (e recordEvent) Msgf(format string, args ...any) {
	e.Msg(fmt.Sprintf(format, args...))
}
func (e recordEvent) Send() { e.Msg("") }

// recordContext is a no-op log.Context.
type recordContext struct{}

func (recordContext) Str(_, _ string) log.Context     { return recordContext{} }
func (recordContext) Int(_ string, _ int) log.Context { return recordContext{} }
func (recordContext) Int64(_ string, _ int64) log.Context {
	return recordContext{}
}
func (recordContext) Float64(_ string, _ float64) log.Context {
	return recordContext{}
}
func (recordContext) Bool(_ string, _ bool) log.Context { return recordContext{} }
func (recordContext) Dur(_ string, _ time.Duration) log.Context {
	return recordContext{}
}
func (recordContext) Time(_ string, _ time.Time) log.Context { return recordContext{} }
func (recordContext) Err(_ error) log.Context                { return recordContext{} }
func (recordContext) AnErr(_ string, _ error) log.Context    { return recordContext{} }
func (recordContext) Any(_ string, _ any) log.Context        { return recordContext{} }
func (recordContext) Logger() log.Logger                     { return stubLogger{} }

// recordLogger is a log.Logger capturing Warn messages for assertions.
type recordLogger struct {
	mu   sync.Mutex
	msgs []string
}

func (l *recordLogger) Debug() log.Event                         { return recordEvent{logger: l, level: "debug"} }
func (l *recordLogger) Info() log.Event                          { return recordEvent{logger: l, level: "info"} }
func (l *recordLogger) Warn() log.Event                          { return recordEvent{logger: l, level: "warn"} }
func (l *recordLogger) Error() log.Event                         { return recordEvent{logger: l, level: "error"} }
func (l *recordLogger) Fatal() log.Event                         { return recordEvent{logger: l, level: "fatal"} }
func (l *recordLogger) With() log.Context                        { return recordContext{} }
func (l *recordLogger) WithContext(_ context.Context) log.Logger { return l }
func (l *recordLogger) Enabled(_ log.Level) bool                 { return true }
func (l *recordLogger) Sync() error                              { return nil }
func (l *recordLogger) Name() string                             { return "record" }

// warnsContaining reports whether any captured message contains sub.
func (l *recordLogger) warnsContaining(sub string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	for _, msg := range l.msgs {
		if strings.Contains(msg, sub) {
			return true
		}
	}

	return false
}

// stubModule is a Module double recording Mount calls.
type stubModule struct {
	name     string
	mounted  atomic.Bool
	mountErr error
	mountFn  func(r router.Router, c *container.Container) error
}

func (m *stubModule) Name() string { return m.name }

// Mount records the call, runs mountFn when set, and returns mountErr.
func (m *stubModule) Mount(r router.Router, c *container.Container) error {
	m.mounted.Store(true)
	if m.mountFn != nil {
		return m.mountFn(r, c)
	}

	return m.mountErr
}
