package slog

import (
	"context"
	"fmt"
	"time"

	stdslog "log/slog"

	"github.com/zenta-dev/zever/log"
)

type slogEvent struct {
	logger *stdslog.Logger
	level  stdslog.Level
	attrs  []stdslog.Attr
}

func (e *slogEvent) Str(key, val string) log.Event {
	e.attrs = append(e.attrs, stdslog.String(key, val))
	return e
}

func (e *slogEvent) Int(key string, val int) log.Event {
	e.attrs = append(e.attrs, stdslog.Int(key, val))
	return e
}

func (e *slogEvent) Int64(key string, val int64) log.Event {
	e.attrs = append(e.attrs, stdslog.Int64(key, val))
	return e
}

func (e *slogEvent) Float64(key string, val float64) log.Event {
	e.attrs = append(e.attrs, stdslog.Float64(key, val))
	return e
}

func (e *slogEvent) Bool(key string, val bool) log.Event {
	e.attrs = append(e.attrs, stdslog.Bool(key, val))
	return e
}

func (e *slogEvent) Dur(key string, val time.Duration) log.Event {
	e.attrs = append(e.attrs, stdslog.Duration(key, val))
	return e
}

func (e *slogEvent) Time(key string, val time.Time) log.Event {
	e.attrs = append(e.attrs, stdslog.Time(key, val))
	return e
}

func (e *slogEvent) Err(err error) log.Event {
	if err == nil {
		return e
	}

	e.attrs = append(e.attrs, stdslog.String("error", err.Error()))
	return e
}

func (e *slogEvent) AnErr(key string, err error) log.Event {
	if err == nil {
		return e
	}

	e.attrs = append(e.attrs, stdslog.String(key, err.Error()))
	return e
}

func (e *slogEvent) Any(key string, val any) log.Event {
	e.attrs = append(e.attrs, stdslog.Any(key, val))
	return e
}

func (e *slogEvent) Msg(msg string)                  { e.emit(msg) }
func (e *slogEvent) Msgf(format string, args ...any) { e.emit(fmt.Sprintf(format, args...)) }
func (e *slogEvent) Send()                           { e.emit("") }

func (e *slogEvent) emit(msg string) {
	e.logger.LogAttrs(context.Background(), e.level, msg, e.attrs...)
}
