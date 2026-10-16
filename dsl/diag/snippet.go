package diag

import (
	"fmt"
	"strings"
)

// Excerpt returns a two-line source excerpt for pos within src: the source
// line itself plus a caret line pointing at the column. Lines and columns are
// 1-based; out-of-range positions (line <1, column <1, or past the end of the
// source) yield "". Tabs in the source line expand to one space on the caret
// line so the caret stays aligned for the common single-tab indent.
func Excerpt(src string, pos Position) string {
	if pos.Line < 1 || pos.Col < 1 {
		return ""
	}

	lines := strings.Split(src, "\n")
	if pos.Line > len(lines) {
		return ""
	}

	text := strings.TrimRight(lines[pos.Line-1], "\r")
	if pos.Col > len(text)+1 {
		return ""
	}

	caret := strings.Repeat(" ", pos.Col-1) + "^"

	return fmt.Sprintf("  | %s\n  | %s", text, caret)
}

// Render formats d as "file:line:col: [phase] msg" followed by the source
// excerpt from sources (keyed by file name) when available. A missing file,
// or a position outside it, renders the header line alone -- rendering never
// fails, so diagnostics stay printable even when sources are stale.
func Render(d *Diagnostic, sources map[string]string) string {
	if d == nil {
		return "<nil diagnostic>"
	}

	header := d.Error()
	if sources == nil {
		return header
	}

	src, ok := sources[d.Pos.File]
	if !ok {
		return header
	}

	if excerpt := Excerpt(src, d.Pos); excerpt != "" {
		return header + "\n" + excerpt
	}

	return header
}
