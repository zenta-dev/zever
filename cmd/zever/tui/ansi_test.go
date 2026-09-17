package tui

import (
	"strings"
)

// stripANSI removes CSI escape sequences so golden snapshots compare
// content, not styling. Tests pin the logical size (80x24) via
// WindowSizeMsg where size matters.
func stripANSI(s string) string {
	var b strings.Builder
	inEsc := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		if inEsc {
			if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') {
				inEsc = false
			}
			continue
		}
		if c == 0x1b {
			inEsc = true
			continue
		}
		b.WriteByte(c)
	}
	// OSC hyperlink sequences (ESC ] ... BEL / ESC \) carry a second
	// terminator; drop any stragglers.
	out := b.String()
	out = strings.ReplaceAll(out, "\a", "")
	return out
}

func splitLines(s string) []string { return strings.Split(s, "\n") }
