package gogen

import (
	"strings"
	"testing"

	"github.com/zenta-dev/zever/dsl/ir"
)

// FuzzImportClass proves import classification never panics on arbitrary
// input and stays consistent: a local import (the repo prefix, which
// carries dots) is never classified as standard library.
func FuzzImportClass(f *testing.F) {
	for _, seed := range []string{
		"",
		"net/http",
		"github.com/zenta-dev/zever/core/db",
		"github.com/google/uuid",
		"./relative",
		"/abs/path",
		"a.b/c",
		"\x00\xff/dot.",
		strings.Repeat("x.", 100),
	} {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, path string) {
		t.Parallel()

		local, std := isLocalImport(path), isStdImport(path)
		if local && std {
			t.Fatalf("isLocalImport(%q) and isStdImport(%q) both true", path, path)
		}
	})
}

// FuzzPathParamNames proves route-path param extraction never panics and
// only reports names that literally appear between braces in the input.
func FuzzPathParamNames(f *testing.F) {
	for _, seed := range []string{
		"",
		"/v1/users",
		"/v1/users/{id}",
		"/v1/{a}/{b}",
		"{unclosed",
		"unopened}",
		"{}",
		"{{nested}}",
		"/v1/{id",
	} {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, path string) {
		t.Parallel()

		// pathParamNames ranges over runes, so invalid UTF-8 decodes to
		// U+FFFD before matching: compare against the same rune-decoded
		// form (strings.ToValidUTF8 coalesces runs differently).
		var sb strings.Builder
		for _, r := range path {
			sb.WriteRune(r)
		}
		decoded := sb.String()
		for name := range pathParamNames(path) {
			if name == "" {
				t.Fatalf("pathParamNames(%q) reported an empty name", path)
			}

			if !strings.Contains(decoded, "{"+name+"}") && !strings.Contains(decoded, name) {
				t.Fatalf("pathParamNames(%q) reported %q, not present in input", path, name)
			}
		}
	})
}

// FuzzResourceIDExpr proves the permission resource-ID expression picker
// never panics on arbitrary paths and only ever returns one of its two
// canned expressions.
func FuzzResourceIDExpr(f *testing.F) {
	for _, seed := range []string{"", "/v1/users/{id}", "/v1/users/{user_id}"} {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, path string) {
		t.Parallel()

		op := opModel{Permission: &ir.PermissionCheck{}, HTTP: &ir.HTTPTransport{Path: path}}
		got := resourceIDFromRequestExpr(op)

		const wantID = `func(r *http.Request) string { return router.Param(r, "id") }`
		const wantEmpty = `func(*http.Request) string { return "" }`
		if got != wantID && got != wantEmpty {
			t.Fatalf("resourceIDFromRequestExpr(%q) = %q, want a canned expression", path, got)
		}

		if got == wantID && !strings.Contains(path, "{id}") {
			t.Fatalf("resourceIDFromRequestExpr(%q) picked the id expression without an {id} param", path)
		}

		if got == wantID && pathParamNames(path)["id"] != true {
			t.Fatalf("resourceIDFromRequestExpr(%q) disagrees with pathParamNames", path)
		}
	})
}
