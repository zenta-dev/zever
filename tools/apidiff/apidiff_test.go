package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
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

	mustWrite(t, filepath.Join(root, "CHANGELOG.md"), `# Changelog

## [Unreleased]

### Breaking

- **Breaking:** drop api.Hello and widen World.Method

## [v0.1.0] - 2026-01-01
`)
	stdout.Reset()
	stderr.Reset()
	code = run(context.Background(), []string{"--root", root, "--modules", "mod"}, &stdout, &stderr)
	if code != 0 {
		t.Errorf("run broken tree with Breaking trailer = exit %d, want 0\nstdout:\n%s\nstderr:\n%s", code, stdout.String(), stderr.String())
	}
}
