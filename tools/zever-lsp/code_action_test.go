package main

import (
	"strings"
	"testing"

	"go.lsp.dev/protocol"
	"go.lsp.dev/uri"

	"github.com/zenta-dev/zever/dsl/compile"
	"github.com/zenta-dev/zever/dsl/diag"
	"github.com/zenta-dev/zever/dsl/parser"
)

// renameFindDiag returns the first diagnostic whose message contains substr,
// failing the test if none matches.
func renameFindDiag(t *testing.T, diags diag.List, substr string) *diag.Diagnostic {
	t.Helper()

	for _, d := range diags {
		if strings.Contains(d.Msg, substr) {
			return d
		}
	}

	t.Fatalf("no diagnostic containing %q in %v", substr, diags)

	return nil
}

// TestLevenshtein_knownCases_matchDistances pins the edit-distance table the
// suggester is built on.
func TestLevenshtein_knownCases_matchDistances(t *testing.T) {
	t.Parallel()

	tests := []struct {
		a, b string
		want int
	}{
		{"kitten", "sitting", 3},
		{"", "", 0},
		{"", "abc", 3},
		{"abc", "", 3},
		{"string", "string", 0},
		{"sting", "string", 1},
		{"flaw", "lawn", 2},
	}

	for _, tc := range tests {
		if got := levenshtein(tc.a, tc.b); got != tc.want {
			t.Errorf("levenshtein(%q, %q) = %d, want %d", tc.a, tc.b, got, tc.want)
		}
	}
}

// TestNearestCandidate_conditions_returnExpected pins the cutoff behavior:
// empty input, a far miss, an exact hit, and the minimum threshold of 1 for
// single-character candidates.
func TestNearestCandidate_conditions_returnExpected(t *testing.T) {
	t.Parallel()

	if got, ok := nearestCandidate("string", nil); ok || got != "" {
		t.Errorf("nearestCandidate(empty) = %q,%v, want \"\",false", got, ok)
	}

	if got, ok := nearestCandidate("zzzqqq", []string{"string", "bool"}); ok || got != "" {
		t.Errorf("nearestCandidate(far) = %q,%v, want \"\",false", got, ok)
	}

	if got, ok := nearestCandidate("sting", []string{"string", "bool"}); !ok || got != "string" {
		t.Errorf("nearestCandidate(sting) = %q,%v, want string,true", got, ok)
	}

	if got, ok := nearestCandidate("b", []string{"a"}); !ok || got != "a" {
		t.Errorf("nearestCandidate(single char) = %q,%v, want a,true (minimum threshold 1)", got, ok)
	}
}

// TestTruePtr_returnsTrue pins the shared IsPreferred helper.
func TestTruePtr_returnsTrue(t *testing.T) {
	t.Parallel()

	p := truePtr()
	if p == nil || !*p {
		t.Errorf("truePtr() = %v, want a pointer to true", p)
	}
}

// TestDiagnosticMessage_arms_extractText pins the union reader every fix
// dispatches through: plain strings, markup content, and the empty result
// for nil or unknown shapes.
func TestDiagnosticMessage_arms_extractText(t *testing.T) {
	t.Parallel()

	plain := protocol.Diagnostic{Message: protocol.String("unknown field type")}
	if got := diagnosticMessage(plain); got != "unknown field type" {
		t.Errorf("diagnosticMessage(string) = %q, want the text", got)
	}

	markup := protocol.Diagnostic{Message: &protocol.MarkupContent{Value: "references unknown entity"}}
	if got := diagnosticMessage(markup); got != "references unknown entity" {
		t.Errorf("diagnosticMessage(markup) = %q, want the value", got)
	}

	var nilMarkup *protocol.MarkupContent

	nilArm := protocol.Diagnostic{Message: nilMarkup}
	if got := diagnosticMessage(nilArm); got != "" {
		t.Errorf("diagnosticMessage(nil markup) = %q, want empty", got)
	}

	empty := protocol.Diagnostic{}
	if got := diagnosticMessage(empty); got != "" {
		t.Errorf("diagnosticMessage(empty) = %q, want empty", got)
	}
}

// TestFixUnknownScalarType_typo_suggestsNearestValid ports the scalar-typo
// case: `sting` offers `string` with a single edit on the type token.
func TestFixUnknownScalarType_typo_suggestsNearestValid(t *testing.T) {
	t.Parallel()

	src := "entity User {\n" +
		"\tid: uuid @primary\n" +
		"\tname: sting\n" +
		"}\n"

	const id uri.URI = "file:///a.zen"

	_, diags := compile.Compile(map[string]string{"a.zen": src})

	d := renameFindDiag(t, diags, "unknown field type")
	lspDiag := diagToLSPDiagnostic(d)

	file, fileDiags := parser.New("a.zen", []byte(src)).ParseFile()
	if fileDiags.HasErrors() {
		t.Fatalf("unexpected parse diagnostics: %v", fileDiags)
	}

	action := fixUnknownScalarType(file, id, lspDiag)
	if action == nil {
		t.Fatal("fixUnknownScalarType() = nil, want a CodeAction")
	}

	if action.Title != `Change to "string"` {
		t.Errorf("Title = %q, want Change to \"string\"", action.Title)
	}

	if action.Kind == nil || *action.Kind != protocol.CodeActionKindQuickFix {
		t.Errorf("Kind = %v, want quickfix", action.Kind)
	}

	if action.IsPreferred == nil || !*action.IsPreferred {
		t.Error("IsPreferred = nil/false, want true")
	}

	edits := action.Edit.Changes[id]
	if len(edits) != 1 {
		t.Fatalf("got %d edits, want 1: %+v", len(edits), edits)
	}

	edit := edits[0]
	if edit.NewText != "string" {
		t.Errorf("NewText = %q, want %q", edit.NewText, "string")
	}

	const wantLine = 2

	if edit.Range.Start.Line != wantLine {
		t.Errorf("Range.Start.Line = %d, want %d", edit.Range.Start.Line, wantLine)
	}

	wantStartCol := strings.Index(strings.Split(src, "\n")[2], "sting")
	if int(edit.Range.Start.Character) != wantStartCol { //nolint:gosec // small test fixture
		t.Errorf("Range.Start.Character = %d, want %d", edit.Range.Start.Character, wantStartCol)
	}

	wantEndCol := wantStartCol + len("sting")
	if int(edit.Range.End.Character) != wantEndCol { //nolint:gosec // small test fixture
		t.Errorf("Range.End.Character = %d, want %d", edit.Range.End.Character, wantEndCol)
	}
}

// TestFixUnknownScalarType_unmatchedYieldsNil pins every guard: an
// unrelated message, a cursor on nothing, and a type too far from any
// scalar to suggest.
func TestFixUnknownScalarType_unmatchedYieldsNil(t *testing.T) {
	t.Parallel()

	src := "entity User {\n" +
		"\tid: uuid @primary\n" +
		"\tname: sting\n" +
		"}\n"

	const id uri.URI = "file:///a.zen"

	file, fileDiags := parser.New("a.zen", []byte(src)).ParseFile()
	if fileDiags.HasErrors() {
		t.Fatalf("unexpected parse diagnostics: %v", fileDiags)
	}

	unrelated := protocol.Diagnostic{
		Range:   protocol.Range{Start: protocol.Position{Line: 2, Character: 7}},
		Message: protocol.String("something else entirely"),
	}
	if got := fixUnknownScalarType(file, id, unrelated); got != nil {
		t.Errorf("fixUnknownScalarType(unrelated) = %+v, want nil", got)
	}

	offTarget := protocol.Diagnostic{
		Range:   protocol.Range{Start: protocol.Position{Line: 99, Character: 0}},
		Message: protocol.String("unknown field type"),
	}
	if got := fixUnknownScalarType(file, id, offTarget); got != nil {
		t.Errorf("fixUnknownScalarType(off-target) = %+v, want nil", got)
	}

	farSrc := "entity User {\n" +
		"\tid: uuid @primary\n" +
		"\tname: zzzqqq\n" +
		"}\n"

	farFile, farDiags := parser.New("a.zen", []byte(farSrc)).ParseFile()
	if farDiags.HasErrors() {
		t.Fatalf("unexpected parse diagnostics: %v", farDiags)
	}

	_, field := fieldAt(farFile, cursorOn(t, farSrc, 2, "zzzqqq"))
	if field == nil {
		t.Fatalf("fieldAt() on the far fixture = nil, want the field")
	}

	far := protocol.Diagnostic{
		Range:   identRange(field.Type.NamePos, field.Type.Name),
		Message: protocol.String("unknown field type"),
	}
	if got := fixUnknownScalarType(farFile, id, far); got != nil {
		t.Errorf("fixUnknownScalarType(far type) = %+v, want nil (no plausible suggestion)", got)
	}

	// A nil file yields no fix rather than a panic.
	nilType := protocol.Diagnostic{
		Range:   protocol.Range{Start: protocol.Position{Line: 0, Character: 0}},
		Message: protocol.String("unknown field type"),
	}
	if got := fixUnknownScalarType(nil, id, nilType); got != nil {
		t.Errorf("fixUnknownScalarType(nil file) = %+v, want nil", got)
	}
}

// TestFixUnresolvedRelationTarget_typo_suggestsNearestEntity ports the
// relation-typo case: `Usr` offers `User` with a single edit on the target.
func TestFixUnresolvedRelationTarget_typo_suggestsNearestEntity(t *testing.T) {
	t.Parallel()

	aSrc := "entity User {\n\tid: uuid @primary\n}\n"
	bSrc := "entity Task {\n" +
		"\tid: uuid @primary\n" +
		"\tuser_id: uuid\n" +
		"\tbelongs_to user: Usr @foreign_key(user_id)\n" +
		"}\n"

	const id uri.URI = "file:///b.zen"

	_, diags := compile.Compile(map[string]string{"a.zen": aSrc, "b.zen": bSrc})

	d := renameFindDiag(t, diags, "references unknown entity")
	lspDiag := diagToLSPDiagnostic(d)

	// The schema is what fixUnresolvedRelationTarget needs candidate names
	// from; the relation itself failed to resolve, but Pass 1 already
	// collected every entity across every module, User included.
	result, _ := compile.Compile(map[string]string{"a.zen": aSrc, "b.zen": bSrc})

	file, fileDiags := parser.New("b.zen", []byte(bSrc)).ParseFile()
	if fileDiags.HasErrors() {
		t.Fatalf("unexpected parse diagnostics: %v", fileDiags)
	}

	action := fixUnresolvedRelationTarget(result.Schema, file, id, lspDiag)
	if action == nil {
		t.Fatal("fixUnresolvedRelationTarget() = nil, want a CodeAction")
	}

	if action.Title != `Change to "User"` {
		t.Errorf("Title = %q, want Change to \"User\"", action.Title)
	}

	edits := action.Edit.Changes[id]
	if len(edits) != 1 {
		t.Fatalf("got %d edits, want 1: %+v", len(edits), edits)
	}

	if edits[0].NewText != "User" {
		t.Errorf("NewText = %q, want %q", edits[0].NewText, "User")
	}

	wantStartCol := strings.Index(strings.Split(bSrc, "\n")[3], "Usr")
	if int(edits[0].Range.Start.Character) != wantStartCol { //nolint:gosec // small test fixture
		t.Errorf("Range.Start.Character = %d, want %d", edits[0].Range.Start.Character, wantStartCol)
	}
}

// TestFixUnresolvedRelationTarget_unmatchedYieldsNil pins every guard: an
// unrelated message, a cursor on nothing, an empty schema, and a target
// too far from any entity to suggest.
func TestFixUnresolvedRelationTarget_unmatchedYieldsNil(t *testing.T) {
	t.Parallel()

	aSrc := "entity User {\n\tid: uuid @primary\n}\n"
	bSrc := "entity Task {\n" +
		"\tid: uuid @primary\n" +
		"\tuser_id: uuid\n" +
		"\tbelongs_to user: Usr @foreign_key(user_id)\n" +
		"}\n"

	const id uri.URI = "file:///b.zen"

	result, _ := compile.Compile(map[string]string{"a.zen": aSrc, "b.zen": bSrc})

	file, fileDiags := parser.New("b.zen", []byte(bSrc)).ParseFile()
	if fileDiags.HasErrors() {
		t.Fatalf("unexpected parse diagnostics: %v", fileDiags)
	}

	unrelated := protocol.Diagnostic{
		Range:   protocol.Range{Start: protocol.Position{Line: 3, Character: 18}},
		Message: protocol.String("something else entirely"),
	}
	if got := fixUnresolvedRelationTarget(result.Schema, file, id, unrelated); got != nil {
		t.Errorf("fixUnresolvedRelationTarget(unrelated) = %+v, want nil", got)
	}

	offTarget := protocol.Diagnostic{
		Range:   protocol.Range{Start: protocol.Position{Line: 99, Character: 0}},
		Message: protocol.String("references unknown entity"),
	}
	if got := fixUnresolvedRelationTarget(result.Schema, file, id, offTarget); got != nil {
		t.Errorf("fixUnresolvedRelationTarget(off-target) = %+v, want nil", got)
	}

	if got := fixUnresolvedRelationTarget(nil, file, id, offTarget); got != nil {
		// Nil schema with an off-target cursor still yields nil; the
		// empty-schema arm below covers the on-target case.
		t.Errorf("fixUnresolvedRelationTarget(nil schema, off-target) = %+v, want nil", got)
	}

	rel := relationTargetAt(file, cursorOn(t, bSrc, 3, "Usr"))
	if rel == nil {
		t.Fatalf("relationTargetAt() = nil, want the Usr relation")
	}

	onTarget := protocol.Diagnostic{
		Range:   identRange(rel.TargetPos, rel.Target),
		Message: protocol.String("references unknown entity"),
	}
	if got := fixUnresolvedRelationTarget(nil, file, id, onTarget); got != nil {
		t.Errorf("fixUnresolvedRelationTarget(nil schema) = %+v, want nil (no candidates)", got)
	}

	farSrc := "entity Task {\n" +
		"\tid: uuid @primary\n" +
		"\tuser_id: uuid\n" +
		"\tbelongs_to user: Zzzqqq @foreign_key(user_id)\n" +
		"}\n"

	farResult, _ := compile.Compile(map[string]string{"a.zen": aSrc})

	farFile, farDiags := parser.New("b.zen", []byte(farSrc)).ParseFile()
	if farDiags.HasErrors() {
		t.Fatalf("unexpected parse diagnostics: %v", farDiags)
	}

	farRel := relationTargetAt(farFile, cursorOn(t, farSrc, 3, "Zzzqqq"))
	if farRel == nil {
		t.Fatalf("relationTargetAt() on the far fixture = nil, want the relation")
	}

	far := protocol.Diagnostic{
		Range:   identRange(farRel.TargetPos, farRel.Target),
		Message: protocol.String("references unknown entity"),
	}
	if got := fixUnresolvedRelationTarget(farResult.Schema, farFile, id, far); got != nil {
		t.Errorf("fixUnresolvedRelationTarget(far target) = %+v, want nil (no plausible suggestion)", got)
	}
}

// TestCodeActionsAt_unrecognized_returnsEmpty ports the dispatch case: a
// diagnostic no fix recognizes yields no actions.
func TestCodeActionsAt_unrecognized_returnsEmpty(t *testing.T) {
	t.Parallel()

	src := "entity User {\n\tid: uuid @primary\n}\n"

	file, fileDiags := parser.New("a.zen", []byte(src)).ParseFile()
	if fileDiags.HasErrors() {
		t.Fatalf("unexpected parse diagnostics: %v", fileDiags)
	}

	unrelated := protocol.Diagnostic{
		Range:   protocol.Range{Start: protocol.Position{Line: 1, Character: 1}},
		Message: protocol.String("this is not a message any fix recognizes"),
	}

	params := &protocol.CodeActionParams{
		Context: protocol.CodeActionContext{Diagnostics: []protocol.Diagnostic{unrelated}},
	}

	got := codeActionsAt(nil, file, "file:///a.zen", params)
	if len(got) != 0 {
		t.Errorf("codeActionsAt() = %+v, want empty", got)
	}
}

// TestCodeActionsAt_recognized_returnsUnionArms proves recognized
// diagnostics produce *CodeAction union arms carrying quickfix metadata,
// one per fix kind.
func TestCodeActionsAt_recognized_returnsUnionArms(t *testing.T) {
	t.Parallel()

	aSrc := "entity User {\n\tid: uuid @primary\n}\n"
	bSrc := "entity Task {\n" +
		"\tid: uuid @primary\n" +
		"\tuser_id: uuid\n" +
		"\tbelongs_to user: Usr @foreign_key(user_id)\n" +
		"\tname: sting\n" +
		"}\n"

	const id uri.URI = "file:///b.zen"

	result, diags := compile.Compile(map[string]string{"a.zen": aSrc, "b.zen": bSrc})

	scalarDiag := diagToLSPDiagnostic(renameFindDiag(t, diags, "unknown field type"))
	relationDiag := diagToLSPDiagnostic(renameFindDiag(t, diags, "references unknown entity"))

	params := &protocol.CodeActionParams{
		Context: protocol.CodeActionContext{Diagnostics: []protocol.Diagnostic{scalarDiag, relationDiag}},
	}

	file, fileDiags := parser.New("b.zen", []byte(bSrc)).ParseFile()
	if fileDiags.HasErrors() {
		t.Fatalf("unexpected parse diagnostics: %v", fileDiags)
	}

	got := codeActionsAt(result.Schema, file, id, params)
	if len(got) != 2 {
		t.Fatalf("codeActionsAt() = %d actions, want 2 (one per fix): %+v", len(got), got)
	}

	titles := map[string]bool{}

	for _, item := range got {
		action, ok := item.(*protocol.CodeAction)
		if !ok || action == nil {
			t.Fatalf("codeActionsAt() item = %T, want *protocol.CodeAction", item)
		}

		if action.Kind == nil || *action.Kind != protocol.CodeActionKindQuickFix {
			t.Errorf("Kind = %v, want quickfix", action.Kind)
		}

		titles[action.Title] = true
	}

	if !titles[`Change to "string"`] {
		t.Errorf("titles = %v, want the scalar fix Change to \"string\"", titles)
	}

	if !titles[`Change to "User"`] {
		t.Errorf("titles = %v, want the relation fix Change to \"User\"", titles)
	}
}
