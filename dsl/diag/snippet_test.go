package diag

import (
	"strings"
	"testing"
)

func TestExcerpt(t *testing.T) {
	src := "entity User {\n  id: uuid @primary\n  name: strnig\n}"

	t.Run("points at column", func(t *testing.T) {
		got := Excerpt(src, Position{File: "a.zen", Line: 3, Col: 9})
		want := "  |   name: strnig\n  | " + strings.Repeat(" ", 8) + "^"
		if got != want {
			t.Fatalf("Excerpt =\n%q\nwant\n%q", got, want)
		}
	})

	t.Run("out of range yields empty", func(t *testing.T) {
		for _, pos := range []Position{
			{Line: 0, Col: 1}, {Line: 1, Col: 0}, {Line: 9, Col: 1}, {Line: 1, Col: 99},
		} {
			if got := Excerpt(src, pos); got != "" {
				t.Fatalf("Excerpt(%v) = %q, want empty", pos, got)
			}
		}
	})
}

func TestRender(t *testing.T) {
	src := "entity User {\n  name: strnig\n}"
	pos := Position{File: "a.zen", Line: 2, Col: 9}
	d := Wrap("resolve", pos, nil, "unknown field type %q", "strnig")

	t.Run("carries position and excerpt", func(t *testing.T) {
		got := Render(d, map[string]string{"a.zen": src})
		if !strings.HasPrefix(got, "a.zen:2:9: [resolve] unknown field type") {
			t.Fatalf("missing file:line:col header:\n%s", got)
		}

		if !strings.Contains(got, "name: strnig") || !strings.Contains(got, "^") {
			t.Fatalf("missing source excerpt:\n%s", got)
		}
	})

	t.Run("header only without sources", func(t *testing.T) {
		for _, sources := range []map[string]string{nil, {}, {"other.zen": src}} {
			if got := Render(d, sources); strings.Contains(got, "\n") {
				t.Fatalf("Render with missing sources must be header-only, got:\n%s", got)
			}
		}
	})
}
