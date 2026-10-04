package router

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// benchRouterAdapter registers a stub router once and returns its adapter so
// Open can be measured without paying the one-shot registration cost.
func benchRouterAdapter(b *testing.B) Adapter {
	b.Helper()

	a := freshRouterAdapter()
	if err := Register(a, func(Options) (Router, error) { return &mockRouter{}, nil }); err != nil {
		b.Fatalf("Register(%v) error = %v", a, err)
	}

	return a
}

func BenchmarkOpen(b *testing.B) {
	a := benchRouterAdapter(b)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		if _, err := Open(a, Options{AppName: "shop"}); err != nil {
			b.Fatalf("Open(%v) error = %v", a, err)
		}
	}
}

func BenchmarkChiPattern(b *testing.B) {
	pattern := "/api/v1/users/:id<[0-9]+>/items/{item:[a-z]+}"

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		if _, err := ChiPattern(pattern); err != nil {
			b.Fatalf("ChiPattern(%q) error = %v", pattern, err)
		}
	}
}

func BenchmarkNormalizePattern(b *testing.B) {
	pattern := "/api/v1/users/{id:[0-9]+}/items/{item}"

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		if got := NormalizePattern(pattern); got == "" {
			b.Fatal("NormalizePattern returned empty")
		}
	}
}

func BenchmarkConvertColonSegments(b *testing.B) {
	seg := "/users/:id/posts/:postID"

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		if _, err := ConvertColonSegments(seg); err != nil {
			b.Fatalf("ConvertColonSegments(%q) error = %v", seg, err)
		}
	}
}

func BenchmarkValidMethod(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
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

	for i := 0; i < b.N; i++ {
		if got := Param(r, "id"); got != "42" {
			b.Fatalf("Param(id) = %q, want 42", got)
		}
	}
}
