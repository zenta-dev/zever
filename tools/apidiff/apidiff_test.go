package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func gitPath(t *testing.T) string {
	t.Helper()
	p, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git binary not available")
	}
	return p
}

func mustGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	gitPath(t)
	cmd := exec.CommandContext(t.Context(), "git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=t",
		"GIT_AUTHOR_EMAIL=t@t",
		"GIT_COMMITTER_NAME=t",
		"GIT_COMMITTER_EMAIL=t@t",
		"GIT_CONFIG_NOSYSTEM=1",
		"HOME="+t.TempDir(),
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
}

// initRepo builds a scratch git repo with a versioned module dir and a
// CHANGELOG, returning the repo root.
func initRepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	mustGit(t, root, "init", "-q")
	mustWrite(t, filepath.Join(root, "mod", "go.mod"), "module example.com/acme/mod\n\ngo 1.27.2\n")
	mustWrite(t, filepath.Join(root, "mod", "api.go"), apiV1)
	mustWrite(t, filepath.Join(root, "CHANGELOG.md"), changelogClean)
	mustGit(t, root, "add", "-A")
	mustGit(t, root, "commit", "-qm", "init")
	return root
}

const changelogClean = `# Changelog

## [Unreleased]

### Added

- nothing breaking here

## [v0.1.0] - 2026-01-01
`

const apiV1 = `package api

// Hello greets by name.
func Hello(name string) string { return "hi " + name }

// World is a sample type.
type World struct{}

// Method answers.
func (World) Method(a int) int { return a }
`

func TestLatestTagPicksHighest(t *testing.T) {
	t.Parallel()
	root := initRepo(t)
	mustGit(t, root, "tag", "mod/v0.5.0")
	mustGit(t, root, "tag", "mod/v0.6.1")
	mustGit(t, root, "tag", "other/v9.9.9")

	got, err := latestTag(context.Background(), root, "mod")
	if err != nil {
		t.Fatalf("latestTag: %v", err)
	}
	if got != "mod/v0.6.1" {
		t.Errorf("latestTag = %q, want %q", got, "mod/v0.6.1")
	}
}

func TestLatestTagMissing(t *testing.T) {
	t.Parallel()
	root := initRepo(t)
	if _, err := latestTag(context.Background(), root, "mod"); err == nil {
		t.Error("latestTag with no tags: got nil error, want error")
	}
}

func TestUnreleasedHasBreaking(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		body string
		want bool
	}{
		{"empty", "", false},
		{"no unreleased", "# Changelog\n\n## [v1.0.0]\n\n- **Breaking:** old\n", false},
		{"marker in unreleased", "# Changelog\n\n## [Unreleased]\n\n- **Breaking:** drop X\n\n## [v1.0.0]\n", true},
		{"marker only in release", "# Changelog\n\n## [Unreleased]\n\n- nothing\n\n## [v1.0.0]\n\n- **Breaking:** old\n", false},
		{"marker no section header", "- **Breaking:** drop X\n", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := unreleasedHasBreaking(tc.body); got != tc.want {
				t.Errorf("unreleasedHasBreaking = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestAppendChangelogFragments(t *testing.T) {
	t.Parallel()
	base := "# Changelog\n\n## [Unreleased]\n\n- existing\n"
	t.Run("no dir unchanged", func(t *testing.T) {
		t.Parallel()
		got := appendChangelogFragments([]byte(base), filepath.Join(t.TempDir(), "missing"))
		if string(got) != base {
			t.Errorf("missing dir: body changed:\n%s", got)
		}
	})
	t.Run("fragments scanned as unreleased", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		mustWrite(t, filepath.Join(dir, "v080-a.md"), "- **Breaking:** `mod`: drop X\n")
		mustWrite(t, filepath.Join(dir, "README.md"), "# fragments\n")
		mustWrite(t, filepath.Join(dir, "empty.md"), "\n")
		got := appendChangelogFragments([]byte(base), dir)
		if !unreleasedHasBreaking(string(got)) {
			t.Errorf("fragment breaking entry not detected:\n%s", got)
		}
		if !strings.Contains(string(got), "- existing") {
			t.Errorf("original body lost:\n%s", got)
		}
	})
	t.Run("empty dir unchanged", func(t *testing.T) {
		t.Parallel()
		got := appendChangelogFragments([]byte(base), t.TempDir())
		if string(got) != base {
			t.Errorf("empty dir: body changed:\n%s", got)
		}
	})
}

func TestGatePassesOnCompatibleTree(t *testing.T) {
	root := initRepo(t)
	mustGit(t, root, "tag", "mod/v0.1.0")

	var stdout, stderr strings.Builder
	code := run(context.Background(), []string{"--root", root, "--modules", "mod"}, &stdout, &stderr)
	if code != 0 {
		t.Errorf("run compatible tree = exit %d, want 0\nstdout:\n%s\nstderr:\n%s", code, stdout.String(), stderr.String())
	}
}

// TestGateFailsOnDeliberateBreak is the P4-4 fixture: removing an exported
// symbol must fail the gate until a **Breaking:** line lands in [Unreleased].
func TestGateFailsOnDeliberateBreak(t *testing.T) {
	root := initRepo(t)
	mustGit(t, root, "tag", "mod/v0.1.0")

	const apiV2 = `package api

// World is a sample type.
type World struct{}

// Method answers with a widened signature.
func (World) Method(a int64) int64 { return a }
`
	mustWrite(t, filepath.Join(root, "mod", "api.go"), apiV2)
	mustGit(t, root, "add", "-A")
	mustGit(t, root, "commit", "-qm", "break api")

	var stdout, stderr strings.Builder
	code := run(context.Background(), []string{"--root", root, "--modules", "mod"}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("run broken tree = exit %d, want 1\nstdout:\n%s\nstderr:\n%s", code, stdout.String(), stderr.String())
	}
	if out := stdout.String() + stderr.String(); !strings.Contains(out, "Hello") {
		t.Errorf("gate output does not name removed symbol Hello:\n%s", out)
	}
	if out := stdout.String() + stderr.String(); !strings.Contains(out, "**Breaking:**") {
		t.Errorf("gate output does not ask for a **Breaking:** line:\n%s", out)
	}

	mustWrite(t, filepath.Join(root, "CHANGELOG.md"),
		changelogWith("- **Breaking:** `mod` drops `api.Hello` and widens `api.World.Method`"))
	stdout.Reset()
	stderr.Reset()
	code = run(context.Background(), []string{"--root", root, "--modules", "mod"}, &stdout, &stderr)
	if code != 0 {
		t.Errorf("run broken tree with Breaking trailer = exit %d, want 0\nstdout:\n%s\nstderr:\n%s", code, stdout.String(), stderr.String())
	}
}

const breakAPI = `package api

// World is a sample type.
type World struct{}

// Method answers with a widened signature.
func (World) Method(a int64) int64 { return a }
`

func breakingRepo(t *testing.T) string {
	t.Helper()
	root := initRepo(t)
	mustGit(t, root, "tag", "mod/v0.1.0")
	mustWrite(t, filepath.Join(root, "mod", "api.go"), breakAPI)
	mustGit(t, root, "add", "-A")
	mustGit(t, root, "commit", "-qm", "break api")
	return root
}

func changelogWith(entry string) string {
	return "# Changelog\n\n## [Unreleased]\n\n### Breaking\n\n" + entry + "\n\n## [v0.1.0] - 2026-01-01\n"
}

func runGate(t *testing.T, root string, extra ...string) (int, string) {
	t.Helper()
	var stdout, stderr strings.Builder
	args := append([]string{"--root", root, "--modules", "mod"}, extra...)
	code := run(context.Background(), args, &stdout, &stderr)
	return code, stdout.String() + stderr.String()
}

func TestGateBreakCoveredOnlyByUnrelatedEntryFails(t *testing.T) {
	root := breakingRepo(t)
	mustWrite(t, filepath.Join(root, "CHANGELOG.md"),
		changelogWith("- **Breaking:** `other/pkg` drops `Foo` and the `payment` adapter changes"))
	code, out := runGate(t, root)
	if code != 1 {
		t.Fatalf("exit %d, want 1\n%s", code, out)
	}
	if !strings.Contains(out, "naming the module") {
		t.Errorf("output does not explain the naming rule:\n%s", out)
	}
}

func TestGateBreakCoveredByNamingEntryPasses(t *testing.T) {
	for _, entry := range []string{
		"- **Breaking:** `mod` drops Hello",
		"- **Breaking:** `example.com/acme/mod` drops Hello",
		"- **Breaking:** `example.com/acme/mod/sub` drops Hello",
		"- fix(mod)!: drop\n  **Breaking:** `mod.Hello` removed",
	} {
		root := breakingRepo(t)
		mustWrite(t, filepath.Join(root, "CHANGELOG.md"), changelogWith(entry))
		if code, out := runGate(t, root); code != 0 {
			t.Errorf("entry %q: exit %d, want 0\n%s", entry, code, out)
		}
	}
}

func TestGateNoBaselineTagFailsUnlessAllowlisted(t *testing.T) {
	root := initRepo(t) // no tag
	code, out := runGate(t, root)
	if code != 1 || !strings.Contains(out, "no baseline tag") {
		t.Fatalf("untagged: exit %d, want 1 with message\n%s", code, out)
	}

	allow := filepath.Join(root, "allow.json")
	mustWrite(t, allow, `{"allowlist":[{"module":"mod","reason":"first release","expires":"2099-01-01"}]}`)
	if c, o := runGate(t, root, "--allowlist", allow); c != 0 {
		t.Errorf("allowlisted: exit %d, want 0\n%s", c, o)
	}

	old := now
	t.Cleanup(func() { now = old })
	now = func() time.Time { return time.Date(2100, 1, 1, 0, 0, 0, 0, time.UTC) }
	code, out = runGate(t, root, "--allowlist", allow)
	if code != 1 || !strings.Contains(out, "expired") {
		t.Errorf("expired allowlist: exit %d, want 1 with 'expired'\n%s", code, out)
	}
}

func TestLoadBaselineAllowlistValidation(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	for name, body := range map[string]string{
		"noreason": `{"allowlist":[{"module":"a","expires":"2099-01-01"}]}`,
		"baddate":  `{"allowlist":[{"module":"a","reason":"r","expires":"soon"}]}`,
		"badjson":  `{`,
	} {
		f := filepath.Join(dir, name)
		mustWrite(t, f, body)
		if _, err := loadBaselineAllowlist(f); err == nil {
			t.Errorf("%s: want error", name)
		}
	}
	if m, err := loadBaselineAllowlist(filepath.Join(dir, "missing")); err != nil || len(m) != 0 {
		t.Errorf("missing file: got %v, %v; want empty, nil", m, err)
	}
}

func TestEntryCovers(t *testing.T) {
	t.Parallel()
	const mod = "github.com/zenta-dev/zever/core/auth"
	tests := []struct {
		entry string
		want  bool
	}{
		{"**Breaking:** `core/auth` x", true},
		{"**Breaking:** `auth` x", true},
		{"**Breaking:** `auth.JWTOptions` x", true},
		{"**Breaking:** `" + mod + "/jwt` x", true},
		{"**Breaking:** `core/auth/jwt/...` x", true},
		{"**Breaking:** `core/authz` x", false},
		{"**Breaking:** auth without backticks", false},
		{"**Breaking:** `orm` x", false},
	}
	for _, tc := range tests {
		if got := entryCovers(tc.entry, "core/auth", mod); got != tc.want {
			t.Errorf("entryCovers(%q) = %v, want %v", tc.entry, got, tc.want)
		}
	}
}

func TestPendingHeaders(t *testing.T) {
	t.Parallel()
	allTagged := func(string) bool { return true }
	noneTagged := func(string) bool { return false }
	upTo072 := func(v string) bool { return v == "0.7.2" }
	tests := []struct {
		name    string
		body    string
		tagged  func(string) bool
		current string
		want    []string
	}{
		{
			name:   "unreleased only",
			body:   "# Changelog\n\n## [Unreleased]\n\n- x\n",
			tagged: upTo072,
			want:   []string{"## [Unreleased]"},
		},
		{
			name:    "untagged top release is pending when it is the release being prepared",
			body:    "# Changelog\n\n## [v0.8.0] - 2026-10-18\n\n- x\n\n## [v0.7.2] - 2026-10-17\n\n- y\n",
			tagged:  upTo072,
			current: "0.8.0",
			want:    []string{"## [v0.8.0] - 2026-10-18"},
		},
		{
			name:    "untagged top release of another version is not pending",
			body:    "# Changelog\n\n## [v0.8.0] - 2026-10-18\n\n- x\n\n## [v0.7.2] - 2026-10-17\n\n- y\n",
			tagged:  upTo072,
			current: "0.9.0",
			want:    nil,
		},
		{
			name:    "untagged top release of another version needs an Unreleased section",
			body:    "# Changelog\n\n## [v0.8.0] - 2026-10-18\n\n- x\n\n## [Unreleased]\n\n- z\n",
			tagged:  upTo072,
			current: "0.9.0",
			want:    []string{"## [Unreleased]"},
		},
		{
			name:   "tagged top release is not pending",
			body:   "# Changelog\n\n## [v0.7.2] - 2026-10-17\n\n- y\n",
			tagged: upTo072,
			want:   nil,
		},
		{
			name:    "unreleased plus untagged top",
			body:    "# Changelog\n\n## [v0.8.0] - 2026-10-18\n\n- x\n\n## [Unreleased]\n\n- z\n",
			tagged:  upTo072,
			current: "0.8.0",
			want:    []string{"## [v0.8.0] - 2026-10-18", "## [Unreleased]"},
		},
		{
			name:    "only the top release is considered",
			body:    "# Changelog\n\n## [Unreleased]\n\n- x\n\n## [v0.7.2] - 2026-10-17\n\n- y\n\n## [v0.6.0] - 2026-01-01\n\n- z\n",
			tagged:  noneTagged,
			current: "0.7.2",
			want:    []string{"## [Unreleased]", "## [v0.7.2] - 2026-10-17"},
		},
		{
			name:    "nil tagged assumes untagged",
			body:    "# Changelog\n\n## [v0.1.0] - 2026-01-01\n\n- x\n",
			tagged:  nil,
			current: "0.1.0",
			want:    []string{"## [v0.1.0] - 2026-01-01"},
		},
		{
			name:    "no in-development version leaves no released section pending",
			body:    "# Changelog\n\n## [v0.1.0] - 2026-01-01\n\n- x\n",
			tagged:  nil,
			current: "",
			want:    nil,
		},
		{
			name:   "everything tagged",
			body:   "# Changelog\n\n## [v0.7.2] - 2026-10-17\n\n- y\n",
			tagged: allTagged,
			want:   nil,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := pendingHeaders(tc.body, tc.tagged, tc.current)
			if strings.Join(got, "|") != strings.Join(tc.want, "|") {
				t.Errorf("pendingHeaders = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestPendingLabel(t *testing.T) {
	t.Parallel()
	if got := pendingLabel(nil); got != "## [Unreleased]" {
		t.Errorf("pendingLabel(nil) = %q, want ## [Unreleased]", got)
	}
	want := "## [v0.8.0] - 2026-10-18 or ## [Unreleased]"
	if got := pendingLabel([]string{"## [v0.8.0] - 2026-10-18", "## [Unreleased]"}); got != want {
		t.Errorf("pendingLabel = %q, want %q", got, want)
	}
}

func TestTagExists(t *testing.T) {
	t.Parallel()
	root := initRepo(t)
	mustGit(t, root, "tag", "mod/v0.1.0")
	mustGit(t, root, "tag", "v0.2.0")
	ctx := context.Background()
	for _, tc := range []struct {
		version string
		want    bool
	}{
		{"0.1.0", true},  // lockstep per-module tag
		{"v0.2.0", true}, // historical bare tag
		{"0.9.9", false},
	} {
		if got := tagExists(ctx, root, tc.version); got != tc.want {
			t.Errorf("tagExists(%q) = %v, want %v", tc.version, got, tc.want)
		}
	}
}

// untaggedReleaseRepo tags mod/v0.1.0, breaks the exported API, and writes a
// CHANGELOG whose top section is a cut-but-untagged release (## [v0.2.0])
// carrying body as its entries (no ## [Unreleased]). cmd/zever/VERSION holds
// the in-development version, so the cut section is the release actually
// being prepared and still counts as pending.
func untaggedReleaseRepo(t *testing.T, body string) string {
	t.Helper()
	root := breakingRepo(t)
	mustWrite(t, filepath.Join(root, "CHANGELOG.md"),
		"# Changelog\n\n## [v0.2.0] - 2026-10-20\n\n"+body+"\n")
	mustWrite(t, filepath.Join(root, "cmd", "zever", "VERSION"), "0.2.0\n")
	return root
}

// TestGateUntaggedTopSectionCoversBreak is fixture (a): the changelog's top
// section is a cut release whose tag does not exist yet, and its
// **Breaking:** entry covers the incompatible change.
func TestGateUntaggedTopSectionCoversBreak(t *testing.T) {
	root := untaggedReleaseRepo(t, "- **Breaking:** `mod` drops `api.Hello` and widens `api.World.Method`")
	if code, out := runGate(t, root); code != 0 {
		t.Errorf("exit %d, want 0 (untagged top section documents the break)\n%s", code, out)
	}
}

// TestGateUntaggedTopSectionWithoutEntryFails is fixture (b): the cut but
// untagged section exists, but no **Breaking:** entry documents the change,
// so the gate must fail and point at that section.
func TestGateUntaggedTopSectionWithoutEntryFails(t *testing.T) {
	root := untaggedReleaseRepo(t, "- a compatible-looking change")
	code, out := runGate(t, root)
	if code == 0 {
		t.Fatalf("exit 0, want non-zero\n%s", out)
	}
	if !strings.Contains(out, "## [v0.2.0]") {
		t.Errorf("message does not name the pending section:\n%s", out)
	}
	if !strings.Contains(out, "**Breaking:**") {
		t.Errorf("message does not ask for a **Breaking:** entry:\n%s", out)
	}
}

// TestGateUntaggedTopSectionOfAnotherVersionIsNotPending is fixture (d): the
// cut-but-untagged top section is not the release being prepared (its
// version differs from cmd/zever/VERSION), so it must not absorb a new
// break: the break belongs under ## [Unreleased].
func TestGateUntaggedTopSectionOfAnotherVersionIsNotPending(t *testing.T) {
	root := breakingRepo(t)
	mustWrite(t, filepath.Join(root, "CHANGELOG.md"),
		"# Changelog\n\n## [v0.2.0] - 2026-10-20\n\n- **Breaking:** `mod` drops `api.Hello`\n")
	mustWrite(t, filepath.Join(root, "cmd", "zever", "VERSION"), "0.3.0\n")

	code, out := runGate(t, root)
	if code == 0 {
		t.Fatalf("exit 0, want non-zero (the untagged section is another release)\n%s", out)
	}
	if !strings.Contains(out, "## [Unreleased]") {
		t.Errorf("message does not point at ## [Unreleased]:\n%s", out)
	}
	if !strings.Contains(out, "**Breaking:**") {
		t.Errorf("message does not ask for a **Breaking:** entry:\n%s", out)
	}

	mustWrite(t, filepath.Join(root, "CHANGELOG.md"),
		"# Changelog\n\n## [Unreleased]\n\n- **Breaking:** `mod` drops `api.Hello`\n\n## [v0.2.0] - 2026-10-20\n")
	if code, out := runGate(t, root); code != 0 {
		t.Errorf("exit %d, want 0 once documented under ## [Unreleased]\n%s", code, out)
	}
}

// TestGateTaggedTopSectionIsReleased is fixture (c): once ## [v0.2.0] is
// tagged, the diff baseline moves to it and its entries no longer count as
// pending documentation, so the break must be documented under
// ## [Unreleased] instead.
func TestGateTaggedTopSectionIsReleased(t *testing.T) {
	root := initRepo(t)
	mustGit(t, root, "tag", "mod/v0.1.0")
	mustGit(t, root, "tag", "mod/v0.2.0") // tagged at the compatible commit
	mustWrite(t, filepath.Join(root, "mod", "api.go"), breakAPI)
	mustGit(t, root, "add", "-A")
	mustGit(t, root, "commit", "-qm", "break api after v0.2.0")
	mustWrite(t, filepath.Join(root, "CHANGELOG.md"),
		"# Changelog\n\n## [v0.2.0] - 2026-10-20\n\n- **Breaking:** `mod` drops `api.Hello`\n")

	code, out := runGate(t, root)
	if code == 0 {
		t.Fatalf("exit 0, want non-zero (tagged section is not pending)\n%s", out)
	}
	if !strings.Contains(out, "breaks mod/v0.2.0") {
		t.Errorf("baseline did not move to the tagged v0.2.0:\n%s", out)
	}
	if !strings.Contains(out, "## [Unreleased]") {
		t.Errorf("message does not point at ## [Unreleased]:\n%s", out)
	}

	mustWrite(t, filepath.Join(root, "CHANGELOG.md"),
		"# Changelog\n\n## [Unreleased]\n\n- **Breaking:** `mod` drops `api.Hello`\n\n## [v0.2.0] - 2026-10-20\n")
	if code, out := runGate(t, root); code != 0 {
		t.Errorf("exit %d, want 0 once documented under ## [Unreleased]\n%s", code, out)
	}
}
