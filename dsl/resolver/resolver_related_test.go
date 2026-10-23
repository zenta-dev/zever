package resolver

import (
	"errors"
	"strings"
	"testing"

	"github.com/zenta-dev/zever/dsl/ast"
	"github.com/zenta-dev/zever/dsl/diag"
	"github.com/zenta-dev/zever/dsl/parser"
)

// findDuplicateDecl returns the first diagnostic wrapping ErrDuplicateDecl.
func findDuplicateDecl(t *testing.T, diags diag.List) *diag.Diagnostic {
	t.Helper()

	for _, d := range diags {
		if errors.Is(d.Wrapped, ErrDuplicateDecl) {
			return d
		}
	}

	t.Fatalf("no diagnostic wraps ErrDuplicateDecl: %v", diags)

	return nil
}

// TestResolveDuplicateDeclCarriesFirstPosition proves the duplicate
// top-level diagnostic points at the first declaration through Related
// while the human message stays pinned.
func TestResolveDuplicateDeclCarriesFirstPosition(t *testing.T) {
	t.Parallel()

	src := "entity Dup {\n  id: uuid\n}\nentity Dup {\n  id: uuid\n}"

	file, parseDiags := parser.New("dup.zen", []byte(src)).ParseFile()
	if parseDiags.HasErrors() {
		t.Fatalf("parse errors: %v", parseDiags)
	}

	var firstPos diag.Position

	for _, d := range file.Decls {
		if ent, ok := d.(*ast.EntityDecl); ok {
			firstPos = ent.NamePos

			break
		}
	}

	_, diags := Resolve([]*ast.File{file})

	dup := findDuplicateDecl(t, diags)

	if !strings.Contains(dup.Msg, `duplicate top-level declaration "Dup"`) {
		t.Fatalf("Msg = %q, want the pinned duplicate-declaration message", dup.Msg)
	}

	if len(dup.Related) != 1 {
		t.Fatalf("Related len = %d, want exactly 1", len(dup.Related))
	}

	if dup.Related[0].Pos != firstPos {
		t.Fatalf("Related[0].Pos = %+v, want first declaration at %+v", dup.Related[0].Pos, firstPos)
	}

	if dup.Related[0].Msg != "first declared here" {
		t.Fatalf("Related[0].Msg = %q, want %q", dup.Related[0].Msg, "first declared here")
	}
}

// TestResolveDuplicateDeclAcrossFiles proves the related position lands in
// the file holding the first declaration when the duplicate spans files.
func TestResolveDuplicateDeclAcrossFiles(t *testing.T) {
	t.Parallel()

	first, parseDiags := parser.New("a.zen", []byte("entity Dup {\n  id: uuid\n}\n")).ParseFile()
	if parseDiags.HasErrors() {
		t.Fatalf("parse errors: %v", parseDiags)
	}

	second, parseDiags := parser.New("b.zen", []byte("entity Dup {\n  id: uuid\n}\n")).ParseFile()
	if parseDiags.HasErrors() {
		t.Fatalf("parse errors: %v", parseDiags)
	}

	_, diags := Resolve([]*ast.File{first, second})

	dup := findDuplicateDecl(t, diags)

	if dup.Pos.File != "b.zen" {
		t.Fatalf("primary Pos.File = %q, want the duplicate in b.zen", dup.Pos.File)
	}

	if len(dup.Related) != 1 {
		t.Fatalf("Related len = %d, want exactly 1", len(dup.Related))
	}

	if dup.Related[0].Pos.File != "a.zen" {
		t.Fatalf("Related[0].Pos.File = %q, want the first declaration in a.zen", dup.Related[0].Pos.File)
	}
}
