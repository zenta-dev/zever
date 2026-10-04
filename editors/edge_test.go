package editors

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/zenta-dev/zever/dsl/gengrammar"
)

// edgeBigWords returns n deterministic synthetic words ("word0".."wordN-1").

func edgeBigWords(n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = fmt.Sprintf("word%d", i)
	}

	return out
}

// edgeJoin renders a word set as one sorted, space-separated string so two
// sets compare with ==.

func edgeJoin(set map[string]struct{}) string {
	return strings.Join(sortedKeys(set), " ")
}

// edgeTmWithExtras builds a canonical tmLanguage grammar carrying additional
// repository entries, to prove unknown rules are ignored.

func edgeTmWithExtras(extras map[string]map[string]string) []byte {
	labels := append(sortedKeys(toSet(canonicalLabels)), canonicalVerbs...)
	sort.Strings(labels)

	alt := func(words []string) map[string]string {
		return map[string]string{"match": `\b(` + strings.Join(words, "|") + `)\b`}
	}

	repo := map[string]map[string]string{
		"keywords": alt(sortedKeys(expectedKeywords())),
		"booleans": alt(expectedBooleans),
		"types":    alt(expectedScalarTypes),
		"labels":   alt(labels),
	}
	for name, entry := range extras {
		repo[name] = entry
	}

	data, _ := json.Marshal(map[string]any{"scopeName": "source.zen", "repository": repo})

	return data
}

// driftedVim drops "message" from the keyword list so the cross parity check
// has non-empty reporter output to compare across determinism runs.

func driftedVim() string {
	return vimSyntaxFrom(without(sortedKeys(expectedKeywords()), "message"), expectedBooleans, expectedScalarTypes, canonicalLabels, canonicalVerbs)
}

// TestEdgeEmptyAndNilInputs covers the empty-collection boundary row: every
// parser returns an empty set or a typed error, never a panic.

func TestEdgeEmptyAndNilInputs(t *testing.T) {
	t.Parallel()

	empty := map[string]struct{}{}

	tests := []struct {
		name    string
		parse   func() (map[string]struct{}, error)
		want    string
		wantErr string
	}{
		{
			name:  "empty vim source",
			parse: func() (map[string]struct{}, error) { return parseVimKeywordGroup("", "zenKeyword"), nil },
			want:  "",
		},
		{
			name: "keyword line without words",
			parse: func() (map[string]struct{}, error) {
				return parseVimKeywordGroup("syntax keyword zenKeyword\n", "zenKeyword"), nil
			},
			want: "",
		},
		{
			name: "empty group name matches nothing",
			parse: func() (map[string]struct{}, error) {
				return parseVimKeywordGroup("syntax keyword zenKeyword entity\n", ""), nil
			},
			want: "",
		},
		{
			name:    "empty vim alternation source",
			parse:   func() (map[string]struct{}, error) { return parseVimAlternation("", "zenType") },
			wantErr: `no syntax match line for "zenType"`,
		},
		{
			name:  "upperWords of empty set",
			parse: func() (map[string]struct{}, error) { return upperWords(empty), nil },
			want:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := tt.parse()
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err=%v, want substring %q", err, tt.wantErr)
				}

				return
			}

			if err != nil {
				t.Fatalf("unexpected err: %v", err)
			}

			if edgeJoin(got) != tt.want {
				t.Fatalf("got %q, want %q", edgeJoin(got), tt.want)
			}
		})
	}

	if missing, extra := diffSets(empty, empty); len(missing) != 0 || len(extra) != 0 {
		t.Fatalf("diffSets(empty, empty) = %q, %q, want both empty", missing, extra)
	}

	if n := len(toSet(nil)); n != 0 {
		t.Fatalf("toSet(nil) len = %d, want 0", n)
	}

	if _, ok := findEntry(map[string]tmEntry{}, "keywords"); ok {
		t.Fatal("findEntry on empty repo should miss")
	}

	if m := entryMatch(tmEntry{}); m != "" {
		t.Fatalf("entryMatch(zero entry) = %q, want empty", m)
	}
}

// TestEdgeMalformedGrammars drives the malformed-input error row: bad JSON
// shapes, truncated documents, and undecodable bytes must hard-fail through
// the reporter, never skip.

func TestEdgeMalformedGrammars(t *testing.T) {
	tests := []struct {
		name    string
		vim     *string
		tm      *string
		run     string
		wantSub string
	}{
		{name: "empty vim file", vim: strPtr(""), run: "vim", wantSub: "no syntax match line"},
		{name: "whitespace-only vim file", vim: strPtr("  \n\t\n"), run: "vim", wantSub: "no syntax match line"},
		{name: "vim invalid utf-8", vim: strPtr(string([]byte{0xff, 0xfe, 0x00})), run: "vim", wantSub: "no syntax match line"},
		{name: "tm repository wrong type", tm: strPtr(`{"repository":"nope"}`), run: "tm", wantSub: "malformed tmLanguage JSON"},
		{name: "tm match wrong type", tm: strPtr(`{"repository":{"keywords":{"match":42}}}`), run: "tm", wantSub: "malformed tmLanguage JSON"},
		{name: "tm truncated json", tm: strPtr(`{"repository":`), run: "tm", wantSub: "malformed tmLanguage JSON"},
		{name: "tm trailing garbage", tm: strPtr(`{}` + "xx"), run: "tm", wantSub: "malformed tmLanguage JSON"},
		{name: "tm empty object", tm: strPtr(`{}`), run: "tm", wantSub: `"keywords" not found`},
		{name: "tm null repository", tm: strPtr(`{"repository":null}`), run: "tm", wantSub: `"keywords" not found`},
		{name: "tm null entry", tm: strPtr(`{"repository":{"keywords":null}}`), run: "tm", wantSub: "no match pattern"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			f := &fakeReporter{}

			switch tt.run {
			case "vim":
				withSeamPaths(t, dir, true, false)
				writeFixture(t, dir, "zen.vim", []byte(*tt.vim))
				runVimParity(f)
			case "tm":
				withSeamPaths(t, dir, false, true)
				writeFixture(t, dir, "zen.tmLanguage.json", []byte(*tt.tm))
				runTmParity(f)
			default:
				t.Fatalf("unknown run %q", tt.run)
			}

			if !strings.Contains(f.joined(), tt.wantSub) {
				t.Fatalf("output %q lacks %q", f.joined(), tt.wantSub)
			}
		})
	}
}

// TestEdgeUnknownKeysIgnored covers the unknown-key boundary: extra vim
// syntax groups, extra tmLanguage repository entries, and extra top-level
// JSON fields must not disturb the parity verdict.

func TestEdgeUnknownKeysIgnored(t *testing.T) {
	t.Parallel()

	tmExtras := edgeTmWithExtras(map[string]map[string]string{
		"futureRule": {"match": `\b(some|future)\b`},
		"zenExtra":   {"match": `\b(x|y)\b`},
	})

	var doc map[string]any
	if err := json.Unmarshal(canonicalTmGrammar(), &doc); err != nil {
		t.Fatal(err)
	}

	doc["futureFlag"] = true

	tmTop, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}

	vimExtras := canonicalVimSyntax() + "syntax keyword zenFuture alpha beta\nsyntax match zenOther \"x\"\n"

	tests := []struct {
		name string
		vim  string
		tm   []byte
	}{
		{name: "vim unknown groups ignored", vim: vimExtras},
		{name: "tm extra repository entries ignored", tm: tmExtras},
		{name: "tm extra top-level fields ignored", tm: tmTop},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := &fakeReporter{}

			if tt.vim != "" {
				if _, _, ok := parseVimGrammar(f, "vim", []byte(tt.vim)); !ok {
					t.Errorf("vim with unknown groups should pass: %q", f.joined())
				}
			}

			if tt.tm != nil {
				if _, _, ok := parseTmGrammar(f, "tm", tt.tm); !ok {
					t.Errorf("tm with unknown keys should pass: %q", f.joined())
				}
			}

			if f.skipped {
				t.Error("unknown keys must not trigger a skip")
			}
		})
	}
}

// TestEdgeDuplicates covers the duplicate-entry boundary: repeated words
// dedupe into the set, and duplicate JSON keys follow Go's last-wins rule.

func TestEdgeDuplicates(t *testing.T) {
	t.Parallel()

	t.Run("vim words repeat across lines", func(t *testing.T) {
		t.Parallel()

		src := "syntax keyword zenKeyword entity\nsyntax keyword zenKeyword entity\nsyntax keyword zenKeyword service\n"
		if got := edgeJoin(parseVimKeywordGroup(src, "zenKeyword")); got != "entity service" {
			t.Fatalf("got %q, want %q", got, "entity service")
		}
	})

	t.Run("vim words repeat within a line", func(t *testing.T) {
		t.Parallel()

		got := parseVimKeywordGroup("syntax keyword zenKeyword entity entity service\n", "zenKeyword")
		if want := "entity service"; edgeJoin(got) != want {
			t.Fatalf("got %q, want %q", edgeJoin(got), want)
		}
	})

	t.Run("tm alternation repeats words", func(t *testing.T) {
		t.Parallel()

		got, err := parseTmAlternation(`\b(entity|entity|service)\b`)
		if err != nil {
			t.Fatal(err)
		}

		if want := "entity service"; edgeJoin(got) != want {
			t.Fatalf("got %q, want %q", edgeJoin(got), want)
		}
	})

	t.Run("json duplicate keys last wins", func(t *testing.T) {
		t.Parallel()

		var grammar struct {
			Repository map[string]tmEntry `json:"repository"`
		}

		data := []byte(`{"repository":{"keywords":{"match":"\\b(a|b)\\b"},"keywords":{"match":"\\b(c|d)\\b"}}}`)
		if err := json.Unmarshal(data, &grammar); err != nil {
			t.Fatal(err)
		}

		m, err := tmRuleMatch(grammar.Repository, "keywords")
		if err != nil {
			t.Fatal(err)
		}

		if want := `\b(c|d)\b`; m != want {
			t.Fatalf("got %q, want %q (Go JSON last-key-wins)", m, want)
		}
	})
}

// TestEdgeSingleElementAlternation covers the single-element boundary: a
// one-word keyword group parses to a one-word set, while both alternation
// parsers require at least two words (the separator must be present) and
// reject a singleton.

func TestEdgeSingleElementAlternation(t *testing.T) {
	t.Parallel()

	kwGot := parseVimKeywordGroup("syntax keyword zenKeyword entity\n", "zenKeyword")
	if want := "entity"; edgeJoin(kwGot) != want {
		t.Fatalf("keyword got %q, want %q", edgeJoin(kwGot), want)
	}

	vim := `syntax match zenType "\<\%(uuid\)\>"` + "\n"
	if _, err := parseVimAlternation(vim, "zenType"); err == nil || !strings.Contains(err.Error(), "no (a\\|b) alternation") {
		t.Fatalf("vim singleton err=%v, want no-alternation error", err)
	}

	if _, err := parseTmAlternation(`\b(GET)\b`); err == nil || !strings.Contains(err.Error(), "no (a|b) alternation") {
		t.Fatalf("tm singleton err=%v, want no-alternation error", err)
	}
}

// TestEdgePathTraversalSeamIsRaw documents that the grammar path seam
// applies no traversal guard: it reads whatever os.ReadFile resolves, so
// ".." segments, absolute paths, and missing targets all behave exactly as
// a direct os.ReadFile would.

func TestEdgePathTraversalSeamIsRaw(t *testing.T) {
	dir := t.TempDir()
	writeFixture(t, dir, "zen.vim", []byte(canonicalVimSyntax()))

	nested := filepath.Join(dir, "nested", "deeper")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}

	old := vimSyntaxPath
	t.Cleanup(func() { vimSyntaxPath = old })

	t.Run("dotdot escapes toward fixture", func(t *testing.T) {
		vimSyntaxPath = filepath.Join(nested, "..", "..", "zen.vim")

		f := &fakeReporter{}
		runVimParity(f)

		if f.skipped || f.failed {
			t.Fatalf("traversal path should load the fixture: %q", f.joined())
		}
	})

	t.Run("absolute path", func(t *testing.T) {
		vimSyntaxPath = filepath.Join(dir, "zen.vim")

		f := &fakeReporter{}
		runVimParity(f)

		if f.skipped || f.failed {
			t.Fatalf("absolute path should load the fixture: %q", f.joined())
		}
	})

	t.Run("traversal to nonexistent skips", func(t *testing.T) {
		vimSyntaxPath = filepath.Join(nested, "..", "..", "missing.vim")

		f := &fakeReporter{}
		runVimParity(f)

		if !f.skipped {
			t.Fatal("nonexistent traversal target should skip")
		}
	})
}

// TestEdgeBoundarySizes covers the size boundary: 5000-word sets, a megabyte
// comment block, and a 100 KB single word all parse without error.

func TestEdgeBoundarySizes(t *testing.T) {
	t.Parallel()

	big := edgeBigWords(5000)

	t.Run("large vim keyword group", func(t *testing.T) {
		t.Parallel()

		src := vimSyntaxFrom(big, expectedBooleans, expectedScalarTypes, canonicalLabels, canonicalVerbs)
		if got := parseVimKeywordGroup(src, "zenKeyword"); len(got) != len(big) {
			t.Fatalf("got %d words, want %d", len(got), len(big))
		}
	})

	t.Run("large tm alternation", func(t *testing.T) {
		t.Parallel()

		var grammar struct {
			Repository map[string]tmEntry `json:"repository"`
		}

		data := tmGrammarJSON(big, expectedBooleans, expectedScalarTypes, canonicalLabels, canonicalVerbs)
		if err := json.Unmarshal(data, &grammar); err != nil {
			t.Fatal(err)
		}

		m, err := tmRuleMatch(grammar.Repository, "keywords")
		if err != nil {
			t.Fatal(err)
		}

		got, err := parseTmAlternation(m)
		if err != nil {
			t.Fatal(err)
		}

		if len(got) != len(big) {
			t.Fatalf("got %d words, want %d", len(got), len(big))
		}
	})

	t.Run("megabyte vim comment block", func(t *testing.T) {
		t.Parallel()

		src := strings.Repeat("\" padding comment line\n", 45000)
		if got := parseVimKeywordGroup(src, "zenKeyword"); len(got) != 0 {
			t.Fatalf("comment-only source should yield no keywords, got %d", len(got))
		}
	})

	t.Run("very long single word", func(t *testing.T) {
		t.Parallel()

		long := strings.Repeat("a", 100000)
		got := parseVimKeywordGroup("syntax keyword zenKeyword "+long+"\n", "zenKeyword")
		if _, ok := got[long]; !ok {
			t.Fatal("100 KB single word should parse")
		}
	})
}

// TestEdgeDeterminism proves same input yields same result: repeated parses
// of the canonical grammars agree, repeated cross parity runs on drifted
// fixtures log byte-identical messages, diffSets output is insertion-order
// independent, and gengrammar generation is byte-stable.

func TestEdgeDeterminism(t *testing.T) {
	vim := canonicalVimSyntax()
	tm := canonicalTmGrammar()

	vimLabels1, vimVerbs1, ok1 := parseVimGrammar(&fakeReporter{}, "vim", []byte(vim))
	vimLabels2, vimVerbs2, ok2 := parseVimGrammar(&fakeReporter{}, "vim", []byte(vim))
	if ok1 != ok2 || edgeJoin(vimLabels1) != edgeJoin(vimLabels2) || edgeJoin(vimVerbs1) != edgeJoin(vimVerbs2) {
		t.Fatal("parseVimGrammar not deterministic")
	}

	tmLabels1, tmVerbs1, tmOK1 := parseTmGrammar(&fakeReporter{}, "tm", tm)
	tmLabels2, tmVerbs2, tmOK2 := parseTmGrammar(&fakeReporter{}, "tm", tm)
	if tmOK1 != tmOK2 || edgeJoin(tmLabels1) != edgeJoin(tmLabels2) || edgeJoin(tmVerbs1) != edgeJoin(tmVerbs2) {
		t.Fatal("parseTmGrammar not deterministic")
	}

	dir := t.TempDir()
	withSeamPaths(t, dir, true, true)
	writeFixture(t, dir, "zen.vim", []byte(driftedVim()))
	writeFixture(t, dir, "zen.tmLanguage.json", tm)

	f1, f2 := &fakeReporter{}, &fakeReporter{}
	runCrossParity(f1)
	runCrossParity(f2)

	if f1.joined() == "" {
		t.Fatal("drifted fixtures should produce reporter output")
	}

	if f1.joined() != f2.joined() {
		t.Fatalf("runCrossParity not deterministic:\n%s\n---\n%s", f1.joined(), f2.joined())
	}

	setA := toSet([]string{"a", "b", "c"})
	setB := toSet([]string{"c", "b", "a"})
	missA, extraA := diffSets(setA, setB)
	missB, extraB := diffSets(setB, setA)
	if strings.Join(missA, ",") != strings.Join(missB, ",") || strings.Join(extraA, ",") != strings.Join(extraB, ",") {
		t.Fatal("diffSets not deterministic")
	}

	if !bytes.Equal(gengrammar.VimSyntax(), gengrammar.VimSyntax()) {
		t.Fatal("gengrammar.VimSyntax not deterministic")
	}

	if !bytes.Equal(gengrammar.TmLanguage(), gengrammar.TmLanguage()) {
		t.Fatal("gengrammar.TmLanguage not deterministic")
	}

	files1, files2 := gengrammar.Files(), gengrammar.Files()
	for path, want := range files1 {
		got, ok := files2[path]
		if !ok || !bytes.Equal(got, want) {
			t.Fatalf("gengrammar.Files not deterministic for %s", path)
		}
	}
}

// TestEdgeConcurrentParsesAgree hammers both full parsers from eight
// goroutines at once; every worker must agree with the group, which also
// keeps the race detector honest about the shared read-only fixtures.

func TestEdgeConcurrentParsesAgree(t *testing.T) {
	t.Parallel()

	vim := canonicalVimSyntax()
	tm := canonicalTmGrammar()

	const workers = 8

	var wg sync.WaitGroup

	results := make([]string, workers)

	for i := range workers {
		wg.Add(1)

		go func() {
			defer wg.Done()

			labels, verbs, ok := parseVimGrammar(&fakeReporter{}, "vim", []byte(vim))
			tmLabels, tmVerbs, tmOK := parseTmGrammar(&fakeReporter{}, "tm", tm)
			results[i] = fmt.Sprintf("%v|%s|%s|%v|%s|%s", ok, edgeJoin(labels), edgeJoin(verbs), tmOK, edgeJoin(tmLabels), edgeJoin(tmVerbs))
		}()
	}

	wg.Wait()

	for _, r := range results[1:] {
		if r != results[0] {
			t.Fatalf("concurrent parse disagree:\n%s\n---\n%s", results[0], r)
		}
	}
}

// TestEdgeGeneratedGrammarsParseClean round-trips the generator output
// through the parsers: gengrammar's own files must parse without drift.

func TestEdgeGeneratedGrammarsParseClean(t *testing.T) {
	t.Parallel()

	if _, _, ok := parseVimGrammar(&fakeReporter{}, "gen", gengrammar.VimSyntax()); !ok {
		t.Error("generated vim grammar does not parse clean")
	}

	if _, _, ok := parseTmGrammar(&fakeReporter{}, "gen", gengrammar.TmLanguage()); !ok {
		t.Error("generated tm grammar does not parse clean")
	}
}

// TestEdgeVimWhitespaceVariants covers separator boundaries: CRLF endings,
// tab separators after the group name, and indented syntax lines all parse
// like their plain-LF counterparts.

func TestEdgeVimWhitespaceVariants(t *testing.T) {
	t.Parallel()

	base := canonicalVimSyntax()

	t.Run("crlf line endings", func(t *testing.T) {
		t.Parallel()

		crlf := strings.ReplaceAll(base, "\n", "\r\n")
		if got := edgeJoin(parseVimKeywordGroup(crlf, "zenKeyword")); got != edgeJoin(expectedKeywords()) {
			t.Fatalf("got %q, want %q", got, edgeJoin(expectedKeywords()))
		}
	})

	t.Run("tab after group name", func(t *testing.T) {
		t.Parallel()

		got := parseVimKeywordGroup("syntax keyword zenKeyword\tentity\n", "zenKeyword")
		if want := "entity"; edgeJoin(got) != want {
			t.Fatalf("got %q, want %q", edgeJoin(got), want)
		}
	})

	t.Run("indented syntax line", func(t *testing.T) {
		t.Parallel()

		got := parseVimKeywordGroup("  syntax keyword zenKeyword entity\n", "zenKeyword")
		if want := "entity"; edgeJoin(got) != want {
			t.Fatalf("got %q, want %q", edgeJoin(got), want)
		}
	})
}

// TestEdgeTmEntryWhitespaceMatch covers the blank-pattern boundary: a
// whitespace-only match string counts as absent, so resolution falls through
// to the entry's sub-patterns.

func TestEdgeTmEntryWhitespaceMatch(t *testing.T) {
	t.Parallel()

	repo := map[string]tmEntry{
		"blank":  {Match: " "},
		"tab":    {Match: "\t"},
		"joined": {Match: " ", Patterns: []tmEntry{{Match: `\b(a|b)\b`}}},
	}

	for _, name := range []string{"blank", "tab"} {
		if _, err := tmRuleMatch(repo, name); err == nil || !strings.Contains(err.Error(), "no match pattern") {
			t.Errorf("tmRuleMatch(%q) err=%v, want no-match-pattern error", name, err)
		}
	}

	m, err := tmRuleMatch(repo, "joined")
	if err != nil {
		t.Fatal(err)
	}

	if want := `\b(a|b)\b`; m != want {
		t.Fatalf("got %q, want %q", m, want)
	}
}

// TestEdgeCheckWordSetEmptySets covers the comparer's empty-set boundaries:
// equal empties pass silently, and one-sided empties report every word.

func TestEdgeCheckWordSetEmptySets(t *testing.T) {
	t.Parallel()

	f := &fakeReporter{}
	if !checkWordSet(f, "vim", "zenKeyword", map[string]struct{}{}, map[string]struct{}{}, truthKeywords) {
		t.Error("empty vs empty should pass")
	}

	if f.failed {
		t.Errorf("reported on empty sets: %q", f.joined())
	}

	missing := &fakeReporter{}
	checkWordSet(missing, "vim", "zenKeyword", map[string]struct{}{}, toSet([]string{"entity"}), truthKeywords)
	if !strings.Contains(missing.joined(), `missing "entity"`) {
		t.Fatalf("empty got-set should report missing entity: %q", missing.joined())
	}

	extra := &fakeReporter{}
	checkWordSet(extra, "vim", "zenKeyword", toSet([]string{"entity"}), map[string]struct{}{}, truthKeywords)
	if !strings.Contains(extra.joined(), `has extra "entity"`) {
		t.Fatalf("empty want-set should report extra entity: %q", extra.joined())
	}
}

// TestEdgeBareWordBoundaries covers the bare-word regex edges: leading
// digits, hyphens, dots, and non-ASCII letters are rejected; underscores and
// trailing digits are accepted.

func TestEdgeBareWordBoundaries(t *testing.T) {
	t.Parallel()

	src := "syntax keyword zenKeyword _x x1 1x x-y x.y café\n"
	got := parseVimKeywordGroup(src, "zenKeyword")
	if missing, extra := diffSets(got, toSet([]string{"_x", "x1"})); len(missing) != 0 || len(extra) != 0 {
		t.Fatalf("missing=%q extra=%q", missing, extra)
	}
}

// TestEdgeNestedParensInnerGroup documents an asymmetry at the
// doubly-wrapped boundary: the vim regex anchors on `\%(` immediately
// followed by word characters, so an extra outer wrapper makes the line
// unparseable, while the TextMate parser resolves through the inner group.

func TestEdgeNestedParensInnerGroup(t *testing.T) {
	t.Parallel()

	vim := `syntax match zenType "\<\%((uuid\|string)\)\>"` + "\n"
	if _, err := parseVimAlternation(vim, "zenType"); err == nil || !strings.Contains(err.Error(), "no (a\\|b) alternation") {
		t.Fatalf("vim doubly-wrapped err=%v, want no-alternation error", err)
	}

	tmGot, err := parseTmAlternation(`\b((uuid|string))\b`)
	if err != nil {
		t.Fatal(err)
	}

	if want := "string uuid"; edgeJoin(tmGot) != want {
		t.Fatalf("tm got %q, want %q", edgeJoin(tmGot), want)
	}
}

// TestEdgeTwoWordAlternationMinimum confirms the smallest accepted
// alternation: exactly two words separated by the alternation marker.

func TestEdgeTwoWordAlternationMinimum(t *testing.T) {
	t.Parallel()

	vim := `syntax match zenType "\<\%(uuid\|string\)\>"` + "\n"
	got, err := parseVimAlternation(vim, "zenType")
	if err != nil {
		t.Fatal(err)
	}

	if want := "string uuid"; edgeJoin(got) != want {
		t.Fatalf("vim got %q, want %q", edgeJoin(got), want)
	}

	tmGot, err := parseTmAlternation(`\b(GET|POST)\b`)
	if err != nil {
		t.Fatal(err)
	}

	if want := "GET POST"; edgeJoin(tmGot) != want {
		t.Fatalf("tm got %q, want %q", edgeJoin(tmGot), want)
	}
}

// TestEdgeCrossParityVimMissingSkips covers the asymmetric-skip boundary:
// the cross check skips when either grammar is absent, not only when the tm
// grammar is the missing one.

func TestEdgeCrossParityVimMissingSkips(t *testing.T) {
	dir := t.TempDir()
	withSeamPaths(t, dir, true, true)
	writeFixture(t, dir, "zen.tmLanguage.json", canonicalTmGrammar())

	f := &fakeReporter{}
	runCrossParity(f)

	if !f.skipped {
		t.Fatal("cross parity with vim grammar absent should skip")
	}
}

// TestEdgeTmRuleMatchNoNamesPanics documents a latent sharp edge: the error
// path indexes names[0], so a caller passing zero candidate names panics.
// Every in-repo caller passes at least one name; the seam API keeps the
// behavior as-is rather than growing a guard for an unreachable case.

func TestEdgeTmRuleMatchNoNamesPanics(t *testing.T) {
	t.Parallel()

	defer func() {
		if r := recover(); r == nil {
			t.Fatal("tmRuleMatch with no candidate names should panic")
		}
	}()

	_, _ = tmRuleMatch(map[string]tmEntry{"keywords": {Match: `\b(a|b)\b`}})
}

// TestEdgeGengrammarFilesMap pins the generator's file map to exactly the two
// checked-in grammar paths.

func TestEdgeGengrammarFilesMap(t *testing.T) {
	t.Parallel()

	files := gengrammar.Files()
	if len(files) != 2 {
		t.Fatalf("gengrammar.Files() has %d entries, want 2", len(files))
	}

	if _, ok := files[gengrammar.VimPath]; !ok {
		t.Errorf("gengrammar.Files() lacks %s", gengrammar.VimPath)
	}

	if _, ok := files[gengrammar.TmPath]; !ok {
		t.Errorf("gengrammar.Files() lacks %s", gengrammar.TmPath)
	}
}
