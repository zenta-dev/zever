package noop

import (
	"context"

	"github.com/zenta-dev/zever/core/log"
)

type noopLogger struct{}

// New returns a Logger that discards all events.
func New() log.Logger { return noopLogger{} }

func (noopLogger) Debug() log.Event                         { return noopEventInstance }
func (noopLogger) Info() log.Event                          { return noopEventInstance }
func (noopLogger) Warn() log.Event                          { return noopEventInstance }
func (noopLogger) Error() log.Event                         { return noopEventInstance }
func (noopLogger) Fatal() log.Event                         { return noopEventInstance }
func (noopLogger) With() log.Context                        { return noopContextInstance }
func (n noopLogger) WithContext(context.Context) log.Logger { return n }
func (noopLogger) Enabled(log.Level) bool                   { return false }
func (noopLogger) Sync() error                              { return nil }
func (noopLogger) Name() string                             { return "noop" }

type noopContext struct{}
