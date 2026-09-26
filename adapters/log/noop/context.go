package noop

import (
	"time"

	"github.com/zenta-dev/zever/core/log"
)

var noopContextInstance log.Context = noopContext{}

func (noopContext) Str(string, string) log.Context        { return noopContextInstance }
func (noopContext) Int(string, int) log.Context           { return noopContextInstance }
func (noopContext) Int64(string, int64) log.Context       { return noopContextInstance }
func (noopContext) Float64(string, float64) log.Context   { return noopContextInstance }
func (noopContext) Bool(string, bool) log.Context         { return noopContextInstance }
func (noopContext) Dur(string, time.Duration) log.Context { return noopContextInstance }
func (noopContext) Time(string, time.Time) log.Context    { return noopContextInstance }
func (noopContext) Err(error) log.Context                 { return noopContextInstance }
func (noopContext) AnErr(string, error) log.Context       { return noopContextInstance }
func (noopContext) Any(string, any) log.Context           { return noopContextInstance }
func (noopContext) Logger() log.Logger                    { return noopLogger{} }
