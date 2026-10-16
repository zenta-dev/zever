package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"sort"
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

// gitRun runs git in dir with a hermetic environment, failing the
// test on error and returning combined output.
func gitRun(t *testing.T, dir string, args ...string) string {
	t.Helper()

	cmd := exec.CommandContext(t.Context(), "git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1", "HOME="+t.TempDir(),
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
	b, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, b)
	}

	return string(b)
}

// TestTagReleasePushIsAtomic pins that --push publishes every tag in one
// `git push --atomic`: when the remote rejects one ref, none of the lockstep
// tags land (a plain `git push --tags` would leave a half-released set).
func TestTagReleasePushIsAtomic(t *testing.T) {
	for _, bin := range []string{"bash", "git"} {
		if _, err := exec.LookPath(bin); err != nil {
			t.Skipf("%s not available", bin)
		}
	}
	script, err := filepath.Abs(filepath.Join("..", "tag-release.sh"))
	if err != nil {
		t.Fatal(err)
	}

	remote := t.TempDir()
	gitRun(t, remote, "init", "-q", "--bare")
	mustWriteFile(t, filepath.Join(remote, "hooks", "pre-receive"),
		"#!/bin/sh\nwhile read old new ref; do\n  case \"$ref\" in refs/tags/libs/b/*) echo rejected >&2; exit 1 ;; esac\ndone\n")
	if err := os.Chmod(filepath.Join(remote, "hooks", "pre-receive"), 0o700); err != nil {
		t.Fatal(err)
	}

	root := t.TempDir()
	for _, m := range []string{"a", "b"} {
		mustWriteFile(t, filepath.Join(root, "libs", m, "go.mod"),
			"module github.com/zenta-dev/zever/libs/"+m+"\n\ngo 1.27.0\n")
	}
	gitRun(t, root, "init", "-q")
	gitRun(t, root, "add", "-A")
	gitRun(t, root, "commit", "-qm", "x")
	gitRun(t, root, "remote", "add", "origin", remote)

	cmd := exec.CommandContext(t.Context(), "bash", script, "--push", "v1.2.3")
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1", "HOME="+t.TempDir())
	if out, runErr := cmd.CombinedOutput(); runErr == nil {
		t.Fatalf("--push succeeded against a rejecting remote:\n%s", out)
	}
	if tags := gitRun(t, remote, "tag", "--list"); strings.TrimSpace(tags) != "" {
		t.Fatalf("atomic push left tags on the remote: %q", tags)
	}

	// Without the rejecting hook the same push lands every tag.
	if err := os.Remove(filepath.Join(remote, "hooks", "pre-receive")); err != nil {
		t.Fatal(err)
	}
	cmd = exec.CommandContext(t.Context(), "bash", script, "--push", "v1.2.3")
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1", "HOME="+t.TempDir())
	if out, runErr := cmd.CombinedOutput(); runErr != nil {
		t.Fatalf("--push: %v\n%s", runErr, out)
	}
	tags := strings.Fields(gitRun(t, remote, "tag", "--list"))
	sort.Strings(tags)
	if len(tags) != 2 || tags[0] != "libs/a/v1.2.3" || tags[1] != "libs/b/v1.2.3" {
		t.Fatalf("remote tags = %v, want libs/a and libs/b at v1.2.3", tags)
	}
}

// TestTagReleaseVerifyProxy pins --verify-proxy: it passes when the module
// proxy resolves every module at the version and fails naming the module the
// proxy cannot serve. A file:// proxy stands in for proxy.golang.org.
func TestTagReleaseVerifyProxy(t *testing.T) {
	for _, bin := range []string{"bash", "go"} {
		if _, err := exec.LookPath(bin); err != nil {
			t.Skipf("%s not available", bin)
		}
	}
	script, err := filepath.Abs(filepath.Join("..", "tag-release.sh"))
	if err != nil {
		t.Fatal(err)
	}

	root := t.TempDir()
	for _, m := range []string{"a", "b"} {
		mustWriteFile(t, filepath.Join(root, "libs", m, "go.mod"),
			"module github.com/zenta-dev/zever/libs/"+m+"\n\ngo 1.27.0\n")
	}
	proxyDir := t.TempDir()
	serve := func(m string) {
		base := filepath.Join(proxyDir, "github.com", "zenta-dev", "zever", "libs", m, "@v")
		mustWriteFile(t, filepath.Join(base, "v1.2.3.info"), `{"Version":"v1.2.3","Time":"2026-01-01T00:00:00Z"}`)
		mustWriteFile(t, filepath.Join(base, "v1.2.3.mod"), "module github.com/zenta-dev/zever/libs/"+m+"\n\ngo 1.27.0\n")
	}
	serve("a")

	run := func() (string, error) {
		cmd := exec.CommandContext(t.Context(), "bash", script, "--verify-proxy", "v1.2.3")
		cmd.Dir = root
		cmd.Env = append(os.Environ(), "HOME="+t.TempDir(), "GOPATH="+t.TempDir(), "GOPROXY=file://"+proxyDir,
			"GOSUMDB=off", "GOFLAGS=-modcacherw", "ATTEMPTS=1", "DELAY=0")
		b, runErr := cmd.CombinedOutput()
		return string(b), runErr
	}

	out, runErr := run()
	if runErr == nil || !strings.Contains(out, "proxy cannot resolve: github.com/zenta-dev/zever/libs/b@v1.2.3") {
		t.Fatalf("missing module on proxy: err=%v out=%q", runErr, out)
	}

	serve("b")
	if out, runErr := run(); runErr != nil || !strings.Contains(out, "verify-proxy ok") {
		t.Fatalf("complete proxy: err=%v out=%q", runErr, out)
	}
}
