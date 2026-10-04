package atlas

import (
	"strings"
	"testing"

	"github.com/zenta-dev/zever/dsl/ir"
)

func TestGenerateEmptySchemaEmitsNoFiles(t *testing.T) {
	t.Parallel()

	out, err := New().Generate(&ir.Schema{})
	if err != nil {
		t.Fatalf("Generate(empty) error = %v, want nil", err)
	}

	if len(out) != 0 {
		t.Fatalf("Generate(empty) = %v, want no files", out)
	}
}

func TestGenerateMessagesOnlyModuleStillRendersFile(t *testing.T) {
	t.Parallel()

	schema := &ir.Schema{Modules: []*ir.Module{{
		Name:     "app",
		Messages: []*ir.Message{{Name: "CreateUserRequest"}},
	}}}

	out, err := New().Generate(schema)
	if err != nil {
		t.Fatalf("Generate error = %v, want nil", err)
	}

	if len(out) != 1 {
		t.Fatalf("Generate = %v, want exactly one schema file", out)
	}

	if _, ok := out["app/schema.hcl"]; !ok {
		t.Fatalf("Generate = %v, want app/schema.hcl", out)
	}
}

func TestGenerateImplicitModuleRendersRootSchema(t *testing.T) {
	t.Parallel()

	schema := &ir.Schema{Modules: []*ir.Module{{
		Name:     "app",
		Entities: []*ir.Entity{{Name: "User"}},
	}}}
	schema.Modules[0].Name = ""

	out, err := New().Generate(schema)
	if err != nil {
		t.Fatalf("Generate error = %v, want nil", err)
	}

	if _, ok := out["schema.hcl"]; !ok {
		t.Fatalf("Generate = %v, want schema.hcl at output root", out)
	}
}

func TestGenerateDeterministic(t *testing.T) {
	t.Parallel()

	schema := compileSchema(t, "entity User {\n  id: uuid @primary\n  name: string\n}")

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

func TestGenerateHCLContainsTableName(t *testing.T) {
	t.Parallel()

	schema := compileSchema(t, "entity User {\n  id: uuid @primary\n}")

	out, err := New().Generate(schema)
	if err != nil {
		t.Fatalf("Generate error = %v, want nil", err)
	}

	content := string(out["schema.hcl"])
	if !strings.Contains(content, "users") {
		t.Fatalf("schema.hcl missing table name %q:\n%s", "users", content)
	}
}
