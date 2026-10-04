package parser

import (
	"net/http"
	"strings"
	"testing"

	"github.com/zenta-dev/zever/dsl/ast"
	"github.com/zenta-dev/zever/dsl/diag"
)

func TestParseEmptySourceYieldsEmptyFileNoDiags(t *testing.T) {
	t.Parallel()

	file, diags := New("empty.zen", nil).ParseFile()

	if file == nil {
		t.Fatal("ParseFile returned nil file, want non-nil")
	}

	if len(file.Decls) != 0 {
		t.Fatalf("decls = %d, want 0", len(file.Decls))
	}

	if len(diags) != 0 {
		t.Fatalf("diags = %v, want none", diags)
	}
}

func TestParseCommentOnlySourceYieldsNoDecls(t *testing.T) {
	t.Parallel()

	file, diags := New("c.zen", []byte("// just a comment\n// another\n")).ParseFile()

	if file == nil || len(file.Decls) != 0 {
		t.Fatalf("file = %+v, want non-nil with zero decls", file)
	}

	if len(diags) != 0 {
		t.Fatalf("diags = %v, want none", diags)
	}
}

func TestParseGarbageReportsParseDiagButReturnsFile(t *testing.T) {
	t.Parallel()

	file, diags := New("bad.zen", []byte("!!!")).ParseFile()

	if file == nil {
		t.Fatal("ParseFile returned nil file, want non-nil even for garbage")
	}

	if !diags.HasErrors() {
		t.Fatal("diags has no errors, want at least one parse diagnostic")
	}

	found := false
	for _, d := range diags {
		if d.Phase == "parse" {
			found = true
		}
	}

	if !found {
		t.Fatalf("diags = %v, want at least one parse-phase diagnostic", diags)
	}
}

func TestParseUnbalancedBracesRecoversToNextDeclaration(t *testing.T) {
	t.Parallel()

	src := "entity Broken { !!! } entity Good {\n  id: uuid @primary\n}\n"
	file, diags := New("mixed.zen", []byte(src)).ParseFile()

	if file == nil {
		t.Fatal("ParseFile returned nil file")
	}

	if len(file.Decls) != 2 {
		t.Fatalf("decls = %d, want 2 (garbage inside a balanced block must not swallow the next declaration)", len(file.Decls))
	}

	ent, ok := file.Decls[1].(*ast.EntityDecl)
	if !ok {
		t.Fatalf("decls[1] = %T, want *ast.EntityDecl", file.Decls[1])
	}
	if got := ent.Name; got != "Good" {
		t.Fatalf("second decl = %q, want \"Good\"", got)
	}

	if !diags.HasErrors() {
		t.Fatal("diags has no errors, want the Broken block's garbage reported")
	}
}

func TestParseDocCommentAttachedOnlyWhenStandalone(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		src  string
		want string
	}{
		{
			name: "standalone comment attaches",
			src:  "// user entity\nentity User {\n  id: uuid\n}\n",
			want: "user entity",
		},
		{
			name: "trailing comment does not attach",
			src:  "entity User { // trailing\n  id: uuid\n}\n",
			want: "",
		},
		{
			name: "blank line breaks attachment",
			src:  "// orphan\n\nentity User {\n  id: uuid\n}\n",
			want: "",
		},
		{
			name: "gap after comment breaks attachment",
			src:  "// first\n// second\n\nentity User {\n  id: uuid\n}\n",
			want: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			file, diags := New("doc.zen", []byte(tc.src)).ParseFile()
			if diags.HasErrors() {
				t.Fatalf("diags = %v, want none", diags)
			}

			if len(file.Decls) != 1 {
				t.Fatalf("decls = %d, want 1", len(file.Decls))
			}

			ent, ok := file.Decls[0].(*ast.EntityDecl)
			if !ok {
				t.Fatalf("decls[0] = %T, want *ast.EntityDecl", file.Decls[0])
			}
			got := ent.DocComment
			if got != tc.want {
				t.Fatalf("DocComment = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestParseMultiLineDocCommentJoinsWithNewline(t *testing.T) {
	t.Parallel()

	src := "// line one\n// line two\nentity User {\n  id: uuid\n}\n"
	file, diags := New("doc.zen", []byte(src)).ParseFile()
	if diags.HasErrors() {
		t.Fatalf("diags = %v, want none", diags)
	}

	ent, ok := file.Decls[0].(*ast.EntityDecl)
	if !ok {
		t.Fatalf("decls[0] = %T, want *ast.EntityDecl", file.Decls[0])
	}
	want := "line one\nline two"
	if got := ent.DocComment; got != want {
		t.Fatalf("DocComment = %q, want %q", got, want)
	}
}

func TestParseNeverPanicsOnAdversarialInput(t *testing.T) {
	t.Parallel()

	inputs := []string{
		"",
		"entity",
		"entity Foo {",
		"entity Foo { id: uuid @validate(" + strings.Repeat("x(", 500) + "1" + strings.Repeat(")", 500) + ") }",
		strings.Repeat("(", 10000),
		strings.Repeat("{", 10000),
		strings.Repeat("}", 10000),
		"\"unterminated",
		"entity \x01\x02 Foo",
		"enum E { , , }",
		"service S { rpc G() -> T { http: } }",
		"job J { queue: }",
		"schedule S { cron: \"not a cron\" dispatch: }",
		"message M { f: }",
		"entity E { has_many x: Y { join_table } }",
		"\x00\x01\x02",
		"entity E { f: uuid? }",
	}

	for _, src := range inputs {
		src := src
		t.Run("", func(t *testing.T) {
			t.Parallel()

			file, _ := New("adv.zen", []byte(src)).ParseFile()
			if file == nil {
				t.Fatal("ParseFile returned nil file, want non-nil (parser never fails)")
			}
		})
	}
}

func TestParseDeterministicOutput(t *testing.T) {
	t.Parallel()

	src := "entity User { id: uuid @primary\nservice S { rpc G() -> User { http: GET \"/u\" auth: required } }\n"

	first, firstDiags := New("d.zen", []byte(src)).ParseFile()
	second, secondDiags := New("d.zen", []byte(src)).ParseFile()

	if len(first.Decls) != len(second.Decls) {
		t.Fatalf("decl count differs: %d vs %d", len(first.Decls), len(second.Decls))
	}

	if len(firstDiags) != len(secondDiags) {
		t.Fatalf("diag count differs: %d vs %d", len(firstDiags), len(secondDiags))
	}
}

func TestParseFileCarriesFileName(t *testing.T) {
	t.Parallel()

	file, _ := New("myname.zen", []byte("entity User {\n  id: uuid\n}\n")).ParseFile()
	if file.Name != "myname.zen" {
		t.Fatalf("file.Name = %q, want %q", file.Name, "myname.zen")
	}
}

func TestParseLexDiagsPrecedeParseDiags(t *testing.T) {
	t.Parallel()

	// Illegal char (lex phase) plus a missing brace (parse phase): the
	// returned list must carry the lexer diagnostic first.
	_, diags := New("mixed.zen", []byte("entity \x01 User {")).ParseFile()

	if len(diags) < 2 {
		t.Fatalf("diags = %v, want at least 2", diags)
	}

	if diags[0].Phase != "lex" {
		t.Fatalf("diags[0].Phase = %q, want \"lex\"", diags[0].Phase)
	}

	if diags[1].Phase != "parse" {
		t.Fatalf("diags[1].Phase = %q, want \"parse\"", diags[1].Phase)
	}
}

func TestParseOptionalFieldQuestionMarker(t *testing.T) {
	t.Parallel()

	file, diags := New("opt.zen", []byte("entity User {\n  name: string?\n}\n")).ParseFile()
	if diags.HasErrors() {
		t.Fatalf("diags = %v, want none", diags)
	}

	ent, ok := file.Decls[0].(*ast.EntityDecl)
	if !ok {
		t.Fatalf("decls[0] = %T, want *ast.EntityDecl", file.Decls[0])
	}
	if len(ent.Fields) != 1 {
		t.Fatalf("fields = %d, want 1", len(ent.Fields))
	}

	if !ent.Fields[0].Optional {
		t.Fatal("Fields[0].Optional = false, want true")
	}
}

func TestParseKeywordLikeIdentAsFieldName(t *testing.T) {
	t.Parallel()

	// "message" and "enum" are ident-like in content position.
	file, diags := New("kw.zen", []byte("entity User {\n  message: string\n  enum: string\n}\n")).ParseFile()
	if diags.HasErrors() {
		t.Fatalf("diags = %v, want none", diags)
	}

	ent, ok := file.Decls[0].(*ast.EntityDecl)
	if !ok {
		t.Fatalf("decls[0] = %T, want *ast.EntityDecl", file.Decls[0])
	}
	if len(ent.Fields) != 2 {
		t.Fatalf("fields = %d, want 2", len(ent.Fields))
	}
}

func TestParseDiagListImplementsError(t *testing.T) {
	t.Parallel()

	var empty diag.List
	if empty.Error() != "" {
		t.Fatalf("empty diag.List.Error() = %q, want \"\"", empty.Error())
	}

	_, diags := New("bad.zen", []byte("entity")).ParseFile()
	if diags.Error() == "" {
		t.Fatal("non-empty diag.List Error() = \"\", want message")
	}
}

func TestParseTokenStreamExhaustedAtEOF(t *testing.T) {
	t.Parallel()

	// A file ending exactly at a declaration boundary must not produce a
	// spurious "expected declaration" error.
	_, diags := New("ok.zen", []byte("entity User {\n  id: uuid\n}")).ParseFile()
	if diags.HasErrors() {
		t.Fatalf("diags = %v, want none", diags)
	}
}

func TestParseServiceWithAllOptionKinds(t *testing.T) {
	t.Parallel()

	src := `service S {
  rpc G(id: uuid) -> User {
    http: GET "/u/{id}"
    auth: required
    permission: owner
    errors: { not_found, already_exists("taken") }
    paginated: true
  }
}`
	file, diags := New("svc.zen", []byte(src)).ParseFile()
	if diags.HasErrors() {
		t.Fatalf("diags = %v, want none", diags)
	}

	svc, ok := file.Decls[0].(*ast.ServiceDecl)
	if !ok {
		t.Fatalf("decls[0] = %T, want *ast.ServiceDecl", file.Decls[0])
	}
	if len(svc.RPCs) != 1 {
		t.Fatalf("rpcs = %d, want 1", len(svc.RPCs))
	}

	rpc := svc.RPCs[0]
	if rpc.HTTP == nil || rpc.HTTP.Method != http.MethodGet || rpc.HTTP.Path != "/u/{id}" {
		t.Fatalf("rpc.HTTP = %+v, want GET /u/{id}", rpc.HTTP)
	}

	if !rpc.Paginated || !rpc.PaginatedSet {
		t.Fatalf("rpc.Paginated = %v/%v, want true/true", rpc.Paginated, rpc.PaginatedSet)
	}

	if len(rpc.Errors) != 2 {
		t.Fatalf("rpc.Errors = %d, want 2", len(rpc.Errors))
	}
}
