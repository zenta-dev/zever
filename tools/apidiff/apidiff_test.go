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
