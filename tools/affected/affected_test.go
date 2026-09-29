package main

import (
	"reflect"
	"testing"
)

func mustEqual(t *testing.T, name string, got, want any) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Errorf("%s: got %#v, want %#v", name, got, want)
	}
}

func TestParseRequires(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		content string
		want    []string
	}{
		{
			name: "block with direct and indirect",
			content: `module github.com/zenta-dev/zever/adapters/cache/redis

go 1.27.0

require (
	github.com/zenta-dev/zever/core/cache v0.0.0
	github.com/zenta-dev/zever/shared/redisclient v0.0.0 // indirect
	github.com/redis/go-redis/v9 v9.0.0
)
`,
			want: []string{"core/cache", "shared/redisclient"},
		},
		{
			name: "single line requires",
			content: `module example

go 1.27.0

require github.com/zenta-dev/zever/core/db v0.0.0
require github.com/zenta-dev/zever/orm v0.0.0 // indirect
`,
			want: []string{"core/db", "orm"},
		},
		{
			name:    "no zever requires",
			content: "module example\n\ngo 1.27.0\n\nrequire github.com/other/pkg v1.0.0\n",
			want:    []string{},
		},
		{
			name:    "commented require ignored",
			content: "module example\n\ngo 1.27.0\n\n// require github.com/zenta-dev/zever/core/log v0.0.0\n",
			want:    []string{},
		},
		{
			name:    "empty",
			content: "module example\n",
			want:    []string{},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			mustEqual(t, "parseRequires", parseRequires(tc.content), tc.want)
		})
	}
}

func TestOwnerModule(t *testing.T) {
	t.Parallel()
	modSet := map[string]bool{"core/cache": true, "adapters/cache/redis": true, "dsl": true}
	tests := []struct {
		name string
		file string
		want string
	}{
		{"direct file", "core/cache/cache.go", "core/cache"},
		{"nested file", "adapters/cache/redis/deep/dir/f.go", "adapters/cache/redis"},
		{"root file has no owner", "Makefile", ""},
		{"root go.work has no owner", "go.work", ""},
		{"unknown tree has no owner", "examples/external-sms/go.mod", ""},
		{"docs examples tree has no owner", "docs/examples/go.mod", ""},
		{"docs file owned by module", "dsl/README.md", "dsl"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			mustEqual(t, "ownerModule", ownerModule(tc.file, modSet), tc.want)
		})
	}
}

func TestDependentsClosure(t *testing.T) {
	t.Parallel()
	requires := map[string][]string{
		"b": {"a"},
		"c": {"a", "b"},
		"d": {"b", "c"},
		"e": {},
	}
	tests := []struct {
		name    string
		changed []string
		want    []string
	}{
		{"leaf pulls diamond", []string{"a"}, []string{"a", "b", "c", "d"}},
		{"mid pulls importers", []string{"b"}, []string{"b", "c", "d"}},
		{"top pulls nothing new", []string{"d"}, []string{"d"}},
		{"unrelated stays alone", []string{"e"}, []string{"e"}},
		{"multiple roots", []string{"d", "e"}, []string{"d", "e"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			mustEqual(t, "dependentsClosure", dependentsClosure(tc.changed, requires), tc.want)
		})
	}
}

func TestDependentsClosureIndirectEdge(t *testing.T) {
	t.Parallel()
	got := dependentsClosure([]string{"shared/redisclient"}, map[string][]string{
		"adapters/cache/redis": {"core/cache", "shared/redisclient"},
		"core/cache":           {},
	})
	mustEqual(t, "indirect", got, []string{"adapters/cache/redis", "shared/redisclient"})
}

func TestChunkGroups(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		mods      []string
		maxGroups int
		want      [][]string
	}{
		{"empty", nil, 4, [][]string{}},
		{"fewer than groups", []string{"a", "b"}, 8, [][]string{{"a"}, {"b"}}},
		{"even split", []string{"a", "b", "c", "d"}, 2, [][]string{{"a", "b"}, {"c", "d"}}},
		{"uneven split", []string{"a", "b", "c", "d", "e"}, 2, [][]string{{"a", "b", "c"}, {"d", "e"}}},
		{"single group", []string{"a", "b"}, 1, [][]string{{"a", "b"}}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			mustEqual(t, "chunkGroups", chunkGroups(tc.mods, tc.maxGroups), tc.want)
		})
	}
}

func TestExcludedDir(t *testing.T) {
	t.Parallel()
	yes := []string{".git", ".git/objects", "examples/external-sms", "examples/external-sms/go.mod", "docs/examples", "docs/examples/cache_test.go"}
	for _, d := range yes {
		if !excludedDir(d) {
			t.Errorf("excludedDir(%q) = false, want true", d)
		}
	}
	no := []string{".", "examples", "examples/bookings", "docs", "docs/src/x.mdx", "tools/affected", "core/cache"}
	for _, d := range no {
		if excludedDir(d) {
			t.Errorf("excludedDir(%q) = true, want false", d)
		}
	}
}

func TestIsDocsPath(t *testing.T) {
	t.Parallel()
	docs := []string{"README.md", "core/cache/README.md", "x/y.mdx", "docs/guide.md", "CHANGELOG.md", "CITATION.cff", "LICENSE", "LICENSE-MIT", "core/db/LICENSE.md"}
	for _, f := range docs {
		if !isDocsPath(f) {
			t.Errorf("isDocsPath(%q) = false, want true", f)
		}
	}
	code := []string{"core/cache/cache.go", "Makefile", "go.work", "dsl/compile/testdata/app.zen", "docs.go"}
	for _, f := range code {
		if isDocsPath(f) {
			t.Errorf("isDocsPath(%q) = true, want false", f)
		}
	}
}

func TestClassify(t *testing.T) {
	t.Parallel()
	modules := []string{"a", "b", "c", "z"}
	requires := map[string][]string{"b": {"a"}, "c": {"b"}}
	tests := []struct {
		name      string
		changed   []string
		maxGroups int
		wantGroup [][]string
		wantWhy   string
	}{
		{"no changes", nil, 8, [][]string{}, "none"},
		{"docs only", []string{"README.md", "docs/x.md", "a/NOTES.mdx"}, 8, [][]string{}, "none"},
		{"go.work is global", []string{"a/a.go", "go.work"}, 8, [][]string{{"a"}, {"b"}, {"c"}, {"z"}}, "all"},
		{"makefile is global", []string{"Makefile"}, 8, [][]string{{"a"}, {"b"}, {"c"}, {"z"}}, "all"},
		{"workflow is global", []string{".github/workflows/ci.yml"}, 1, [][]string{{"a", "b", "c", "z"}}, "all"},
		{"tool self change is global", []string{"tools/affected/main.go"}, 8, [][]string{{"a"}, {"b"}, {"c"}, {"z"}}, "all"},
		{"root file has no owner", []string{"unknown.txt"}, 8, [][]string{{"a"}, {"b"}, {"c"}, {"z"}}, "all"},
		{"leaf with dependents", []string{"a/a.go"}, 8, [][]string{{"a"}, {"b"}, {"c"}}, "affected"},
		{"chunked affected", []string{"a/a.go"}, 2, [][]string{{"a", "b"}, {"c"}}, "affected"},
		{"docs plus code is affected", []string{"z/NOTES.md", "z/z.go"}, 8, [][]string{{"z"}}, "affected"},
		{"docs examples only is none", []string{"docs/examples/cache_test.go", "docs/examples/go.mod"}, 8, [][]string{}, "none"},
		{"external-sms only is none", []string{"examples/external-sms/sms.go"}, 8, [][]string{}, "none"},
		{"out-of-workspace plus leaf is leaf scope", []string{"docs/examples/x_test.go", "examples/external-sms/y.go", "a/a.go"}, 8, [][]string{{"a"}, {"b"}, {"c"}}, "affected"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			groups, reason := classify(tc.changed, modules, requires, tc.maxGroups)
			mustEqual(t, "reason", reason, tc.wantWhy)
			mustEqual(t, "groups", groups, tc.wantGroup)
		})
	}
}
