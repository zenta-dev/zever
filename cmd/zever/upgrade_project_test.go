package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const projectUpgradeFixtureGoMod = `module example.com/shop

go 1.27.2

require (
	github.com/zenta-dev/zever/config v0.6.0
	github.com/zenta-dev/zever/container v0.6.0
	github.com/spf13/cobra v1.9.1
)
`

func mustWriteProjectGoMod(t *testing.T, dir, content string) {
	t.Helper()

	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(content), 0o600); err != nil {
		t.Fatalf("write fixture go.mod: %v", err)
	}
}

func TestBumpProjectGoMod(t *testing.T) {
	t.Parallel()

	got, n, err := bumpProjectGoMod([]byte(projectUpgradeFixtureGoMod), "v0.7.0")
	if err != nil {
		t.Fatalf("bumpProjectGoMod: %v", err)
	}

	if n != 2 {
		t.Errorf("bumped %d requires, want 2", n)
	}

	s := string(got)
	if strings.Contains(s, "zever/config v0.6.0") || strings.Contains(s, "zever/container v0.6.0") {
		t.Errorf("old versions remain:\n%s", s)
	}

	if strings.Count(s, "v0.7.0") != 2 {
		t.Errorf("want 2 occurrences of v0.7.0:\n%s", s)
	}

	if !strings.Contains(s, "cobra v1.9.1") {
		t.Errorf("third-party require must be untouched:\n%s", s)
	}
}

func TestBumpProjectGoModNoZeverRequires(t *testing.T) {
	t.Parallel()

	_, _, err := bumpProjectGoMod([]byte("module example.com/shop\n\ngo 1.27.2\n"), "v0.7.0")
	if !errors.Is(err, ErrUpgradeProjectNoRequires) {
		t.Fatalf("err = %v, want ErrUpgradeProjectNoRequires", err)
	}
}

func TestBumpProjectGoModSkipsReplacedAndKeepsIndirect(t *testing.T) {
	t.Parallel()

	input := `module example.com/shop

go 1.27.2

require (
	github.com/zenta-dev/zever/config v0.6.0
	github.com/zenta-dev/zever/container v0.6.0 // indirect
)

replace github.com/zenta-dev/zever/container => ../container
`

	got, n, err := bumpProjectGoMod([]byte(input), "v0.7.0")
	if err != nil {
		t.Fatalf("bumpProjectGoMod: %v", err)
	}

	if n != 1 {
		t.Errorf("bumped %d requires, want 1 (container has a replace)", n)
	}

	s := string(got)
	if !strings.Contains(s, "zever/config v0.7.0") {
		t.Errorf("config not bumped:\n%s", s)
	}

	if !strings.Contains(s, "zever/container v0.6.0 // indirect") {
		t.Errorf("replaced container require must stay pinned with one // indirect:\n%s", s)
	}

	if strings.Count(s, "// indirect") != 1 {
		t.Errorf("indirect comment duplicated:\n%s", s)
	}
}

func TestProjectBreakingNotesBetweenVersions(t *testing.T) {
	t.Parallel()

	changelog := `## [0.7.0]

### Breaking

- **Breaking:** reflection off by default
- routine feature

## [0.6.1]

- patch fix

## [0.6.0]

### Breaking

- **Breaking:** old change
`
	notes := projectBreakingNotes([]byte(changelog), "v0.6.0", "v0.7.0")
	if len(notes) != 1 || !strings.Contains(notes[0], "reflection off by default") {
		t.Fatalf("notes = %q, want the single 0.7.0 breaking entry", notes)
	}
}

func TestRunUpgradeProjectDryRun(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	mustWriteProjectGoMod(t, dir, projectUpgradeFixtureGoMod)

	var out bytes.Buffer

	tidied := false

	err := runUpgradeProjectWith(t.Context(), projectUpgradeOptions{
		dir:    dir,
		target: "v0.7.0",
		dryRun: true,
		out:    &out,
		fetch: func(context.Context, string) ([]byte, error) {
			t.Error("dry-run must not fetch release notes")

			return nil, errors.New("zever: unexpected fetch")
		},
		tidy: func(context.Context, string) error {
			tidied = true

			return nil
		},
	})
	if err != nil {
		t.Fatalf("dry-run: %v", err)
	}

	if tidied {
		t.Error("dry-run ran go mod tidy")
	}

	data, rerr := os.ReadFile(filepath.Join(dir, "go.mod"))
	if rerr != nil {
		t.Fatalf("read go.mod: %v", rerr)
	}

	if strings.Contains(string(data), "v0.7.0") {
		t.Error("dry-run rewrote go.mod")
	}

	for _, want := range []string{"v0.6.0", "v0.7.0", "2"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("dry-run output missing %q:\n%s", want, out.String())
		}
	}
}

func TestRunUpgradeProjectRewritesAndTidies(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	mustWriteProjectGoMod(t, dir, projectUpgradeFixtureGoMod)

	var out bytes.Buffer

	var tidiedDir string

	changelog := "## [0.7.0]\n\n- **Breaking:** reflection off by default\n"

	err := runUpgradeProjectWith(t.Context(), projectUpgradeOptions{
		dir:    dir,
		target: "v0.7.0",
		out:    &out,
		fetch: func(_ context.Context, url string) ([]byte, error) {
			if strings.Contains(url, "CHANGELOG") {
				return []byte(changelog), nil
			}

			return nil, errors.New("zever: unexpected url " + url)
		},
		tidy: func(_ context.Context, d string) error {
			tidiedDir = d

			return nil
		},
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	if tidiedDir != dir {
		t.Errorf("tidy dir = %q, want %q", tidiedDir, dir)
	}

	data, rerr := os.ReadFile(filepath.Join(dir, "go.mod"))
	if rerr != nil {
		t.Fatalf("read go.mod: %v", rerr)
	}

	if strings.Count(string(data), "v0.7.0") != 2 {
		t.Errorf("go.mod not bumped:\n%s", data)
	}

	if !strings.Contains(out.String(), "reflection off by default") {
		t.Errorf("output missing breaking note:\n%s", out.String())
	}
}

func TestRunUpgradeProjectMissingGoMod(t *testing.T) {
	t.Parallel()

	err := runUpgradeProjectWith(t.Context(), projectUpgradeOptions{
		dir:    t.TempDir(),
		target: "v0.7.0",
		out:    &bytes.Buffer{},
	})
	if !errors.Is(err, ErrUpgradeProjectNoGoMod) {
		t.Fatalf("err = %v, want ErrUpgradeProjectNoGoMod", err)
	}
}
