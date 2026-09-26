package zenorm

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
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
// an isolated throwaway module that requires `zever/orm` with a replace
// to this checkout, then runs `go mod tidy` + `go build` over them.
// Golden comparison plus go/parser only prove the emission is well-formed
// text; this test proves the emitted generics (Table[T], Column[T, V],
// Relation[P, C], Query[T, *T], Join2/LeftJoin2) actually instantiate and
// type-check against the orm package without relying on the enclosing
// workspace to resolve the import.
func TestGeneratedOutputCompiles(t *testing.T) {
	root := moduleRoot(t)
	repo := filepath.Dir(root)
	ormDir := filepath.Join(repo, "orm")

	dir := t.TempDir()

	replaces := []string{
		"github.com/zenta-dev/zever/orm => " + ormDir,
		"github.com/zenta-dev/zever/core/db => " + filepath.Join(repo, "core", "db"),
		"github.com/zenta-dev/zever/dsl => " + repo + "/dsl",
		"github.com/zenta-dev/zever/shared/lrucache => " + filepath.Join(repo, "shared", "lrucache"),
		"github.com/zenta-dev/zever/shared/registry => " + filepath.Join(repo, "shared", "registry"),
		"github.com/zenta-dev/zever/shared/retry => " + filepath.Join(repo, "shared", "retry"),
		"github.com/zenta-dev/zever/adapters/db/postgres => " + filepath.Join(repo, "adapters", "db", "postgres"),
		"github.com/zenta-dev/zever/adapters/db/sqlite => " + filepath.Join(repo, "adapters", "db", "sqlite"),
	}
	goMod := "module tmpzenormcompile\n\ngo 1.27.0\n\nrequire github.com/zenta-dev/zever/orm v0.0.0\n\nreplace (\n"
	var sb strings.Builder
	sb.WriteString(goMod)
	for _, r := range replaces {
		sb.WriteString("\t" + r + "\n")
	}
	sb.WriteString(")\n")
	goMod = sb.String()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(goMod), 0o600); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}

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

		pkgs = append(pkgs, "./"+filepath.Join(name, "..."))
	}

	tidy := exec.CommandContext(t.Context(), "go", "mod", "tidy")
	tidy.Dir = dir
	tidy.Env = append(os.Environ(), "GOWORK=off", "GOPROXY=off")
	if combined, err := tidy.CombinedOutput(); err != nil {
		t.Fatalf("go mod tidy: %v\n%s", err, combined)
	}

	args := append([]string{"build"}, pkgs...)

	cmd := exec.CommandContext(t.Context(), "go", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOWORK=off", "GOPROXY=off")

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
