package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// These tests drive run(), which shells out to git in the process working
// directory, so they chdir into scratch repos and must not run in parallel.

// mustGitRepo initializes a scratch git repo with one commit, chdirs into it,
// and returns the repo root.
func mustGitRepo(t *testing.T) string {
	t.Helper()

	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	root := t.TempDir()
	t.Setenv("HOME", root)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("GIT_AUTHOR_NAME", "t")
	t.Setenv("GIT_AUTHOR_EMAIL", "t@t")
	t.Setenv("GIT_COMMITTER_NAME", "t")
	t.Setenv("GIT_COMMITTER_EMAIL", "t@t")

	git := func(args ...string) {
		t.Helper()
		cmd := exec.CommandContext(t.Context(), "git", args...)
		cmd.Dir = root
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}

	write := func(rel, content string) {
		t.Helper()
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	git("init", "-b", "main", ".")
	write("a/go.mod", "module github.com/zenta-dev/zever/a\n\ngo 1.27.0\n")
	write("a/a.go", "package a\n")
	write("b/go.mod", "module github.com/zenta-dev/zever/b\n\ngo 1.27.0\n")
	write("b/b.go", "package b\n")
	git("add", "-A")
	git("commit", "-m", "base")

	t.Chdir(root)

	return root
}

// commitChange writes rel and commits it, simulating a second commit.
func commitChange(t *testing.T, root, rel, content string) {
	t.Helper()

	p := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.CommandContext(t.Context(), "git", "-C", root, "commit", "-am", "change")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git commit: %v\n%s", err, out)
	}
}

// runAffected executes run() and returns exit code plus the parsed JSON output.
func runAffected(t *testing.T, args []string) (int, output) {
	t.Helper()

	out := mustTempFile(t)
	errOut := mustTempFile(t)
	code := run(args, out, errOut)

	var got output
	if err := json.Unmarshal([]byte(readSink(t, out)), &got); err != nil {
		t.Fatalf("decode output: %v", err)
	}

	return code, got
}

// TestRunModeAll pins the --mode=all contract: every module chunked, exit 0.
func TestRunModeAll(t *testing.T) {
	mustGitRepo(t)

	code, got := runAffected(t, []string{"-mode=all", "-max-groups", "1"})
	if code != 0 {
		t.Fatalf("run = %d, want 0", code)
	}
	mustEqual(t, "reason", got.Reason, "all")
	mustEqual(t, "groups", got.Groups, [][]string{{"a", "b"}})
	mustEqual(t, "count", got.Count, 2)
}

// TestRunModeAffected pins the --mode=affected contract: changed file maps to
// its owning module, exit 0.
func TestRunModeAffected(t *testing.T) {
	root := mustGitRepo(t)
	commitChange(t, root, "a/a.go", "package a\n\n// changed\n")

	code, got := runAffected(t, []string{"-mode=affected", "-base", "HEAD~1"})
	if code != 0 {
		t.Fatalf("run = %d, want 0", code)
	}
	mustEqual(t, "reason", got.Reason, "affected")
	mustEqual(t, "groups", got.Groups, [][]string{{"a"}})
	mustEqual(t, "count", got.Count, 1)
}

// TestRunFallback pins the fail-safe path: outside a git repo, run() emits the
// fallback contract with a stderr warning and still exits 0.
func TestRunFallback(t *testing.T) {
	t.Chdir(t.TempDir())

	out := mustTempFile(t)
	errOut := mustTempFile(t)
	if code := run([]string{"-mode=all"}, out, errOut); code != 0 {
		t.Fatalf("run = %d, want 0", code)
	}

	var got output
	if err := json.Unmarshal([]byte(readSink(t, out)), &got); err != nil {
		t.Fatal(err)
	}
	mustEqual(t, "reason", got.Reason, "fallback")
	mustEqual(t, "groups", got.Groups, [][]string{{}})

	if warn := readSink(t, errOut); !strings.Contains(warn, "warning") {
		t.Errorf("stderr = %q, want warning", warn)
	}
}

// TestGitTopLevel pins that gitTopLevel reports the scratch repo root.
func TestGitTopLevel(t *testing.T) {
	root := mustGitRepo(t)

	got, err := gitTopLevel()
	if err != nil {
		t.Fatal(err)
	}
	want, err := filepath.Abs(root)
	if err != nil {
		t.Fatal(err)
	}
	mustEqual(t, "gitTopLevel", got, want)
}

// TestGitMergeBaseAndChanged pins the two git helpers run() depends on.
func TestGitMergeBaseAndChanged(t *testing.T) {
	root := mustGitRepo(t)
	commitChange(t, root, "a/a.go", "package a\n\n// changed\n")

	headOut, err := exec.CommandContext(t.Context(), "git", "-C", root, "rev-parse", "HEAD~1").Output()
	if err != nil {
		t.Fatal(err)
	}
	first := string(headOut[:len(headOut)-1])

	mb, err := gitMergeBase(root, "HEAD~1")
	if err != nil {
		t.Fatal(err)
	}
	mustEqual(t, "merge-base", mb, first)

	changed, err := gitChanged(root, mb)
	if err != nil {
		t.Fatal(err)
	}
	mustEqual(t, "changed", changed, []string{"a/a.go"})
}

// TestCwdModules pins the fallback module lister: a dir holding go.mod maps
// to the root module ".".
func TestCwdModules(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module x\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(root)

	mustEqual(t, "cwdModules", cwdModules(), []string{"."})
}
