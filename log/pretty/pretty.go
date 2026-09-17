package pretty

import (
	"context"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/zenta-dev/zever/log"
	"github.com/zenta-dev/zever/observability"
)

const (
	colorReset  = "\033[0m"
	colorGray   = "\033[90m"
	colorBlue   = "\033[34m"
	colorGreen  = "\033[32m"
	colorYellow = "\033[33m"
	colorRed    = "\033[31m"
	colorBold   = "\033[1m"
	colorCyan   = "\033[36m"
)

const defaultTimeFormat = "15:04:05.000"

// logger is a human-readable log.Logger writing one line per event.
// The mutex is shared by pointer so child loggers built by WithContext and
// Context.Logger serialize on the same writer.
type logger struct {
	mu         *sync.Mutex
	out        io.Writer
	minLevel   log.Level
	fixed      []log.Field
	useColor   bool
	timeFormat string
	requestID  string
}

// New creates a Logger writing human-readable lines to stdout at the
// configured minimum level. An unparseable level falls back to info.
func New(opts Options) log.Logger {
	return NewWithWriter(opts, os.Stdout)
}

// NewWithWriter creates a Logger writing human-readable lines to out at
// the configured minimum level. A nil out falls back to stdout. Color is
// forced by opts.Color when set, otherwise auto-detected from out and the
// NO_COLOR/FORCE_COLOR/TERM environment.
func NewWithWriter(opts Options, out io.Writer) log.Logger {
	if out == nil {
		out = os.Stdout
	}

	minLevel := log.LevelInfo

	if opts.Level != "" {
		if parsed, err := log.ParseLevel(opts.Level); err == nil {
			minLevel = parsed
		}
	}

	return &logger{
		mu:         &sync.Mutex{},
		out:        out,
		minLevel:   minLevel,
		useColor:   autoColor(out, opts.Color),
		timeFormat: defaultTimeFormat,
	}
}

// autoColor decides whether ANSI escapes may be emitted. An explicit force
// always wins. Otherwise color requires all of: NO_COLOR unset (a
// NO_COLOR of "0" counts as unset, permitting explicit enabling),
// TERM not "dumb", no CI marker unless FORCE_COLOR opts in, and out being
// a character-device file (pipes, redirects, and buffers never color).
func autoColor(out io.Writer, force *bool) bool {
	if force != nil {
		return *force
	}

	if noColor := os.Getenv("NO_COLOR"); noColor != "" && noColor != "0" {
		return false
	}

	if os.Getenv("TERM") == "dumb" {
		return false
	}

	if os.Getenv("CI") != "" && !truthy(os.Getenv("FORCE_COLOR")) {
		return false
	}

	if truthy(os.Getenv("FORCE_COLOR")) {
		return isCharDevice(out)
	}

	return isCharDevice(out)
}

// truthy reports whether an env value opts into a feature.
func truthy(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

// isCharDevice reports whether out is a character device, using only the
// standard library: pipes, files, and buffers are never terminals.
func isCharDevice(out io.Writer) bool {
	f, ok := out.(*os.File)
	if !ok {
		return false
	}

	st, err := f.Stat()
	if err != nil {
		return false
	}

	return st.Mode()&os.ModeCharDevice != 0
}

// levelStyle returns a level's label and color.
func levelStyle(l log.Level) (label, color string) {
	switch l {
	case log.LevelDebug:
		return "DEBUG", colorGray
	case log.LevelInfo:
		return "INFO ", colorBlue
	case log.LevelWarn:
		return "WARN ", colorYellow
	case log.LevelError:
		return "ERROR", colorRed
	case log.LevelFatal:
		return "FATAL", colorRed
	default:
		return "UNKNOWN", colorReset
	}
}

// Name returns the adapter name backing this logger.
func (l *logger) Name() string { return "pretty" }

// Enabled reports whether the given level will be emitted.
func (l *logger) Enabled(level log.Level) bool { return level >= l.minLevel }

// Sync flushes buffered output. Pretty output is unbuffered, so Sync is a
// no-op reporting success.
func (l *logger) Sync() error { return nil }

// Debug starts a debug-level event.
func (l *logger) Debug() log.Event { return l.event(log.LevelDebug) }

// Info starts an info-level event.
func (l *logger) Info() log.Event { return l.event(log.LevelInfo) }

// Warn starts a warn-level event.
func (l *logger) Warn() log.Event { return l.event(log.LevelWarn) }

// Error starts an error-level event.
func (l *logger) Error() log.Event { return l.event(log.LevelError) }

// Fatal starts a fatal-level event.
func (l *logger) Fatal() log.Event { return l.event(log.LevelFatal) }

// With returns a Context for building a child logger with more fields.
func (l *logger) With() log.Context {
	return &prettyContext{l: l, fields: append([]log.Field(nil), l.fixed...)}
}

// WithContext returns a Logger carrying the request id in ctx, if any.
func (l *logger) WithContext(ctx context.Context) log.Logger {
	cp := *l
	cp.fixed = append([]log.Field(nil), l.fixed...)

	if id := observability.RequestIDFromContext(ctx); id != "" {
		cp.requestID = id
	}

	return &cp
}

// event starts an event at the given level, inheriting fixed fields and
// the request id captured via WithContext.
func (l *logger) event(level log.Level) log.Event {
	return &prettyEvent{l: l, level: level, fields: append([]log.Field(nil), l.fixed...), requestID: l.requestID}
}

// emit renders and writes one line. Output and color are fixed at
// construction, so a disabled level is the only skip.
func (l *logger) emit(level log.Level, msg string, fields []log.Field, requestID string) {
	if level < l.minLevel {
		return
	}

	var b strings.Builder

	writeTimestamp(&b, l.timeFormat, l.useColor)
	writeLevel(&b, level, l.useColor)

	if requestID != "" {
		writeRequestID(&b, requestID, l.useColor)
	}

	b.WriteString(msg)

	if len(fields) > 0 {
		b.WriteByte(' ')
		writeFields(&b, fields, l.useColor)
	}

	b.WriteByte('\n')

	l.mu.Lock()
	_, err := io.WriteString(l.out, b.String())
	l.mu.Unlock()

	if err != nil {
		fmt.Fprintf(os.Stderr, "[pretty] write failed: %v\n", err)
	}
}

// writeTimestamp appends the current time.
func writeTimestamp(b *strings.Builder, format string, useColor bool) {
	ts := time.Now().Format(format)

	if useColor {
		b.WriteString(colorGray)
		b.WriteString(ts)
		b.WriteString(colorReset)
	} else {
		b.WriteString(ts)
	}

	b.WriteByte(' ')
}

// writeLevel appends the level label, bold for warn and above.
func writeLevel(b *strings.Builder, level log.Level, useColor bool) {
	label, color := levelStyle(level)

	if useColor {
		style := color
		if level >= log.LevelWarn {
			style = colorBold + color
		}

		b.WriteString(style)
		b.WriteString(label)
		b.WriteString(colorReset)
	} else {
		b.WriteString(label)
	}

	b.WriteByte(' ')
}

// writeRequestID appends the request correlation prefix.
func writeRequestID(b *strings.Builder, id string, useColor bool) {
	if useColor {
		b.WriteString(colorCyan)
	}

	b.WriteByte('[')
	b.WriteString(id)
	b.WriteByte(']')

	if useColor {
		b.WriteString(colorReset)
	}

	b.WriteByte(' ')
}

// writeFields appends sorted key=value pairs.
func writeFields(b *strings.Builder, fields []log.Field, useColor bool) {
	sorted := append([]log.Field(nil), fields...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Key < sorted[j].Key })

	for i, f := range sorted {
		if i > 0 {
			b.WriteByte(' ')
		}

		if useColor {
			keyColor := colorGray
			if f.Key == "error" || f.Key == "err" {
				keyColor = colorRed
			}

			b.WriteString(keyColor)
			b.WriteString(f.Key)
			b.WriteByte('=')
			b.WriteString(colorReset)
		} else {
			b.WriteString(f.Key)
			b.WriteByte('=')
		}

		b.WriteString(formatField(f))
	}
}

// formatField renders one field value, quoting values with whitespace.
func formatField(f log.Field) string {
	var s string

	switch f.Type {
	case log.StringType:
		s = f.String
	case log.IntType, log.Int64Type:
		s = strconv.FormatInt(f.Int64, 10)
	case log.Float64Type:
		s = fmt.Sprintf("%v", f.Float64)
	case log.BoolType:
		s = strconv.FormatBool(f.Bool)
	case log.DurationType:
		s = f.Duration.String()
	case log.TimeType:
		s = f.Time.Format(time.RFC3339)
	case log.ErrorType:
		s = fmt.Sprintf("%v", f.Err)
	default:
		s = fmt.Sprintf("%v", f.Any)
	}

	if strings.ContainsAny(s, " \t\n") {
		return fmt.Sprintf("%q", s)
	}

	return s
}

// prettyEvent is a single log entry under construction.
type prettyEvent struct {
	l         *logger
	level     log.Level
	fields    []log.Field
	requestID string
}

// Str appends a string field to the event.
func (e *prettyEvent) Str(key, val string) log.Event {
	e.fields = append(e.fields, log.String(key, val))
	return e
}

// Int appends an int field to the event.
func (e *prettyEvent) Int(key string, val int) log.Event {
	e.fields = append(e.fields, log.Int(key, val))
	return e
}

// Int64 appends an int64 field to the event.
func (e *prettyEvent) Int64(key string, val int64) log.Event {
	e.fields = append(e.fields, log.Int64(key, val))
	return e
}

// Float64 appends a float64 field to the event.
func (e *prettyEvent) Float64(key string, val float64) log.Event {
	e.fields = append(e.fields, log.Float64(key, val))
	return e
}

// Bool appends a bool field to the event.
func (e *prettyEvent) Bool(key string, val bool) log.Event {
	e.fields = append(e.fields, log.Bool(key, val))
	return e
}

// Dur appends a duration field to the event.
func (e *prettyEvent) Dur(key string, val time.Duration) log.Event {
	e.fields = append(e.fields, log.Duration(key, val))
	return e
}

// Time appends a time field to the event.
func (e *prettyEvent) Time(key string, val time.Time) log.Event {
	e.fields = append(e.fields, log.Time(key, val))
	return e
}

// Err appends an error under the conventional "error" key.
func (e *prettyEvent) Err(err error) log.Event {
	e.fields = append(e.fields, log.Err(err))
	return e
}

// AnErr appends an error under a custom key.
func (e *prettyEvent) AnErr(key string, err error) log.Event {
	e.fields = append(e.fields, log.ErrKey(key, err))
	return e
}

// Any appends an arbitrary value to the event.
func (e *prettyEvent) Any(key string, val any) log.Event {
	e.fields = append(e.fields, log.Any(key, val))
	return e
}

// Msg emits the event with the given message.
func (e *prettyEvent) Msg(msg string) {
	e.l.emit(e.level, msg, e.fields, e.requestIDOrLogger())
}

// Msgf emits the event with a formatted message.
func (e *prettyEvent) Msgf(format string, args ...any) {
	e.l.emit(e.level, fmt.Sprintf(format, args...), e.fields, e.requestIDOrLogger())
}

// Send emits the event without a message.
func (e *prettyEvent) Send() {
	e.l.emit(e.level, "", e.fields, e.requestIDOrLogger())
}

// requestIDOrLogger falls back to the logger's request id captured via
// WithContext when the event carries none of its own.
func (e *prettyEvent) requestIDOrLogger() string {
	if e.requestID != "" {
		return e.requestID
	}

	return e.l.requestID
}

// prettyContext accumulates fields for a child logger.
type prettyContext struct {
	l      *logger
	fields []log.Field
}

// Str appends a string field to the context.
func (c *prettyContext) Str(key, val string) log.Context {
	c.fields = append(c.fields, log.String(key, val))
	return c
}

// Int appends an int field to the context.
func (c *prettyContext) Int(key string, val int) log.Context {
	c.fields = append(c.fields, log.Int(key, val))
	return c
}

// Int64 appends an int64 field to the context.
func (c *prettyContext) Int64(key string, val int64) log.Context {
	c.fields = append(c.fields, log.Int64(key, val))
	return c
}

// Float64 appends a float64 field to the context.
func (c *prettyContext) Float64(key string, val float64) log.Context {
	c.fields = append(c.fields, log.Float64(key, val))
	return c
}

// Bool appends a bool field to the context.
func (c *prettyContext) Bool(key string, val bool) log.Context {
	c.fields = append(c.fields, log.Bool(key, val))
	return c
}

// Dur appends a duration field to the context.
func (c *prettyContext) Dur(key string, val time.Duration) log.Context {
	c.fields = append(c.fields, log.Duration(key, val))
	return c
}

// Time appends a time field to the context.
func (c *prettyContext) Time(key string, val time.Time) log.Context {
	c.fields = append(c.fields, log.Time(key, val))
	return c
}

// Err appends an error under the conventional "error" key.
func (c *prettyContext) Err(err error) log.Context {
	c.fields = append(c.fields, log.Err(err))
	return c
}

// AnErr appends an error under a custom key.
func (c *prettyContext) AnErr(key string, err error) log.Context {
	c.fields = append(c.fields, log.ErrKey(key, err))
	return c
}

// Any appends an arbitrary value to the context.
func (c *prettyContext) Any(key string, val any) log.Context {
	c.fields = append(c.fields, log.Any(key, val))
	return c
}

// Logger builds a Logger carrying the accumulated fields.
func (c *prettyContext) Logger() log.Logger {
	cp := *c.l
	cp.fixed = append([]log.Field(nil), c.fields...)

	return &cp
}
