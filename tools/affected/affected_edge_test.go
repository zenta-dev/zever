package main

import (
	"encoding/json"
	"io"
	"os"
	"strings"
	"testing"
)

// mustTempFile returns a fresh temp file usable as a run/emit sink.
func mustTempFile(t *testing.T) *os.File {
	t.Helper()

	f, err := os.CreateTemp(t.TempDir(), "sink-*")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.Close() })

	return f
}

// readSink rewinds f and returns everything written to it.
func readSink(t *testing.T, f *os.File) string {
	t.Helper()

	if _, err := f.Seek(0, io.SeekStart); err != nil {
		t.Fatal(err)
	}
	b, err := io.ReadAll(f)
	if err != nil {
		t.Fatal(err)
	}

	return string(b)
}

// TestRunFlagErrors pins every pre-git validation path to exit code 2.
func TestRunFlagErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		args []string
	}{
		{"empty args", nil},
		{"bad mode", []string{"-mode=bogus"}},
		{"zero groups", []string{"-mode=all", "-max-groups=0"}},
		{"negative groups", []string{"-mode=all", "-max-groups=-1"}},
		{"affected without base", []string{"-mode=affected"}},
		{"unknown flag", []string{"-nope"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := run(tc.args, mustTempFile(t), mustTempFile(t)); got != 2 {
				t.Errorf("run(%v) = %d, want 2", tc.args, got)
			}
		})
	}
}

// TestEmitNilGroups pins that a nil group slice marshals as [] not null.
func TestEmitNilGroups(t *testing.T) {
	t.Parallel()

	out := mustTempFile(t)
	if got := emit(nil, "none", out); got != 0 {
		t.Fatalf("emit = %d, want 0", got)
	}

	var got output
	if err := json.Unmarshal([]byte(readSink(t, out)), &got); err != nil {
		t.Fatal(err)
	}
	mustEqual(t, "groups", got.Groups, [][]string{})
	mustEqual(t, "reason", got.Reason, "none")
	mustEqual(t, "count", got.Count, 0)
}

// TestFallbackNilModules pins the fail-safe output shape and warning when no
// module list is available.
func TestFallbackNilModules(t *testing.T) {
	t.Parallel()

	out := mustTempFile(t)
	errOut := mustTempFile(t)

	if got := fallback(nil, "boom", out, errOut); got != 0 {
		t.Fatalf("fallback = %d, want 0", got)
	}

	var got output
	if err := json.Unmarshal([]byte(readSink(t, out)), &got); err != nil {
		t.Fatal(err)
	}
	mustEqual(t, "groups", got.Groups, [][]string{{}})
	mustEqual(t, "reason", got.Reason, "fallback")

	if warn := readSink(t, errOut); !strings.Contains(warn, "boom") {
		t.Errorf("stderr = %q, want cause", warn)
	}
}

// TestLoadRequiresMissingGoMod pins that a module dir without go.mod is a hard
// error.
func TestLoadRequiresMissingGoMod(t *testing.T) {
	t.Parallel()

	if _, err := loadRequires(t.TempDir(), []string{"missing"}); err == nil {
		t.Fatal("loadRequires = nil error, want read error")
	}
}

// TestIsGlobalPathBoundaries pins the exact/prefix matching of global paths.
func TestIsGlobalPathBoundaries(t *testing.T) {
	t.Parallel()

	global := []string{
		"go.work", "go.work.sum", "Makefile",
		".github/workflows/ci.yml", ".github/workflows",
		"tools/affected", "tools/affected/main.go",
	}
	for _, f := range global {
		if !isGlobalPath(f) {
			t.Errorf("isGlobalPath(%q) = false, want true", f)
		}
	}

	local := []string{
		"go.workx", "Makefile2", ".github/workflowsx/x.yml",
		"tools/affectedx/main.go", "tools/errscan/main.go", "core/cache/cache.go",
	}
	for _, f := range local {
		if isGlobalPath(f) {
			t.Errorf("isGlobalPath(%q) = true, want false", f)
		}
	}
}

// TestIsOutOfWorkspaceBoundaries pins the exact/prefix matching of the
// out-of-workspace proof modules.
func TestIsOutOfWorkspaceBoundaries(t *testing.T) {
	t.Parallel()

	out := []string{
		"docs/examples", "docs/examples/go.mod", "examples/external-sms",
		"examples/external-sms/sms.go",
	}
	for _, f := range out {
		if !isOutOfWorkspace(f) {
			t.Errorf("isOutOfWorkspace(%q) = false, want true", f)
		}
	}

	in := []string{
		"docs/examples2/x.go", "docs/example/x.go",
		"examples/external-smsx/x.go", "examples/bookings/x.go", "docs/guide.md",
	}
	for _, f := range in {
		if isOutOfWorkspace(f) {
			t.Errorf("isOutOfWorkspace(%q) = true, want false", f)
		}
	}
}
