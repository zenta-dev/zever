package main

import (
	"testing"

	"github.com/zenta-dev/zever/dsl/ast"
	"github.com/zenta-dev/zever/dsl/ir"
)

func TestScalarDescription(t *testing.T) {
	cases := []struct {
		name string
		want string
	}{
		{"uuid", "UUID scalar"},
		{"string", "string scalar"},
		{"enum", "inline enumeration"},
		{"bogus", "bogus"},
	}

	for _, c := range cases {
		if got := scalarDescription(c.name); got != c.want {
			t.Errorf("scalarDescription(%q) = %q, want %q", c.name, got, c.want)
		}
	}
}

func TestFieldTypeHint(t *testing.T) {
	mkField := func(typeName string, attrs ...*ast.Attribute) *ast.FieldDecl {
		f := &ast.FieldDecl{
			Type:       &ast.TypeExpr{Name: typeName},
			Attributes: attrs,
		}
		return f
	}

	strPtr := func(s string) *string { return &s }

	cases := []struct {
		name string
		f    *ast.FieldDecl
		want string
	}{
		{"plain", mkField("string"), "string"},
		{"primary", mkField("int32", &ast.Attribute{Name: "primary"}), "int32 (primary)"},
		{"unique", mkField("uuid", &ast.Attribute{Name: "unique"}), "uuid (unique)"},
		{"default", mkField("timestamp", &ast.Attribute{Name: "default", Args: []*ast.Arg{{Value: &ast.CallValue{Name: "now"}}}}), "timestamp (default: now())"},
		{"nil", nil, ""},
		{"renamed unused", mkField("int64", &ast.Attribute{Name: "renamed_from", Args: []*ast.Arg{{Value: &ast.StringLit{Value: "old"}}}}), "int64"},
	}
	_ = strPtr

	for _, c := range cases {
		if got := fieldTypeHint(c.f); got != c.want {
			t.Errorf("%s: fieldTypeHint = %q, want %q", c.name, got, c.want)
		}
	}
}

func TestEntitySummary(t *testing.T) {
	e := &ast.EntityDecl{
		Name:      "User",
		Fields:    []*ast.FieldDecl{{}, {}},
		Relations: []*ast.RelationDecl{{}},
	}
	if got := entitySummary(e); got != "entity User: 2 field(s), 1 relation(s)" {
		t.Errorf("got %q", got)
	}
	if got := entitySummary(nil); got != "" {
		t.Errorf("nil got %q", got)
	}
}

func TestRelationTargetHint(t *testing.T) {
	schema := &ir.Schema{
		Modules: []*ir.Module{{
			Entities: []*ir.Entity{{
				Name:      "Order",
				Fields:    []*ir.Field{{}, {}},
				Relations: []*ir.Relation{{}},
			}},
		}},
	}
	rel := &ast.RelationDecl{Target: "Order"}
	if got := relationTargetHint(schema, rel); got != "entity Order: 2 field(s), 1 relation(s)" {
		t.Errorf("got %q", got)
	}

	rel2 := &ast.RelationDecl{Target: "Missing"}
	if got := relationTargetHint(schema, rel2); got != "unresolved entity Missing" {
		t.Errorf("got %q", got)
	}
}

func TestErrorCaseHint(t *testing.T) {
	cases := []struct {
		name string
		want string
	}{
		{"not_found", "HTTP 404, gRPC NOT_FOUND"},
		{"bogus", "unknown error code"},
	}
	for _, c := range cases {
		if got := errorCaseHint(c.name); got != c.want {
			t.Errorf("errorCaseHint(%q) = %q, want %q", c.name, got, c.want)
		}
	}
}
