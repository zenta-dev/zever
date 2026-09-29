package main

import (
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"
)

func mustWriteFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func mustEqual(t *testing.T, name string, got, want any) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Errorf("%s: got %#v, want %#v", name, got, want)
	}
}

func sortedSet(set map[string]bool) []string {
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func TestScanImportsExcludesTemplateText(t *testing.T) {
	t.Parallel()
	src := "package gen\n\n" +
		"const tmpl = `import \"github.com/zenta-dev/zever/core/cache\"`\n\n" +
		"const s = \"github.com/zenta-dev/zever/core/queue\"\n\n" +
		"// import \"github.com/zenta-dev/zever/core/log\"\n\n" +
		"/* import \"github.com/zenta-dev/zever/core/db\" */\n\n" +
		"var _ = 1\n"
	mustEqual(t, "scanImports", sortedSet(scanImports(src)), []string{})
}

func TestScanImportsFixtureTemplateExcluded(t *testing.T) {
	t.Parallel()
	content, err := os.ReadFile("testdata/template_imports.go")
	if err != nil {
		t.Fatal(err)
	}
	mustEqual(t, "scanImports", sortedSet(scanImports(string(content))), []string{})
}

func TestScanImportsDetectsForms(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		src  string
		want []string
	}{
		{
			name: "block",
			src:  "package a\n\nimport (\n\t\"github.com/zenta-dev/zever/core/cache\"\n\t\"github.com/zenta-dev/zever/shared/codec\"\n)\n",
			want: []string{"github.com/zenta-dev/zever/core/cache", "github.com/zenta-dev/zever/shared/codec"},
		},
		{
			name: "single",
			src:  "package a\n\nimport \"github.com/zenta-dev/zever/core/db\"\n",
			want: []string{"github.com/zenta-dev/zever/core/db"},
		},
		{
			name: "aliased",
			src:  "package a\n\nimport mycache \"github.com/zenta-dev/zever/core/cache\"\n",
			want: []string{"github.com/zenta-dev/zever/core/cache"},
		},
		{
			name: "blank",
			src:  "package a\n\nimport _ \"github.com/zenta-dev/zever/core/log\"\n",
			want: []string{"github.com/zenta-dev/zever/core/log"},
		},
		{
			name: "dot",
			src:  "package a\n\nimport . \"github.com/zenta-dev/zever/core/auth\"\n",
			want: []string{"github.com/zenta-dev/zever/core/auth"},
		},
		{
			name: "nonzever ignored",
			src:  "package a\n\nimport (\n\t\"fmt\"\n\t\"github.com/other/pkg\"\n)\n",
			want: []string{},
		},
		{
			name: "unparsable yields empty",
			src:  "package a\n\nimport (\n",
			want: []string{},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			mustEqual(t, "scanImports", sortedSet(scanImports(tc.src)), tc.want)
		})
	}
}

func TestProviderLongestPrefix(t *testing.T) {
	t.Parallel()
	mods := map[string]string{
		"github.com/zenta-dev/zever/core/cache":    "core/cache",
		"github.com/zenta-dev/zever/shared/redis":  "shared/redis",
		"github.com/zenta-dev/zever/shared":        "shared",
		"github.com/zenta-dev/zever/adapters/db/x": "adapters/db/x",
	}
	tests := []struct {
		name string
		imp  string
		want string
	}{
		{"exact", "github.com/zenta-dev/zever/core/cache", "github.com/zenta-dev/zever/core/cache"},
		{"subpackage", "github.com/zenta-dev/zever/core/cache/sub", "github.com/zenta-dev/zever/core/cache"},
		{"longest wins", "github.com/zenta-dev/zever/shared/redis/extra", "github.com/zenta-dev/zever/shared/redis"},
		{"no match", "github.com/other/pkg", ""},
		{"prefix without slash is not a match", "github.com/zenta-dev/zever/sharedx", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			mustEqual(t, "provider", provider(tc.imp, mods), tc.want)
		})
	}
}

func TestLockstepVersionDefault(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "CHANGELOG.md"),
		"# Changelog\n\n## [Unreleased]\n\n## [v9.8.7] - 2026-01-01\n")
	mustEqual(t, "lockstepVersion", lockstepVersion(root), "v9.8.7")
}

func TestLockstepVersionFallback(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		content *string
	}{
		{"missing file", nil},
		{"no version header", strptr("# Changelog\n\n## [Unreleased]\n")},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			if tc.content != nil {
				mustWriteFile(t, filepath.Join(root, "CHANGELOG.md"), *tc.content)
			}
			mustEqual(t, "lockstepVersion", lockstepVersion(root), "v0.0.0")
		})
	}
}

func strptr(s string) *string { return &s }

// TestCheckDriftFixture builds two scratch modules where app imports lib
// but its go.mod lacks the require, asserting --check drift names lib.
func TestCheckDriftFixture(t *testing.T) {
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "libs", "leaf", "go.mod"),
		"module github.com/zenta-dev/zever/libs/leaf\n\ngo 1.27.0\n")
	mustWriteFile(t, filepath.Join(root, "libs", "leaf", "leaf.go"), "package leaf\n")
	mustWriteFile(t, filepath.Join(root, "libs", "app", "go.mod"),
		"module github.com/zenta-dev/zever/libs/app\n\ngo 1.27.0\n")
	mustWriteFile(t, filepath.Join(root, "libs", "app", "app.go"),
		"package app\n\nimport \"github.com/zenta-dev/zever/libs/leaf\"\n\nvar _ = leaf.X\n")

	mods, err := findModules(root)
	if err != nil {
		t.Fatal(err)
	}
	mustEqual(t, "modules", mods, map[string]string{
		"github.com/zenta-dev/zever/libs/app":  "libs/app",
		"github.com/zenta-dev/zever/libs/leaf": "libs/leaf",
	})

	drift := checkModules(root, mods)
	app, ok := drift["libs/app"]
	if !ok {
		t.Fatalf("checkModules drift = %#v, want entry for libs/app", drift)
	}
	mustEqual(t, "require", app.Require, []string{"github.com/zenta-dev/zever/libs/leaf"})
	mustEqual(t, "replace", app.Replace, []string{"github.com/zenta-dev/zever/libs/leaf"})
	if len(drift) != 1 {
		t.Errorf("drift modules = %#v, want only libs/app", drift)
	}
}

// TestNestedModuleSubtreeSkipped asserts imports under a nested module dir
// do not count toward the outer module's closure.
func TestNestedModuleSubtreeSkipped(t *testing.T) {
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "outer", "go.mod"),
		"module github.com/zenta-dev/zever/outer\n\ngo 1.27.0\n\nrequire github.com/zenta-dev/zever/libs/leaf v0.0.0\n\nreplace github.com/zenta-dev/zever/libs/leaf => ../libs/leaf\n")
	mustWriteFile(t, filepath.Join(root, "outer", "outer.go"), "package outer\n")
	mustWriteFile(t, filepath.Join(root, "outer", "inner", "go.mod"),
		"module github.com/zenta-dev/zever/outer/inner\n\ngo 1.27.0\n")
	mustWriteFile(t, filepath.Join(root, "outer", "inner", "inner.go"),
		"package inner\n\nimport \"github.com/zenta-dev/zever/libs/leaf\"\n")
	mustWriteFile(t, filepath.Join(root, "libs", "leaf", "go.mod"),
		"module github.com/zenta-dev/zever/libs/leaf\n\ngo 1.27.0\n")
	mustWriteFile(t, filepath.Join(root, "libs", "leaf", "leaf.go"), "package leaf\n")

	mods, err := findModules(root)
	if err != nil {
		t.Fatal(err)
	}
	got, err := localImports(root, "outer", mods)
	if err != nil {
		t.Fatal(err)
	}
	mustEqual(t, "localImports", sortedSet(got), []string{})
}
