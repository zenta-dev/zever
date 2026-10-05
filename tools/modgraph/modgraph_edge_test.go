package main

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// mustTempFile returns a fresh temp file usable as a run sink.
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

func TestJoinAll(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   []string
		want string
	}{
		{"empty", []string{}, ""},
		{"single", []string{"a"}, "a"},
		{"many", []string{"a", "b", "c"}, "a b c"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			mustEqual(t, "joinAll", joinAll(tc.in), tc.want)
		})
	}
}

func TestSortedKeys(t *testing.T) {
	t.Parallel()

	mustEqual(t, "empty", sortedKeys(nil), []string{})
	mustEqual(t, "sorted", sortedKeys(map[string]drift{"b": {}, "a": {}}), []string{"a", "b"})
}

func TestHasGoMod(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "m", "go.mod"), "module x\n")
	if err := os.MkdirAll(filepath.Join(root, "d", "go.mod"), 0o755); err != nil {
		t.Fatal(err)
	}

	if !hasGoMod(filepath.Join(root, "m")) {
		t.Error("hasGoMod(module dir) = false, want true")
	}
	if hasGoMod(filepath.Join(root, "nope")) {
		t.Error("hasGoMod(missing dir) = true, want false")
	}
	if hasGoMod(filepath.Join(root, "d")) {
		t.Error("hasGoMod(dir named go.mod) = true, want false")
	}
}

func TestParseRequiresForms(t *testing.T) {
	t.Parallel()

	const content = `module github.com/zenta-dev/zever/config

go 1.27.0

require (
	github.com/zenta-dev/zever/core/cache v0.5.3
	github.com/zenta-dev/zever/shared/codec v0.5.3 // indirect
	github.com/other/pkg v1.0.0
)

require github.com/zenta-dev/zever/orm v0.5.3
`
	got := sortedSet(parseRequires(content))
	want := []string{
		"github.com/zenta-dev/zever/core/cache",
		"github.com/zenta-dev/zever/orm",
		"github.com/zenta-dev/zever/shared/codec",
	}
	mustEqual(t, "parseRequires", got, want)
}

func TestParseReplacesBlock(t *testing.T) {
	t.Parallel()

	const content = `module github.com/zenta-dev/zever/config

go 1.27.0

replace (
	github.com/zenta-dev/zever/core/cache => ../core/cache
	github.com/other/pkg => ../other
)
`
	got := sortedSet(parseReplaces(content))
	mustEqual(t, "parseReplaces", got, []string{"github.com/zenta-dev/zever/core/cache"})
}

func TestGomodHelpersMissingFile(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if got := gomodRequires(root, "nope"); len(got) != 0 {
		t.Errorf("gomodRequires(missing) = %v, want empty", got)
	}
	if got := gomodReplaces(root, "nope"); len(got) != 0 {
		t.Errorf("gomodReplaces(missing) = %v, want empty", got)
	}
}

func TestFixErrorMessageAndUnwrap(t *testing.T) {
	t.Parallel()

	base := errors.New("boom")
	e := &fixError{dep: "core/cache", output: "bad output", err: base}

	msg := e.Error()
	for _, want := range []string{"modgraph: go mod edit for core/cache", "bad output"} {
		if !strings.Contains(msg, want) {
			t.Errorf("fixError.Error() = %q, want substring %q", msg, want)
		}
	}
	if !errors.Is(e, base) {
		t.Error("errors.Is(fixError, cause) = false, want true")
	}
}

// TestRunCheckDriftFixture pins the drift exit code and listing.
func TestRunCheckDriftFixture(t *testing.T) {
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "libs", "leaf", "go.mod"),
		"module github.com/zenta-dev/zever/libs/leaf\n\ngo 1.27.0\n")
	mustWriteFile(t, filepath.Join(root, "libs", "leaf", "leaf.go"), "package leaf\n")
	mustWriteFile(t, filepath.Join(root, "libs", "app", "go.mod"),
		"module github.com/zenta-dev/zever/libs/app\n\ngo 1.27.0\n")
	mustWriteFile(t, filepath.Join(root, "libs", "app", "app.go"),
		"package app\n\nimport \"github.com/zenta-dev/zever/libs/leaf\"\n\nvar _ = leaf.X\n")

	mods, err := findModules(t.Context(), root)
	if err != nil {
		t.Fatal(err)
	}

	out := mustTempFile(t)
	if code := runCheck(root, mods, out); code != 1 {
		t.Fatalf("runCheck(drift) = %d, want 1", code)
	}
	if got := readSink(t, out); !strings.Contains(got, "missing require: github.com/zenta-dev/zever/libs/leaf") {
		t.Errorf("runCheck output = %q, want missing require", got)
	}
}

// TestRunCheckCleanFixture pins the clean exit code and message.
func TestRunCheckCleanFixture(t *testing.T) {
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "libs", "leaf", "go.mod"),
		"module github.com/zenta-dev/zever/libs/leaf\n\ngo 1.27.0\n")
	mustWriteFile(t, filepath.Join(root, "libs", "leaf", "leaf.go"), "package leaf\n")
	mustWriteFile(t, filepath.Join(root, "libs", "app", "go.mod"),
		"module github.com/zenta-dev/zever/libs/app\n\ngo 1.27.0\n\n"+
			"require github.com/zenta-dev/zever/libs/leaf v0.0.0\n\n"+
			"replace github.com/zenta-dev/zever/libs/leaf => ../leaf\n")
	mustWriteFile(t, filepath.Join(root, "libs", "app", "app.go"),
		"package app\n\nimport \"github.com/zenta-dev/zever/libs/leaf\"\n\nvar _ = leaf.X\n")

	mods, err := findModules(t.Context(), root)
	if err != nil {
		t.Fatal(err)
	}

	out := mustTempFile(t)
	if code := runCheck(root, mods, out); code != 0 {
		t.Fatalf("runCheck(clean) = %d, want 0", code)
	}
	if got := readSink(t, out); !strings.Contains(got, "module-graph clean") {
		t.Errorf("runCheck output = %q, want clean message", got)
	}
}

// TestRunFlagParseError pins the flag-usage exit code.
func TestRunFlagParseError(t *testing.T) {
	t.Parallel()

	if code := run(t.Context(), []string{"-nope"}, mustTempFile(t), mustTempFile(t)); code != 2 {
		t.Errorf("run(bad flag) = %d, want 2", code)
	}
}
