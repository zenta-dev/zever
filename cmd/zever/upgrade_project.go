package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"golang.org/x/mod/semver"
)

// ErrUpgradeProjectNoGoMod is returned when upgrade --project runs outside a
// Go module.
var ErrUpgradeProjectNoGoMod = errors.New("zever: upgrade --project needs go.mod")

// ErrUpgradeProjectNoRequires is returned when the project go.mod has no
// github.com/zenta-dev/zever/* requires to bump.
var ErrUpgradeProjectNoRequires = errors.New("zever: upgrade --project found no zever requires")

// zeverModulePrefix marks requires owned by this framework in a project go.mod.
const zeverModulePrefix = "github.com/zenta-dev/zever/"

// projectUpgradeOptions carries every input runUpgradeProjectWith needs. Dir
// defaults to ".", fetch defaults to fetchUpgradeURL, tidy defaults to
// tidyProjectGoMod, and cacheDir enables the offline release-notes cache
// (empty disables it, which tests use to stay hermetic).
type projectUpgradeOptions struct {
	dir      string
	target   string
	check    bool
	dryRun   bool
	json     bool
	out      io.Writer
	fetch    func(ctx context.Context, url string) ([]byte, error)
	tidy     func(ctx context.Context, dir string) error
	cacheDir string
}

// bumpProjectGoMod rewrites every zever require in data to target and reports
// how many it bumped. Third-party requires are left untouched.
func bumpProjectGoMod(data []byte, target string) ([]byte, int, error) {
	tag := ensureUpgradeTag(target)
	if !semver.IsValid(tag) {
		return nil, 0, fmt.Errorf("%w: %q", ErrUpgradeVersion, target)
	}

	var out bytes.Buffer

	// Requires pinned to a local path by a matching replace directive
	// stay untouched: bumping them would fight the developer's checkout.
	replaced := map[string]bool{}
	for line := range strings.Lines(string(data)) {
		trimmed := strings.TrimSpace(line)
		code, _, _ := strings.Cut(trimmed, "//")
		fields := strings.Fields(code)
		if len(fields) == 0 || fields[0] != "replace" {
			continue
		}

		for _, f := range fields[1:] {
			if strings.HasPrefix(f, zeverModulePrefix) {
				replaced[strings.TrimSuffix(f, "/")] = true
			}
		}
	}

	n := 0

	for line := range strings.Lines(string(data)) {
		trimmed := strings.TrimSpace(line)
		if strings.Contains(trimmed, zeverModulePrefix) && !strings.HasPrefix(trimmed, "//") {
			// Split off any trailing comment first so it is neither
			// parsed as a version nor duplicated on rebuild.
			code, comment, _ := strings.Cut(line, "//")
			fields := strings.Fields(code)
			bumped := false
			for i, f := range fields {
				if strings.HasPrefix(f, zeverModulePrefix) && i+1 < len(fields) && semver.IsValid(ensureUpgradeTag(fields[i+1])) && !replaced[strings.TrimSuffix(f, "/")] {
					fields[i+1] = tag
					n++
					bumped = true

					break
				}
			}
			if !bumped {
				out.WriteString(line)

				continue
			}
			// Rebuilding from fields collapses interior whitespace,
			// which gofmt/go mod tidy normalizes anyway on the
			// following tidy run.
			indent := line[:len(line)-len(strings.TrimLeft(line, " \t"))]
			eol := ""
			if strings.HasSuffix(line, "\n") {
				eol = "\n"
			}
			suffix := ""
			if comment != "" {
				suffix = " //" + comment
			}
			out.WriteString(indent + strings.Join(fields, " ") + suffix + eol)

			continue
		}

		out.WriteString(line)
	}

	if n == 0 {
		return nil, 0, ErrUpgradeProjectNoRequires
	}

	return out.Bytes(), n, nil
}

// projectCurrentVersion returns the first zever require version in data, or
// "" when there is none.
func projectCurrentVersion(data []byte) string {
	for line := range strings.Lines(string(data)) {
		trimmed := strings.TrimSpace(line)
		if !strings.Contains(trimmed, zeverModulePrefix) || strings.HasPrefix(trimmed, "//") {
			continue
		}

		fields := strings.Fields(trimmed)
		for i, f := range fields {
			if strings.HasPrefix(f, zeverModulePrefix) && i+1 < len(fields) && semver.IsValid(ensureUpgradeTag(fields[i+1])) {
				return ensureUpgradeTag(fields[i+1])
			}
		}
	}

	return ""
}

// projectBreakingNotes extracts **Breaking:** entries from changelog sections
// newer than from and up to and including to. Headers look like "## [0.7.0]"
// or "## 0.7.0".
func projectBreakingNotes(changelog []byte, from, to string) []string {
	lo := ensureUpgradeTag(from)
	hi := ensureUpgradeTag(to)

	var notes []string

	inRange := false

	for line := range strings.Lines(string(changelog)) {
		trimmed := strings.TrimSpace(line)
		if v, ok := strings.CutPrefix(trimmed, "## "); ok {
			v = strings.TrimSpace(strings.Trim(strings.TrimSpace(v), "[]()"))
			fields := strings.Fields(v)
			if len(fields) > 0 {
				v = fields[0]
			}

			tag := ensureUpgradeTag(strings.TrimSpace(v))
			if semver.IsValid(tag) && semver.IsValid(lo) && semver.IsValid(hi) {
				inRange = semver.Compare(tag, lo) > 0 && semver.Compare(tag, hi) <= 0
			} else {
				inRange = false
			}

			continue
		}

		if inRange && strings.Contains(line, "**Breaking:**") {
			notes = append(notes, strings.TrimSpace(line))
		}
	}

	return notes
}

// projectChangelogURL builds the raw CHANGELOG.md URL for a release tag.
func projectChangelogURL(tag string) string {
	return "https://raw.githubusercontent.com/zenta-dev/zever/" + tag + "/CHANGELOG.md"
}

// loadCachedOrFetch returns the cached changelog for tag when cacheDir holds
// one, else fetches it and caches the result best-effort.
func loadCachedOrFetch(ctx context.Context, fetch func(context.Context, string) ([]byte, error), cacheDir, tag string) []byte {
	if cacheDir != "" {
		path := filepath.Join(cacheDir, "zever-changelog-"+tag+".md")
		if data, err := os.ReadFile(path); err == nil {
			return data
		}

		data, err := fetch(ctx, projectChangelogURL(tag))
		if err != nil {
			return nil
		}

		_ = os.MkdirAll(cacheDir, 0o750)
		_ = os.WriteFile(path, data, 0o600)

		return data
	}

	data, err := fetch(ctx, projectChangelogURL(tag))
	if err != nil {
		return nil
	}

	return data
}

// tidyProjectGoMod runs `go mod tidy` in dir.
func tidyProjectGoMod(ctx context.Context, dir string) error {
	cmd := exec.CommandContext(ctx, "go", "mod", "tidy")
	cmd.Dir = dir

	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("zever upgrade: go mod tidy: %w\n%s", err, out)
	}

	return nil
}

// runUpgradeProjectWith bumps every zever require in dir/go.mod to the target
// version, runs go mod tidy, and prints **Breaking:** entries between the
// installed and target versions. --dry-run prints the plan without writing
// anything, fetching nothing, or tidying.
func runUpgradeProjectWith(ctx context.Context, opts projectUpgradeOptions) error {
	out := outOrStdout(opts.out)

	dir := opts.dir
	if dir == "" {
		dir = "."
	}

	data, err := os.ReadFile(filepath.Join(dir, "go.mod"))
	if err != nil {
		if os.IsNotExist(err) {
			return ErrUpgradeProjectNoGoMod
		}

		return fmt.Errorf("zever upgrade: read go.mod: %w", err)
	}

	fetch := opts.fetch
	if fetch == nil {
		fetch = fetchUpgradeURL
	}

	tidy := opts.tidy
	if tidy == nil {
		tidy = tidyProjectGoMod
	}

	tag := ensureUpgradeTag(opts.target)
	if strings.TrimSpace(opts.target) == "" {
		body, ferr := fetch(ctx, upgradeLatestAPIURL)
		if ferr != nil {
			return fmt.Errorf("zever upgrade: fetch latest release: %w", ferr)
		}

		release, perr := parseUpgradeRelease(body)
		if perr != nil {
			return perr
		}

		tag = ensureUpgradeTag(release.tag)
	}

	if !semver.IsValid(tag) {
		return fmt.Errorf("%w: %q", ErrUpgradeVersion, opts.target)
	}

	current := projectCurrentVersion(data)
	if current == "" {
		return ErrUpgradeProjectNoRequires
	}

	if semver.Compare(current, tag) == 0 {
		if opts.json {
			emitSuccess("upgrade", map[string]any{
				"current": current,
				"target":  tag,
				"action":  upgradeActionCurrent,
				"project": true,
			})
		} else {
			_, _ = fmt.Fprintln(out, "already current: "+tag)
		}

		return nil
	}

	if opts.check {
		if opts.json {
			emitSuccess("upgrade", map[string]any{
				"current":   current,
				"available": tag,
				"check":     true,
				"project":   true,
			})
		} else {
			_, _ = fmt.Fprintf(out, "%s available (installed %s)\n", tag, current)
		}

		return nil
	}

	bumped, n, berr := bumpProjectGoMod(data, tag)
	if berr != nil {
		return berr
	}

	if opts.dryRun {
		_, _ = fmt.Fprintln(out, "Upgrade plan (project go.mod):")
		_, _ = fmt.Fprintf(out, "  bump %d zever requires: %s -> %s\n", n, current, tag)
		_, _ = fmt.Fprintln(out, "  run go mod tidy")

		if opts.json {
			emitSuccess("upgrade", map[string]any{
				"current":  current,
				"target":   tag,
				"requires": n,
				"dryRun":   true,
				"project":  true,
			})
		}

		return nil
	}

	notes := []string{}
	if raw := loadCachedOrFetch(ctx, fetch, opts.cacheDir, tag); raw != nil {
		notes = projectBreakingNotes(raw, current, tag)
	}

	if werr := os.WriteFile(filepath.Join(dir, "go.mod"), bumped, 0o644); werr != nil { //nolint:gosec // developer project file
		return fmt.Errorf("zever upgrade: write go.mod: %w", werr)
	}

	if terr := tidy(ctx, dir); terr != nil {
		return terr
	}

	if opts.json {
		emitSuccess("upgrade", map[string]any{
			"current":  current,
			"target":   tag,
			"requires": n,
			"breaking": notes,
			"project":  true,
		})

		return nil
	}

	_, _ = fmt.Fprintf(out, "upgraded %d zever requires: %s -> %s\n", n, current, tag)

	if len(notes) == 0 {
		_, _ = fmt.Fprintln(out, "no breaking changes noted")
	} else {
		_, _ = fmt.Fprintln(out, "Breaking changes:")
		for _, note := range notes {
			_, _ = fmt.Fprintln(out, "  "+note)
		}
	}

	return nil
}
