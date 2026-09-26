package slog

import (
	stdslog "log/slog"

	"github.com/zenta-dev/zever/log"
)

// fatalLevel is log.LevelFatal's slog equivalent. slog has no fatal level, so
// it sits above error to keep fatal-only filtering meaningful.
const fatalLevel = stdslog.LevelError + 4

func toSlogLevel(l log.Level) stdslog.Level {
	switch l {
	case log.LevelDebug:
		return stdslog.LevelDebug
	case log.LevelInfo:
		return stdslog.LevelInfo
	case log.LevelWarn:
		return stdslog.LevelWarn
	case log.LevelError:
		return stdslog.LevelError
	case log.LevelFatal:
		return fatalLevel
	default:
		return stdslog.LevelInfo
	}
}
