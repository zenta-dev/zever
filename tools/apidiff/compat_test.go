package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCompatMatrixCoversCoreTier(t *testing.T) {
	t.Parallel()

	out := compatMatrix()
	for _, dir := range coreModules {
		if !strings.Contains(out, "`"+dir+"`") {
			t.Errorf("matrix does not mention Core module %s", dir)
		}
		if !strings.Contains(out, zeverModulePrefix+dir) {
			t.Errorf("matrix does not carry the import path of %s", dir)
		}
	}
	if got := compatMatrix(); got != out {
		t.Error("compatMatrix is not deterministic")
	}
}

func TestCompatMatrixCheckRoundTrip(t *testing.T) {
	f := filepath.Join(t.TempDir(), "matrix.mdx")

	var stdout, stderr strings.Builder
	if code := run(context.Background(), []string{"--compat-matrix", f}, &stdout, &stderr); code != 0 {
		t.Fatalf("--compat-matrix = exit %d, want 0\n%s\n%s", code, stdout.String(), stderr.String())
	}
	stdout.Reset()
	if code := run(context.Background(), []string{"--check-compat-matrix", f}, &stdout, &stderr); code != 0 {
		t.Errorf("--check-compat-matrix on generated file = exit %d, want 0\n%s\n%s", code, stdout.String(), stderr.String())
	}

	raw, err := os.ReadFile(f)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(f, append(raw, '\n'), 0o600); err != nil { //nolint:gosec // f is under t.TempDir(), not user input
		t.Fatal(err)
	}
	stdout.Reset()
	if code := run(context.Background(), []string{"--check-compat-matrix", f}, &stdout, &stderr); code != 1 {
		t.Errorf("--check-compat-matrix on drifted file = exit %d, want 1", code)
	}
}
