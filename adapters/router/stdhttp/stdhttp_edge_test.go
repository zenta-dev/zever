package stdhttp

import (
	"reflect"
	"testing"
)

func TestParamNames_table(t *testing.T) {
	t.Parallel()

	cases := []struct {
		path string
		want []string
	}{
		{"/users/{id}", []string{"id"}},
		{"/a/{x}/b/{y}", []string{"x", "y"}},
		{"/files/{rest...}", []string{"rest"}},
		{"/exact/{$}", nil},
		{"/{}", nil},
		{"/u1/{id", nil},
		{"/{a}{b}", []string{"a", "b"}},
		{"/static", nil},
	}
	for _, tc := range cases {
		got := paramNames(tc.path)
		if !reflect.DeepEqual(got, tc.want) {
			t.Errorf("paramNames(%q) = %v, want %v", tc.path, got, tc.want)
		}
	}
}

func TestNormalizePrefix_table(t *testing.T) {
	t.Parallel()

	cases := []struct {
		in, want string
	}{
		{"", "/"},
		{"/", "/"},
		{"api", "/api"},
		{"/api/", "/api"},
		{"  /api  ", "/api"},
		{"///", "/"},
	}
	for _, tc := range cases {
		if got := normalizePrefix(tc.in); got != tc.want {
			t.Errorf("normalizePrefix(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestJoinPath_table(t *testing.T) {
	t.Parallel()

	cases := []struct {
		prefix, pattern, want string
	}{
		{"/api", "/users", "/api/users"},
		{"/api/", "users", "/api/users"},
		{"/", "/x", "/x"},
		{"", "x", "/x"},
	}
	for _, tc := range cases {
		if got := joinPath(tc.prefix, tc.pattern); got != tc.want {
			t.Errorf("joinPath(%q, %q) = %q, want %q", tc.prefix, tc.pattern, got, tc.want)
		}
	}
}

func TestHasColonSegment_table(t *testing.T) {
	t.Parallel()

	cases := []struct {
		in   string
		want bool
	}{
		{"/users/:id", true},
		{"/users/{id}", false},
		{"/a/b/c", false},
		{"/time/12:30", true},
	}
	for _, tc := range cases {
		if got := hasColonSegment(tc.in); got != tc.want {
			t.Errorf("hasColonSegment(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

func TestHasBraceColon_table(t *testing.T) {
	t.Parallel()

	cases := []struct {
		in   string
		want bool
	}{
		{"/users/{id:[0-9]+}", true},
		{"/users/{id}", false},
		{"/u/{a}/{b}", false},
		{"/u/{id", false},
	}
	for _, tc := range cases {
		if got := hasBraceColon(tc.in); got != tc.want {
			t.Errorf("hasBraceColon(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}
