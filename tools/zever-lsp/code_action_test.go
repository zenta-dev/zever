package main

import (
	"math"
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
	lspDiag := diagToLSPDiagnostic(d, "", nil)

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
	if int(edit.Range.Start.Character) != wantStartCol { // small test fixture
		t.Errorf("Range.Start.Character = %d, want %d", edit.Range.Start.Character, wantStartCol)
	}

	wantEndCol := wantStartCol + len("sting")
	if int(edit.Range.End.Character) != wantEndCol { // small test fixture
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
	lspDiag := diagToLSPDiagnostic(d, "", nil)

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
	if int(edits[0].Range.Start.Character) != wantStartCol { // small test fixture
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
		Context: protocol.CodeActionContext{
			Diagnostics: []protocol.Diagnostic{unrelated},
			// Scope to quickfix so the format action (which this
			// tab-indented fixture would otherwise offer) stays out of
			// what this guard-pinning test observes.
			Only: []protocol.CodeActionKind{protocol.CodeActionKindQuickFix},
		},
	}

	got := codeActionsAt(nil, file, "file:///a.zen", []byte(src), params)
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

	scalarDiag := diagToLSPDiagnostic(renameFindDiag(t, diags, "unknown field type"), "", nil)
	relationDiag := diagToLSPDiagnostic(renameFindDiag(t, diags, "references unknown entity"), "", nil)

	params := &protocol.CodeActionParams{
		Context: protocol.CodeActionContext{
			Diagnostics: []protocol.Diagnostic{scalarDiag, relationDiag},
			// Scope to quickfix so the count pins the two fixes, not
			// the format action this tab-indented fixture also offers.
			Only: []protocol.CodeActionKind{protocol.CodeActionKindQuickFix},
		},
	}

	file, fileDiags := parser.New("b.zen", []byte(bSrc)).ParseFile()
	if fileDiags.HasErrors() {
		t.Fatalf("unexpected parse diagnostics: %v", fileDiags)
	}

	got := codeActionsAt(result.Schema, file, id, []byte(bSrc), params)
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

// findActionByTitle returns the action with the given title, failing the
// test when no such arm exists.
func findActionByTitle(
	t *testing.T,
	actions []protocol.CommandOrCodeAction,
	title string,
) *protocol.CodeAction {
	t.Helper()

	for _, item := range actions {
		if action, ok := item.(*protocol.CodeAction); ok && action != nil && action.Title == title {
			return action
		}
	}

	t.Fatalf("no code action titled %q in %+v", title, actions)

	return nil
}

// findFormatAction returns the source.formatAll action in actions, or nil
// when the dispatch offered none.
func findFormatAction(t *testing.T, actions []protocol.CommandOrCodeAction) *protocol.CodeAction {
	t.Helper()

	for _, item := range actions {
		action, ok := item.(*protocol.CodeAction)
		if !ok || action == nil || action.Kind == nil {
			continue
		}

		if *action.Kind == sourceFormatAllKind {
			return action
		}
	}

	return nil
}

// formatEditsOf extracts the *protocol.TextEdit arms of a format action's
// single TextDocumentEdit, failing the test on any shape surprise.
func formatEditsOf(t *testing.T, action *protocol.CodeAction) []protocol.TextEdit {
	t.Helper()

	if action == nil || action.Edit == nil {
		t.Fatal("format action has no edit")
	}

	if len(action.Edit.DocumentChanges) != 1 {
		t.Fatalf("format action has %d document changes, want 1", len(action.Edit.DocumentChanges))
	}

	docEdit, ok := action.Edit.DocumentChanges[0].(*protocol.TextDocumentEdit)
	if !ok {
		t.Fatalf("document change = %T, want *protocol.TextDocumentEdit", action.Edit.DocumentChanges[0])
	}

	out := make([]protocol.TextEdit, 0, len(docEdit.Edits))

	for _, element := range docEdit.Edits {
		edit, ok := element.(*protocol.TextEdit)
		if !ok {
			t.Fatalf("document edit element = %T, want *protocol.TextEdit", element)
		}

		out = append(out, *edit)
	}

	return out
}

// textEditsEqual reports whether two TextEdit slices match elementwise.
// protocol.TextEdit is all comparable fields, so == is exact.
func textEditsEqual(a, b []protocol.TextEdit) bool {
	if len(a) != len(b) {
		return false
	}

	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}

	return true
}

// workspaceChangesEqual reports whether two Changes maps hold the same
// edits per document.
func workspaceChangesEqual(a, b map[uri.URI][]protocol.TextEdit) bool {
	if len(a) != len(b) {
		return false
	}

	for id, edits := range a {
		other, ok := b[id]
		if !ok || !textEditsEqual(edits, other) {
			return false
		}
	}

	return true
}

// TestWantsSourceAction_filter_admitsSourceKinds pins the Only filter: no
// filter admits everything, any source kind (base or dotted) admits, and
// narrower kinds alone refuse.
func TestWantsSourceAction_filter_admitsSourceKinds(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		only []protocol.CodeActionKind
		want bool
	}{
		{"nil admits", nil, true},
		{"empty admits", []protocol.CodeActionKind{}, true},
		{"source admits", []protocol.CodeActionKind{protocol.CodeActionKindSource}, true},
		{"source.formatAll admits", []protocol.CodeActionKind{sourceFormatAllKind}, true},
		{
			"source.organizeImports admits",
			[]protocol.CodeActionKind{protocol.CodeActionKindSourceOrganizeImports},
			true,
		},
		{"source.fixAll admits", []protocol.CodeActionKind{protocol.CodeActionKindSourceFixAll}, true},
		{"quickfix refuses", []protocol.CodeActionKind{protocol.CodeActionKindQuickFix}, false},
		{"refactor refuses", []protocol.CodeActionKind{protocol.CodeActionKindRefactor}, false},
		{
			"mixed quickfix and source admits",
			[]protocol.CodeActionKind{protocol.CodeActionKindQuickFix, protocol.CodeActionKindSource},
			true,
		},
		{"sourceX refuses", []protocol.CodeActionKind{protocol.CodeActionKind("sourceX")}, false},
	}

	for _, tc := range tests {
		if got := wantsSourceAction(tc.only); got != tc.want {
			t.Errorf("%s: wantsSourceAction(%v) = %v, want %v", tc.name, tc.only, got, tc.want)
		}
	}
}

// TestFormatAllAction_tabIndented_returnsDocumentChanges pins the format
// action's shape: title, kind, a single versionless TextDocumentEdit for
// the document, and edits identical to formatDocument output.
func TestFormatAllAction_tabIndented_returnsDocumentChanges(t *testing.T) {
	t.Parallel()

	const id uri.URI = "file:///a.zen"

	src := "entity User {\n\tid: uuid @primary\n}\n"

	action := formatAllAction(id, []byte(src))
	if action == nil {
		t.Fatal("formatAllAction() = nil, want the format action")
	}

	if action.Title != "Format document" {
		t.Errorf("Title = %q, want %q", action.Title, "Format document")
	}

	if action.Kind == nil || *action.Kind != sourceFormatAllKind {
		t.Errorf("Kind = %v, want source.formatAll", action.Kind)
	}

	if action.Edit == nil || len(action.Edit.DocumentChanges) != 1 {
		t.Fatalf("Edit.DocumentChanges has %d entries, want 1", len(action.Edit.DocumentChanges))
	}

	docEdit, ok := action.Edit.DocumentChanges[0].(*protocol.TextDocumentEdit)
	if !ok {
		t.Fatalf("document change = %T, want *protocol.TextDocumentEdit", action.Edit.DocumentChanges[0])
	}

	if docEdit.TextDocument.URI != id {
		t.Errorf("TextDocument.URI = %v, want %v", docEdit.TextDocument.URI, id)
	}

	if docEdit.TextDocument.Version != nil {
		t.Errorf("TextDocument.Version = %v, want nil (versionless edit)", *docEdit.TextDocument.Version)
	}

	if want := formatDocument("a.zen", []byte(src)); !textEditsEqual(formatEditsOf(t, action), want) {
		t.Errorf("format edits = %+v, want formatDocument output %+v", formatEditsOf(t, action), want)
	}
}

// TestFormatAllAction_canonicalYieldsNil pins the no-op case: an already
// canonical document offers no format action.
func TestFormatAllAction_canonicalYieldsNil(t *testing.T) {
	t.Parallel()

	const id uri.URI = "file:///a.zen"

	for _, src := range []string{"", "entity User {\n  id: uuid @primary\n}\n"} {
		if edits := formatDocument("a.zen", []byte(src)); len(edits) != 0 && src != "" {
			t.Fatalf("fixture produces %d format edits, want a canonical one", len(edits))
		}

		if got := formatAllAction(id, []byte(src)); got != nil {
			t.Errorf("formatAllAction(%q) = %+v, want nil (nothing to apply)", src, got)
		}
	}
}

// TestCodeActionsAt_sourceFormatAll_filtering pins dispatch of the format
// action: offered without an Only filter or under any source kind, never
// under quickfix or refactor alone.
func TestCodeActionsAt_sourceFormatAll_filtering(t *testing.T) {
	t.Parallel()

	const id uri.URI = "file:///a.zen"

	src := "entity User {\n\tid: uuid @primary\n}\n"

	file, fileDiags := parser.New("a.zen", []byte(src)).ParseFile()
	if fileDiags.HasErrors() {
		t.Fatalf("unexpected parse diagnostics: %v", fileDiags)
	}

	tests := []struct {
		name string
		only []protocol.CodeActionKind
		want bool
	}{
		{"nil Only offers", nil, true},
		{"source Only offers", []protocol.CodeActionKind{protocol.CodeActionKindSource}, true},
		{"source.formatAll Only offers", []protocol.CodeActionKind{sourceFormatAllKind}, true},
		{"quickfix Only suppresses", []protocol.CodeActionKind{protocol.CodeActionKindQuickFix}, false},
		{"refactor Only suppresses", []protocol.CodeActionKind{protocol.CodeActionKindRefactor}, false},
	}

	for _, tc := range tests {
		params := &protocol.CodeActionParams{
			Context: protocol.CodeActionContext{Only: tc.only},
		}

		got := findFormatAction(t, codeActionsAt(nil, file, id, []byte(src), params))

		if (got != nil) != tc.want {
			t.Errorf("%s: format action present = %v, want %v", tc.name, got != nil, tc.want)
		}

		if got != nil {
			if want := formatDocument("a.zen", []byte(src)); !textEditsEqual(formatEditsOf(t, got), want) {
				t.Errorf("%s: format edits = %+v, want formatDocument output %+v", tc.name, formatEditsOf(t, got), want)
			}
		}
	}
}

// TestCodeActionResolveDataFromAny_shapes_recoversPayload pins the payload
// decode every resolve path funnels through: raw JSON, in-process value and
// pointer, and the map a JSON decode into an `any` yields.
func TestCodeActionResolveDataFromAny_shapes_recoversPayload(t *testing.T) {
	t.Parallel()

	want := codeActionResolveData{
		Kind:      codeActionResolveKindScalar,
		URI:       "file:///a.zen",
		Line:      2,
		Character: 7,
		Message:   `unknown field type "sting"`,
	}

	// The raw-JSON shape, built the same way dispatch attaches it.
	holder := &protocol.CodeAction{}
	attachCodeActionData(holder, want)

	if holder.Data == nil {
		t.Fatal("attachCodeActionData left Data nil")
	}

	t.Run("lspany", func(t *testing.T) {
		t.Parallel()

		got, ok := codeActionResolveDataFromAny(holder.Data)
		if !ok || got != want {
			t.Errorf("codeActionResolveDataFromAny(LSPAny) = %+v,%v, want %+v,true", got, ok, want)
		}
	})

	t.Run("value", func(t *testing.T) {
		t.Parallel()

		got, ok := codeActionResolveDataFromAny(want)
		if !ok || got != want {
			t.Errorf("codeActionResolveDataFromAny(value) = %+v,%v, want %+v,true", got, ok, want)
		}
	})

	t.Run("pointer", func(t *testing.T) {
		t.Parallel()

		got, ok := codeActionResolveDataFromAny(&want)
		if !ok || got != want {
			t.Errorf("codeActionResolveDataFromAny(pointer) = %+v,%v, want %+v,true", got, ok, want)
		}
	})

	t.Run("map", func(t *testing.T) {
		t.Parallel()

		m := map[string]any{
			"kind":      want.Kind,
			"uri":       want.URI,
			"line":      float64(want.Line),
			"character": float64(want.Character),
			"message":   want.Message,
		}

		got, ok := codeActionResolveDataFromAny(m)
		if !ok || got != want {
			t.Errorf("codeActionResolveDataFromAny(map) = %+v,%v, want %+v,true", got, ok, want)
		}
	})
}

// TestCodeActionResolveDataFromAny_unmatched_reportsFalse pins every
// rejection: bad JSON, a payload without a kind, and foreign shapes.
func TestCodeActionResolveDataFromAny_unmatched_reportsFalse(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		data any
	}{
		{"bad json", protocol.LSPAny([]byte("{oops"))},
		{"json without kind", protocol.LSPAny([]byte(`{"uri":"file:///a.zen"}`))},
		{"map without kind", map[string]any{"uri": "file:///a.zen"}},
		{"nil pointer", (*codeActionResolveData)(nil)},
		{"foreign string", "scalar"},
		{"nil", nil},
	}

	for _, tc := range cases {
		if got, ok := codeActionResolveDataFromAny(tc.data); ok || got != (codeActionResolveData{}) {
			t.Errorf("%s: codeActionResolveDataFromAny() = %+v,%v, want zero,false", tc.name, got, ok)
		}
	}
}

// TestUint32FromAny_shapes_convertsNumbers pins the map-path number
// decoding: JSON float64s and Go ints land, negatives and foreign shapes
// degrade to zero.
func TestUint32FromAny_shapes_convertsNumbers(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		in   any
		want uint32
	}{
		{"float", float64(3), 3},
		{"negative float", float64(-1), 0},
		{"huge float", float64(math.MaxUint32) + 1024, 0},
		{"uint32", uint32(7), 7},
		{"int", 5, 5},
		{"negative int", -2, 0},
		{"huge int", math.MaxUint32 + 1, 0},
		{"string", "3", 0},
		{"nil", nil, 0},
	}

	for _, tc := range cases {
		if got := uint32FromAny(tc.in); got != tc.want {
			t.Errorf("%s: uint32FromAny(%v) = %d, want %d", tc.name, tc.in, got, tc.want)
		}
	}
}

// TestCodeActionResolve_scalarFillsEditWhenAbsent pins the lazy path: an
// action stripped of its eager edit regains the identical edit on resolve.
func TestCodeActionResolve_scalarFillsEditWhenAbsent(t *testing.T) {
	s := NewServer()
	openDoc(t, s, serverDocA, serverBrokenDoc)

	actions, err := s.CodeAction(t.Context(), &protocol.CodeActionParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: uri.URI(serverDocA)},
		Context: protocol.CodeActionContext{
			Diagnostics: []protocol.Diagnostic{{
				Range:   protocol.Range{Start: cursorOn(t, serverBrokenDoc, 1, "strnig")},
				Message: protocol.String(`unknown field type "strnig"`),
			}},
		},
	})
	if err != nil {
		t.Fatalf("CodeAction: %v", err)
	}

	eager := findActionByTitle(t, actions, `Change to "string"`)
	if eager.Edit == nil {
		t.Fatal("eager scalar fix has no edit")
	}

	stripped := *eager
	stripped.Edit = nil

	resolved, err := s.CodeActionResolve(t.Context(), &stripped)
	if err != nil {
		t.Fatalf("CodeActionResolve: %v", err)
	}

	if resolved.Edit == nil {
		t.Fatal("CodeActionResolve left Edit nil, want the rebuilt edit")
	}

	if !workspaceChangesEqual(resolved.Edit.Changes, eager.Edit.Changes) {
		t.Errorf("resolved edits = %+v, want eager edits %+v", resolved.Edit.Changes, eager.Edit.Changes)
	}
}

// TestCodeActionResolve_relationFillsEditWhenAbsent pins the lazy path for
// the relation fix across two documents.
func TestCodeActionResolve_relationFillsEditWhenAbsent(t *testing.T) {
	aSrc := "entity User {\n\tid: uuid @primary\n}\n"
	bSrc := "entity Task {\n" +
		"\tid: uuid @primary\n" +
		"\tuser_id: uuid\n" +
		"\tbelongs_to user: Usr @foreign_key(user_id)\n" +
		"}\n"

	s := NewServer()
	openDoc(t, s, serverDocA, aSrc)
	openDoc(t, s, serverDocB, bSrc)

	actions, err := s.CodeAction(t.Context(), &protocol.CodeActionParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: uri.URI(serverDocB)},
		Context: protocol.CodeActionContext{
			Diagnostics: []protocol.Diagnostic{{
				Range:   protocol.Range{Start: cursorOn(t, bSrc, 3, "Usr")},
				Message: protocol.String(`relation Task.user references unknown entity "Usr"`),
			}},
		},
	})
	if err != nil {
		t.Fatalf("CodeAction: %v", err)
	}

	eager := findActionByTitle(t, actions, `Change to "User"`)
	if eager.Edit == nil {
		t.Fatal("eager relation fix has no edit")
	}

	stripped := *eager
	stripped.Edit = nil

	resolved, err := s.CodeActionResolve(t.Context(), &stripped)
	if err != nil {
		t.Fatalf("CodeActionResolve: %v", err)
	}

	if resolved.Edit == nil {
		t.Fatal("CodeActionResolve left Edit nil, want the rebuilt edit")
	}

	if !workspaceChangesEqual(resolved.Edit.Changes, eager.Edit.Changes) {
		t.Errorf("resolved edits = %+v, want eager edits %+v", resolved.Edit.Changes, eager.Edit.Changes)
	}
}

// TestCodeActionResolve_formatFillsDocumentChangesWhenAbsent pins the lazy
// path for the format action: the stripped DocumentChanges come back
// identical to the formatting output.
func TestCodeActionResolve_formatFillsDocumentChangesWhenAbsent(t *testing.T) {
	s := NewServer()
	openDoc(t, s, serverDocA, serverValidDoc)

	actions, err := s.CodeAction(t.Context(), &protocol.CodeActionParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: uri.URI(serverDocA)},
		Context: protocol.CodeActionContext{
			Only: []protocol.CodeActionKind{protocol.CodeActionKindSource},
		},
	})
	if err != nil {
		t.Fatalf("CodeAction: %v", err)
	}

	if len(actions) != 1 {
		t.Fatalf("CodeAction(source only) = %d actions, want 1 (the format action)", len(actions))
	}

	eager := findFormatAction(t, actions)
	if eager == nil {
		t.Fatal("CodeAction(source only) offered no format action")
	}

	stripped := *eager
	stripped.Edit = nil

	resolved, err := s.CodeActionResolve(t.Context(), &stripped)
	if err != nil {
		t.Fatalf("CodeActionResolve: %v", err)
	}

	if resolved.Edit == nil {
		t.Fatal("CodeActionResolve left Edit nil, want the rebuilt DocumentChanges")
	}

	if want := formatDocument("a.zen", []byte(serverValidDoc)); !textEditsEqual(formatEditsOf(t, resolved), want) {
		t.Errorf("resolved edits = %+v, want formatDocument output %+v", formatEditsOf(t, resolved), want)
	}
}

// TestCodeActionResolve_editPresent_roundTripsUnchanged pins idempotence:
// an action that already carries an edit keeps it byte-for-byte, even with
// no resolve payload at all.
func TestCodeActionResolve_editPresent_roundTripsUnchanged(t *testing.T) {
	s := NewServer()
	openDoc(t, s, serverDocA, serverBrokenDoc)

	actions, err := s.CodeAction(t.Context(), &protocol.CodeActionParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: uri.URI(serverDocA)},
		Context: protocol.CodeActionContext{
			Diagnostics: []protocol.Diagnostic{{
				Range:   protocol.Range{Start: cursorOn(t, serverBrokenDoc, 1, "strnig")},
				Message: protocol.String(`unknown field type "strnig"`),
			}},
		},
	})
	if err != nil {
		t.Fatalf("CodeAction: %v", err)
	}

	eager := findActionByTitle(t, actions, `Change to "string"`)

	before := eager.Edit

	resolved, err := s.CodeActionResolve(t.Context(), eager)
	if err != nil {
		t.Fatalf("CodeActionResolve: %v", err)
	}

	if resolved.Edit == nil || !workspaceChangesEqual(resolved.Edit.Changes, before.Changes) {
		t.Errorf("resolved edits = %+v, want the untouched eager edits %+v", resolved.Edit, before)
	}

	// No payload and no edit either: still a no-op, never an error.
	bare, err := s.CodeActionResolve(t.Context(), &protocol.CodeAction{Title: "bare"})
	if err != nil {
		t.Fatalf("CodeActionResolve(bare): %v", err)
	}

	if bare.Edit != nil {
		t.Errorf("CodeActionResolve(bare).Edit = %+v, want nil", bare.Edit)
	}
}

// TestCodeActionResolve_staleDocument_yieldsNoEdit pins the best-effort
// contract: when the document changed such that no fix applies anymore,
// resolve finds nothing and leaves the action edit-less rather than
// inventing an edit.
func TestCodeActionResolve_staleDocument_yieldsNoEdit(t *testing.T) {
	s := NewServer()
	openDoc(t, s, serverDocA, serverBrokenDoc)

	actions, err := s.CodeAction(t.Context(), &protocol.CodeActionParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: uri.URI(serverDocA)},
		Context: protocol.CodeActionContext{
			Diagnostics: []protocol.Diagnostic{{
				Range:   protocol.Range{Start: cursorOn(t, serverBrokenDoc, 1, "strnig")},
				Message: protocol.String(`unknown field type "strnig"`),
			}},
		},
	})
	if err != nil {
		t.Fatalf("CodeAction: %v", err)
	}

	eager := findActionByTitle(t, actions, `Change to "string"`)

	fixed := "entity User {\n\tid: zzzqqq\n}\n"
	openDoc(t, s, serverDocA, fixed)

	stripped := *eager
	stripped.Edit = nil

	resolved, err := s.CodeActionResolve(t.Context(), &stripped)
	if err != nil {
		t.Fatalf("CodeActionResolve: %v", err)
	}

	if resolved.Edit != nil {
		t.Errorf("CodeActionResolve(stale).Edit = %+v, want nil (no fix applies anymore)", resolved.Edit)
	}
}

// TestCodeActionResolve_unknownKind_yieldsUnchanged pins the fallback: a
// payload with an unrecognized kind round-trips without an edit.
func TestCodeActionResolve_unknownKind_yieldsUnchanged(t *testing.T) {
	s := NewServer()

	action := &protocol.CodeAction{Title: "unknown"}
	attachCodeActionData(action, codeActionResolveData{Kind: "nope", URI: serverDocA})
	action.Edit = nil

	resolved, err := s.CodeActionResolve(t.Context(), action)
	if err != nil {
		t.Fatalf("CodeActionResolve: %v", err)
	}

	if resolved.Edit != nil {
		t.Errorf("CodeActionResolve(unknown kind).Edit = %+v, want nil", resolved.Edit)
	}
}

// TestCodeActionResolve_nilParams_returnsNull pins the handler's nil
// guard: resolving nothing answers null, not an error.
func TestCodeActionResolve_nilParams_returnsNull(t *testing.T) {
	s := NewServer()

	resolved, err := s.CodeActionResolve(t.Context(), nil)
	if err != nil {
		t.Fatalf("CodeActionResolve(nil): %v", err)
	}

	if resolved != nil {
		t.Errorf("CodeActionResolve(nil) = %+v, want null", resolved)
	}
}
