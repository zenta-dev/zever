package postgres

import "testing"

// TestName_returns_postgres verifies the dialect name.
func TestName_returns_postgres(t *testing.T) {
	t.Parallel()

	if got := New().Name(); got != "postgres" {
		t.Fatalf("Name() = %q, want %q", got, "postgres")
	}
}

// TestPlaceholder_numbered verifies numbered placeholder edge cases.
func TestPlaceholder_numbered(t *testing.T) {
	t.Parallel()

	d := New()

	tests := []struct {
		name string
		n    int
		want string
	}{
		{name: "first", n: 1, want: "$1"},
		{name: "second", n: 2, want: "$2"},
		{name: "tenth", n: 10, want: "$10"},
		{name: "large", n: 100, want: "$100"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := d.Placeholder(tt.n); got != tt.want {
				t.Fatalf("Placeholder(%d) = %q, want %q", tt.n, got, tt.want)
			}
		})
	}
}

// TestQuoteIdent_double_quotes verifies identifier quoting edge cases.
func TestQuoteIdent_double_quotes(t *testing.T) {
	t.Parallel()

	d := New()

	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "table", in: "widgets", want: `"widgets"`},
		{name: "column", in: "id", want: `"id"`},
		{name: "empty", in: "", want: `""`},
		{name: "space", in: "weird name", want: `"weird name"`},
		{name: "embedded_quote", in: `foo"bar`, want: `"foo""bar"`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := d.QuoteIdent(tt.in); got != tt.want {
				t.Fatalf("QuoteIdent(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

// TestCapabilities_default_truth_table pins the default (16.0)
// capability answers for the non-version-gated surface.
func TestCapabilities_default_truth_table(t *testing.T) {
	t.Parallel()

	d := New()

	tests := []struct {
		name string
		got  bool
		want bool
	}{
		{name: "SupportsCTE", got: d.SupportsCTE(), want: true},
		{name: "SupportsRecursive", got: d.SupportsRecursive(), want: true},
		{name: "SupportsIntersectExcept", got: d.SupportsIntersectExcept(), want: true},
		{name: "SupportsReturning", got: d.SupportsReturning(), want: true},
		{name: "SupportsExplainAnalyze", got: d.SupportsExplainAnalyze(), want: true},
		{name: "SupportsNullsOrdering", got: d.SupportsNullsOrdering(), want: true},
		{name: "SupportsArrayPredicates", got: d.SupportsArrayPredicates(), want: true},
		{name: "SupportsRightJoin", got: d.SupportsRightJoin(), want: true},
		{name: "SupportsFullJoin", got: d.SupportsFullJoin(), want: true},
		{name: "SupportsMerge", got: d.SupportsMerge(), want: true},
		{name: "SupportsConflictTargetWhere", got: d.SupportsConflictTargetWhere(), want: true},
		{name: "SupportsConflictUpdateWhere", got: d.SupportsConflictUpdateWhere(), want: true},
		{name: "SupportsUpdateJoin", got: d.SupportsUpdateJoin(), want: true},
		{name: "SupportsDeleteJoin", got: d.SupportsDeleteJoin(), want: true},
		{name: "SupportsLeftMutateJoin", got: d.SupportsLeftMutateJoin(), want: false},
		{name: "SupportsForUpdate", got: d.SupportsForUpdate(), want: true},
		{name: "SupportsForShare", got: d.SupportsForShare(), want: true},
		{name: "SupportsNoWait", got: d.SupportsNoWait(), want: true},
		{name: "SupportsSkipLocked", got: d.SupportsSkipLocked(), want: true},
		{name: "SupportsUpdateOrderLimit", got: d.SupportsUpdateOrderLimit(), want: false},
		{name: "SupportsDeleteOrderLimit", got: d.SupportsDeleteOrderLimit(), want: false},
		{name: "SupportsMutateOffset", got: d.SupportsMutateOffset(), want: false},
		{name: "SupportsJoinedMutateOrderLimit", got: d.SupportsJoinedMutateOrderLimit(), want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if tt.got != tt.want {
				t.Fatalf("%s = %v, want %v", tt.name, tt.got, tt.want)
			}
		})
	}
}

// TestSupportsMerge_version_gate pins the Postgres 15 floor.
func TestSupportsMerge_version_gate(t *testing.T) {
	t.Parallel()

	if !New().SupportsMerge() {
		t.Fatal("New().SupportsMerge() = false, want true (default is 16.x)")
	}

	tests := []struct {
		name    string
		version string
		want    bool
	}{
		{name: "below floor", version: "14.0", want: false},
		{name: "patch below", version: "14.13", want: false},
		{name: "at floor", version: "15.0", want: true},
		{name: "above floor", version: "15.4", want: true},
		{name: "current", version: "16.2", want: true},
		{name: "future", version: "17.0", want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			d, err := NewWithVersion(tt.version)
			if err != nil {
				t.Fatalf("NewWithVersion(%q): %v", tt.version, err)
			}

			if got := d.SupportsMerge(); got != tt.want {
				t.Fatalf("SupportsMerge() on %s = %v, want %v", tt.version, got, tt.want)
			}
		})
	}
}
