package format

import (
	"errors"
	"strings"
	"testing"

	"github.com/zenta-dev/zever/dsl/token"
)

func TestFormatEmptySource(t *testing.T) {
	t.Parallel()

	out, err := Format(nil)
	if err != nil {
		t.Fatalf("Format(nil) error = %v, want nil", err)
	}

	if len(out) != 0 {
		t.Fatalf("Format(nil) = %q, want empty", out)
	}
}

func TestFormatWhitespaceOnlySource(t *testing.T) {
	t.Parallel()

	out, err := Format([]byte("   \n\t\n"))
	if err != nil {
		t.Fatalf("Format(whitespace) error = %v, want nil", err)
	}

	// Whitespace-only lines collapse to empty; the newlines survive.
	if string(out) != "\n\n" {
		t.Fatalf("Format(whitespace) = %q, want %q", out, "\n\n")
	}
}

func TestFormatVeryLongSingleLine(t *testing.T) {
	t.Parallel()

	src := []byte("entity X { f: string " + strings.Repeat("// pad ", 20000) + "}")

	out, err := Format(src)
	if err != nil {
		t.Fatalf("Format(long line) error = %v, want nil", err)
	}

	if len(out) == 0 {
		t.Fatal("Format(long line) returned empty output")
	}
}

func TestFormatDeterministic(t *testing.T) {
	t.Parallel()

	src := []byte("entity    User   {\nid:   uuid\n}\n")

	first, err1 := Format(src)
	second, err2 := Format(src)

	if err1 != nil || err2 != nil {
		t.Fatalf("Format errors: %v / %v", err1, err2)
	}

	if string(first) != string(second) {
		t.Fatal("Format not deterministic")
	}
}

func TestFormatIdempotentOnMessyInput(t *testing.T) {
	t.Parallel()

	src := []byte("entity    User   {\nid:   uuid   @primary\n}\n")

	once, err := Format(src)
	if err != nil {
		t.Fatalf("Format: %v", err)
	}

	twice, err := Format(once)
	if err != nil {
		t.Fatalf("Format(once): %v", err)
	}

	if string(once) != string(twice) {
		t.Fatalf("Format not idempotent:\n--- once ---\n%s\n--- twice ---\n%s", once, twice)
	}
}

func TestFormatErrDirtyOnUnterminatedString(t *testing.T) {
	t.Parallel()

	if _, err := Format([]byte("entity X { f: \"unclosed")); !errors.Is(err, ErrDirty) {
		t.Fatalf("Format error = %v, want ErrDirty", err)
	}
}

func TestEditsEmptySource(t *testing.T) {
	t.Parallel()

	edits, ok := Edits("empty.zen", nil)
	if !ok {
		t.Fatal("Edits ok = false, want true for empty source")
	}

	if len(edits) != 0 {
		t.Fatalf("edits = %v, want none", edits)
	}
}

func TestCanonicalGapBoundaries(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		prev token.Kind
		next token.Kind
		want string
		ok   bool
	}{
		{"colon hugs left", token.IDENT, token.COLON, "", true},
		{"comma hugs left", token.IDENT, token.COMMA, "", true},
		{"question hugs left", token.IDENT, token.QUESTION, "", true},
		{"at hugs right", token.AT, token.IDENT, "", true},
		{"lparen hugs right", token.IDENT, token.LPAREN, "", true},
		{"minus hugs right", token.MINUS, token.IDENT, "", true},
		{"default single space", token.IDENT, token.IDENT, " ", true},
		{"arrow spaced from left", token.ARROW, token.IDENT, " ", true},
		{"illegal prev refused", token.ILLEGAL, token.IDENT, "", false},
		{"illegal next refused", token.IDENT, token.ILLEGAL, "", false},
		{"eof prev refused", token.EOF, token.IDENT, "", false},
		{"eof next refused", token.IDENT, token.EOF, "", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, ok := CanonicalGap(token.Token{Kind: tc.prev}, token.Token{Kind: tc.next})
			if ok != tc.ok || got != tc.want {
				t.Fatalf("CanonicalGap(%s, %s) = %q/%v, want %q/%v", tc.prev, tc.next, got, ok, tc.want, tc.ok)
			}
		})
	}
}

func TestSplitLinesBoundaries(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		src  string
		want int
	}{
		{"empty", "", 0},
		{"single no newline", "a", 1},
		{"single with newline", "a\n", 1},
		{"trailing newline no extra line", "a\nb\n", 2},
		{"blank lines count", "a\n\nb", 3},
		{"only newlines", "\n\n", 2},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := len(SplitLines([]byte(tc.src))); got != tc.want {
				t.Fatalf("SplitLines(%q) = %d lines, want %d", tc.src, got, tc.want)
			}
		})
	}
}

func TestOffsetOfBoundaries(t *testing.T) {
	t.Parallel()

	src := []byte("ab\ncd")
	lines := SplitLines(src)

	if got := OffsetOf(src, lines[0], 1); got != 0 {
		t.Fatalf("OffsetOf col 1 = %d, want 0", got)
	}

	if got := OffsetOf(src, lines[0], 2); got != 1 {
		t.Fatalf("OffsetOf col 2 = %d, want 1", got)
	}

	// Column past end of line clamps to line end.
	if got := OffsetOf(src, lines[0], 99); got != 2 {
		t.Fatalf("OffsetOf col 99 = %d, want 2 (line end)", got)
	}
}

func TestApplyNilEditsCopies(t *testing.T) {
	t.Parallel()

	src := []byte("hello")
	out := Apply(src, nil)
	if string(out) != string(src) {
		t.Fatalf("Apply(nil) = %q, want copy of %q", out, src)
	}
}

func TestApplyUnsortedEditsSkipOverlap(t *testing.T) {
	t.Parallel()

	src := []byte("abcd")
	// Second edit starts before the first one ends: must be skipped.
	edits := []Edit{
		{Line: 0, StartCol: 0, EndCol: 2, NewText: "X"},
		{Line: 0, StartCol: 1, EndCol: 3, NewText: "Y"},
	}

	out := Apply(src, edits)
	if string(out) != "Xcd" {
		t.Fatalf("Apply = %q, want %q (overlapping edit skipped)", out, "Xcd")
	}
}

func TestApplyOutOfRangeLineSkipped(t *testing.T) {
	t.Parallel()

	src := []byte("ab")
	edits := []Edit{{Line: 5, StartCol: 0, EndCol: 1, NewText: "Z"}}

	if out := Apply(src, edits); string(out) != "ab" {
		t.Fatalf("Apply = %q, want %q (out-of-range line skipped)", out, "ab")
	}
}

func TestLexAllEmptySource(t *testing.T) {
	t.Parallel()

	toks, clean := LexAll("empty.zen", nil)
	if !clean || len(toks) != 0 {
		t.Fatalf("LexAll(nil) = %v/%v, want no tokens and clean", toks, clean)
	}
}

func TestLexAllReportsLexErrors(t *testing.T) {
	t.Parallel()

	_, clean := LexAll("bad.zen", []byte("\x01"))
	if clean {
		t.Fatal("LexAll clean = true, want false for illegal character")
	}
}
