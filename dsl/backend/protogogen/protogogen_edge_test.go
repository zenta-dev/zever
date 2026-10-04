package protogogen

import (
	"strings"
	"testing"

	"github.com/zenta-dev/zever/dsl/ast"
	"github.com/zenta-dev/zever/dsl/ir"
	"github.com/zenta-dev/zever/dsl/resolver"
)

// newEmptyIRSchema returns an empty schema, the boundary input for Generate.
func newEmptyIRSchema() *ir.Schema { return &ir.Schema{} }

// newEntitiesOnlyIRSchema returns a schema with one entity and no services.
func newEntitiesOnlyIRSchema() *ir.Schema {
	return &ir.Schema{Modules: []*ir.Module{{
		Name:     "app",
		Entities: []*ir.Entity{{Name: "User"}},
	}}}
}

// mustResolveSchema resolves file, failing the test on any error.
func mustResolveSchema(t *testing.T, file *ast.File) *ir.Schema {
	t.Helper()

	schema, diags := resolver.Resolve([]*ast.File{file})
	if diags.HasErrors() {
		t.Fatalf("resolve errors: %v", diags)
	}

	return schema
}

func TestGenerateEmptySchemaEmitsOnlyAnnotations(t *testing.T) {
	t.Parallel()

	out, err := New().Generate(newEmptyIRSchema())
	if err != nil {
		t.Fatalf("Generate(empty) error = %v, want nil", err)
	}

	if len(out) != 1 {
		t.Fatalf("Generate(empty) = %v, want exactly one file", out)
	}

	if _, ok := out["zever/annotations.pb.go"]; !ok {
		t.Fatalf("Generate(empty) = %v, want zever/annotations.pb.go", out)
	}
}

func TestGenerateEntitiesOnlyHasNoGRPCStub(t *testing.T) {
	t.Parallel()

	out, err := New().Generate(newEntitiesOnlyIRSchema())
	if err != nil {
		t.Fatalf("Generate error = %v, want nil", err)
	}

	for path := range out {
		if strings.HasSuffix(path, "_grpc.pb.go") {
			t.Fatalf("output %q for entity-only schema, want no grpc stub", path)
		}
	}

	if _, ok := out["app/schema.pb.go"]; !ok {
		t.Fatalf("Generate = %v, want app/schema.pb.go", out)
	}
}

func TestGenerateDeterministic(t *testing.T) {
	t.Parallel()

	file := compileSchema(t, pingFixture)
	schema := mustResolveSchema(t, file)

	first, err1 := New().Generate(schema)
	second, err2 := New().Generate(schema)

	if err1 != nil || err2 != nil {
		t.Fatalf("Generate errors: %v / %v", err1, err2)
	}

	if len(first) != len(second) {
		t.Fatalf("file count differs: %d vs %d", len(first), len(second))
	}

	for path, content := range first {
		other, ok := second[path]
		if !ok || string(content) != string(other) {
			t.Fatalf("file %q differs between runs", path)
		}
	}
}

func TestGenerateWithAnnotationsGoPackageRoot(t *testing.T) {
	t.Parallel()

	file := compileSchema(t, pingFixture)
	schema := mustResolveSchema(t, file)

	out, err := NewWithAnnotationsGoPackageRoot("example.com/gen").Generate(schema)
	if err != nil {
		t.Fatalf("Generate error = %v, want nil", err)
	}

	content, ok := out["zever/annotations.pb.go"]
	if !ok {
		t.Fatalf("Generate = %v, want zever/annotations.pb.go", out)
	}

	if !strings.Contains(string(content), "example.com/gen") {
		t.Fatalf("annotations.pb.go missing rewritten import root:\n%s", content)
	}
}
