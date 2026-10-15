package gengrammar

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/zenta-dev/zever/dsl/resolver"
	"github.com/zenta-dev/zever/dsl/token"
)

// zenSyntaxDoc is the reference page that must mention every DSL word.
const zenSyntaxDoc = "../../docs/src/content/docs/reference/zen-syntax.mdx"

// docAttributes are the @attribute names the resolver accepts. There is no
// exported table for them, so the list is pinned here: add a new attribute
// here and to the doc page together.
var docAttributes = []string{
	"primary", "unique", "default", "validate", "renamed_from",
	"schema", "foreign_key", "on_delete",
}

// docLabels are the contextual block labels, option values, validation
// rules, and HTTP verbs the resolver accepts.
var docLabels = []string{
	"http", "auth", "permission", "errors", "paginated",
	"queue", "retry", "max_attempts", "backoff", "exponential",
	"cron", "dispatch", "execute", "compensate", "pivot", "step", "saga",
	"join_table", "required", "none",
	"restrict", "cascade", "set_null",
	"format", "min_len", "max_len", "gt", "gte", "lt", "lte",
	"now", "email", "url",
	"GET", "POST", "PUT", "PATCH", "DELETE",
}

// TestZenSyntaxDocCoversDSL fails when the .zen syntax reference omits a
// reserved keyword, scalar type, attribute, or block label.
func TestZenSyntaxDocCoversDSL(t *testing.T) {
	t.Parallel()

	raw, err := os.ReadFile(filepath.FromSlash(zenSyntaxDoc))
	if err != nil {
		t.Fatalf("read doc page: %v", err)
	}

	doc := string(raw)

	words := make([]string, 0, len(token.Keywords)+len(docAttributes)+len(docLabels)+16)

	for kw := range token.Keywords {
		words = append(words, kw)
	}

	words = append(words, resolver.ScalarTypeNames()...)

	for _, a := range docAttributes {
		words = append(words, "@"+a)
	}

	words = append(words, docLabels...)

	for _, w := range words {
		if !containsWord(doc, w) {
			t.Errorf("%s does not mention %q", zenSyntaxDoc, w)
		}
	}
}

// containsWord reports whether doc contains w delimited by non-identifier
// characters, so "gt" does not match inside "gte".
func containsWord(doc, w string) bool {
	for i := 0; i+len(w) <= len(doc); i++ {
		if doc[i:i+len(w)] != w {
			continue
		}

		if i > 0 && isIdent(doc[i-1]) && w[0] != '@' {
			continue
		}

		if j := i + len(w); j < len(doc) && isIdent(doc[j]) {
			continue
		}

		return true
	}

	return false
}

func isIdent(c byte) bool {
	return c == '_' || c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
}
