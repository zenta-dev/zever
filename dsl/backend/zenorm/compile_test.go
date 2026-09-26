package zenorm

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

// compileFixtureScalars covers every scalar mapping row plus nullable
// columns, so the compiled output exercises Table, Column,
// NullableColumn, Option, Row, time.Time and json.RawMessage against the
// real orm package.
const compileFixtureScalars = `entity Widget {
	id: uuid @primary
	name: string
	quantity: int32
	views: int64
	ratio: float32
	price: float64
	active: bool
	created_at: timestamp
	valid_on: date
	payload: bytes
	metadata: json
	nickname: string?
	seen_at: timestamp?
}`

// compileFixtureRelations covers the relation helpers, so the compiled
// output exercises Relation, NewRelation, Query, JoinOn, LeftJoinOn,
// Join2, LeftJoin2 and JoinType against the real orm package.
const compileFixtureRelations = `entity User {
	id: uuid @primary
	email: string @unique

	has_many orders: Order @foreign_key(user_id)
}

entity Order {
	id: uuid @primary
	user_id: uuid
	amount_cents: int64

	belongs_to user: User @foreign_key(user_id)
}`

// TestGeneratedOutputCompiles writes two generated module outputs into
// throwaway packages inside this module and runs `go build` over them.
// Golden comparison plus go/parser only prove the emission is well-formed
// text; this test proves the emitted generics (Table[T], Column[T, V],
// Relation[P, C], Query[T, *T], Join2/LeftJoin2) actually instantiate and
// type-check against the orm package.
func TestGeneratedOutputCompiles(t *testing.T) {
	root := moduleRoot(t)

	// os.Mkdir (not MkdirTemp): the package must live inside the module so
	// the generated `github.com/zenta-dev/zever/orm` import resolves
	// without go.mod tricks.
	dir := filepath.Join(root, fmt.Sprintf("tmp_zenorm_compile_%d", os.Getpid()))
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}

	t.Cleanup(func() {
		if err := os.RemoveAll(dir); err != nil {
			t.Fatalf("remove %s: %v", dir, err)
		}
	})

	fixtures := map[string]string{
		"scalars":   compileFixtureScalars,
		"relations": compileFixtureRelations,
	}

	pkgs := make([]string, 0, len(fixtures))

	for name, src := range fixtures {
		out, err := New().Generate(compileSchema(t, src))
		if err != nil {
			t.Fatalf("Generate (%s): %v", name, err)
		}

		for rel, content := range out {
			validateGoSyntax(t, rel, content)

			dst := filepath.Join(dir, name, rel)
			if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
				t.Fatalf("mkdir %s: %v", filepath.Dir(dst), err)
			}

			if err := os.WriteFile(dst, content, 0o600); err != nil {
				t.Fatalf("write %s: %v", dst, err)
			}
		}

		pkgs = append(pkgs, "./"+filepath.Join(filepath.Base(dir), name, "..."))
	}

	args := append([]string{"build"}, pkgs...)

	cmd := exec.CommandContext(t.Context(), "go", args...)
	cmd.Dir = root

	if combined, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go build %v: %v\n%s", pkgs, err, combined)
	}
}

// moduleRoot returns the enclosing module root by walking up from this
// test file to the directory holding go.mod.
func moduleRoot(t *testing.T) string {
	t.Helper()

	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller: not ok")
	}

	dir := filepath.Dir(file)

	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("go.mod not found walking up from %s", file)
		}

		dir = parent
	}
}
