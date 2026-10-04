package main

import (
	"io"
	"os"
	"path/filepath"
	"testing"
)

// BenchmarkDamerauLevenshtein measures the edit-distance kernel behind
// command/flag suggestions.
func BenchmarkDamerauLevenshtein(b *testing.B) {
	const a, c = "compile", "recompile"

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if d := damerauLevenshtein(a, c); d != 2 {
			b.Fatalf("distance = %d, want 2", d)
		}
	}
}

// BenchmarkClosest measures the suggestion lookup against the full
// top-level command list.
func BenchmarkClosest(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if got := closest("compil", allTopLevel); got != "compile" {
			b.Fatalf("closest = %q, want compile", got)
		}
	}
}

// BenchmarkRunFmtWith measures the check-only schema formatting path over an
// already-canonical .zen file (parse + render, no write).
func BenchmarkRunFmtWith(b *testing.B) {
	path := filepath.Join(b.TempDir(), "user.zen")
	if err := os.WriteFile(path, []byte(zeverUserSchema), 0o600); err != nil {
		b.Fatal(err)
	}
	// Canonicalize once so the measured check-only path reports no change.
	if err := runFmtWith(FmtConfig{Files: []string{path}, Write: true, Out: io.Discard}); err != nil {
		b.Fatalf("canonicalize: %v", err)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := runFmtWith(FmtConfig{Files: []string{path}, Out: io.Discard}); err != nil {
			b.Fatalf("runFmtWith: %v", err)
		}
	}
}
