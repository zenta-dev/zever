package render

import (
	"fmt"
	"testing"
)

// BenchmarkFlattenGroupTermsFew measures the common case: a handful of plain
// group leaves, where the lazy dedup uses an in-place linear scan instead of
// allocating a map.
func BenchmarkFlattenGroupTermsFew(b *testing.B) {
	groups := []GroupTerm{
		{Kind: GroupPlain, Column: "a"},
		{Kind: GroupPlain, Column: "b"},
		{Kind: GroupPlain, Column: "c"},
		{Kind: GroupPlain, Column: "d"},
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = flattenGroupTerms(groups)
	}
}

// BenchmarkFlattenGroupTermsRollup exercises the recursive walk: a ROLLUP
// over several plain leaves, still few enough for the linear-scan dedup.
func BenchmarkFlattenGroupTermsRollup(b *testing.B) {
	groups := []GroupTerm{
		{Kind: GroupRollup, Terms: []GroupTerm{
			{Kind: GroupPlain, Column: "a"},
			{Kind: GroupPlain, Column: "b"},
			{Kind: GroupPlain, Column: "c"},
		}},
		{Kind: GroupPlain, Column: "d"},
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = flattenGroupTerms(groups)
	}
}

// BenchmarkFlattenGroupTermsMany pushes past the linear-scan threshold so the
// map fallback path is measured.
func BenchmarkFlattenGroupTermsMany(b *testing.B) {
	terms := make([]GroupTerm, 0, 64)
	for i := 0; i < 64; i++ {
		terms = append(terms, GroupTerm{Kind: GroupPlain, Column: fmt.Sprintf("col_%d", i)})
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = flattenGroupTerms(terms)
	}
}
