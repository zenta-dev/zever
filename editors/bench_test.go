package editors

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zenta-dev/zever/dsl/gengrammar"
)

// benchFixtureDir writes canonical grammar fixtures under a fresh TempDir
// and retargets the path seam at them for the duration of the benchmark, so
// end-to-end benchmarks exercise the real load/parse/compare pipeline without
// touching the checked-in grammars.

func benchFixtureDir(b *testing.B) {
	b.Helper()

	dir := b.TempDir()
	vim, tm := filepath.Join(dir, "zen.vim"), filepath.Join(dir, "zen.tmLanguage.json")

	if err := os.WriteFile(vim, []byte(canonicalVimSyntax()), 0o600); err != nil {
		b.Fatalf("write vim fixture: %v", err)
	}

	if err := os.WriteFile(tm, canonicalTmGrammar(), 0o600); err != nil {
		b.Fatalf("write tm fixture: %v", err)
	}

	oldVim, oldTm := vimSyntaxPath, tmGrammarPath
	vimSyntaxPath, tmGrammarPath = vim, tm
	b.Cleanup(func() { vimSyntaxPath, tmGrammarPath = oldVim, oldTm })
}

// BenchmarkParseVimKeywordGroup measures the vim keyword-group line scan,
// the hottest pure function on the vim side.

func BenchmarkParseVimKeywordGroup(b *testing.B) {
	src := canonicalVimSyntax()

	b.ReportAllocs()
	for b.Loop() {
		parseVimKeywordGroup(src, "zenKeyword")
	}
}

// BenchmarkParseVimKeywordGroup_Parallel is the concurrent variant; the
// parser is a pure function of its inputs, so RunParallel is safe.

func BenchmarkParseVimKeywordGroup_Parallel(b *testing.B) {
	src := canonicalVimSyntax()

	b.ReportAllocs()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			parseVimKeywordGroup(src, "zenKeyword")
		}
	})
}

// BenchmarkParseVimAlternation measures the vim \%(a\|b\) alternation parser
// against the scalar-type rule.

func BenchmarkParseVimAlternation(b *testing.B) {
	src := canonicalVimSyntax()

	b.ReportAllocs()
	for b.Loop() {
		if _, err := parseVimAlternation(src, "zenType"); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkParseVimAlternationLabel is the same parser against the
// contextual-label rule.

func BenchmarkParseVimAlternationLabel(b *testing.B) {
	src := canonicalVimSyntax()

	b.ReportAllocs()
	for b.Loop() {
		if _, err := parseVimAlternation(src, "zenLabel"); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkParseTmAlternation measures the TextMate (a|b) alternation parser
// against the full keyword match pattern.

func BenchmarkParseTmAlternation(b *testing.B) {
	match := `\b(` + strings.Join(sortedKeys(expectedKeywords()), "|") + `)\b`

	b.ReportAllocs()
	for b.Loop() {
		if _, err := parseTmAlternation(match); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkTmRuleMatch measures named-repository rule resolution, including
// the alias fallback chain.

func BenchmarkTmRuleMatch(b *testing.B) {
	var grammar struct {
		Repository map[string]tmEntry `json:"repository"`
	}

	if err := json.Unmarshal(canonicalTmGrammar(), &grammar); err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	for b.Loop() {
		if _, err := tmRuleMatch(grammar.Repository, "keywords"); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkUpperWords measures HTTP-verb extraction from the mixed
// labels-plus-verbs set.

func BenchmarkUpperWords(b *testing.B) {
	set := toSet(append(append([]string{}, canonicalLabels...), canonicalVerbs...))

	b.ReportAllocs()
	for b.Loop() {
		upperWords(set)
	}
}

// BenchmarkDiffSets measures the set comparer on two equal 200-word sets.

func BenchmarkDiffSets(b *testing.B) {
	got := toSet(edgeBigWords(200))
	want := toSet(edgeBigWords(200))

	b.ReportAllocs()
	for b.Loop() {
		diffSets(got, want)
	}
}

// BenchmarkCheckWordSet measures the drift reporter on equal 200-word sets,
// the no-drift hot path.

func BenchmarkCheckWordSet(b *testing.B) {
	set := toSet(edgeBigWords(200))
	r := &fakeReporter{}

	b.ReportAllocs()
	for b.Loop() {
		checkWordSet(r, "vim", "zenKeyword", set, set, truthKeywords)
	}
}

// BenchmarkParseVimGrammar measures the full vim parse-and-compare pipeline
// against the canonical grammar.

func BenchmarkParseVimGrammar(b *testing.B) {
	data := []byte(canonicalVimSyntax())
	r := &fakeReporter{}

	b.ReportAllocs()
	for b.Loop() {
		if _, _, ok := parseVimGrammar(r, "vim", data); !ok {
			b.Fatal("canonical vim grammar should parse clean")
		}
	}
}

// BenchmarkParseTmGrammar measures the full TextMate parse-and-compare
// pipeline against the canonical grammar.

func BenchmarkParseTmGrammar(b *testing.B) {
	data := canonicalTmGrammar()
	r := &fakeReporter{}

	b.ReportAllocs()
	for b.Loop() {
		if _, _, ok := parseTmGrammar(r, "tm", data); !ok {
			b.Fatal("canonical tm grammar should parse clean")
		}
	}
}

// BenchmarkLoadGrammarFile measures a single grammar file read from disk.

func BenchmarkLoadGrammarFile(b *testing.B) {
	benchFixtureDir(b)

	r := &fakeReporter{}

	b.ReportAllocs()
	for b.Loop() {
		if _, ok := loadGrammarFile(r, vimSyntaxPath); !ok {
			b.Fatal("fixture should load")
		}
	}
}

// BenchmarkRunVimParity measures the end-to-end vim parity check: file load
// plus parse plus compare.

func BenchmarkRunVimParity(b *testing.B) {
	benchFixtureDir(b)

	r := &fakeReporter{}

	b.ReportAllocs()
	for b.Loop() {
		runVimParity(r)
	}
}

// BenchmarkRunTmParity measures the end-to-end TextMate parity check.

func BenchmarkRunTmParity(b *testing.B) {
	benchFixtureDir(b)

	r := &fakeReporter{}

	b.ReportAllocs()
	for b.Loop() {
		runTmParity(r)
	}
}

// BenchmarkRunCrossParity measures the end-to-end cross-grammar label/verb
// parity check over both fixture files.

func BenchmarkRunCrossParity(b *testing.B) {
	benchFixtureDir(b)

	r := &fakeReporter{}

	b.ReportAllocs()
	for b.Loop() {
		runCrossParity(r)
	}
}

// BenchmarkGengrammarVimSyntax measures vim grammar generation from the
// compiler tables.

func BenchmarkGengrammarVimSyntax(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		gengrammar.VimSyntax()
	}
}

// BenchmarkGengrammarTmLanguage measures TextMate grammar generation.

func BenchmarkGengrammarTmLanguage(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		gengrammar.TmLanguage()
	}
}

// BenchmarkGengrammarFiles measures generation of both grammar files at once.

func BenchmarkGengrammarFiles(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		gengrammar.Files()
	}
}

// BenchmarkGeneratedMatchesCheckedIn measures the full drift-detection loop:
// regenerate both grammars and byte-compare against the checked-in files.

func BenchmarkGeneratedMatchesCheckedIn(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		for path, want := range gengrammar.Files() {
			got, err := os.ReadFile(filepath.Join("..", path))
			if err != nil {
				b.Fatalf("read %s: %v", path, err)
			}

			if string(got) != string(want) {
				b.Fatalf("%s is out of date with internal/dsl/gengrammar", path)
			}
		}
	}
}
