package main

import (
	"bytes"
	"io"
	"strings"
	"testing"
	"testing/synctest"
	"time"
)

// TestHelpStartupDoesNotBlock is the deterministic replacement for the
// old 200ms wall-clock budget. Inside a synctest bubble the clock is
// virtual: it only moves when every goroutine in the bubble is durably
// blocked. A help path that waits on a timer, a file, or the network
// advances it; a help path that renders the in-memory command tree and
// returns synchronously leaves it untouched.
//
// That is the "the help path is fast enough" property without a
// wall-clock assertion: a loaded or cold CI runner can no longer fail it,
// while a help path that starts blocking still fails on the spot.
//
// Wall-clock startup is measured, not asserted: see BenchmarkHelpStartup
// (cmd/zever/bench_test.go). The deterministic work bound for the same
// path is TestHelpStartupAllocs.
func TestHelpStartupDoesNotBlock(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		start := time.Now()

		root := newRootCmd()
		root.SetArgs([]string{"--help"})

		var out bytes.Buffer
		root.SetOut(&out)
		root.SetErr(&out)

		if err := root.Execute(); err != nil {
			t.Fatalf("root --help Execute: %v", err)
		}
		if elapsed := time.Since(start); elapsed != 0 {
			t.Fatalf("--help blocked for %v of virtual time, the help path must not wait", elapsed)
		}

		// The help still renders the tree: the budget guarded a working
		// help path, and the render contract stays pinned.
		help := out.String()
		for _, want := range []string{"Usage:", "Flags:", "new", "generate"} {
			if !strings.Contains(help, want) {
				t.Fatalf("--help output missing %q:\n%s", want, help)
			}
		}
	})
}

// TestHelpStartupAllocs caps the heap allocations of one in-process root
// --help run. Allocations are deterministic and catch accidental cold-path
// bloat (new per-invocation maps, template re-parses, registry copies),
// which is the deterministic half of what the old wall-clock budget
// guarded. The bound is generous headroom over the measured 2026-10-14
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
