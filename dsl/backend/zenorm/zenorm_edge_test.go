package zenorm

import (
	"go/format"
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

func TestGenerateMessagesOnlyModuleSkipped(t *testing.T) {
	t.Parallel()

	schema := &ir.Schema{Modules: []*ir.Module{{
		Name:     "app",
		Messages: []*ir.Message{{Name: "CreateUserRequest"}},
	}}}

	out, err := New().Generate(schema)
	if err != nil {
		t.Fatalf("Generate error = %v, want nil", err)
	}

	if len(out) != 0 {
		t.Fatalf("Generate = %v, want no files for an entity-less module", out)
	}
}

func TestGenerateImplicitModuleRendersAppPath(t *testing.T) {
	t.Parallel()

	schema := &ir.Schema{Modules: []*ir.Module{{
		Name:     "",
		Entities: []*ir.Entity{{Name: "User"}},
	}}}

	out, err := New().Generate(schema)
	if err != nil {
		t.Fatalf("Generate error = %v, want nil", err)
	}

	if _, ok := out["orm/gen/app/app.go"]; !ok {
		t.Fatalf("Generate = %v, want orm/gen/app/app.go", out)
	}
}

func TestGenerateNamedModuleRendersSnakePath(t *testing.T) {
	t.Parallel()

	schema := &ir.Schema{Modules: []*ir.Module{{
		Name:     "user_profile",
		Entities: []*ir.Entity{{Name: "User"}},
	}}}

	out, err := New().Generate(schema)
	if err != nil {
		t.Fatalf("Generate error = %v, want nil", err)
	}

	if _, ok := out["orm/gen/user_profile/user_profile.go"]; !ok {
		t.Fatalf("Generate = %v, want orm/gen/user_profile/user_profile.go", out)
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

func TestGenerateOutputIsValidGo(t *testing.T) {
	t.Parallel()

	schema := compileSchema(t, "entity User {\n  id: uuid @primary\n  name: string\n}")

	out, err := New().Generate(schema)
	if err != nil {
		t.Fatalf("Generate error = %v, want nil", err)
	}

	for path, content := range out {
		if _, err := format.Source(content); err != nil {
			t.Fatalf("%s is not valid Go: %v", path, err)
		}
	}
}

func TestGenerateOutputMentionsTableName(t *testing.T) {
	t.Parallel()

	schema := compileSchema(t, "entity User {\n  id: uuid @primary\n}")

	out, err := New().Generate(schema)
	if err != nil {
		t.Fatalf("Generate error = %v, want nil", err)
	}

	content := string(out["orm/gen/app/app.go"])
	if !strings.Contains(content, "users") {
		t.Fatalf("generated source missing table name %q:\n%s", "users", content)
	}
}
