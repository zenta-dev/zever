package main

import (
	"encoding/json"
	"errors"
	"testing"

	"go.lsp.dev/protocol"
	"go.lsp.dev/uri"

	"github.com/zenta-dev/zever/dsl/ast"
	"github.com/zenta-dev/zever/dsl/compile"
	"github.com/zenta-dev/zever/dsl/ir"
	"github.com/zenta-dev/zever/dsl/parser"
)

// executeArgs marshals values the way a client encodes executeCommand
// arguments: each value becomes one raw-JSON LSPAny.
func executeArgs(t *testing.T, values ...any) []protocol.LSPAny {
	t.Helper()

	out := make([]protocol.LSPAny, 0, len(values))

	for _, v := range values {
		raw, err := json.Marshal(v)
		if err != nil {
			t.Fatalf("json.Marshal(%v): %v", v, err)
		}

		out = append(out, protocol.LSPAny(raw))
	}

	return out
}

// executeFixAllEdits runs the fixAll command for docURI and decodes the
// WorkspaceEdit result, failing the test on any shape surprise.
func executeFixAllEdits(t *testing.T, s *Server, docURI string) protocol.WorkspaceEdit {
	t.Helper()

	raw, err := s.ExecuteCommand(t.Context(), &protocol.ExecuteCommandParams{
		Command:   CommandFixAll,
		Arguments: executeArgs(t, docURI),
	})
	if err != nil {
		t.Fatalf("ExecuteCommand(fixAll): %v", err)
	}

	if raw == nil {
		t.Fatal("ExecuteCommand(fixAll) returned null, want a WorkspaceEdit")
	}

	var edit protocol.WorkspaceEdit

	if err := json.Unmarshal([]byte(raw), &edit); err != nil {
		t.Fatalf("fixAll result is not a WorkspaceEdit: %v", err)
	}

	return edit
}

// TestInitialize_executeCommandProvider_advertisesCommands pins the
// capability: the server serves exactly validate and fixAll.
func TestInitialize_executeCommandProvider_advertisesCommands(t *testing.T) {
	t.Parallel()

	s := NewServer()

	res, err := s.Initialize(t.Context(), &protocol.InitializeParams{})
	if err != nil {
		t.Fatalf("Initialize: %v", err)
	}

	got := res.Capabilities.ExecuteCommandProvider.Commands
	want := []string{CommandValidate, CommandFixAll}

	if len(got) != len(want) {
		t.Fatalf("ExecuteCommandProvider.Commands = %v, want %v", got, want)
	}

	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("ExecuteCommandProvider.Commands = %v, want %v", got, want)
		}
	}
}

// TestExecuteCommand_unknownCommand_returnsError pins the closed command
// set: anything outside validate/fixAll fails with the sentinel, never a
// guess and never a panic.
func TestExecuteCommand_unknownCommand_returnsError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		command string
	}{
		{"unknown command", "zever.nope"},
		{"empty command", ""},
		{"near miss", "zever.validateX"},
		{"wrong prefix", "zen.validate"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			s := NewServer()

			_, err := s.ExecuteCommand(t.Context(), &protocol.ExecuteCommandParams{Command: tc.command})
			if !errors.Is(err, errExecuteUnknownCommand) {
				t.Errorf("ExecuteCommand(%q) error = %v, want errExecuteUnknownCommand", tc.command, err)
			}
		})
	}
}

// TestExecuteCommand_nilParams_returnsError pins the nil guard: a missing
// params block is an unknown invocation, not a panic.
func TestExecuteCommand_nilParams_returnsError(t *testing.T) {
	t.Parallel()

	s := NewServer()

	if _, err := s.ExecuteCommand(t.Context(), nil); !errors.Is(err, errExecuteUnknownCommand) {
		t.Errorf("ExecuteCommand(nil) error = %v, want errExecuteUnknownCommand", err)
	}
}

// TestExecuteCommand_validate_recompilesAndPublishes proves validate answers
// null and publishes diagnostics for the document through the captured
// client, the same channel didOpen publishes through.
func TestExecuteCommand_validate_recompilesAndPublishes(t *testing.T) {
	t.Parallel()

	s := NewServer()
	openDoc(t, s, serverDocA, serverBrokenDoc)

	fake := &fakeClient{}
	s.setClient(fake)

	raw, err := s.ExecuteCommand(t.Context(), &protocol.ExecuteCommandParams{
		Command:   CommandValidate,
		Arguments: executeArgs(t, serverDocA),
	})
	if err != nil {
		t.Fatalf("ExecuteCommand(validate): %v", err)
	}

	if raw != nil {
		t.Errorf("ExecuteCommand(validate) = %s, want null", string(raw))
	}

	if _, ok := fake.published(diagURI(t, serverDocA)); !ok {
		t.Error("validate published nothing for the document, want one publishDiagnostics round")
	}
}

// TestExecuteCommand_validate_objectArg_acceptsWrappers proves the [{uri}]
// argument also arrives as an object: {uri} or {textDocument: {uri}}, the
// shapes clients that mirror their TextDocumentIdentifier use.
func TestExecuteCommand_validate_objectArg_acceptsWrappers(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		arg  any
	}{
		{"uri object", map[string]any{"uri": serverDocA}},
		{"textDocument wrapper", map[string]any{"textDocument": map[string]any{"uri": serverDocA}}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			s := NewServer()
			openDoc(t, s, serverDocA, serverBrokenDoc)

			raw, err := s.ExecuteCommand(t.Context(), &protocol.ExecuteCommandParams{
				Command:   CommandValidate,
				Arguments: executeArgs(t, tc.arg),
			})
			if err != nil {
				t.Fatalf("ExecuteCommand(validate): %v", err)
			}

			if raw != nil {
				t.Errorf("ExecuteCommand(validate) = %s, want null", string(raw))
			}
		})
	}
}

// TestExecuteCommand_badArgs_returnsError pins the never-panic contract:
// missing, non-URI, or non-document arguments fail every command with the
// same sentinel unknown commands use.
func TestExecuteCommand_badArgs_returnsError(t *testing.T) {
	t.Parallel()

	bad := []struct {
		name string
		args []protocol.LSPAny
	}{
		{"no arguments", nil},
		{"non-JSON argument", []protocol.LSPAny{protocol.LSPAny([]byte("{oops"))}},
		{"numeric argument", []protocol.LSPAny{protocol.LSPAny([]byte(`123`))}},
		{"empty string", executeArgs(t, "")},
		{"object without uri", executeArgs(t, map[string]any{"other": 1})},
		{"non-file URI", executeArgs(t, "untitled:Untitled-1")},
	}

	for _, tc := range bad {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			for _, command := range []string{CommandValidate, CommandFixAll} {
				s := NewServer()

				_, err := s.ExecuteCommand(t.Context(), &protocol.ExecuteCommandParams{
					Command:   command,
					Arguments: tc.args,
				})
				if !errors.Is(err, errExecuteUnknownCommand) {
					t.Errorf("ExecuteCommand(%s, %s) error = %v, want errExecuteUnknownCommand", command, tc.name, err)
				}
			}
		})
	}
}

// TestExecuteCommand_fixAll_collectsScalarFix ports the scalar-typo case to
// the command path: fixAll on the broken document returns one edit offering
// "string", the same fix codeAction offers.
func TestExecuteCommand_fixAll_collectsScalarFix(t *testing.T) {
	t.Parallel()

	s := NewServer()
	openDoc(t, s, serverDocA, serverBrokenDoc)

	edit := executeFixAllEdits(t, s, serverDocA)

	edits := edit.Changes[uri.URI(serverDocA)]
	if len(edits) != 1 {
		t.Fatalf("fixAll edits = %+v, want 1 scalar fix", edit.Changes)
	}

	if edits[0].NewText != "string" {
		t.Errorf("NewText = %q, want %q", edits[0].NewText, "string")
	}
}

// TestExecuteCommand_fixAll_collectsRelationFix ports the relation-typo case
// across two documents: fixAll on b.zen returns the "User" fix even though
// the candidate entity lives in a.zen.
func TestExecuteCommand_fixAll_collectsRelationFix(t *testing.T) {
	t.Parallel()

	aSrc := "entity User {\n\tid: uuid @primary\n}\n"
	bSrc := "entity Task {\n" +
		"\tid: uuid @primary\n" +
		"\tuser_id: uuid\n" +
		"\tbelongs_to user: Usr @foreign_key(user_id)\n" +
		"}\n"

	s := NewServer()
	openDoc(t, s, serverDocA, aSrc)
	openDoc(t, s, serverDocB, bSrc)

	edit := executeFixAllEdits(t, s, serverDocB)

	edits := edit.Changes[uri.URI(serverDocB)]
	if len(edits) != 1 {
		t.Fatalf("fixAll edits = %+v, want 1 relation fix", edit.Changes)
	}

	if edits[0].NewText != "User" {
		t.Errorf("NewText = %q, want %q", edits[0].NewText, "User")
	}
}

// TestExecuteCommand_fixAll_cleanDoc_returnsEmptyEdit pins the nothing-to-do
// case: a document that compiles cleanly yields an empty edit, not null and
// not an error.
func TestExecuteCommand_fixAll_cleanDoc_returnsEmptyEdit(t *testing.T) {
	t.Parallel()

	s := NewServer()
	openDoc(t, s, serverDocA, serverValidDoc)

	edit := executeFixAllEdits(t, s, serverDocA)

	if len(edit.Changes) != 0 || len(edit.DocumentChanges) != 0 {
		t.Errorf("fixAll on a clean document = %+v, want an empty edit", edit)
	}
}

// TestExecuteCommand_fixAll_unknownDoc_returnsEmptyEdit pins the unknown
// document case: nothing is known, so there is nothing to fix, and the
// command answers an empty edit rather than an error.
func TestExecuteCommand_fixAll_unknownDoc_returnsEmptyEdit(t *testing.T) {
	t.Parallel()

	s := NewServer()

	edit := executeFixAllEdits(t, s, serverUnknown)

	if len(edit.Changes) != 0 || len(edit.DocumentChanges) != 0 {
		t.Errorf("fixAll on an unknown document = %+v, want an empty edit", edit)
	}
}

// TestDiagnosticCodeOf_shapes_extractsStringCode pins the union reader the
// code-first dispatch funnels through: string codes land, and every other
// arm degrades to absent.
func TestDiagnosticCodeOf_shapes_extractsStringCode(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		code protocol.ProgressToken
		want string
	}{
		{"string code", protocol.String(diagCodeUnknownFieldType), diagCodeUnknownFieldType},
		{"integer code", protocol.Integer(1), ""},
		{"absent code", nil, ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			d := protocol.Diagnostic{Code: tc.code}
			if got := diagnosticCodeOf(d); got != tc.want {
				t.Errorf("diagnosticCodeOf() = %q, want %q", got, tc.want)
			}
		})
	}
}

// scalarTypoFixture parses the sting-typo document and returns the parsed
// file plus its source, failing the test when parsing fails.
func scalarTypoFixture(t *testing.T) (*ast.File, string) {
	t.Helper()

	const src = "entity User {\n" +
		"\tid: uuid @primary\n" +
		"\tname: sting\n" +
		"}\n"

	file, fileDiags := parser.New("a.zen", []byte(src)).ParseFile()
	if fileDiags.HasErrors() {
		t.Fatalf("unexpected parse diagnostics: %v", fileDiags)
	}

	return file, src
}

// relationTypoFixture compiles the Usr-typo workspace and returns the schema
// plus the parsed b.zen file and its source, failing the test when parsing
// fails.
func relationTypoFixture(t *testing.T) (*ir.Schema, *ast.File, string) {
	t.Helper()

	const aSrc = "entity User {\n\tid: uuid @primary\n}\n"
	const bSrc = "entity Task {\n" +
		"\tid: uuid @primary\n" +
		"\tuser_id: uuid\n" +
		"\tbelongs_to user: Usr @foreign_key(user_id)\n" +
		"}\n"

	result, _ := compile.Compile(map[string]string{"a.zen": aSrc, "b.zen": bSrc})

	file, fileDiags := parser.New("b.zen", []byte(bSrc)).ParseFile()
	if fileDiags.HasErrors() {
		t.Fatalf("unexpected parse diagnostics: %v", fileDiags)
	}

	return result.Schema, file, bSrc
}

// TestFixUnknownScalarType_codeDispatch_needsNoMessage proves the code-first
// dispatch: a zen-001 code fires the scalar fix even when the message
// matches nothing, and any other code refuses it.
func TestFixUnknownScalarType_codeDispatch_needsNoMessage(t *testing.T) {
	t.Parallel()

	const id uri.URI = "file:///a.zen"

	file, src := scalarTypoFixture(t)

	_, field := fieldAt(file, cursorOn(t, src, 2, "sting"))
	if field == nil || field.Type == nil {
		t.Fatal("fieldAt() on the scalar fixture = nil, want the sting field")
	}

	d := protocol.Diagnostic{
		Range:   identRange(field.Type.NamePos, field.Type.Name),
		Code:    protocol.String(diagCodeUnknownFieldType),
		Message: protocol.String("unrelated text with no class pattern"),
	}

	if action := fixUnknownScalarType(file, id, d); action == nil {
		t.Error("fixUnknownScalarType(code zen-001, unrelated message) = nil, want the scalar fix")
	} else if action.Title != `Change to "string"` {
		t.Errorf("Title = %q, want Change to \"string\"", action.Title)
	}

	d.Code = protocol.String(diagCodeUnknownRelationTarget)
	if action := fixUnknownScalarType(file, id, d); action != nil {
		t.Errorf("fixUnknownScalarType(code zen-002) = %+v, want nil (code decides, not the message)", action)
	}
}

// TestFixUnresolvedRelationTarget_codeDispatch_needsNoMessage proves the
// code-first dispatch for the relation fix: a zen-002 code fires even when
// the message matches nothing, and any other code refuses it.
func TestFixUnresolvedRelationTarget_codeDispatch_needsNoMessage(t *testing.T) {
	t.Parallel()

	const id uri.URI = "file:///b.zen"

	schema, file, bSrc := relationTypoFixture(t)

	rel := relationTargetAt(file, cursorOn(t, bSrc, 3, "Usr"))
	if rel == nil {
		t.Fatal("relationTargetAt() = nil, want the Usr relation")
	}

	d := protocol.Diagnostic{
		Range:   identRange(rel.TargetPos, rel.Target),
		Code:    protocol.String(diagCodeUnknownRelationTarget),
		Message: protocol.String("unrelated text with no class pattern"),
	}

	if action := fixUnresolvedRelationTarget(schema, file, id, d); action == nil {
		t.Error("fixUnresolvedRelationTarget(code zen-002, unrelated message) = nil, want the relation fix")
	} else if action.Title != `Change to "User"` {
		t.Errorf("Title = %q, want Change to \"User\"", action.Title)
	}

	d.Code = protocol.String(diagCodeUnknownFieldType)
	if action := fixUnresolvedRelationTarget(schema, file, id, d); action != nil {
		t.Errorf("fixUnresolvedRelationTarget(code zen-001) = %+v, want nil (code decides, not the message)", action)
	}
}
