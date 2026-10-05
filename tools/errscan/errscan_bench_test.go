package main

import (
	"os"
	"path/filepath"
	"testing"
)

// BenchmarkCheckFile measures the per-file hot path: read, parse, AST walk,
// and rule matching over a file that trips several rules.
func BenchmarkCheckFile(b *testing.B) {
	root := b.TempDir()
	src, err := os.ReadFile(filepath.Join("testdata", "violations.txt"))
	if err != nil {
		b.Fatal(err)
	}

	p := filepath.Join(root, "violations.go")
	if err := os.WriteFile(p, src, 0o600); err != nil { //nolint:gosec // p is under b.TempDir(), not user input
		b.Fatal(err)
	}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		vs, err := checkFile("pkg/violations.go", p)
		if err != nil || len(vs) == 0 {
			b.Fatalf("checkFile = (%v, %v)", vs, err)
		}
	}
}

// BenchmarkScan measures the directory walk plus per-file checks over a small
// tree of clean packages.
func BenchmarkScan(b *testing.B) {
	root := b.TempDir()
	for _, rel := range []string{"a/a.go", "a/b.go", "b/c.go", "b/sub/d.go"} {
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			b.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("package p\n"), 0o600); err != nil {
			b.Fatal(err)
		}
	}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if _, err := scan(root); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkBracketPrefix measures the bracket-prefix regex on a representative
// message.
func BenchmarkBracketPrefix(b *testing.B) {
	const msg = "[CODE] something failed"

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if !bracketPrefix.MatchString(msg) {
			b.Fatal("no match")
		}
	}
}
