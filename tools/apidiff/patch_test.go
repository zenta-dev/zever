package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// badPatchLines is the deliberately bad fixture: v0.7.2 is a patch
// release but documents a breaking change, so --check-patch must fail.
// Lines join with newlines in the test.
var badPatchLines = []string{
	"# Changelog",
	"",
	"## [Unreleased]",
	"",
	"### Added",
	"",
	"- something new",
	"",
	"## [v0.7.2] - 2026-10-16",
	"",
	"### Fixed",
	"",
	"- **Breaking:** `container` drops `CloseTimeoutError` wrapping (Core break in a patch)",
	"",
	"## [v0.7.1] - 2026-10-15",
	"",
	"### Fixed",
	"",
	"- nothing breaking here",
	"",
}

// goodPatchLines documents breaks only where allowed: Unreleased and a
// minor release. No patch section carries a marker.
var goodPatchLines = []string{
	"# Changelog",
	"",
	"## [Unreleased]",
	"",
	"### Added",
	"",
	"- **Breaking:** `container` reworks shutdown ordering (ships in the next minor)",
	"",
	"## [v0.7.2] - 2026-10-16",
	"",
	"### Fixed",
	"",
	"- a compatible bugfix",
	"",
	"- **Breaking:** `payment` normalizes amounts (Extended, allowed in patches)",
	"",
	"## [v0.8.0] - 2026-10-10",
	"",
	"### Changed",
	"",
	"- **Breaking:** `orm` renames `From` (minor, allowed with notes)",
	"",
	"## [v0.7.0] - 2026-10-01",
	"",
	"### Added",
	"",
	"- initial release",
	"",
}

func mustChangelog(t *testing.T, lines []string) string {
	t.Helper()
	f := filepath.Join(t.TempDir(), "CHANGELOG.md")
	if err := os.WriteFile(f, []byte(strings.Join(lines, "\n")), 0o600); err != nil {
		t.Fatal(err)
	}
	return f
}

func TestPatchReleasesWithBreaking(t *testing.T) {
	t.Parallel()

	bad := strings.Join(badPatchLines, "\n")
	if got := patchReleasesWithBreaking(bad, "v0.7.1"); len(got) != 1 {
		t.Errorf("bad fixture offenders = %v, want one entry naming v0.7.2", got)
	} else if !strings.Contains(got[0], "v0.7.2") {
		t.Errorf("bad fixture offender = %q, want it to name v0.7.2", got[0])
	}

	if got := patchReleasesWithBreaking(strings.Join(goodPatchLines, "\n"), "v0.7.1"); len(got) != 0 {
		t.Errorf("good fixture offenders = %v, want none (Unreleased/minor breaks are allowed)", got)
	}

	if got := patchReleasesWithBreaking("# Changelog\n\n## [Unreleased]\n", "v0.7.1"); len(got) != 0 {
		t.Errorf("empty changelog offenders = %v, want none", got)
	}

	// Sections at or below the floor are grandfathered: the same Core
	// break that fails above the floor passes when already released.
	if got := patchReleasesWithBreaking(strings.Join(badPatchLines, "\n"), "v0.7.2"); len(got) != 0 {
		t.Errorf("grandfathered offenders = %v, want none", got)
	}
}

// TestCheckPatchFailsOnBadFixture proves the guard bites: the bad fixture
// exits 1 naming the offending patch release.
func TestCheckPatchFailsOnBadFixture(t *testing.T) {
	f := mustChangelog(t, badPatchLines)

	var stdout, stderr strings.Builder
	code := run(context.Background(), []string{"--check-patch", "--floor", "v0.7.1", "--changelog", f}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("--check-patch on bad fixture = exit %d, want 1\nstdout:\n%s\nstderr:\n%s", code, stdout.String(), stderr.String())
	}
	if out := stdout.String() + stderr.String(); !strings.Contains(out, "v0.7.2") {
		t.Errorf("guard output does not name v0.7.2:\n%s", out)
	}
}

func TestCheckPatchPassesOnGoodFixture(t *testing.T) {
	f := mustChangelog(t, goodPatchLines)

	var stdout, stderr strings.Builder
	code := run(context.Background(), []string{"--check-patch", "--floor", "v0.7.1", "--changelog", f}, &stdout, &stderr)
	if code != 0 {
		t.Errorf("--check-patch on good fixture = exit %d, want 0\nstdout:\n%s\nstderr:\n%s", code, stdout.String(), stderr.String())
	}
}
