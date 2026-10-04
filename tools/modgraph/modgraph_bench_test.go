package main

import "testing"

// benchSource is a representative generated file with several intra-repo
// imports and a template string that must not count.
const benchSource = `package gen

import (
	"context"
	"fmt"

	"github.com/zenta-dev/zever/core/cache"
	"github.com/zenta-dev/zever/core/db"
	"github.com/zenta-dev/zever/orm"
	"github.com/redis/go-redis/v9"
)

const tmpl = ` + "`" + `import "github.com/zenta-dev/zever/core/queue"` + "`" + `

var _ = context.Background
var _ = fmt.Sprint
var _ = cache.Options{}
var _ = db.Options{}
var _ = orm.NewTable
var _ = redis.NewClient
`

// BenchmarkScanImports measures the AST-based import scan over a file that
// mixes real imports with decoy template text.
func BenchmarkScanImports(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if got := scanImports(benchSource); len(got) != 3 {
			b.Fatalf("scanImports = %v, want 3 paths", got)
		}
	}
}

// BenchmarkProvider measures the longest-prefix module lookup.
func BenchmarkProvider(b *testing.B) {
	mods := map[string]string{
		"github.com/zenta-dev/zever/core/cache":     "core/cache",
		"github.com/zenta-dev/zever/core":           "core",
		"github.com/zenta-dev/zever/adapters/cache": "adapters/cache",
	}
	const imp = "github.com/zenta-dev/zever/core/cache/typed"

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if got := provider(imp, mods); got != "github.com/zenta-dev/zever/core/cache" {
			b.Fatalf("provider = %q", got)
		}
	}
}

// BenchmarkParseRequires measures the regex-based require scan.
func BenchmarkParseRequires(b *testing.B) {
	const content = `module github.com/zenta-dev/zever/config

go 1.27.0

require (
	github.com/zenta-dev/zever/core/cache v0.5.3
	github.com/zenta-dev/zever/core/db v0.5.3
	github.com/zenta-dev/zever/shared/codec v0.5.3
	go.yaml.in/yaml/v3 v3.0.5
)
`

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if got := parseRequires(content); len(got) != 3 {
			b.Fatalf("parseRequires = %v, want 3", got)
		}
	}
}

// BenchmarkParseReplaces measures the regex-based replace scan.
func BenchmarkParseReplaces(b *testing.B) {
	const content = `module github.com/zenta-dev/zever/config

replace (
	github.com/zenta-dev/zever/core/cache => ../core/cache
	github.com/zenta-dev/zever/core/db => ../core/db
	github.com/zenta-dev/zever/shared/codec => ../shared/codec
)
`

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if got := parseReplaces(content); len(got) != 3 {
			b.Fatalf("parseReplaces = %v, want 3", got)
		}
	}
}
