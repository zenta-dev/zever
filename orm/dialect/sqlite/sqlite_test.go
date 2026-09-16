package sqlite

import "testing"

// TestName_returns_sqlite verifies the dialect name.
func TestName_returns_sqlite(t *testing.T) {
	t.Parallel()

	if got := New().Name(); got != "sqlite" {
		t.Fatalf("Name() = %q, want %q", got, "sqlite")
	}
}

// TestPlaceholder_always_question_mark verifies placeholder edge cases.
func TestPlaceholder_always_question_mark(t *testing.T) {
	t.Parallel()

	d := New()

	tests := []struct {
		name string
		n    int
		want string
	}{
		{name: "zero", n: 0, want: "?"},
		{name: "negative", n: -1, want: "?"},
		{name: "first", n: 1, want: "?"},
		{name: "tenth", n: 10, want: "?"},
		{name: "large", n: 1000, want: "?"},
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
		{name: "table", in: "users", want: `"users"`},
		{name: "column", in: "user_id", want: `"user_id"`},
		{name: "empty", in: "", want: `""`},
		{name: "space", in: "weird name", want: `"weird name"`},
		{name: "dotted", in: "u.id", want: `"u.id"`},
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

// TestCapabilities_default_truth_table pins the default (3.46.0)
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
		{name: "SupportsExplainAnalyze", got: d.SupportsExplainAnalyze(), want: false},
		{name: "SupportsNullsOrdering", got: d.SupportsNullsOrdering(), want: true},
		{name: "SupportsRightJoin", got: d.SupportsRightJoin(), want: true},
		{name: "SupportsFullJoin", got: d.SupportsFullJoin(), want: true},
		{name: "SupportsMerge", got: d.SupportsMerge(), want: false},
		{name: "SupportsArrayPredicates", got: d.SupportsArrayPredicates(), want: false},
		{name: "SupportsConflictTargetWhere", got: d.SupportsConflictTargetWhere(), want: true},
		{name: "SupportsConflictUpdateWhere", got: d.SupportsConflictUpdateWhere(), want: true},
		{name: "SupportsUpdateJoin", got: d.SupportsUpdateJoin(), want: true},
		{name: "SupportsDeleteJoin", got: d.SupportsDeleteJoin(), want: false},
		{name: "SupportsLeftMutateJoin", got: d.SupportsLeftMutateJoin(), want: false},
		{name: "SupportsForUpdate", got: d.SupportsForUpdate(), want: false},
		{name: "SupportsForShare", got: d.SupportsForShare(), want: false},
		{name: "SupportsNoWait", got: d.SupportsNoWait(), want: false},
		{name: "SupportsSkipLocked", got: d.SupportsSkipLocked(), want: false},
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
