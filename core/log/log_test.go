package log

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

var adapterSeq atomic.Int64

func freshAdapter() Adapter { return Adapter(fmt.Sprintf("test-%d", 1000+adapterSeq.Add(1))) }

type stubEvent struct{}

func (stubEvent) Str(string, string) Event        { return stubEvent{} }
func (stubEvent) Int(string, int) Event           { return stubEvent{} }
func (stubEvent) Int64(string, int64) Event       { return stubEvent{} }
func (stubEvent) Float64(string, float64) Event   { return stubEvent{} }
func (stubEvent) Bool(string, bool) Event         { return stubEvent{} }
func (stubEvent) Dur(string, time.Duration) Event { return stubEvent{} }
func (stubEvent) Time(string, time.Time) Event    { return stubEvent{} }
func (stubEvent) Err(error) Event                 { return stubEvent{} }
func (stubEvent) AnErr(string, error) Event       { return stubEvent{} }
func (stubEvent) Any(string, any) Event           { return stubEvent{} }
func (stubEvent) Msg(string)                      {}
func (stubEvent) Msgf(string, ...any)             {}
func (stubEvent) Send()                           {}

type stubContext struct{ logger Logger }

func (c stubContext) Str(string, string) Context        { return c }
func (c stubContext) Int(string, int) Context           { return c }
func (c stubContext) Int64(string, int64) Context       { return c }
func (c stubContext) Float64(string, float64) Context   { return c }
func (c stubContext) Bool(string, bool) Context         { return c }
func (c stubContext) Dur(string, time.Duration) Context { return c }
func (c stubContext) Time(string, time.Time) Context    { return c }
func (c stubContext) Err(error) Context                 { return c }
func (c stubContext) AnErr(string, error) Context       { return c }
func (c stubContext) Any(string, any) Context           { return c }
func (c stubContext) Logger() Logger                    { return c.logger }

type stubLogger struct{ level Level }

func (l stubLogger) Debug() Event                       { return stubEvent{} }
func (l stubLogger) Info() Event                        { return stubEvent{} }
func (l stubLogger) Warn() Event                        { return stubEvent{} }
func (l stubLogger) Error() Event                       { return stubEvent{} }
func (l stubLogger) Fatal() Event                       { return stubEvent{} }
func (l stubLogger) With() Context                      { return stubContext{logger: l} }
func (l stubLogger) WithContext(context.Context) Logger { return l }
func (l stubLogger) Enabled(lv Level) bool              { return lv >= l.level }
func (l stubLogger) Sync() error                        { return nil }
func (l stubLogger) Name() string                       { return "stub" }

var (
	_ Logger  = stubLogger{}
	_ Event   = stubEvent{}
	_ Context = stubContext{}
)

func TestOpen_registeredFactory_returnsLogger(t *testing.T) {
	adapter := freshAdapter()
	wantOpts := Options{MinLevel: LevelWarn}
	var gotOpts Options

	err := Register(adapter, func(opts Options) (Logger, error) {
		gotOpts = opts
		return stubLogger{}, nil
	})
	if err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	got, err := Open(adapter, wantOpts)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}

	if got == nil {
		t.Fatal("Open() = nil, want logger")
	}

	if gotOpts != wantOpts {
		t.Errorf("factory opts = %+v, want %+v", gotOpts, wantOpts)
	}
}

func TestOpen_factoryError_wrappedWithAdapter(t *testing.T) {
	adapter := freshAdapter()
	sentinel := errors.New("boom")

	_ = Register(adapter, func(Options) (Logger, error) { return nil, sentinel })

	_, err := Open(adapter, Options{})
	if err == nil {
		t.Fatal("Open() = nil, want wrapped factory error")
	}

	if !errors.Is(err, sentinel) {
		t.Errorf("errors.Is(err, sentinel) = false (err = %v)", err)
	}
}

func TestRegister_concurrent_uniqueAdapters_allSucceed(t *testing.T) {
	t.Parallel()

	const workers = 20

	var wg sync.WaitGroup

	errs := make([]error, workers)
	for i := range workers {
		wg.Add(1)

		go func() {
			defer wg.Done()
			a := freshAdapter()
			errs[i] = Register(a, func(Options) (Logger, error) { return stubLogger{}, nil })
		}()
	}

	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Errorf("Register() worker %d error = %v", i, err)
		}
	}
}

func TestOpen_concurrentReads_safe(t *testing.T) {
	t.Parallel()

	adapter := freshAdapter()
	_ = Register(adapter, func(Options) (Logger, error) { return stubLogger{}, nil })

	const workers = 50

	var wg sync.WaitGroup

	for range workers {
		wg.Add(1)

		go func() {
			defer wg.Done()

			if _, err := Open(adapter, Options{}); err != nil {
				t.Errorf("Open() error = %v", err)
			}
		}()
	}

	wg.Wait()
}
