package main

import (
	"slices"
	"testing"
)

// benchRequires is a representative dependency graph: shared leaves fan out
// into core modules, which fan out into adapters and examples.
var benchRequires = map[string][]string{
	"core/cache":            {"shared/registry"},
	"core/db":               {"shared/registry"},
	"adapters/cache/redis":  {"core/cache", "shared/redisclient"},
	"adapters/cache/memory": {"core/cache"},
	"adapters/db/sqlite":    {"core/db"},
	"config":                {"core/cache", "core/db", "shared/registry"},
	"container":             {"config", "core/cache", "core/db"},
	"cmd/zever":             {"config", "container", "dsl"},
	"examples/demoapp":      {"config", "container", "orm"},
}

// BenchmarkDependentsClosure measures the reverse-dependency closure over a
// realistic module graph.
func BenchmarkDependentsClosure(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if got := dependentsClosure([]string{"shared/registry"}, benchRequires); !slices.Contains(got, "cmd/zever") {
			b.Fatalf("closure missing cmd/zever: %v", got)
		}
	}
}

// BenchmarkClassify measures the full changed-file classification that drives
// CI fan-out.
func BenchmarkClassify(b *testing.B) {
	modules := make([]string, 0, len(benchRequires))
	for m := range benchRequires {
		modules = append(modules, m)
	}
	changed := []string{"core/cache/cache.go", "core/cache/README.md", "cmd/zever/main.go"}

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		groups, reason := classify(changed, modules, benchRequires, 4)
		if reason != "affected" || len(groups) == 0 {
			b.Fatalf("classify = (%v, %q)", groups, reason)
		}
	}
}

// BenchmarkParseRequires measures go.mod require scanning.
func BenchmarkParseRequires(b *testing.B) {
	content := `module github.com/zenta-dev/zever/adapters/cache/redis

go 1.27.0

require (
	github.com/zenta-dev/zever/core/cache v0.5.3
	github.com/zenta-dev/zever/shared/redisclient v0.5.3 // indirect
	github.com/redis/go-redis/v9 v9.0.0
)
`

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if got := parseRequires(content); len(got) != 2 {
			b.Fatalf("parseRequires = %v, want 2 entries", got)
		}
	}
}

// BenchmarkChunkGroups measures the CI job chunking helper.
func BenchmarkChunkGroups(b *testing.B) {
	mods := make([]string, 100)
	for i := range mods {
		mods[i] = "mod-" + string(rune('a'+i%26))
	}

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if got := chunkGroups(mods, 8); len(got) != 8 {
			b.Fatalf("groups = %d, want 8", len(got))
		}
	}
}

// BenchmarkOwnerModule measures the longest-prefix module ownership lookup.
func BenchmarkOwnerModule(b *testing.B) {
	modSet := map[string]bool{
		"core/cache": true, "adapters/cache/redis": true, "cmd/zever": true, "dsl": true,
	}

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if got := ownerModule("adapters/cache/redis/deep/dir/f.go", modSet); got != "adapters/cache/redis" {
			b.Fatalf("ownerModule = %q", got)
		}
	}
}
