package router

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

func TestParam_missing_returnsEmpty(t *testing.T) {
	t.Parallel()

	r := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil)
	if got := Param(r, "id"); got != "" {
		t.Fatalf("want empty, got %q", got)
	}
}

func TestParam_roundTrip(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		params map[string]string
		key    string
		want   string
	}{
		{name: "id", params: map[string]string{"id": "42", "slug": "a-b"}, key: "id", want: "42"},
		{name: "slug", params: map[string]string{"id": "42", "slug": "a-b"}, key: "slug", want: "a-b"},
		{name: "absent", params: map[string]string{"id": "42"}, key: "missing", want: ""},
		{name: "empty value", params: map[string]string{"id": ""}, key: "id", want: ""},
		{name: "empty name", params: map[string]string{"": "root"}, key: "", want: "root"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			r := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil)
			r = r.WithContext(WithParams(r.Context(), tt.params))

			if got := Param(r, tt.key); got != tt.want {
				t.Fatalf("want %q, got %q", tt.want, got)
			}
		})
	}
}

func TestWithParams_nilSafe(t *testing.T) {
	t.Parallel()

	r := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil)
	r = r.WithContext(WithParams(r.Context(), nil))

	if got := Param(r, "id"); got != "" {
		t.Fatalf("want empty for nil params, got %q", got)
	}

	if got := ParamNames(r); got == nil || len(got) != 0 {
		t.Fatalf("want empty map for nil params, got %v", got)
	}
}

func TestWithParams_inputIsolation(t *testing.T) {
	t.Parallel()

	r := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil)
	in := map[string]string{"id": "42"}
	r = r.WithContext(WithParams(r.Context(), in))

	in["id"] = "mutated"
	in["extra"] = "nope"

	if got := Param(r, "id"); got != "42" {
		t.Fatalf("stored params aliased caller map: got %q", got)
	}

	if got := Param(r, "extra"); got != "" {
		t.Fatalf("stored params picked up later key: got %q", got)
	}
}

func TestWithParams_emptyMap(t *testing.T) {
	t.Parallel()

	r := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil)
	r = r.WithContext(WithParams(r.Context(), map[string]string{}))

	if got := Param(r, "id"); got != "" {
		t.Fatalf("want empty, got %q", got)
	}

	if got := ParamNames(r); got == nil || len(got) != 0 {
		t.Fatalf("want empty non-nil map, got %v", got)
	}
}

func TestParamNames_copy(t *testing.T) {
	t.Parallel()

	r := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil)
	r = r.WithContext(WithParams(r.Context(), map[string]string{"id": "42"}))

	names := ParamNames(r)
	if len(names) != 1 || names["id"] != "42" {
		t.Fatalf("want {id:42}, got %v", names)
	}

	names["id"] = "mutated"
	names["extra"] = "nope"

	if got := Param(r, "id"); got != "42" {
		t.Fatalf("stored params mutated via copy: got %q", got)
	}
}

func TestParamNames_missing_returnsNil(t *testing.T) {
	t.Parallel()

	r := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil)
	if got := ParamNames(r); got != nil {
		t.Fatalf("want nil, got %v", got)
	}
}

func TestWithParams_concurrent(t *testing.T) {
	t.Parallel()

	r := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil)
	r = r.WithContext(WithParams(r.Context(), map[string]string{"id": "42", "slug": "a-b"}))

	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)

		go func() {
			defer wg.Done()

			if got := Param(r, "id"); got != "42" {
				t.Errorf("want %q, got %q", "42", got)
			}

			names := ParamNames(r)
			if len(names) != 2 || names["slug"] != "a-b" {
				t.Errorf("want 2 params, got %v", names)
			}
		}()
	}

	wg.Wait()
}

func TestNormalizePattern_table(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		pattern string
		want    string
	}{
		{name: "plain passthrough", pattern: "/users", want: "/users"},
		{name: "empty", pattern: "", want: ""},
		{name: "simple brace", pattern: "/users/{id}", want: "/users/:id"},
		{name: "regex brace", pattern: "/users/{id:[0-9]+}", want: "/users/:id<regex(^(?:[0-9]+)$)>"},
		{name: "empty name passthrough", pattern: "/x/{:[0-9]+}", want: "/x/{:[0-9]+}"},
		{name: "empty regex to colon", pattern: "/x/{id:}", want: "/x/:id"},
		{name: "nested brace quantifier", pattern: "/x/{id:[0-9]{4}}", want: "/x/:id<regex(^(?:[0-9]{4})$)>"},
		{name: "unclosed passthrough", pattern: "/x/{id", want: "/x/{id"},
		{name: "stray close passthrough", pattern: "/x/id}", want: "/x/id}"},
		{name: "no double wrap", pattern: "{id:regex(a(b)}", want: ":id<regex(^(?:a(b))$)>"},
		{name: "valid wrapper stripped", pattern: "{id:regex(^[0-9]+$)}", want: ":id<regex(^(?:^[0-9]+$)$)>"},
		{name: "wildcard untouched", pattern: "/files/*", want: "/files/*"},
		{name: "colon untouched", pattern: "/users/:id", want: "/users/:id"},
		{name: "multiple params", pattern: "/x/{a:[0-9]+}/{b}", want: "/x/:a<regex(^(?:[0-9]+)$)>/:b"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := NormalizePattern(tt.pattern); got != tt.want {
				t.Fatalf("NormalizePattern(%q) = %q, want %q", tt.pattern, got, tt.want)
			}
		})
	}
}

func TestStripRegexWrapper_table(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "no wrapper", input: "[0-9]+", want: "[0-9]+"},
		{name: "valid wrapper", input: "regex(^[0-9]+$)", want: "^[0-9]+$"},
		{name: "simple wrapper", input: "regex(abc)", want: "abc"},
		{name: "nested parens", input: "regex(a(b)c)", want: "a(b)c"},
		{name: "unclosed returns inner", input: "regex(a(b)", want: "a(b)"},
		{name: "trailing extra anchored fail", input: "regex(a)extra", want: "regex(a)extra"},
		{name: "empty inner", input: "regex()", want: ""},
		{name: "empty string", input: "", want: ""},
		{name: "prefix only", input: "regex(", want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := StripRegexWrapper(tt.input); got != tt.want {
				t.Fatalf("StripRegexWrapper(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
