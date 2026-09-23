package protogogen

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/pluginpb"

	"github.com/zenta-dev/zever/internal/dsl/ast"
	protobackend "github.com/zenta-dev/zever/internal/dsl/backend/proto"
	"github.com/zenta-dev/zever/internal/dsl/ir"
	"github.com/zenta-dev/zever/internal/dsl/resolver"
)

// resolvePingSchema parses pingFixture and resolves it, failing the test on
// any diagnostic. Shared by the error-path tests that need a valid schema
// or its compiled request as a starting point.
func resolvePingSchema(t *testing.T) *ir.Schema {
	t.Helper()

	file := compileSchema(t, pingFixture)

	schema, diags := resolver.Resolve([]*ast.File{file})
	if diags.HasErrors() {
		t.Fatalf("resolve errors: %v", diags)
	}

	return schema
}

// pingRequest runs the ping schema through the proto backend and
// buildCodeGeneratorRequest, returning the same request Generate feeds both
// plugins. Tests mutate the returned request (e.g. Parameter) to steer the
// plugins into their error paths.
func pingRequest(t *testing.T) *pluginpb.CodeGeneratorRequest {
	t.Helper()

	protoFiles, err := protobackend.New().Generate(resolvePingSchema(t))
	if err != nil {
		t.Fatalf("proto Generate: %v", err)
	}

	req, err := buildCodeGeneratorRequest(protoFiles)
	if err != nil {
		t.Fatalf("buildCodeGeneratorRequest: %v", err)
	}

	return req
}

// noGoPackageRequest compiles a proto file without an `option go_package`
// line. protocompile accepts it (go_package is a plugin-level concern), but
// both protogen.New and the real protoc-gen-go-grpc binary reject it, which
// steers both codegen paths into their error branches.
func noGoPackageRequest(t *testing.T) *pluginpb.CodeGeneratorRequest {
	t.Helper()

	req, err := buildCodeGeneratorRequest(map[string][]byte{
		"nogo.proto": []byte("syntax = \"proto3\";\npackage t;\nmessage M { string a = 1; }\n"),
	})
	if err != nil {
		t.Fatalf("buildCodeGeneratorRequest: %v", err)
	}

	return req
}

// writeFakeGo shadows the `go` binary on PATH with a script whose body is
// body. Tests use it to fault-inject generateGRPCGo's subprocess: a script
// that prints garbage covers the response-unmarshal branch, one that exits
// nonzero covers the run branch through Generate.
func writeFakeGo(t *testing.T, body string) {
	t.Helper()

	dir := t.TempDir()

	if err := os.WriteFile(filepath.Join(dir, "go"), []byte(body), 0o755); err != nil { //nolint:gosec // test-only fake `go` binary must be executable to shadow PATH
		t.Fatalf("write fake go: %v", err)
	}

	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func TestNameReturnsProtogogen(t *testing.T) {
	if got := New().Name(); got != "protogogen" {
		t.Errorf("Name() = %q, want %q", got, "protogogen")
	}
}

func TestGenerateProtoRenderError(t *testing.T) {
	// "myVal" and "my_val" both scream to MY_VAL, so the proto backend
	// rejects the schema before any compilation happens.
	schema := &ir.Schema{Modules: []*ir.Module{{
		Name:  "m",
		Enums: []*ir.Enum{{Name: "E", Values: []string{"myVal", "my_val"}}},
	}}}

	_, err := New().Generate(schema)
	if err == nil {
		t.Fatal("Generate succeeded, want proto render error")
	}

	if !strings.Contains(err.Error(), "render proto") {
		t.Errorf("Generate error = %v, want it to mention render proto", err)
	}
}

func TestGenerateCompileError(t *testing.T) {
	// The module name flows raw into `package zever.<name>.v1`, so a space
	// renders fine but fails protocompile.
	schema := &ir.Schema{Modules: []*ir.Module{{
		Name: "bad name",
		Entities: []*ir.Entity{{
			Name:   "Ping",
			Fields: []*ir.Field{{Name: "message", Type: ir.FieldType{Scalar: ir.TString}}},
		}},
	}}}

	_, err := New().Generate(schema)
	if err == nil {
		t.Fatal("Generate succeeded, want proto compile error")
	}

	if !strings.Contains(err.Error(), "compile proto sources") {
		t.Errorf("Generate error = %v, want it to mention compile proto sources", err)
	}
}

func TestGeneratePBError(t *testing.T) {
	// Dots are legal in a proto package but the derived Go package name
	// "my.modv1" is not a valid identifier, so protocompile succeeds and
	// protoc-gen-go fails.
	schema := &ir.Schema{Modules: []*ir.Module{{
		Name: "my.mod",
		Entities: []*ir.Entity{{
			Name:   "Ping",
			Fields: []*ir.Field{{Name: "message", Type: ir.FieldType{Scalar: ir.TString}}},
		}},
	}}}

	_, err := New().Generate(schema)
	if err == nil {
		t.Fatal("Generate succeeded, want protoc-gen-go error")
	}

	if !strings.Contains(err.Error(), "protoc-gen-go") {
		t.Errorf("Generate error = %v, want it to mention protoc-gen-go", err)
	}
}

func TestGenerateGRPCError(t *testing.T) {
	// The message path is generated in-process and succeeds; only the
	// grpc plugin subprocess fails, via a shadowed `go` that exits nonzero.
	writeFakeGo(t, "#!/bin/sh\nexit 1\n")

	_, err := New().Generate(resolvePingSchema(t))
	if err == nil {
		t.Fatal("Generate succeeded, want protoc-gen-go-grpc error")
	}

	if !strings.Contains(err.Error(), "protoc-gen-go-grpc") {
		t.Errorf("Generate error = %v, want it to mention protoc-gen-go-grpc", err)
	}
}

func TestBuildCodeGeneratorRequestCompileError(t *testing.T) {
	_, err := buildCodeGeneratorRequest(map[string][]byte{
		"bad.proto": []byte("syntax = \"proto3\";\npackage;\nthis is not proto\n"),
	})
	if err == nil {
		t.Fatal("buildCodeGeneratorRequest succeeded, want compile error")
	}

	if !strings.Contains(err.Error(), "compile proto sources") {
		t.Errorf("buildCodeGeneratorRequest error = %v, want it to mention compile proto sources", err)
	}
}

func TestGeneratePBGoProtogenNewError(t *testing.T) {
	_, err := generatePBGo(noGoPackageRequest(t))
	if err == nil {
		t.Fatal("generatePBGo succeeded, want protogen.New error")
	}

	if !strings.Contains(err.Error(), "protogen.Options.New") {
		t.Errorf("generatePBGo error = %v, want it to mention protogen.Options.New", err)
	}
}

func TestGeneratePBGoResponseError(t *testing.T) {
	// module=zzz makes every generated filename miss its required prefix,
	// so protogen.New succeeds but Response carries the failure.
	req := pingRequest(t)
	req.Parameter = proto.String("module=zzz")

	_, err := generatePBGo(req)
	if err == nil {
		t.Fatal("generatePBGo succeeded, want response error")
	}

	if !strings.Contains(err.Error(), "protoc-gen-go") {
		t.Errorf("generatePBGo error = %v, want it to mention protoc-gen-go", err)
	}
}

func TestGenerateGRPCGoMarshalError(t *testing.T) {
	// No t.Parallel here: this test swaps the package-level protoMarshal
	// test seam, and the package has no parallel tests, so a plain
	// save/restore with t.Cleanup is race-safe without a mutex.
	orig := protoMarshal
	protoMarshal = func(proto.Message) ([]byte, error) {
		return nil, errors.New("boom")
	}
	t.Cleanup(func() { protoMarshal = orig })

	_, err := generateGRPCGo(context.Background(), pingRequest(t))
	if err == nil {
		t.Fatal("generateGRPCGo succeeded, want marshal error")
	}

	if !strings.Contains(err.Error(), "marshal CodeGeneratorRequest") {
		t.Errorf("generateGRPCGo error = %v, want it to mention marshal CodeGeneratorRequest", err)
	}
}

func TestGenerateGRPCGoRunError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := generateGRPCGo(ctx, pingRequest(t))
	if err == nil {
		t.Fatal("generateGRPCGo succeeded, want run error")
	}

	if !strings.Contains(err.Error(), "protoc-gen-go-grpc") {
		t.Errorf("generateGRPCGo error = %v, want it to mention protoc-gen-go-grpc", err)
	}
}

func TestGenerateGRPCGoUnmarshalError(t *testing.T) {
	// A shadowed `go` that exits 0 printing garbage: the run succeeds but
	// stdout is not a CodeGeneratorResponse.
	writeFakeGo(t, "#!/bin/sh\nprintf 'not-protobuf-at-all'\n")

	_, err := generateGRPCGo(context.Background(), pingRequest(t))
	if err == nil {
		t.Fatal("generateGRPCGo succeeded, want unmarshal error")
	}

	if !strings.Contains(err.Error(), "unmarshal") {
		t.Errorf("generateGRPCGo error = %v, want it to mention unmarshal", err)
	}
}

func TestGenerateGRPCGoResponseError(t *testing.T) {
	// module=zzz makes the real plugin exit 0 with Error set in its
	// response, covering the response-error branch of the subprocess path.
	// A file without go_package would fail earlier inside protogen.New
	// (exit 1, run branch), so the ping request with an overridden
	// parameter is the shape that reaches here.
	req := pingRequest(t)
	req.Parameter = proto.String("module=zzz")

	_, err := generateGRPCGo(context.Background(), req)
	if err == nil {
		t.Fatal("generateGRPCGo succeeded, want response error")
	}

	if !strings.Contains(err.Error(), "protoc-gen-go-grpc") {
		t.Errorf("generateGRPCGo error = %v, want it to mention protoc-gen-go-grpc", err)
	}
}

// TestGenerateGolden locks the full Generate output for testdata/fixture.zen
// byte-for-byte against testdata/golden. The toolchain is pinned in go.mod
// (protocompile + protoc-gen-go-grpc tool directive on protobuf v1.36.12),
// so codegen output is deterministic; any golden drift means the backend or
// the toolchain changed and deserves a conscious golden refresh.
func TestGenerateGolden(t *testing.T) {
	src, err := os.ReadFile(filepath.Join("testdata", "fixture.zen"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}

	// Route through the shared helper so fixture.zen and pingFixture can
	// never drift apart silently: any divergence fails the golden compare
	// below rather than testing a stale inline copy.
	file := compileSchema(t, string(src))

	schema, diags := resolver.Resolve([]*ast.File{file})
	if diags.HasErrors() {
		t.Fatalf("resolve errors: %v", diags)
	}

	out, err := New().Generate(schema)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	wantFiles := []string{
		"schema.pb.go",
		"schema_grpc.pb.go",
		"zever/annotations.pb.go",
	}

	if len(out) != len(wantFiles) {
		t.Fatalf("Generate produced %d files (%v), want %d", len(out), keysOf(out), len(wantFiles))
	}

	for _, path := range wantFiles {
		got, ok := out[path]
		if !ok {
			t.Errorf("Generate output missing %q, got %v", path, keysOf(out))
			continue
		}

		want, err := os.ReadFile(filepath.Join("testdata", "golden", filepath.FromSlash(path)))
		if err != nil {
			t.Errorf("read golden %q: %v", path, err)
			continue
		}

		if string(got) != string(want) {
			t.Errorf("golden mismatch for %q: regenerate with -update and inspect the diff", path)
		}
	}
}
