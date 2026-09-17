package protogogen

import (
	"strings"
	"testing"

	"github.com/zenta-dev/zever/internal/dsl/ast"
	"github.com/zenta-dev/zever/internal/dsl/parser"
	"github.com/zenta-dev/zever/internal/dsl/resolver"
)

// compileSchema mirrors the other backends' own test helper: run src through
// the parser and resolver, failing on any diagnostic from either stage.
func compileSchema(t *testing.T, src string) *ast.File {
	t.Helper()

	p := parser.New("test.zen", []byte(src))

	file, diags := p.ParseFile()
	if diags.HasErrors() {
		t.Fatalf("parse errors: %v", diags)
	}

	return file
}

const pingFixture = `entity Ping {
	id: uuid @primary
	message: string
}

service PingService {
	rpc SendPing(message: string) -> Ping {
		http: POST "/ping"
		auth: required
		errors: { not_found }
	}
}`

func TestGenerateProducesPBAndGRPCFiles(t *testing.T) {
	file := compileSchema(t, pingFixture)

	schema, diags := resolver.Resolve([]*ast.File{file})
	if diags.HasErrors() {
		t.Fatalf("resolve errors: %v", diags)
	}

	out, err := New().Generate(schema)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	want := []string{
		"schema.pb.go",
		"schema_grpc.pb.go",
		"zengo/annotations.pb.go",
	}

	for _, path := range want {
		content, ok := out[path]
		if !ok {
			t.Errorf("Generate output missing %q, got %v", path, keysOf(out))
			continue
		}

		if len(content) == 0 {
			t.Errorf("Generate output %q is empty", path)
		}
	}

	if len(out) != len(want) {
		t.Fatalf("Generate produced %d files (%v), want %d", len(out), keysOf(out), len(want))
	}
}

func TestGenerateIsDeterministic(t *testing.T) {
	file := compileSchema(t, pingFixture)

	schema, diags := resolver.Resolve([]*ast.File{file})
	if diags.HasErrors() {
		t.Fatalf("resolve errors: %v", diags)
	}

	first, err := New().Generate(schema)
	if err != nil {
		t.Fatalf("Generate (first): %v", err)
	}

	second, err := New().Generate(schema)
	if err != nil {
		t.Fatalf("Generate (second): %v", err)
	}

	if len(first) != len(second) {
		t.Fatalf("output file count differs: %d vs %d", len(first), len(second))
	}

	for path, want := range first {
		got, ok := second[path]
		if !ok {
			t.Fatalf("second run missing %q", path)
		}

		if string(got) != string(want) {
			t.Fatalf("output for %q is not deterministic:\n--- first ---\n%s\n--- second ---\n%s", path, want, got)
		}
	}
}

// TestNewDefaultAnnotationsImportUnchanged locks in New()'s zero-config
// output: schema.pb.go must still import zever's own default annotations
// package path, exactly as before NewWithAnnotationsGoPackageRoot existed.
func TestNewDefaultAnnotationsImportUnchanged(t *testing.T) {
	file := compileSchema(t, pingFixture)

	schema, diags := resolver.Resolve([]*ast.File{file})
	if diags.HasErrors() {
		t.Fatalf("resolve errors: %v", diags)
	}

	out, err := New().Generate(schema)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	if !strings.Contains(string(out["schema.pb.go"]), "github.com/zenta-dev/zever/gen/zengo/annotations") {
		t.Fatalf("New()'s schema.pb.go no longer imports the default annotations package:\n%s", out["schema.pb.go"])
	}
}

// TestNewWithAnnotationsGoPackageRootAppliesToGeneratedGo is the regression
// case a real external project hit: protogogen drives its OWN internal
// proto.Backend instance (Generate's b.proto.Generate call), so the CLI's
// separately-constructed "proto" registry entry using the override has no
// effect on protogogen's output unless protogogen is ALSO given the same
// override -- confirming the override actually reaches the compiled Go
// import, not just the intermediate .proto text.
func TestNewWithAnnotationsGoPackageRootAppliesToGeneratedGo(t *testing.T) {
	file := compileSchema(t, pingFixture)

	schema, diags := resolver.Resolve([]*ast.File{file})
	if diags.HasErrors() {
		t.Fatalf("resolve errors: %v", diags)
	}

	const overrideRoot = "api/generated/protogogen/zengo/annotations"

	out, err := NewWithAnnotationsGoPackageRoot(overrideRoot).Generate(schema)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	got := string(out["schema.pb.go"])

	if strings.Contains(got, "github.com/zenta-dev/zever/gen/zengo/annotations") {
		t.Fatalf("schema.pb.go still imports the default annotations package despite the override:\n%s", got)
	}

	if !strings.Contains(got, overrideRoot) {
		t.Fatalf("schema.pb.go does not import the overridden annotations package %q:\n%s", overrideRoot, got)
	}
}

func keysOf(m map[string][]byte) []string {
	out := make([]string, 0, len(m))

	for k := range m {
		out = append(out, k)
	}

	return out
}
