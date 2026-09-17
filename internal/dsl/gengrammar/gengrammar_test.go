package gengrammar

import (
	"encoding/json"
	"sort"
	"strings"
	"testing"

	"github.com/zenta-dev/zever/internal/dsl/resolver"
	"github.com/zenta-dev/zever/internal/dsl/token"
)

// TestKeywords_matchesTokenKeywordsMinusBooleans proves Keywords() is
// exactly token.Keywords with the boolean literals removed, sorted.
func TestKeywords_matchesTokenKeywordsMinusBooleans(t *testing.T) {
	want := make([]string, 0, len(token.Keywords))

	for w := range token.Keywords {
		if w == "true" || w == "false" {
			continue
		}

		want = append(want, w)
	}

	sort.Strings(want)

	got := Keywords()
	if len(got) != len(want) {
		t.Fatalf("Keywords() = %v, want %v", got, want)
	}

	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("Keywords() = %v, want %v", got, want)
		}
	}

	if !sort.StringsAreSorted(got) {
		t.Fatalf("Keywords() not sorted: %v", got)
	}
}

// TestScalarTypes_matchesResolver proves ScalarTypes() is exactly
// resolver.ScalarTypeNames().
func TestScalarTypes_matchesResolver(t *testing.T) {
	want := resolver.ScalarTypeNames()
	got := ScalarTypes()

	if len(got) != len(want) {
		t.Fatalf("ScalarTypes() = %v, want %v", got, want)
	}

	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("ScalarTypes() = %v, want %v", got, want)
		}
	}
}

// TestFiles_deterministic proves repeated calls produce byte-identical
// output, which is what lets `make generate` be a no-op on a clean tree.
func TestFiles_deterministic(t *testing.T) {
	a := Files()
	b := Files()

	if len(a) != len(b) {
		t.Fatalf("Files() length changed between calls: %d vs %d", len(a), len(b))
	}

	for path, content := range a {
		other, ok := b[path]
		if !ok {
			t.Fatalf("Files() missing %s on second call", path)
		}

		if string(content) != string(other) {
			t.Fatalf("Files()[%s] not deterministic", path)
		}
	}
}

// TestFiles_knownPaths proves Files() only ever emits the two known
// grammar paths.
func TestFiles_knownPaths(t *testing.T) {
	files := Files()

	if _, ok := files[VimPath]; !ok {
		t.Errorf("Files() missing %s", VimPath)
	}

	if _, ok := files[TmPath]; !ok {
		t.Errorf("Files() missing %s", TmPath)
	}

	if len(files) != 2 {
		t.Errorf("Files() = %d entries, want 2", len(files))
	}
}

// TestVimSyntax_containsGeneratedWords proves every keyword, boolean, and
// scalar type appears as its own vim syntax line.
func TestVimSyntax_containsGeneratedWords(t *testing.T) {
	src := string(VimSyntax())

	if !strings.Contains(src, "syntax keyword zenKeyword ") {
		t.Error("VimSyntax() missing zenKeyword line")
	}

	if !strings.Contains(src, "syntax keyword zenBoolean true false") && !strings.Contains(src, "syntax keyword zenBoolean false true") {
		t.Error("VimSyntax() missing zenBoolean line with true/false")
	}

	for _, w := range ScalarTypes() {
		if !strings.Contains(src, w) {
			t.Errorf("VimSyntax() missing scalar type %q", w)
		}
	}

	for _, w := range Keywords() {
		if !strings.Contains(src, w) {
			t.Errorf("VimSyntax() missing keyword %q", w)
		}
	}
}

// TestTmLanguage_validJSON proves the generated TextMate grammar is valid
// JSON with the expected repository entries.
func TestTmLanguage_validJSON(t *testing.T) {
	var doc struct {
		Repository map[string]struct {
			Match string `json:"match"`
		} `json:"repository"`
	}

	if err := json.Unmarshal(TmLanguage(), &doc); err != nil {
		t.Fatalf("TmLanguage() is not valid JSON: %v", err)
	}

	for _, name := range []string{"keywords", "booleans", "types", "labels"} {
		entry, ok := doc.Repository[name]
		if !ok {
			t.Errorf("TmLanguage() repository missing %q", name)

			continue
		}

		if entry.Match == "" {
			t.Errorf("TmLanguage() repository %q has empty match", name)
		}
	}

	for _, w := range ScalarTypes() {
		if !strings.Contains(doc.Repository["types"].Match, w) {
			t.Errorf("TmLanguage() types pattern missing %q", w)
		}
	}
}
