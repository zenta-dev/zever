package proto

import (
	"strings"
	"testing"

	"github.com/zenta-dev/zever/dsl/ir"
)

func TestGenerateEmptySchemaEmitsOnlyAnnotations(t *testing.T) {
	t.Parallel()

	out, err := New().Generate(&ir.Schema{})
	if err != nil {
		t.Fatalf("Generate(empty) error = %v, want nil", err)
	}

	if len(out) != 1 {
		t.Fatalf("Generate(empty) = %v, want exactly one file", out)
	}

	if _, ok := out["zever/annotations.proto"]; !ok {
		t.Fatalf("Generate(empty) = %v, want zever/annotations.proto", out)
	}
}

func TestGenerateEntitiesOnlyHasNoServiceBlock(t *testing.T) {
	t.Parallel()

	schema := &ir.Schema{Modules: []*ir.Module{{
		Name:     "app",
		Entities: []*ir.Entity{{Name: "User"}},
	}}}

	out, err := New().Generate(schema)
	if err != nil {
		t.Fatalf("Generate error = %v, want nil", err)
	}

	content := string(out["app/schema.proto"])
	if strings.Contains(content, "service ") {
		t.Fatalf("schema.proto contains a service block for an entity-only module:\n%s", content)
	}

	if !strings.Contains(content, "User") {
		t.Fatalf("schema.proto missing message User:\n%s", content)
	}
}

func TestGenerateImplicitModuleRendersRootProto(t *testing.T) {
	t.Parallel()

	schema := &ir.Schema{Modules: []*ir.Module{{
		Name:     "",
		Entities: []*ir.Entity{{Name: "User"}},
	}}}

	out, err := New().Generate(schema)
	if err != nil {
		t.Fatalf("Generate error = %v, want nil", err)
	}

	if _, ok := out["schema.proto"]; !ok {
		t.Fatalf("Generate = %v, want schema.proto at output root", out)
	}
}

func TestGenerateAnnotationsGoPackageRewrite(t *testing.T) {
	t.Parallel()

	out, err := NewWithAnnotationsGoPackageRoot("example.com/gen").Generate(&ir.Schema{})
	if err != nil {
		t.Fatalf("Generate error = %v, want nil", err)
	}

	content := string(out["zever/annotations.proto"])
	if !strings.Contains(content, "example.com/gen") {
		t.Fatalf("annotations.proto missing rewritten go_package:\n%s", content)
	}
}

func TestGenerateDeterministic(t *testing.T) {
	t.Parallel()

	schema := compileSchema(t, "entity User {\n  id: uuid @primary\n}\nservice S {\n  rpc G(id: uuid) -> User {\n    auth: required\n  }\n}")

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
