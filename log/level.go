package log

// Level is the severity of a log event.
type Level uint8

const (
	// LevelDebug marks verbose diagnostic events.
	LevelDebug Level = iota
	// LevelInfo marks normal operational events.
	LevelInfo
	// LevelWarn marks potentially harmful situations.
	LevelWarn
	// LevelError marks failures that do not stop execution.
	LevelError
	// LevelFatal marks failures that terminate execution.
	LevelFatal
)

// String returns the canonical lowercase name of the level.
func (l Level) String() string {
	switch l {
	case LevelDebug:
		return "debug"
	case LevelInfo:
		return "info"
	case LevelWarn:
		return "warn"
	case LevelError:
		return "error"
	case LevelFatal:
		return "fatal"
	default:
		return "unknown"
	}
}

// ParseLevel converts a canonical level name into a Level.
func ParseLevel(level string) (Level, error) {
	switch level {
	case "debug":
		return LevelDebug, nil
	case "info":
		return LevelInfo, nil
	case "warn":
		return LevelWarn, nil
	case "error":
		return LevelError, nil
	case "fatal":
		return LevelFatal, nil
	default:
		return LevelDebug, &InvalidLevelError{Level: level}
	}
}
