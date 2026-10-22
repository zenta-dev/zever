package main

// Drift check: the retired config-file extension set is enforced in three
// places, and the three lists must not diverge:
//
//  1. the exts list in newLegacyConfigRule (no_config_file.go),
//  2. the LEGACY_FILE and legacyNames patterns in
//     docs/scripts/no-legacy-config.mjs,
//  3. the depguard no-yaml deny-list in .golangci.yml (parser packages,
//     mapped back to the extensions they parse).
//
// The tagcheck no-config-parser module list (configParserModules in
// no_config_parser.go) is a fourth enforcement point for the same set,
// and it is covered too.
//
// Design: the test reads the three files as TEXT and compares the sets
// they contain. It does not add a YAML dependency to this module (that
// is the whole point of the gate), and it does not duplicate the lists:
// the only hand-written data here is the package-to-extension mapping,
// which maps package names to extensions rather than restating either
// list. The alternatives were worse: a generated list needs a generator
// plus a checked-in artifact, and a shared config file adds a file
// format to a gate whose point is fewer parsers.
//
// The parsing is deliberately fail-closed: every extraction asserts it
// found its anchor (exactly one exts line, the no-yaml block marker, a
// known package), so a reformat that defeats the scanner fails the test
// instead of passing vacuously. The comparisons use bare extensions only,
// never a full retired file name.

import (
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// mustRepoRoot returns the repository root: the first directory at or
// above the working directory that holds .golangci.yml.
func mustRepoRoot(t *testing.T) string {
	t.Helper()

	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

	for range 8 {
		if _, err := os.Stat(filepath.Join(dir, ".golangci.yml")); err == nil {
			return dir
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}

		dir = parent
	}

	t.Fatal("denylist drift check: no ancestor directory holds .golangci.yml")

	return ""
}

// mustFileLines returns the lines of the repo-relative file.
func mustFileLines(t *testing.T, root, rel string) []string {
	t.Helper()

	raw, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
	if err != nil {
		t.Fatal(err)
	}

	return strings.Split(string(raw), "\n")
}

// mustGoExtensions returns the retired extension set from the exts list in
// newLegacyConfigRule: the quoted tokens on the exts := []string line.
func mustGoExtensions(t *testing.T, root string) []string {
	t.Helper()

	var found []string

	for _, line := range mustFileLines(t, root, "tools/tagcheck/no_config_file.go") {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "exts := []string{") {
			continue
		}

		for _, field := range strings.Split(trimmed, `"`) {
			if len(field) >= 2 && !strings.ContainsAny(field, " {}") {
				found = append(found, field)
			}
		}

		if len(found) == 0 {
			t.Fatalf("denylist drift check: exts line holds no extensions: %q", line)
		}

		return found
	}

	t.Fatal("denylist drift check: no exts := []string line in no_config_file.go")

	return nil
}

// expandPatternToken expands one regex alternation token to the
// extensions it matches. Only the ya?ml shorthand exists; any other
// pattern syntax fails the test so a cleverer pattern cannot slip past
// the comparison unnoticed.
func expandPatternToken(t *testing.T, token string) []string {
	t.Helper()

	if token == "ya?ml" {
		return []string{"yaml", "yml"}
	}

	if strings.ContainsAny(token, `?*+()\`) {
		t.Fatalf("denylist drift check: unsupported pattern token %q", token)
	}

	return []string{token}
}

// mustMjsExtensions returns the retired extension set from
// docs/scripts/no-legacy-config.mjs: every (?:...) alternation on a line
// naming a legacy file (LEGACY_FILE and legacyNames) must agree.
func mustMjsExtensions(t *testing.T, root string) []string {
	t.Helper()

	var sets [][]string

	for _, line := range mustFileLines(t, root, "docs/scripts/no-legacy-config.mjs") {
		if !strings.Contains(line, "zever") || !strings.Contains(line, "(?:") {
			continue
		}

		start := strings.Index(line, "(?:")
		rest := line[start+len("(?:"):]
		end := strings.Index(rest, ")")

		if end < 0 {
			t.Fatalf("denylist drift check: unterminated (?:...) in line: %q", line)
		}

		var set []string

		for _, token := range strings.Split(rest[:end], "|") {
			set = append(set, expandPatternToken(t, token)...)
		}

		if len(set) == 0 {
			t.Fatalf("denylist drift check: empty extension set in line: %q", line)
		}

		sets = append(sets, set)
	}

	if len(sets) < 2 {
		t.Fatalf("denylist drift check: want the LEGACY_FILE and legacyNames patterns, found %d", len(sets))
	}

	for _, set := range sets[1:] {
		if !extensionSetsEqual(sets[0], set) {
			t.Fatalf("denylist drift check: prose-guard patterns disagree: %q vs %q", sets[0], set)
		}
	}

	return sets[0]
}

// mustDepguardPackages returns the denied package paths of the depguard
// no-yaml rule: the - pkg: entries below the no-yaml: line, up to the
// first dedent. The known yaml anchor proves the right block parsed.
func mustDepguardPackages(t *testing.T, root string) []string {
	t.Helper()

	lines := mustFileLines(t, root, ".golangci.yml")

	start := -1
	indent := ""

	for i, line := range lines {
		if strings.TrimSpace(line) == "no-yaml:" {
			start = i
			indent = line[:len(line)-len(strings.TrimLeft(line, " "))]
			break
		}
	}

	if start < 0 {
		t.Fatal("denylist drift check: no no-yaml: rule in .golangci.yml")
	}

	var pkgs []string

	for _, line := range lines[start+1:] {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}

		lineIndent := line[:len(line)-len(strings.TrimLeft(line, " "))]
		if len(lineIndent) <= len(indent) {
			break
		}

		rest, ok := strings.CutPrefix(trimmed, "- pkg:")
		if !ok {
			continue
		}

		pkg := strings.Trim(strings.TrimSpace(rest), `"`)
		if pkg == "" {
			t.Fatalf("denylist drift check: empty pkg entry in line: %q", line)
		}

		pkgs = append(pkgs, pkg)
	}

	if !slices.Contains(pkgs, "go.yaml.in/yaml") {
		t.Fatalf("denylist drift check: no-yaml block parsed as %q, want the yaml anchor in it", pkgs)
	}

	return pkgs
}

// parserModuleRoots lists the denied module roots per retired extension
// family. A denied import matches when it equals a root or lives under
// it (root + "/..."): explicit roots, no bare stems, so an unrelated
// package that merely shares a name prefix never maps.
var parserModuleRoots = map[string][]string{
	"yaml/yml": {
		"go.yaml.in/yaml",
		"gopkg.in/yaml.v2",
		"gopkg.in/yaml.v3",
		"gopkg.in/yaml.v4",
		"github.com/gopkg.in/yaml.v2",
		"github.com/gopkg.in/yaml.v3",
		"sigs.k8s.io/yaml",
	},
	"toml":         {"github.com/BurntSushi/toml", "github.com/pelletier/go-toml"},
	"ini/conf/cfg": {"gopkg.in/ini.v1", "github.com/go-ini/ini"},
	"properties":   {"github.com/magiconair/properties"},
}

// parserModuleExtensions maps a denied parser package to the retired
// extensions it parses. The second result is false for a package that
// parses none of them: an unrelated package in either deny-list fails
// the coverage test.
func parserModuleExtensions(path string) ([]string, bool) {
	matches := func(roots []string) bool {
		for _, root := range roots {
			if path == root || strings.HasPrefix(path, root+"/") {
				return true
			}
		}

		return false
	}

	switch {
	case matches(parserModuleRoots["yaml/yml"]):
		return []string{"yaml", "yml"}, true
	case matches(parserModuleRoots["toml"]):
		return []string{"toml"}, true
	case matches(parserModuleRoots["ini/conf/cfg"]):
		return []string{"ini", "conf", "cfg"}, true
	case matches(parserModuleRoots["properties"]):
		return []string{"properties"}, true
	}

	return nil, false
}

// extensionSetsEqual reports whether two extension lists hold the same
// set.
func extensionSetsEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}

	counts := map[string]int{}

	for _, ext := range a {
		counts[ext]++
	}

	for _, ext := range b {
		counts[ext]--
		if counts[ext] < 0 {
			return false
		}
	}

	return true
}

// coveredExtensions unions the extensions parsed by the denied packages.
// Every package must map to the retired set: an unrelated package is a
// finding, not silent coverage.
func coveredExtensions(t *testing.T, pkgs []string) map[string]bool {
	t.Helper()

	covered := map[string]bool{}

	for _, pkg := range pkgs {
		exts, ok := parserModuleExtensions(pkg)
		if !ok {
			t.Fatalf("denylist drift check: denied package %q parses no retired extension", pkg)
		}

		for _, ext := range exts {
			covered[ext] = true
		}
	}

	if len(covered) == 0 {
		t.Fatal("denylist drift check: no covered extensions, the package list parsed empty")
	}

	return covered
}

func TestDenylistDriftExtensions(t *testing.T) {
	t.Parallel()

	root := mustRepoRoot(t)
	goExts := mustGoExtensions(t, root)
	mjsExts := mustMjsExtensions(t, root)

	if len(goExts) == 0 || len(mjsExts) == 0 {
		t.Fatal("denylist drift check: refusing a vacuous comparison of empty sets")
	}

	if !extensionSetsEqual(goExts, mjsExts) {
		t.Fatalf("denylist drift check: tagcheck extensions %q != prose-guard extensions %q",
			slices.Sorted(slices.Values(goExts)), slices.Sorted(slices.Values(mjsExts)))
	}
}

func TestDenylistDriftParserCoverage(t *testing.T) {
	t.Parallel()

	root := mustRepoRoot(t)
	goExts := mustGoExtensions(t, root)

	// json has no third-party parser to deny: encoding/json is the
	// standard library, and the framework's own wire format still
	// marshals through it. Every other retired extension must have a
	// denied parser behind it.
	want := map[string]bool{}

	for _, ext := range goExts {
		if ext != "json" {
			want[ext] = true
		}
	}

	for name, pkgs := range map[string][]string{
		"tagcheck no-config-parser": configParserModules,
		"depguard no-yaml":          mustDepguardPackages(t, root),
	} {
		covered := coveredExtensions(t, pkgs)

		var missing []string

		for ext := range want {
			if !covered[ext] {
				missing = append(missing, ext)
			}
		}

		if len(missing) != 0 {
			covered := slices.Sorted(maps.Keys(covered))
			t.Errorf("denylist drift check: %s covers %q, missing retired extensions %q",
				name, covered, slices.Sorted(slices.Values(missing)))
		}
	}
}
