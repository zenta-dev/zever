package main

import (
	"bytes"
	"errors"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// mustBuild compiles the errscan binary into a temp dir and returns its path.
func mustBuild(t *testing.T) string {
	t.Helper()

	bin := filepath.Join(t.TempDir(), "errscan")
	cmd := exec.CommandContext(t.Context(), "go", "build", "-o", bin, ".")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}

	return bin
}

// runBin executes bin with args and returns exit code, stdout, and stderr.
func runBin(t *testing.T, bin string, args ...string) (int, string, string) {
	t.Helper()

	var stdout, stderr bytes.Buffer
	cmd := exec.CommandContext(t.Context(), bin, args...)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()

	code := 0
	if err != nil {
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) {
			t.Fatalf("run %v: %v", args, err)
		}
		code = exitErr.ExitCode()
	}

	return code, stdout.String(), stderr.String()
}

// TestMainCLI pins the end-to-end CLI contract: violation lines on stdout,
// summary on stderr, exit 1 on violations, 0 when clean, 2 on scan error.
func TestMainCLI(t *testing.T) {
	t.Parallel()

	bin := mustBuild(t)

	t.Run("violations exit 1 with formatted lines", func(t *testing.T) {
		t.Parallel()

		root := t.TempDir()
		mustWrite(t, root, "pkg/violations.go", mustFixture(t, "violations.txt"))

		code, stdout, stderr := runBin(t, bin, root)
		if code != 1 {
			t.Fatalf("exit = %d, want 1 (stdout %q, stderr %q)", code, stdout, stderr)
		}
		for _, want := range []string{
			"pkg/violations.go:9: unprefixed:",
			"pkg/violations.go:11: bracket-prefix:",
			"pkg/violations.go:12: double-%w:",
			"pkg/violations.go:13: panic:",
		} {
			if !strings.Contains(stdout, want) {
				t.Errorf("stdout lacks %q:\n%s", want, stdout)
			}
		}
		if !strings.Contains(stderr, "violation(s)") {
			t.Errorf("stderr = %q, want violation summary", stderr)
		}
	})

	t.Run("clean exit 0 with empty stdout", func(t *testing.T) {
		t.Parallel()

		root := t.TempDir()
		mustWrite(t, root, "pkg/clean.go", mustFixture(t, "clean.txt"))

		code, stdout, stderr := runBin(t, bin, root)
		if code != 0 {
			t.Fatalf("exit = %d, want 0 (stdout %q, stderr %q)", code, stdout, stderr)
		}
		if stdout != "" {
			t.Errorf("stdout = %q, want empty", stdout)
		}
		if stderr != "" {
			t.Errorf("stderr = %q, want empty", stderr)
		}
	})

	t.Run("scan error exits 2", func(t *testing.T) {
		t.Parallel()

		code, stdout, stderr := runBin(t, bin, filepath.Join(t.TempDir(), "does-not-exist"))
		if code != 2 {
			t.Fatalf("exit = %d, want 2 (stdout %q, stderr %q)", code, stdout, stderr)
		}
		if stdout != "" {
			t.Errorf("stdout = %q, want empty", stdout)
		}
		if !strings.Contains(stderr, "errscan:") {
			t.Errorf("stderr = %q, want errscan prefix", stderr)
		}
	})
}

// TestMainCLIDefaultRoot pins that the no-arg invocation scans the current
// working directory.
func TestMainCLIDefaultRoot(t *testing.T) {
	t.Parallel()

	bin := mustBuild(t)

	root := t.TempDir()
	mustWrite(t, root, "pkg/clean.go", mustFixture(t, "clean.txt"))

	cmd := exec.CommandContext(t.Context(), bin)
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("run in clean dir: %v\n%s", err, out)
	}
}
