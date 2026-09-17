package main

import (
	"strings"
	"testing"

	"go.lsp.dev/protocol"
)

// applyEdits applies non-overlapping single-line edits to src. Edits from
// formatDocument arrive sorted, so applying them back-to-front keeps every
// remaining offset valid.
func applyEdits(src []byte, edits []protocol.TextEdit) string {
	lines := splitLines(src)
	out := append([]byte(nil), src...)

	for i := len(edits) - 1; i >= 0; i-- {
		edit := edits[i]

		index := int(edit.Range.Start.Line)
		if index < 0 || index >= len(lines) {
			continue
		}

		line := lines[index]
		start := offsetOf(src, line, int(edit.Range.Start.Character)+1)
		end := offsetOf(src, line, int(edit.Range.End.Character)+1)

		tail := append([]byte(edit.NewText), out[end:]...)
		out = append(out[:start:start], tail...)
	}

	return string(out)
}

func formatted(t *testing.T, src string) string {
	t.Helper()

	return applyEdits([]byte(src), formatDocument("test.zen", []byte(src)))
}

func TestFormatFixesIndentation(t *testing.T) {
	t.Parallel()

	src := "entity User {\n" +
		"id: uuid @primary\n" +
		"        email: string\n" +
		"\t\t\tcreated_at: timestamp\n" +
		"  }\n"

	want := "entity User {\n" +
		"  id: uuid @primary\n" +
		"  email: string\n" +
		"  created_at: timestamp\n" +
		"}\n"

	if got := formatted(t, src); got != want {
		t.Fatalf("indentation not canonical:\ngot:\n%q\nwant:\n%q", got, want)
	}
}

func TestFormatFixesNestedIndentation(t *testing.T) {
	t.Parallel()

	src := "service TaskService {\n" +
		"rpc ListTasks(user_id: uuid) -> Task {\n" +
		"http: GET \"/tasks\"\n" +
		"auth: required\n" +
		"}\n" +
		"}\n"

	want := "service TaskService {\n" +
		"  rpc ListTasks(user_id: uuid) -> Task {\n" +
		"    http: GET \"/tasks\"\n" +
		"    auth: required\n" +
		"  }\n" +
		"}\n"

	if got := formatted(t, src); got != want {
		t.Fatalf("nested indentation not canonical:\ngot:\n%q\nwant:\n%q", got, want)
	}
}

func TestFormatFixesSpacing(t *testing.T) {
	t.Parallel()

	src := "entity User {\n" +
		"\tid:uuid   @ primary\n" +
		"\temail : string @validate(format:\"email\" , min_len : 3)\n" +
		"}\n" +
		"\n" +
		"service S {\n" +
		"\trpc Get(id: uuid)->User {\n" +
		"\t\thttp: GET \"/u\"\n" +
		"\t}\n" +
		"}\n"

	want := "entity User {\n" +
		"  id: uuid @primary\n" +
		"  email: string @validate(format: \"email\", min_len: 3)\n" +
		"}\n" +
		"\n" +
		"service S {\n" +
		"  rpc Get(id: uuid) -> User {\n" +
		"    http: GET \"/u\"\n" +
		"  }\n" +
		"}\n"

	if got := formatted(t, src); got != want {
		t.Fatalf("spacing not canonical:\ngot:\n%q\nwant:\n%q", got, want)
	}
}

func TestFormatNormalizesWhitespaceOnlyLines(t *testing.T) {
	t.Parallel()

	src := "entity User {\n" +
		"\tid: uuid @primary\n" +
		"   \t \n" +
		"\temail: string\n" +
		"}\n"

	want := "entity User {\n" +
		"  id: uuid @primary\n" +
		"\n" +
		"  email: string\n" +
		"}\n"

	if got := formatted(t, src); got != want {
		t.Fatalf("whitespace-only line not cleared:\ngot:\n%q\nwant:\n%q", got, want)
	}
}

// TestFormatPreservesComments is the test the whole design exists for: the
// lexer drops "//" comments with no token and no recorded position, so an
// AST-reprinting formatter would delete all of these.
func TestFormatPreservesComments(t *testing.T) {
	t.Parallel()

	comments := []string{
		"// header comment: describes the file",
		"//",
		"// second header line",
		"// note: not a real colon",
		"// trailing: also not a real colon",
	}

	src := "// header comment: describes the file\n" +
		"//\n" +
		"   // second header line\n" +
		"\n" +
		"entity User {\n" +
		"id:uuid @primary\n" +
		"// note: not a real colon\n" +
		"        email : string  // trailing: also not a real colon\n" +
		"}\n"

	want := "// header comment: describes the file\n" +
		"//\n" +
		"// second header line\n" +
		"\n" +
		"entity User {\n" +
		"  id: uuid @primary\n" +
		"  // note: not a real colon\n" +
		"  email: string  // trailing: also not a real colon\n" +
		"}\n"

	got := formatted(t, src)

	for _, comment := range comments {
		if !strings.Contains(got, comment) {
			t.Errorf("comment %q was lost or rewritten by the formatter", comment)
		}
	}

	if strings.Count(got, "//") != strings.Count(src, "//") {
		t.Errorf("comment count changed: got %d, want %d",
			strings.Count(got, "//"), strings.Count(src, "//"))
	}

	if got != want {
		t.Fatalf("comment-bearing source not canonical:\ngot:\n%q\nwant:\n%q", got, want)
	}
}

func TestFormatIsIdempotent(t *testing.T) {
	t.Parallel()

	src := "// leading\n" +
		"entity User {\n" +
		"id:uuid @ primary\n" +
		"   // inner note\n" +
		"  \n" +
		"        email : string @validate(min_len : 1 , max_len:200)\n" +
		"  }\n"

	once := formatted(t, src)

	if second := formatDocument("test.zen", []byte(once)); len(second) != 0 {
		t.Fatalf("second format pass produced %d edits, want 0: %+v", len(second), second)
	}

	if twice := formatted(t, once); twice != once {
		t.Fatalf("format is not idempotent:\nfirst:\n%q\nsecond:\n%q", once, twice)
	}
}

func TestFormatBailsOutOnSyntaxError(t *testing.T) {
	t.Parallel()

	src := "entity User {\n" +
		"name: string @default(\"unterminated)\n" +
		"}\n"

	if edits := formatDocument("test.zen", []byte(src)); len(edits) != 0 {
		t.Fatalf("expected no edits for source the lexer rejects, got %d: %+v", len(edits), edits)
	}
}

func TestFormatBailsOutOnIllegalCharacter(t *testing.T) {
	t.Parallel()

	src := "entity User {\n" +
		"id: uuid # nope\n" +
		"}\n"

	if edits := formatDocument("test.zen", []byte(src)); len(edits) != 0 {
		t.Fatalf("expected no edits for illegal input, got %d: %+v", len(edits), edits)
	}
}

// TestFormatCanonicalSourcesProduceNoEdits ties the rule table back to
// reality: sources already written in canonical style must produce no edits.
// (The dirty suite read repo example files here; this module has no examples
// directory and tests must not touch the filesystem, so equivalent inline
// fixtures stand in.)
func TestFormatCanonicalSourcesProduceNoEdits(t *testing.T) {
	t.Parallel()

	srcs := []string{
		"entity User {\n" +
			"  id: uuid @primary\n" +
			"  email: string\n" +
			"}\n",
		"service S {\n" +
			"  rpc Get(id: uuid) -> User {\n" +
			"    http: GET \"/u\"\n" +
			"  }\n" +
			"}\n",
	}

	for _, src := range srcs {
		if edits := formatDocument("test.zen", []byte(src)); len(edits) != 0 {
			t.Errorf("canonical source produced %d edit(s), want 0: %+v", len(edits), edits)
		}
	}
}

func TestRangeFormattingFiltersToRequestedLines(t *testing.T) {
	t.Parallel()

	src := "entity User {\n" + // line 0
		"        id: uuid @primary\n" + // line 1 (bad indent)
		"        email:string\n" + // line 2 (bad indent + spacing)
		"        created_at: timestamp\n" + // line 3 (bad indent)
		"}\n" // line 4

	edits := formatDocument("test.zen", []byte(src))
	if len(edits) == 0 {
		t.Fatalf("fixture produced no edits to filter")
	}

	// Request formatting for line 2 only.
	rng := protocol.Range{
		Start: protocol.Position{Line: 2, Character: 0},
		End:   protocol.Position{Line: 2, Character: 30},
	}

	filtered := filterEditsToRange(edits, rng)
	if len(filtered) == 0 {
		t.Fatalf("expected at least one edit touching line 2")
	}

	for _, edit := range filtered {
		if edit.Range.Start.Line != 2 && edit.Range.End.Line != 2 {
			t.Errorf("edit outside requested range leaked through: %+v", edit)
		}
	}

	for _, edit := range edits {
		if edit.Range.Start.Line == 1 || edit.Range.Start.Line == 3 {
			for _, kept := range filtered {
				if kept == edit {
					t.Errorf("edit outside requested range was kept: %+v", edit)
				}
			}
		}
	}
}

func TestRangeFormattingIncludesBoundaryStraddlingEdit(t *testing.T) {
	t.Parallel()

	// spacingEdits reports the edit's range on the token's own line (next.Pos.Line),
	// so to straddle a requested boundary we need an edit whose Start/End lines
	// differ from the requested range's edge in a way that still overlaps it.
	// Here the indentation edit for line 1 spans exactly line 1; request a range
	// that ends exactly at line 1 (inclusive boundary) to confirm it's kept.
	src := "entity User {\n" + // line 0
		"        id: uuid @primary\n" + // line 1 (bad indent)
		"  email: string\n" + // line 2 (already canonical)
		"}\n" // line 3

	edits := formatDocument("test.zen", []byte(src))

	var indentEdit *protocol.TextEdit

	for i := range edits {
		if edits[i].Range.Start.Line == 1 {
			indentEdit = &edits[i]

			break
		}
	}

	if indentEdit == nil {
		t.Fatalf("expected an indentation edit on line 1, got: %+v", edits)
	}

	// Requested range starts and ends at line 1's boundary exactly (a
	// zero-width range at the start of line 1's content), which still
	// overlaps the edit per the line-overlap convention.
	rng := protocol.Range{
		Start: protocol.Position{Line: 1, Character: 0},
		End:   protocol.Position{Line: 1, Character: 0},
	}

	filtered := filterEditsToRange(edits, rng)

	found := false

	for _, edit := range filtered {
		if edit == *indentEdit {
			found = true
		}
	}

	if !found {
		t.Fatalf("boundary-straddling edit was dropped, want kept whole: %+v not in %+v", *indentEdit, filtered)
	}
}

func TestRangeFormattingOnDirtyDocumentReturnsNil(t *testing.T) {
	t.Parallel()

	src := "entity User {\n" +
		"name: string @default(\"unterminated)\n" +
		"}\n"

	edits := formatDocument("test.zen", []byte(src))
	if len(edits) != 0 {
		t.Fatalf("expected no edits for source the lexer rejects, got %d: %+v", len(edits), edits)
	}

	rng := protocol.Range{
		Start: protocol.Position{Line: 0, Character: 0},
		End:   protocol.Position{Line: 2, Character: 1},
	}

	if filtered := filterEditsToRange(edits, rng); len(filtered) != 0 {
		t.Fatalf("expected no edits after filtering a nil edit set, got %d: %+v", len(filtered), filtered)
	}
}

func TestCanonicalGapRules(t *testing.T) {
	t.Parallel()

	src := "entity User {\n\tid: uuid @primary\n}\n"

	tokens, clean := lexAll("test.zen", []byte(src))
	if !clean {
		t.Fatalf("fixture failed to lex cleanly")
	}

	if len(tokens) == 0 {
		t.Fatalf("fixture produced no tokens")
	}

	for i := 1; i < len(tokens); i++ {
		if _, ok := canonicalGap(tokens[i-1], tokens[i]); !ok {
			t.Errorf("no rule for %v -> %v", tokens[i-1].Kind, tokens[i].Kind)
		}
	}
}

func TestLineRange_condition_expected(t *testing.T) {
	tests := []struct {
		name               string
		line, start, end   int
		wantStart, wantEnd uint32
	}{
		{"plain", 3, 1, 5, 1, 5},
		{"negative start clamps to zero", 3, -2, 5, 0, 5},
		{"end before start collapses", 3, 5, 2, 5, 5},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := lineRange(tc.line, tc.start, tc.end)

			if got.Start.Line != uint32(tc.line) || got.End.Line != uint32(tc.line) { //nolint:gosec // table values non-negative
				t.Errorf("lineRange() lines = %d/%d, want %d", got.Start.Line, got.End.Line, tc.line)
			}

			if got.Start.Character != tc.wantStart || got.End.Character != tc.wantEnd {
				t.Errorf("lineRange() columns = %d-%d, want %d-%d",
					got.Start.Character, got.End.Character, tc.wantStart, tc.wantEnd)
			}
		})
	}
}
