package main

import (
	"path"
	"strings"
	"testing"

	"golang.org/x/mod/modfile"
)

// BenchmarkGoModPath measures the go.mod module path parsing hot path.
func BenchmarkGoModPath(b *testing.B) {
	content := []byte(`module github.com/zenta-dev/zever/config

go 1.27.3

require (
	github.com/cespare/xxhash/v2 v2.3.0
	github.com/zenta-dev/zever/shared/registry v0.0.0
)
`)

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		_, _ = goModPathFromBytes(content)
	}
}

// goModPathFromBytes is a copy for benchmarking without file I/O.
func goModPathFromBytes(raw []byte) (string, error) {
	f, err := modfile.Parse("go.mod", raw, nil)
	if err != nil {
		return "", err
	}
	if f.Module == nil || f.Module.Mod.Path == "" {
		return "", nil
	}
	return f.Module.Mod.Path, nil
}

// BenchmarkIsInternalPackage measures the internal package detection.
func BenchmarkIsInternalPackage(b *testing.B) {
	pkgPaths := []string{
		"github.com/zenta-dev/zever/config/internal",
		"github.com/zenta-dev/zever/config/internal/foo",
		"github.com/zenta-dev/zever/config/internal/foo/bar",
		"github.com/zenta-dev/zever/internal",
		"github.com/zenta-dev/zever/internal/pkg",
		"github.com/zenta-dev/zever/config",
		"github.com/zenta-dev/zever/config/options",
		"internal",
		"internal/pkg",
	}
	modulePath := "github.com/zenta-dev/zever/config"
	i := 0

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		_ = isInternalPackage(pkgPaths[i%len(pkgPaths)], modulePath)
		i++
	}
}

// BenchmarkSameVersion measures the version normalization and comparison.
func BenchmarkSameVersion(b *testing.B) {
	pairs := []struct{ a, b string }{
		{"0.8.0", "v0.8.0"},
		{"v1.2.3", "1.2.3"},
		{"v0.7.2", "v0.7.2"},
		{"2.0.0", "v2.0.0"},
		{"0.8.0", "0.8.1"},
		{"v1.0.0", "v2.0.0"},
	}
	i := 0

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		_ = sameVersion(pairs[i%len(pairs)].a, pairs[i%len(pairs)].b)
		i++
	}
}

// BenchmarkBackticked measures the backtick extraction from text.
func BenchmarkBackticked(b *testing.B) {
	texts := []string{
		"Use `github.com/zenta-dev/zever/config` for config.",
		"Module `config` and package `config/options` are affected.",
		"Changes: `config`, `config/options`, `orm`, `orm/query`.",
		"No backticks here.",
		"`short.Ident` and `short.Method` are covered.",
		"Multiple `one` `two` `three` `four` `five` tokens.",
	}
	i := 0

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		_ = backticked(texts[i%len(texts)])
		i++
	}
}

// BenchmarkEntryCovers measures the entry coverage check.
func BenchmarkEntryCovers(b *testing.B) {
	entries := []string{
		"- **Breaking:** `github.com/zenta-dev/zever/config` changed",
		"- **Breaking:** `config` module updated",
		"- **Breaking:** `config/options` package modified",
		"- **Breaking:** `orm` and `orm/query` affected",
		"- **Breaking:** `config/...` all packages",
		"- **Breaking:** `short.Ident` and `short.Method`",
		"Regular entry without breaking marker",
	}
	dir := "config"
	mod := "github.com/zenta-dev/zever/config"
	i := 0

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		_ = entryCovers(entries[i%len(entries)], dir, mod)
		i++
	}
}

// BenchmarkSectionHeaders measures the changelog section header parsing.
func BenchmarkSectionHeaders(b *testing.B) {
	body := `# Changelog

## [Unreleased]

### Changed
- Something

## [v0.8.0] - 2026-10-18

### Added
- Feature

## [v0.7.2] - 2026-09-15

### Fixed
- Bug fix
`

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		_ = sectionHeaders(body)
	}
}

// BenchmarkPendingHeaders measures the pending headers detection.
func BenchmarkPendingHeaders(b *testing.B) {
	body := `# Changelog

## [Unreleased]

### Changed
- Something

## [v0.8.0] - 2026-10-18

### Added
- Feature

## [v0.7.2] - 2026-09-15

### Fixed
- Bug fix
`
	tagged := func(version string) bool {
		return version == "0.7.2"
	}
	currentVersion := "0.8.0"

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		_ = pendingHeaders(body, tagged, currentVersion)
	}
}

// BenchmarkBreakingEntries measures the breaking entry extraction.
func BenchmarkBreakingEntries(b *testing.B) {
	body := "# Changelog\n\n## [Unreleased]\n\n### Changed\n- **Breaking:** \x60config\x60 module changed\n- Regular entry\n- **Breaking:** \x60orm/query\x60 modified\n\n## [v0.8.0] - 2026-10-18\n\n### Added\n- Feature\n"
	headers := []string{"## [Unreleased]"}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		_ = breakingEntries(body, headers)
	}
}

// BenchmarkAnyCovers measures the anyCovers check over multiple entries.
func BenchmarkAnyCovers(b *testing.B) {
	entries := []string{
		"- **Breaking:** \x60config\x60 module changed",
		"- **Breaking:** \x60orm/query\x60 modified",
		"- Regular entry",
	}
	dir := "config"
	mod := "github.com/zenta-dev/zever/config"

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		_ = anyCovers(entries, dir, mod)
	}
}

// BenchmarkVersionFloor measures the version floor parsing.
func BenchmarkVersionFloor(b *testing.B) {
	content := []byte("v0.8.0")

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		_, _ = versionFloorFromBytes(content)
	}
}

// versionFloorFromBytes is a copy for benchmarking without file I/O.
func versionFloorFromBytes(raw []byte) (string, error) {
	s := strings.TrimSpace(string(raw))
	s = strings.TrimPrefix(s, "v")
	return s, nil
}

// BenchmarkAppendChangelogFragments measures the fragment appending logic.
func BenchmarkAppendChangelogFragments(b *testing.B) {
	body := []byte("# Changelog\n\n## [v0.8.0]\n\n### Added\n- Feature\n")
	// We can't easily test without filesystem, so skip filesystem-dependent
	// version for now
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		// Just test the string building portion
		var sb strings.Builder
		sb.Write(body)
		sb.WriteString("\n## [Unreleased]\n")
		sb.WriteString("\n- **Breaking:** \x60config\x60 changed\n")
		_ = sb.String()
	}
}

// BenchmarkTagExists_StringProcessing measures the string processing in tagExists.
func BenchmarkTagExists_StringProcessing(b *testing.B) {
	versions := []string{"0.8.0", "v0.8.0", "1.2.3", "v1.2.3"}
	out := "v0.8.0\nconfig/v0.8.0\norm/v0.8.0\n"
	i := 0

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		v := versions[i%len(versions)]
		_ = "v" + strings.TrimPrefix(v, "v")
		_ = strings.TrimSpace(out) != ""
		i++
	}
}

// BenchmarkPathBase measures path.Base for module short names.
func BenchmarkPathBase(b *testing.B) {
	dirs := []string{
		"config",
		"core/cache",
		"adapters/cache/redis",
		"github.com/zenta-dev/zever/config",
	}
	i := 0

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		_ = path.Base(dirs[i%len(dirs)])
		i++
	}
}
