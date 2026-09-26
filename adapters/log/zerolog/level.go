package zerolog

import (
	zl "github.com/rs/zerolog"

	"github.com/zenta-dev/zever/log"
)

func toZeroLogLevel(l log.Level) zl.Level {
	switch l {
	case log.LevelDebug:
		return zl.DebugLevel
	case log.LevelInfo:
		return zl.InfoLevel
	case log.LevelWarn:
		return zl.WarnLevel
	case log.LevelError:
		return zl.ErrorLevel
	case log.LevelFatal:
		return zl.FatalLevel
	default:
		return zl.InfoLevel
	}
}
