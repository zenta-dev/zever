package main

import "testing"

// TestScanImportsUnparsable pins the best-effort contract: malformed source
// yields an empty set rather than an error or panic.
func TestScanImportsUnparsable(t *testing.T) {
	t.Parallel()

	if got := scanImports("this is not go source"); len(got) != 0 {
		t.Fatalf("scanImports(malformed) = %v, want empty", got)
	}
}

// TestProviderNoMatch pins the no-provider boundary.
func TestProviderNoMatch(t *testing.T) {
	t.Parallel()

	mods := map[string]string{"github.com/zenta-dev/zever/core/cache": "core/cache"}
	if got := provider("github.com/other/pkg", mods); got != "" {
		t.Fatalf("provider(other) = %q, want empty", got)
	}
	if got := provider("github.com/zenta-dev/zever/core/cache", nil); got != "" {
		t.Fatalf("provider(nil mods) = %q, want empty", got)
	}
}

// TestParseRequiresAndReplacesEmpty pins the empty-input boundaries.
func TestParseRequiresAndReplacesEmpty(t *testing.T) {
	t.Parallel()

	if got := parseRequires(""); len(got) != 0 {
		t.Fatalf("parseRequires(empty) = %v, want empty", got)
	}
	if got := parseReplaces(""); len(got) != 0 {
		t.Fatalf("parseReplaces(empty) = %v, want empty", got)
	}
}

// TestParseReplacesSingleLine pins the single-line replace form.
func TestParseReplacesSingleLine(t *testing.T) {
	t.Parallel()

	const content = "module x\n\nreplace github.com/zenta-dev/zever/core/db => ../core/db\n"
	got := sortedSet(parseReplaces(content))
	mustEqual(t, "single replace", got, []string{"github.com/zenta-dev/zever/core/db"})
}
