package log

import (
	"context"
	"sync/atomic"
	"time"
)

var adapterSeq atomic.Int64

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
