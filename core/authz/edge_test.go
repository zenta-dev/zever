package authz

import "testing"

func TestEdgeParseBearerToken_table(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		header string
		want   string
	}{
		{"empty", "", ""},
		{"scheme only", "Bearer", ""},
		{"scheme space", "Bearer ", ""},
		{"lowercase scheme", "bearer tok", "tok"},
		{"mixed case", "BeArEr tok", "tok"},
		{"surrounding space", "  Bearer tok  ", "tok"},
		{"internal space rejected", "Bearer a b", ""},
		{"tab rejected", "Bearer a\tb", ""},
		{"wrong scheme", "Basic tok", ""},
		{"no scheme", "tok", ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := parseBearerToken(tc.header); got != tc.want {
				t.Fatalf("parseBearerToken(%q) = %q, want %q", tc.header, got, tc.want)
			}
		})
	}
}

func TestEdgeStringAttrs_table(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   map[string]any
		want int
	}{
		{"nil", nil, 0},
		{"empty", map[string]any{}, 0},
		{"all non-string", map[string]any{"n": 1, "b": true}, 0},
		{"mixed", map[string]any{"s": "v", "n": 1}, 1},
		{"all string", map[string]any{"a": "1", "b": "2"}, 2},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := stringAttrs(tc.in); len(got) != tc.want {
				t.Fatalf("stringAttrs(%v) len = %d, want %d", tc.in, len(got), tc.want)
			}
		})
	}
}

func TestEdgeStringAttrs_preservesValues(t *testing.T) {
	t.Parallel()

	got := stringAttrs(map[string]any{"team": "eng", "count": 3})
	if got["team"] != "eng" {
		t.Fatalf("stringAttrs team = %q, want eng", got["team"])
	}

	if _, ok := got["count"]; ok {
		t.Fatal("stringAttrs kept non-string value")
	}
}
