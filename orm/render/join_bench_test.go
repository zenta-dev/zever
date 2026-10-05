package render

import (
	"testing"

	sqlited "github.com/zenta-dev/zever/orm/dialect/sqlite"
)

// BenchmarkSelectJoin3Mixed measures the flat mixed-chain three-table
// render: two JOIN clauses with different keywords, no WHERE, no ORDER BY.
func BenchmarkSelectJoin3Mixed(b *testing.B) {
	d := sqlited.New()

	b.ReportAllocs()

	for b.Loop() {
		q, _, err := SelectJoin3Mixed(d, InnerJoin, LeftJoin,
			"a", []string{"id"}, "b", []string{"id"}, "c", []string{"id"},
			"id", "a_id", "id", "b_id",
			Node{}, Node{}, Node{}, nil, nil, nil, 0, 0)
		if err != nil {
			b.Fatalf("SelectJoin3Mixed: %v", err)
		}

		if q == "" {
			b.Fatal("SelectJoin3Mixed returned empty query")
		}
	}
}
