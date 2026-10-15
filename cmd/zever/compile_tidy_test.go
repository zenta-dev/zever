package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const tidyOrmOutput = "package app\n\nimport (\n\t\"fmt\"\n\n\t\"github.com/zenta-dev/zever/orm\"\n)\n\nvar _ = orm.NewTable[int]\nvar _ = fmt.Sprint\n"

func tidyOutputs() map[string]map[string][]byte {
	return map[string]map[string][]byte{"zenorm": {"orm/gen/app/app.go": []byte(tidyOrmOutput)}}
}

func writeTidyGoMod(t *testing.T, dir, body string) {
	t.Helper()

	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

// stubTidy swaps runGoModTidyFunc for a recorder and returns the call log.
func stubTidy(t *testing.T, err error) *[]string {
	t.Helper()

	var calls []string

	old := runGoModTidyFunc
	runGoModTidyFunc = func(_, dir string) error {
		calls = append(calls, dir)
		return err
	}

	t.Cleanup(func() { runGoModTidyFunc = old })

	return &calls
}

func TestEnsureGeneratedDepsRunsTidyWhenOrmMissing(t *testing.T) {
	dir := t.TempDir()
	writeTidyGoMod(t, dir, "module demo\n\ngo 1.27\n")

	calls := stubTidy(t, nil)

	if err := ensureGeneratedDeps(dir, filepath.Join(dir, "generated"), tidyOutputs(), false, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}

	if len(*calls) != 1 || (*calls)[0] != dir {
		t.Fatalf("tidy calls = %v, want one in %s", *calls, dir)
	}
}

func TestEnsureGeneratedDepsNoTidyPrintsHint(t *testing.T) {
	dir := t.TempDir()
	writeTidyGoMod(t, dir, "module demo\n\ngo 1.27\n")

	calls := stubTidy(t, nil)

	var buf bytes.Buffer
	if err := ensureGeneratedDeps(dir, filepath.Join(dir, "generated"), tidyOutputs(), true, &buf); err != nil {
		t.Fatal(err)
	}

	if len(*calls) != 0 {
		t.Fatalf("--no-tidy must not run tidy, got %v", *calls)
	}

	if !strings.Contains(buf.String(), "go mod tidy") || !strings.Contains(buf.String(), "github.com/zenta-dev/zever/orm") {
		t.Fatalf("hint = %q", buf.String())
	}
}

func TestEnsureGeneratedDepsSatisfied(t *testing.T) {
	dir := t.TempDir()
	writeTidyGoMod(t, dir, "module demo\n\ngo 1.27\n\nrequire (\n\tgithub.com/zenta-dev/zever/orm v0.1.0\n)\n")

	calls := stubTidy(t, nil)

	if err := ensureGeneratedDeps(dir, filepath.Join(dir, "generated"), tidyOutputs(), false, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}

	if len(*calls) != 0 {
		t.Fatalf("tidy ran although orm is required: %v", *calls)
	}
}

func TestEnsureGeneratedDepsSkipsOutsideModuleAndNoGoMod(t *testing.T) {
	dir := t.TempDir()
	calls := stubTidy(t, nil)

	// No go.mod at all.
	if err := ensureGeneratedDeps(dir, filepath.Join(dir, "generated"), tidyOutputs(), false, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}

	// Output dir outside the module.
	writeTidyGoMod(t, dir, "module demo\n\ngo 1.27\n")

	if err := ensureGeneratedDeps(dir, t.TempDir(), tidyOutputs(), false, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}

	if len(*calls) != 0 {
		t.Fatalf("unexpected tidy: %v", *calls)
	}
}

func TestEnsureGeneratedDepsTidyFailureIsActionableNotFatal(t *testing.T) {
	dir := t.TempDir()
	writeTidyGoMod(t, dir, "module demo\n\ngo 1.27\n")
	stubTidy(t, os.ErrPermission)

	var buf bytes.Buffer
	if err := ensureGeneratedDeps(dir, filepath.Join(dir, "generated"), tidyOutputs(), false, &buf); err != nil {
		t.Fatalf("tidy failure must not fail compile: %v", err)
	}

	if !strings.Contains(buf.String(), "run 'go mod tidy'") {
		t.Fatalf("missing actionable hint: %q", buf.String())
	}
}

func TestNewScaffoldGoModRequiresOrm(t *testing.T) {
	out, err := renderNewGoMod("zever new", NewConfig{
		ModulePath: "demo", GoVersion: "1.27", FrameworkVersion: "v0.1.0", OutDir: t.TempDir(),
	})
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(string(out), "require github.com/zenta-dev/zever/orm v0.1.0") {
		t.Fatalf("go.mod lacks orm require:\n%s", out)
	}
}
