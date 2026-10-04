package proto

import (
	"os"
	"testing"

	"github.com/zenta-dev/zever/dsl/ast"
	"github.com/zenta-dev/zever/dsl/ir"
	"github.com/zenta-dev/zever/dsl/parser"
	"github.com/zenta-dev/zever/dsl/resolver"
)

// BenchmarkGenerateAppFixture renders the canonical app.zen fixture into
// .proto source: one schema.proto per module plus the shared annotations
// companion.
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

// BenchmarkGenerateEmptySchema is the boundary case: no modules, so only
// the annotations companion is emitted.
func BenchmarkGenerateEmptySchema(b *testing.B) {
	schema := &ir.Schema{}

	b.ReportAllocs()
	for b.Loop() {
		if _, err := New().Generate(schema); err != nil {
			b.Fatalf("Generate: %v", err)
		}
	}
}

// BenchmarkGenerateEntitiesOnly covers a schema with no services: schema
// files render, service definitions do not.
func BenchmarkGenerateEntitiesOnly(b *testing.B) {
	schema := &ir.Schema{Modules: []*ir.Module{{
		Name:     "app",
		Entities: []*ir.Entity{{Name: "User", Fields: []*ir.Field{{Name: "id"}}}},
	}}}

	b.ReportAllocs()
	for b.Loop() {
		if _, err := New().Generate(schema); err != nil {
			b.Fatalf("Generate: %v", err)
		}
	}
}

// BenchmarkGenerateSingleEntity is the smallest non-empty case.
func BenchmarkGenerateSingleEntity(b *testing.B) {
	schema := &ir.Schema{Modules: []*ir.Module{{
		Name:     "app",
		Entities: []*ir.Entity{{Name: "User", Fields: []*ir.Field{{Name: "id"}}}},
	}}}

	b.ReportAllocs()
	for b.Loop() {
		if _, err := New().Generate(schema); err != nil {
			b.Fatalf("Generate: %v", err)
		}
	}
}
