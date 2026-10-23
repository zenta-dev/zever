package main

// cmd_facts_docs_test.go gates the documented CLI surface against the
// harvested facts: every `--flag` token in the CLI reference and the READMEs
// must exist in the golden facts for the command the docs attribute it to.
// It closes the auditing hole where prose flags sitting OUTSIDE a same-line
// `zever <cmd>` invocation (e.g. the deprecated hidden `--profile` alias)
// were never machine-checked.

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/zenta-dev/zever/shared/codec"
)

// docsFlagAllowEntry is one allowlisted flag token: a token that looks like a
// flag but is not a zever CLI flag (foreign tool syntax, a documented
// pseudo-flag, or a flag living in a legacy stdlib flagset no longer visible
// on the Cobra help tree). Why carries the one-line justification reviewed at
// seed time; Via optionally restricts the entry to one attributed command
// path ("" = any attribution in that file).
type docsFlagAllowEntry struct {
	File  string `json:"file"`
	Token string `json:"token"`
	Via   string `json:"via"`
	Why   string `json:"why"`
}

// docsFlagAllow is the committed allowlist file.
type docsFlagAllow struct {
	Entries []docsFlagAllowEntry `json:"entries"`
}

// cmdDocsSource is one parsed documentation file.
type cmdDocsSource struct {
	Rel  string // path relative to the repo root, as allowlist entries name it
	Path string // absolute path on disk
}

// cmdDocsFinding is one gate result: where, what, and how it was attributed.
type cmdDocsFinding struct {
	File   string // repo-root-relative file
	Line   int
	Token  string
	Attrib string // attributed command path ("zever", "new", …) or "" when unattributed
	Reason string // human-readable reason, "unattributed" when no owner found
}

// docsFilesRelative lists the documentation inputs relative to the repo root.
var docsFilesRelative = []string{
	"docs/src/content/docs/reference/cli.mdx",
	"README.md",
	"cmd/zever/README.md",
}

// docsFlagTokenRe matches --long and -s flag tokens with a hard boundary on
// the left (not inside a word like "golangci--lint"); find captures the flag
// itself in group 1, trimming the boundary rune and any =value.
var docsFlagTokenRe = regexp.MustCompile(`(?:^|[^\w-])(-{1,2}[A-Za-z][A-Za-z0-9-]*)`)

// cmdDocsToken trims a matched token to its bare flag shape (--foo / -x).
func cmdDocsToken(raw string) string {
	tok := strings.TrimLeft(raw, "-")
	if i := strings.IndexByte(tok, '='); i >= 0 {
		tok = tok[:i]
	}

	return strings.Repeat("-", len(raw)-len(strings.TrimLeft(raw, "-"))) + tok
}

// cmdDocsMentionRe captures the bare `zever` word in group 1 with a hard
// non-word boundary on both sides (line start/end included).
var cmdDocsMentionRe = regexp.MustCompile(`(?:^|[^\w])(zever)(?:[^\w]|$)`)

// docsMention is a resolved `zever <cmd>` mention on one line.
type docsMention struct {
	Col  int // rune column of the zever token start
	Path string
}

// cmdDocsLine holds the pre-parsed shape of one documentation line.
type cmdDocsLine struct {
	Number  int
	Text    string
	InFence bool
	Heading bool
}

// cmdDocsFenceState walks lines once marking fenced regions. Opening fences
// (```) start a block, closing fences end it; the info string is ignored so
// ```sh, ```bash, ```text and ```go all parse. Flag tokens inside a fenced
// block resolve against mentions inside the same block first.
func cmdDocsFenceState(lines []cmdDocsLine) {
	inFence := false

	for i := range lines {
		trimmed := strings.TrimSpace(lines[i].Text)
		if strings.HasPrefix(trimmed, "```") {
			if !inFence {
				inFence = true
				lines[i].Heading = false
			} else {
				inFence = false
			}
		}

		lines[i].InFence = inFence
		lines[i].Heading = !inFence && strings.HasPrefix(lines[i].Text, "#")
	}
}

// isDocsInsideURL reports whether the zever token at col sits inside a URL
// without whitespace between the scheme and the token (curl installers,
// "zenta-dev/zever/main/install.sh"), which never name CLI commands.
func isDocsInsideURL(line string, col int) bool {
	lo := col
	for lo > 0 && line[lo-1] != ' ' && line[lo-1] != '\t' {
		lo--
	}

	return strings.Contains(line[lo:col], "://")
}

// cmdDocsResolvePath greedily extends a `zever` mention into the longest
// known command path formed by the whitespace-separated tokens that follow
// it, falling back to shorter prefixes and then to the bare root ("zever").
func cmdDocsResolvePath(words []string, paths map[string]bool) string {
	for n := wordsJoinedCandidates(words); n > 0; n-- {
		cand := strings.Join(words[:n], " ")
		if paths[cand] {
			return cand
		}
	}

	return "zever"
}

// wordsJoinedCandidates bounds the greedy extension: never more words than
// the mention carries, never fewer than 1.
func wordsJoinedCandidates(words []string) int {
	if len(words) > 4 {
		return 4
	}

	return len(words)
}

// cmdDocsWordsAfter splits the remainder of a line after a `zever` mention
// into the whitespace-separated word tokens that can extend the command
// path. A token like `new`,` contributes its valid "new" prefix and drops
// the glued backtick.
func cmdDocsWordsAfter(line string, start int) []string {
	fields := strings.Fields(line[start:])
	words := []string{}

	for _, f := range fields {
		if f == "" || strings.HasPrefix(f, "-") || strings.ContainsAny(f, "<([{") {
			break
		}

		prefix := docsWordNamePrefix(f)
		if prefix == "" {
			break
		}

		words = append(words, prefix)
	}

	return words
}

// docsWordNamePrefix returns the leading run of command-name characters in a
// token, stopping at the first glue character (`,`, backtick, colon-ends).
func docsWordNamePrefix(f string) string {
	end := 0
	for end < len(f) {
		r := rune(f[end])
		isName := r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' ||
			r == '.' || r == ':' || r == '-' || r == '_' || r == '/'
		if !isName {
			break
		}

		end++
	}

	if end == 0 {
		return ""
	}

	// A digit-led token ("8080") or bare "." is prose, not a command name.
	if !docsWordIsLetter(rune(f[0])) {
		return ""
	}

	return f[:end]
}

// docsWordIsLetter reports whether r is an ASCII letter (command names start
// with one).
func docsWordIsLetter(r rune) bool {
	return r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z'
}

// cmdDocsScanFile returns, for one file, the per-line mentions and flag
// tokens, in line order; read errors fail the test (must-helper).
func mustCmdDocsScanFile(t *testing.T, src cmdDocsSource) (mentions map[int][]docsMention, tokens map[int][]string, lines []cmdDocsLine) {
	t.Helper()

	raw, err := os.ReadFile(src.Path)
	if err != nil {
		t.Fatalf("read docs source %s: %v", src.Rel, err)
	}

	text := strings.ReplaceAll(string(raw), "\r\n", "\n")
	split := strings.Split(text, "\n")
	lines = make([]cmdDocsLine, len(split))
	for i, l := range split {
		lines[i] = cmdDocsLine{Number: i + 1, Text: l}
	}

	cmdDocsFenceState(lines)

	mentions = map[int][]docsMention{}
	tokens = map[int][]string{}

	for i, l := range lines {
		for _, loc := range cmdDocsMentionRe.FindAllStringSubmatchIndex(l.Text, -1) {
			// Group 1 is the bare "zever" token itself; the regex consumes
			// one boundary rune on each side.
			start := loc[2]

			if start > 0 && isDocsWordByte(l.Text[start-1]) {
				continue
			}

			// Compound names ("zever-docs", "zever-lsp") and URL paths
			// ("zenta-dev/zever/...") are prose, not commands.
			if start+5 < len(l.Text) && isDocsWordByte(l.Text[start+5]) {
				continue
			}

			if isDocsInsideURL(l.Text, start) {
				continue
			}

			words := cmdDocsWordsAfter(l.Text, start+5)
			mentions[i] = append(mentions[i], docsMention{Col: start, Path: cmdDocsResolvePath(words, harvestPathsCache)})
		}

		for _, loc := range docsFlagTokenRe.FindAllStringSubmatchIndex(l.Text, -1) {
			tok := cmdDocsToken(l.Text[loc[2]:loc[3]])
			tokens[i] = append(tokens[i], tok)
		}
	}

	return mentions, tokens, lines
}

// isDocsWordByte reports whether b is a word character for the glued-token
// boundary check.
func isDocsWordByte(b byte) bool {
	return b == '_' || b == '-' ||
		b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z' || b >= '0' && b <= '9'
}

// harvestPathsCache is populated once per test run from the fact harvest.
var harvestPathsCache = map[string]bool{}

// cmdDocsAttribute resolves the owning command for a flag token per the gate
// heuristic: the nearest preceding `zever <cmd>` mention on the same line
// (or, when no mention precedes the token, the first one on the line, which
// is how flag-first table rows read); inside a fenced block, the nearest
// block mention; otherwise markdown list-item inheritance; otherwise the
// statement-of-record carry from the nearest heading that names a command
// (heading mentions count; bare headings reset). With no owner at all the
// finding is unattributed and must be allowlisted with a reviewed why.
func cmdDocsAttribute(idx, tokCol int, lines []cmdDocsLine, mentions map[int][]docsMention) (string, string) {
	if ms := mentions[idx]; len(ms) > 0 {
		return ms[cmdDocsPrecedingMention(ms, tokCol)].Path, "same line"
	}

	if lines[idx].InFence {
		// Same fenced block: prefer previous lines, then following ones.
		for i := idx - 1; i >= 0 && lines[i].InFence; i-- {
			if ms := mentions[i]; len(ms) > 0 {
				return ms[len(ms)-1].Path, "same fenced block, above"
			}
		}

		for i := idx + 1; i < len(lines) && lines[i].InFence; i++ {
			if ms := mentions[i]; len(ms) > 0 {
				return ms[0].Path, "same fenced block, below"
			}
		}

		return "", "no command mention in the fenced block"
	}

	// Markdown list-item continuations inherit the item's first line.
	if itemStart := cmdDocsListItemStart(idx, lines); itemStart < idx {
		if ms := mentions[itemStart]; len(ms) > 0 {
			return ms[len(ms)-1].Path, "own markdown list item"
		}
	}

	// Section carry: the nearest preceding command mention above the line;
	// headings participate when they themselves name a command, and bare
	// headings break the carry.
	for i := idx - 1; i >= 0; i-- {
		if ms := mentions[i]; len(ms) > 0 {
			return ms[len(ms)-1].Path, fmt.Sprintf("carry from line %d", lines[i].Number)
		}

		if lines[i].Heading {
			break
		}
	}

	return "", "no command mention nearby"
}

// cmdDocsPrecedingMention picks the mention that owns a token: the last one
// starting before it that resolved to a command path (bare `zever` mentions
// from prose like "prints usage" never claim trailing flags); else the last
// preceding mention of any kind; else the first on the line (flag-first
// table cells like `| -y | zever new: ... |`).
func cmdDocsPrecedingMention(ms []docsMention, tokCol int) int {
	owner := -1

	for i, m := range ms {
		if m.Col <= tokCol && m.Path != "zever" {
			owner = i
		}
	}

	if owner == -1 {
		for i := range ms {
			if ms[i].Col <= tokCol {
				owner = i
			}
		}
	}

	if owner == -1 {
		return 0
	}

	return owner
}

// cmdDocsListItemStart walks up over markdown list-continuation lines to the
// line that opened the current bullet item; returns idx when not in one.
// Continuation lines are non-empty, non-heading, non-fence, not bullets.
func cmdDocsListItemStart(idx int, lines []cmdDocsLine) int {
	start := idx

	for i := idx - 1; i >= 0; i-- {
		if lines[i].InFence || lines[i].Heading || isDocsBulletStart(lines[i].Text) ||
			strings.TrimSpace(lines[i].Text) == "" {
			break
		}

		start = i
	}

	return start
}

// isDocsBulletStart reports whether the line opens a markdown bullet item.
func isDocsBulletStart(t string) bool {
	t = strings.TrimLeft(t, " \t")

	return strings.HasPrefix(t, "- ") || strings.HasPrefix(t, "* ") ||
		strings.HasPrefix(t, "1. ")
}

// cmdDocsLoadAllow reads the seed allowlist, missing file is a setup error.
func cmdDocsLoadAllow(t *testing.T) docsFlagAllow {
	t.Helper()

	path := filepath.Join("testdata", "cmd_facts_allow.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read allowlist %s: %v", path, err)
	}

	var allow docsFlagAllow
	if err := codec.Unmarshal(raw, &allow); err != nil {
		t.Fatalf("parse allowlist %s: %v", path, err)
	}

	return allow
}

// cmdDocsFilterAllowed drops findings an allowlist entry covers.
func cmdDocsFilterAllowed(allow docsFlagAllow, findings []cmdDocsFinding) []cmdDocsFinding {
	kept := findings[:0]

	for _, f := range findings {
		if !cmdDocsAllowed(allow, f) {
			kept = append(kept, f)
		}
	}

	return kept
}

// cmdDocsAllowed reports whether an entry allows the finding.
func cmdDocsAllowed(allow docsFlagAllow, f cmdDocsFinding) bool {
	for _, e := range allow.Entries {
		if e.Token != f.Token || e.File != f.File {
			continue
		}

		if e.Via == "" || e.Via == f.Attrib {
			return true
		}
	}

	return false
}

// docsSourcesAtInventory enumerates every parsed doc source relative to the
// repo root so allowlist file keys stay stable.
func docsSourcesAtInventory(t *testing.T, repoRoot string) []cmdDocsSource {
	t.Helper()

	sources := []cmdDocsSource{}

	for _, rel := range docsFilesRelative {
		p := filepath.Join(repoRoot, rel)
		if _, err := os.Stat(p); err != nil {
			t.Fatalf("docs source missing: %v", err)
		}

		sources = append(sources, cmdDocsSource{Rel: rel, Path: p})
	}

	exDir := filepath.Join(repoRoot, "examples")
	dirents, err := os.ReadDir(exDir)
	if err != nil {
		t.Fatalf("read examples dir: %v", err)
	}

	names := []string{}
	for _, d := range dirents {
		if d.IsDir() {
			names = append(names, d.Name())
		}
	}

	sort.Strings(names)

	for _, n := range names {
		p := filepath.Join(exDir, n, "README.md")
		if _, err := os.Stat(p); err == nil {
			sources = append(sources, cmdDocsSource{Rel: "examples/" + n + "/README.md", Path: p})
		}
	}

	return sources
}

// TestCmdFactsDocsGate replays the docs-vs-truth contract: every documented
// flag token must exist in the CLI facts for its attributed command, and the
// deprecated hidden alias --profile must be visible to the harvest. Unknown
// or unattributable tokens fail with file:line unless the allowlist carries
// a reviewed seed.
func TestCmdFactsDocsGate(t *testing.T) {
	facts := harvestCmdFacts(factsRoot(t))
	paths, localFlags, globals := facts.factsFlagsByName()
	for p := range paths {
		harvestPathsCache[p] = true
	}

	// The prove-it-live assertion from the sample hole: the deprecated
	// --profile alias is a fact for `zever new`, so docs may cite it.
	newFlags := localFlags["new"]
	if !newFlags["profile"] {
		t.Fatalf(`facts for "new" lost the deprecated --profile alias; the docs gate must see hidden aliases`)
	}

	if !globals["no-color"] || !globals["quiet"] || !globals["json"] {
		t.Fatalf("root persistent globals drifted: %v", facts.Globals)
	}

	allow := cmdDocsLoadAllow(t)

	findings := []cmdDocsFinding{}
	docsDirs := docsSourcesAtInventory(t, docsRepoRoot(t))

	for _, src := range docsDirs {
		mentions, tokens, lines := mustCmdDocsScanFile(t, src)

		for i, toks := range tokens {
			for col, tok := range toks {
				locs := docsFlagTokenRe.FindAllStringSubmatchIndex(lines[i].Text, -1)
				tokCol := locs[col][2]
				attrib, how := cmdDocsAttribute(i, tokCol, lines, mentions)

				if facts.factsHasFlag(attribPath(attrib), flagName(tok)) {
					continue
				}

				findings = append(findings, cmdDocsFinding{
					File:   src.Rel,
					Line:   lines[i].Number,
					Token:  tok,
					Attrib: attrib,
					Reason: how,
				})
			}
		}
	}

	findings = cmdDocsFilterAllowed(allow, findings)

	if len(findings) > 0 {
		msgs := make([]string, 0, len(findings))
		for _, f := range findings {
			msgs = append(msgs, fmt.Sprintf("  %s:%d: %s attributed to %q (%s)", f.File, f.Line, f.Token, f.Attrib, f.Reason))
		}

		t.Errorf("documented flags not backed by the CLI facts (%d):\n%s\n"+
			"review each: if it is genuinely not a zever flag, seed testdata/cmd_facts_allow.json with a justification;\n"+
			"if it is a real flag, the harvest (and golden) must be regenerated", len(findings), strings.Join(msgs, "\n"))
	}
}

// attribPath maps "" (unattributed) to the bare root for lookup.
func attribPath(a string) string {
	if a == "" {
		return "zever"
	}

	return a
}

// flagName maps a shorthand token (-x) onto the matching long name in the
// facts so docs shorthands gate against real shorthands only.
func flagName(tok string) string {
	return strings.TrimLeft(tok, "-")
}

// docsRepoRoot walks up from the package dir to the repo root (the directory
// holding go.work), so the test works from module or workspace contexts.
func docsRepoRoot(t *testing.T) string {
	t.Helper()

	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}

	for i := 0; i < 6; i++ {
		if _, err := os.Stat(filepath.Join(dir, "go.work")); err == nil {
			return dir
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("no repo root (go.work) above the test dir")
		}

		dir = parent
	}

	t.Fatal("no repo root (go.work) within six levels")
	return ""
}
