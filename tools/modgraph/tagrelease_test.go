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
