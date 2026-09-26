package slog

import (
	"time"

	stdslog "log/slog"

	"github.com/zenta-dev/zever/log"
)

type slogContext struct {
	logger *stdslog.Logger
	args   []any
}

func (c *slogContext) Str(key, val string) log.Context {
	c.args = append(c.args, stdslog.String(key, val))
	return c
}

func (c *slogContext) Int(key string, val int) log.Context {
	c.args = append(c.args, stdslog.Int(key, val))
	return c
}

func (c *slogContext) Int64(key string, val int64) log.Context {
	c.args = append(c.args, stdslog.Int64(key, val))
	return c
}

func (c *slogContext) Float64(key string, val float64) log.Context {
	c.args = append(c.args, stdslog.Float64(key, val))
	return c
}

func (c *slogContext) Bool(key string, val bool) log.Context {
	c.args = append(c.args, stdslog.Bool(key, val))
	return c
}

func (c *slogContext) Dur(key string, val time.Duration) log.Context {
	c.args = append(c.args, stdslog.Duration(key, val))
	return c
}

func (c *slogContext) Time(key string, val time.Time) log.Context {
	c.args = append(c.args, stdslog.Time(key, val))
	return c
}

func (c *slogContext) Err(err error) log.Context {
	if err == nil {
		return c
	}

	c.args = append(c.args, stdslog.String("error", err.Error()))
	return c
}

func (c *slogContext) AnErr(key string, err error) log.Context {
	if err == nil {
		return c
	}

	c.args = append(c.args, stdslog.String(key, err.Error()))
	return c
}

func (c *slogContext) Any(key string, val any) log.Context {
	c.args = append(c.args, stdslog.Any(key, val))
	return c
}

func (c *slogContext) Logger() log.Logger {
	return &slogLogger{logger: c.logger.With(c.args...)}
}
