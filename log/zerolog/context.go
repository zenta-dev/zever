package zerolog

import (
	"time"

	zl "github.com/rs/zerolog"

	"github.com/zenta-dev/zever/log"
)

type zerologContext struct {
	ctx zl.Context
}

func (c *zerologContext) Str(key, val string) log.Context { c.ctx = c.ctx.Str(key, val); return c }

func (c *zerologContext) Int(key string, val int) log.Context { c.ctx = c.ctx.Int(key, val); return c }

func (c *zerologContext) Int64(key string, val int64) log.Context {
	c.ctx = c.ctx.Int64(key, val)
	return c
}

func (c *zerologContext) Float64(key string, val float64) log.Context {
	c.ctx = c.ctx.Float64(key, val)
	return c
}

func (c *zerologContext) Bool(key string, val bool) log.Context {
	c.ctx = c.ctx.Bool(key, val)
	return c
}

func (c *zerologContext) Dur(key string, val time.Duration) log.Context {
	c.ctx = c.ctx.Dur(key, val)
	return c
}

func (c *zerologContext) Time(key string, val time.Time) log.Context {
	c.ctx = c.ctx.Time(key, val)
	return c
}
func (c *zerologContext) Err(err error) log.Context { c.ctx = c.ctx.Err(err); return c }
func (c *zerologContext) AnErr(key string, err error) log.Context {
	c.ctx = c.ctx.AnErr(key, err)
	return c
}

func (c *zerologContext) Any(key string, val any) log.Context {
	c.ctx = c.ctx.Interface(key, val)
	return c
}

func (c *zerologContext) Logger() log.Logger { return &zerologAdapter{log: c.ctx.Logger()} }
