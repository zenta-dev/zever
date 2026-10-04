package router

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

func TestChiPattern_emptyAndPlain(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		pattern string
		want    string
	}{
		{"empty", "", ""},
		{"plain", "/health", "/health"},
		{"root", "/", "/"},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := ChiPattern(tc.pattern)
			if err != nil {
				t.Fatalf("ChiPattern(%q) error = %v", tc.pattern, err)
			}
			if got != tc.want {
				t.Fatalf("ChiPattern(%q) = %q, want %q", tc.pattern, got, tc.want)
			}
		})
	}
}

func TestNormalizePattern_veryLong(t *testing.T) {
	t.Parallel()

	pattern := "/" + strings.Repeat("a", 1<<20) + "/{id}"
	got := NormalizePattern(pattern)
	if !strings.HasSuffix(got, "/:id") {
		t.Fatalf("NormalizePattern(1MB) suffix = %q, want /:id", got[len(got)-16:])
	}
	if len(got) != len(pattern)-len("{id}")+len(":id") {
		t.Fatalf("NormalizePattern(1MB) len = %d, want %d", len(got), len(pattern)-len("{id}")+len(":id"))
	}
}

func TestConvertColonSegments_repeatedColons(t *testing.T) {
	t.Parallel()

	got, err := ConvertColonSegments("::::")
	if err != nil {
		t.Fatalf("ConvertColonSegments error = %v", err)
	}
	if got != "::::" {
		t.Fatalf("ConvertColonSegments(::::) = %q, want ::::", got)
	}
}

func TestParam_wrongContextType_returnsEmpty(t *testing.T) {
	t.Parallel()

	r := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil)
	r = r.WithContext(context.WithValue(r.Context(), paramsKey{}, "not-a-map"))

	if got := Param(r, "id"); got != "" {
		t.Fatalf("Param(wrong type) = %q, want empty", got)
	}
	if got := ParamNames(r); got != nil {
		t.Fatalf("ParamNames(wrong type) = %v, want nil", got)
	}
}

func TestValidMethod_unicode(t *testing.T) {
	t.Parallel()

	if ValidMethod("GÉT") {
		t.Fatal("ValidMethod(GÉT) = true, want false")
	}
}

func TestRegister_Open_concurrent(t *testing.T) {
	t.Parallel()

	adapters := make([]Adapter, 8)
	for i := range adapters {
		adapters[i] = freshRouterAdapter()
	}

	var wg sync.WaitGroup
	wg.Add(len(adapters))

	for _, a := range adapters {
		go func(a Adapter) {
			defer wg.Done()

			if err := Register(a, func(Options) (Router, error) { return &mockRouter{}, nil }); err != nil {
				t.Errorf("Register(%v) error = %v", a, err)
				return
			}
			if _, err := Open(a, Options{}); err != nil {
				t.Errorf("Open(%v) error = %v", a, err)
			}
		}(a)
	}

	wg.Wait()
}
