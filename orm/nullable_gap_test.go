package orm

import "testing"

// TestNullableSomeNone pins the present/absent constructor pair and the
// zero-value contract of Nullable, the arbitrary-type maybe-wrapper used for
// outer-join row sides.
func TestNullableSomeNone(t *testing.T) {
	t.Parallel()

	some := NullableSome("v")
	if !some.IsSome() {
		t.Fatal("NullableSome(\"v\").IsSome() = false, want true")
	}

	v, ok := some.Get()
	if !ok || v != "v" {
		t.Fatalf("NullableSome Get() = (%q, %v), want (\"v\", true)", v, ok)
	}

	if got := some.GetOr("fallback"); got != "v" {
		t.Fatalf("NullableSome GetOr() = %q, want \"v\"", got)
	}

	none := NullableNone[string]()
	if none.IsSome() {
		t.Fatal("NullableNone[string]().IsSome() = true, want false")
	}

	nv, nok := none.Get()
	if nok || nv != "" {
		t.Fatalf("NullableNone Get() = (%q, %v), want (\"\", false)", nv, nok)
	}

	if got := none.GetOr("fallback"); got != "fallback" {
		t.Fatalf("NullableNone GetOr() = %q, want \"fallback\"", got)
	}

	var zero Nullable[int]
	if zero.IsSome() {
		t.Fatal("zero Nullable[int].IsSome() = true, want false")
	}

	if got := zero.GetOr(7); got != 7 {
		t.Fatalf("zero Nullable.GetOr(7) = %d, want 7", got)
	}
}

// TestNullableStructPayload proves Nullable carries an unconstrained struct
// value without any ScanValue requirement.
func TestNullableStructPayload(t *testing.T) {
	t.Parallel()

	type row struct {
		ID   string
		Name string
	}

	n := NullableSome(row{ID: "w1", Name: "widget"})
	if !n.IsSome() {
		t.Fatal("NullableSome(row).IsSome() = false, want true")
	}

	got, ok := n.Get()
	if !ok || got.ID != "w1" || got.Name != "widget" {
		t.Fatalf("Get() = (%+v, %v), want ({w1 widget}, true)", got, ok)
	}
}

// BenchmarkNullableGetOr measures the present fast path of the wrapper's
// value accessor.
func BenchmarkNullableGetOr(b *testing.B) {
	n := NullableSome(42)

	b.ReportAllocs()

	for b.Loop() {
		_ = n.GetOr(0)
	}
}
