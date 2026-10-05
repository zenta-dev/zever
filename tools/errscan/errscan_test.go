package main

import (
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"
)

// mustWrite creates rel under root (slash-separated) with content and returns
// the on-disk path.
func mustWrite(t *testing.T, root, rel, content string) string {
	t.Helper()

	p := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	return p
}

// mustFixture reads a committed testdata fixture.
func mustFixture(t *testing.T, name string) string {
	t.Helper()

	content, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}

	return string(content)
}

// vkey projects a violation to its sortable identity, dropping the free-form
// message.
type vkey struct {
	path string
	line int
	rule string
}

// vkeys returns the violations projected to keys, sorted the way main() sorts.
func vkeys(vs []violation) []vkey {
	out := make([]vkey, 0, len(vs))
	for _, v := range vs {
		out = append(out, vkey{path: v.path, line: v.line, rule: v.rule})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].path != out[j].path {
			return out[i].path < out[j].path
		}
		if out[i].line != out[j].line {
			return out[i].line < out[j].line
		}

		return out[i].rule < out[j].rule
	})

	return out
}

// scanFixture materializes one fixture at rel under a temp root and scans it.
func scanFixture(t *testing.T, rel, fixture string) []violation {
	t.Helper()

	root := t.TempDir()
	mustWrite(t, root, rel, mustFixture(t, fixture))

	vs, err := scan(root)
	if err != nil {
		t.Fatalf("scan(%s): %v", rel, err)
	}

	return vs
}

// checkSource materializes src as rel under a temp root and checks that file.
func checkSource(t *testing.T, rel, src string) []violation {
	t.Helper()

	root := t.TempDir()
	p := mustWrite(t, root, rel, src)

	vs, err := checkFile(rel, p)
	if err != nil {
		t.Fatalf("checkFile(%s): %v", rel, err)
	}

	return vs
}

func TestScanCleanFixture(t *testing.T) {
	t.Parallel()

	if vs := scanFixture(t, "pkg/clean.go", "clean.txt"); len(vs) != 0 {
		t.Errorf("clean fixture = %#v, want no violations", vs)
	}
}

func TestScanViolationsFixture(t *testing.T) {
	t.Parallel()

	got := vkeys(scanFixture(t, "pkg/violations.go", "violations.txt"))
	want := []vkey{
		{path: "pkg/violations.go", line: 9, rule: "unprefixed"},
		{path: "pkg/violations.go", line: 10, rule: "unprefixed"},
		{path: "pkg/violations.go", line: 11, rule: "bracket-prefix"},
		{path: "pkg/violations.go", line: 11, rule: "unprefixed"},
		{path: "pkg/violations.go", line: 12, rule: "double-%w"},
		{path: "pkg/violations.go", line: 13, rule: "panic"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("violations = %#v, want %#v", got, want)
	}
}

func TestCheckFileAllowFileFixture(t *testing.T) {
	t.Parallel()

	if vs := scanFixture(t, "pkg/allow.go", "allow_file.txt"); len(vs) != 0 {
		t.Errorf("file-allow fixture = %#v, want no violations", vs)
	}
}

func TestCheckFileAllowLineFixture(t *testing.T) {
	t.Parallel()

	got := vkeys(scanFixture(t, "pkg/allow.go", "allow_line.txt"))
	want := []vkey{{path: "pkg/allow.go", line: 7, rule: "unprefixed"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("line-allow = %#v, want %#v", got, want)
	}
}

func TestCheckFileTestSuffix(t *testing.T) {
	t.Parallel()

	got := vkeys(scanFixture(t, "pkg/testfile_test.go", "testfile.txt"))
	want := []vkey{
		{path: "pkg/testfile_test.go", line: 10, rule: "bracket-prefix"},
		{path: "pkg/testfile_test.go", line: 11, rule: "double-%w"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("_test.go rules = %#v, want %#v", got, want)
	}
}

func TestCheckFileApperror(t *testing.T) {
	t.Parallel()

	got := vkeys(scanFixture(t, "shared/apperror/errors.go", "apperror.txt"))
	want := []vkey{{path: "shared/apperror/errors.go", line: 16, rule: "double-%w"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("apperror rules = %#v, want %#v", got, want)
	}
}

func TestCheckFileBuildTag(t *testing.T) {
	t.Parallel()

	got := vkeys(scanFixture(t, "pkg/buildtag.go", "buildtag.txt"))
	want := []vkey{{path: "pkg/buildtag.go", line: 8, rule: "unprefixed"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("build-tag rules = %#v, want %#v", got, want)
	}
}

func TestSkipDir(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		want bool
	}{
		{".git", true},
		{".worktrees", true},
		{".hidden", true},
		{"vendor", true},
		{"node_modules", true},
		{"generated", true},
		{"pkg", false},
		{"gen", false},
		{"generated_code", false},
		{"vendorX", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := skipDir(tc.name); got != tc.want {
				t.Errorf("skipDir(%q) = %v, want %v", tc.name, got, tc.want)
			}
		})
	}
}
