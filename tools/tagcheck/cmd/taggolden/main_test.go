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

	data, err := generate(context.Background(), &repo{dir: root}, entries, "v0.7.2", nil)
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

// TestGeneratePinsStructThatGainsItsTagsLater covers the case the v0.8.0
// env-only migration created: a struct that existed at the baseline release
// with untagged fields and only became a wire type later. The commit that
// gave it its json tags never touches the struct's "type X" line, so the
// generator has to look at the field lines too, and it pins the struct at
// the commit that established its wire contract.
func TestGeneratePinsStructThatGainsItsTagsLater(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	gitInit(t, root)

	writeSource(t, root, "pkg/wire.go",
		"package pkg\n\ntype wireStruct struct {\n\tName string\n}\n",
		"add untagged wire struct")

	runGit(t, root, "tag", "v0.7.2")

	writeSource(t, root, "pkg/wire.go",
		"package pkg\n\ntype wireStruct struct {\n\tName string `json:\"name\"`\n}\n",
		"give the wire struct its json tag")

	tagged := strings.TrimSpace(runGit(t, root, "rev-parse", "--short=12", "HEAD"))

	registry := "pkg\twireStruct\tpkg/wire.go\n"
	writeFile(t, filepath.Join(root, "tools", "tagcheck", "golden_wire_structs.txt"), registry)

	entries, err := loadRegistry(filepath.Join(root, "tools", "tagcheck", "golden_wire_structs.txt"))
	if err != nil {
		t.Fatal(err)
	}

	data, err := generate(context.Background(), &repo{dir: root}, entries, "v0.7.2", nil)
	if err != nil {
		t.Fatal(err)
	}

	want := "pkg\twireStruct\tName\tname\t" + tagged + "\n"
	if !strings.Contains(string(data), want) {
		t.Errorf("generated data does not contain %q:\n%s", want, data)
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

	if _, err := generate(context.Background(), &repo{dir: root}, entries, "v0.7.2", nil); err == nil {
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
	writeFile(t, filepath.Join(root, "tools", "tagcheck", "golden_wire_removed.txt"),
		"# no intentional removals\n")

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

// TestGenerateOmitsRemovedKeys covers the intentional-removal escape hatch:
// a key listed in golden_wire_removed.txt is omitted from the generated
// data instead of pinned, while the remaining fields keep their base
// revision.
func TestGenerateOmitsRemovedKeys(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	gitInit(t, root)

	writeSource(t, root, "pkg/wire.go",
		"package pkg\n\ntype wireStruct struct {\n\tName string `json:\"name\"`\n\tGone string `json:\"gone\"`\n}\n",
		"add wire struct")

	runGit(t, root, "tag", "v0.7.2")

	writeFile(t, filepath.Join(root, "tools", "tagcheck", "golden_wire_structs.txt"),
		"pkg\twireStruct\tpkg/wire.go\n")

	entries, err := loadRegistry(filepath.Join(root, "tools", "tagcheck", "golden_wire_structs.txt"))
	if err != nil {
		t.Fatal(err)
	}

	removed := map[string]bool{"pkg\twireStruct\tGone": true}

	data, err := generate(context.Background(), &repo{dir: root}, entries, "v0.7.2", removed)
	if err != nil {
		t.Fatal(err)
	}

	got := string(data)

	if strings.Contains(got, "\tGone\t") {
		t.Errorf("generated data still pins the removed key:\n%s", got)
	}

	if want := "pkg\twireStruct\tName\tname\tv0.7.2\n"; !strings.Contains(got, want) {
		t.Errorf("generated data does not contain %q:\n%s", want, got)
	}
}

// TestGenerateRejectsRemovedKeyForUnregisteredStruct covers the rot check:
// a removed-key entry that names a struct which is not registered has
// nothing to bless.
func TestGenerateRejectsRemovedKeyForUnregisteredStruct(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	gitInit(t, root)

	writeSource(t, root, "pkg/wire.go",
		"package pkg\n\ntype wireStruct struct {\n\tName string `json:\"name\"`\n}\n",
		"add wire struct")

	runGit(t, root, "tag", "v0.7.2")

	writeFile(t, filepath.Join(root, "tools", "tagcheck", "golden_wire_structs.txt"),
		"pkg\twireStruct\tpkg/wire.go\n")

	entries, err := loadRegistry(filepath.Join(root, "tools", "tagcheck", "golden_wire_structs.txt"))
	if err != nil {
		t.Fatal(err)
	}

	removed := map[string]bool{"pkg\tgoneStruct\tGone": true}

	_, err = generate(context.Background(), &repo{dir: root}, entries, "v0.7.2", removed)
	if err == nil {
		t.Fatal("generate accepted a removed key for an unregistered struct")
	}

	if !strings.Contains(err.Error(), "golden_wire_structs.txt") {
		t.Errorf("error %q does not name the registry", err)
	}
}

// TestGenerateRejectsRemovedKeyForUnknownField covers the rot check: a
// removed-key entry that matches no field of the struct at its base
// revision has nothing to bless.
func TestGenerateRejectsRemovedKeyForUnknownField(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	gitInit(t, root)

	writeSource(t, root, "pkg/wire.go",
		"package pkg\n\ntype wireStruct struct {\n\tName string `json:\"name\"`\n}\n",
		"add wire struct")

	runGit(t, root, "tag", "v0.7.2")

	writeFile(t, filepath.Join(root, "tools", "tagcheck", "golden_wire_structs.txt"),
		"pkg\twireStruct\tpkg/wire.go\n")

	entries, err := loadRegistry(filepath.Join(root, "tools", "tagcheck", "golden_wire_structs.txt"))
	if err != nil {
		t.Fatal(err)
	}

	removed := map[string]bool{"pkg\twireStruct\tTypo": true}

	_, err = generate(context.Background(), &repo{dir: root}, entries, "v0.7.2", removed)
	if err == nil {
		t.Fatal("generate accepted a removed key that matches no history field")
	}

	if !strings.Contains(err.Error(), "nothing to bless") {
		t.Errorf("error %q does not say there is nothing to bless", err)
	}
}
