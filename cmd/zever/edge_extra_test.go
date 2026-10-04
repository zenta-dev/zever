package main

import (
	"io"
	"testing"
)

// TestRunFmtWithDirectoryPath pins the error path when a path is a directory
// rather than a file: runFmtWith must return an error, not panic.
func TestRunFmtWithDirectoryPath(t *testing.T) {
	t.Parallel()

	err := runFmtWith(FmtConfig{Files: []string{t.TempDir()}, Out: io.Discard})
	if err == nil {
		t.Fatal("runFmtWith(directory) = nil error, want error")
	}
}

// TestDamerauLevenshteinBoundaries covers the empty and single-character
// boundaries of the edit-distance kernel.
func TestDamerauLevenshteinBoundaries(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		a, b string
		want int
	}{
		{"both empty", "", "", 0},
		{"empty left", "", "abc", 3},
		{"empty right", "abc", "", 3},
		{"single equal", "a", "a", 0},
		{"single different", "a", "b", 1},
		{"one char vs long", "a", "abcdef", 5},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := damerauLevenshtein(tc.a, tc.b); got != tc.want {
				t.Fatalf("damerauLevenshtein(%q, %q) = %d, want %d", tc.a, tc.b, got, tc.want)
			}
		})
	}
}
