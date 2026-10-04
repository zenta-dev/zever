package openapi

import (
	"os"
	"testing"

	"github.com/zenta-dev/zever/dsl/ast"
	"github.com/zenta-dev/zever/dsl/ir"
	"github.com/zenta-dev/zever/dsl/parser"
	"github.com/zenta-dev/zever/dsl/resolver"
)

// BenchmarkGenerateAppFixture renders the canonical app.zen fixture into
// per-module OpenAPI documents plus the merged root document.
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
// the merged root document is emitted.
func BenchmarkGenerateEmptySchema(b *testing.B) {
	schema := &ir.Schema{}

	b.ReportAllocs()
	for b.Loop() {
		if _, err := New().Generate(schema); err != nil {
			b.Fatalf("Generate: %v", err)
		}
	}
}

// BenchmarkGenerateEntitiesOnly covers a schema with no HTTP operations:
// documents render with empty path sets.
func BenchmarkGenerateEntitiesOnly(b *testing.B) {
	mod := &ir.Module{Name: "app"}
	mod.Entities = []*ir.Entity{{Name: "User", Module: mod}, {Name: "Order", Module: mod}}
	schema := &ir.Schema{Modules: []*ir.Module{mod}}

	b.ReportAllocs()
	for b.Loop() {
		if _, err := New().Generate(schema); err != nil {
			b.Fatalf("Generate: %v", err)
		}
	}
}

// BenchmarkGenerateSingleOperation is the smallest non-empty case.
func BenchmarkGenerateSingleOperation(b *testing.B) {
	mod := &ir.Module{Name: "app"}
	mod.Entities = []*ir.Entity{{Name: "User", Module: mod}}
	svc := &ir.Service{Name: "UserService", Module: mod, Operations: []*ir.Operation{{
		Name:       "GetUser",
		Transports: []ir.Transport{ir.HTTPTransport{Method: "GET", Path: "/v1/users/{id}"}},
	}}}
	mod.Services = []*ir.Service{svc}
	schema := &ir.Schema{Modules: []*ir.Module{mod}}

	b.ReportAllocs()
	for b.Loop() {
		if _, err := New().Generate(schema); err != nil {
			b.Fatalf("Generate: %v", err)
		}
	}
}
