package resolver

import (
	"errors"
	nethttp "net/http"
	"strings"
	"testing"
	"time"

	"github.com/zenta-dev/zever/dsl/ast"
	"github.com/zenta-dev/zever/dsl/diag"
	"github.com/zenta-dev/zever/dsl/ir"
)

// --- small AST-building helpers, local to this test file ---

func primaryUUIDField(name string) *ast.FieldDecl {
	return &ast.FieldDecl{
		Name:       name,
		Type:       &ast.TypeExpr{Name: "uuid"},
		Attributes: []*ast.Attribute{{Name: "primary"}},
	}
}

func stringField(name string, attrs ...*ast.Attribute) *ast.FieldDecl {
	return &ast.FieldDecl{Name: name, Type: &ast.TypeExpr{Name: "string"}, Attributes: attrs}
}

func entityWithIDField(name string, extra ...*ast.FieldDecl) *ast.EntityDecl {
	return &ast.EntityDecl{
		Name:   name,
		Fields: append([]*ast.FieldDecl{primaryUUIDField("id")}, extra...),
	}
}

func mustHaveErr(t *testing.T, diags diag.List, target error) {
	t.Helper()

	for _, d := range diags {
		if errors.Is(d, target) {
			return
		}
	}

	t.Fatalf("expected a diagnostic wrapping %v, got: %v", target, diags)
}

func mustNotHaveErrors(t *testing.T, diags diag.List) {
	t.Helper()

	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
}

func findEntity(schema *ir.Schema, name string) *ir.Entity {
	for _, m := range schema.Modules {
		for _, e := range m.Entities {
			if e.Name == name {
				return e
			}
		}
	}

	return nil
}

func findMessage(schema *ir.Schema, name string) *ir.Message {
	for _, m := range schema.Modules {
		for _, msg := range m.Messages {
			if msg.Name == name {
				return msg
			}
		}
	}

	return nil
}

// --- Pass -1: module grouping ---

func TestResolveModuleGrouping(t *testing.T) {
	t.Run("pure monolith has one implicit module", func(t *testing.T) {
		files := []*ast.File{{Name: "schema/app.zen", Decls: []ast.Decl{entityWithIDField("User")}}}

		schema, diags := ResolveWithSchemaDir(files, "schema")
		mustNotHaveErrors(t, diags)

		if len(schema.Modules) != 1 {
			t.Fatalf("Modules len = %d, want 1", len(schema.Modules))
		}

		if schema.Modules[0].Name != "" {
			t.Fatalf("Modules[0].Name = %q, want empty", schema.Modules[0].Name)
		}
	})

	t.Run("dir-derived immediate subdir only", func(t *testing.T) {
		files := []*ast.File{
			{Name: "schema/billing/invoices/x.zen", Decls: []ast.Decl{entityWithIDField("Order")}},
			{Name: "schema/billing/other.zen", Decls: []ast.Decl{entityWithIDField("Invoice")}},
			{Name: "schema/app.zen", Decls: []ast.Decl{entityWithIDField("User")}},
		}

		schema, diags := ResolveWithSchemaDir(files, "schema")
		mustNotHaveErrors(t, diags)

		if len(schema.Modules) != 2 {
			t.Fatalf("Modules len = %d, want 2 (billing + public)", len(schema.Modules))
		}
		// sorted: "" < "billing"
		if schema.Modules[0].Name != "" || len(schema.Modules[0].Entities) != 1 || schema.Modules[0].Entities[0].Name != "User" {
			t.Fatalf("public module wrong: %+v", schema.Modules[0])
		}
		if schema.Modules[1].Name != "billing" || len(schema.Modules[1].Entities) != 2 {
			t.Fatalf("billing module wrong: %+v", schema.Modules[1])
		}
	})

	t.Run("version segment derives version distinct from module", func(t *testing.T) {
		files := []*ast.File{
			{Name: "schema/v1/iam/user.zen", Decls: []ast.Decl{entityWithIDField("User")}},
			{Name: "schema/v1/iam/sub/x.zen", Decls: []ast.Decl{entityWithIDField("Session")}},
		}

		schema, diags := ResolveWithSchemaDir(files, "schema")
		mustNotHaveErrors(t, diags)

		if len(schema.Modules) != 1 {
			t.Fatalf("Modules len = %d, want 1", len(schema.Modules))
		}

		mod := schema.Modules[0]
		if mod.Name != "iam" || mod.Version != "v1" {
			t.Fatalf("module Name/Version = %q/%q, want iam/v1", mod.Name, mod.Version)
		}

		if len(mod.Entities) != 2 {
			t.Fatalf("Entities len = %d, want 2 (deeper nesting still belongs to iam/v1)", len(mod.Entities))
		}
	})

	t.Run("version segment with no further module dir is public at that version", func(t *testing.T) {
		files := []*ast.File{
			{Name: "schema/v1/app.zen", Decls: []ast.Decl{entityWithIDField("User")}},
		}

		schema, diags := ResolveWithSchemaDir(files, "schema")
		mustNotHaveErrors(t, diags)

		if len(schema.Modules) != 1 {
			t.Fatalf("Modules len = %d, want 1", len(schema.Modules))
		}

		if schema.Modules[0].Name != "" || schema.Modules[0].Version != "v1" {
			t.Fatalf("module Name/Version = %q/%q, want \"\"/v1", schema.Modules[0].Name, schema.Modules[0].Version)
		}
	})

	t.Run("same module name across versions produces distinct modules", func(t *testing.T) {
		// Entity names must still be globally unique across the whole
		// compile (resolveSymbols' duplicate-decl check and the flat
		// entityByName/messageByName indexes are not version-scoped -- that
		// is an existing, unrelated invariant this feature does not change),
		// so this uses different entity names per version to isolate what
		// IS being tested here: that v1/iam and v2/iam land in two separate
		// *ir.Module values rather than merging into one "iam" bucket keyed
		// on name alone.
		files := []*ast.File{
			{Name: "schema/v1/iam/user.zen", Decls: []ast.Decl{entityWithIDField("UserV1")}},
			{Name: "schema/v2/iam/user.zen", Decls: []ast.Decl{entityWithIDField("UserV2")}},
		}

		schema, diags := ResolveWithSchemaDir(files, "schema")
		mustNotHaveErrors(t, diags)

		if len(schema.Modules) != 2 {
			t.Fatalf("Modules len = %d, want 2 (v1/iam and v2/iam are distinct modules)", len(schema.Modules))
		}

		if schema.Modules[0].Version != "v1" || schema.Modules[1].Version != "v2" {
			t.Fatalf("module versions = %q, %q, want v1, v2 (sorted)", schema.Modules[0].Version, schema.Modules[1].Version)
		}

		if schema.Modules[0].Name != "iam" || schema.Modules[1].Name != "iam" {
			t.Fatalf("module names = %q, %q, want iam, iam", schema.Modules[0].Name, schema.Modules[1].Name)
		}
	})

	t.Run("unversioned dir-derived module has empty Version", func(t *testing.T) {
		files := []*ast.File{
			{Name: "schema/billing/order.zen", Decls: []ast.Decl{entityWithIDField("Order")}},
		}

		schema, diags := ResolveWithSchemaDir(files, "schema")
		mustNotHaveErrors(t, diags)

		if schema.Modules[0].Version != "" {
			t.Fatalf("Version = %q, want empty for an unversioned schema", schema.Modules[0].Version)
		}
	})

	t.Run("reserved public name is error", func(t *testing.T) {
		files := []*ast.File{
			{Name: "schema/public/x.zen", Decls: []ast.Decl{entityWithIDField("User")}},
		}

		_, diags := ResolveWithSchemaDir(files, "schema")
		mustHaveErr(t, diags, ErrInvalidAttribute)
	})

	t.Run("entities partitioned into their own module via dirs", func(t *testing.T) {
		files := []*ast.File{
			{Name: "schema/billing/order.zen", Decls: []ast.Decl{entityWithIDField("Order")}},
			{Name: "schema/shipping/package.zen", Decls: []ast.Decl{entityWithIDField("Package")}},
		}

		schema, diags := ResolveWithSchemaDir(files, "schema")
		mustNotHaveErrors(t, diags)

		if len(schema.Modules) != 2 {
			t.Fatalf("Modules len = %d, want 2", len(schema.Modules))
		}

		// sorted by name: "billing" < "shipping"
		if schema.Modules[0].Name != "billing" || len(schema.Modules[0].Entities) != 1 ||
			schema.Modules[0].Entities[0].Name != "Order" {
			t.Fatalf("billing module wrong: %+v", schema.Modules[0])
		}

		if schema.Modules[1].Name != "shipping" || len(schema.Modules[1].Entities) != 1 ||
			schema.Modules[1].Entities[0].Name != "Package" {
			t.Fatalf("shipping module wrong: %+v", schema.Modules[1])
		}
	})

	t.Run("same module name reopened across files merges into one module", func(t *testing.T) {
		files := []*ast.File{
			{Name: "schema/billing/a.zen", Decls: []ast.Decl{entityWithIDField("Order")}},
			{Name: "schema/billing/b.zen", Decls: []ast.Decl{entityWithIDField("Invoice")}},
		}

		schema, diags := ResolveWithSchemaDir(files, "schema")
		mustNotHaveErrors(t, diags)

		if len(schema.Modules) != 1 {
			t.Fatalf("Modules len = %d, want 1", len(schema.Modules))
		}

		if len(schema.Modules[0].Entities) != 2 {
			t.Fatalf("Entities len = %d, want 2", len(schema.Modules[0].Entities))
		}
	})
}

// --- Pass 0: symbols ---

func TestResolveSymbolsDuplicates(t *testing.T) {
	t.Run("no collision across files", func(t *testing.T) {
		files := []*ast.File{
			{Name: "a.zen", Decls: []ast.Decl{entityWithIDField("User")}},
			{Name: "b.zen", Decls: []ast.Decl{entityWithIDField("Order")}},
		}

		_, diags := Resolve(files)
		mustNotHaveErrors(t, diags)
	})

	t.Run("same-kind collision", func(t *testing.T) {
		files := []*ast.File{
			{Name: "a.zen", Decls: []ast.Decl{entityWithIDField("User")}},
			{Name: "b.zen", Decls: []ast.Decl{entityWithIDField("User")}},
		}

		_, diags := Resolve(files)
		mustHaveErr(t, diags, ErrDuplicateDecl)
	})

	t.Run("entity vs job name collision", func(t *testing.T) {
		files := []*ast.File{
			{
				Name: "a.zen",
				Decls: []ast.Decl{
					entityWithIDField("Widget"),
					&ast.JobDecl{Name: "Widget", Queue: "default"},
				},
			},
		}

		_, diags := Resolve(files)
		mustHaveErr(t, diags, ErrDuplicateDecl)
	})
}

// --- Pass 1: field type resolution ---

func TestResolveFieldTypeScalars(t *testing.T) {
	scalars := map[string]ir.ScalarType{
		"uuid": ir.TUUID, "string": ir.TString, "int32": ir.TInt32, "int64": ir.TInt64,
		"float32": ir.TFloat32, "float64": ir.TFloat64, "bool": ir.TBool,
		"timestamp": ir.TTimestamp, "date": ir.TDate, "bytes": ir.TBytes, "json": ir.TJSON,
	}

	for name, want := range scalars {
		t.Run(name, func(t *testing.T) {
			ft, d := resolveFieldType(&ast.TypeExpr{Name: name}, nil)
			if d != nil {
				t.Fatalf("unexpected diagnostic: %v", d)
			}

			if ft.Scalar != want {
				t.Fatalf("Scalar = %v, want %v", ft.Scalar, want)
			}
		})
	}

	t.Run("unknown type name", func(t *testing.T) {
		_, d := resolveFieldType(&ast.TypeExpr{Name: "widget"}, nil)
		if d == nil || !errors.Is(d, ErrUnresolvedReference) {
			t.Fatalf("expected ErrUnresolvedReference, got %v", d)
		}
	})

	t.Run("enum with duplicate value", func(t *testing.T) {
		_, d := resolveFieldType(&ast.TypeExpr{Name: "enum", Args: []string{"a", "b", "a"}}, nil)
		if d == nil {
			t.Fatalf("expected a diagnostic for duplicate enum value")
		}
	})

	t.Run("enum requires at least one value", func(t *testing.T) {
		_, d := resolveFieldType(&ast.TypeExpr{Name: "enum"}, nil)
		if d == nil {
			t.Fatalf("expected a diagnostic for empty enum")
		}
	})

	t.Run("arg list on a non-enum scalar", func(t *testing.T) {
		_, d := resolveFieldType(&ast.TypeExpr{Name: "string", Args: []string{"x"}}, nil)
		if d == nil {
			t.Fatalf("expected a diagnostic for arg list on scalar type")
		}
	})

	t.Run("valid enum builds TEnum", func(t *testing.T) {
		ft, d := resolveFieldType(&ast.TypeExpr{Name: "enum", Args: []string{"a", "b", "c"}}, nil)
		if d != nil {
			t.Fatalf("unexpected diagnostic: %v", d)
		}

		if ft.Scalar != ir.TEnum {
			t.Fatalf("Scalar = %v, want TEnum", ft.Scalar)
		}

		if len(ft.EnumValues) != 3 {
			t.Fatalf("EnumValues = %v, want 3 values", ft.EnumValues)
		}
	})
}

// --- Pass 1: validate applicability ---

func TestResolveValidateApplicability(t *testing.T) {
	t.Run("min_len on string is fine", func(t *testing.T) {
		entity := &ast.EntityDecl{
			Name: "User",
			Fields: []*ast.FieldDecl{
				primaryUUIDField("id"),
				stringField("name", &ast.Attribute{
					Name: "validate",
					Args: []*ast.Arg{{Name: "min_len", Value: &ast.IntLit{Value: 1}}},
				}),
			},
		}

		files := []*ast.File{{Name: "a.zen", Decls: []ast.Decl{entity}}}

		schema, diags := Resolve(files)
		mustNotHaveErrors(t, diags)

		field := findEntity(schema, "User").FieldByName("name")
		if len(field.Validate) != 1 || field.Validate[0].Kind != "min_len" {
			t.Fatalf("Validate = %+v, want one min_len validation", field.Validate)
		}
	})

	t.Run("min_len on int64 is invalid", func(t *testing.T) {
		entity := &ast.EntityDecl{
			Name: "User",
			Fields: []*ast.FieldDecl{
				primaryUUIDField("id"),
				{
					Name: "age",
					Type: &ast.TypeExpr{Name: "int64"},
					Attributes: []*ast.Attribute{{
						Name: "validate",
						Args: []*ast.Arg{{Name: "min_len", Value: &ast.IntLit{Value: 1}}},
					}},
				},
			},
		}

		files := []*ast.File{{Name: "a.zen", Decls: []ast.Decl{entity}}}

		_, diags := Resolve(files)
		mustHaveErr(t, diags, ErrInvalidAttribute)
	})

	t.Run("unknown validate kind", func(t *testing.T) {
		entity := &ast.EntityDecl{
			Name: "User",
			Fields: []*ast.FieldDecl{
				primaryUUIDField("id"),
				stringField("name", &ast.Attribute{
					Name: "validate",
					Args: []*ast.Arg{{Name: "bogus", Value: &ast.IntLit{Value: 1}}},
				}),
			},
		}

		files := []*ast.File{{Name: "a.zen", Decls: []ast.Decl{entity}}}

		_, diags := Resolve(files)
		mustHaveErr(t, diags, ErrInvalidAttribute)
	})

	t.Run("gt on numeric is fine", func(t *testing.T) {
		entity := &ast.EntityDecl{
			Name: "Product",
			Fields: []*ast.FieldDecl{
				primaryUUIDField("id"),
				{
					Name: "price",
					Type: &ast.TypeExpr{Name: "float64"},
					Attributes: []*ast.Attribute{{
						Name: "validate",
						Args: []*ast.Arg{{Name: "gt", Value: &ast.FloatLit{Value: 0}}},
					}},
				},
			},
		}

		files := []*ast.File{{Name: "a.zen", Decls: []ast.Decl{entity}}}

		_, diags := Resolve(files)
		mustNotHaveErrors(t, diags)
	})

	t.Run("format restricted to fixed v1 set", func(t *testing.T) {
		entity := &ast.EntityDecl{
			Name: "User",
			Fields: []*ast.FieldDecl{
				primaryUUIDField("id"),
				stringField("contact", &ast.Attribute{
					Name: "validate",
					Args: []*ast.Arg{{Name: "format", Value: &ast.StringLit{Value: "phone"}}},
				}),
			},
		}

		files := []*ast.File{{Name: "a.zen", Decls: []ast.Decl{entity}}}

		_, diags := Resolve(files)
		mustHaveErr(t, diags, ErrInvalidAttribute)
	})

	t.Run("format email is valid", func(t *testing.T) {
		entity := &ast.EntityDecl{
			Name: "User",
			Fields: []*ast.FieldDecl{
				primaryUUIDField("id"),
				stringField("contact", &ast.Attribute{
					Name: "validate",
					Args: []*ast.Arg{{Name: "format", Value: &ast.StringLit{Value: "email"}}},
				}),
			},
		}

		files := []*ast.File{{Name: "a.zen", Decls: []ast.Decl{entity}}}

		_, diags := Resolve(files)
		mustNotHaveErrors(t, diags)
	})
}

// TestResolveValidateApplicabilityOnRPCParams runs the identical
// applicability-rule matrix TestResolveValidateApplicability runs against
// entity fields, but against RPC params instead -- proving
// resolveValidateRules (the shared helper both resolveFieldAttribute and
// resolveParams call) enforces the exact same rules at both attachment
// points, not two independently-diverging implementations.
func TestResolveValidateApplicabilityOnRPCParams(t *testing.T) {
	user := entityWithIDField("User")

	paramWithValidate := func(name, typeName string, attr *ast.Attribute) *ast.ParamDecl {
		return &ast.ParamDecl{
			Name:       name,
			Type:       &ast.TypeExpr{Name: typeName},
			Attributes: []*ast.Attribute{attr},
		}
	}

	t.Run("min_len on string param is fine", func(t *testing.T) {
		rpc := rpcDecl("CreateUser", "User", []*ast.ParamDecl{
			paramWithValidate("name", "string", &ast.Attribute{
				Name: "validate",
				Args: []*ast.Arg{{Name: "min_len", Value: &ast.IntLit{Value: 1}}},
			}),
		})
		svc := serviceDecl("UserService", rpc)

		files := []*ast.File{{Name: "a.zen", Decls: []ast.Decl{user, svc}}}

		schema, diags := Resolve(files)
		mustNotHaveErrors(t, diags)

		param := schema.Modules[0].Services[0].Operations[0].Params[0]
		if len(param.Validate) != 1 || param.Validate[0].Kind != "min_len" {
			t.Fatalf("Validate = %+v, want one min_len validation", param.Validate)
		}
	})

	t.Run("min_len on int64 param is invalid", func(t *testing.T) {
		rpc := rpcDecl("CreateUser", "User", []*ast.ParamDecl{
			paramWithValidate("age", "int64", &ast.Attribute{
				Name: "validate",
				Args: []*ast.Arg{{Name: "min_len", Value: &ast.IntLit{Value: 1}}},
			}),
		})
		svc := serviceDecl("UserService", rpc)

		files := []*ast.File{{Name: "a.zen", Decls: []ast.Decl{user, svc}}}

		_, diags := Resolve(files)
		mustHaveErr(t, diags, ErrInvalidAttribute)
	})

	t.Run("unknown validate kind on a param", func(t *testing.T) {
		rpc := rpcDecl("CreateUser", "User", []*ast.ParamDecl{
			paramWithValidate("name", "string", &ast.Attribute{
				Name: "validate",
				Args: []*ast.Arg{{Name: "bogus", Value: &ast.IntLit{Value: 1}}},
			}),
		})
		svc := serviceDecl("UserService", rpc)

		files := []*ast.File{{Name: "a.zen", Decls: []ast.Decl{user, svc}}}

		_, diags := Resolve(files)
		mustHaveErr(t, diags, ErrInvalidAttribute)
	})

	t.Run("gt on numeric param is fine", func(t *testing.T) {
		rpc := rpcDecl("CreateUser", "User", []*ast.ParamDecl{
			paramWithValidate("price", "float64", &ast.Attribute{
				Name: "validate",
				Args: []*ast.Arg{{Name: "gt", Value: &ast.FloatLit{Value: 0}}},
			}),
		})
		svc := serviceDecl("UserService", rpc)

		files := []*ast.File{{Name: "a.zen", Decls: []ast.Decl{user, svc}}}

		_, diags := Resolve(files)
		mustNotHaveErrors(t, diags)
	})

	t.Run("gt on a string param is invalid", func(t *testing.T) {
		rpc := rpcDecl("CreateUser", "User", []*ast.ParamDecl{
			paramWithValidate("name", "string", &ast.Attribute{
				Name: "validate",
				Args: []*ast.Arg{{Name: "gt", Value: &ast.IntLit{Value: 1}}},
			}),
		})
		svc := serviceDecl("UserService", rpc)

		files := []*ast.File{{Name: "a.zen", Decls: []ast.Decl{user, svc}}}

		_, diags := Resolve(files)
		mustHaveErr(t, diags, ErrInvalidAttribute)
	})

	t.Run("format restricted to fixed v1 set on a param", func(t *testing.T) {
		rpc := rpcDecl("CreateUser", "User", []*ast.ParamDecl{
			paramWithValidate("contact", "string", &ast.Attribute{
				Name: "validate",
				Args: []*ast.Arg{{Name: "format", Value: &ast.StringLit{Value: "phone"}}},
			}),
		})
		svc := serviceDecl("UserService", rpc)

		files := []*ast.File{{Name: "a.zen", Decls: []ast.Decl{user, svc}}}

		_, diags := Resolve(files)
		mustHaveErr(t, diags, ErrInvalidAttribute)
	})

	t.Run("format email on a param is valid", func(t *testing.T) {
		rpc := rpcDecl("CreateUser", "User", []*ast.ParamDecl{
			paramWithValidate("email", "string", &ast.Attribute{
				Name: "validate",
				Args: []*ast.Arg{{Name: "format", Value: &ast.StringLit{Value: "email"}}},
			}),
		})
		svc := serviceDecl("UserService", rpc)

		files := []*ast.File{{Name: "a.zen", Decls: []ast.Decl{user, svc}}}

		schema, diags := Resolve(files)
		mustNotHaveErrors(t, diags)

		param := schema.Modules[0].Services[0].Operations[0].Params[0]
		if len(param.Validate) != 1 || param.Validate[0].Kind != "format" {
			t.Fatalf("Validate = %+v, want one format validation", param.Validate)
		}
	})

	t.Run("unknown param attribute", func(t *testing.T) {
		rpc := rpcDecl("CreateUser", "User", []*ast.ParamDecl{
			{Name: "name", Type: &ast.TypeExpr{Name: "string"}, Attributes: []*ast.Attribute{{Name: "bogus"}}},
		})
		svc := serviceDecl("UserService", rpc)

		files := []*ast.File{{Name: "a.zen", Decls: []ast.Decl{user, svc}}}

		_, diags := Resolve(files)
		mustHaveErr(t, diags, ErrInvalidAttribute)
	})
}

// --- Pass 1: default values ---

func TestResolveDefaultValues(t *testing.T) {
	t.Parallel()

	t.Run("now() on timestamp is fine", func(t *testing.T) {
		entity := &ast.EntityDecl{
			Name: "User",
			Fields: []*ast.FieldDecl{
				primaryUUIDField("id"),
				{
					Name: "created_at",
					Type: &ast.TypeExpr{Name: "timestamp"},
					Attributes: []*ast.Attribute{{
						Name: "default",
						Args: []*ast.Arg{{Value: &ast.CallValue{Name: "now"}}},
					}},
				},
			},
		}

		files := []*ast.File{{Name: "a.zen", Decls: []ast.Decl{entity}}}

		schema, diags := Resolve(files)
		mustNotHaveErrors(t, diags)

		field := findEntity(schema, "User").FieldByName("created_at")
		if field.Default == nil || field.Default.Kind != "now" {
			t.Fatalf("Default = %+v, want Kind=now", field.Default)
		}
	})

	t.Run("now() on non-timestamp is invalid", func(t *testing.T) {
		entity := &ast.EntityDecl{
			Name: "User",
			Fields: []*ast.FieldDecl{
				primaryUUIDField("id"),
				stringField("name", &ast.Attribute{
					Name: "default",
					Args: []*ast.Arg{{Value: &ast.CallValue{Name: "now"}}},
				}),
			},
		}

		files := []*ast.File{{Name: "a.zen", Decls: []ast.Decl{entity}}}

		_, diags := Resolve(files)
		mustHaveErr(t, diags, ErrInvalidAttribute)
	})

	t.Run("enum default membership valid", func(t *testing.T) {
		entity := &ast.EntityDecl{
			Name: "Order",
			Fields: []*ast.FieldDecl{
				primaryUUIDField("id"),
				{
					Name: "status",
					Type: &ast.TypeExpr{Name: "enum", Args: []string{"pending", "shipped"}},
					Attributes: []*ast.Attribute{{
						Name: "default",
						Args: []*ast.Arg{{Value: &ast.IdentValue{Name: "pending"}}},
					}},
				},
			},
		}

		files := []*ast.File{{Name: "a.zen", Decls: []ast.Decl{entity}}}

		schema, diags := Resolve(files)
		mustNotHaveErrors(t, diags)

		field := findEntity(schema, "Order").FieldByName("status")
		if field.Default == nil || field.Default.Lit != "pending" {
			t.Fatalf("Default = %+v, want Lit=pending", field.Default)
		}
	})

	t.Run("enum default membership invalid", func(t *testing.T) {
		entity := &ast.EntityDecl{
			Name: "Order",
			Fields: []*ast.FieldDecl{
				primaryUUIDField("id"),
				{
					Name: "status",
					Type: &ast.TypeExpr{Name: "enum", Args: []string{"pending", "shipped"}},
					Attributes: []*ast.Attribute{{
						Name: "default",
						Args: []*ast.Arg{{Value: &ast.IdentValue{Name: "bogus"}}},
					}},
				},
			},
		}

		files := []*ast.File{{Name: "a.zen", Decls: []ast.Decl{entity}}}

		_, diags := Resolve(files)
		mustHaveErr(t, diags, ErrUnresolvedReference)
	})

	for _, tc := range []struct {
		name string
		lit  string
		want bool
	}{
		{"bool default true", "true", true},
		{"bool default false", "false", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			entity := &ast.EntityDecl{
				Name: "User",
				Fields: []*ast.FieldDecl{
					primaryUUIDField("id"),
					{
						Name: "active",
						Type: &ast.TypeExpr{Name: "bool"},
						Attributes: []*ast.Attribute{{
							Name: "default",
							Args: []*ast.Arg{{Value: &ast.IdentValue{Name: tc.lit}}},
						}},
					},
				},
			}

			files := []*ast.File{{Name: "a.zen", Decls: []ast.Decl{entity}}}

			schema, diags := Resolve(files)
			mustNotHaveErrors(t, diags)

			field := findEntity(schema, "User").FieldByName("active")
			if field.Default == nil || field.Default.Kind != "literal" || field.Default.Lit != tc.want {
				t.Fatalf("Default = %+v, want Kind=literal Lit=%v", field.Default, tc.want)
			}
		})
	}

	t.Run("bool default with a non-bool identifier is invalid", func(t *testing.T) {
		entity := &ast.EntityDecl{
			Name: "User",
			Fields: []*ast.FieldDecl{
				primaryUUIDField("id"),
				{
					Name: "active",
					Type: &ast.TypeExpr{Name: "bool"},
					Attributes: []*ast.Attribute{{
						Name: "default",
						Args: []*ast.Arg{{Value: &ast.IdentValue{Name: "yes"}}},
					}},
				},
			},
		}

		files := []*ast.File{{Name: "a.zen", Decls: []ast.Decl{entity}}}

		_, diags := Resolve(files)
		mustHaveErr(t, diags, ErrInvalidAttribute)
	})

	t.Run("timestamp accepts a string literal default", func(t *testing.T) {
		entity := &ast.EntityDecl{
			Name: "User",
			Fields: []*ast.FieldDecl{
				primaryUUIDField("id"),
				{
					Name: "created_at",
					Type: &ast.TypeExpr{Name: "timestamp"},
					Attributes: []*ast.Attribute{{
						Name: "default",
						Args: []*ast.Arg{{Value: &ast.StringLit{Value: "2024-01-01T00:00:00Z"}}},
					}},
				},
			},
		}

		files := []*ast.File{{Name: "a.zen", Decls: []ast.Decl{entity}}}

		schema, diags := Resolve(files)
		mustNotHaveErrors(t, diags)

		field := findEntity(schema, "User").FieldByName("created_at")
		if field.Default == nil || field.Default.Kind != "literal" || field.Default.Lit != "2024-01-01T00:00:00Z" {
			t.Fatalf("Default = %+v, want Kind=literal Lit=2024-01-01T00:00:00Z", field.Default)
		}
	})

	t.Run("literal default kind mismatches field scalar family", func(t *testing.T) {
		entity := &ast.EntityDecl{
			Name: "User",
			Fields: []*ast.FieldDecl{
				primaryUUIDField("id"),
				{
					Name: "age",
					Type: &ast.TypeExpr{Name: "int64"},
					Attributes: []*ast.Attribute{{
						Name: "default",
						Args: []*ast.Arg{{Value: &ast.StringLit{Value: "not-a-number"}}},
					}},
				},
			},
		}

		files := []*ast.File{{Name: "a.zen", Decls: []ast.Decl{entity}}}

		_, diags := Resolve(files)
		mustHaveErr(t, diags, ErrInvalidAttribute)
	})
}

// --- Pass 1: optional/nullable field marker ---

func TestResolveOptionalField(t *testing.T) {
	t.Run("field without ? resolves Optional=false", func(t *testing.T) {
		entity := entityWithIDField("User", stringField("name"))

		files := []*ast.File{{Name: "a.zen", Decls: []ast.Decl{entity}}}

		schema, diags := Resolve(files)
		mustNotHaveErrors(t, diags)

		field := findEntity(schema, "User").FieldByName("name")
		if field.Optional {
			t.Fatalf("Optional = true, want false")
		}
	})

	t.Run("field with ? resolves Optional=true", func(t *testing.T) {
		entity := entityWithIDField("User", &ast.FieldDecl{
			Name:     "email",
			Type:     &ast.TypeExpr{Name: "string"},
			Optional: true,
		})

		files := []*ast.File{{Name: "a.zen", Decls: []ast.Decl{entity}}}

		schema, diags := Resolve(files)
		mustNotHaveErrors(t, diags)

		field := findEntity(schema, "User").FieldByName("email")
		if !field.Optional {
			t.Fatalf("Optional = false, want true")
		}
	})

	t.Run("optional field with @default resolves both independently, no coupling", func(t *testing.T) {
		entity := entityWithIDField("User", &ast.FieldDecl{
			Name:     "nickname",
			Type:     &ast.TypeExpr{Name: "string"},
			Optional: true,
			Attributes: []*ast.Attribute{{
				Name: "default",
				Args: []*ast.Arg{{Value: &ast.StringLit{Value: "anon"}}},
			}},
		})

		files := []*ast.File{{Name: "a.zen", Decls: []ast.Decl{entity}}}

		schema, diags := Resolve(files)
		mustNotHaveErrors(t, diags)

		field := findEntity(schema, "User").FieldByName("nickname")
		if !field.Optional {
			t.Fatalf("Optional = false, want true")
		}

		if field.Default == nil || field.Default.Lit != "anon" {
			t.Fatalf("Default = %+v, want Lit=anon", field.Default)
		}
	})
}

// --- Pass 1: primary key cardinality ---

func TestResolvePrimaryCardinality(t *testing.T) {
	t.Run("zero primary fields is invalid", func(t *testing.T) {
		entity := &ast.EntityDecl{
			Name:   "User",
			Fields: []*ast.FieldDecl{stringField("name")},
		}

		files := []*ast.File{{Name: "a.zen", Decls: []ast.Decl{entity}}}

		_, diags := Resolve(files)
		mustHaveErr(t, diags, ErrInvalidAttribute)
	})

	t.Run("two primary fields is invalid", func(t *testing.T) {
		entity := &ast.EntityDecl{
			Name: "User",
			Fields: []*ast.FieldDecl{
				primaryUUIDField("id"),
				primaryUUIDField("other_id"),
			},
		}

		files := []*ast.File{{Name: "a.zen", Decls: []ast.Decl{entity}}}

		_, diags := Resolve(files)
		mustHaveErr(t, diags, ErrInvalidAttribute)
	})

	t.Run("exactly one primary field is fine", func(t *testing.T) {
		entity := entityWithIDField("User")

		files := []*ast.File{{Name: "a.zen", Decls: []ast.Decl{entity}}}

		_, diags := Resolve(files)
		mustNotHaveErrors(t, diags)
	})
}

// --- Pass 1: @schema resolution ---

func TestResolveEntitySchema(t *testing.T) {
	t.Run("explicit @schema wins over module name", func(t *testing.T) {
		entity := entityWithIDField("Order")
		entity.Attributes = []*ast.Attribute{{
			Name: "schema",
			Args: []*ast.Arg{{Value: &ast.IdentValue{Name: "billing_archive"}}},
		}}

		files := []*ast.File{
			{Name: "schema/billing/order.zen", Decls: []ast.Decl{entity}},
		}

		schema, diags := ResolveWithSchemaDir(files, "schema")
		mustNotHaveErrors(t, diags)

		if got := findEntity(schema, "Order").Schema; got != "billing_archive" {
			t.Fatalf("Schema = %q, want billing_archive", got)
		}
	})

	t.Run("module entity with no @schema defaults to module name", func(t *testing.T) {
		entity := entityWithIDField("Order")

		files := []*ast.File{
			{Name: "schema/billing/order.zen", Decls: []ast.Decl{entity}},
		}

		schema, diags := ResolveWithSchemaDir(files, "schema")
		mustNotHaveErrors(t, diags)

		if got := findEntity(schema, "Order").Schema; got != "billing" {
			t.Fatalf("Schema = %q, want billing", got)
		}
	})

	t.Run("unmoduled entity with no @schema defaults to public", func(t *testing.T) {
		entity := entityWithIDField("User")

		files := []*ast.File{{Name: "a.zen", Decls: []ast.Decl{entity}}}

		schema, diags := Resolve(files)
		mustNotHaveErrors(t, diags)

		if got := findEntity(schema, "User").Schema; got != "public" {
			t.Fatalf("Schema = %q, want public", got)
		}
	})

	t.Run("unknown entity-level attribute", func(t *testing.T) {
		entity := entityWithIDField("User")
		entity.Attributes = []*ast.Attribute{{Name: "bogus"}}

		files := []*ast.File{{Name: "a.zen", Decls: []ast.Decl{entity}}}

		_, diags := Resolve(files)
		mustHaveErr(t, diags, ErrInvalidAttribute)
	})
}

// --- multi-error / determinism ---

func TestResolveMultiError(t *testing.T) {
	entityA := &ast.EntityDecl{
		Name: "Alpha",
		Fields: []*ast.FieldDecl{
			primaryUUIDField("id"),
			{Name: "bad", Type: &ast.TypeExpr{Name: "not_a_type_a"}},
		},
	}
	entityB := &ast.EntityDecl{
		Name: "Beta",
		Fields: []*ast.FieldDecl{
			primaryUUIDField("id"),
			{Name: "bad", Type: &ast.TypeExpr{Name: "not_a_type_b"}},
		},
	}

	files := []*ast.File{{Name: "a.zen", Decls: []ast.Decl{entityA, entityB}}}

	_, diags := Resolve(files)

	count := 0

	for _, d := range diags {
		if errors.Is(d, ErrUnresolvedReference) {
			count++
		}
	}

	if count != 2 {
		t.Fatalf("expected 2 ErrUnresolvedReference diagnostics (one per entity), got %d: %v", count, diags)
	}
}

func TestResolveAlwaysReturnsNonNilSchema(t *testing.T) {
	schema, diags := Resolve(nil)
	if schema == nil {
		t.Fatalf("Resolve(nil) returned nil schema")
	}

	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %v", diags)
	}

	if len(schema.Modules) != 1 || schema.Modules[0].Name != "" {
		t.Fatalf("expected one implicit module for an empty file set, got %+v", schema.Modules)
	}
}

func TestSentinelsDeclared(t *testing.T) {
	sentinels := []error{
		ErrDuplicateDecl, ErrUnresolvedReference, ErrInvalidRelation, ErrInvalidAttribute,
		ErrInvalidOption, ErrInvalidCron, ErrMixedModuleMode, ErrCrossModule,
	}
	for _, s := range sentinels {
		if s == nil {
			t.Fatalf("sentinel is nil")
		}
	}
}

// --- Task 8 test helpers: relations, services, jobs, schedules ---

func relDecl(kind ast.RelationKind, fieldName, target string, attrs ...*ast.Attribute) *ast.RelationDecl {
	return &ast.RelationDecl{Kind: kind, FieldName: fieldName, Target: target, Attributes: attrs}
}

func m2mDecl(fieldName, target, joinTable string) *ast.RelationDecl {
	return &ast.RelationDecl{Kind: ast.ManyToMany, FieldName: fieldName, Target: target, Join: &ast.JoinBlock{Table: joinTable}}
}

func relAttr(name, val string) *ast.Attribute {
	return &ast.Attribute{Name: name, Args: []*ast.Arg{{Value: &ast.IdentValue{Name: val}}}}
}

// rpcDecl builds a minimal *ast.RPCDecl for tests unrelated to the
// secure-everything opinion (§3): it defaults Auth to `required` so those
// tests don't also have to satisfy "every operation needs auth or
// permission" just to resolve cleanly. A test that specifically exercises
// auth:/permission: resolution overwrites .Auth/.Permission afterward (see
// e.g. TestResolveAuthDecoding), which takes priority over this default.
func rpcDecl(name, returns string, params []*ast.ParamDecl) *ast.RPCDecl {
	return &ast.RPCDecl{Name: name, Returns: returns, Params: params, Auth: &ast.IdentValue{Name: "required"}}
}

func serviceDecl(name string, rpcs ...*ast.RPCDecl) *ast.ServiceDecl {
	return &ast.ServiceDecl{Name: name, RPCs: rpcs}
}

func jobDecl(name string, retry ...ast.Value) *ast.JobDecl {
	return &ast.JobDecl{Name: name, Queue: "default", Retry: retry}
}

func permCheck(check, resource, ownerField string) *ast.CallValue {
	return &ast.CallValue{Name: "check", Args: []*ast.Arg{
		{Value: &ast.StringLit{Value: check}},
		{Name: "resource", Value: &ast.IdentValue{Name: resource}},
		{Name: "owner_field", Value: &ast.IdentValue{Name: ownerField}},
	}}
}

// --- Pass 2: relation target resolution ---

func TestResolveRelationTarget(t *testing.T) {
	t.Run("valid target", func(t *testing.T) {
		user := entityWithIDField("User")
		order := entityWithIDField("Order", stringField("user_id"))
		order.Relations = []*ast.RelationDecl{
			relDecl(ast.BelongsTo, "user", "User", relAttr("foreign_key", "user_id")),
		}

		files := []*ast.File{{Name: "a.zen", Decls: []ast.Decl{user, order}}}

		schema, diags := Resolve(files)
		mustNotHaveErrors(t, diags)

		orderEntity := findEntity(schema, "Order")
		if len(orderEntity.Relations) != 1 || orderEntity.Relations[0].Target.Name != "User" {
			t.Fatalf("Relations = %+v, want one relation targeting User", orderEntity.Relations)
		}
	})

	t.Run("unknown target", func(t *testing.T) {
		order := entityWithIDField("Order", stringField("user_id"))
		order.Relations = []*ast.RelationDecl{
			relDecl(ast.BelongsTo, "user", "Nonexistent", relAttr("foreign_key", "user_id")),
		}

		files := []*ast.File{{Name: "a.zen", Decls: []ast.Decl{order}}}

		_, diags := Resolve(files)
		mustHaveErr(t, diags, ErrUnresolvedReference)
	})
}

// --- Pass 2: has_one FK uniqueness ---

func TestResolveHasOneFKUniqueness(t *testing.T) {
	t.Run("FK has @unique is valid", func(t *testing.T) {
		profile := entityWithIDField("Profile", stringField("user_id", &ast.Attribute{Name: "unique"}))
		user := entityWithIDField("User")
		user.Relations = []*ast.RelationDecl{
			relDecl(ast.HasOne, "profile", "Profile", relAttr("foreign_key", "user_id")),
		}

		files := []*ast.File{{Name: "a.zen", Decls: []ast.Decl{profile, user}}}

		_, diags := Resolve(files)
		mustNotHaveErrors(t, diags)
	})

	t.Run("FK without @unique is invalid", func(t *testing.T) {
		profile := entityWithIDField("Profile", stringField("user_id"))
		user := entityWithIDField("User")
		user.Relations = []*ast.RelationDecl{
			relDecl(ast.HasOne, "profile", "Profile", relAttr("foreign_key", "user_id")),
		}

		files := []*ast.File{{Name: "a.zen", Decls: []ast.Decl{profile, user}}}

		_, diags := Resolve(files)
		mustHaveErr(t, diags, ErrInvalidRelation)
	})
}

// --- Pass 2: @foreign_key field existence per kind ---

func TestResolveForeignKeyFieldExistence(t *testing.T) {
	t.Run("has_many FK missing on target", func(t *testing.T) {
		author := entityWithIDField("Author")
		book := entityWithIDField("Book")
		author.Relations = []*ast.RelationDecl{
			relDecl(ast.HasMany, "books", "Book", relAttr("foreign_key", "author_id")),
		}

		files := []*ast.File{{Name: "a.zen", Decls: []ast.Decl{author, book}}}

		_, diags := Resolve(files)
		mustHaveErr(t, diags, ErrInvalidRelation)
	})

	t.Run("has_many FK present on target", func(t *testing.T) {
		author := entityWithIDField("Author")
		book := entityWithIDField("Book", stringField("author_id"))
		author.Relations = []*ast.RelationDecl{
			relDecl(ast.HasMany, "books", "Book", relAttr("foreign_key", "author_id")),
		}

		files := []*ast.File{{Name: "a.zen", Decls: []ast.Decl{author, book}}}

		_, diags := Resolve(files)
		mustNotHaveErrors(t, diags)
	})

	t.Run("belongs_to FK missing on owner", func(t *testing.T) {
		author := entityWithIDField("Author")
		book := entityWithIDField("Book")
		book.Relations = []*ast.RelationDecl{
			relDecl(ast.BelongsTo, "author", "Author", relAttr("foreign_key", "author_id")),
		}

		files := []*ast.File{{Name: "a.zen", Decls: []ast.Decl{author, book}}}

		_, diags := Resolve(files)
		mustHaveErr(t, diags, ErrInvalidRelation)
	})

	t.Run("relation missing @foreign_key entirely", func(t *testing.T) {
		author := entityWithIDField("Author")
		book := entityWithIDField("Book")
		book.Relations = []*ast.RelationDecl{
			relDecl(ast.BelongsTo, "author", "Author"),
		}

		files := []*ast.File{{Name: "a.zen", Decls: []ast.Decl{author, book}}}

		_, diags := Resolve(files)
		mustHaveErr(t, diags, ErrInvalidRelation)
	})
}

// --- Pass 2: @on_delete validation ---

func TestResolveOnDelete(t *testing.T) {
	t.Run("valid on_delete value", func(t *testing.T) {
		author := entityWithIDField("Author")
		book := entityWithIDField("Book", stringField("author_id"))
		author.Relations = []*ast.RelationDecl{
			relDecl(ast.HasMany, "books", "Book", relAttr("foreign_key", "author_id"), relAttr("on_delete", "cascade")),
		}

		files := []*ast.File{{Name: "a.zen", Decls: []ast.Decl{author, book}}}

		_, diags := Resolve(files)
		mustNotHaveErrors(t, diags)
	})

	t.Run("invalid on_delete value", func(t *testing.T) {
		author := entityWithIDField("Author")
		book := entityWithIDField("Book", stringField("author_id"))
		author.Relations = []*ast.RelationDecl{
			relDecl(ast.HasMany, "books", "Book", relAttr("foreign_key", "author_id"), relAttr("on_delete", "bogus")),
		}

		files := []*ast.File{{Name: "a.zen", Decls: []ast.Decl{author, book}}}

		_, diags := Resolve(files)
		mustHaveErr(t, diags, ErrInvalidRelation)
	})
}

// --- Pass 2: many-to-many symmetry ---

func TestResolveManyToManySymmetry(t *testing.T) {
	t.Run("matched reciprocal pair", func(t *testing.T) {
		student := entityWithIDField("Student")
		course := entityWithIDField("Course")
		student.Relations = []*ast.RelationDecl{m2mDecl("courses", "Course", "student_courses")}
		course.Relations = []*ast.RelationDecl{m2mDecl("students", "Student", "student_courses")}

		files := []*ast.File{{Name: "a.zen", Decls: []ast.Decl{student, course}}}

		schema, diags := Resolve(files)
		mustNotHaveErrors(t, diags)

		s := findEntity(schema, "Student")
		c := findEntity(schema, "Course")

		if s.Relations[0].Reciprocal != c.Relations[0] || c.Relations[0].Reciprocal != s.Relations[0] {
			t.Fatalf("expected reciprocal linking both ways: %+v / %+v", s.Relations[0], c.Relations[0])
		}
	})

	t.Run("one-sided many_to_many has no reciprocal", func(t *testing.T) {
		student := entityWithIDField("Student")
		course := entityWithIDField("Course")
		student.Relations = []*ast.RelationDecl{m2mDecl("courses", "Course", "student_courses")}

		files := []*ast.File{{Name: "a.zen", Decls: []ast.Decl{student, course}}}

		_, diags := Resolve(files)
		mustHaveErr(t, diags, ErrInvalidRelation)
	})

	t.Run("mismatched join_table names is not matched", func(t *testing.T) {
		student := entityWithIDField("Student")
		course := entityWithIDField("Course")
		student.Relations = []*ast.RelationDecl{m2mDecl("courses", "Course", "table_a")}
		course.Relations = []*ast.RelationDecl{m2mDecl("students", "Student", "table_b")}

		files := []*ast.File{{Name: "a.zen", Decls: []ast.Decl{student, course}}}

		_, diags := Resolve(files)
		mustHaveErr(t, diags, ErrInvalidRelation)
	})

	t.Run("join table name reused by a different pair is an error", func(t *testing.T) {
		a1 := entityWithIDField("A1")
		b1 := entityWithIDField("B1")
		a2 := entityWithIDField("A2")
		b2 := entityWithIDField("B2")
		a1.Relations = []*ast.RelationDecl{m2mDecl("bs", "B1", "shared_table")}
		b1.Relations = []*ast.RelationDecl{m2mDecl("as", "A1", "shared_table")}
		a2.Relations = []*ast.RelationDecl{m2mDecl("bs", "B2", "shared_table")}
		b2.Relations = []*ast.RelationDecl{m2mDecl("as", "A2", "shared_table")}

		files := []*ast.File{{Name: "a.zen", Decls: []ast.Decl{a1, b1, a2, b2}}}

		_, diags := Resolve(files)
		mustHaveErr(t, diags, ErrInvalidRelation)
	})
}

// --- Pass 3: index column resolution ---

func TestResolveIndexColumns(t *testing.T) {
	t.Run("valid columns", func(t *testing.T) {
		entity := entityWithIDField("User", stringField("email"), stringField("name"))
		entity.Indexes = []*ast.IndexDecl{{Columns: []string{"email", "name"}}}

		files := []*ast.File{{Name: "a.zen", Decls: []ast.Decl{entity}}}

		schema, diags := Resolve(files)
		mustNotHaveErrors(t, diags)

		idx := findEntity(schema, "User").Indexes
		if len(idx) != 1 || len(idx[0].Columns) != 2 {
			t.Fatalf("Indexes = %+v, want one index with 2 columns", idx)
		}
	})

	t.Run("one bad column among several reported independently", func(t *testing.T) {
		entity := entityWithIDField("User", stringField("email"), stringField("name"))
		entity.Indexes = []*ast.IndexDecl{{Columns: []string{"email", "bogus1", "name", "bogus2"}}}

		files := []*ast.File{{Name: "a.zen", Decls: []ast.Decl{entity}}}

		_, diags := Resolve(files)

		count := 0

		for _, d := range diags {
			if errors.Is(d, ErrUnresolvedReference) {
				count++
			}
		}

		if count != 2 {
			t.Fatalf("expected 2 ErrUnresolvedReference for bad columns, got %d: %v", count, diags)
		}
	})
}

// --- Pass 4: RPC Returns resolution ---

func TestResolveRPCReturns(t *testing.T) {
	t.Run("valid entity return", func(t *testing.T) {
		user := entityWithIDField("User")
		svc := serviceDecl("UserService", rpcDecl("GetUser", "User", nil))

		files := []*ast.File{{Name: "a.zen", Decls: []ast.Decl{user, svc}}}

		schema, diags := Resolve(files)
		mustNotHaveErrors(t, diags)

		ret := schema.Modules[0].Services[0].Operations[0].Returns
		if ret == nil || ret.Name() != "User" || !ret.IsEntity() {
			t.Fatalf("Returns = %+v, want User entity", ret)
		}
	})

	t.Run("unknown returns name", func(t *testing.T) {
		svc := serviceDecl("UserService", rpcDecl("GetUser", "Nonexistent", nil))

		files := []*ast.File{{Name: "a.zen", Decls: []ast.Decl{svc}}}

		_, diags := Resolve(files)
		mustHaveErr(t, diags, ErrUnresolvedReference)
	})

	t.Run("returns a job name, not an entity", func(t *testing.T) {
		job := jobDecl("CleanupJob")
		svc := serviceDecl("UserService", rpcDecl("GetUser", "CleanupJob", nil))

		files := []*ast.File{{Name: "a.zen", Decls: []ast.Decl{job, svc}}}

		_, diags := Resolve(files)
		mustHaveErr(t, diags, ErrUnresolvedReference)
	})
}

// TestResolveOperationTransports proves every resolved ir.Operation always
// carries a GRPCTransport, and additionally gets an HTTPTransport when the
// rpc declares an http: option.
func TestResolveOperationTransports(t *testing.T) {
	t.Run("no http: option still gets gRPC transport", func(t *testing.T) {
		user := entityWithIDField("User")
		svc := serviceDecl("UserService", rpcDecl("GetUser", "User", nil))

		files := []*ast.File{{Name: "a.zen", Decls: []ast.Decl{user, svc}}}

		schema, diags := Resolve(files)
		mustNotHaveErrors(t, diags)

		transports := schema.Modules[0].Services[0].Operations[0].Transports
		if len(transports) != 1 {
			t.Fatalf("Transports = %+v, want exactly 1 (gRPC)", transports)
		}

		if _, ok := transports[0].(ir.GRPCTransport); !ok {
			t.Fatalf("Transports[0] = %#v, want ir.GRPCTransport", transports[0])
		}
	})

	t.Run("http: option adds HTTPTransport alongside gRPC", func(t *testing.T) {
		user := entityWithIDField("User")
		rpc := rpcDecl("GetUser", "User", []*ast.ParamDecl{{Name: "id", Type: &ast.TypeExpr{Name: "uuid"}}})
		rpc.HTTP = &ast.HTTPOption{Method: "GET", Path: "/users/{id}"}
		svc := serviceDecl("UserService", rpc)

		files := []*ast.File{{Name: "a.zen", Decls: []ast.Decl{user, svc}}}

		schema, diags := Resolve(files)
		mustNotHaveErrors(t, diags)

		transports := schema.Modules[0].Services[0].Operations[0].Transports
		if len(transports) != 2 {
			t.Fatalf("Transports = %+v, want exactly 2 (gRPC + HTTP)", transports)
		}

		if _, ok := transports[0].(ir.GRPCTransport); !ok {
			t.Fatalf("Transports[0] = %#v, want ir.GRPCTransport", transports[0])
		}

		http, ok := transports[1].(ir.HTTPTransport)
		if !ok {
			t.Fatalf("Transports[1] = %#v, want ir.HTTPTransport", transports[1])
		}

		if http.Method != nethttp.MethodGet || http.Path != "/users/{id}" {
			t.Fatalf("HTTPTransport = %+v, want Method=GET Path=/users/{id}", http)
		}
	})
}

// TestResolveOperationPaginated proves rd.Paginated threads through to
// ir.Operation.Paginated unchanged, and that an rpc with no paginated:
// option (the regression case: every rpc built before this phase) resolves
// to Paginated == false with everything else about the operation unaffected.
func TestResolveOperationPaginated(t *testing.T) {
	t.Run("paginated: true resolves to Operation.Paginated = true", func(t *testing.T) {
		task := entityWithIDField("Task")
		rpc := rpcDecl("ListTasks", "Task", nil)
		rpc.Paginated = true
		rpc.PaginatedSet = true
		svc := serviceDecl("TaskService", rpc)

		files := []*ast.File{{Name: "a.zen", Decls: []ast.Decl{task, svc}}}

		schema, diags := Resolve(files)
		mustNotHaveErrors(t, diags)

		op := schema.Modules[0].Services[0].Operations[0]
		if !op.Paginated {
			t.Fatalf("Operation.Paginated = false, want true")
		}

		if op.Returns == nil || op.Returns.Name() != "Task" {
			t.Fatalf("Returns = %+v, want the Task entity (unaffected by Paginated)", op.Returns)
		}
	})

	t.Run("absent paginated: option resolves to false (regression check)", func(t *testing.T) {
		user := entityWithIDField("User")
		svc := serviceDecl("UserService", rpcDecl("GetUser", "User", nil))

		files := []*ast.File{{Name: "a.zen", Decls: []ast.Decl{user, svc}}}

		schema, diags := Resolve(files)
		mustNotHaveErrors(t, diags)

		if schema.Modules[0].Services[0].Operations[0].Paginated {
			t.Fatalf("Operation.Paginated = true, want false for an rpc with no paginated: option")
		}
	})
}

// TestResolveNilParamType proves resolveFieldType (shared by field and
// RPC/job param resolution) reports a diagnostic instead of panicking when
// handed a *ast.ParamDecl with a nil Type — a case the parser never
// produces but the resolver's own test suite (and any other caller-built
// []*ast.File) can construct directly.
func TestResolveNilParamType(t *testing.T) {
	user := entityWithIDField("User")
	rpc := rpcDecl("GetUser", "User", []*ast.ParamDecl{{Name: "id", Type: nil}})
	svc := serviceDecl("UserService", rpc)

	files := []*ast.File{{Name: "a.zen", Decls: []ast.Decl{user, svc}}}

	var diags diag.List

	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("Resolve panicked on a nil ParamDecl.Type: %v", r)
			}
		}()

		_, diags = Resolve(files)
	}()

	mustHaveErr(t, diags, ErrUnresolvedReference)
}

// --- Pass 4: HTTP method validation ---

func TestResolveHTTPMethod(t *testing.T) {
	t.Run("valid method", func(t *testing.T) {
		user := entityWithIDField("User")
		rpc := rpcDecl("GetUser", "User", []*ast.ParamDecl{{Name: "id", Type: &ast.TypeExpr{Name: "uuid"}}})
		rpc.HTTP = &ast.HTTPOption{Method: "GET", Path: "/users/{id}"}
		svc := serviceDecl("UserService", rpc)

		files := []*ast.File{{Name: "a.zen", Decls: []ast.Decl{user, svc}}}

		_, diags := Resolve(files)
		mustNotHaveErrors(t, diags)
	})

	t.Run("invalid method", func(t *testing.T) {
		user := entityWithIDField("User")
		rpc := rpcDecl("GetUser", "User", nil)
		rpc.HTTP = &ast.HTTPOption{Method: "FETCH", Path: "/users"}
		svc := serviceDecl("UserService", rpc)

		files := []*ast.File{{Name: "a.zen", Decls: []ast.Decl{user, svc}}}

		_, diags := Resolve(files)
		mustHaveErr(t, diags, ErrInvalidOption)
	})
}

// --- Pass 4: HTTP path placeholders ---

func TestResolveHTTPPathPlaceholders(t *testing.T) {
	t.Run("matching placeholder", func(t *testing.T) {
		user := entityWithIDField("User")
		rpc := rpcDecl("GetUser", "User", []*ast.ParamDecl{{Name: "id", Type: &ast.TypeExpr{Name: "uuid"}}})
		rpc.HTTP = &ast.HTTPOption{Method: "GET", Path: "/users/{id}"}
		svc := serviceDecl("UserService", rpc)

		files := []*ast.File{{Name: "a.zen", Decls: []ast.Decl{user, svc}}}

		_, diags := Resolve(files)
		mustNotHaveErrors(t, diags)
	})

	t.Run("mismatched placeholder", func(t *testing.T) {
		user := entityWithIDField("User")
		rpc := rpcDecl("GetUser", "User", []*ast.ParamDecl{{Name: "id", Type: &ast.TypeExpr{Name: "uuid"}}})
		rpc.HTTP = &ast.HTTPOption{Method: "GET", Path: "/users/{userId}"}
		svc := serviceDecl("UserService", rpc)

		files := []*ast.File{{Name: "a.zen", Decls: []ast.Decl{user, svc}}}

		_, diags := Resolve(files)
		mustHaveErr(t, diags, ErrUnresolvedReference)
	})

	t.Run("unbalanced braces", func(t *testing.T) {
		user := entityWithIDField("User")
		rpc := rpcDecl("GetUser", "User", []*ast.ParamDecl{{Name: "id", Type: &ast.TypeExpr{Name: "uuid"}}})
		rpc.HTTP = &ast.HTTPOption{Method: "GET", Path: "/users/{id"}
		svc := serviceDecl("UserService", rpc)

		files := []*ast.File{{Name: "a.zen", Decls: []ast.Decl{user, svc}}}

		_, diags := Resolve(files)
		mustHaveErr(t, diags, ErrInvalidOption)
	})

	t.Run("empty braces", func(t *testing.T) {
		user := entityWithIDField("User")
		rpc := rpcDecl("GetUser", "User", nil)
		rpc.HTTP = &ast.HTTPOption{Method: "GET", Path: "/users/{}"}
		svc := serviceDecl("UserService", rpc)

		files := []*ast.File{{Name: "a.zen", Decls: []ast.Decl{user, svc}}}

		_, diags := Resolve(files)
		mustHaveErr(t, diags, ErrInvalidOption)
	})
}

// --- Pass 4: auth decoding ---

func TestResolveAuthDecoding(t *testing.T) {
	buildRPCWithAuth := func(auth ast.Value) []*ast.File {
		user := entityWithIDField("User")
		rpc := rpcDecl("GetUser", "User", nil)
		rpc.Auth = auth
		// This test exercises auth: decoding in isolation; a Permission is
		// attached so secure-everything (which requires Auth OR Permission)
		// doesn't itself produce a diagnostic here regardless of what auth
		// resolves to -- that opinion has its own dedicated tests.
		rpc.Permission = permCheck("read", "User", "id")
		svc := serviceDecl("UserService", rpc)

		return []*ast.File{{Name: "a.zen", Decls: []ast.Decl{user, svc}}}
	}

	t.Run("absent auth is nil", func(t *testing.T) {
		schema, diags := Resolve(buildRPCWithAuth(nil))
		mustNotHaveErrors(t, diags)

		if schema.Modules[0].Services[0].Operations[0].Auth != nil {
			t.Fatalf("Auth = %+v, want nil", schema.Modules[0].Services[0].Operations[0].Auth)
		}
	})

	t.Run("none is nil", func(t *testing.T) {
		schema, diags := Resolve(buildRPCWithAuth(&ast.IdentValue{Name: "none"}))
		mustNotHaveErrors(t, diags)

		if schema.Modules[0].Services[0].Operations[0].Auth != nil {
			t.Fatalf("Auth = %+v, want nil", schema.Modules[0].Services[0].Operations[0].Auth)
		}
	})

	t.Run("bare required", func(t *testing.T) {
		schema, diags := Resolve(buildRPCWithAuth(&ast.IdentValue{Name: "required"}))
		mustNotHaveErrors(t, diags)

		auth := schema.Modules[0].Services[0].Operations[0].Auth
		if auth == nil || !auth.Required || len(auth.Roles) != 0 {
			t.Fatalf("Auth = %+v, want Required=true, no roles", auth)
		}
	})

	t.Run("required with roles", func(t *testing.T) {
		authVal := &ast.CallValue{Name: "required", Args: []*ast.Arg{
			{Name: "roles", Value: &ast.SetLit{Items: []string{"owner", "admin"}}},
		}}

		schema, diags := Resolve(buildRPCWithAuth(authVal))
		mustNotHaveErrors(t, diags)

		auth := schema.Modules[0].Services[0].Operations[0].Auth
		if auth == nil || !auth.Required || len(auth.Roles) != 2 {
			t.Fatalf("Auth = %+v, want Required=true with 2 roles", auth)
		}
	})

	t.Run("invalid auth shape", func(t *testing.T) {
		_, diags := Resolve(buildRPCWithAuth(&ast.IdentValue{Name: "bogus"}))
		mustHaveErr(t, diags, ErrInvalidOption)
	})
}

// TestSecureEverything exercises the secure-by-default opinion
// (secureEverything): every RPC must declare auth or permission, except an
// RPC that explicitly wrote auth: none, which is the intentional escape
// hatch for public endpoints (e.g. login/sign-up).
func TestSecureEverything(t *testing.T) {
	t.Run("auth: none compiles clean", func(t *testing.T) {
		user := entityWithIDField("User")
		rpc := rpcDecl("Login", "User", nil)
		rpc.Auth = &ast.IdentValue{Name: "none"}
		svc := serviceDecl("UserService", rpc)

		files := []*ast.File{{Name: "a.zen", Decls: []ast.Decl{user, svc}}}

		_, diags := Resolve(files)
		for _, d := range diags {
			if errors.Is(d, ErrMissingAuth) {
				t.Fatalf("unexpected ErrMissingAuth for auth: none rpc: %v", diags)
			}
		}
	})

	t.Run("neither auth nor permission fails", func(t *testing.T) {
		user := entityWithIDField("User")
		rpc := rpcDecl("GetUser", "User", nil)
		rpc.Auth = nil
		svc := serviceDecl("UserService", rpc)

		files := []*ast.File{{Name: "a.zen", Decls: []ast.Decl{user, svc}}}

		_, diags := Resolve(files)
		mustHaveErr(t, diags, ErrMissingAuth)
	})

	t.Run("permission only still passes", func(t *testing.T) {
		user := entityWithIDField("User", stringField("owner_id"))
		rpc := rpcDecl("GetUser", "User", nil)
		rpc.Auth = nil
		rpc.Permission = permCheck("can_read", "User", "owner_id")
		svc := serviceDecl("UserService", rpc)

		files := []*ast.File{{Name: "a.zen", Decls: []ast.Decl{user, svc}}}

		_, diags := Resolve(files)
		mustNotHaveErrors(t, diags)
	})
}

// --- Pass 4: permission resource/owner_field ---

func TestResolvePermission(t *testing.T) {
	t.Run("valid permission", func(t *testing.T) {
		user := entityWithIDField("User", stringField("owner_id"))
		rpc := rpcDecl("GetUser", "User", nil)
		rpc.Permission = permCheck("can_read", "User", "owner_id")
		svc := serviceDecl("UserService", rpc)

		files := []*ast.File{{Name: "a.zen", Decls: []ast.Decl{user, svc}}}

		schema, diags := Resolve(files)
		mustNotHaveErrors(t, diags)

		perm := schema.Modules[0].Services[0].Operations[0].Permission
		if perm == nil || perm.Resource == nil || perm.Resource.Name != "User" ||
			perm.OwnerField == nil || perm.OwnerField.Name != "owner_id" {
			t.Fatalf("Permission = %+v, want resolved User/owner_id", perm)
		}
	})

	t.Run("unknown resource", func(t *testing.T) {
		user := entityWithIDField("User", stringField("owner_id"))
		rpc := rpcDecl("GetUser", "User", nil)
		rpc.Permission = permCheck("can_read", "Nonexistent", "owner_id")
		svc := serviceDecl("UserService", rpc)

		files := []*ast.File{{Name: "a.zen", Decls: []ast.Decl{user, svc}}}

		_, diags := Resolve(files)
		mustHaveErr(t, diags, ErrUnresolvedReference)
	})

	t.Run("unknown owner field", func(t *testing.T) {
		user := entityWithIDField("User")
		rpc := rpcDecl("GetUser", "User", nil)
		rpc.Permission = permCheck("can_read", "User", "bogus_field")
		svc := serviceDecl("UserService", rpc)

		files := []*ast.File{{Name: "a.zen", Decls: []ast.Decl{user, svc}}}

		_, diags := Resolve(files)
		mustHaveErr(t, diags, ErrUnresolvedReference)
	})

	t.Run("invalid permission shape", func(t *testing.T) {
		user := entityWithIDField("User")
		rpc := rpcDecl("GetUser", "User", nil)
		rpc.Permission = &ast.IdentValue{Name: "bogus"}
		svc := serviceDecl("UserService", rpc)

		files := []*ast.File{{Name: "a.zen", Decls: []ast.Decl{user, svc}}}

		_, diags := Resolve(files)
		mustHaveErr(t, diags, ErrInvalidOption)
	})
}

// --- Pass 4: errors: decoding ---

func TestResolveErrors(t *testing.T) {
	buildRPCWithErrors := func(items []ast.Value) []*ast.File {
		user := entityWithIDField("User")
		rpc := rpcDecl("GetUser", "User", nil)
		rpc.Errors = items
		svc := serviceDecl("UserService", rpc)

		return []*ast.File{{Name: "a.zen", Decls: []ast.Decl{user, svc}}}
	}

	t.Run("valid mixed set in declaration order", func(t *testing.T) {
		items := []ast.Value{
			&ast.IdentValue{Name: "not_found"},
			&ast.CallValue{Name: "invalid_argument", Args: []*ast.Arg{
				{Value: &ast.StringLit{Value: "email is malformed"}},
			}},
		}

		schema, diags := Resolve(buildRPCWithErrors(items))
		mustNotHaveErrors(t, diags)

		errs := schema.Modules[0].Services[0].Operations[0].Errors
		if len(errs) != 2 {
			t.Fatalf("Errors = %+v, want 2 cases", errs)
		}

		if errs[0].Code != ir.ENotFound || errs[0].Message != "" {
			t.Fatalf("Errors[0] = %+v, want {ENotFound, \"\"}", errs[0])
		}

		if errs[1].Code != ir.EInvalidArgument || errs[1].Message != "email is malformed" {
			t.Fatalf("Errors[1] = %+v, want {EInvalidArgument, \"email is malformed\"}", errs[1])
		}
	})

	t.Run("unknown code", func(t *testing.T) {
		items := []ast.Value{&ast.IdentValue{Name: "bogus_code"}}

		_, diags := Resolve(buildRPCWithErrors(items))
		mustHaveErr(t, diags, ErrInvalidOption)
	})

	t.Run("unknown code in call form", func(t *testing.T) {
		items := []ast.Value{&ast.CallValue{Name: "bogus_code"}}

		_, diags := Resolve(buildRPCWithErrors(items))
		mustHaveErr(t, diags, ErrInvalidOption)
	})

	t.Run("non-string message arg", func(t *testing.T) {
		items := []ast.Value{&ast.CallValue{Name: "not_found", Args: []*ast.Arg{
			{Value: &ast.IntLit{Value: 1}},
		}}}

		_, diags := Resolve(buildRPCWithErrors(items))
		mustHaveErr(t, diags, ErrInvalidOption)
	})

	t.Run("named arg rejected", func(t *testing.T) {
		items := []ast.Value{&ast.CallValue{Name: "not_found", Args: []*ast.Arg{
			{Name: "message", Value: &ast.StringLit{Value: "no such user"}},
		}}}

		_, diags := Resolve(buildRPCWithErrors(items))
		mustHaveErr(t, diags, ErrInvalidOption)
	})

	t.Run("extra positional arg rejected", func(t *testing.T) {
		items := []ast.Value{&ast.CallValue{Name: "not_found", Args: []*ast.Arg{
			{Value: &ast.StringLit{Value: "first"}},
			{Value: &ast.StringLit{Value: "second"}},
		}}}

		_, diags := Resolve(buildRPCWithErrors(items))
		mustHaveErr(t, diags, ErrInvalidOption)
	})

	t.Run("duplicate code within one set", func(t *testing.T) {
		items := []ast.Value{
			&ast.IdentValue{Name: "not_found"},
			&ast.IdentValue{Name: "not_found"},
		}

		_, diags := Resolve(buildRPCWithErrors(items))
		mustHaveErr(t, diags, ErrInvalidOption)
	})

	t.Run("duplicate code across bare and call forms", func(t *testing.T) {
		items := []ast.Value{
			&ast.IdentValue{Name: "not_found"},
			&ast.CallValue{Name: "not_found", Args: []*ast.Arg{
				{Value: &ast.StringLit{Value: "no such user"}},
			}},
		}

		_, diags := Resolve(buildRPCWithErrors(items))
		mustHaveErr(t, diags, ErrInvalidOption)
	})

	t.Run("invalid item shape", func(t *testing.T) {
		items := []ast.Value{&ast.StringLit{Value: "not_found"}}

		_, diags := Resolve(buildRPCWithErrors(items))
		mustHaveErr(t, diags, ErrInvalidOption)
	})

	t.Run("empty set resolves to no errors, no diagnostics", func(t *testing.T) {
		schema, diags := Resolve(buildRPCWithErrors(nil))
		mustNotHaveErrors(t, diags)

		if len(schema.Modules[0].Services[0].Operations[0].Errors) != 0 {
			t.Fatalf("Errors = %+v, want empty", schema.Modules[0].Services[0].Operations[0].Errors)
		}
	})
}

// --- Pass 5: job retry decoding ---

func TestResolveJobRetry(t *testing.T) {
	t.Run("absent retry defaults to MaxAttempts 1", func(t *testing.T) {
		job := jobDecl("CleanupJob")
		files := []*ast.File{{Name: "a.zen", Decls: []ast.Decl{job}}}

		schema, diags := Resolve(files)
		mustNotHaveErrors(t, diags)

		got := schema.Modules[0].Jobs[0].Retry
		if got.MaxAttempts != 1 {
			t.Fatalf("Retry = %+v, want MaxAttempts=1", got)
		}
	})

	t.Run("valid max_attempts", func(t *testing.T) {
		job := jobDecl("CleanupJob", &ast.CallValue{Name: "max_attempts", Args: []*ast.Arg{{Value: &ast.IntLit{Value: 5}}}})
		files := []*ast.File{{Name: "a.zen", Decls: []ast.Decl{job}}}

		schema, diags := Resolve(files)
		mustNotHaveErrors(t, diags)

		if schema.Modules[0].Jobs[0].Retry.MaxAttempts != 5 {
			t.Fatalf("MaxAttempts = %d, want 5", schema.Modules[0].Jobs[0].Retry.MaxAttempts)
		}
	})

	t.Run("invalid max_attempts (zero)", func(t *testing.T) {
		job := jobDecl("CleanupJob", &ast.CallValue{Name: "max_attempts", Args: []*ast.Arg{{Value: &ast.IntLit{Value: 0}}}})
		files := []*ast.File{{Name: "a.zen", Decls: []ast.Decl{job}}}

		_, diags := Resolve(files)
		mustHaveErr(t, diags, ErrInvalidOption)
	})

	t.Run("valid backoff", func(t *testing.T) {
		job := jobDecl("CleanupJob", &ast.CallValue{Name: "backoff", Args: []*ast.Arg{
			{Value: &ast.IdentValue{Name: "exponential"}},
			{Name: "base", Value: &ast.DurationLit{Raw: "30s", Value: 30 * time.Second, Valid: true}},
		}})
		files := []*ast.File{{Name: "a.zen", Decls: []ast.Decl{job}}}

		schema, diags := Resolve(files)
		mustNotHaveErrors(t, diags)

		retry := schema.Modules[0].Jobs[0].Retry
		if retry.Backoff != "exponential" || retry.Base != 30*time.Second {
			t.Fatalf("Retry = %+v, want exponential backoff base 30s", retry)
		}
	})

	t.Run("invalid backoff (bad duration)", func(t *testing.T) {
		job := jobDecl("CleanupJob", &ast.CallValue{Name: "backoff", Args: []*ast.Arg{
			{Value: &ast.IdentValue{Name: "exponential"}},
			{Name: "base", Value: &ast.DurationLit{Raw: "bogus", Valid: false}},
		}})
		files := []*ast.File{{Name: "a.zen", Decls: []ast.Decl{job}}}

		_, diags := Resolve(files)
		mustHaveErr(t, diags, ErrInvalidOption)
	})

	t.Run("unknown retry call name", func(t *testing.T) {
		job := jobDecl("CleanupJob", &ast.CallValue{Name: "bogus_retry"})
		files := []*ast.File{{Name: "a.zen", Decls: []ast.Decl{job}}}

		_, diags := Resolve(files)
		mustHaveErr(t, diags, ErrInvalidOption)
	})
}

// --- Pass 6: dispatch arity and cron shape ---

func TestResolveScheduleDispatch(t *testing.T) {
	t.Run("matching arity", func(t *testing.T) {
		job := jobDecl("SendEmails")
		job.Params = []*ast.ParamDecl{{Name: "batchSize", Type: &ast.TypeExpr{Name: "int32"}}}
		sched := &ast.ScheduleDecl{
			Name: "Nightly", Cron: "0 0 * * *",
			Dispatch: &ast.CallValue{Name: "SendEmails", Args: []*ast.Arg{{Value: &ast.IntLit{Value: 100}}}},
		}

		files := []*ast.File{{Name: "a.zen", Decls: []ast.Decl{job, sched}}}

		_, diags := Resolve(files)
		mustNotHaveErrors(t, diags)
	})

	t.Run("too few args", func(t *testing.T) {
		job := jobDecl("SendEmails")
		job.Params = []*ast.ParamDecl{{Name: "batchSize", Type: &ast.TypeExpr{Name: "int32"}}}
		sched := &ast.ScheduleDecl{
			Name: "Nightly", Cron: "0 0 * * *",
			Dispatch: &ast.CallValue{Name: "SendEmails"},
		}

		files := []*ast.File{{Name: "a.zen", Decls: []ast.Decl{job, sched}}}

		_, diags := Resolve(files)
		mustHaveErr(t, diags, ErrInvalidOption)
	})

	t.Run("too many args", func(t *testing.T) {
		job := jobDecl("SendEmails")
		sched := &ast.ScheduleDecl{
			Name: "Nightly", Cron: "0 0 * * *",
			Dispatch: &ast.CallValue{Name: "SendEmails", Args: []*ast.Arg{{Value: &ast.IntLit{Value: 1}}}},
		}

		files := []*ast.File{{Name: "a.zen", Decls: []ast.Decl{job, sched}}}

		_, diags := Resolve(files)
		mustHaveErr(t, diags, ErrInvalidOption)
	})

	t.Run("undeclared job", func(t *testing.T) {
		sched := &ast.ScheduleDecl{
			Name: "Nightly", Cron: "0 0 * * *",
			Dispatch: &ast.CallValue{Name: "Nonexistent"},
		}

		files := []*ast.File{{Name: "a.zen", Decls: []ast.Decl{sched}}}

		_, diags := Resolve(files)
		mustHaveErr(t, diags, ErrUnresolvedReference)
	})

	t.Run("dispatch target that isn't a job", func(t *testing.T) {
		user := entityWithIDField("User")
		sched := &ast.ScheduleDecl{
			Name: "Nightly", Cron: "0 0 * * *",
			Dispatch: &ast.CallValue{Name: "User"},
		}

		files := []*ast.File{{Name: "a.zen", Decls: []ast.Decl{user, sched}}}

		_, diags := Resolve(files)
		mustHaveErr(t, diags, ErrUnresolvedReference)
	})
}

func TestResolveCron(t *testing.T) {
	t.Run("valid cron", func(t *testing.T) {
		sched := &ast.ScheduleDecl{Name: "Nightly", Cron: "0 0 * * *"}
		files := []*ast.File{{Name: "a.zen", Decls: []ast.Decl{sched}}}

		_, diags := Resolve(files)
		mustNotHaveErrors(t, diags)
	})

	t.Run("invalid cron reachable via errors.Is through the Diagnostic wrapper", func(t *testing.T) {
		sched := &ast.ScheduleDecl{Name: "Nightly", Cron: "not a cron"}
		files := []*ast.File{{Name: "a.zen", Decls: []ast.Decl{sched}}}

		_, diags := Resolve(files)
		mustHaveErr(t, diags, ErrInvalidCron)
	})

	t.Run("missing cron option reports a real position and a clear message, not the cron library's raw error", func(t *testing.T) {
		pos := diag.Position{File: "a.zen", Line: 5, Col: 3}
		sched := &ast.ScheduleDecl{Name: "Nightly", Pos: pos}
		files := []*ast.File{{Name: "a.zen", Decls: []ast.Decl{sched}}}

		_, diags := Resolve(files)
		mustHaveErr(t, diags, ErrInvalidOption)

		var found *diag.Diagnostic

		for _, d := range diags {
			if errors.Is(d, ErrInvalidOption) {
				found = d
				break
			}
		}

		if found == nil {
			t.Fatalf("expected a diagnostic wrapping ErrInvalidOption, got: %v", diags)
		}

		if found.Pos != pos {
			t.Fatalf("diagnostic position = %v, want the schedule's own position %v", found.Pos, pos)
		}

		msg := found.Error()
		if strings.Contains(msg, "empty spec string") || strings.Contains(msg, `""`) {
			t.Fatalf("diagnostic message %q leaks the cron library's raw error / empty-string internals", msg)
		}

		if !strings.Contains(msg, "cron") {
			t.Fatalf("diagnostic message %q does not mention the missing cron option", msg)
		}
	})
}

// --- cross-module checks ---

func TestResolveCrossModuleRelations(t *testing.T) {
	buildModules := func(relKind ast.RelationKind, joinBlock *ast.JoinBlock, fk *ast.Attribute) []*ast.File {
		target := entityWithIDField("Target")
		owner := entityWithIDField("Owner")
		rel := &ast.RelationDecl{Kind: relKind, FieldName: "rel", Target: "Target", Join: joinBlock}

		if fk != nil {
			rel.Attributes = []*ast.Attribute{fk}
		}

		owner.Relations = []*ast.RelationDecl{rel}

		return []*ast.File{
			{Name: "schema/billing/owner.zen", Decls: []ast.Decl{owner}},
			{Name: "schema/shipping/target.zen", Decls: []ast.Decl{target}},
		}
	}

	t.Run("belongs_to cross-module", func(t *testing.T) {
		files := buildModules(ast.BelongsTo, nil, relAttr("foreign_key", "id"))
		_, diags := ResolveWithSchemaDir(files, "schema")
		mustHaveErr(t, diags, ErrCrossModule)
	})

	t.Run("has_many cross-module", func(t *testing.T) {
		files := buildModules(ast.HasMany, nil, relAttr("foreign_key", "id"))
		_, diags := ResolveWithSchemaDir(files, "schema")
		mustHaveErr(t, diags, ErrCrossModule)
	})

	t.Run("has_one cross-module", func(t *testing.T) {
		files := buildModules(ast.HasOne, nil, relAttr("foreign_key", "id"))
		_, diags := ResolveWithSchemaDir(files, "schema")
		mustHaveErr(t, diags, ErrCrossModule)
	})

	t.Run("many_to_many cross-module", func(t *testing.T) {
		files := buildModules(ast.ManyToMany, &ast.JoinBlock{Table: "t"}, nil)
		_, diags := ResolveWithSchemaDir(files, "schema")
		mustHaveErr(t, diags, ErrCrossModule)
	})

	t.Run("same-module relation inside a named module has no cross-module error", func(t *testing.T) {
		target := entityWithIDField("Target")
		owner := entityWithIDField("Owner")
		owner.Relations = []*ast.RelationDecl{
			relDecl(ast.BelongsTo, "rel", "Target", relAttr("foreign_key", "id")),
		}

		files := []*ast.File{
			{Name: "schema/billing/owner.zen", Decls: []ast.Decl{owner}},
			{Name: "schema/billing/target.zen", Decls: []ast.Decl{target}},
		}

		_, diags := ResolveWithSchemaDir(files, "schema")

		for _, d := range diags {
			if errors.Is(d, ErrCrossModule) {
				t.Fatalf("unexpected ErrCrossModule: %v", diags)
			}
		}
	})
}

func TestResolveCrossModuleRPCReturns(t *testing.T) {
	target := entityWithIDField("Target")
	svc := serviceDecl("BillingService", rpcDecl("GetTarget", "Target", nil))

	files := []*ast.File{
		{Name: "schema/billing/service.zen", Decls: []ast.Decl{svc}},
		{Name: "schema/shipping/target.zen", Decls: []ast.Decl{target}},
	}

	_, diags := ResolveWithSchemaDir(files, "schema")
	mustHaveErr(t, diags, ErrCrossModule)
}

func TestResolveSameModuleRPCReturnsNoError(t *testing.T) {
	target := entityWithIDField("Target")
	svc := serviceDecl("BillingService", rpcDecl("GetTarget", "Target", nil))

	files := []*ast.File{
		{Name: "schema/billing/service.zen", Decls: []ast.Decl{svc}},
		{Name: "schema/billing/target.zen", Decls: []ast.Decl{target}},
	}

	_, diags := ResolveWithSchemaDir(files, "schema")
	// Expect no cross-module error; other hard opinions (auth) may still fire, filter them.
	for _, d := range diags {
		if errors.Is(d, ErrCrossModule) {
			t.Fatalf("unexpected ErrCrossModule: %v", diags)
		}
	}
}

func TestResolveCrossModulePermissionResource(t *testing.T) {
	local := entityWithIDField("Local")
	target := entityWithIDField("Target")
	rpc := rpcDecl("GetLocal", "Local", nil)
	rpc.Permission = permCheck("can_read", "Target", "id")
	svc := serviceDecl("BillingService", rpc)

	files := []*ast.File{
		{Name: "schema/billing/service.zen", Decls: []ast.Decl{svc, local}},
		{Name: "schema/shipping/target.zen", Decls: []ast.Decl{target}},
	}

	_, diags := ResolveWithSchemaDir(files, "schema")
	mustHaveErr(t, diags, ErrCrossModule)
}

// --- Task 8 multi-error test ---

func TestResolveRelationMultiError(t *testing.T) {
	entityA := entityWithIDField("Alpha")
	entityA.Relations = []*ast.RelationDecl{relDecl(ast.BelongsTo, "bad", "NonexistentA")}

	entityB := entityWithIDField("Beta")
	entityB.Relations = []*ast.RelationDecl{relDecl(ast.BelongsTo, "bad", "NonexistentB")}

	files := []*ast.File{{Name: "a.zen", Decls: []ast.Decl{entityA, entityB}}}

	_, diags := Resolve(files)

	count := 0

	for _, d := range diags {
		if errors.Is(d, ErrUnresolvedReference) {
			count++
		}
	}

	if count != 2 {
		t.Fatalf("expected 2 ErrUnresolvedReference diagnostics (one per entity), got %d: %v", count, diags)
	}
}

// --- @renamed_from ---

// renamedFromAttr builds one @renamed_from(<value>) attribute whose single
// argument is the given AST value, so the tests below can supply a
// deliberately wrong node kind as easily as a correct one.
func renamedFromAttr(value ast.Value) *ast.Attribute {
	return &ast.Attribute{Name: "renamed_from", Args: []*ast.Arg{{Value: value}}}
}

func TestResolveFieldRenamedFromValid(t *testing.T) {
	entity := entityWithIDField("Post", stringField("heading", renamedFromAttr(&ast.StringLit{Value: "title"})))
	files := []*ast.File{{Name: "a.zen", Decls: []ast.Decl{entity}}}

	schema, diags := Resolve(files)
	mustNotHaveErrors(t, diags)

	field := findEntity(schema, "Post").FieldByName("heading")
	if field == nil {
		t.Fatalf("field %q not resolved", "heading")
	}

	if field.RenamedFrom == nil {
		t.Fatalf("RenamedFrom is nil, want %q", "title")
	}

	if *field.RenamedFrom != "title" {
		t.Fatalf("RenamedFrom = %q, want %q", *field.RenamedFrom, "title")
	}

	// Every other field keeps the nil-means-not-renamed default.
	if id := findEntity(schema, "Post").FieldByName("id"); id.RenamedFrom != nil {
		t.Fatalf("unrelated field carries RenamedFrom = %q", *id.RenamedFrom)
	}
}

func TestResolveFieldRenamedFromRejectsNonString(t *testing.T) {
	cases := map[string]ast.Value{
		"bare identifier": &ast.IdentValue{Name: "title"},
		"integer literal": &ast.IntLit{Value: 123},
	}

	for name, value := range cases {
		t.Run(name, func(t *testing.T) {
			entity := entityWithIDField("Post", stringField("heading", renamedFromAttr(value)))
			files := []*ast.File{{Name: "a.zen", Decls: []ast.Decl{entity}}}

			schema, diags := Resolve(files)
			mustHaveErr(t, diags, ErrInvalidAttribute)

			if f := findEntity(schema, "Post").FieldByName("heading"); f != nil && f.RenamedFrom != nil {
				t.Fatalf("rejected @renamed_from still set RenamedFrom = %q", *f.RenamedFrom)
			}
		})
	}
}

func TestResolveFieldRenamedFromRejectsEmptyString(t *testing.T) {
	entity := entityWithIDField("Post", stringField("heading", renamedFromAttr(&ast.StringLit{Value: ""})))
	files := []*ast.File{{Name: "a.zen", Decls: []ast.Decl{entity}}}

	schema, diags := Resolve(files)
	mustHaveErr(t, diags, ErrInvalidAttribute)

	if f := findEntity(schema, "Post").FieldByName("heading"); f != nil && f.RenamedFrom != nil {
		t.Fatalf("empty @renamed_from still set RenamedFrom = %q", *f.RenamedFrom)
	}
}

// TestResolveFieldRenamedFromRejectsSelfCollision covers both nonsensical
// shapes: renaming from a column the entity still declares as a separate
// field, and renaming a field from its own name.
func TestResolveFieldRenamedFromRejectsSelfCollision(t *testing.T) {
	t.Run("old name is another declared field", func(t *testing.T) {
		entity := entityWithIDField("Post",
			stringField("title"),
			stringField("heading", renamedFromAttr(&ast.StringLit{Value: "title"})),
		)
		files := []*ast.File{{Name: "a.zen", Decls: []ast.Decl{entity}}}

		schema, diags := Resolve(files)
		mustHaveErr(t, diags, ErrInvalidAttribute)

		if f := findEntity(schema, "Post").FieldByName("heading"); f != nil && f.RenamedFrom != nil {
			t.Fatalf("colliding @renamed_from still set RenamedFrom = %q", *f.RenamedFrom)
		}
	})

	t.Run("old name is the field itself", func(t *testing.T) {
		entity := entityWithIDField("Post", stringField("heading", renamedFromAttr(&ast.StringLit{Value: "heading"})))
		files := []*ast.File{{Name: "a.zen", Decls: []ast.Decl{entity}}}

		schema, diags := Resolve(files)
		mustHaveErr(t, diags, ErrInvalidAttribute)

		if f := findEntity(schema, "Post").FieldByName("heading"); f != nil && f.RenamedFrom != nil {
			t.Fatalf("self-naming @renamed_from still set RenamedFrom = %q", *f.RenamedFrom)
		}
	})
}

// requestParam builds an *ast.ParamDecl of ref type typeName (an
// entity/message name), the shape validateParamFields walks.
func requestParam(name, typeName string) *ast.ParamDecl {
	return &ast.ParamDecl{Name: name, Type: &ast.TypeExpr{Name: typeName}}
}

// refRPC builds an *ast.RPCDecl with auth already satisfied (so tests below
// exercise only validate-everything-in, not secure-everything) whose single
// param references reqType by name.
func refRPC(name, returns, paramName, reqType string) *ast.RPCDecl {
	return &ast.RPCDecl{
		Name:    name,
		Returns: returns,
		Params:  []*ast.ParamDecl{requestParam(paramName, reqType)},
		Auth:    &ast.IdentValue{Name: "required"},
	}
}

// refField builds an *ast.FieldDecl whose type is a bare identifier naming
// another entity/message (typeName), the shape resolveFieldForMessage
// resolves as an ir.Field.Ref.
func refField(name, typeName string) *ast.FieldDecl {
	return &ast.FieldDecl{Name: name, Type: &ast.TypeExpr{Name: typeName}}
}

// TestValidateEverythingInExemptsUnapplicableScalars covers Gap A: a
// request-position field whose scalar type has no @validate kind at all
// (uuid, bool, timestamp, ...) must not be required to declare @validate,
// since no @validate kind could ever satisfy the rule for it. A field whose
// scalar does have an applicable kind (string) must still be required to
// declare one, proving the exemption isn't overbroad.
func TestValidateEverythingInExemptsUnapplicableScalars(t *testing.T) {
	user := entityWithIDField("User")

	t.Run("uuid/bool/timestamp fields need no @validate", func(t *testing.T) {
		req := &ast.MessageDecl{
			Name: "ImageRequest",
			Fields: []*ast.FieldDecl{
				{Name: "image_id", Type: &ast.TypeExpr{Name: "uuid"}},
				{Name: "is_public", Type: &ast.TypeExpr{Name: "bool"}},
				{Name: "captured_at", Type: &ast.TypeExpr{Name: "timestamp"}},
			},
		}
		svc := serviceDecl("ImageService", refRPC("UploadImage", "User", "req", "ImageRequest"))

		files := []*ast.File{{Name: "a.zen", Decls: []ast.Decl{user, req, svc}}}

		schema, diags := Resolve(files)
		mustNotHaveErrors(t, diags)

		msg := findMessage(schema, "ImageRequest")
		if msg == nil {
			t.Fatalf("ImageRequest not resolved")
		}

		for _, fname := range []string{"image_id", "is_public", "captured_at"} {
			if f := msg.FieldByName(fname); f == nil || len(f.Validate) != 0 {
				t.Fatalf("field %s: Validate = %+v, want none declared and none required", fname, f)
			}
		}
	})

	t.Run("string field still requires @validate", func(t *testing.T) {
		req := &ast.MessageDecl{
			Name:   "NameRequest",
			Fields: []*ast.FieldDecl{stringField("name")},
		}
		svc := serviceDecl("UserService", refRPC("SetName", "User", "req", "NameRequest"))

		files := []*ast.File{{Name: "a.zen", Decls: []ast.Decl{user, req, svc}}}

		_, diags := Resolve(files)
		mustHaveErr(t, diags, ErrMissingValidation)
	})
}

// TestResolveMessageFieldRef covers Gap B: a message field can reference
// another message (declared earlier or later in source order) or an entity
// by bare name, resolving as ir.Field.Ref exactly like ir.Param.Ref does for
// RPC params -- and @validate directly on such a field is rejected, mirroring
// resolveRefParam's identical rule.
func TestResolveMessageFieldRef(t *testing.T) {
	t.Run("message field references a message declared later in source order", func(t *testing.T) {
		user := entityWithIDField("User")

		pair := &ast.MessageDecl{
			Name: "TokenPairResponse",
			Fields: []*ast.FieldDecl{
				refField("access", "TokenResponse"),
				refField("refresh", "TokenResponse"),
			},
		}
		token := &ast.MessageDecl{
			Name: "TokenResponse",
			Fields: []*ast.FieldDecl{
				stringField("value", &ast.Attribute{
					Name: "validate",
					Args: []*ast.Arg{{Name: "min_len", Value: &ast.IntLit{Value: 1}}},
				}),
				{Name: "expires_at", Type: &ast.TypeExpr{Name: "timestamp"}},
			},
		}

		// pair is declared before token: proves declaration order doesn't matter.
		files := []*ast.File{{Name: "a.zen", Decls: []ast.Decl{user, pair, token}}}

		schema, diags := Resolve(files)
		mustNotHaveErrors(t, diags)

		msg := findMessage(schema, "TokenPairResponse")
		if msg == nil {
			t.Fatalf("TokenPairResponse not resolved")
		}

		access := msg.FieldByName("access")
		if access == nil || access.Ref == nil || !access.Ref.IsMessage() || access.Ref.Name() != "TokenResponse" {
			t.Fatalf("access field = %+v, want Ref to message TokenResponse", access)
		}

		if access.Ref.Message.FieldByName("value") == nil {
			t.Fatalf("access.Ref.Message has no fields resolved: %+v", access.Ref.Message)
		}
	})

	t.Run("message field can reference an entity by name", func(t *testing.T) {
		user := entityWithIDField("User", stringField("name"))
		wrapper := &ast.MessageDecl{
			Name:   "UserWrapper",
			Fields: []*ast.FieldDecl{refField("user", "User")},
		}

		files := []*ast.File{{Name: "a.zen", Decls: []ast.Decl{user, wrapper}}}

		schema, diags := Resolve(files)
		mustNotHaveErrors(t, diags)

		msg := findMessage(schema, "UserWrapper")
		field := msg.FieldByName("user")
		if field == nil || field.Ref == nil || !field.Ref.IsEntity() || field.Ref.Name() != "User" {
			t.Fatalf("user field = %+v, want Ref to entity User", field)
		}
	})

	t.Run("@validate on a ref-typed message field is rejected", func(t *testing.T) {
		token := &ast.MessageDecl{Name: "TokenResponse", Fields: []*ast.FieldDecl{stringField("value")}}
		wrapper := &ast.MessageDecl{
			Name: "Wrapper",
			Fields: []*ast.FieldDecl{
				{
					Name: "token",
					Type: &ast.TypeExpr{Name: "TokenResponse"},
					Attributes: []*ast.Attribute{{
						Name: "validate",
						Args: []*ast.Arg{{Name: "min_len", Value: &ast.IntLit{Value: 1}}},
					}},
				},
			},
		}

		files := []*ast.File{{Name: "a.zen", Decls: []ast.Decl{token, wrapper}}}

		_, diags := Resolve(files)
		mustHaveErr(t, diags, ErrInvalidAttribute)
	})
}

// TestValidateEverythingInRecursesIntoRefFields covers the interaction
// between Gap A and Gap B: a request-position message that embeds another
// message via a Ref field must have the validate-everything-in opinion
// recurse into that embedded message's own fields too, not silently skip
// them just because the outer field itself has no @validate. It also proves
// a self-referential message (a field referencing its own message type)
// terminates rather than recursing forever.
func TestValidateEverythingInRecursesIntoRefFields(t *testing.T) {
	user := entityWithIDField("User")

	buildFiles := func(innerHasValidate bool) []*ast.File {
		var innerValue *ast.FieldDecl
		if innerHasValidate {
			innerValue = stringField("value", &ast.Attribute{
				Name: "validate",
				Args: []*ast.Arg{{Name: "min_len", Value: &ast.IntLit{Value: 1}}},
			})
		} else {
			innerValue = stringField("value")
		}

		inner := &ast.MessageDecl{Name: "Inner", Fields: []*ast.FieldDecl{innerValue}}
		outer := &ast.MessageDecl{Name: "Outer", Fields: []*ast.FieldDecl{refField("inner", "Inner")}}
		svc := serviceDecl("ThingService", refRPC("DoThing", "User", "req", "Outer"))

		return []*ast.File{{Name: "a.zen", Decls: []ast.Decl{user, inner, outer, svc}}}
	}

	t.Run("nested @validate present passes", func(t *testing.T) {
		_, diags := Resolve(buildFiles(true))
		mustNotHaveErrors(t, diags)
	})

	t.Run("missing nested @validate fails on the nested field", func(t *testing.T) {
		_, diags := Resolve(buildFiles(false))
		mustHaveErr(t, diags, ErrMissingValidation)
	})

	t.Run("self-referential message does not infinite-loop", func(_ *testing.T) {
		node := &ast.MessageDecl{Name: "Node", Fields: []*ast.FieldDecl{refField("next", "Node")}}
		svc := serviceDecl("NodeService", refRPC("Walk", "Node", "n", "Node"))

		files := []*ast.File{{Name: "a.zen", Decls: []ast.Decl{user, node, svc}}}

		// The assertion here is simply that Resolve returns at all (no
		// infinite recursion/stack overflow); go test's own default timeout
		// catches a regression.
		_, _ = Resolve(files)
	})
}

func TestVersionPathDrift(t *testing.T) {
	t.Run("mismatched path warns but does not fail compilation", func(t *testing.T) {
		user := entityWithIDField("User")
		rpc := rpcDecl("GetUser", "User", []*ast.ParamDecl{{Name: "id", Type: &ast.TypeExpr{Name: "uuid"}}})
		rpc.HTTP = &ast.HTTPOption{Method: "GET", Path: "/v2/users/{id}"}
		rpc.Auth = &ast.IdentValue{Name: "required"}
		svc := serviceDecl("UserService", rpc)

		files := []*ast.File{{Name: "schema/v1/iam/user.zen", Decls: []ast.Decl{user, svc}}}

		schema, diags := ResolveWithSchemaDir(files, "schema")

		mustHaveErr(t, diags, ErrVersionDrift)

		if diags.HasErrors() {
			t.Fatalf("version drift must be a warning, not an error: %v", diags)
		}

		if len(schema.Modules) != 1 || schema.Modules[0].Version != "v1" {
			t.Fatalf("module = %+v, want Version v1", schema.Modules)
		}
	})

	t.Run("matching path is clean", func(t *testing.T) {
		user := entityWithIDField("User")
		rpc := rpcDecl("GetUser", "User", []*ast.ParamDecl{{Name: "id", Type: &ast.TypeExpr{Name: "uuid"}}})
		rpc.HTTP = &ast.HTTPOption{Method: "GET", Path: "/v1/users/{id}"}
		rpc.Auth = &ast.IdentValue{Name: "required"}
		svc := serviceDecl("UserService", rpc)

		files := []*ast.File{{Name: "schema/v1/iam/user.zen", Decls: []ast.Decl{user, svc}}}

		_, diags := ResolveWithSchemaDir(files, "schema")
		mustNotHaveErrors(t, diags)

		for _, d := range diags {
			if errors.Is(d, ErrVersionDrift) {
				t.Fatalf("unexpected drift warning for a matching path: %v", diags)
			}
		}
	})

	t.Run("unversioned module is never checked", func(t *testing.T) {
		user := entityWithIDField("User")
		rpc := rpcDecl("GetUser", "User", []*ast.ParamDecl{{Name: "id", Type: &ast.TypeExpr{Name: "uuid"}}})
		rpc.HTTP = &ast.HTTPOption{Method: "GET", Path: "/anything/{id}"}
		rpc.Auth = &ast.IdentValue{Name: "required"}
		svc := serviceDecl("UserService", rpc)

		files := []*ast.File{{Name: "schema/billing/user.zen", Decls: []ast.Decl{user, svc}}}

		_, diags := ResolveWithSchemaDir(files, "schema")
		mustNotHaveErrors(t, diags)

		for _, d := range diags {
			if errors.Is(d, ErrVersionDrift) {
				t.Fatalf("unversioned module should never be checked: %v", diags)
			}
		}
	})
}
