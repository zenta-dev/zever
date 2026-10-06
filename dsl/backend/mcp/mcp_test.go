package mcp

import (
	"encoding/json"
	"testing"

	"github.com/zenta-dev/zever/dsl/ast"
	"github.com/zenta-dev/zever/dsl/ir"
	"github.com/zenta-dev/zever/dsl/parser"
	"github.com/zenta-dev/zever/dsl/resolver"
)

const testFixture = `enum Role { admin, member }

entity User {
	id: uuid @primary
	email: string
	role: Role
}

message UserCreated {
	user_id: uuid
}

service UserService {
	rpc GetUser(id: uuid) -> User {
		http: GET "/v1/users/{id}"
		auth: required
	}

	rpc ListUsers(limit: int32) -> User {
		http: GET "/v1/users"
		auth: required
		paginated: true
	}
}`

// mustResolve runs src files through the parser and resolver, failing the
// test on any diagnostic from either stage.
func mustResolve(t *testing.T, files map[string]string) *ir.Schema {
	t.Helper()

	astFiles := make([]*ast.File, 0, len(files))
	for name, src := range files {
		f, diags := parser.New(name, []byte(src)).ParseFile()
		if diags.HasErrors() {
			t.Fatalf("parse errors in %s: %v", name, diags)
		}
		astFiles = append(astFiles, f)
	}

	schema, diags := resolver.ResolveWithSchemaDir(astFiles, "schema")
	if diags.HasErrors() {
		t.Fatalf("resolve errors: %v", diags)
	}

	return schema
}

func mustGenerate(t *testing.T, schema *ir.Schema) map[string][]byte {
	t.Helper()

	out, err := New().Generate(schema)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	return out
}

func decodeManifest(t *testing.T, raw []byte) map[string]any {
	t.Helper()

	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("manifest is not JSON: %v", err)
	}

	return m
}

func TestGenerate(t *testing.T) {
	t.Parallel()

	schema := mustResolve(t, map[string]string{"schema/blog/blog.zen": testFixture})
	out := mustGenerate(t, schema)

	raw, ok := out["blog/mcp.json"]
	if !ok {
		t.Fatalf("missing blog/mcp.json, got keys %v", keysOf(out))
	}

	merged, ok := out["mcp.json"]
	if !ok {
		t.Fatalf("missing mcp.json, got keys %v", keysOf(out))
	}

	mod := decodeManifest(t, raw)
	if mod["module"] != "blog" {
		t.Fatalf("module = %v, want blog", mod["module"])
	}

	services, ok := mod["services"].([]any)
	if !ok || len(services) != 1 {
		t.Fatalf("services = %v, want one service", mod["services"])
	}

	svc, ok := services[0].(map[string]any)
	if !ok {
		t.Fatalf("service type = %T", services[0])
	}

	tools, ok := svc["tools"].([]any)
	if !ok || len(tools) != 2 {
		t.Fatalf("tools = %v, want two tools", svc["tools"])
	}

	get, ok := tools[0].(map[string]any)
	if !ok {
		t.Fatalf("tool type = %T", tools[0])
	}

	if get["name"] != "blog_UserService_GetUser" {
		t.Fatalf("tool name = %v, want blog_UserService_GetUser", get["name"])
	}

	input, ok := get["inputSchema"].(map[string]any)
	if !ok {
		t.Fatalf("inputSchema type = %T", get["inputSchema"])
	}

	props, ok := input["properties"].(map[string]any)
	if !ok {
		t.Fatalf("properties type = %T", input["properties"])
	}

	id, ok := props["id"].(map[string]any)
	if !ok || id["type"] != "string" {
		t.Fatalf("id schema = %v, want type string", props["id"])
	}

	output, ok := get["outputSchema"].(map[string]any)
	if !ok {
		t.Fatalf("outputSchema type = %T", get["outputSchema"])
	}

	if output["$ref"] != "#/definitions/blog_User" {
		t.Fatalf("outputSchema = %v, want $ref #/definitions/blog_User", output)
	}

	defs, ok := svc["definitions"].(map[string]any)
	if !ok {
		t.Fatalf("definitions type = %T", svc["definitions"])
	}

	user, ok := defs["blog_User"].(map[string]any)
	if !ok {
		t.Fatalf("definitions missing blog_User: %v", keysOfAny(defs))
	}

	userProps, ok := user["properties"].(map[string]any)
	if !ok {
		t.Fatalf("user properties type = %T", user["properties"])
	}

	role, ok := userProps["role"].(map[string]any)
	if !ok {
		t.Fatalf("role type = %T", userProps["role"])
	}

	if role["$ref"] != "#/definitions/blog_Role" {
		t.Fatalf("role schema = %v, want $ref #/definitions/blog_Role", role)
	}

	if _, hasRole := defs["blog_Role"].(map[string]any); !hasRole {
		t.Fatalf("definitions missing blog_Role: %v", keysOfAny(defs))
	}

	list, ok := tools[1].(map[string]any)
	if !ok {
		t.Fatalf("tool type = %T", tools[1])
	}

	if list["paginated"] != true {
		t.Fatalf("ListUsers paginated = %v, want true", list["paginated"])
	}

	mergedDoc := decodeManifest(t, merged)
	if _, ok := mergedDoc["services"].([]any); !ok {
		t.Fatalf("merged services type = %T", mergedDoc["services"])
	}
}

func TestGenerateDeterminism(t *testing.T) {
	t.Parallel()

	schema := mustResolve(t, map[string]string{"schema/blog/blog.zen": testFixture})

	first := mustGenerate(t, schema)
	second := mustGenerate(t, schema)

	for name, content := range first {
		other, ok := second[name]
		if !ok {
			t.Fatalf("second run missing %s", name)
		}
		if string(content) != string(other) {
			t.Fatalf("non-deterministic output for %s", name)
		}
	}
}

func TestGenerateNilSchema(t *testing.T) {
	t.Parallel()

	if _, err := New().Generate(nil); err == nil {
		t.Fatal("expected error for nil schema, got nil")
	}
}

func TestGenerateMultipleModules(t *testing.T) {
	t.Parallel()

	schema := mustResolve(t, map[string]string{
		"schema/a/a.zen": "entity ItemA {\n\tid: uuid @primary\n}\n\nservice SvcA {\n\trpc Get(id: uuid) -> ItemA {\n\t\thttp: GET \"/v1/a/{id}\"\n\t\tauth: required\n\t}\n}",
		"schema/b/b.zen": "entity ItemB {\n\tid: uuid @primary\n}\n\nservice SvcB {\n\trpc Get(id: uuid) -> ItemB {\n\t\thttp: GET \"/v1/b/{id}\"\n\t\tauth: required\n\t}\n}",
	})
	out := mustGenerate(t, schema)

	merged := decodeManifest(t, out["mcp.json"])
	services, ok := merged["services"].([]any)
	if !ok || len(services) != 2 {
		t.Fatalf("merged services = %v, want two", merged["services"])
	}

	names := map[string]bool{}
	for _, s := range services {
		svc, ok := s.(map[string]any)
		if !ok {
			t.Fatalf("service type = %T", s)
		}

		tools, ok := svc["tools"].([]any)
		if !ok || len(tools) != 1 {
			t.Fatalf("tools = %v, want one", svc["tools"])
		}

		tool, ok := tools[0].(map[string]any)
		if !ok {
			t.Fatalf("tool type = %T", tools[0])
		}

		name, ok := tool["name"].(string)
		if !ok {
			t.Fatalf("tool name type = %T", tool["name"])
		}

		if names[name] {
			t.Fatalf("duplicate tool name %q", name)
		}
		names[name] = true
	}
}

func keysOf(m map[string][]byte) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

func keysOfAny(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
