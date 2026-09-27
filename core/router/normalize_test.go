package router

import (
	"strings"
	"sync"
	"testing"
)

func TestConvertColonSegments_table(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{name: "plain", input: "users", want: "users"},
		{name: "empty", input: "", want: ""},
		{name: "simple param", input: ":id", want: "{id}"},
		{name: "param with slash", input: "/users/:id", want: "/users/{id}"},
		{name: "colon regex", input: ":id<[0-9]+>", want: "{id:[0-9]+}"},
		{name: "bare colon", input: ":", want: ":"},
		{name: "colon slash", input: ":/", want: ":/"},
		{name: "trailing colon", input: "id:", want: "id:"},
		{name: "double colon", input: "::id", want: ":{id}"},
		{name: "underscore digit name", input: ":user_1", want: "{user_1}"},
		{name: "dash stops name", input: ":id-x", want: "{id}-x"},
		{name: "unclosed regex", input: ":id<abc", wantErr: true},
		{name: "unclosed class", input: ":id<[0-9]+", wantErr: true},
		{name: "empty regex", input: ":id<", wantErr: true},
		{name: "empty angle", input: ":id<>", wantErr: true},
		{name: "nested brace regex", input: ":id<[0-9]{4}>", want: "{id:[0-9]{4}}"},
		{name: "wildcard untouched", input: "*", want: "*"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := ConvertColonSegments(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("ConvertColonSegments(%q) want error, got nil", tt.input)
				}

				if got != "" {
					t.Fatalf("on error want zero value, got %q", got)
				}

				if !strings.HasPrefix(err.Error(), "router:") {
					t.Fatalf("error must carry router: prefix, got %q", err.Error())
				}

				return
			}

			if err != nil {
				t.Fatalf("ConvertColonSegments(%q) unexpected error: %v", tt.input, err)
			}

			if got != tt.want {
				t.Fatalf("ConvertColonSegments(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestConvertColonRegex_table(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		pattern string
		want    string
		wantErr bool
	}{
		{name: "brace untouched", pattern: "/items/{id}", want: "/items/{id}"},
		{name: "brace regex untouched", pattern: "/items/{id:[0-9]+}", want: "/items/{id:[0-9]+}"},
		{name: "colon simple", pattern: "/items/:id", want: "/items/{id}"},
		{name: "colon regex", pattern: "/items/:id<[0-9]+>", want: "/items/{id:[0-9]+}"},
		{name: "mixed", pattern: "/x/{a:[0-9]+}/:b", want: "/x/{a:[0-9]+}/{b}"},
		{name: "nested quantifier safe", pattern: "/x/{id:[0-9]{4}}", want: "/x/{id:[0-9]{4}}"},
		{name: "colon after brace", pattern: "/x/{a}/:b", want: "/x/{a}/{b}"},
		{name: "unclosed brace passthrough", pattern: "/x/{id:[0-9]+", want: "/x/{id:[0-9]+"},
		{name: "malformed colon", pattern: "/items/:id<abc", wantErr: true},
		{name: "malformed colon class", pattern: "/items/:id<[0-9]+", wantErr: true},
		{name: "empty regex", pattern: "/items/:id<", wantErr: true},
		{name: "malformed colon before brace", pattern: "/:id</x/{a:b}", wantErr: true},
		{name: "wildcard", pattern: "/files/*", want: "/files/*"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := ConvertColonRegex(tt.pattern)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("ConvertColonRegex(%q) want error, got nil", tt.pattern)
				}

				return
			}

			if err != nil {
				t.Fatalf("ConvertColonRegex(%q) unexpected error: %v", tt.pattern, err)
			}

			if got != tt.want {
				t.Fatalf("ConvertColonRegex(%q) = %q, want %q", tt.pattern, got, tt.want)
			}
		})
	}
}

func TestBraceRegion_table(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		pattern   string
		wantOpen  int
		wantEnd   int
		wantOK    bool
		skipMatch bool
	}{
		{name: "simple", pattern: "/x/{id:[0-9]+}", wantOpen: 3, wantEnd: 13, wantOK: true},
		{name: "nested quantifier", pattern: "/x/{id:[0-9]{4}}", wantOpen: 3, wantEnd: 15, wantOK: true},
		{name: "unclosed", pattern: "/x/{id:[0-9]+", wantOK: false},
		{name: "no brace", pattern: "/x/plain", skipMatch: true, wantOK: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			loc := braceParamRe.FindStringSubmatchIndex(tt.pattern)
			if tt.skipMatch {
				if loc != nil {
					t.Fatalf("want no match in %q, got %v", tt.pattern, loc)
				}

				return
			}

			if loc == nil {
				t.Fatalf("want match in %q, got none", tt.pattern)
			}

			open, end, ok := BraceRegion(tt.pattern, loc)
			if ok != tt.wantOK {
				t.Fatalf("BraceRegion(%q) ok = %v, want %v", tt.pattern, ok, tt.wantOK)
			}

			if !ok {
				if open != 0 || end != 0 {
					t.Fatalf("on failure want zero span, got %d:%d", open, end)
				}

				return
			}

			if open != tt.wantOpen || end != tt.wantEnd {
				t.Fatalf("BraceRegion(%q) = %d:%d, want %d:%d", tt.pattern, open, end, tt.wantOpen, tt.wantEnd)
			}
		})
	}
}

func TestStripBraceRegexWrapper_table(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		pattern string
		want    string
		wantErr bool
	}{
		{name: "plain brace", pattern: "/x/{id}", want: "/x/{id}"},
		{name: "plain regex", pattern: "/x/{id:[0-9]+}", want: "/x/{id:[0-9]+}"},
		{name: "wrapped stripped", pattern: "/x/{id:regex(^[0-9]+$)}", want: "/x/{id:^[0-9]+$}"},
		{name: "nested wrapper", pattern: "/x/{id:regex(a(b)c)}", want: "/x/{id:a(b)c}"},
		{name: "unclosed wrapper stripped", pattern: "/x/{id:regex(a(b)}", want: "/x/{id:a(b)}"},
		{name: "unanchored wrapper", pattern: "/x/{id:regex(a)extra}", wantErr: true},
		{name: "nested unanchored wrapper", pattern: "/x/{id:regex(a(b))extra}", wantErr: true},
		{name: "unclosed brace passthrough", pattern: "/x/{id:[0-9]+", want: "/x/{id:[0-9]+"},
		{name: "nested unanchored wrapper", pattern: "/x/{id:regex(a(b))extra}", wantErr: true},
		{name: "unclosed brace passthrough", pattern: "/x/{id:[0-9]+", want: "/x/{id:[0-9]+"},
		{name: "no brace", pattern: "/plain", want: "/plain"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := StripBraceRegexWrapper(tt.pattern)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("StripBraceRegexWrapper(%q) want error, got nil", tt.pattern)
				}

				if got != "" {
					t.Fatalf("on error want zero value, got %q", got)
				}

				if !strings.HasPrefix(err.Error(), "router:") {
					t.Fatalf("error must carry router: prefix, got %q", err.Error())
				}

				return
			}

			if err != nil {
				t.Fatalf("StripBraceRegexWrapper(%q) unexpected error: %v", tt.pattern, err)
			}

			if got != tt.want {
				t.Fatalf("StripBraceRegexWrapper(%q) = %q, want %q", tt.pattern, got, tt.want)
			}
		})
	}
}

func TestChiPattern_table(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		pattern string
		want    string
		wantErr bool
	}{
		{name: "colon", pattern: "/users/:id", want: "/users/{id}"},
		{name: "brace", pattern: "/users/{id}", want: "/users/{id}"},
		{name: "colon regex", pattern: "/items/:id<[0-9]+>", want: "/items/{id:[0-9]+}"},
		{name: "brace regex", pattern: "/files/{name:[a-z]+}", want: "/files/{name:[a-z]+}"},
		{name: "wrapped regex", pattern: "/x/{id:regex(^[0-9]+$)}", want: "/x/{id:^[0-9]+$}"},
		{name: "mixed", pattern: "/x/{a:[0-9]+}/:b", want: "/x/{a:[0-9]+}/{b}"},
		{name: "nested quantifier", pattern: "/x/{id:[0-9]{4}}", want: "/x/{id:[0-9]{4}}"},
		{name: "wildcard", pattern: "/files/*", want: "/files/*"},
		{name: "malformed colon", pattern: "/items/:id<[0-9]+", wantErr: true},
		{name: "unclosed wrapper stripped", pattern: "/x/{id:regex(a(b)}", want: "/x/{id:a(b)}"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := ChiPattern(tt.pattern)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("ChiPattern(%q) want error, got nil", tt.pattern)
				}

				return
			}

			if err != nil {
				t.Fatalf("ChiPattern(%q) unexpected error: %v", tt.pattern, err)
			}

			if got != tt.want {
				t.Fatalf("ChiPattern(%q) = %q, want %q", tt.pattern, got, tt.want)
			}
		})
	}
}

func TestHelpers_concurrent(t *testing.T) {
	t.Parallel()

	var wg sync.WaitGroup

	for i := 0; i < 16; i++ {
		wg.Add(1)

		go func() {
			defer wg.Done()

			if got := NormalizePattern("/x/{id:[0-9]+}"); got != "/x/:id<regex(^(?:[0-9]+)$)>" {
				t.Errorf("NormalizePattern race: got %q", got)
			}

			if got, err := ChiPattern("/x/:id<[0-9]+>"); err != nil || got != "/x/{id:[0-9]+}" {
				t.Errorf("ChiPattern race: got %q, err %v", got, err)
			}
		}()
	}

	wg.Wait()
}

func TestIsValidRegexWrapper_branches(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{name: "no prefix passes", input: "plain", want: true},
		{name: "empty passes", input: "", want: true},
		{name: "anchored passes", input: "regex([0-9]+)", want: true},
		{name: "nested anchored passes", input: "regex((a)(b))", want: true},
		{name: "trailing content fails", input: "regex([0-9]+)extra", want: false},
		{name: "unclosed fails", input: "regex(", want: false},
		{name: "unclosed nested fails", input: "regex((a)", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := isValidRegexWrapper(tt.input); got != tt.want {
				t.Fatalf("isValidRegexWrapper(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}
