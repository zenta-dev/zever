package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// writeFile writes content to path, failing the test on any error.
func writeFile(t *testing.T, path, content string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

// gitInit creates a git repository in dir.
func gitInit(t *testing.T, dir string) {
	t.Helper()

	runGit(t, dir, "init", "-b", "main")
	runGit(t, dir, "config", "user.email", "tagcheck@example.invalid")
	runGit(t, dir, "config", "user.name", "tagcheck")
}

// runGit runs a git command in dir and returns its output.
func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()

	cmd := exec.CommandContext(context.Background(), "git", args...)
	cmd.Dir = dir

	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v: %s", strings.Join(args, " "), err, out)
	}

	return string(out)
}

// writeSource writes a Go file and commits it.
func writeSource(t *testing.T, dir, rel, source, message string) {
	t.Helper()

	path := filepath.Join(dir, rel)

	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(path, []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}

	runGit(t, dir, "add", rel)
	runGit(t, dir, "commit", "-m", message)
}

func TestGeneratePinsKeysFromHistory(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	gitInit(t, root)

	writeSource(t, root, "pkg/wire.go",
		"package pkg\n\ntype wireStruct struct {\n\tName string `json:\"name\"`\n}\n",
		"add wire struct")

	runGit(t, root, "tag", "v0.7.2")

	writeSource(t, root, "pkg/new.go",
		"package pkg\n\ntype newStruct struct {\n\tValue string `json:\"value\"`\n}\n",
		"add new struct")

	introduced := strings.TrimSpace(runGit(t, root, "rev-parse", "--short=12", "HEAD"))

	registry := "pkg\twireStruct\tpkg/wire.go\npkg\tnewStruct\tpkg/new.go\n"
	writeFile(t, filepath.Join(root, "tools", "tagcheck", "golden_wire_structs.txt"), registry)

	entries, err := loadRegistry(filepath.Join(root, "tools", "tagcheck", "golden_wire_structs.txt"))
	if err != nil {
		t.Fatal(err)
	}

	data, err := generate(context.Background(), &repo{dir: root}, entries, "v0.7.2")
	if err != nil {
		t.Fatal(err)
	}

	got := string(data)

	for _, want := range []string{
		"pkg\twireStruct\tName\tname\tv0.7.2\n",
		"pkg\tnewStruct\tValue\tvalue\t" + introduced + "\n",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("generated data does not contain %q:\n%s", want, got)
		}
	}
}

func TestGenerateFailsWithoutJsonTags(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	gitInit(t, root)

	writeSource(t, root, "pkg/wire.go",
		"package pkg\n\ntype wireStruct struct {\n\tName string\n}\n",
		"add untagged wire struct")

	writeFile(t, filepath.Join(root, "tools", "tagcheck", "golden_wire_structs.txt"),
		"pkg\twireStruct\tpkg/wire.go\n")

	entries, err := loadRegistry(filepath.Join(root, "tools", "tagcheck", "golden_wire_structs.txt"))
	if err != nil {
		t.Fatal(err)
	}

	if _, err := generate(context.Background(), &repo{dir: root}, entries, "v0.7.2"); err == nil {
		t.Fatal("generate accepted a struct without json tags")
	}
}

func TestRunCheckDetectsDrift(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	gitInit(t, root)

	writeSource(t, root, "pkg/wire.go",
		"package pkg\n\ntype wireStruct struct {\n\tName string `json:\"name\"`\n}\n",
		"add wire struct")

	runGit(t, root, "tag", "v0.7.2")

	writeFile(t, filepath.Join(root, "tools", "tagcheck", "golden_wire_structs.txt"),
		"pkg\twireStruct\tpkg/wire.go\n")

	var stdout, stderr strings.Builder

	if code := run(context.Background(), []string{"-root", root}, &stdout, &stderr); code != 0 {
		t.Fatalf("run wrote the golden file: code %d, stderr %q", code, stderr.String())
	}

	out := filepath.Join(root, "tools", "tagcheck", "golden_wire_fields.txt")

	stdout.Reset()
	stderr.Reset()

	if code := run(context.Background(), []string{"-root", root, "-check"}, &stdout, &stderr); code != 0 {
		t.Fatalf("run -check rejected the generated file: code %d, stderr %q", code, stderr.String())
	}

	if err := os.WriteFile(out, []byte("drifted\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	stdout.Reset()
	stderr.Reset()

	if code := run(context.Background(), []string{"-root", root, "-check"}, &stdout, &stderr); code != 1 {
		t.Fatalf("run -check accepted a drifted file: code %d", code)
	}
}
