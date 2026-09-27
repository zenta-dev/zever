package log

import (
	"context"
	"time"
)

// Context accumulates fields for building a child logger.
type Context interface {
	// Str appends a string field to the context.
	Str(key, val string) Context
	// Int appends an int field to the context.
	Int(key string, val int) Context
	// Int64 appends an int64 field to the context.
	Int64(key string, val int64) Context
	// Float64 appends a float64 field to the context.
	Float64(key string, val float64) Context
	// Bool appends a bool field to the context.
	Bool(key string, val bool) Context
	// Dur appends a duration field to the context.
	Dur(key string, val time.Duration) Context
	// Time appends a time field to the context.
	Time(key string, val time.Time) Context
	// Err appends an error under the conventional "error" key.
	Err(err error) Context
	// AnErr appends an error under a custom key.
	AnErr(key string, err error) Context
	// Any appends an arbitrary value to the context.
	Any(key string, val any) Context

	// Logger builds a Logger carrying the accumulated fields.
	Logger() Logger
}

type ctxKey struct{}

// FromContext returns the Logger stored in ctx, or fallback when absent.
func FromContext(ctx context.Context, fallback Logger) Logger {
	if l, ok := ctx.Value(ctxKey{}).(Logger); ok {
		return l
	}
	return fallback
}

// ContextWithLogger returns a new context carrying the given Logger.
func ContextWithLogger(ctx context.Context, l Logger) context.Context {
	return context.WithValue(ctx, ctxKey{}, l)
}
