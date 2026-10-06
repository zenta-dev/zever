package main

import "testing"

// BenchmarkUseCobra measures the Cobra-vs-legacy routing probe on the common
// ported-subcommand shape.
func BenchmarkUseCobra(b *testing.B) {
	args := []string{"generate", "entity"}

	b.ReportAllocs()
	for b.Loop() {
		if !useCobra(args) {
			b.Fatal("useCobra = false, want true")
		}
	}
}

// BenchmarkGlobalFlagToken measures the global-flag classifier over the mix
// of known flags and a positional seen by the router.
func BenchmarkGlobalFlagToken(b *testing.B) {
	tokens := []string{"-i", "--quiet", "generate", "--bogus"}

	b.ReportAllocs()
	for b.Loop() {
		for _, tok := range tokens {
			_ = globalFlagToken(tok)
		}
	}
}
