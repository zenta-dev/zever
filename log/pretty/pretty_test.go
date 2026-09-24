package pretty

import (
	"errors"
	"io"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/zenta-dev/zever/log"
	"github.com/zenta-dev/zever/observability"
)

type buffer struct {
	mu sync.Mutex
	sb strings.Builder
}

func (b *buffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	return b.sb.Write(p)
}

func (b *buffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()

	return b.sb.String()
}

type failWriter struct{}

func (failWriter) Write([]byte) (int, error) { return 0, errors.New("write failed") }

// newTestLogger builds a logger directly, bypassing New/NewWithWriter, so
// tests can force color on or off deterministically without depending on a
// real terminal. Production code always goes through NewWithWriter, whose
// own auto-detection (no forcing) is covered separately below.
func newTestLogger(out *buffer, minLevel log.Level, forceColor bool) log.Logger {
	return &logger{
		mu:         &sync.Mutex{},
		out:        out,
		minLevel:   resolveMinLevel(minLevel),
		useColor:   forceColor,
		timeFormat: defaultTimeFormat,
	}
}

func TestPretty_infoLine(t *testing.T) {
	t.Parallel()

	var out buffer
	l := newTestLogger(&out, log.LevelDebug, false)
	l.Info().Str("key", "value").Msg("hello")

	got := out.String()
	for _, want := range []string{"INFO", "hello", "key=value"} {
		if !strings.Contains(got, want) {
			t.Fatalf("output = %q, want %q", got, want)
		}
	}

	if strings.Contains(got, "\x1b[") {
		t.Fatalf("output = %q, want no ANSI escapes with color disabled", got)
	}
}

func TestPretty_levelsRender(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		emit  func(l log.Logger) log.Event
		label string
	}{
		{"debug", func(l log.Logger) log.Event { return l.Debug() }, "DEBUG"},
		{"info", func(l log.Logger) log.Event { return l.Info() }, "INFO"},
		{"warn", func(l log.Logger) log.Event { return l.Warn() }, "WARN"},
		{"error", func(l log.Logger) log.Event { return l.Error() }, "ERROR"},
		{"fatal", func(l log.Logger) log.Event { return l.Fatal() }, "FATAL"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var out buffer
			l := newTestLogger(&out, log.LevelDebug, false)
			tt.emit(l).Msg("m")

			if !strings.Contains(out.String(), tt.label) {
				t.Fatalf("output = %q, want label %q", out.String(), tt.label)
			}
		})
	}
}

func TestPretty_minLevelFilters(t *testing.T) {
	t.Parallel()

	var out buffer
	l := newTestLogger(&out, log.LevelError, false)
	l.Info().Msg("should-not-appear")

	if got := out.String(); got != "" {
		t.Fatalf("output = %q, want empty (info below error)", got)
	}
}

func TestPretty_zeroOptionsDefaultsToDebug(t *testing.T) {
	t.Parallel()

	var out buffer
	l := NewWithWriter(log.Options{}, &out)
	l.Debug().Msg("should-appear")

	if got := out.String(); !strings.Contains(got, "should-appear") {
		t.Fatalf("output = %q, want debug message at zero-value MinLevel", got)
	}
}

func TestPretty_unknownMinLevelFallsBackToInfo(t *testing.T) {
	t.Parallel()

	var out buffer
	l := NewWithWriter(log.Options{MinLevel: log.Level(99)}, &out)

	l.Debug().Msg("suppressed")
	l.Info().Msg("shown")

	got := out.String()
	if strings.Contains(got, "suppressed") {
		t.Fatalf("output = %q, want debug suppressed at fallback info level", got)
	}

	if !strings.Contains(got, "shown") {
		t.Fatalf("output = %q, want info message at fallback info level", got)
	}
}

func TestPretty_sortedFields(t *testing.T) {
	t.Parallel()

	var out buffer
	l := newTestLogger(&out, log.LevelDebug, false)
	l.Info().Int("zebra", 1).Int("apple", 2).Msg("sort")

	got := out.String()
	iApple := strings.Index(got, "apple=2")
	iZebra := strings.Index(got, "zebra=1")

	if iApple == -1 || iZebra == -1 {
		t.Fatalf("output = %q, want both fields", got)
	}

	if iApple > iZebra {
		t.Fatalf("output = %q, fields not sorted", got)
	}
}

func TestPretty_allFieldTypes(t *testing.T) {
	t.Parallel()

	var out buffer
	l := newTestLogger(&out, log.LevelDebug, false)
	l.Info().
		Str("s", "v").
		Int("i", -3).
		Int64("i64", 9).
		Float64("f", 1.5).
		Bool("b", true).
		Dur("d", 2*time.Second).
		Time("t", time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)).
		Err(errors.New("boom")).
		AnErr("custom", errors.New("bam")).
		Any("any", 42).
		Msg("types")

	got := out.String()
	for _, want := range []string{
		"s=v", "i=-3", "i64=9", "f=1.5", "b=true", "d=2s",
		"error=boom", "custom=bam", "any=42", "2026-01-02",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("output = %q, want %q", got, want)
		}
	}
}

func TestPretty_valueWithSpacesIsQuoted(t *testing.T) {
	t.Parallel()

	var out buffer
	l := newTestLogger(&out, log.LevelDebug, false)
	l.Info().Str("msg", "hello world").Msg("q")

	if got := out.String(); !strings.Contains(got, `msg="hello world"`) {
		t.Fatalf("output = %q, want quoted value", got)
	}
}

func TestPretty_msgfAndSend(t *testing.T) {
	t.Parallel()

	var out buffer
	l := newTestLogger(&out, log.LevelDebug, false)
	l.Info().Int("n", 7).Msgf("count %d", 7)

	if got := out.String(); !strings.Contains(got, "count 7") {
		t.Fatalf("output = %q, want formatted message", got)
	}

	out.sb.Reset()
	l.Info().Str("k", "v").Send()

	if got := out.String(); !strings.Contains(got, "k=v") {
		t.Fatalf("output = %q, want field from Send", got)
	}
}

func TestPretty_withChildLogger(t *testing.T) {
	t.Parallel()

	var out buffer
	l := newTestLogger(&out, log.LevelDebug, false)
	child := l.With().Str("service", "api").Logger()
	child.Info().Msg("child")

	got := out.String()
	if !strings.Contains(got, "service=api") {
		t.Fatalf("output = %q, want inherited context field", got)
	}

	if !strings.Contains(got, "child") {
		t.Fatalf("output = %q, want message", got)
	}
}

func TestPretty_withContextCarriesRequestID(t *testing.T) {
	t.Parallel()

	var out buffer
	l := newTestLogger(&out, log.LevelDebug, false)
	ctx := observability.WithRequestID(t.Context(), "req1234")
	l.WithContext(ctx).Info().Msg("got request")

	got := out.String()
	if !strings.Contains(got, "[req1234]") {
		t.Fatalf("output = %q, want request id prefix", got)
	}

	if !strings.Contains(got, "got request") {
		t.Fatalf("output = %q, want message", got)
	}
}

func TestPretty_colorEnabledEmitsANSI(t *testing.T) {
	t.Parallel()

	var out buffer
	l := newTestLogger(&out, log.LevelDebug, true)
	l.Info().Str("error", "x").Msg("colored")

	if got := out.String(); !strings.Contains(got, "\x1b[") {
		t.Fatalf("output = %q, want ANSI escapes with color forced on", got)
	}
}

func TestPretty_noColorEnvDisablesAutoColor(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	t.Setenv("FORCE_COLOR", "")
	t.Setenv("TERM", "xterm")

	var out buffer
	l := NewWithWriter(log.Options{MinLevel: log.LevelDebug}, &out)
	l.Info().Msg("plain")

	if got := out.String(); strings.Contains(got, "\x1b[") {
		t.Fatalf("output = %q, want no ANSI with NO_COLOR set", got)
	}
}

func TestPretty_dumbTermDisablesAutoColor(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	t.Setenv("FORCE_COLOR", "")
	t.Setenv("TERM", "dumb")

	var out buffer
	l := NewWithWriter(log.Options{MinLevel: log.LevelDebug}, &out)
	l.Info().Msg("plain")

	if got := out.String(); strings.Contains(got, "\x1b[") {
		t.Fatalf("output = %q, want no ANSI with TERM=dumb", got)
	}
}

func TestPretty_nonFileWriterNeverColors(t *testing.T) {
	t.Setenv("FORCE_COLOR", "1")
	t.Setenv("NO_COLOR", "")
	t.Setenv("TERM", "xterm")

	var out buffer
	l := NewWithWriter(log.Options{MinLevel: log.LevelDebug}, &out)
	l.Info().Msg("plain")

	if got := out.String(); strings.Contains(got, "\x1b[") {
		t.Fatalf("output = %q, want no ANSI for non-file writer", got)
	}
}

func TestPretty_nilWriterFallsBackToStdout(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}

	old := os.Stdout
	os.Stdout = w
	defer func() { os.Stdout = old }()

	var sb strings.Builder

	done := make(chan struct{})

	go func() {
		_, _ = io.Copy(&sb, r)
		close(done)
	}()

	l := NewWithWriter(log.Options{MinLevel: log.LevelDebug}, nil)
	if l.Name() != "pretty" {
		t.Fatalf("Name() = %q, want pretty", l.Name())
	}

	l.Info().Msg("to-stdout")

	_ = w.Close()
	<-done

	if !strings.Contains(sb.String(), "to-stdout") {
		t.Fatalf("output = %q, want message on stdout", sb.String())
	}
}

func TestPretty_ciDisablesAutoColor(t *testing.T) {
	t.Setenv("CI", "true")
	t.Setenv("NO_COLOR", "")
	t.Setenv("FORCE_COLOR", "")
	t.Setenv("TERM", "xterm")

	var out buffer
	l := NewWithWriter(log.Options{MinLevel: log.LevelDebug}, &out)
	l.Info().Msg("plain")

	if got := out.String(); strings.Contains(got, "\x1b[") {
		t.Fatalf("output = %q, want no ANSI under CI without FORCE_COLOR", got)
	}
}

func TestPretty_statErrorDisablesColor(t *testing.T) {
	t.Setenv("FORCE_COLOR", "1")
	t.Setenv("NO_COLOR", "")
	t.Setenv("TERM", "xterm")

	f, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatalf("open devnull: %v", err)
	}

	_ = f.Close() // Stat on a closed file fails.

	l := NewWithWriter(log.Options{MinLevel: log.LevelDebug}, f)
	l.Info().Msg("plain") // Must not panic; output discarded.
}

func TestPretty_unknownLevelRendersUnknown(t *testing.T) {
	t.Parallel()

	var out buffer
	gotLogger := newTestLogger(&out, log.LevelDebug, false)
	l, ok := gotLogger.(*logger)
	if !ok {
		t.Fatalf("logger = %T, want *logger", gotLogger)
	}
	l.emit(log.Level(99), "weird", nil, "")

	if got := out.String(); !strings.Contains(got, "UNKNOWN") {
		t.Fatalf("output = %q, want UNKNOWN label", got)
	}
}

func TestPretty_coloredWarnAndRequestID(t *testing.T) {
	t.Parallel()

	var out buffer
	l := newTestLogger(&out, log.LevelDebug, true)
	ctx := observability.WithRequestID(t.Context(), "req9")
	l.WithContext(ctx).Warn().Msg("watch out")

	got := out.String()
	for _, want := range []string{"\x1b[", "[req9]", "watch out"} {
		if !strings.Contains(got, want) {
			t.Fatalf("output = %q, want %q", got, want)
		}
	}
}
func TestPretty_writeErrorFallsBackToStderr(_ *testing.T) {
	l := NewWithWriter(log.Options{MinLevel: log.LevelDebug}, failWriter{})
	// Must not panic; the failure note goes to stderr.
	l.Info().Msg("broken")
}

func TestPretty_concurrentWritesNotCorrupted(t *testing.T) {
	t.Parallel()

	var out buffer
	l := newTestLogger(&out, log.LevelDebug, false)

	const n = 50

	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)

		go func(n int) {
			defer wg.Done()
			l.Info().Int("i", n).Msg("concurrent")
		}(i)
	}

	wg.Wait()

	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != n {
		t.Fatalf("got %d lines, want %d", len(lines), n)
	}

	for _, line := range lines {
		if !strings.Contains(line, "INFO") || !strings.Contains(line, "concurrent") {
			t.Fatalf("corrupted line: %q", line)
		}
	}
}

func TestPretty_contextChainers(t *testing.T) {
	t.Parallel()

	var out buffer
	l := newTestLogger(&out, log.LevelDebug, false)
	l.With().
		Str("s", "v").
		Int("i", 1).
		Int64("i64", 2).
		Float64("f", 1.5).
		Bool("b", true).
		Dur("d", time.Second).
		Time("t", time.Date(2026, 5, 6, 0, 0, 0, 0, time.UTC)).
		Err(errors.New("e")).
		AnErr("ae", errors.New("ae")).
		Any("a", "x").
		Logger().
		Info().Msg("ctx")

	got := out.String()
	for _, want := range []string{"s=v", "i=1", "i64=2", "f=1.5", "b=true", "error=e", "ae=ae", "a=x"} {
		if !strings.Contains(got, want) {
			t.Fatalf("output = %q, want %q", got, want)
		}
	}
}

func TestPretty_nilErrorField(t *testing.T) {
	t.Parallel()

	var out buffer
	l := newTestLogger(&out, log.LevelDebug, false)
	l.Info().Err(nil).Msg("nilerr")

	if got := out.String(); !strings.Contains(got, "error=<nil>") {
		t.Fatalf("output = %q, want nil error rendered", got)
	}
}
