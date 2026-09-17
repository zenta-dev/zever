package main

import (
	"testing"

	"go.lsp.dev/protocol"

	"github.com/zenta-dev/zever/internal/dsl/ast"
	"github.com/zenta-dev/zever/internal/dsl/diag"
	"github.com/zenta-dev/zever/internal/dsl/parser"
)

func TestCoversIdent(t *testing.T) {
	t.Parallel()

	// `email` starts at line 2, column 3 (1-based) and is 5 characters long,
	// so it occupies 1-based columns 3..7 -> 0-based characters 2..6.
	pos := diag.Position{File: "a.zen", Line: 2, Col: 3}

	tests := []struct {
		name   string
		cursor protocol.Position
		want   bool
	}{
		{"first character", protocol.Position{Line: 1, Character: 2}, true},
		{"middle", protocol.Position{Line: 1, Character: 4}, true},
		{"last character", protocol.Position{Line: 1, Character: 6}, true},
		{"trailing edge is inclusive", protocol.Position{Line: 1, Character: 7}, true},
		{"one before the start", protocol.Position{Line: 1, Character: 1}, false},
		{"past the trailing edge", protocol.Position{Line: 1, Character: 8}, false},
		{"wrong line", protocol.Position{Line: 2, Character: 4}, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := coversIdent(pos, "email", tc.cursor); got != tc.want {
				t.Errorf("coversIdent() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestCoversIdentRejectsUnsetInputs(t *testing.T) {
	t.Parallel()

	cursor := protocol.Position{Line: 0, Character: 0}

	if coversIdent(diag.Position{}, "x", cursor) {
		t.Error("a zero position should never match")
	}

	if coversIdent(diag.Position{Line: 1, Col: 1}, "", cursor) {
		t.Error("an empty identifier should never match")
	}
}

func TestLspPositionConvertsAndClamps(t *testing.T) {
	t.Parallel()

	got := lspPosition(diag.Position{Line: 4, Col: 7})
	if got.Line != 3 || got.Character != 6 {
		t.Errorf("lspPosition = %+v, want {3 6}", got)
	}

	// The compiler occasionally emits a zero position; it must not underflow
	// into a huge unsigned value.
	got = lspPosition(diag.Position{})
	if got.Line != 0 || got.Character != 0 {
		t.Errorf("lspPosition on a zero position = %+v, want {0 0}", got)
	}
}

// TestPointRangeIsOneCharacterWide proves the point anchor used for compiler
// positions spans exactly one character.
func TestPointRangeIsOneCharacterWide(t *testing.T) {
	t.Parallel()

	got := pointRange(diag.Position{File: "a.zen", Line: 3, Col: 5})
	want := protocol.Range{
		Start: protocol.Position{Line: 2, Character: 4},
		End:   protocol.Position{Line: 2, Character: 5},
	}

	if got != want {
		t.Errorf("pointRange() = %+v, want %+v", got, want)
	}
}

// TestIdentRangeSpansTheIdentifier proves identRange extends from the start
// position by exactly len(ident) characters on the same line.
func TestIdentRangeSpansTheIdentifier(t *testing.T) {
	t.Parallel()

	got := identRange(diag.Position{File: "a.zen", Line: 1, Col: 1}, "email")
	want := protocol.Range{
		Start: protocol.Position{Line: 0, Character: 0},
		End:   protocol.Position{Line: 0, Character: 5},
	}

	if got != want {
		t.Errorf("identRange() = %+v, want %+v", got, want)
	}
}

// TestFlattenDeclsReturnsTopLevelDecls checks that flattenDecls surfaces
// every top-level declaration in the file (module identity is now
// dir-derived, so there is no nested block to expand).
func TestFlattenDeclsReturnsTopLevelDecls(t *testing.T) {
	t.Parallel()

	src := "entity Invoice {\n\tid: uuid @primary\n}\n" +
		"entity Loose {\n\tid: uuid @primary\n}\n"

	file, diags := parser.New("m.zen", []byte(src)).ParseFile()
	if diags.HasErrors() {
		t.Fatalf("unexpected parse diagnostics: %v", diags)
	}

	decls := flattenDecls(file)
	if len(decls) != 2 {
		t.Fatalf("flattenDecls returned %d decls, want 2", len(decls))
	}

	if flattenDecls(nil) != nil {
		t.Error("flattenDecls(nil) should be nil")
	}
}

// TestPositionLookupsAcrossTopLevelEntities proves hover/definition position
// helpers reach every top-level entity declaration.
func TestPositionLookupsAcrossTopLevelEntities(t *testing.T) {
	t.Parallel()

	src := "entity Invoice {\n" +
		"\tid: uuid @primary\n" +
		"\tcustomer_id: uuid\n" +
		"\tbelongs_to customer: Customer @foreign_key(customer_id)\n" +
		"}\n" +
		"entity Customer {\n" +
		"\tid: uuid @primary\n" +
		"}\n"

	file, diags := parser.New("m.zen", []byte(src)).ParseFile()
	if diags.HasErrors() {
		t.Fatalf("unexpected parse diagnostics: %v", diags)
	}

	cursor := cursorOn(t, src, 3, "Customer")

	rel := relationTargetAt(file, cursor)
	if rel == nil {
		t.Fatal("relationTargetAt found nothing")
	}

	if rel.Target != "Customer" {
		t.Errorf("relation target = %q, want Customer", rel.Target)
	}

	entity, field := fieldAt(file, cursorOn(t, src, 2, "customer_id"))
	if field == nil || field.Name != "customer_id" {
		t.Fatalf("fieldAt returned %v, want the customer_id field", field)
	}

	if entity == nil || entity.Name != "Invoice" {
		t.Errorf("fieldAt owner = %v, want Invoice", entity)
	}
}

// TestRelationTargetAtMiss covers the negative paths: no relation under the
// cursor, no entity decls at all, and a nil file.
func TestRelationTargetAtMiss(t *testing.T) {
	t.Parallel()

	file := &ast.File{Decls: []ast.Decl{
		&ast.EntityDecl{Name: "Solo", NamePos: diag.Position{File: "m.zen", Line: 1, Col: 8}},
		&ast.JobDecl{Name: "Nightly", NamePos: diag.Position{File: "m.zen", Line: 5, Col: 5}},
	}}

	if got := relationTargetAt(file, protocol.Position{Line: 0, Character: 0}); got != nil {
		t.Errorf("relationTargetAt on a file without relations = %v, want nil", got)
	}

	if got := relationTargetAt(nil, protocol.Position{Line: 0, Character: 0}); got != nil {
		t.Errorf("relationTargetAt(nil) = %v, want nil", got)
	}
}

// TestFieldAtTypeNameHit proves the field-type branch: the cursor is on the
// type reference rather than the field name.
func TestFieldAtTypeNameHit(t *testing.T) {
	t.Parallel()

	file := &ast.File{Decls: []ast.Decl{
		&ast.EntityDecl{
			Name:    "Invoice",
			NamePos: diag.Position{File: "m.zen", Line: 1, Col: 8},
			Fields: []*ast.FieldDecl{
				{
					Name:    "status",
					NamePos: diag.Position{File: "m.zen", Line: 2, Col: 2},
					Type:    &ast.TypeExpr{Name: "Status", NamePos: diag.Position{File: "m.zen", Line: 2, Col: 10}},
				},
				{Name: "note", NamePos: diag.Position{File: "m.zen", Line: 3, Col: 2}},
			},
		},
		&ast.JobDecl{Name: "Nightly", NamePos: diag.Position{File: "m.zen", Line: 5, Col: 5}},
	}}

	// "Status" starts at 1-based col 10, so 0-based characters 9..14.
	entity, field := fieldAt(file, protocol.Position{Line: 1, Character: 10})
	if field == nil || field.Name != "status" {
		t.Fatalf("fieldAt on the type name returned %v, want the status field", field)
	}

	if entity == nil || entity.Name != "Invoice" {
		t.Errorf("fieldAt owner = %v, want Invoice", entity)
	}

	if _, field := fieldAt(file, protocol.Position{Line: 9, Character: 0}); field != nil {
		t.Errorf("fieldAt off every field returned %v, want nil", field)
	}
}

// TestMessageFieldAtHits proves both the name and the type branches of the
// message-decl counterpart of fieldAt, plus the miss paths.
func TestMessageFieldAtHits(t *testing.T) {
	t.Parallel()

	file := &ast.File{Decls: []ast.Decl{
		&ast.EntityDecl{Name: "Other", NamePos: diag.Position{File: "m.zen", Line: 1, Col: 8}},
		&ast.MessageDecl{
			Name:    "Created",
			NamePos: diag.Position{File: "m.zen", Line: 4, Col: 9},
			Fields: []*ast.FieldDecl{
				{
					Name:    "payload",
					NamePos: diag.Position{File: "m.zen", Line: 5, Col: 2},
					Type:    &ast.TypeExpr{Name: "Payload", NamePos: diag.Position{File: "m.zen", Line: 5, Col: 11}},
				},
			},
		},
	}}

	msg, field := messageFieldAt(file, protocol.Position{Line: 4, Character: 2})
	if field == nil || field.Name != "payload" {
		t.Fatalf("messageFieldAt on the field name returned %v, want payload", field)
	}

	if msg == nil || msg.Name != "Created" {
		t.Errorf("messageFieldAt owner = %v, want Created", msg)
	}

	msg, field = messageFieldAt(file, protocol.Position{Line: 4, Character: 11})
	if field == nil || field.Name != "payload" {
		t.Fatalf("messageFieldAt on the type name returned %v, want payload", field)
	}

	if msg == nil || msg.Name != "Created" {
		t.Errorf("messageFieldAt owner = %v, want Created", msg)
	}

	if msg, field := messageFieldAt(file, protocol.Position{Line: 0, Character: 0}); msg != nil || field != nil {
		t.Errorf("messageFieldAt off every field returned %v/%v, want nil/nil", msg, field)
	}

	if msg, field := messageFieldAt(nil, protocol.Position{Line: 4, Character: 2}); msg != nil || field != nil {
		t.Errorf("messageFieldAt(nil) returned %v/%v, want nil/nil", msg, field)
	}
}

// serviceFixture builds a service with two rpcs (the second preceded by a
// nil entry, as defensive callers must tolerate) for the rpc-position tests.
func serviceFixture() *ast.File {
	return &ast.File{Decls: []ast.Decl{
		&ast.EntityDecl{Name: "Unrelated", NamePos: diag.Position{File: "m.zen", Line: 1, Col: 8}},
		&ast.ServiceDecl{
			Name:    "Billing",
			NamePos: diag.Position{File: "m.zen", Line: 1, Col: 9},
			RPCs: []*ast.RPCDecl{
				{
					Name:       "Charge",
					NamePos:    diag.Position{File: "m.zen", Line: 2, Col: 6},
					Returns:    "Receipt",
					ReturnsPos: diag.Position{File: "m.zen", Line: 2, Col: 20},
					Params: []*ast.ParamDecl{
						{
							Name:    "amount",
							NamePos: diag.Position{File: "m.zen", Line: 3, Col: 3},
							Type:    &ast.TypeExpr{Name: "Money", NamePos: diag.Position{File: "m.zen", Line: 3, Col: 11}},
						},
						{Name: "untyped", NamePos: diag.Position{File: "m.zen", Line: 4, Col: 3}},
						nil,
					},
				},
				nil,
			},
		},
	}}
}

// TestRPCReturnsAtHitAndMiss proves the Returns-type lookup and its skips.
func TestRPCReturnsAtHitAndMiss(t *testing.T) {
	t.Parallel()

	file := serviceFixture()

	// "Receipt" starts at 1-based col 20 on line 2 -> 0-based line 1.
	got := rpcReturnsAt(file, protocol.Position{Line: 1, Character: 20})
	if got == nil || got.Name != "Charge" {
		t.Fatalf("rpcReturnsAt returned %v, want the Charge rpc", got)
	}

	if got := rpcReturnsAt(file, protocol.Position{Line: 7, Character: 0}); got != nil {
		t.Errorf("rpcReturnsAt off every rpc returned %v, want nil", got)
	}

	if got := rpcReturnsAt(nil, protocol.Position{Line: 1, Character: 20}); got != nil {
		t.Errorf("rpcReturnsAt(nil) = %v, want nil", got)
	}
}

// TestParamTypeAtAcrossRPCAndJob proves param type lookup reaches rpc params
// and job params while skipping untyped and nil entries.
func TestParamTypeAtAcrossRPCAndJob(t *testing.T) {
	t.Parallel()

	file := serviceFixture()
	file.Decls = append(file.Decls, &ast.JobDecl{
		Name:    "Nightly",
		NamePos: diag.Position{File: "m.zen", Line: 8, Col: 5},
		Params: []*ast.ParamDecl{
			{
				Name:    "limit",
				NamePos: diag.Position{File: "m.zen", Line: 9, Col: 3},
				Type:    &ast.TypeExpr{Name: "Count", NamePos: diag.Position{File: "m.zen", Line: 9, Col: 10}},
			},
		},
	})

	// "Money" starts at 1-based col 11 on line 3 -> 0-based line 2.
	got := paramTypeAt(file, protocol.Position{Line: 2, Character: 11})
	if got == nil || got.Name != "amount" {
		t.Fatalf("paramTypeAt on the rpc param type returned %v, want amount", got)
	}

	got = paramTypeAt(file, protocol.Position{Line: 8, Character: 10})
	if got == nil || got.Name != "limit" {
		t.Fatalf("paramTypeAt on the job param type returned %v, want limit", got)
	}

	if got := paramTypeAt(file, protocol.Position{Line: 0, Character: 0}); got != nil {
		t.Errorf("paramTypeAt off every param returned %v, want nil", got)
	}
}

// TestDeclNameLookups proves the entity/job/service/rpc name lookups each
// find their declaration and report nothing elsewhere.
func TestDeclNameLookups(t *testing.T) {
	t.Parallel()

	file := &ast.File{Decls: []ast.Decl{
		&ast.EntityDecl{Name: "User", NamePos: diag.Position{File: "m.zen", Line: 1, Col: 8}},
		&ast.JobDecl{Name: "Nightly", NamePos: diag.Position{File: "m.zen", Line: 4, Col: 5}},
		&ast.ServiceDecl{
			Name:    "Billing",
			NamePos: diag.Position{File: "m.zen", Line: 7, Col: 9},
			RPCs: []*ast.RPCDecl{
				{
					Name:    "Charge",
					NamePos: diag.Position{File: "m.zen", Line: 8, Col: 6},
				},
				nil,
			},
		},
		&ast.MessageDecl{Name: "Ping", NamePos: diag.Position{File: "m.zen", Line: 11, Col: 9}},
	}}

	// "User" at 1-based col 8 on line 1 -> 0-based {0,7..10}.
	if got := entityNameAt(file, protocol.Position{Line: 0, Character: 8}); got == nil || got.Name != "User" {
		t.Errorf("entityNameAt returned %v, want User", got)
	}

	if got := entityNameAt(file, protocol.Position{Line: 6, Character: 9}); got != nil {
		t.Errorf("entityNameAt on the service name returned %v, want nil", got)
	}

	if got := jobNameAt(file, protocol.Position{Line: 3, Character: 5}); got == nil || got.Name != "Nightly" {
		t.Errorf("jobNameAt returned %v, want Nightly", got)
	}

	if got := jobNameAt(file, protocol.Position{Line: 3, Character: 0}); got != nil {
		t.Errorf("jobNameAt off the name returned %v, want nil", got)
	}

	if got := serviceNameAt(file, protocol.Position{Line: 6, Character: 9}); got == nil || got.Name != "Billing" {
		t.Errorf("serviceNameAt returned %v, want Billing", got)
	}

	if got := serviceNameAt(file, protocol.Position{Line: 0, Character: 8}); got != nil {
		t.Errorf("serviceNameAt on the entity name returned %v, want nil", got)
	}

	svc, rpc := rpcNameAt(file, protocol.Position{Line: 7, Character: 6})
	if rpc == nil || rpc.Name != "Charge" {
		t.Fatalf("rpcNameAt returned %v, want the Charge rpc", rpc)
	}

	if svc == nil || svc.Name != "Billing" {
		t.Errorf("rpcNameAt owner = %v, want Billing", svc)
	}

	if svc, rpc := rpcNameAt(file, protocol.Position{Line: 10, Character: 9}); svc != nil || rpc != nil {
		t.Errorf("rpcNameAt on the message name returned %v/%v, want nil/nil", svc, rpc)
	}

	if svc, rpc := rpcNameAt(nil, protocol.Position{Line: 7, Character: 6}); svc != nil || rpc != nil {
		t.Errorf("rpcNameAt(nil) returned %v/%v, want nil/nil", svc, rpc)
	}

	if got := entityNameAt(nil, protocol.Position{Line: 0, Character: 8}); got != nil {
		t.Errorf("entityNameAt(nil) = %v, want nil", got)
	}

	if got := jobNameAt(nil, protocol.Position{Line: 3, Character: 5}); got != nil {
		t.Errorf("jobNameAt(nil) = %v, want nil", got)
	}

	if got := serviceNameAt(nil, protocol.Position{Line: 6, Character: 9}); got != nil {
		t.Errorf("serviceNameAt(nil) = %v, want nil", got)
	}
}

// TestDispatchTargetAtHits proves the schedule dispatch lookup finds a call
// target, ignores non-call dispatch values, and reports nothing elsewhere.
func TestDispatchTargetAtHits(t *testing.T) {
	t.Parallel()

	file := &ast.File{Decls: []ast.Decl{
		&ast.ScheduleDecl{
			Name:    "Nightly",
			NamePos: diag.Position{File: "m.zen", Line: 1, Col: 10},
			Dispatch: &ast.CallValue{
				Name:    "RunBilling",
				NamePos: diag.Position{File: "m.zen", Line: 2, Col: 12},
			},
		},
		&ast.ScheduleDecl{
			Name:     "Plain",
			NamePos:  diag.Position{File: "m.zen", Line: 4, Col: 10},
			Dispatch: &ast.IdentValue{Name: "NotACall", Pos: diag.Position{File: "m.zen", Line: 5, Col: 12}},
		},
		&ast.EntityDecl{Name: "Other", NamePos: diag.Position{File: "m.zen", Line: 7, Col: 8}},
	}}

	// "RunBilling" starts at 1-based col 12 on line 2 -> 0-based {1,11..20}.
	got := dispatchTargetAt(file, protocol.Position{Line: 1, Character: 12})
	if got == nil || got.Name != "Nightly" {
		t.Fatalf("dispatchTargetAt returned %v, want the Nightly schedule", got)
	}

	if got := dispatchTargetAt(file, protocol.Position{Line: 4, Character: 12}); got != nil {
		t.Errorf("dispatchTargetAt on a non-call dispatch returned %v, want nil", got)
	}

	if got := dispatchTargetAt(file, protocol.Position{Line: 6, Character: 8}); got != nil {
		t.Errorf("dispatchTargetAt off every schedule returned %v, want nil", got)
	}

	if got := dispatchTargetAt(nil, protocol.Position{Line: 1, Character: 12}); got != nil {
		t.Errorf("dispatchTargetAt(nil) = %v, want nil", got)
	}
}
