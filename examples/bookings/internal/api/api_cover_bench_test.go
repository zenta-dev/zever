package api_test

import (
	"testing"

	genapp "github.com/zenta-dev/zever/examples/bookings/generated/zenorm/orm/gen/app"
	"github.com/zenta-dev/zever/orm"
)

// BenchmarkListBookingsQuery measures the guest booking listing behind
// GET /api/bookings.
func BenchmarkListBookingsQuery(b *testing.B) {
	conn, ctx := benchConn(b)

	const stamp = "2026-01-02T03:04:05Z"
	for i := 0; i < 10; i++ {
		if _, err := conn.Exec(ctx,
			`INSERT INTO bookings (id, space_id, guest_id, start_date, end_date, status, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
			"b-"+string(rune('a'+i)), "s-a", "host-1", "2026-10-01", "2026-10-03", "confirmed", stamp); err != nil {
			b.Fatalf("seed booking: %v", err)
		}
	}

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		rows, err := orm.From(genapp.Bookings).Where(genapp.BookingCols.GuestID.Eq("host-1")).All(ctx, conn)
		if err != nil {
			b.Fatalf("All: %v", err)
		}
		if len(rows) != 10 {
			b.Fatalf("rows = %d, want 10", len(rows))
		}
	}
}

// BenchmarkNearbySpaceScan measures the full space scan behind GET /api/nearby.
func BenchmarkNearbySpaceScan(b *testing.B) {
	conn, ctx := benchConn(b)

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		rows, err := orm.From(genapp.Spaces).All(ctx, conn)
		if err != nil {
			b.Fatalf("All: %v", err)
		}
		if len(rows) != 20 {
			b.Fatalf("rows = %d, want 20", len(rows))
		}
	}
}
