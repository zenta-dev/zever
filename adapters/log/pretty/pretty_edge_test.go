package pretty

import (
	"strings"
	"testing"

	"github.com/zenta-dev/zever/core/log"
)

func TestEdge_FormatFieldQuotesWhitespace(t *testing.T) {
	t.Parallel()

	for name, f := range map[string]log.Field{
		"space":   log.String("k", "a b"),
		"tab":     log.String("k", "a\tb"),
		"newline": log.String("k", "a\nb"),
	} {
		if got := formatField(f); !strings.HasPrefix(got, `"`) {
			t.Errorf("formatField(%s) = %q, want quoted", name, got)
		}
	}

	if got := formatField(log.String("k", "plain")); got != "plain" {
		t.Errorf("formatField(plain) = %q, want unquoted", got)
	}
}

func TestEdge_EnabledBoundaries(t *testing.T) {
	t.Parallel()

	var out buffer

	l := newTestLogger(&out, log.LevelError, false)

	for _, tc := range []struct {
		level log.Level
		want  bool
	}{
		{log.LevelDebug, false},
		{log.LevelInfo, false},
		{log.LevelWarn, false},
		{log.LevelError, true},
		{log.LevelFatal, true},
	} {
		if got := l.Enabled(tc.level); got != tc.want {
			t.Errorf("Enabled(%v) = %v, want %v", tc.level, got, tc.want)
		}
	}
}

func TestEdge_LevelStyleUnknown(t *testing.T) {
	t.Parallel()

	label, color := levelStyle(log.Level(99))
	if label != "UNKNOWN" || color != colorReset {
		t.Fatalf("levelStyle(99) = (%q, %q), want (UNKNOWN, reset)", label, color)
	}
}

func TestEdge_WithContextNoRequestID(t *testing.T) {
	t.Parallel()

	var out buffer

	l := newTestLogger(&out, log.LevelDebug, false)
	l.WithContext(t.Context()).Info().Msg("plain")

	if got := out.String(); strings.Contains(got, "[") {
		t.Fatalf("output = %q, want no request-id prefix", got)
	}
}

func TestEdge_ContextLoggerChildFields(t *testing.T) {
	t.Parallel()

	var out buffer

	l := newTestLogger(&out, log.LevelDebug, false)
	l.With().Str("service", "api").Int("version", 2).Logger().Info().Msg("child")

	got := out.String()
	for _, want := range []string{"service=api", "version=2", "child"} {
		if !strings.Contains(got, want) {
			t.Fatalf("output = %q, want %q", got, want)
		}
	}
}

func TestEdge_RequestIDFallsBackToLogger(t *testing.T) {
	t.Parallel()

	var out buffer

	got := newTestLogger(&out, log.LevelDebug, false)

	l, ok := got.(*logger)
	if !ok {
		t.Fatalf("logger type = %T, want *logger", got)
	}

	l.requestID = "req9"

	// An event with no id of its own falls back to the logger's id.
	ev := &prettyEvent{l: l, level: log.LevelInfo}
	ev.Msg("fallback")

	if got := out.String(); !strings.Contains(got, "[req9]") {
		t.Fatalf("output = %q, want fallback request id", got)
	}
}

func TestEdge_Truthy(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		value string
		want  bool
	}{
		{"1", true}, {"true", true}, {"yes", true}, {"on", true},
		{"0", false}, {"false", false}, {"no", false}, {"off", false}, {"", false}, {" maybe ", false},
	} {
		if got := truthy(tc.value); got != tc.want {
			t.Errorf("truthy(%q) = %v, want %v", tc.value, got, tc.want)
		}
	}
}
