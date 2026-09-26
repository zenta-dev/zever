package noop

import (
	"time"

	"github.com/zenta-dev/zever/core/log"
)

type noopEvent struct{}

var noopEventInstance log.Event = noopEvent{}

func (noopEvent) Str(string, string) log.Event        { return noopEventInstance }
func (noopEvent) Int(string, int) log.Event           { return noopEventInstance }
func (noopEvent) Int64(string, int64) log.Event       { return noopEventInstance }
func (noopEvent) Float64(string, float64) log.Event   { return noopEventInstance }
func (noopEvent) Bool(string, bool) log.Event         { return noopEventInstance }
func (noopEvent) Dur(string, time.Duration) log.Event { return noopEventInstance }
func (noopEvent) Time(string, time.Time) log.Event    { return noopEventInstance }
func (noopEvent) Err(error) log.Event                 { return noopEventInstance }
func (noopEvent) AnErr(string, error) log.Event       { return noopEventInstance }
func (noopEvent) Any(string, any) log.Event           { return noopEventInstance }
func (noopEvent) Msg(string)                          {}
func (noopEvent) Msgf(string, ...any)                 {}
func (noopEvent) Send()                               {}
