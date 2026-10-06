package main

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"
)

// mustWriteBench creates rel under root with content for benchmarks.
func mustWriteBench(b *testing.B, root, rel, content string) {
	b.Helper()

	p := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		b.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		b.Fatal(err)
	}
}

// BenchmarkRun measures the full CLI hot path (walk, check, sort, print)
// over a small tree containing one violation.
func BenchmarkRun(b *testing.B) {
	root := b.TempDir()
	src := "package p\n\nimport \"errors\"\n\nvar _ = errors.New(\"x\")\n"
	mustWriteBench(b, root, "pkg/v.go", src)
	mustWriteBench(b, root, "pkg/ok.go", "package p\n")

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if code := run([]string{root}, io.Discard, io.Discard); code != 1 {
			b.Fatalf("run = %d, want 1", code)
		}
	}
}

// BenchmarkRunClean measures the same path when the tree is violation-free.
func BenchmarkRunClean(b *testing.B) {
	root := b.TempDir()
	mustWriteBench(b, root, "pkg/ok.go", "package p\n")

	var stdout, stderr bytes.Buffer

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		stdout.Reset()
		stderr.Reset()
		if code := run([]string{root}, &stdout, &stderr); code != 0 {
			b.Fatalf("run = %d, want 0", code)
		}
	}
}
