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

// TestHelpStartupAllocs caps the heap allocations of one in-process root
// --help run. Wall-clock startup is covered by TestHelpStartupBudget (and
// varies by machine); allocations are deterministic and catch accidental
// cold-path bloat (new per-invocation maps, template re-parses, registry
// copies). The bound is generous headroom over the measured 2026-10-14
// value (625 allocs/op), not a ratchet: tighten it only with a measured
// before/after.
func TestHelpStartupAllocs(t *testing.T) {
	allocs := testing.AllocsPerRun(20, func() {
		root := newRootCmd()
		root.SetArgs([]string{"--help"})
		root.SetOut(io.Discard)
		root.SetErr(io.Discard)
		if err := root.Execute(); err != nil {
			t.Fatalf("root --help Execute: %v", err)
		}
	})
	const maxHelpAllocs = 1500
	if allocs > maxHelpAllocs {
		t.Fatalf("--help allocs = %v, budget %d", allocs, maxHelpAllocs)
	}
}
