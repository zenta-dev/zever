package protogogen

import (
	"testing"

	"github.com/zenta-dev/zever/dsl/ast"
	"github.com/zenta-dev/zever/dsl/ir"
	"github.com/zenta-dev/zever/dsl/parser"
	"github.com/zenta-dev/zever/dsl/resolver"
)

const benchSchemaSrc = `entity User {
  id: uuid @primary
  email: string @unique
}

service UserService {
  rpc GetUser(id: uuid) -> User {
    http: GET "/v1/users/{id}"
    auth: required
  }
}`

// BenchmarkGenerateBenchSchema renders the bench schema all the way through
// protoc-gen-go and the protoc-gen-go-grpc subprocess.
func BenchmarkGenerateBenchSchema(b *testing.B) {
	file, diags := parser.New("bench.zen", []byte(benchSchemaSrc)).ParseFile()
	if diags.HasErrors() {
		b.Fatalf("parse: %v", diags)
	}

	schema, diags := resolver.Resolve([]*ast.File{file})
	if diags.HasErrors() {
		b.Fatalf("resolve: %v", diags)
	}

	b.ReportAllocs()
	b.SetBytes(int64(len(benchSchemaSrc)))
	for b.Loop() {
		if _, err := New().Generate(schema); err != nil {
			b.Fatalf("Generate: %v", err)
		}
	}
}

// BenchmarkGenerateEmptySchema is the boundary case: no modules, so only
// the annotations companion is compiled and generated.
func BenchmarkGenerateEmptySchema(b *testing.B) {
	schema := &ir.Schema{}

	b.ReportAllocs()
	for b.Loop() {
		if _, err := New().Generate(schema); err != nil {
			b.Fatalf("Generate: %v", err)
		}
	}
}

// BenchmarkGenerateEntitiesOnly covers a schema with no services: .pb.go
// files render, no grpc stubs are needed.
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
