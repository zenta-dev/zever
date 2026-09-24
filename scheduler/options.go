package scheduler

import (
	"time"

	"github.com/zenta-dev/zever/job"
	"github.com/zenta-dev/zever/log"
)

const (
	// DefaultCloseTimeout is the default shutdown wait applied by adapters.
	DefaultCloseTimeout = 5 * time.Second
	// MaxSpecLen is the maximum allowed cron spec length.
	MaxSpecLen = 256
)

// Options configures scheduler behavior.
type Options struct {
	// Dispatcher enqueues jobs when a cron tick fires. Required.
	Dispatcher *job.Dispatcher `json:"-" toml:"-" yaml:"-"`
	// Locker deduplicates each schedule slot across scheduler instances.
	// Nil means no cross-instance dedup (single-instance mode).
	Locker *job.UniqueLocker `json:"-" toml:"-" yaml:"-"`
	// Logger reports schedule errors. Zero means the adapter default.
	Logger log.Logger `json:"-" toml:"-" yaml:"-"`
	// CloseTimeout bounds Stop's wait for running ticks.
	// Zero means DefaultCloseTimeout; negative fails validation.
	CloseTimeout time.Duration `json:"close_timeout" toml:"close_timeout" yaml:"close_timeout"`
}

// Validate checks options for consistency, joining all violations.
func (o Options) Validate() error {
	if o.Dispatcher == nil {
		return &InvalidOptionsError{Reason: "dispatcher is required"}
	}

	if o.CloseTimeout < 0 {
		return &InvalidOptionsError{Reason: "close_timeout must be >= 0"}
	}

	return nil
}
