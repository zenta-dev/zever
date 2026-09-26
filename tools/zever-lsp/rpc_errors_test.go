package main

import (
	"testing"

	"go.lsp.dev/protocol"

	"github.com/zenta-dev/zever/dsl/ast"
	"github.com/zenta-dev/zever/dsl/diag"
)

// errorFixture builds a service whose rpcs carry both error-case shapes plus
// a nil entry (the default branch), alongside a non-service declaration the
// walk must skip.
func errorFixture() *ast.File {
	return &ast.File{Decls: []ast.Decl{
		&ast.EntityDecl{Name: "User", NamePos: diag.Position{File: "m.zen", Line: 1, Col: 8}},
		&ast.ServiceDecl{
			Name:    "Billing",
			NamePos: diag.Position{File: "m.zen", Line: 3, Col: 9},
			RPCs: []*ast.RPCDecl{
				{
					Name:    "Charge",
					NamePos: diag.Position{File: "m.zen", Line: 4, Col: 6},
					Errors: []ast.Value{
						&ast.IdentValue{Name: "NotFound", Pos: diag.Position{File: "m.zen", Line: 5, Col: 4}},
						&ast.CallValue{Name: "Denied", Pos: diag.Position{File: "m.zen", Line: 6, Col: 4}, NamePos: diag.Position{File: "m.zen", Line: 6, Col: 4}},
						nil,
					},
				},
				{
					Name:    "Refund",
					NamePos: diag.Position{File: "m.zen", Line: 8, Col: 6},
					Errors: []ast.Value{
						&ast.IdentValue{Name: "NotFound", Pos: diag.Position{File: "m.zen", Line: 9, Col: 4}},
					},
				},
			},
		},
	}}
}

// TestWalkErrorCasesVisitsInOrder proves the walk yields every error case in
// declaration order, destructuring both shapes and skipping anything else.
func TestWalkErrorCasesVisitsInOrder(t *testing.T) {
	t.Parallel()

	var names []string

	var positions []diag.Position

	walkErrorCases(errorFixture(), func(name string, pos diag.Position) {
		names = append(names, name)
		positions = append(positions, pos)
	})

	want := []string{"NotFound", "Denied", "NotFound"}
	if len(names) != len(want) {
		t.Fatalf("walk visited %v, want %v", names, want)
	}

	for i := range want {
		if names[i] != want[i] {
			t.Errorf("visit %d = %q, want %q", i, names[i], want[i])
		}
	}

	if positions[0].Line != 5 || positions[1].Line != 6 || positions[2].Line != 9 {
		t.Errorf("visit positions = %+v, want lines 5, 6, 9", positions)
	}
}

// TestWalkErrorCasesSkipsNonServices proves files without services yield
// nothing.
func TestWalkErrorCasesSkipsNonServices(t *testing.T) {
	t.Parallel()

	calls := 0

	walkErrorCases(&ast.File{Decls: []ast.Decl{
		&ast.EntityDecl{Name: "User", NamePos: diag.Position{File: "m.zen", Line: 1, Col: 8}},
	}}, func(_ string, _ diag.Position) { calls++ })

	walkErrorCases(nil, func(_ string, _ diag.Position) { calls++ })

	if calls != 0 {
		t.Errorf("walk visited %d cases in files without services, want 0", calls)
	}
}

// TestErrorCaseAtFound proves the cursor on an error-case name resolves it,
// with the first match winning when two share a name.
func TestErrorCaseAtFound(t *testing.T) {
	t.Parallel()

	// "Denied" starts at 1-based col 4 on line 6 -> 0-based {5,3..8}.
	name, pos, ok := errorCaseAt(errorFixture(), protocol.Position{Line: 5, Character: 4})
	if !ok {
		t.Fatal("errorCaseAt on the Denied case reported no match")
	}

	if name != "Denied" {
		t.Errorf("name = %q, want Denied", name)
	}

	if pos.Line != 6 || pos.Col != 4 {
		t.Errorf("pos = %+v, want line 6 col 4", pos)
	}

	// Both Charge and Refund declare NotFound; the cursor on the second one
	// must resolve the second, proving the walk does not stop early.
	name, pos, ok = errorCaseAt(errorFixture(), protocol.Position{Line: 8, Character: 4})
	if !ok || name != "NotFound" {
		t.Fatalf("errorCaseAt on the second NotFound = %q, %v; want NotFound, true", name, ok)
	}

	if pos.Line != 9 {
		t.Errorf("pos = %+v, want the line-9 occurrence (first match at the cursor wins)", pos)
	}
}

// TestErrorCaseAtMiss proves cursors outside every error case report nothing.
func TestErrorCaseAtMiss(t *testing.T) {
	t.Parallel()

	if name, _, ok := errorCaseAt(errorFixture(), protocol.Position{Line: 0, Character: 0}); ok {
		t.Errorf("errorCaseAt off every case returned %q, want no match", name)
	}

	if _, _, ok := errorCaseAt(nil, protocol.Position{Line: 5, Character: 4}); ok {
		t.Error("errorCaseAt(nil) reported a match, want none")
	}
}
