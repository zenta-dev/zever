package main

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// writeDriftRepo builds the two-module scratch repo (app imports leaf, app's
// go.mod lacks the require) and returns its root.
func writeDriftRepo(t *testing.T) string {
	t.Helper()

	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "libs", "leaf", "go.mod"),
		"module github.com/zenta-dev/zever/libs/leaf\n\ngo 1.27.0\n")
	mustWriteFile(t, filepath.Join(root, "libs", "leaf", "leaf.go"), "package leaf\n")
	mustWriteFile(t, filepath.Join(root, "libs", "app", "go.mod"),
		"module github.com/zenta-dev/zever/libs/app\n\ngo 1.27.0\n")
	mustWriteFile(t, filepath.Join(root, "libs", "app", "app.go"),
		"package app\n\nimport \"github.com/zenta-dev/zever/libs/leaf\"\n\nvar _ = leaf.X\n")

	return root
}

// runModgraph executes run() with args and returns exit code plus captured
// stdout and stderr.
func runModgraph(t *testing.T, args []string) (int, string, string) {
	t.Helper()

	out := mustTempFile(t)
	errOut := mustTempFile(t)
	code := run(t.Context(), args, out, errOut)

	return code, readSink(t, out), readSink(t, errOut)
}

// TestRunCheckDrift pins the --check contract on a drifted repo: exit 1 and a
// missing-require listing.
func TestRunCheckDrift(t *testing.T) {
	root := writeDriftRepo(t)

	code, stdout, stderr := runModgraph(t, []string{"-check", "-root", root})
	if code != 1 {
		t.Fatalf("run = %d, want 1 (stdout %q, stderr %q)", code, stdout, stderr)
	}
	for _, want := range []string{"libs/app", "missing require: github.com/zenta-dev/zever/libs/leaf"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("stdout lacks %q:\n%s", want, stdout)
		}
	}
}

// TestRunCheckClean pins the --check contract on a satisfied repo: exit 0 and
// the clean message.
func TestRunCheckClean(t *testing.T) {
	root := writeDriftRepo(t)
	mustWriteFile(t, filepath.Join(root, "libs", "app", "go.mod"),
		"module github.com/zenta-dev/zever/libs/app\n\ngo 1.27.0\n\n"+
			"require github.com/zenta-dev/zever/libs/leaf v0.0.0\n\n"+
			"replace github.com/zenta-dev/zever/libs/leaf => ../leaf\n")

	code, stdout, stderr := runModgraph(t, []string{"-check", "-root", root})
	if code != 0 {
		t.Fatalf("run = %d, want 0 (stdout %q, stderr %q)", code, stdout, stderr)
	}
	if !strings.Contains(stdout, "module-graph clean") {
		t.Errorf("stdout = %q, want clean message", stdout)
	}
}

// TestRunFix pins the default fix path: run() adds require+replace via
// `go mod edit` and exits 0.
func TestRunFix(t *testing.T) {
	root := writeDriftRepo(t)

	code, stdout, stderr := runModgraph(t, []string{"-root", root, "-version=v1.2.3"})
	if code != 0 {
		t.Fatalf("run = %d, want 0 (stdout %q, stderr %q)", code, stdout, stderr)
	}
	if !strings.Contains(stdout, "libs/app: +1") {
		t.Errorf("stdout = %q, want fix summary", stdout)
	}

	content, err := os.ReadFile(filepath.Join(root, "libs", "app", "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"require github.com/zenta-dev/zever/libs/leaf v1.2.3",
		"replace github.com/zenta-dev/zever/libs/leaf => ../leaf",
	} {
		if !strings.Contains(string(content), want) {
			t.Errorf("go.mod lacks %q:\n%s", want, content)
		}
	}
}

// TestRunBadRoot pins that an unreadable root fails closed with exit 1.
func TestRunBadRoot(t *testing.T) {
	code, stdout, stderr := runModgraph(t, []string{"-check", "-root", filepath.Join(t.TempDir(), "does-not-exist")})
	if code != 1 {
		t.Fatalf("run = %d, want 1 (stdout %q, stderr %q)", code, stdout, stderr)
	}
	if !strings.Contains(stderr, "list modules") {
		t.Errorf("stderr = %q, want list modules prefix", stderr)
	}
}

// TestDefaultRoot pins that defaultRoot reports an existing absolute dir.
func TestDefaultRoot(t *testing.T) {
	got := defaultRoot()
	if !filepath.IsAbs(got) {
		t.Errorf("defaultRoot = %q, want absolute", got)
	}
	if fi, err := os.Stat(got); err != nil || !fi.IsDir() {
		t.Errorf("defaultRoot = %q, want existing dir (err %v)", got, err)
	}
}

// TestFindModulesBadGoMod pins that an unparsable go.mod surfaces the
// ErrGoModEdit sentinel.
func TestFindModulesBadGoMod(t *testing.T) {
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "m", "go.mod"), "garbage {{{\n")

	_, err := findModules(t.Context(), root)
	if err == nil {
		t.Fatal("findModules = nil error, want go mod edit failure")
	}
	if !errors.Is(err, ErrGoModEdit) {
		t.Errorf("findModules error = %v, want ErrGoModEdit", err)
	}
}

// TestMainCLI pins the end-to-end binary contract: main() dispatches --check
// and exits 1 on drift.
func TestMainCLI(t *testing.T) {
	root := writeDriftRepo(t)

	bin := filepath.Join(t.TempDir(), "modgraph")
	build := exec.CommandContext(t.Context(), "go", "build", "-o", bin, ".")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}

	var stdout, stderr strings.Builder
	cmd := exec.CommandContext(t.Context(), bin, "-check", "-root", root)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()

	exitErr := &exec.ExitError{}
	if err == nil {
		t.Fatal("exit = 0, want 1 on drift")
	}
	if !errors.As(err, &exitErr) || exitErr.ExitCode() != 1 {
		t.Fatalf("exit = %v, want 1", err)
	}
	if !strings.Contains(stdout.String(), "missing require:") {
		t.Errorf("stdout = %q, want drift listing", stdout.String())
	}
}
