package log

import "time"

// Event is a single structured log entry under construction.
type Event interface {
	// Str appends a string field to the event.
	Str(key, val string) Event
	// Int appends an int field to the event.
	Int(key string, val int) Event
	// Int64 appends an int64 field to the event.
	Int64(key string, val int64) Event
	// Float64 appends a float64 field to the event.
	Float64(key string, val float64) Event
	// Bool appends a bool field to the event.
	Bool(key string, val bool) Event
	// Dur appends a duration field to the event.
	Dur(key string, val time.Duration) Event
	// Time appends a time field to the event.
	Time(key string, val time.Time) Event
	// Err appends an error under the conventional "error" key.
	Err(err error) Event
	// AnErr appends an error under a custom key.
	AnErr(key string, err error) Event
	// Any appends an arbitrary value to the event.
	Any(key string, val any) Event
	// Msg emits the event with the given message.
	Msg(msg string)
	// Msgf emits the event with a formatted message.
	Msgf(format string, args ...any)
	// Send emits the event without a message.
	Send()
}
