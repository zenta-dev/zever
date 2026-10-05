package atlas

import (
	"testing"

	"github.com/zenta-dev/zever/dsl/ir"
)

// TestTableName pins the entity-to-table-name contract: snake_case of the
// entity name, pluralized via naming.PluralizeNaive, including the
// documented naive-plural limitation for irregular nouns.
func TestTableName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		entity string
		want   string
	}{
		{entity: "User", want: "users"},
		{entity: "OrderItem", want: "order_items"},
		{entity: "Category", want: "categories"},
		{entity: "Box", want: "boxes"},
		{entity: "Person", want: "persons"},
	}

	for _, tt := range tests {
		t.Run(tt.entity, func(t *testing.T) {
			t.Parallel()

			got := TableName(&ir.Entity{Name: tt.entity})
			if got != tt.want {
				t.Fatalf("TableName(%q) = %q, want %q", tt.entity, got, tt.want)
			}
		})
	}
}

// BenchmarkTableName measures the per-entity table-name computation the
// atlas backend runs once per entity during DDL generation.
func BenchmarkTableName(b *testing.B) {
	e := &ir.Entity{Name: "OrderItem"}

	b.ReportAllocs()

	for b.Loop() {
		_ = TableName(e)
	}
}
