package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"golang.org/x/exp/apidiff"
	"golang.org/x/mod/modfile"
	"golang.org/x/tools/go/packages"
)

// coreModules is the Core-tier API surface the gate protects: every module
// whose exported API other modules and consumer projects compile against.
// Adapter modules are excluded by design: adapters register behind the
// core/<battery> interfaces, so their API is not a compatibility surface.
var coreModules = []string{
	"config",
	"container",
	"core/auth",
	"core/cache",
	"core/crypto",
	"core/db",
	"core/password",
	"core/queue",
	"core/router",
	"core/secrets",
	"core/session",
	"dsl",
	"orm",
}

func run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("apidiff", flag.ContinueOnError)
	fs.SetOutput(stderr)
	rootFlag := fs.String("root", "", "repo root (default: git toplevel of cwd)")
	modulesFlag := fs.String("modules", "", "comma-separated module dirs (default: Core tier)")
	changelogFlag := fs.String("changelog", "", "changelog path (default: <root>/CHANGELOG.md)")
	allowFlag := fs.String("allowlist", "", "no-baseline allowlist JSON (default: <root>/.github/apidiff-baseline-allowlist.json)")
	checkPatchFlag := fs.Bool("check-patch", false, "fail when a released patch section carries a **Breaking:** entry (no git needed)")
	floorFlag := fs.String("floor", "", "grandfather patch sections at or below this version (default: <root>/cmd/zever/VERSION)")
	matrixFlag := fs.String("compat-matrix", "", "write the generated compatibility-matrix doc to FILE and exit")
	checkMatrixFlag := fs.String("check-compat-matrix", "", "exit 1 when FILE differs from the generated compatibility-matrix doc")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	root := *rootFlag
	if root == "" {
		var err error
		root, err = gitTopLevel(ctx)
		if err != nil {
			fmt.Fprintf(stderr, "apidiff: git rev-parse --show-toplevel: %v\n", err)
			return 1
		}
	}
	modules := coreModules
	if *modulesFlag != "" {
		modules = strings.Split(*modulesFlag, ",")
	}
	changelog := *changelogFlag
	if changelog == "" {
		changelog = filepath.Join(root, "CHANGELOG.md")
	}

	if *matrixFlag != "" {
		return runCompatMatrix(*matrixFlag, stdout)
	}
	if *checkMatrixFlag != "" {
		return runCheckCompatMatrix(*checkMatrixFlag, stdout)
	}
	if *checkPatchFlag {
		floor := *floorFlag
		if floor == "" {
			var err error
			floor, err = versionFloor(filepath.Join(root, "cmd", "zever", "VERSION"))
			if err != nil {
				fmt.Fprintf(stderr, "apidiff: read version floor: %v\n", err)
				return 1
			}
		}
		return runPatchGuard(changelog, floor, stdout, stderr)
	}

	allowPath := *allowFlag
	if allowPath == "" {
		allowPath = filepath.Join(root, ".github", "apidiff-baseline-allowlist.json")
	}
	allow, allowErr := loadBaselineAllowlist(allowPath)
	if allowErr != nil {
		fmt.Fprintf(stderr, "apidiff: %v\n", allowErr)
		return 1
	}

	type failure struct {
		module string
		tag    string
		report string
	}
	var failures []failure
	missing := 0
	for _, dir := range modules {
		dir = strings.TrimSpace(dir)
		tag, err := latestTag(ctx, root, dir)
		if err != nil {
			entry, ok := allow[dir]
			switch {
			case !ok:
				fmt.Fprintf(stdout, "apidiff: %s: no baseline tag %s/v*; Core-tier modules must be tagged "+
					"(or listed with an expiry in %s)\n", dir, dir, allowPath)
				missing++
			case entry.expired(now()):
				fmt.Fprintf(stdout, "apidiff: %s: no baseline tag and its allowlist entry expired on %s\n", dir, entry.Expires)
				missing++
			default:
				fmt.Fprintf(stdout, "apidiff: %s: no baseline tag; allowlisted until %s (%s)\n", dir, entry.Expires, entry.Reason)
			}
			continue
		}
		report, err := compareModule(ctx, root, dir, tag)
		if err != nil {
			fmt.Fprintf(stderr, "apidiff: %s: compare against %s: %v\n", dir, tag, err)
			return 1
		}
		if report != "" {
			failures = append(failures, failure{module: dir, tag: tag, report: report})
		} else {
			fmt.Fprintf(stdout, "apidiff: %s compatible with %s\n", dir, tag)
		}
	}

	if len(failures) == 0 {
		if missing > 0 {
			fmt.Fprintf(stdout, "apidiff: %d Core-tier module(s) without a usable baseline tag\n", missing)
			return 1
		}
		fmt.Fprintf(stdout, "apidiff: all checked modules compatible\n")
		return 0
	}

	for _, f := range failures {
		fmt.Fprintf(stdout, "apidiff: %s breaks %s:\n%s\n", f.module, f.tag, f.report)
	}
	raw, err := os.ReadFile(changelog)
	if err != nil {
		fmt.Fprintf(stderr, "apidiff: read %s: %v\n", changelog, err)
		return 1
	}
	raw = appendChangelogFragments(raw, filepath.Join(filepath.Dir(changelog), ".changelog", "unreleased"))
	body := string(raw)
	// The in-development version names the release actually being prepared:
	// only that untagged top section counts as pending, so an untagged
	// section of any other version cannot absorb a new break.
	currentVersion, err := versionFloor(filepath.Join(root, "cmd", "zever", "VERSION"))
	if err != nil {
		fmt.Fprintf(stdout, "apidiff: no cmd/zever/VERSION: only %s documents a pending break\n", unreleasedHeader)
	}
	pending := pendingHeaders(body, func(version string) bool { return tagExists(ctx, root, version) }, currentVersion)
	label := pendingLabel(pending)
	entries := breakingEntries(body, pending)
	uncovered := 0
	for _, f := range failures {
		mod, merr := goModPath(filepath.Join(root, f.module, "go.mod"))
		if merr != nil {
			fmt.Fprintf(stderr, "apidiff: %v\n", merr)
			return 1
		}
		if !anyCovers(entries, f.module, mod) {
			fmt.Fprintf(stdout, "apidiff: %s: incompatible changes need a **Breaking:** entry under %s in %s "+
				"naming the module (`%s`), a package under it or its short name (`%s`) in backticks\n",
				f.module, label, changelog, f.module, path.Base(f.module))
			uncovered++
		}
	}
	if uncovered > 0 || missing > 0 {
		return 1
	}
	fmt.Fprintf(stdout, "apidiff: incompatible changes documented under %s\n", label)
	return 0
}

// now is the clock used for allowlist expiry; tests replace it.
var now = time.Now

// baselineEntry is one allowlist record exempting a Core-tier module from the
// "must have a baseline tag" rule until Expires.
type baselineEntry struct {
	// Module is the module directory relative to the repo root.
	Module string `json:"module"`
	// Reason explains why no baseline tag exists yet.
	Reason string `json:"reason"`
	// Expires is the last day (YYYY-MM-DD, UTC) the exemption holds.
	Expires string `json:"expires"`
}

// expired reports whether the exemption has lapsed at t.
func (e baselineEntry) expired(t time.Time) bool {
	d, err := time.Parse("2006-01-02", e.Expires)
	if err != nil {
		return true
	}
	return t.UTC().After(d.AddDate(0, 0, 1))
}

// loadBaselineAllowlist reads the allowlist file ({"allowlist": [...]}); a
// missing file is an empty list. Every entry needs a module, a reason and a
// valid expiry date.
func loadBaselineAllowlist(file string) (map[string]baselineEntry, error) {
	raw, err := os.ReadFile(file)
	if errors.Is(err, os.ErrNotExist) {
		return map[string]baselineEntry{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read allowlist: %w", err)
	}
	var doc struct {
		Allowlist []baselineEntry `json:"allowlist"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("parse allowlist %s: %w", file, err)
	}
	out := make(map[string]baselineEntry, len(doc.Allowlist))
	for _, e := range doc.Allowlist {
		if e.Module == "" || e.Reason == "" {
			return nil, fmt.Errorf("allowlist %s: entry needs module and reason: %+v", file, e)
		}
		if _, perr := time.Parse("2006-01-02", e.Expires); perr != nil {
			return nil, fmt.Errorf("allowlist %s: %s: expires must be YYYY-MM-DD: %w", file, e.Module, perr)
		}
		out[e.Module] = e
	}
	return out, nil
}

func gitTopLevel(ctx context.Context) (string, error) {
	out, err := exec.CommandContext(ctx, "git", "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// latestTag returns the highest <dir>/v* tag, e.g. "core/cache/v0.6.1".
func latestTag(ctx context.Context, root, dir string) (string, error) {
	out, err := exec.CommandContext(ctx, "git", "-C", root, "tag", "--list", dir+"/v*", "--sort=-v:refname").Output()
	if err != nil {
		return "", err
	}
	line, _, _ := strings.Cut(string(out), "\n")
	if line == "" {
		return "", fmt.Errorf("apidiff: no tag matching %s/v*", dir)
	}
	return line, nil
}

// compareModule diffs the exported API of dir at tag against the working
// tree and returns the rendered incompatible-changes report (empty when
// compatible).
func compareModule(ctx context.Context, root, dir, tag string) (string, error) {
	base, err := os.MkdirTemp("", "apidiff-base-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(base)

	archive, err := exec.CommandContext(ctx, "git", "-C", root, "archive", tag).Output()
	if err != nil {
		return "", fmt.Errorf("git archive %s: %w", tag, err)
	}
	untar := exec.CommandContext(ctx, "tar", "-x", "-C", base)
	untar.Stdin = strings.NewReader(string(archive))
	if out, untarErr := untar.CombinedOutput(); untarErr != nil {
		return "", fmt.Errorf("extract %s\n%s: %w", tag, out, untarErr)
	}

	importPath, err := goModPath(filepath.Join(root, dir, "go.mod"))
	if err != nil {
		return "", err
	}
	oldMod, err := loadModule(filepath.Join(base, dir), importPath)
	if err != nil {
		return "", fmt.Errorf("load base %s: %w", tag, err)
	}
	newMod, err := loadModule(filepath.Join(root, dir), importPath)
	if err != nil {
		return "", fmt.Errorf("load working tree: %w", err)
	}
	report := apidiff.ModuleChanges(oldMod, newMod)
	var sb strings.Builder
	if err := report.TextIncompatible(&sb, true); err != nil {
		return "", err
	}
	return sb.String(), nil
}

// goModPath reads the module path from a go.mod file.
func goModPath(goMod string) (string, error) {
	raw, err := os.ReadFile(goMod)
	if err != nil {
		return "", err
	}
	f, err := modfile.Parse(goMod, raw, nil)
	if err != nil {
		return "", err
	}
	if f.Module == nil || f.Module.Mod.Path == "" {
		return "", fmt.Errorf("apidiff: no module line in %s", goMod)
	}
	return f.Module.Mod.Path, nil
}

// loadModule type-checks every non-internal package of importPath rooted at
// dir (a module dir, loaded in module mode so workspace replaces never leak
// the other side of the comparison in).
func loadModule(dir, importPath string) (*apidiff.Module, error) {
	cfg := &packages.Config{
		Mode: packages.NeedName | packages.NeedTypes | packages.NeedModule,
		Dir:  dir,
		Env:  append(os.Environ(), "GOWORK=off"),
	}
	loaded, err := packages.Load(cfg, importPath+"/...")
	if err != nil {
		return nil, err
	}
	if len(loaded) == 0 {
		return nil, fmt.Errorf("apidiff: found no packages for module %s", importPath)
	}
	var pkgs []*packages.Package
	var errs []error
	for _, p := range loaded {
		if len(p.Errors) > 0 {
			for _, e := range p.Errors {
				errs = append(errs, e)
			}
			continue
		}
		if isInternalPackage(p.PkgPath, importPath) {
			continue
		}
		pkgs = append(pkgs, p)
	}
	if len(errs) > 0 {
		msgs := make([]string, 0, len(errs))
		for _, e := range errs {
			msgs = append(msgs, e.Error())
		}
		return nil, fmt.Errorf("load %s: %s", importPath, strings.Join(msgs, "\n"))
	}
	mod := &apidiff.Module{Path: importPath}
	for _, p := range pkgs {
		mod.Packages = append(mod.Packages, p.Types)
	}
	return mod, nil
}

func isInternalPackage(pkgPath, modulePath string) bool {
	rel := strings.TrimPrefix(pkgPath, modulePath)
	switch {
	case strings.HasSuffix(rel, "/internal"):
		return true
	case strings.Contains(rel, "/internal/"):
		return true
	case rel == "internal":
		return true
	case strings.HasPrefix(rel, "internal/"):
		return true
	}
	return false
}

// appendChangelogFragments appends every .md fragment in fragDir (except
// README.md) under a synthetic ## [Unreleased] header, so the pending-section
// scan in breakingEntries also covers entries carried by parallel PRs that
// have not been compiled into the changelog yet. Missing dir or no
// fragments leaves body unchanged. Fragment entries must already satisfy
// the changelog-lint rules; this function does not re-lint them.
func appendChangelogFragments(body []byte, fragDir string) []byte {
	matches, err := filepath.Glob(filepath.Join(fragDir, "*.md"))
	if err != nil || len(matches) == 0 {
		return body
	}
	var frags []string
	for _, m := range matches {
		if filepath.Base(m) == "README.md" {
			continue
		}
		frag, err := os.ReadFile(m)
		if err != nil {
			continue
		}
		if len(strings.TrimSpace(string(frag))) == 0 {
			continue
		}
		frags = append(frags, m)
	}
	if len(frags) == 0 {
		return body
	}
	sort.Strings(frags)
	var sb strings.Builder
	sb.Write(body)
	if len(sb.String()) > 0 && !strings.HasSuffix(sb.String(), "\n") {
		sb.WriteByte('\n')
	}
	sb.WriteString("\n## [Unreleased]\n")
	for _, m := range frags {
		frag, err := os.ReadFile(m)
		if err != nil {
			continue
		}
		sb.WriteString("\n")
		sb.Write(bytes.TrimSpace(frag))
		sb.WriteString("\n")
	}
	return []byte(sb.String())
}

// unreleasedHeader is the Keep a Changelog section for changes not yet cut
// into a release.
const unreleasedHeader = "## [Unreleased]"

// releaseHeader matches a released section header and captures its version,
// e.g. "## [v0.8.0] - 2026-10-18" or "## [0.8.0]".
var releaseHeader = regexp.MustCompile(`^## \[[vV]?(\d+\.\d+\.\d+)[^]]*\]`)

// sectionHeaders returns every "## ..." header line in body, trimmed, in
// document order.
func sectionHeaders(body string) []string {
	var out []string
	for _, line := range strings.Split(body, "\n") {
		if trimmed := strings.TrimSpace(line); strings.HasPrefix(trimmed, "## ") {
			out = append(out, trimmed)
		}
	}
	return out
}

// pendingHeaders returns the section headers whose entries document changes
// that are not released yet, so a **Breaking:** entry there covers an
// incompatible change. ## [Unreleased] always counts. The top-most released
// section counts too when it is the release actually being prepared: its
// version equals the in-development version (cmd/zever/VERSION) and it
// carries no git tag yet. The release checklist cuts ## [Unreleased] to
// ## [vX.Y.Z] - DATE before the tags are created, and the API-diff baseline
// stays at the last existing tag, so a break documented in that
// cut-but-untagged section must still satisfy the gate. Matching the
// in-development version is what makes the rule precise: an untagged
// section of some other version is a release that is not being prepared,
// so it must not absorb a new break - the break belongs under
// ## [Unreleased]. Only the top-most released section is considered, so an
// old untagged section can never silently absorb a new break. tagged reports
// whether a version (e.g. "0.8.0") already has a git tag; nil means "assume
// untagged". currentVersion is the in-development version (cmd/zever/VERSION);
// empty means no released section can be pending.
func pendingHeaders(body string, tagged func(version string) bool, currentVersion string) []string {
	var out []string
	seenRelease := false
	seen := make(map[string]bool)
	for _, h := range sectionHeaders(body) {
		if seen[h] {
			continue // the same header twice (a fragment section) is one section
		}
		seen[h] = true
		if h == unreleasedHeader {
			out = append(out, h)
			continue
		}
		m := releaseHeader.FindStringSubmatch(h)
		if m == nil || seenRelease {
			continue
		}
		seenRelease = true
		if tagged != nil && tagged(m[1]) {
			continue
		}
		if !sameVersion(m[1], currentVersion) {
			continue
		}
		out = append(out, h)
	}
	return out
}

// sameVersion reports whether two version strings name the same release
// ("0.8.0" and "v0.8.0" are the same).
func sameVersion(a, b string) bool {
	norm := func(v string) string {
		return strings.TrimPrefix(strings.TrimSpace(v), "v")
	}
	return norm(a) == norm(b)
}

// pendingLabel renders the pending section headers for a gate message,
// falling back to ## [Unreleased] when the changelog has none.
func pendingLabel(headers []string) string {
	if len(headers) == 0 {
		return unreleasedHeader
	}
	return strings.Join(headers, " or ")
}

// tagExists reports whether version has been released: a historical bare
// "vX.Y.Z" tag or any lockstep per-module "<dir>/vX.Y.Z" tag. A git failure
// counts as untagged, so the check degrades to the pending-section rule
// instead of passing an undocumented break.
func tagExists(ctx context.Context, root, version string) bool {
	tag := "v" + strings.TrimPrefix(version, "v")
	out, err := exec.CommandContext(ctx, "git", "-C", root, "tag", "--list", tag, "*/"+tag).Output()
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(out)) != ""
}

// unreleasedHasBreaking reports whether body carries a **Breaking:** entry
// inside the ## [Unreleased] section (stopping at the next ## [x] header).
func unreleasedHasBreaking(body string) bool {
	return len(breakingEntries(body, []string{unreleasedHeader})) > 0
}

// breakingEntries returns the list entries (a "- " bullet plus its indented
// continuation lines) inside the given section headers that contain a
// **Breaking:** marker.
func breakingEntries(body string, headers []string) []string {
	want := make(map[string]bool, len(headers))
	for _, h := range headers {
		want[h] = true
	}
	var entries []string
	var cur []string
	flush := func() {
		if len(cur) == 0 {
			return
		}
		text := strings.Join(cur, "\n")
		if strings.Contains(text, "**Breaking:**") {
			entries = append(entries, text)
		}
		cur = nil
	}
	inSection := false
	for _, line := range strings.Split(body, "\n") {
		trimmed := strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(trimmed, "## "):
			flush()
			inSection = want[trimmed]
		case !inSection:
		case strings.HasPrefix(line, "- ") || strings.HasPrefix(line, "* "):
			flush()
			cur = []string{line}
		case trimmed == "" || strings.HasPrefix(trimmed, "#"):
			flush()
		case len(cur) > 0:
			cur = append(cur, line)
		default:
			// Free text outside a bullet still counts as its own entry.
			cur = append(make([]string, 0, 2), line)
		}
	}
	flush()
	return entries
}

// anyCovers reports whether some entry names module dir (import path mod).
func anyCovers(entries []string, dir, mod string) bool {
	for _, e := range entries {
		if entryCovers(e, dir, mod) {
			return true
		}
	}
	return false
}

// entryCovers implements the naming convention: a **Breaking:** entry covers
// module dir when it carries a backticked token that is the module import
// path, the module directory, a package path under either, or the short name
// (last directory element, optionally qualified as `short.Ident`). A
// trailing "/..." is ignored.
func entryCovers(entry, dir, mod string) bool {
	short := path.Base(dir)
	for _, tok := range backticked(entry) {
		tok = strings.TrimSuffix(tok, "/...")
		switch {
		case tok == mod, tok == dir, tok == short:
			return true
		case strings.HasPrefix(tok, mod+"/"), strings.HasPrefix(tok, dir+"/"):
			return true
		case strings.HasPrefix(tok, short+"."):
			return true
		}
	}
	return false
}

// backticked returns the inline-code spans of s.
func backticked(s string) []string {
	var out []string
	for {
		i := strings.IndexByte(s, '`')
		if i < 0 {
			return out
		}
		s = s[i+1:]
		j := strings.IndexByte(s, '`')
		if j < 0 {
			return out
		}
		out = append(out, s[:j])
		s = s[j+1:]
	}
}
