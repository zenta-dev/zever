package ormdrill

import (
	"testing"

	"github.com/zenta-dev/zever/orm"
)

// BenchmarkWidgetsAll measures a full typed table scan ordered by id.
func BenchmarkWidgetsAll(b *testing.B) {
	ctx := b.Context()
	conn, err := OpenShop(ctx)
	if err != nil {
		b.Fatalf("OpenShop: %v", err)
	}
	b.Cleanup(func() { _ = conn.Close(b.Context()) })

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rows, err := orm.From(Widgets).OrderBy(WidgetCols.ID.Asc()).All(ctx, conn)
		if err != nil {
			b.Fatalf("All: %v", err)
		}
		if len(rows) != 8 {
			b.Fatalf("rows = %d, want 8", len(rows))
		}
	}
}

// BenchmarkPreload measures the N+1-free two-query preload of widgets with
// their orders.
func BenchmarkPreload(b *testing.B) {
	ctx := b.Context()
	conn, err := OpenShop(ctx)
	if err != nil {
		b.Fatalf("OpenShop: %v", err)
	}
	b.Cleanup(func() { _ = conn.Close(b.Context()) })

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		out, err := orm.Preload(ctx, conn,
			orm.From(Widgets).OrderBy(WidgetCols.ID.Asc()),
			OrderCols.WidgetID,
			orm.From(Orders).OrderBy(OrderCols.CreatedAt.Asc()),
			func(w *Widget) string { return w.ID },
			func(o *Order) string { return o.WidgetID },
		)
		if err != nil {
			b.Fatalf("Preload: %v", err)
		}
		if len(out) != 8 {
			b.Fatalf("parents = %d, want 8", len(out))
		}
	}
}
