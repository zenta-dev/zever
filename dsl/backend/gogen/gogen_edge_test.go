package gogen

import (
	"strings"
	"testing"

	"github.com/zenta-dev/zever/dsl/ir"
)

func TestGenerateEmptySchema(t *testing.T) {
	t.Parallel()

	out, err := New().Generate(&ir.Schema{})
	if err != nil {
		t.Fatalf("Generate(empty) error = %v, want nil", err)
	}

	if len(out) != 0 {
		t.Fatalf("Generate(empty) = %v, want no files", out)
	}
}

func TestGenerateSchemaWithNoServices(t *testing.T) {
	t.Parallel()

	schema := &ir.Schema{Modules: []*ir.Module{moduleWithEntities("app")}}

	out, err := New().Generate(schema)
	if err != nil {
		t.Fatalf("Generate error = %v, want nil", err)
	}

	if len(out) != 0 {
		t.Fatalf("Generate = %v, want no files for a service-less module", out)
	}
}

func TestGenerateModuleWithNoServicesAmongOnesWith(t *testing.T) {
	t.Parallel()

	withSvc := mustResolve(t, "entity Ping {\n  id: uuid @primary\n}\nservice PingService {\n  rpc Send(id: uuid) -> Ping {\n    auth: required\n  }\n}")
	schema := &ir.Schema{Modules: append([]*ir.Module{moduleWithEntities("empty")}, withSvc.Modules...)}

	out, err := New().Generate(schema)
	if err != nil {
		t.Fatalf("Generate error = %v, want nil", err)
	}

	for path := range out {
		if strings.HasPrefix(path, "empty/") {
			t.Fatalf("output %q for service-less module, want skipped", path)
		}
	}

	if len(out) == 0 {
		t.Fatal("Generate produced no files, want the service-ful module rendered")
	}
}

func TestGenerateDeterministic(t *testing.T) {
	t.Parallel()

	schema := mustResolve(t, "entity Ping {\n  id: uuid @primary\n}\nservice PingService {\n  rpc Send(id: uuid) -> Ping {\n    auth: required\n  }\n}")

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

func TestGenerateImplicitModuleRendersToAppDir(t *testing.T) {
	t.Parallel()

	schema := mustResolve(t, "entity Ping {\n  id: uuid @primary\n}\nservice PingService {\n  rpc Send(id: uuid) -> Ping {\n    auth: required\n  }\n}")

	out, err := New().Generate(schema)
	if err != nil {
		t.Fatalf("Generate error = %v, want nil", err)
	}

	if _, ok := out["app/service.go"]; !ok {
		t.Fatalf("outputs = %v, want app/service.go for the implicit module", keysOf(out))
	}
}

func TestGenerateNamedModuleRendersToSnakeDir(t *testing.T) {
	t.Parallel()

	schema := mustResolve(t, "entity Ping {\n  id: uuid @primary\n}\nservice PingService {\n  rpc Send(id: uuid) -> Ping {\n    auth: required\n  }\n}")
	for _, m := range schema.Modules {
		m.Name = "user_profile"
	}

	out, err := New().Generate(schema)
	if err != nil {
		t.Fatalf("Generate error = %v, want nil", err)
	}

	if _, ok := out["user_profile/service.go"]; !ok {
		t.Fatalf("outputs = %v, want user_profile/service.go", keysOf(out))
	}
}
