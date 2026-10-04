package gogen

import (
	"os"
	"testing"

	"github.com/zenta-dev/zever/dsl/ast"
	"github.com/zenta-dev/zever/dsl/ir"
	"github.com/zenta-dev/zever/dsl/parser"
	"github.com/zenta-dev/zever/dsl/resolver"
)

// newTestSchema returns an empty schema, the boundary input for Generate.
func newTestSchema() *ir.Schema { return &ir.Schema{} }

// moduleWithEntities returns a module holding one entity and no services.
func moduleWithEntities(name string) *ir.Module {
	return &ir.Module{
		Name:     name,
		Entities: []*ir.Entity{{Name: "User"}},
	}
}

// BenchmarkGenerateAppFixture renders the canonical app.zen fixture (two
// services, four RPCs, one paginated) into the application-layer file set.
func BenchmarkGenerateAppFixture(b *testing.B) {
	src, err := os.ReadFile("../../compile/testdata/app.zen")
	if err != nil {
		b.Fatalf("read fixture: %v", err)
	}

	file, diags := parser.New("app.zen", src).ParseFile()
	if diags.HasErrors() {
		b.Fatalf("parse: %v", diags)
	}

	schema, diags := resolver.Resolve([]*ast.File{file})
	if diags.HasErrors() {
		b.Fatalf("resolve: %v", diags)
	}

	b.ReportAllocs()
	b.SetBytes(int64(len(src)))
	for b.Loop() {
		if _, err := New().Generate(schema); err != nil {
			b.Fatalf("Generate: %v", err)
		}
	}
}

// BenchmarkGenerateEmptySchema is the boundary case: no modules at all.
func BenchmarkGenerateEmptySchema(b *testing.B) {
	schema := newTestSchema()

	b.ReportAllocs()
	for b.Loop() {
		if _, err := New().Generate(schema); err != nil {
			b.Fatalf("Generate: %v", err)
		}
	}
}

// BenchmarkGenerateEntitiesNoServices covers the skip rule: a schema with
// entities but no services generates nothing but still walks every module.
func BenchmarkGenerateEntitiesNoServices(b *testing.B) {
	schema := newTestSchema()
	extra := moduleWithEntities("app")
	schema.Modules = append(schema.Modules, extra)

	b.ReportAllocs()
	for b.Loop() {
		if _, err := New().Generate(schema); err != nil {
			b.Fatalf("Generate: %v", err)
		}
	}
}

// BenchmarkGenerateSingleModuleOneService is the smallest non-empty case.
func BenchmarkGenerateSingleModuleOneService(b *testing.B) {
	src := "entity Ping {\n  id: uuid @primary\n}\nservice PingService {\n  rpc Send(id: uuid) -> Ping {\n    auth: required\n  }\n}"

	file, diags := parser.New("ping.zen", []byte(src)).ParseFile()
	if diags.HasErrors() {
		b.Fatalf("parse: %v", diags)
	}

	schema, diags := resolver.Resolve([]*ast.File{file})
	if diags.HasErrors() {
		b.Fatalf("resolve: %v", diags)
	}

	b.ReportAllocs()
	for b.Loop() {
		if _, err := New().Generate(schema); err != nil {
			b.Fatalf("Generate: %v", err)
		}
	}
}
