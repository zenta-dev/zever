// Package editors guards parity between the editor grammars shipped under
// editors/nvim and editors/vscode and the DSL compiler's source of truth
// (token.Keywords and the resolver scalar table).
//
// The package contains only tests — there is no runtime code — so the
// package comment lives here instead of a doc.go.
package editors

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/zenta-dev/zever/internal/dsl/token"
)

// Grammar file locations, relative to the editors package directory (the
// working directory of `go test ./editors/`).
//
// They are variables — not constants — so tests can retarget them at
// t.TempDir() fixtures and exercise the pass, fail, and skip paths
// without touching the real grammars.
var (
	vimSyntaxPath = filepath.Join("nvim", "syntax", "zen.vim")
	tmGrammarPath = filepath.Join("vscode", "syntaxes", "zen.tmLanguage.json")
)

// Sources of truth named in failure messages.
const (
	truthKeywords = "token.Keywords"
	truthScalars  = "resolver scalarTypes (internal/dsl/resolver/resolver_entity.go)"
)

// expectedScalarTypes is the fixed v1 scalar set. It intentionally
// duplicates — rather than imports — the unexported scalarTypes table in
// internal/dsl/resolver/resolver_entity.go, so this test fails loudly if
// that table gains or loses a type without a matching grammar update.
var expectedScalarTypes = []string{
	"uuid", "string", "int32", "int64",
	"float32", "float64", "bool",
	"timestamp", "date", "bytes", "json",
}

// expectedBooleans lists the boolean literals, which are keywords to the
// lexer (token.TRUE/token.FALSE) but highlighted as a separate group.
var expectedBooleans = []string{"true", "false"}

// canonicalLabels and canonicalVerbs are the contextual block labels and
// HTTP verbs shared by both grammars. The lexer sees plain identifiers
// here, so no compiler table pins them down — parity between the two
// grammars IS the contract, enforced by runCrossParity.
var canonicalLabels = []string{
	"auth", "cron", "dispatch", "http",
	"join_table", "permission", "queue", "retry",
}

var canonicalVerbs = []string{"GET", "POST", "PUT", "PATCH", "DELETE"}

// reporter is the subset of testing.TB used by the parity helpers. Real
// tests pass *testing.T; negative-case tests pass *fakeReporter so every
// failure path (Errorf/Fatalf/Skipf bodies) executes without failing the
// suite.
type reporter interface {
	Helper()
	Skipf(format string, args ...any)
	Fatalf(format string, args ...any)
	Errorf(format string, args ...any)
}

// fakeReporter records reports instead of failing or exiting, letting
// negative-case tests drive the red paths and stay green.
type fakeReporter struct {
	messages []string
	skipped  bool
	failed   bool
}

func (f *fakeReporter) Helper() {}

func (f *fakeReporter) Skipf(format string, args ...any) {
	f.skipped = true
	f.messages = append(f.messages, fmt.Sprintf(format, args...))
}

func (f *fakeReporter) Fatalf(format string, args ...any) {
	f.failed = true
	f.messages = append(f.messages, fmt.Sprintf(format, args...))
}

func (f *fakeReporter) Errorf(format string, args ...any) {
	f.failed = true
	f.messages = append(f.messages, fmt.Sprintf(format, args...))
}

func (f *fakeReporter) joined() string { return strings.Join(f.messages, "\n") }

// displayPath renders a grammar path the way failure messages name it:
// repo-relative for the real grammars, absolute for TempDir fixtures.
func displayPath(p string) string {
	if filepath.IsAbs(p) {
		return p
	}

	return filepath.Join("editors", p)
}

// loadGrammarFile reads a grammar file. A missing file means the sibling
// agent has not landed it yet, so the caller skips; any other read error,
// or malformed content found later, hard-fails — never skips.
func loadGrammarFile(r reporter, path string) ([]byte, bool) {
	r.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			r.Skipf("grammar file %s absent (landed by sibling worktree); skipping", displayPath(path))

			return nil, false
		}

		r.Fatalf("cannot read grammar file %s: %v", displayPath(path), err)

		return nil, false
	}

	return data, true
}

var bareWordRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// parseVimKeywordGroup unions the words of every
// `syntax keyword <group> ...` line, ignoring blank lines, Vim comments,
// and non-bare tokens (e.g. match options such as `contains=@Spell`).
func parseVimKeywordGroup(src, group string) map[string]struct{} {
	out := make(map[string]struct{})
	prefix := "syntax keyword " + group

	for _, line := range strings.Split(src, "\n") {
		t := strings.TrimSpace(line)

		if t == "" || strings.HasPrefix(t, `"`) {
			continue
		}

		if !strings.HasPrefix(t, prefix) {
			continue
		}

		rest := t[len(prefix):]
		if len(rest) > 0 && rest[0] != ' ' && rest[0] != '\t' {
			continue
		}

		for _, f := range strings.Fields(rest) {
			if bareWordRe.MatchString(f) {
				out[f] = struct{}{}
			}
		}
	}

	return out
}

var vimAltRe = regexp.MustCompile(`\\%?\(([^()]*)\)`)

// parseVimAlternation extracts the `\%(a\|b\|c\)` word list of the
// `syntax match <group> ...` line. Regex shapes may evolve; only the
// word set is contractual.
func parseVimAlternation(src, group string) (map[string]struct{}, error) {
	prefix := "syntax match " + group

	for _, line := range strings.Split(src, "\n") {
		t := strings.TrimSpace(line)

		if t == "" || strings.HasPrefix(t, `"`) {
			continue
		}

		if !strings.HasPrefix(t, prefix) {
			continue
		}

		rest := t[len(prefix):]
		if len(rest) > 0 && rest[0] != ' ' && rest[0] != '\t' {
			continue
		}

		m := vimAltRe.FindStringSubmatch(t)
		if m == nil || !strings.Contains(m[1], `\|`) {
			return nil, fmt.Errorf("line for %q has no (a\\|b) alternation", group)
		}

		out := make(map[string]struct{})

		for _, w := range strings.Split(m[1], `\|`) {
			// TrimSpace first, then the backslash of the `\)` closer that
			// clings to the final word; re-check emptiness after both.
			w = strings.TrimSuffix(strings.TrimSpace(w), `\`)
			if w == "" {
				continue
			}

			out[w] = struct{}{}
		}

		if len(out) == 0 {
			return nil, fmt.Errorf("line for %q alternation holds no words", group)
		}

		return out, nil
	}

	return nil, fmt.Errorf("no syntax match line for %q", group)
}

// tmEntry is the subset of a TextMate repository rule this test reads.
type tmEntry struct {
	Match    string    `json:"match"`
	Patterns []tmEntry `json:"patterns"`
}

// findEntry returns the first repository entry found under any of the
// candidate names, so renames of the verbs rule stay backward compatible.
func findEntry(repo map[string]tmEntry, names ...string) (tmEntry, bool) {
	for _, n := range names {
		if e, ok := repo[n]; ok {
			return e, true
		}
	}

	return tmEntry{}, false
}

// entryMatch returns the effective match pattern: the rule's own match,
// else the union of its sub-pattern matches.
func entryMatch(e tmEntry) string {
	if m := strings.TrimSpace(e.Match); m != "" {
		return m
	}

	var parts []string

	for _, p := range e.Patterns {
		if m := strings.TrimSpace(p.Match); m != "" {
			parts = append(parts, m)
		}
	}

	return strings.Join(parts, "|")
}

// tmRuleMatch resolves a named repository rule to its match pattern.
func tmRuleMatch(repo map[string]tmEntry, names ...string) (string, error) {
	e, ok := findEntry(repo, names...)
	if !ok {
		return "", fmt.Errorf("tmLanguage repository entry %q not found", names[0])
	}

	if m := entryMatch(e); m != "" {
		return m, nil
	}

	return "", fmt.Errorf("tmLanguage repository entry %q has no match pattern", names[0])
}

var tmAltRe = regexp.MustCompile(`\(([^()]*)\)`)

// parseTmAlternation extracts every word of every (a|b|...) alternation in
// a TextMate match pattern. Groups without `|` (e.g. (?:...) wrappers or
// singletons) are ignored; regex shapes are not contractual, word sets are.
func parseTmAlternation(match string) (map[string]struct{}, error) {
	out := make(map[string]struct{})

	for _, m := range tmAltRe.FindAllStringSubmatch(match, -1) {
		if !strings.Contains(m[1], "|") {
			continue
		}

		for _, w := range strings.Split(m[1], "|") {
			w = strings.TrimSpace(w)
			if w == "" {
				continue
			}

			out[w] = struct{}{}
		}
	}

	if len(out) == 0 {
		return nil, fmt.Errorf("tmLanguage match %q contains no (a|b) alternation", match)
	}

	return out, nil
}

// upperWords extracts the ALL-CAPS tokens (HTTP verbs) from a word set.
// It covers grammars that embed verbs in the labels alternation instead of
// a dedicated verbs rule.
func upperWords(set map[string]struct{}) map[string]struct{} {
	out := make(map[string]struct{})

	for w := range set {
		if w != "" && w == strings.ToUpper(w) && bareWordRe.MatchString(w) {
			out[w] = struct{}{}
		}
	}

	return out
}

func toSet(words []string) map[string]struct{} {
	out := make(map[string]struct{}, len(words))
	for _, w := range words {
		out[w] = struct{}{}
	}

	return out
}

func sortedKeys(set map[string]struct{}) []string {
	out := make([]string, 0, len(set))
	for w := range set {
		out = append(out, w)
	}

	sort.Strings(out)

	return out
}

// diffSets returns sorted missing (in want, absent from got) and extra
// (in got, absent from want) words. Sorted output keeps failures
// deterministic.
func diffSets(got, want map[string]struct{}) (missing, extra []string) {
	for w := range want {
		if _, ok := got[w]; !ok {
			missing = append(missing, w)
		}
	}

	for w := range got {
		if _, ok := want[w]; !ok {
			extra = append(extra, w)
		}
	}

	sort.Strings(missing)
	sort.Strings(extra)

	return missing, extra
}

// expectedKeywords derives the reserved-word set from token.Keywords,
// minus the true/false literals highlighted as booleans.
func expectedKeywords() map[string]struct{} {
	out := make(map[string]struct{}, len(token.Keywords))

	for w := range token.Keywords {
		if w == "true" || w == "false" {
			continue
		}

		out[w] = struct{}{}
	}

	return out
}

// checkWordSet reports one line per drifted word naming the exact file,
// group, word, and source of truth, e.g.:
//
//	editors/nvim/syntax/zen.vim zenKeyword missing "message" (token.Keywords)
func checkWordSet(r reporter, file, group string, got, want map[string]struct{}, truth string) bool {
	r.Helper()

	missing, extra := diffSets(got, want)
	for _, w := range missing {
		r.Errorf("%s %s missing %q (%s)", file, group, w, truth)
	}

	for _, w := range extra {
		r.Errorf("%s %s has extra %q (%s)", file, group, w, truth)
	}

	return len(missing) == 0 && len(extra) == 0
}

// parseVimGrammar asserts the vim grammar against the compiler truth and
// returns the label/verb sets for the cross-grammar comparison.
func parseVimGrammar(r reporter, file string, data []byte) (labels, verbs map[string]struct{}, ok bool) {
	r.Helper()

	src := string(data)
	good := true

	if !checkWordSet(r, file, "zenKeyword", parseVimKeywordGroup(src, "zenKeyword"), expectedKeywords(), truthKeywords) {
		good = false
	}

	if !checkWordSet(r, file, "zenBoolean", parseVimKeywordGroup(src, "zenBoolean"), toSet(expectedBooleans), truthKeywords) {
		good = false
	}

	types, err := parseVimAlternation(src, "zenType")
	if err != nil {
		r.Fatalf("%s: %v (malformed grammar must fail, not skip)", file, err)

		return nil, nil, false
	}

	if !checkWordSet(r, file, "zenType", types, toSet(expectedScalarTypes), truthScalars) {
		good = false
	}

	labels, err = parseVimAlternation(src, "zenLabel")
	if err != nil {
		r.Fatalf("%s: %v (malformed grammar must fail, not skip)", file, err)

		return nil, nil, false
	}

	verbs = parseVimKeywordGroup(src, "zenHTTPVerb")
	if len(verbs) == 0 {
		r.Fatalf("%s zenHTTPVerb: no HTTP verbs found (malformed grammar must fail, not skip)", file)

		return nil, nil, false
	}

	return labels, verbs, good
}

// parseTmGrammar asserts the TextMate grammar against the compiler truth
// and returns the label/verb sets for the cross-grammar comparison.
func parseTmGrammar(r reporter, file string, data []byte) (labels, verbs map[string]struct{}, ok bool) {
	r.Helper()

	var grammar struct {
		Repository map[string]tmEntry `json:"repository"`
	}

	if err := json.Unmarshal(data, &grammar); err != nil {
		r.Fatalf("%s: malformed tmLanguage JSON: %v (must fail, not skip)", file, err)

		return nil, nil, false
	}

	good := true

	kwMatch, err := tmRuleMatch(grammar.Repository, "keywords")
	if err != nil {
		r.Fatalf("%s: %v (malformed grammar must fail, not skip)", file, err)

		return nil, nil, false
	}

	keywords, err := parseTmAlternation(kwMatch)
	if err != nil {
		r.Fatalf("%s keywords: %v (malformed grammar must fail, not skip)", file, err)

		return nil, nil, false
	}

	if !checkWordSet(r, file, "keywords", keywords, expectedKeywords(), truthKeywords) {
		good = false
	}

	boolMatch, err := tmRuleMatch(grammar.Repository, "booleans")
	if err != nil {
		r.Fatalf("%s: %v (malformed grammar must fail, not skip)", file, err)

		return nil, nil, false
	}

	booleans, err := parseTmAlternation(boolMatch)
	if err != nil {
		r.Fatalf("%s booleans: %v (malformed grammar must fail, not skip)", file, err)

		return nil, nil, false
	}

	if !checkWordSet(r, file, "booleans", booleans, toSet(expectedBooleans), truthKeywords) {
		good = false
	}

	typeMatch, err := tmRuleMatch(grammar.Repository, "types")
	if err != nil {
		r.Fatalf("%s: %v (malformed grammar must fail, not skip)", file, err)

		return nil, nil, false
	}

	types, err := parseTmAlternation(typeMatch)
	if err != nil {
		r.Fatalf("%s types: %v (malformed grammar must fail, not skip)", file, err)

		return nil, nil, false
	}

	if !checkWordSet(r, file, "types", types, toSet(expectedScalarTypes), truthScalars) {
		good = false
	}

	labelMatch, err := tmRuleMatch(grammar.Repository, "labels")
	if err != nil {
		r.Fatalf("%s: %v (malformed grammar must fail, not skip)", file, err)

		return nil, nil, false
	}

	labels, err = parseTmAlternation(labelMatch)
	if err != nil {
		r.Fatalf("%s labels: %v (malformed grammar must fail, not skip)", file, err)

		return nil, nil, false
	}

	if verbMatch, err := tmRuleMatch(grammar.Repository, "verbs", "httpVerb", "http_verb", "httpVerbs", "http_verbs"); err != nil {
		// Reference shape: verbs embedded in the labels alternation.
		// Split them out so the labels comparison sees labels only.
		verbs = upperWords(labels)

		for v := range verbs {
			delete(labels, v)
		}
	} else if verbs, err = parseTmAlternation(verbMatch); err != nil {
		r.Fatalf("%s verbs: %v (malformed grammar must fail, not skip)", file, err)

		return nil, nil, false
	}

	if len(verbs) == 0 {
		r.Fatalf("%s: no HTTP verbs found (malformed grammar must fail, not skip)", file)

		return nil, nil, false
	}

	return labels, verbs, good
}

func runVimParity(r reporter) {
	r.Helper()

	data, ok := loadGrammarFile(r, vimSyntaxPath)
	if ok {
		parseVimGrammar(r, displayPath(vimSyntaxPath), data)
	}
}

func runTmParity(r reporter) {
	r.Helper()

	data, ok := loadGrammarFile(r, tmGrammarPath)
	if ok {
		parseTmGrammar(r, displayPath(tmGrammarPath), data)
	}
}

func runCrossParity(r reporter) {
	r.Helper()

	vimData, ok := loadGrammarFile(r, vimSyntaxPath)
	if !ok {
		return
	}

	tmData, ok := loadGrammarFile(r, tmGrammarPath)
	if !ok {
		return
	}

	vimFile := displayPath(vimSyntaxPath)
	tmFile := displayPath(tmGrammarPath)

	vimLabels, vimVerbs, vimOK := parseVimGrammar(r, vimFile, vimData)
	if !vimOK {
		return
	}

	tmLabels, tmVerbs, tmOK := parseTmGrammar(r, tmFile, tmData)
	if !tmOK {
		return
	}

	checkWordSet(r, tmFile, "labels", tmLabels, vimLabels, vimFile+" zenLabel")
	checkWordSet(r, tmFile, "verbs", tmVerbs, vimVerbs, vimFile+" zenHTTPVerb")
}

// TestParityVimGrammar_skipsWhenAbsent checks the vim grammar against the
// compiler truth, skipping while the sibling worktree has not landed it.
func TestParityVimGrammar_skipsWhenAbsent(t *testing.T) {
	runVimParity(t)
}

// TestParityTmGrammar_skipsWhenAbsent checks the TextMate grammar against
// the compiler truth, skipping while the sibling worktree is pending.
func TestParityTmGrammar_skipsWhenAbsent(t *testing.T) {
	runTmParity(t)
}

// TestParityLabelsVerbsCross_skipsWhenAbsent checks label/verb parity
// between the two grammars, skipping unless both are present.
func TestParityLabelsVerbsCross_skipsWhenAbsent(t *testing.T) {
	runCrossParity(t)
}

// TestParseVimKeywordGroup_synthetic unit-tests the vim keyword parser.
func TestParseVimKeywordGroup_synthetic(t *testing.T) {
	tests := []struct {
		name  string
		src   string
		group string
		want  []string
	}{
		{
			name:  "single line",
			src:   "syntax keyword zenKeyword entity service message\n",
			group: "zenKeyword",
			want:  []string{"entity", "service", "message"},
		},
		{
			name: "multiple lines union",
			src: "syntax keyword zenKeyword entity service\n" +
				"syntax keyword zenKeyword message\n",
			group: "zenKeyword",
			want:  []string{"entity", "service", "message"},
		},
		{
			name: "comments blanks and unrelated lines ignored",
			src: "\" Vim syntax file\n" +
				"\n" +
				"syntax keyword zenBoolean true false\n" +
				"syntax match zenType \"\\\\<\\\\%(string\\\\)\\\\>\"\n",
			group: "zenKeyword",
			want:  nil,
		},
		{
			name:  "group prefix boundary respected",
			src:   "syntax keyword zenKeywordExtra foo\nsyntax keyword zenKeyword bar\n",
			group: "zenKeyword",
			want:  []string{"bar"},
		},
		{
			name:  "non bare tokens filtered",
			src:   "syntax keyword zenKeyword entity foo-bar x=y has_many\n",
			group: "zenKeyword",
			want:  []string{"entity", "has_many"},
		},
		{
			name:  "http verbs",
			src:   "syntax keyword zenHTTPVerb GET POST PUT PATCH DELETE\n",
			group: "zenHTTPVerb",
			want:  []string{"GET", "POST", "PUT", "PATCH", "DELETE"},
		},
		{
			name:  "empty input",
			src:   "",
			group: "zenKeyword",
			want:  nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if missing, extra := diffSets(parseVimKeywordGroup(tt.src, tt.group), toSet(tt.want)); len(missing) != 0 || len(extra) != 0 {
				t.Fatalf("parseVimKeywordGroup(%q) missing=%q extra=%q", tt.group, missing, extra)
			}
		})
	}
}

// TestParseVimAlternation_synthetic unit-tests the vim \%(a\|b) parser.
func TestParseVimAlternation_synthetic(t *testing.T) {
	tests := []struct {
		name    string
		src     string
		group   string
		want    []string
		wantErr string
	}{
		{
			name:  "type alternation",
			src:   `syntax match zenType "\<\%(uuid\|string\|bool\)\>"` + "\n",
			group: "zenType",
			want:  []string{"uuid", "string", "bool"},
		},
		{
			name:  "label line trailing options ignored",
			src:   `syntax match zenLabel "\<\%(http\|auth\)\>\s*:"me=e-1` + "\n",
			group: "zenLabel",
			want:  []string{"http", "auth"},
		},
		{
			name:    "missing group line",
			src:     "syntax keyword zenKeyword entity\n",
			group:   "zenType",
			wantErr: `no syntax match line for "zenType"`,
		},
		{
			name:    "group line without alternation",
			src:     `syntax match zenType "\d\+"` + "\n",
			group:   "zenType",
			wantErr: "no (a\\|b) alternation",
		},
		{
			name:    "alternation without words",
			src:     `syntax match zenType "\%(\|\)"` + "\n",
			group:   "zenType",
			wantErr: "holds no words",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseVimAlternation(tt.src, tt.group)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("parseVimAlternation() err=%v, want substring %q", err, tt.wantErr)
				}

				return
			}

			if err != nil {
				t.Fatalf("parseVimAlternation() unexpected err: %v", err)
			}

			if missing, extra := diffSets(got, toSet(tt.want)); len(missing) != 0 || len(extra) != 0 {
				t.Fatalf("parseVimAlternation() missing=%q extra=%q", missing, extra)
			}
		})
	}
}

// TestParseTmAlternation_synthetic unit-tests the tmLanguage (a|b) parser.
func TestParseTmAlternation_synthetic(t *testing.T) {
	tests := []struct {
		name    string
		match   string
		want    []string
		wantErr string
	}{
		{
			name:  "word boundaries stripped",
			match: `\b(entity|service|has_many)\b`,
			want:  []string{"entity", "service", "has_many"},
		},
		{
			name:  "multiple groups union",
			match: `\b(a|b)\b.*\b(c|d)\b`,
			want:  []string{"a", "b", "c", "d"},
		},
		{
			name:  "groups without pipe ignored",
			match: `(foo) (a|b)`,
			want:  []string{"a", "b"},
		},
		{
			name:    "no alternation",
			match:   `@\w+`,
			wantErr: "no (a|b) alternation",
		},
		{
			name:    "empty match",
			match:   ``,
			wantErr: "no (a|b) alternation",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseTmAlternation(tt.match)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("parseTmAlternation() err=%v, want substring %q", err, tt.wantErr)
				}

				return
			}

			if err != nil {
				t.Fatalf("parseTmAlternation() unexpected err: %v", err)
			}

			if missing, extra := diffSets(got, toSet(tt.want)); len(missing) != 0 || len(extra) != 0 {
				t.Fatalf("parseTmAlternation() missing=%q extra=%q", missing, extra)
			}
		})
	}
}

// TestTmEntryLookup_synthetic unit-tests repository entry resolution.
func TestTmEntryLookup_synthetic(t *testing.T) {
	repo := map[string]tmEntry{
		"keywords":  {Match: `\b(entity|service)\b`},
		"http_verb": {Match: `\b(GET|POST)\b`},
		"joined":    {Patterns: []tmEntry{{Match: `\b(a|x)\b`}, {Match: ""}}},
		"empty":     {},
	}

	t.Run("direct hit", func(t *testing.T) {
		m, err := tmRuleMatch(repo, "keywords")
		if err != nil || m != `\b(entity|service)\b` {
			t.Fatalf("tmRuleMatch() = %q, %v", m, err)
		}
	})

	t.Run("alias hit", func(t *testing.T) {
		m, err := tmRuleMatch(repo, "verbs", "httpVerb", "http_verb")
		if err != nil || m != `\b(GET|POST)\b` {
			t.Fatalf("tmRuleMatch() = %q, %v", m, err)
		}
	})

	t.Run("patterns joined skipping blanks", func(t *testing.T) {
		m, err := tmRuleMatch(repo, "joined")
		if err != nil || m != `\b(a|x)\b` {
			t.Fatalf("tmRuleMatch() = %q, %v", m, err)
		}
	})

	t.Run("entry without pattern fails", func(t *testing.T) {
		if _, err := tmRuleMatch(repo, "empty"); err == nil || !strings.Contains(err.Error(), "no match pattern") {
			t.Fatalf("tmRuleMatch() err=%v, want no-match-pattern error", err)
		}
	})

	t.Run("absent entry fails", func(t *testing.T) {
		if _, err := tmRuleMatch(repo, "types"); err == nil || !strings.Contains(err.Error(), `"types" not found`) {
			t.Fatalf("tmRuleMatch() err=%v, want not-found error", err)
		}
	})
}

// TestUpperWords_extractsOnlyBareUppercase checks verb extraction from a
// mixed labels set.
func TestUpperWords_extractsOnlyBareUppercase(t *testing.T) {
	got := upperWords(toSet([]string{"http", "GET", "POST", "join_table", "FOO-BAR", ""}))
	if missing, extra := diffSets(got, toSet([]string{"GET", "POST"})); len(missing) != 0 || len(extra) != 0 {
		t.Fatalf("upperWords() missing=%q extra=%q", missing, extra)
	}
}

// TestCheckWordSet_reportsDrift proves the comparer bites and that every
// failure line names the file, group, word, and source of truth.
func TestCheckWordSet_reportsDrift(t *testing.T) {
	full := toSet([]string{"entity", "service", "message"})

	tests := []struct {
		name     string
		got      map[string]struct{}
		want     map[string]struct{}
		wantPass bool
		wantSubs []string
	}{
		{
			name:     "equal sets pass silently",
			got:      full,
			want:     full,
			wantPass: true,
		},
		{
			name:     "dropped word reported",
			got:      toSet([]string{"entity", "service"}),
			want:     full,
			wantSubs: []string{"syntax/zen.vim", "zenKeyword", `"message"`, "token.Keywords", "missing"},
		},
		{
			name:     "extra word reported",
			got:      toSet([]string{"entity", "service", "message", "module"}),
			want:     full,
			wantSubs: []string{"syntax/zen.vim", "zenKeyword", `"module"`, "token.Keywords", "has extra"},
		},
		{
			name:     "label mismatch names grammar truth",
			got:      toSet([]string{"http", "auth"}),
			want:     toSet([]string{"http", "cron"}),
			wantSubs: []string{`"cron"`, `"auth"`, "vim zenLabel"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &fakeReporter{}
			file := "editors/nvim/syntax/zen.vim"
			truth := truthKeywords

			if strings.Contains(tt.name, "label") {
				file = "editors/vscode/syntaxes/zen.tmLanguage.json"
				truth = "editors/nvim/syntax/zen.vim zenLabel"
			}

			if pass := checkWordSet(f, file, "zenKeyword", tt.got, tt.want, truth); pass != tt.wantPass {
				t.Fatalf("checkWordSet() = %v, want %v", pass, tt.wantPass)
			}

			for _, sub := range tt.wantSubs {
				if !strings.Contains(f.joined(), sub) {
					t.Fatalf("checkWordSet() output %q lacks %q", f.joined(), sub)
				}
			}

			if tt.wantPass && f.failed {
				t.Fatalf("checkWordSet() reported on equal sets: %q", f.joined())
			}
		})
	}
}

// without returns words minus drop, preserving order.
func without(words []string, drop string) []string {
	var kept []string

	for _, w := range words {
		if w != drop {
			kept = append(kept, w)
		}
	}

	return kept
}

// vimSyntaxFrom builds a vim fixture from explicit word lists.
func vimSyntaxFrom(keywords, booleans, types, labels, verbs []string) string {
	return "\" canonical test fixture (not shipped)\n" +
		fmt.Sprintf("syntax keyword zenKeyword %s\n", strings.Join(keywords, " ")) +
		fmt.Sprintf("syntax keyword zenBoolean %s\n", strings.Join(booleans, " ")) +
		fmt.Sprintf(`syntax match zenType "\<\%%(%s)\>"`+"\n", strings.Join(types, `\|`)) +
		fmt.Sprintf(`syntax match zenLabel "\<\%%(%s)\>\s*:"me=e-1`+"\n", strings.Join(labels, `\|`)) +
		fmt.Sprintf("syntax keyword zenHTTPVerb %s\n", strings.Join(verbs, " "))
}

// canonicalVimSyntax builds a fully correct vim fixture from the compiler
// truth, so the fixture test fails only on harness bugs, never on drift.
func canonicalVimSyntax() string {
	return vimSyntaxFrom(sortedKeys(expectedKeywords()), expectedBooleans, expectedScalarTypes, canonicalLabels, canonicalVerbs)
}

// tmGrammarJSON builds a tmLanguage fixture from explicit word lists; a
// nil verbs list omits the verbs rule entirely.
func tmGrammarJSON(keywords, booleans, types, labels, verbs []string) []byte {
	alt := func(words []string) map[string]string {
		return map[string]string{"match": `\b(` + strings.Join(words, "|") + `)\b`}
	}

	repo := map[string]map[string]string{
		"keywords": alt(keywords),
		"booleans": alt(booleans),
		"types":    alt(types),
		"labels":   alt(labels),
	}

	if verbs != nil {
		repo["verbs"] = alt(verbs)
	}

	data, _ := json.Marshal(map[string]any{"scopeName": "source.zen", "repository": repo})

	return data
}

// canonicalTmGrammar builds a fully correct tmLanguage fixture from the
// compiler truth.
func canonicalTmGrammar() []byte {
	return tmGrammarJSON(sortedKeys(expectedKeywords()), expectedBooleans, expectedScalarTypes, canonicalLabels, canonicalVerbs)
}

func writeFixture(t *testing.T, dir, name string, data []byte) string {
	t.Helper()

	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, data, 0o600); err != nil {
		t.Fatalf("write fixture %s: %v", p, err)
	}

	return p
}

// withSeamPaths retargets the grammar path seam at dir for one test.
func withSeamPaths(t *testing.T, dir string, vim, tm bool) {
	t.Helper()

	oldVim, oldTm := vimSyntaxPath, tmGrammarPath
	t.Cleanup(func() { vimSyntaxPath, tmGrammarPath = oldVim, oldTm })

	if vim {
		vimSyntaxPath = filepath.Join(dir, "zen.vim")
	}

	if tm {
		tmGrammarPath = filepath.Join(dir, "zen.tmLanguage.json")
	}
}

// TestParityFixtures_fullPass proves the suite passes fully when correct
// grammars are present, by pointing the path seam at canonical fixtures.
func TestParityFixtures_fullPass(t *testing.T) {
	dir := t.TempDir()
	withSeamPaths(t, dir, true, true)
	writeFixture(t, dir, "zen.vim", []byte(canonicalVimSyntax()))
	writeFixture(t, dir, "zen.tmLanguage.json", canonicalTmGrammar())

	runVimParity(t)
	runTmParity(t)
	runCrossParity(t)
}

// TestParityHelpers_failRed drives every red path through a fake reporter:
// absent files skip, unreadable or malformed files hard-fail, and any word
// drift errors — proving the test bites in both directions.
func TestParityHelpers_failRed(t *testing.T) {
	kw := sortedKeys(expectedKeywords())
	noMessageVim := vimSyntaxFrom(without(kw, "message"), expectedBooleans, expectedScalarTypes, canonicalLabels, canonicalVerbs)
	moduleVim := canonicalVimSyntax() + "syntax keyword zenKeyword module\n"
	noVerbsVim := vimSyntaxFrom(kw, expectedBooleans, expectedScalarTypes, canonicalLabels, nil)
	noJsonTm := string(tmGrammarJSON(kw, expectedBooleans, without(expectedScalarTypes, "json"), canonicalLabels, canonicalVerbs))
	noCronTm := string(tmGrammarJSON(kw, expectedBooleans, expectedScalarTypes, without(canonicalLabels, "cron"), canonicalVerbs))
	noDeleteTm := string(tmGrammarJSON(kw, expectedBooleans, expectedScalarTypes, canonicalLabels, without(canonicalVerbs, "DELETE")))

	embeddedVerbsTm := func() []byte {
		labels := append(sortedKeys(toSet(canonicalLabels)), canonicalVerbs...)
		sort.Strings(labels)

		repo := map[string]map[string]string{
			"keywords": {"match": `\b(` + strings.Join(sortedKeys(expectedKeywords()), "|") + `)\b`},
			"booleans": {"match": `\b(true|false)\b`},
			"types":    {"match": `\b(` + strings.Join(expectedScalarTypes, "|") + `)\b`},
			"labels":   {"match": `\b(` + strings.Join(labels, "|") + `)\b`},
		}

		data, _ := json.Marshal(map[string]any{"scopeName": "source.zen", "repository": repo})

		return data
	}

	tests := []struct {
		name string
		// "missing" leaves the file absent; "dir" points at a directory
		// (unreadable); otherwise the string is file content.
		vim     *string
		tm      *string
		run     string // "vim", "tm", or "cross"
		wantSkp bool
		wantSub []string // required message substrings; empty means expect pass
	}{
		{name: "vim missing skips", run: "vim", wantSkp: true, wantSub: []string{"absent", "skipping"}},
		{name: "tm missing skips", run: "tm", wantSkp: true, wantSub: []string{"absent", "skipping"}},
		{name: "cross skips when tm missing", vim: strPtr(canonicalVimSyntax()), run: "cross", wantSkp: true},
		{name: "vim unreadable fails", vim: strPtr("dir"), run: "vim", wantSub: []string{"cannot read"}},
		{name: "vim without types fails", vim: strPtr("syntax keyword zenKeyword " + strings.Join(kw, " ") + "\n"), run: "vim", wantSub: []string{"zenType", "must fail, not skip"}},
		{name: "vim without verbs fails", vim: strPtr(noVerbsVim), run: "vim", wantSub: []string{"zenHTTPVerb", "no HTTP verbs"}},
		{name: "vim dropped keyword bites", vim: strPtr(noMessageVim), run: "vim", wantSub: []string{`missing "message"`, truthKeywords}},
		{name: "vim extra word bites", vim: strPtr(moduleVim), run: "vim", wantSub: []string{`has extra "module"`, truthKeywords}},
		{name: "tm malformed json fails", tm: strPtr("{not json"), run: "tm", wantSub: []string{"malformed tmLanguage JSON", "must fail, not skip"}},
		{name: "tm missing types fails", tm: strPtr(`{"repository":{"keywords":{"match":"\\b(entity|service)\\b"},"booleans":{"match":"\\b(true|false)\\b"}}}`), run: "tm", wantSub: []string{`"types" not found`, "must fail, not skip"}},
		{name: "tm dropped type bites", tm: strPtr(noJsonTm), run: "tm", wantSub: []string{`missing "json"`, truthScalars}},
		{name: "cross label drift bites", vim: strPtr(canonicalVimSyntax()), tm: strPtr(noCronTm), run: "cross", wantSub: []string{"labels", `missing "cron"`, "zenLabel"}},
		{name: "cross verb drift bites", vim: strPtr(canonicalVimSyntax()), tm: strPtr(noDeleteTm), run: "cross", wantSub: []string{"verbs", `missing "DELETE"`, "zenHTTPVerb"}},
		{name: "cross vim drift stops early", vim: strPtr(noMessageVim), tm: strPtr(string(canonicalTmGrammar())), run: "cross", wantSub: []string{`missing "message"`}},
		{name: "cross tm drift stops early", vim: strPtr(canonicalVimSyntax()), tm: strPtr(noJsonTm), run: "cross", wantSub: []string{`missing "json"`}},
		{name: "cross verbs embedded in labels pass", vim: strPtr(canonicalVimSyntax()), tm: strPtr(string(embeddedVerbsTm())), run: "cross"},
		{name: "cross labels without verbs fail", vim: strPtr(canonicalVimSyntax()), tm: strPtr(string(canonicalTmGrammarWithoutVerbs())), run: "cross", wantSub: []string{"no HTTP verbs found"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			withSeamPaths(t, dir, true, true)

			if tt.vim != nil {
				if *tt.vim == "dir" {
					vimSyntaxPath = dir
				} else {
					writeFixture(t, dir, "zen.vim", []byte(*tt.vim))
				}
			}

			if tt.tm != nil {
				writeFixture(t, dir, "zen.tmLanguage.json", []byte(*tt.tm))
			}

			f := &fakeReporter{}

			switch tt.run {
			case "vim":
				runVimParity(f)
			case "tm":
				runTmParity(f)
			case "cross":
				runCrossParity(f)
			default:
				t.Fatalf("unknown run %q", tt.run)
			}

			if f.skipped != tt.wantSkp {
				t.Fatalf("skipped=%v, want %v (%q)", f.skipped, tt.wantSkp, f.joined())
			}

			for _, sub := range tt.wantSub {
				if !strings.Contains(f.joined(), sub) {
					t.Fatalf("output %q lacks %q", f.joined(), sub)
				}
			}

			if len(tt.wantSub) == 0 && !tt.wantSkp && f.failed {
				t.Fatalf("expected pass, got failures: %q", f.joined())
			}
		})
	}
}

// canonicalTmGrammarWithoutVerbs builds a tm fixture whose labels carry no
// verbs and which has no verbs rule, exercising the empty-verbs hard fail.
func canonicalTmGrammarWithoutVerbs() []byte {
	return tmGrammarJSON(sortedKeys(expectedKeywords()), expectedBooleans, expectedScalarTypes, canonicalLabels, nil)
}

func strPtr(s string) *string { return &s }
