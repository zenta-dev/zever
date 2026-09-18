package log_test

import (
	"github.com/zenta-dev/zever/log"
	"github.com/zenta-dev/zever/log/noop"
)

// ExampleOpen emits one info event through the noop adapter.
func ExampleOpen() {
	_ = log.Register(log.Noop, func(log.Options) (log.Logger, error) { return noop.New(), nil })

	logger, err := log.Open(log.Noop, log.Options{})
	if err != nil {
		return
	}

	if logger.Enabled(log.LevelInfo) {
		logger.Info().Str("service", "example").Msg("hello")
	}

	_ = logger.Sync()
}
