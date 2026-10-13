package render

import (
	"strings"
	"testing"
)

// FuzzSQLiteJSONPathKey proves the sqlite json1 path renderer never emits a
// JSON path key that terminates the enclosing SQL string literal early:
// arbitrary keys -- quotes, backslashes, NUL, unicode, `}`/`,`/`{` -- must
// render with every embedded `'` doubled, and the decoded literal must
// still carry the original key inside its `$."..."` label. Runs as an
// ordinary seed-corpus regression test; -fuzz=FuzzSQLiteJSONPathKey
// -fuzztime=... fuzzes continuously.
func FuzzSQLiteJSONPathKey(f *testing.F) {
	for _, seed := range jsonPathKeySeeds() {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, key string) {
		t.Parallel()

		lit := sqlitePathLiteral([]JSONStep{{Key: key}})

		mustHaveNoUnescapedQuote(t, lit)
		mustDecodeSQLitePathToKey(t, lit, key)
	})
}

// FuzzPostgresJSONPathKey proves the postgres text[] path-array renderer
// never emits an array element that breaks out of the enclosing SQL string
// literal or its double-quoted element: every `'` is doubled, every `"` and
// `\` is backslash-escaped, and unescaping the element must recover the
// original key. Runs as an ordinary seed-corpus regression test;
// -fuzz=FuzzPostgresJSONPathKey -fuzztime=... fuzzes continuously.
func FuzzPostgresJSONPathKey(f *testing.F) {
	for _, seed := range jsonPathKeySeeds() {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, key string) {
		t.Parallel()

		lit := pgPathArray([]JSONStep{{Key: key}})

		mustHaveNoUnescapedQuote(t, lit)
		mustRoundTripPostgresElement(t, lit, key)
	})
}

// jsonPathKeySeeds returns the shared seed corpus for the JSON path key
// fuzz targets: simple keys plus quotes, backslashes, NUL, unicode, and
// the postgres array-literal metacharacters `}`/`,`/`{` in isolation and
// combined.
func jsonPathKeySeeds() []string {
	return []string{
		"",
		"plain",
		"a'b",
		"a''b",
		"'",
		"\"",
		`\`,
		`a\"b`,
		"a\"b",
		`a\\b`,
		"a\\b",
		`abc\`,
		"\x00",
		"a\x00b",
		"漢字'🚀\"\\",
		"}",
		"{",
		",",
		"a}b{c,d",
		"$",
		"$.a",
		"a[0]b",
		"'; DROP TABLE widgets; --",
		`\"''`,
		`a\'`,
		"line\nbreak",
		"tab\tsep",
	}
}

// mustHaveNoUnescapedQuote fails unless lit is a single-quoted SQL string
// literal whose body contains no unescaped quote: every `'` inside must be
// doubled, so no key byte can terminate the literal.
func mustHaveNoUnescapedQuote(t *testing.T, lit string) {
	t.Helper()

	if len(lit) < 2 || lit[0] != '\'' || lit[len(lit)-1] != '\'' {
		t.Fatalf("rendered literal %q is not single-quoted", lit)
	}

	inner := lit[1 : len(lit)-1]

	for i := 0; i < len(inner); i++ {
		if inner[i] != '\'' {
			continue
		}

		if i+1 >= len(inner) || inner[i+1] != '\'' {
			t.Fatalf("unescaped quote at body offset %d in rendered literal %q", i, lit)
		}

		i++
	}
}

// mustDecodeSQLitePathToKey fails unless the literal's body, after collapsing
// doubled quotes, is exactly the `$`-rooted json path for key -- quoted via
// `$."..."` with backslash-escaped quotes/backslashes, or dotted verbatim
// for simple keys.
func mustDecodeSQLitePathToKey(t *testing.T, lit, key string) {
	t.Helper()

	body := strings.ReplaceAll(lit[1:len(lit)-1], "''", "'")

	want := "$."

	if isSimpleKey(key) {
		want += key
	} else {
		want += `"` + jsonQuoteReplacer.Replace(key) + `"`
	}

	if body != want {
		t.Fatalf("decoded path body %q does not match expected %q for key %q", body, want, key)
	}
}

// mustRoundTripPostgresElement fails unless lit is a single-quoted
// `'{"element"}'` array literal whose single double-quoted element unescapes
// (\" -> ", \\ -> \, ” -> ') back to exactly key.
func mustRoundTripPostgresElement(t *testing.T, lit, key string) {
	t.Helper()

	body := lit[1 : len(lit)-1]

	if !strings.HasPrefix(body, "{") || !strings.HasSuffix(body, "}") {
		t.Fatalf("rendered array literal %q has no {...} body", lit)
	}

	elem := body[1 : len(body)-1]

	if !strings.HasPrefix(elem, `"`) || !strings.HasSuffix(elem, `"`) {
		t.Fatalf("array element %q in %q is not double-quoted", elem, lit)
	}

	got := pgArrayElementUnescape(elem[1 : len(elem)-1])

	if got != key {
		t.Fatalf("unescaped element %q does not recover key %q", got, key)
	}
}

// pgArrayElementUnescape reverses pgArrayEscape: \" -> ", \\ -> \, ” -> '.
func pgArrayElementUnescape(s string) string {
	var b strings.Builder

	for i := 0; i < len(s); i++ {
		switch {
		case s[i] == '\\' && i+1 < len(s):
			i++
			b.WriteByte(s[i])
		case s[i] == '\'' && i+1 < len(s) && s[i+1] == '\'':
			i++
			b.WriteByte('\'')
		default:
			b.WriteByte(s[i])
		}
	}

	return b.String()
}
