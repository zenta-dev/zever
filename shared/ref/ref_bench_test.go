package ref

import (
	"testing"
)

// BenchmarkDefault measures the Default Ref constructor.
func BenchmarkDefault(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		_ = Default[any, any]("db")
	}
}

// BenchmarkNamed measures the Named Ref constructor.
func BenchmarkNamed(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		_ = Named[any, any]("db", "primary")
	}
}

// BenchmarkWithName measures the WithName method on an existing Ref.
func BenchmarkWithName(b *testing.B) {
	r := Default[any, any]("db")

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		_ = r.WithName("replica")
	}
}

// BenchmarkString measures the String method for default instance.
func BenchmarkString(b *testing.B) {
	r := Default[any, any]("db")

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		_ = r.String()
	}
}

// BenchmarkString_Named measures the String method for named instance.
func BenchmarkString_Named(b *testing.B) {
	r := Named[any, any]("db", "primary")

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		_ = r.String()
	}
}

// BenchmarkRefAsMapKey measures using Ref as a map key.
func BenchmarkRefAsMapKey(b *testing.B) {
	refs := make([]Ref[any, any], 100)
	for i := range refs {
		refs[i] = Named[any, any]("svc", "inst"+string(rune('0'+i%10)))
	}
	m := make(map[Ref[any, any]]int, len(refs))
	i := 0

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		m[refs[i%len(refs)]] = i
		i++
	}
}

// BenchmarkRefEquality measures Ref equality comparison.
func BenchmarkRefEquality(b *testing.B) {
	r1 := Default[any, any]("db")
	r2 := Default[any, any]("db")
	r3 := Named[any, any]("db", "primary")

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		_ = r1 == r2
		_ = r1 == r3
	}
}

// BenchmarkRefWithDifferentTypes measures that Ref works with different
// type parameters (compile-time only, no runtime cost).
func BenchmarkRefWithDifferentTypes(b *testing.B) {
	type DBOptions struct{ MaxConns int }
	type CacheOptions struct{ TTL int }

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		_ = Default[*DBOptions, DBOptions]("db")
		_ = Default[*CacheOptions, CacheOptions]("cache")
		_ = Named[*DBOptions, DBOptions]("db", "replica")
		_ = Named[*CacheOptions, CacheOptions]("cache", "secondary")
	}
}