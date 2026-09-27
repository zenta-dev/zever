package queue

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/zenta-dev/zever/core/log"
	"github.com/zenta-dev/zever/core/webhook"
)

type captureEvent struct {
	parent *captureLogger
	level  string
}

func (e *captureEvent) Str(_, _ string) log.Event         { return e }
func (e *captureEvent) Int(_ string, _ int) log.Event     { return e }
func (e *captureEvent) Int64(_ string, _ int64) log.Event { return e }
func (e *captureEvent) Float64(_ string, _ float64) log.Event {
	return e
}
func (e *captureEvent) Bool(_ string, _ bool) log.Event { return e }
func (e *captureEvent) Dur(_ string, _ time.Duration) log.Event {
	return e
}
func (e *captureEvent) Time(_ string, _ time.Time) log.Event { return e }
func (e *captureEvent) Err(_ error) log.Event                { return e }
func (e *captureEvent) AnErr(_ string, _ error) log.Event    { return e }
func (e *captureEvent) Any(_ string, _ any) log.Event        { return e }
func (e *captureEvent) Msg(msg string) {
	e.parent.mu.Lock()
	defer e.parent.mu.Unlock()
	e.parent.msgs = append(e.parent.msgs, e.level+": "+msg)
}
func (e *captureEvent) Msgf(format string, args ...any) {
	e.parent.mu.Lock()
	defer e.parent.mu.Unlock()
	e.parent.msgs = append(e.parent.msgs, e.level+": "+fmt.Sprintf(format, args...))
}
func (e *captureEvent) Send() {
	e.parent.mu.Lock()
	defer e.parent.mu.Unlock()
	e.parent.msgs = append(e.parent.msgs, e.level+": send")
}

type captureLogger struct {
	mu   sync.Mutex
	msgs []string
}

func (c *captureLogger) Debug() log.Event                       { return &captureEvent{parent: c, level: "debug"} }
func (c *captureLogger) Info() log.Event                        { return &captureEvent{parent: c, level: "info"} }
func (c *captureLogger) Warn() log.Event                        { return &captureEvent{parent: c, level: "warn"} }
func (c *captureLogger) Error() log.Event                       { return &captureEvent{parent: c, level: "error"} }
func (c *captureLogger) Fatal() log.Event                       { return &captureEvent{parent: c, level: "fatal"} }
func (c *captureLogger) With() log.Context                      { return captureContext{parent: c} }
func (c *captureLogger) WithContext(context.Context) log.Logger { return c }
func (c *captureLogger) Enabled(log.Level) bool                 { return true }
func (c *captureLogger) Sync() error                            { return nil }
func (c *captureLogger) Name() string                           { return "capture" }

func (c *captureLogger) contains(substr string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, m := range c.msgs {
		if strings.Contains(m, substr) {
			return true
		}
	}
	return false
}

type captureContext struct {
	parent *captureLogger
}

func (c captureContext) Str(string, string) log.Context        { return c }
func (c captureContext) Int(string, int) log.Context           { return c }
func (c captureContext) Int64(string, int64) log.Context       { return c }
func (c captureContext) Float64(string, float64) log.Context   { return c }
func (c captureContext) Bool(string, bool) log.Context         { return c }
func (c captureContext) Dur(string, time.Duration) log.Context { return c }
func (c captureContext) Time(string, time.Time) log.Context    { return c }
func (c captureContext) Err(error) log.Context                 { return c }
func (c captureContext) AnErr(string, error) log.Context       { return c }
func (c captureContext) Any(string, any) log.Context           { return c }
func (c captureContext) Logger() log.Logger                    { return c.parent }

func TestLoggerReceivesConsumeError(t *testing.T) {
	t.Parallel()

	caplog := &captureLogger{}
	a := &adapter{logger: caplog, timeout: time.Second}
	c := &consumer{stop: make(chan struct{})}
	consecutive := 0

	a.handleConsumeError("evt", errors.New("boom"), &consecutive, c)

	if !caplog.contains("consumer pop failed") {
		t.Fatalf("expected injected logger to receive pop failure, got %v", caplog.msgs)
	}
}

func TestOpenWiresOptionsLogger(t *testing.T) {
	t.Parallel()

	caplog := &captureLogger{}
	got, err := New(webhook.Options{
		QueueAdapter: "memory",
		Timeout:      time.Second,
		Logger:       caplog,
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer func() { _ = got.Close() }()

	ad, ok := got.(*adapter)
	if !ok {
		t.Fatal("Open did not return *adapter")
	}
	if ad.log() != caplog {
		t.Fatal("Open did not wire Options.Logger into adapter")
	}
}

func TestNilLoggerDefaultsNoop(t *testing.T) {
	t.Parallel()

	a := &adapter{timeout: time.Second}
	if a.log() == nil {
		t.Fatal("nil logger must default to noop, got nil")
	}
	if got := a.log().Name(); got != "noop" {
		t.Fatalf("nil logger Name=%q want noop", got)
	}
}
