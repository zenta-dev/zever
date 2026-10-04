package cookie

import (
	"net/http/httptest"
	"testing"
)

// BenchmarkNew measures building a session cookie with secure defaults.
func BenchmarkNew(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if c := New("sess-id", Options{}); c == nil {
			b.Fatal("New() returned nil")
		}
	}
}

// BenchmarkNewHostPrefix measures building a __Host- prefixed cookie, which
// forces Secure, Path=/, and no Domain.
func BenchmarkNewHostPrefix(b *testing.B) {
	opts := Options{Prefix: PrefixHost}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if c := New("sess-id", opts); c == nil {
			b.Fatal("New() returned nil")
		}
	}
}

// BenchmarkNewMaxAge measures the branch that also computes Expires.
func BenchmarkNewMaxAge(b *testing.B) {
	opts := Options{MaxAge: 3600}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if c := New("sess-id", opts); c == nil {
			b.Fatal("New() returned nil")
		}
	}
}

// BenchmarkSet measures building and serializing a Set-Cookie header.
func BenchmarkSet(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		rec := httptest.NewRecorder()
		Set(rec, "sess-id", Options{})
	}
}
