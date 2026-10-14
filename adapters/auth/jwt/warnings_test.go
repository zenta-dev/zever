package jwt

import (
	"strings"
	"testing"
)

func TestWarnings_defaultMemoryStore(t *testing.T) {
	t.Parallel()

	a := freshAdapter(t, baseOpts())

	wr, ok := a.(WarningReporter)
	if !ok {
		t.Fatal("adapter does not implement WarningReporter")
	}

	w := wr.Warnings()
	if len(w) != 1 {
		t.Fatalf("Warnings() = %v, want exactly one warning", w)
	}

	for _, want := range []string{"per-process", "restart", "redis"} {
		if !strings.Contains(w[0], want) {
			t.Errorf("warning %q missing %q", w[0], want)
		}
	}

	w[0] = "mutated"
	if wr.Warnings()[0] == "mutated" {
		t.Error("Warnings() leaks internal slice")
	}
}

func TestWarnings_injectedStoreSilent(t *testing.T) {
	t.Parallel()

	opts := baseOpts()
	opts.JWT.RevocationStore = &fakeRevocationStore{}

	a := freshAdapter(t, opts)

	wr, ok := a.(WarningReporter)
	if !ok {
		t.Fatal("adapter does not implement WarningReporter")
	}

	if w := wr.Warnings(); len(w) != 0 {
		t.Fatalf("Warnings() = %v, want none with injected store", w)
	}
}
