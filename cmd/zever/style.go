package main

import (
	"os"
	"strings"
)

// isStderrTerminal reports whether stderr is a terminal. Seam var so tests
// can override it; stdlib-only (see isStdinTerminal in main.go for why there
// is no term.IsTerminal call here).
var isStderrTerminal = func() bool {
	fi, err := os.Stderr.Stat()
	if err != nil {
		return false
	}

	return fi.Mode()&os.ModeCharDevice != 0
}

// colorEnabled reports whether ANSI styling should be emitted.
var colorEnabled = func() bool {
	if os.Getenv("NO_COLOR") != "" {
		return false
	}

	if os.Getenv("TERM") == "dumb" {
		return false
	}
	// os.Stderr is the primary CLI output for help/errors.
	return isStderrTerminal()
}()

// ANSI SGR codes. Palette mirrors the source lipgloss theme: bright blue
// title, bright cyan commands, gray dim/hint, bright green/red status,
// bright yellow warnings.
const (
	ansiBold         = "\x1b[1m"
	ansiItalic       = "\x1b[3m"
	ansiGray         = "\x1b[90m"
	ansiBrightRed    = "\x1b[91m"
	ansiBrightGreen  = "\x1b[92m"
	ansiBrightYellow = "\x1b[93m"
	ansiBrightBlue   = "\x1b[94m"
	ansiBrightCyan   = "\x1b[96m"
	ansiReset        = "\x1b[0m"
)

func render(code, s string) string {
	if !colorEnabled {
		return s
	}

	return code + s + ansiReset
}

func bold(s string) string   { return render(ansiBold, s) }
func dim(s string) string    { return render(ansiGray, s) }
func cyan(s string) string   { return render(ansiBrightCyan, s) }
func green(s string) string  { return render(ansiBrightGreen, s) }
func red(s string) string    { return render(ansiBrightRed, s) }
func yellow(s string) string { return render(ansiBrightYellow, s) }

func title(s string) string   { return render(ansiBold+ansiBrightBlue, s) }
func cmd(s string) string     { return render(ansiBold+ansiBrightCyan, s) }
func hint(s string) string    { return render(ansiItalic+ansiGray, s) }
func success(s string) string { return render(ansiBold+ansiBrightGreen, s) }
func failure(s string) string { return render(ansiBold+ansiBrightRed, s) }

// formatHint returns a dim hint line. If useColor, includes 💡 prefix.
func formatHint(msg string) string {
	prefix := "hint: "
	if colorEnabled {
		prefix = "💡 hint: "
	}

	return hint(prefix + msg)
}

// successMark / failMark return colored icons, ascii fallback when !colorEnabled.
func successMark() string {
	if colorEnabled {
		return success("✓")
	}

	return "OK"
}

func failMark() string {
	if colorEnabled {
		return failure("✗")
	}

	return "FAIL"
}

// visibleWidth returns the printable width of s in runes, ignoring ANSI
// escape sequences. Approximate: wide runes count as 1.
func visibleWidth(s string) int {
	w := 0
	inEsc := false
	for _, r := range s {
		if inEsc {
			// SGR sequences terminate with a letter.
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
				inEsc = false
			}

			continue
		}

		if r == '\x1b' {
			inEsc = true
			continue
		}

		w++
	}

	return w
}

// box renders a rounded box around content when color enabled, else plain.
func box(content string) string {
	if !colorEnabled {
		return content
	}

	lines := strings.Split(content, "\n")
	width := 0
	for _, ln := range lines {
		if w := visibleWidth(ln); w > width {
			width = w
		}
	}

	var b strings.Builder
	b.WriteString("╭" + strings.Repeat("─", width+2) + "╮\n")
	for _, ln := range lines {
		b.WriteString("│ " + ln + strings.Repeat(" ", width-visibleWidth(ln)) + " │\n")
	}
	b.WriteString("╰" + strings.Repeat("─", width+2) + "╯")

	return b.String()
}

// joinLines joins with newline, helper to keep help text tidy.
func joinLines(lines ...string) string {
	return strings.Join(lines, "\n")
}
