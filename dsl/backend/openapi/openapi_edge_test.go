package openapi

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/zenta-dev/zever/dsl/ir"
)

func TestGenerateEmptySchemaEmitsOnlyMergedDoc(t *testing.T) {
	t.Parallel()

	out, err := New().Generate(&ir.Schema{})
	if err != nil {
		t.Fatalf("Generate(empty) error = %v, want nil", err)
	}

	if len(out) != 1 {
		t.Fatalf("Generate(empty) = %v, want exactly one file", out)
	}

	if _, ok := out["openapi.json"]; !ok {
		t.Fatalf("Generate(empty) = %v, want openapi.json", out)
	}
}

func TestGenerateConflictingPathsAcrossModulesIsError(t *testing.T) {
	t.Parallel()

	op := func() *ir.Operation {
		return &ir.Operation{
			Name:       "GetUser",
			Transports: []ir.Transport{ir.HTTPTransport{Method: "GET", Path: "/v1/users/{id}"}},
		}
	}

	schema := &ir.Schema{Modules: []*ir.Module{
		{
			Name:     "a",
			Services: []*ir.Service{{Name: "S", Operations: []*ir.Operation{op()}}},
		},
		{
			Name:     "b",
			Services: []*ir.Service{{Name: "S", Operations: []*ir.Operation{op()}}},
		},
	}}

	if _, err := New().Generate(schema); err == nil {
		t.Fatal("Generate error = nil, want conflict error for identical method+path in two modules")
	}
}

func TestGenerateDeterministic(t *testing.T) {
	t.Parallel()

	schema := compileSchema(t, "entity User {\n  id: uuid @primary\n}\nservice S {\n  rpc G(id: uuid) -> User {\n    http: GET \"/v1/users/{id}\"\n    auth: required\n  }\n}")

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

func TestGenerateOutputIsValidJSON(t *testing.T) {
	t.Parallel()

	schema := compileSchema(t, "entity User {\n  id: uuid @primary\n}\nservice S {\n  rpc G(id: uuid) -> User {\n    http: GET \"/v1/users/{id}\"\n    auth: required\n  }\n}")

	out, err := New().Generate(schema)
	if err != nil {
		t.Fatalf("Generate error = %v, want nil", err)
	}

	for path, content := range out {
		var doc map[string]any
		if err := json.Unmarshal(content, &doc); err != nil {
			t.Fatalf("%s is not valid JSON: %v", path, err)
		}
	}
}

func TestGenerateModuleDocContainsPath(t *testing.T) {
	t.Parallel()

	schema := compileSchema(t, "entity User {\n  id: uuid @primary\n}\nservice S {\n  rpc G(id: uuid) -> User {\n    http: GET \"/v1/users/{id}\"\n    auth: required\n  }\n}")

	out, err := New().Generate(schema)
	if err != nil {
		t.Fatalf("Generate error = %v, want nil", err)
	}

	content := string(out["default/openapi.json"])
	if !strings.Contains(content, "/v1/users/{id}") {
		t.Fatalf("module doc missing path:\n%s", content)
	}
}
