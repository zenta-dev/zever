package render

import (
	"testing"

	"github.com/zenta-dev/zever/orm/dialect/postgres"
	"github.com/zenta-dev/zever/orm/dialect/sqlite"
)

// BenchmarkSelect measures the common single-table read shape against both
// dialects, exercising the shape cache and identifier quoting.
func BenchmarkSelect(b *testing.B) {
	d := sqlite.New()
	cols := []string{"id", "name", "quantity"}
	order := []OrderTerm{{Column: "id"}}

	b.ReportAllocs()
	for b.Loop() {
		if _, _, err := Select(d, "widgets", cols, Node{}, order, 0, 0); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkSelectPostgres is the same read shape rendered for the postgres
// dialect (dollar placeholders, different quoting).
func BenchmarkSelectPostgres(b *testing.B) {
	d := postgres.New()
	cols := []string{"id", "name", "quantity"}
	order := []OrderTerm{{Column: "id"}}

	b.ReportAllocs()
	for b.Loop() {
		if _, _, err := Select(d, "widgets", cols, Node{}, order, 0, 0); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkInsert measures a single-row INSERT with bound values.
func BenchmarkInsert(b *testing.B) {
	d := sqlite.New()
	cols := []string{"id", "name", "quantity"}
	vals := []any{"w1", "Alpha", int64(10)}

	b.ReportAllocs()
	for b.Loop() {
		if _, _, err := Insert(d, "widgets", cols, vals); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkUpdate measures an UPDATE with a SET list and an ORDER BY.
func BenchmarkUpdate(b *testing.B) {
	d := sqlite.New()
	sets := []Assignment{{Column: "name", Value: "Beta"}}
	order := []OrderTerm{{Column: "id"}}

	b.ReportAllocs()
	for b.Loop() {
		if _, _, err := Update(d, "widgets", sets, Node{}, order, 0, 0); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkDelete measures a DELETE with an empty WHERE and an ORDER BY.
func BenchmarkDelete(b *testing.B) {
	d := sqlite.New()
	order := []OrderTerm{{Column: "id"}}

	b.ReportAllocs()
	for b.Loop() {
		if _, _, err := Delete(d, "widgets", Node{}, order, 0, 0); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkGroupedSelect measures GROUP BY with two aggregates.
func BenchmarkGroupedSelect(b *testing.B) {
	d := sqlite.New()
	groups := []GroupTerm{{Kind: GroupPlain, Column: "category"}}
	aggs := []Aggregate{
		{Func: AggCount, Alias: "count"},
		{Func: AggSum, Column: "amount", Alias: "sum_amount"},
	}

	b.ReportAllocs()
	for b.Loop() {
		if _, _, err := GroupedSelect(d, "orders", groups, aggs, Node{}, HavingNode{}); err != nil {
			b.Fatal(err)
		}
	}
}
