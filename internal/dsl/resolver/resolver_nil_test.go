package resolver

import (
	"testing"

	"github.com/zenta-dev/zever/internal/dsl/ast"
)

func TestResolveModulesNilFileNeverPanics(t *testing.T) {
	// R24: resolver must not panic on nil *ast.File or empty Name.
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("Resolve panicked on nil file: %v", r)
		}
	}()

	cases := []struct {
		name  string
		files []*ast.File
	}{
		{"nil slice", nil},
		{"nil entry", []*ast.File{nil}},
		{"empty name", []*ast.File{{Name: "", Decls: nil}}},
		{"nil decl entry", []*ast.File{{Name: "schema/app.zen", Decls: []ast.Decl{nil, entityWithIDField("User")}}}},
		{"mixed nil and valid", []*ast.File{nil, {Name: "schema/v1/iam/user.zen", Decls: []ast.Decl{entityWithIDField("User")}}, {Name: "", Decls: nil}, nil}},
		{"all nil", []*ast.File{nil, nil}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("ResolveWithSchemaDir panicked on %q: %v", tc.name, r)
				}
			}()
			schema, diags := ResolveWithSchemaDir(tc.files, "schema")
			_ = schema
			_ = diags
			if schema == nil {
				t.Fatalf("ResolveWithSchemaDir returned nil schema for %q", tc.name)
			}
		})
	}
}

func TestVersionAndModuleForPathNeverPanics(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("versionAndModuleForPath panicked: %v", r)
		}
	}()
	// Should not panic on any combination, even empty or weird paths.
	for _, schemaDir := range []string{"", "schema", "a/b", "/abs/schema"} {
		for _, filePath := range []string{"", "schema/app.zen", "schema/v1/iam/user.zen", "/abs/schema/v2/billing/x.zen", "weird/../path.zen", string([]byte{0, 1, 2})} {
			_, _ = versionAndModuleForPath(schemaDir, filePath)
			_ = pathSegmentsUnderSchemaDir(schemaDir, filePath)
		}
	}
}
