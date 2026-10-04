package gengrammar

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestKeywordsExcludeBooleans(t *testing.T) {
	t.Parallel()

	for _, w := range Keywords() {
		if w == "true" || w == "false" {
			t.Fatalf("Keywords() contains boolean literal %q", w)
		}
	}
}

func TestKeywordsSortedAndNonEmpty(t *testing.T) {
	t.Parallel()

	kw := Keywords()
	if len(kw) == 0 {
		t.Fatal("Keywords() empty, want reserved words")
	}

	for i := 1; i < len(kw); i++ {
		if kw[i-1] >= kw[i] {
			t.Fatalf("Keywords() not strictly sorted at %d: %q >= %q", i, kw[i-1], kw[i])
		}
	}
}

func TestScalarTypesNonEmpty(t *testing.T) {
	t.Parallel()

	if len(ScalarTypes()) == 0 {
		t.Fatal("ScalarTypes() empty, want the v1 scalar set")
	}
}

func TestLabelsVerbsBooleansNonEmpty(t *testing.T) {
	t.Parallel()

	if len(Labels) == 0 {
		t.Fatal("Labels empty")
	}

	if len(Verbs) == 0 {
		t.Fatal("Verbs empty")
	}

	if len(Booleans) != 2 {
		t.Fatalf("Booleans = %v, want exactly true/false", Booleans)
	}
}

func TestVimSyntaxStructure(t *testing.T) {
	t.Parallel()

	src := string(VimSyntax())

	for _, want := range []string{
		"if exists(\"b:current_syntax\")",
		"syntax keyword zenKeyword ",
		"syntax keyword zenBoolean ",
		"syntax match zenType ",
		"syntax match zenLabel ",
		"syntax keyword zenHTTPVerb ",
		"syntax match zenComment ",
		"let b:current_syntax = \"zen\"",
	} {
		if !strings.Contains(src, want) {
			t.Fatalf("VimSyntax() missing %q", want)
		}
	}
}

func TestTmLanguageIsValidJSON(t *testing.T) {
	t.Parallel()

	var doc map[string]any
	if err := json.Unmarshal(TmLanguage(), &doc); err != nil {
		t.Fatalf("TmLanguage() is not valid JSON: %v", err)
	}

	for _, key := range []string{"$schema", "name", "scopeName", "patterns", "repository"} {
		if _, ok := doc[key]; !ok {
			t.Fatalf("TmLanguage() missing top-level key %q", key)
		}
	}
}

func TestTmLanguageEmbedsEveryWordList(t *testing.T) {
	t.Parallel()

	src := string(TmLanguage())

	for _, w := range Keywords() {
		if !strings.Contains(src, w) {
			t.Fatalf("TmLanguage() missing keyword %q", w)
		}
	}

	for _, w := range ScalarTypes() {
		if !strings.Contains(src, w) {
			t.Fatalf("TmLanguage() missing scalar type %q", w)
		}
	}

	for _, w := range Labels {
		if !strings.Contains(src, w) {
			t.Fatalf("TmLanguage() missing label %q", w)
		}
	}

	for _, w := range Verbs {
		if !strings.Contains(src, w) {
			t.Fatalf("TmLanguage() missing verb %q", w)
		}
	}
}

func TestFilesDeterministicAndComplete(t *testing.T) {
	t.Parallel()

	first := Files()
	second := Files()

	if len(first) != 2 || len(second) != 2 {
		t.Fatalf("Files() = %d entries, want 2", len(first))
	}

	for path, content := range first {
		other, ok := second[path]
		if !ok {
			t.Fatalf("second Files() missing %q", path)
		}

		if string(content) != string(other) {
			t.Fatalf("Files()[%q] differs between calls", path)
		}
	}
}

func TestGrammarOutputsDiffer(t *testing.T) {
	t.Parallel()

	if string(VimSyntax()) == string(TmLanguage()) {
		t.Fatal("VimSyntax() and TmLanguage() identical, want distinct formats")
	}
}
