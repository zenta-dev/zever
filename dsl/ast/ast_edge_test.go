package ast

import (
	"testing"

	"github.com/zenta-dev/zever/dsl/diag"
)

func TestRelationKindZeroValueIsHasMany(t *testing.T) {
	t.Parallel()

	var k RelationKind
	if k != HasMany {
		t.Fatalf("zero RelationKind = %d, want HasMany (%d)", k, HasMany)
	}
}

func TestRelationKindInvalidStringIsEmpty(t *testing.T) {
	t.Parallel()

	if got := RelationKind(-1).String(); got != "" {
		t.Fatalf("RelationKind(-1).String() = %q, want \"\"", got)
	}
}

func TestDeclPosZeroValue(t *testing.T) {
	t.Parallel()

	var d EntityDecl
	if got := d.declPos(); got != (diag.Position{}) {
		t.Fatalf("zero EntityDecl.declPos() = %v, want zero Position", got)
	}
}

func TestValuePosZeroValue(t *testing.T) {
	t.Parallel()

	var v StringLit
	if got := v.valuePos(); got != (diag.Position{}) {
		t.Fatalf("zero StringLit.valuePos() = %v, want zero Position", got)
	}
}

func TestFileZeroValue(t *testing.T) {
	t.Parallel()

	var f File
	if f.Name != "" || f.Decls != nil {
		t.Fatalf("zero File = %+v, want {Name: \"\", Decls: nil}", f)
	}
}

func TestEntityDeclEmptyCollections(t *testing.T) {
	t.Parallel()

	d := &EntityDecl{Name: "User"}
	if d.Attributes != nil || d.Fields != nil || d.Relations != nil || d.Indexes != nil {
		t.Fatalf("zero EntityDecl collections = %+v, want all nil", d)
	}

	if d.DocComment != "" {
		t.Fatalf("zero EntityDecl.DocComment = %q, want \"\"", d.DocComment)
	}
}

func TestTypeExprZeroValue(t *testing.T) {
	t.Parallel()

	var te TypeExpr
	if te.Name != "" || te.Args != nil || te.ArgPos != nil {
		t.Fatalf("zero TypeExpr = %+v, want empty name and nil slices", te)
	}
}

func TestJoinBlockZeroValue(t *testing.T) {
	t.Parallel()

	var j JoinBlock
	if j.Table != "" {
		t.Fatalf("zero JoinBlock.Table = %q, want \"\"", j.Table)
	}
}

func TestAttributeAndArgZeroValues(t *testing.T) {
	t.Parallel()

	var a Attribute
	if a.Name != "" || a.Args != nil {
		t.Fatalf("zero Attribute = %+v, want empty name and nil args", a)
	}

	var arg Arg
	if arg.Name != "" || arg.Value != nil {
		t.Fatalf("zero Arg = %+v, want empty name and nil value", arg)
	}
}

func TestValueImplementationsSatisfyInterface(t *testing.T) {
	t.Parallel()

	var _ Value = (*StringLit)(nil)
	var _ Value = (*IntLit)(nil)
	var _ Value = (*FloatLit)(nil)
	var _ Value = (*DurationLit)(nil)
	var _ Value = (*IdentValue)(nil)
	var _ Value = (*CallValue)(nil)
	var _ Value = (*SetLit)(nil)
}

func TestDeclImplementationsSatisfyInterface(t *testing.T) {
	t.Parallel()

	var _ Decl = (*EntityDecl)(nil)
	var _ Decl = (*EnumDecl)(nil)
	var _ Decl = (*JobDecl)(nil)
	var _ Decl = (*MessageDecl)(nil)
	var _ Decl = (*ScheduleDecl)(nil)
	var _ Decl = (*ServiceDecl)(nil)
}
