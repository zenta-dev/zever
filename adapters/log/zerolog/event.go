package zerolog

import (
	"time"

	zl "github.com/rs/zerolog"

	"github.com/zenta-dev/zever/log"
)

type zerologEvent struct {
	e *zl.Event
}

func wrapZerologEvent(e *zl.Event) log.Event { return &zerologEvent{e: e} }

func (w *zerologEvent) Str(key, val string) log.Event         { w.e.Str(key, val); return w }
func (w *zerologEvent) Int(key string, val int) log.Event     { w.e.Int(key, val); return w }
func (w *zerologEvent) Int64(key string, val int64) log.Event { w.e.Int64(key, val); return w }

func (w *zerologEvent) Float64(key string, val float64) log.Event   { w.e.Float64(key, val); return w }
func (w *zerologEvent) Bool(key string, val bool) log.Event         { w.e.Bool(key, val); return w }
func (w *zerologEvent) Dur(key string, val time.Duration) log.Event { w.e.Dur(key, val); return w }
func (w *zerologEvent) Time(key string, val time.Time) log.Event    { w.e.Time(key, val); return w }
func (w *zerologEvent) Err(err error) log.Event                     { w.e.Err(err); return w }
func (w *zerologEvent) AnErr(key string, err error) log.Event       { w.e.AnErr(key, err); return w }

func (w *zerologEvent) Any(key string, val any) log.Event { w.e.Interface(key, val); return w }

func (w *zerologEvent) Msg(msg string)                  { w.e.Msg(msg) }
func (w *zerologEvent) Msgf(format string, args ...any) { w.e.Msgf(format, args...) }
func (w *zerologEvent) Send()                           { w.e.Send() }
