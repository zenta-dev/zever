package format

import (
	"testing"

	"github.com/zenta-dev/zever/dsl/diag"
	"github.com/zenta-dev/zever/dsl/token"
)

func TestCanonicalGapRefusesIllegalAndEOF(t *testing.T) {
	t.Parallel()

	good := token.Token{Kind: token.IDENT}
	for _, tc := range []struct {
		name string
		prev token.Token
		next token.Token
	}{
		{"illegal prev", token.Token{Kind: token.ILLEGAL}, good},
		{"illegal next", good, token.Token{Kind: token.ILLEGAL}},
		{"eof prev", token.Token{Kind: token.EOF}, good},
		{"eof next", good, token.Token{Kind: token.EOF}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if _, ok := CanonicalGap(tc.prev, tc.next); ok {
				t.Fatalf("CanonicalGap(%v, %v) = ok=true, want false", tc.prev.Kind, tc.next.Kind)
			}
		})
	}
}

func TestApplySkipsOutOfRangeAndOverlappingEdits(t *testing.T) {
	t.Parallel()

	src := []byte("entity User {\n  id: uuid\n}\n")

	out := Apply(src, []Edit{
		{Line: -1, StartCol: 0, EndCol: 5, NewText: "xx"},
		{Line: 99, StartCol: 0, EndCol: 5, NewText: "xx"},
		{Line: 1, StartCol: 0, EndCol: 2, NewText: "    "},
		{Line: 1, StartCol: 1, EndCol: 3, NewText: "yy"}, // overlaps previous: skipped
	})
	want := "entity User {\n    id: uuid\n}\n"

	if string(out) != want {
		t.Fatalf("Apply with bad/overlapping edits:\ngot:\n%q\nwant:\n%q", out, want)
	}
}

func TestApplyClampsColumnsToLine(t *testing.T) {
	t.Parallel()

	src := []byte("ab\n")

	out := Apply(src, []Edit{
		{Line: 0, StartCol: -50, EndCol: 99, NewText: "cd"},
	})

	if string(out) != "cd\n" {
		t.Fatalf("Apply with out-of-range columns:\ngot:\n%q\nwant:\n%q", out, "cd\n")
	}
}

func TestApplyEmptyEditsCopiesInput(t *testing.T) {
	t.Parallel()

	src := []byte("entity User {\n}\n")
	out := Apply(src, nil)

	if string(out) != string(src) {
		t.Fatalf("Apply(src, nil) changed input:\ngot:\n%q\nwant:\n%q", out, src)
	}
}

func TestSplitLinesEdgeCases(t *testing.T) {
	t.Parallel()

	if got := SplitLines(nil); len(got) != 0 {
		t.Fatalf("SplitLines(nil) = %v, want empty", got)
	}

	if got := SplitLines([]byte("ab\n")); len(got) != 1 {
		t.Fatalf("SplitLines(\"ab\\n\") produced %d lines, want 1 (no trailing empty line)", len(got))
	}

	if got := SplitLines([]byte("ab")); len(got) != 1 {
		t.Fatalf("SplitLines(\"ab\") produced %d lines, want 1", len(got))
	}
}

func TestOffsetOfBeyondLineEndStopsAtEnd(t *testing.T) {
	t.Parallel()

	src := []byte("ab\n")
	lines := SplitLines(src)

	if got := OffsetOf(src, lines[0], 99); got != lines[0].end {
		t.Fatalf("OffsetOf beyond line end = %d, want line end %d", got, lines[0].end)
	}
}

func TestLineStatesSkipsTokenBehindCurrentLine(t *testing.T) {
	t.Parallel()

	toks := []token.Token{
		{Kind: token.IDENT, Pos: diag.Position{Line: 0, Col: 1}},
		{Kind: token.LBRACE, Pos: diag.Position{Line: 1, Col: 1}},
	}
	states := lineStates(1, toks)

	if !states[0].hasToken || states[0].firstKind != token.LBRACE {
		t.Fatalf("lineStates = %+v, want the line-1 LBRACE recorded (line-0 token skipped)", states[0])
	}
}

func TestSpacingEditsSkipsOutOfRangeLine(t *testing.T) {
	t.Parallel()

	src := []byte("entity User {\n}\n")
	lines := SplitLines(src)
	toks := []token.Token{
		{Kind: token.IDENT, Pos: diag.Position{Line: 50, Col: 1}},
		{Kind: token.IDENT, Pos: diag.Position{Line: 50, Col: 7}},
	}

	if edits := spacingEdits(src, lines, toks); len(edits) != 0 {
		t.Fatalf("spacingEdits with out-of-range token line produced %d edits, want 0", len(edits))
	}
}

func TestSpacingEditsSkipsPairsWithoutCanonicalGap(t *testing.T) {
	t.Parallel()

	src := []byte("entity User {\n}\n")
	lines := SplitLines(src)
	toks := []token.Token{
		{Kind: token.IDENT, Pos: diag.Position{Line: 1, Col: 1}},
		{Kind: token.ILLEGAL, Pos: diag.Position{Line: 1, Col: 8}},
	}

	if edits := spacingEdits(src, lines, toks); len(edits) != 0 {
		t.Fatalf("spacingEdits with an ILLEGAL token produced %d edits, want 0", len(edits))
	}
}

func TestIndentEditsClampsNegativeDepth(t *testing.T) {
	t.Parallel()

	src := []byte("  }\n")
	out, err := Format(src)
	if err != nil {
		t.Fatalf("Format: %v", err)
	}

	if string(out) != "}\n" {
		t.Fatalf("unbalanced closing brace:\ngot:\n%q\nwant:\n%q", out, "}\n")
	}
}
