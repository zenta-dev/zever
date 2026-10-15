package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestTagReleaseVerify pins tools/tag-release.sh --verify: it fails naming the
// untagged module, passes once the tag exists, and never creates tags.
func TestTagReleaseVerify(t *testing.T) {
	for _, bin := range []string{"bash", "git"} {
		if _, err := exec.LookPath(bin); err != nil {
			t.Skipf("%s not available", bin)
		}
	}
	script, err := filepath.Abs(filepath.Join("..", "tag-release.sh"))
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "libs", "ok", "go.mod"),
		"module github.com/zenta-dev/zever/libs/ok\n\ngo 1.27.0\n")

	run := func(name string, args ...string) (string, error) {
		cmd := exec.CommandContext(t.Context(), name, args...)
		cmd.Dir = root
		cmd.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1", "HOME="+t.TempDir())
		b, runErr := cmd.CombinedOutput()
		return string(b), runErr
	}
	if o, e := run("git", "init", "-q"); e != nil {
		t.Fatalf("git init: %v\n%s", e, o)
	}

	out, err := run("bash", script, "--verify", "v1.2.3")
	if err == nil || !strings.Contains(out, "missing tag: libs/ok/v1.2.3") {
		t.Fatalf("verify without tag: err=%v out=%q, want failure naming libs/ok/v1.2.3", err, out)
	}
	if tags, _ := run("git", "tag", "--list"); strings.TrimSpace(tags) != "" {
		t.Fatalf("--verify created tags: %q", tags)
	}

	if out, err := run("git", "-c", "user.name=t", "-c", "user.email=t@t", "commit", "--allow-empty", "-qm", "x"); err != nil {
		t.Fatalf("commit: %v\n%s", err, out)
	}
	if out, err := run("git", "tag", "libs/ok/v1.2.3"); err != nil {
		t.Fatalf("tag: %v\n%s", err, out)
	}
	if out, err := run("bash", script, "--verify", "v1.2.3"); err != nil || !strings.Contains(out, "verify ok") {
		t.Fatalf("verify with tag: err=%v out=%q", err, out)
	}
}

// TestTagReleaseRefusesStalePins pins the release-time pin gate: --verify
// and --dry-run both fail while a listed file hard-codes a version other than
// cmd/zever/VERSION, naming the file and the fix.
func TestTagReleaseRefusesStalePins(t *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash not available")
	}
	script, err := filepath.Abs(filepath.Join("..", "tag-release.sh"))
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "libs", "ok", "go.mod"),
		"module github.com/zenta-dev/zever/libs/ok\n\ngo 1.27.0\n")
	mustWriteFile(t, filepath.Join(root, "cmd", "zever", "VERSION"), "1.2.3\n")
	mustWriteFile(t, filepath.Join(root, "tools", "release-pins.txt"), "INSTALL\tdefault=v\n")
	mustWriteFile(t, filepath.Join(root, "INSTALL"), "default=v1.2.2\n")

	run := func(args ...string) (string, error) {
		cmd := exec.CommandContext(t.Context(), "bash", append([]string{script}, args...)...) //nolint:gosec // test runs the repo script with fixed args
		cmd.Dir = root
		cmd.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1", "HOME="+t.TempDir())
		b, runErr := cmd.CombinedOutput()
		return string(b), runErr
	}
	for _, mode := range []string{"--verify", "--dry-run"} {
		out, runErr := run(mode, "v1.2.3")
		if runErr == nil || !strings.Contains(out, "INSTALL pins 1.2.2, want 1.2.3") {
			t.Fatalf("%s with stale pin: err=%v out=%q, want failure naming INSTALL", mode, runErr, out)
		}
	}

	mustWriteFile(t, filepath.Join(root, "INSTALL"), "default=v1.2.3\n")
	if out, runErr := run("--dry-run", "v1.2.3"); runErr != nil || !strings.Contains(out, "would tag: libs/ok/v1.2.3") {
		t.Fatalf("dry-run with consistent pins: err=%v out=%q", runErr, out)
	}
}
