package orm

import "testing"

// BenchmarkNewCTEName measures CTE-identifier validation on the valid path,
// which every WITH clause pays before rendering.
func BenchmarkNewCTEName(b *testing.B) {
	b.ReportAllocs()

	for b.Loop() {
		if _, err := NewCTEName("user_orders_2026"); err != nil {
			b.Fatalf("NewCTEName: %v", err)
		}
	}
}
