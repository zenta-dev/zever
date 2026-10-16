package main

import (
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
)

// patchSectionHeader matches a released CHANGELOG section header, e.g.
// "## [v0.7.2] - 2026-10-15". The patchGuard enforces "no breaking change
// in a patch release": any such section whose patch number is nonzero must
// not carry a **Breaking:** entry.
var patchSectionHeader = regexp.MustCompile(`^## \[[vV]?(\d+)\.(\d+)\.(\d+)[^]]*\]`)

// zeverModulePrefix builds the import path of a repo module dir, matching
// the module lines in each go.mod (e.g. core/db).
const zeverModulePrefix = "github.com/zenta-dev/zever/"

// patchReleasesWithBreaking returns one line per released patch section
// (X.Y.Z with Z nonzero) newer than floor that documents a breaking change
// to a Core-tier module, e.g. "## [v0.7.2] - ...: <entry first line>".
// Sections at or below floor (the in-development VERSION: already released
// under an older policy) are grandfathered. Breaks to Extended packages
// may still ship in patches pre-1.0 (STABILITY.md), so only entries naming
// a Core module (same backtick convention as the apidiff gate) offend.
// ## [Unreleased] and minor/major sections never do: breaks land there,
// documented, and ship in the next minor or major.
func patchReleasesWithBreaking(body, floor string) []string {
	var offenders []string
	var header string
	var section []string
	flush := func() {
		if header == "" {
			return
		}
		m := patchSectionHeader.FindStringSubmatch(header)
		if m == nil || m[3] == "0" || !newerThan(m[1], m[2], m[3], floor) {
			return
		}
		for _, e := range sectionEntries(section) {
			if coreCovered(e) {
				first, _, _ := strings.Cut(e, "\n")
				offenders = append(offenders, header+": "+strings.TrimSpace(first))
			}
		}
	}
	for _, line := range strings.Split(body, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "## ") {
			flush()
			header = trimmed
			section = nil
			continue
		}
		section = append(section, line)
	}
	flush()
	return offenders
}

// sectionEntries returns the entries (a "- " bullet plus its indented
// continuation lines) of one released section that contain a
// **Breaking:** marker, mirroring breakingEntries for ## [Unreleased].
func sectionEntries(lines []string) []string {
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
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		switch {
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

// newerThan reports whether X.Y.Z is strictly above the floor version
// ("v1.2.3" or "1.2.3"; empty floor means everything is newer).
func newerThan(x, y, z, floor string) bool {
	floor = strings.TrimSpace(strings.TrimPrefix(floor, "v"))
	if floor == "" {
		return true
	}
	var fx, fy, fz int
	if _, err := fmt.Sscanf(floor, "%d.%d.%d", &fx, &fy, &fz); err != nil {
		return true
	}
	var ix, iy, iz int
	fmt.Sscanf(x, "%d", &ix)
	fmt.Sscanf(y, "%d", &iy)
	fmt.Sscanf(z, "%d", &iz)
	switch {
	case ix != fx:
		return ix > fx
	case iy != fy:
		return iy > fy
	default:
		return iz > fz
	}
}

// versionFloor reads the in-development version (cmd/zever/VERSION).
func versionFloor(file string) (string, error) {
	raw, err := os.ReadFile(file)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(raw)), nil
}

// coreCovered reports whether entry names a Core-tier module under the
// apidiff naming convention.
func coreCovered(entry string) bool {
	for _, dir := range coreModules {
		if entryCovers(entry, dir, zeverModulePrefix+dir) {
			return true
		}
	}
	return false
}

// runPatchGuard implements --check-patch: exit 1 naming every patch
// section newer than floor that documents a Core-tier breaking change.
func runPatchGuard(changelog, floor string, stdout, stderr io.Writer) int {
	raw, err := os.ReadFile(changelog)
	if err != nil {
		fmt.Fprintf(stderr, "apidiff: read %s: %v\n", changelog, err)
		return 1
	}
	offenders := patchReleasesWithBreaking(string(raw), floor)
	if len(offenders) == 0 {
		fmt.Fprintln(stdout, "apidiff: no Core-tier breaking changes in patch releases")
		return 0
	}
	for _, v := range offenders {
		fmt.Fprintf(stdout, "apidiff: patch release documents a Core-tier break: %s; ship Core breaks in a minor or major instead\n", v)
	}
	return 1
}
