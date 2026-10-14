package main

import (
	_ "embed" // embeds the VERSION fallback
	"regexp"
	"runtime/debug"
	"strings"
)

// cliVersion is the release version injected by the release build via
// -ldflags "-X main.cliVersion=X.Y.Z" (no leading "v"). It is empty in plain
// `go build`/`go test` runs; use buildVersion for the resolved value.
var cliVersion string

// fallbackVersion is the in-repo release version (no leading "v"), kept in
// the VERSION file next to this package. tools/tag-release.sh refuses to tag
// until it matches, and tools/check-version.sh fails CI when it lags the
// newest release tag.
//
//go:embed VERSION
var fallbackVersion string

// buildVersion is the CLI release version printed by -V/--version, stamped
// into generated files and pinned by scaffolds. Resolution order: the
// -ldflags value, the main module version recorded by `go install
// module@vX.Y.Z`, then the embedded VERSION file.
var buildVersion = resolveVersion(cliVersion, mainModuleVersion(), fallbackVersion)

var (
	semverRe = regexp.MustCompile(`^v?\d+\.\d+\.\d+(-[0-9A-Za-z.-]+)?$`)
	// pseudoVersionRe matches the "<timestamp>-<commit>" tail of a Go
	// pseudo-version, which names no published release.
	pseudoVersionRe = regexp.MustCompile(`\d{14}-[0-9a-f]{12}$`)
)

// mainModuleVersion returns the main module version from the embedded build
// info, or "" when unavailable.
func mainModuleVersion() string {
	info, ok := debug.ReadBuildInfo()
	if !ok || info == nil {
		return ""
	}

	return info.Main.Version
}

// resolveVersion picks the first usable candidate (ldflags, build info,
// fallback) and returns it without a leading "v". Build-info values such as
// "(devel)", pseudo-versions and "+dirty" builds are rejected because they
// name no published release a scaffold could pin.
func resolveVersion(ldflags, buildInfo, fallback string) string {
	if v := strings.TrimSpace(ldflags); v != "" {
		return strings.TrimPrefix(v, "v")
	}

	if v := strings.TrimSpace(buildInfo); semverRe.MatchString(v) && !pseudoVersionRe.MatchString(v) {
		return strings.TrimPrefix(v, "v")
	}

	return strings.TrimPrefix(strings.TrimSpace(fallback), "v")
}
