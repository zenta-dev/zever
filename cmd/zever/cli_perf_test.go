package main

import (
	"io"
	"testing"
	"time"
)

// TestHelpStartupBudget asserts root --help executes in-process under
// 200ms. No network, no randomness, deterministic. Uses a fresh root (not
// the shared rootCmd global) and stays sequential: Cobra command state is
// not safe for concurrent Execute.
func TestHelpStartupBudget(t *testing.T) {
	root := newRootCmd()
	root.SetArgs([]string{"--help"})
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)

	start := time.Now()
	if err := root.Execute(); err != nil {
		t.Fatalf("root --help Execute: %v", err)
	}
	if elapsed := time.Since(start); elapsed >= 200*time.Millisecond {
		t.Fatalf("--help took %v, budget 200ms", elapsed)
	}
}
