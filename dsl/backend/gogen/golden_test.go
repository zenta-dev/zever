package gogen

import (
	"flag"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/zenta-dev/zever/internal/dsl/ast"
	"github.com/zenta-dev/zever/internal/dsl/resolver"
)

var updateGolden = flag.Bool("update", false, "rewrite golden files under testdata/golden")

// goldenFixture exercises every generated-code shape in one module: a
// single-ref message param with @validate rules (CreateTask), a path-param
// GET (GetTask), a paginated operation (ListTasks), an auth+roles operation,
// and a permission-check operation (DeleteTask).
const goldenFixture = `message CreateTaskInput {
	title: string @validate(min_len: 2, max_len: 128)
	email: string @validate(format: "email")
}

entity Task {
	id: uuid @primary
	user_id: uuid
	title: string
}

service TaskService {
	rpc CreateTask(req: CreateTaskInput) -> Task {
		http: POST "/tasks"
		auth: required(roles: {admin})
	}

	rpc GetTask(id: uuid) -> Task {
		http: GET "/tasks/{id}"
		auth: required
	}

	rpc ListTasks(user_id: uuid) -> Task {
		http: GET "/tasks"
		paginated: true
		auth: required
	}

	rpc DeleteTask(id: uuid) -> Task {
		http: DELETE "/tasks/{id}"
		permission: check("task.delete", resource: Task, owner_field: user_id)
	}
}`

var goldenFiles = []string{
	"app/types.go",
	"app/service.go",
	"app/router.go",
	"app/grpc.go",
	"app/register.go",
}

func generateGolden(t *testing.T) map[string][]byte {
	t.Helper()

	file := compileSchema(t, goldenFixture)

	schema, diags := resolver.Resolve([]*ast.File{file})
	if diags.HasErrors() {
		t.Fatalf("resolve errors: %v", diags)
	}

	out, err := New().Generate(schema)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	return out
}

// TestGoldenFiles compares every generated file against the committed golden
// under testdata/golden. Run with -update to rewrite goldens after an
// intentional output change, then review the diff before committing.
func TestGoldenFiles(t *testing.T) {
	out := generateGolden(t)

	if len(out) != len(goldenFiles) {
		t.Fatalf("Generate produced %d files, want %d: %v", len(out), len(goldenFiles), keysOf(out))
	}

	for _, path := range goldenFiles {
		got, ok := out[path]
		if !ok {
			t.Errorf("Generate output missing %q", path)
			continue
		}

		assertGolden(t, path, got)
	}
}

func assertGolden(t *testing.T, name string, got []byte) {
	t.Helper()

	path := filepath.Join("testdata", "golden", name+".golden")

	if *updateGolden {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("create golden dir: %v", err)
		}

		if err := os.WriteFile(path, got, 0o600); err != nil {
			t.Fatalf("write golden file %s: %v", path, err)
		}

		return
	}

	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden file %s: %v (run with -update to create it)", path, err)
	}

	if string(got) != string(want) {
		t.Errorf("golden mismatch for %s\n--- want ---\n%s\n--- got ---\n%s", name, want, got)
	}
}

// TestGoldenOutputParsesAsGo proves every generated file is syntactically
// valid Go beyond go/format (which already parses): a full go/parser pass
// over each buffer.
func TestGoldenOutputParsesAsGo(t *testing.T) {
	out := generateGolden(t)

	for _, path := range goldenFiles {
		got, ok := out[path]
		if !ok {
			t.Fatalf("Generate output missing %q", path)
		}

		if _, err := parser.ParseFile(token.NewFileSet(), path, got, parser.SkipObjectResolution); err != nil {
			t.Errorf("generated %s does not parse: %v", path, err)
		}
	}
}

// TestGoldenOutputWrittenToTempParses writes the whole generated module to a
// temp dir (mirroring the on-disk layout a caller would produce) and parses
// every file back from disk. A full go build is not possible here: the
// generated files import the protogogen message package and the zever
// runtime, neither of which exists under the temp module, so parsing plus
// the golden byte-comparison is the strongest available compile signal.
func TestGoldenOutputWrittenToTempParses(t *testing.T) {
	out := generateGolden(t)

	dir := t.TempDir()

	var paths = make([]string, 0, len(out))

	for path, content := range out {
		full := filepath.Join(dir, path)

		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatalf("create temp dir: %v", err)
		}

		if err := os.WriteFile(full, content, 0o600); err != nil {
			t.Fatalf("write temp file %s: %v", path, err)
		}

		paths = append(paths, full)
	}

	sort.Strings(paths)

	for _, full := range paths {
		src, err := os.ReadFile(full)
		if err != nil {
			t.Fatalf("read temp file %s: %v", full, err)
		}

		if _, err := parser.ParseFile(token.NewFileSet(), full, src, parser.SkipObjectResolution); err != nil {
			t.Errorf("temp file %s does not parse: %v", full, err)
		}
	}
}

// TestGoldenHeadersMarkGenerated asserts every always-regenerated file
// carries the DO NOT EDIT header while the stub entry point (tested in
// gogen_test.go) deliberately does not.
func TestGoldenHeadersMarkGenerated(t *testing.T) {
	out := generateGolden(t)

	for _, path := range goldenFiles {
		if !strings.Contains(string(out[path]), "DO NOT EDIT") {
			t.Errorf("generated %s missing DO NOT EDIT header", path)
		}
	}
}
