package slog

import (
	"github.com/zenta-dev/zever/core/log"
)

// Register wires this adapter into its battery registry. Call from your app's main or generated app.go; no init magic.
func Register() {
	_ = log.Register(log.Slog, func(o log.Options) (log.Logger, error) { return New(o), nil })
}
