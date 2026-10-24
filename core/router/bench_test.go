package router

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

// BenchmarkChiPattern measures chi-style route pattern parsing.
func BenchmarkChiPattern(b *testing.B) {
	pattern := "/api/v1/users/:id<[0-9]+>/items/{item:[a-z]+}"

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if _, err := ChiPattern(pattern); err != nil {
			b.Fatalf("ChiPattern(%q) error = %v", pattern, err)
		}
	}
}

func BenchmarkNormalizePattern(b *testing.B) {
	pattern := "/api/v1/users/{id:[0-9]+}/items/{item}"

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if got := NormalizePattern(pattern); got == "" {
			b.Fatal("NormalizePattern returned empty")
		}
	}
}

func BenchmarkConvertColonSegments(b *testing.B) {
	seg := "/users/:id/posts/:postID"

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if _, err := ConvertColonSegments(seg); err != nil {
			b.Fatalf("ConvertColonSegments(%q) error = %v", seg, err)
		}
	}
}

func BenchmarkValidMethod(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if !ValidMethod("get") {
			b.Fatal("ValidMethod(get) = false")
		}
	}
}

func BenchmarkParam(b *testing.B) {
	r := httptest.NewRequestWithContext(b.Context(), http.MethodGet, "/", nil)
	r = r.WithContext(WithParams(r.Context(), map[string]string{"id": "42", "slug": "a-b"}))

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if got := Param(r, "id"); got != "42" {
			b.Fatalf("Param(id) = %q, want 42", got)
		}
	}
}

// BenchmarkWithParams measures attaching route params to a request context,
// the per-request cost both HTTP drivers pay on routes with path params.
func BenchmarkWithParams(b *testing.B) {
	ctx := context.Background()
	params := map[string]string{"id": "42", "slug": "hello-world"}

	b.ReportAllocs()

	for b.Loop() {
		_ = WithParams(ctx, params)
	}
}

// BenchmarkWithOwnedParams is the no-copy variant the drivers use for the
// fresh per-request map.
func BenchmarkWithOwnedParams(b *testing.B) {
	ctx := context.Background()
	params := map[string]string{"id": "42", "slug": "hello-world"}

	b.ReportAllocs()

	for b.Loop() {
		_ = WithOwnedParams(ctx, params)
	}
}
