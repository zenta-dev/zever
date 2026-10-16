package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// ErrVersionFile reports a missing or malformed cmd/zever/VERSION.
var ErrVersionFile = errors.New("modgraph: cmd/zever/VERSION unreadable")

// pinLine matches a go.mod line that requires an intra-repo module, either
// as a single-line `require` directive or as a block entry. Lines holding a
// `=>` (replace directives) never match because the version must be followed
// only by an optional trailing comment.
var pinLine = regexp.MustCompile(`^(\s*(?:require\s+)?` + regexp.QuoteMeta(zeverPrefix) + `\S+\s+)(v\S+)(\s*(?://.*)?)$`)

// releaseVersion returns the version in cmd/zever/VERSION with a leading "v".
func releaseVersion(root string) (string, error) {
	b, err := os.ReadFile(filepath.Join(root, "cmd", "zever", "VERSION"))
	if err != nil {
		return "", errors.Join(ErrVersionFile, err)
	}
	v := strings.TrimSpace(string(b))
	if !regexp.MustCompile(`^v?\d+\.\d+\.\d+$`).MatchString(v) {
		return "", fmt.Errorf("%w: want X.Y.Z, got %q", ErrVersionFile, v)
	}
	return "v" + strings.TrimPrefix(v, "v"), nil
}

// rewritePins sets every intra-repo require in a go.mod's text to ver and
// returns the new text plus the number of lines changed. Replace directives
// and external requires are left untouched.
func rewritePins(text, ver string) (string, int) {
	lines := strings.Split(text, "\n")
	changed := 0
	for i, l := range lines {
		m := pinLine.FindStringSubmatch(l)
		if m == nil || m[2] == ver {
			continue
		}
		lines[i] = m[1] + ver + m[3]
		changed++
	}
	return strings.Join(lines, "\n"), changed
}

// stalePins lists the intra-repo requires in a go.mod's text whose version
// differs from ver, formatted "<module> <found>".
func stalePins(text, ver string) []string {
	var out []string
	for _, l := range strings.Split(text, "\n") {
		m := pinLine.FindStringSubmatch(l)
		if m == nil || m[2] == ver {
			continue
		}
		fields := strings.Fields(m[1])
		out = append(out, fields[len(fields)-1]+" "+m[2])
	}
	return out
}

// runPins implements --check-pins and --set-pins over every module.
func runPins(root string, mods map[string]string, ver string, set bool, stdout, stderr *os.File) int {
	if ver == "" {
		v, err := releaseVersion(root)
		if err != nil {
			fmt.Fprintf(stderr, "modgraph: %v\n", err)
			return 1
		}
		ver = v
	}
	stale := 0
	for _, moddir := range sortedValues(mods) {
		path := filepath.Join(root, filepath.FromSlash(moddir), "go.mod")
		b, err := os.ReadFile(path)
		if err != nil {
			fmt.Fprintf(stderr, "modgraph: %v\n", err)
			return 1
		}
		if set {
			out, n := rewritePins(string(b), ver)
			if n == 0 {
				continue
			}
			if err := os.WriteFile(path, []byte(out), 0o644); err != nil { //nolint:gosec // go.mod is world-readable
				fmt.Fprintf(stderr, "modgraph: %v\n", err)
				return 1
			}
			stale += n
			continue
		}
		for _, s := range stalePins(string(b), ver) {
			fmt.Fprintf(stdout, "%s/go.mod: requires %s, want %s\n", moddir, s, ver)
			stale++
		}
	}
	if set {
		fmt.Fprintf(stdout, "modules: %d; rewrote %d intra-repo require(s) to %s\n", len(mods), stale, ver)
		return 0
	}
	if stale > 0 {
		fmt.Fprintf(stdout, "%d intra-repo require(s) differ from %s (run: make release-prep VERSION=%s)\n", stale, ver, ver)
		return 1
	}
	fmt.Fprintf(stdout, "modules: %d; every intra-repo require is %s\n", len(mods), ver)
	return 0
}

// sortedValues returns the sorted values of m.
func sortedValues(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for _, v := range m {
		out = append(out, v)
	}
	sort.Strings(out)
	return out
}
