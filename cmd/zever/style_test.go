package main

import (
	"strings"
	"testing"
)

func stubColor(t *testing.T, v bool) {
	t.Helper()
	old := colorEnabled
	t.Cleanup(func() { colorEnabled = old })
	colorEnabled = v
}

func TestStyleHelpers_plainWhenDisabled(t *testing.T) {
	stubColor(t, false)

	for name, fn := range map[string]func(string) string{
		"bold": bold, "dim": dim, "cyan": cyan, "green": green,
		"red": red, "yellow": yellow, "title": title, "cmd": cmd,
		"hint": hint, "success": success, "failure": failure,
	} {
		if got := fn("x"); got != "x" {
			t.Errorf("%s(x) = %q, want plain passthrough", name, got)
		}
	}
}

func TestStyleHelpers_wrappedWhenEnabled(t *testing.T) {
	stubColor(t, true)

	for name, fn := range map[string]func(string) string{
		"bold": bold, "dim": dim, "cyan": cyan, "green": green,
		"red": red, "yellow": yellow, "title": title, "cmd": cmd,
		"hint": hint, "success": success, "failure": failure,
	} {
		got := fn("x")
		if !strings.HasPrefix(got, "\x1b[") || !strings.HasSuffix(got, "\x1b[0m") || !strings.Contains(got, "x") {
			t.Errorf("%s(x) = %q, want ANSI-wrapped x", name, got)
		}
	}
}

func TestFormatHint_prefix(t *testing.T) {
	t.Run("plain", func(t *testing.T) {
		stubColor(t, false)
		if got := formatHint("msg"); got != "hint: msg" {
			t.Fatalf("formatHint = %q, want %q", got, "hint: msg")
		}
	})

	t.Run("color", func(t *testing.T) {
		stubColor(t, true)
		got := formatHint("msg")
		if !strings.Contains(got, "💡 hint: msg") {
			t.Fatalf("formatHint = %q, want 💡 prefix", got)
		}
	})
}

func TestMarks(t *testing.T) {
	t.Run("plain", func(t *testing.T) {
		stubColor(t, false)
		if got := successMark(); got != "OK" {
			t.Fatalf("successMark = %q, want OK", got)
		}
		if got := failMark(); got != "FAIL" {
			t.Fatalf("failMark = %q, want FAIL", got)
		}
	})

	t.Run("color", func(t *testing.T) {
		stubColor(t, true)
		if got := successMark(); !strings.Contains(got, "✓") {
			t.Fatalf("successMark = %q, want ✓", got)
		}
		if got := failMark(); !strings.Contains(got, "✗") {
			t.Fatalf("failMark = %q, want ✗", got)
		}
	})
}

func TestBox(t *testing.T) {
	t.Run("plain passthrough", func(t *testing.T) {
		stubColor(t, false)
		if got := box("a\nb"); got != "a\nb" {
			t.Fatalf("box = %q, want passthrough", got)
		}
	})

	t.Run("rounded frame", func(t *testing.T) {
		stubColor(t, true)
		got := box("hi")
		for _, want := range []string{"╭", "╮", "╰", "╯", "│ hi │"} {
			if !strings.Contains(got, want) {
				t.Fatalf("box(%q) missing %q:\n%s", "hi", want, got)
			}
		}
	})

	t.Run("pads to widest line", func(t *testing.T) {
		stubColor(t, true)
		got := box("a\nlonger")
		lines := strings.Split(got, "\n")
		if len(lines) != 4 {
			t.Fatalf("box lines = %d, want 4", len(lines))
		}
		if !strings.Contains(lines[1], "│ a      │") {
			t.Fatalf("short line not padded: %q", lines[1])
		}
	})

	t.Run("ansi content does not break padding", func(t *testing.T) {
		stubColor(t, true)
		got := box(bold("hi") + "\nplain!")
		lines := strings.Split(got, "\n")
		if visibleWidth(lines[1]) != visibleWidth(lines[2]) {
			t.Fatalf("uneven padded widths: %q vs %q", lines[1], lines[2])
		}
	})
}

func TestVisibleWidth(t *testing.T) {
	for _, tc := range []struct {
		name  string
		input string
		want  int
	}{
		{"empty", "", 0},
		{"plain", "hello", 5},
		{"ansi wrapped", "\x1b[1mhi\x1b[0m", 2},
		{"bare escapes", "\x1b[90mdim\x1b[0m", 3},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := visibleWidth(tc.input); got != tc.want {
				t.Fatalf("visibleWidth(%q) = %d, want %d", tc.input, got, tc.want)
			}
		})
	}
}

func TestJoinLines(t *testing.T) {
	if got := joinLines("a", "b", "c"); got != "a\nb\nc" {
		t.Fatalf("joinLines = %q", got)
	}
	if got := joinLines(); got != "" {
		t.Fatalf("joinLines() = %q, want empty", got)
	}
	if got := joinLines("solo"); got != "solo" {
		t.Fatalf("joinLines(solo) = %q", got)
	}
}
