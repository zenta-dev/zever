package main

import (
	"path/filepath"
	"reflect"
	"testing"
)

// TestScanEmptyFile pins that a zero-byte .go file fails the scan: the parser
// reports the missing package clause rather than silently yielding nothing.
func TestScanEmptyFile(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	mustWrite(t, root, "pkg/empty.go", "")

	if _, err := scan(root); err == nil {
		t.Fatal("scan(empty file) = nil error, want parse error")
	}
}

// TestScanNoMatches pins that valid Go with no error constructors yields no
// violations.
func TestScanNoMatches(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	mustWrite(t, root, "pkg/ok.go", "package ok\n\nfunc f() {}\n")

	vs, err := scan(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(vs) != 0 {
		t.Errorf("scan(no matches) = %#v, want none", vs)
	}
}

// TestCheckFileMalformed pins that malformed Go source is a hard error, not a
// best-effort empty result.
func TestCheckFileMalformed(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	p := mustWrite(t, root, "pkg/broken.go", mustFixture(t, "malformed.txt"))

	if _, err := checkFile("pkg/broken.go", p); err == nil {
		t.Fatal("checkFile(malformed) = nil error, want parse error")
	}
}

// TestScanMissingRoot pins that a nonexistent root surfaces the walk error.
func TestScanMissingRoot(t *testing.T) {
	t.Parallel()

	if _, err := scan(filepath.Join(t.TempDir(), "does-not-exist")); err == nil {
		t.Fatal("scan(missing root) = nil error, want walk error")
	}
}

// TestScanNestedDirs pins that violations carry slash-separated paths relative
// to the scan root.
func TestScanNestedDirs(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	mustWrite(t, root, "a/b/c.go", "package c\n\nimport \"errors\"\n\nvar _ = errors.New(\"x\")\n")
	mustWrite(t, root, "a/d.go", "package d\n\nimport \"errors\"\n\nvar _ = errors.New(\"y\")\n")

	got := vkeys(mustScan(t, root))
	want := []vkey{
		{path: "a/b/c.go", line: 5, rule: "unprefixed"},
		{path: "a/d.go", line: 5, rule: "unprefixed"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("nested = %#v, want %#v", got, want)
	}
}

// TestScanSkipsDirsAndNonGo pins the walk filter: hidden, vendor,
// node_modules, and generated subtrees are pruned, and non-.go files ignored.
func TestScanSkipsDirsAndNonGo(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	bad := "package p\n\nimport \"errors\"\n\nvar _ = errors.New(\"x\")\n"
	mustWrite(t, root, "pkg/keep.go", bad)
	mustWrite(t, root, "pkg/notes.txt", bad)
	mustWrite(t, root, "vendor/v.go", bad)
	mustWrite(t, root, ".hidden/h.go", bad)
	mustWrite(t, root, "node_modules/n.go", bad)
	mustWrite(t, root, "generated/g.go", bad)

	got := vkeys(mustScan(t, root))
	want := []vkey{{path: "pkg/keep.go", line: 5, rule: "unprefixed"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("filtered = %#v, want %#v", got, want)
	}
}

// mustScan runs scan and fails on error.
func mustScan(t *testing.T, root string) []violation {
	t.Helper()

	vs, err := scan(root)
	if err != nil {
		t.Fatalf("scan(%s): %v", root, err)
	}

	return vs
}

// TestBracketPrefixRegex pins the anchored bracket-prefix matcher.
func TestBracketPrefixRegex(t *testing.T) {
	t.Parallel()

	tests := []struct {
		in   string
		want bool
	}{
		{"[CODE] x", true},
		{"[]", true},
		{"[a b] c", true},
		{"x [y]", false},
		{"[unclosed", false},
		{"[multi\nline]", false},
		{"", false},
	}
	for _, tc := range tests {
		if got := bracketPrefix.MatchString(tc.in); got != tc.want {
			t.Errorf("bracketPrefix.MatchString(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

// TestCheckFileCallShapes pins the AST call-shape boundaries that decide
// whether a literal is examined at all.
func TestCheckFileCallShapes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		src  string
		want []vkey
	}{
		{
			name: "non-literal argument is skipped",
			src:  "package p\n\nimport \"errors\"\n\nfunc f(msg string) {\n\t_ = errors.New(msg)\n}\n",
			want: nil,
		},
		{
			name: "concatenated literal is skipped",
			src:  "package p\n\nimport \"errors\"\n\nfunc f() {\n\t_ = errors.New(\"a\" + \"b\")\n}\n",
			want: nil,
		},
		{
			name: "colon anywhere suppresses unprefixed",
			src:  "package p\n\nimport \"errors\"\n\nfunc f() {\n\t_ = errors.New(\"http://example.com\")\n}\n",
			want: nil,
		},
		{
			name: "other selector is ignored",
			src:  "package p\n\ntype t struct{}\n\nfunc (t) New(s string) error { return nil }\n\nfunc f() {\n\tvar q t\n\t_ = q.New(\"x\")\n}\n",
			want: nil,
		},
		{
			name: "empty brackets are a bracket prefix",
			src:  "package p\n\nimport \"errors\"\n\nfunc f() {\n\t_ = errors.New(\"[]\")\n}\n",
			want: []vkey{
				{path: "pkg/shapes.go", line: 6, rule: "bracket-prefix"},
				{path: "pkg/shapes.go", line: 6, rule: "unprefixed"},
			},
		},
		{
			name: "bare New ident is examined",
			src:  "package p\n\nfunc New(s string) error { return nil }\n\nfunc f() {\n\t_ = New(\"x\")\n}\n",
			want: []vkey{{path: "pkg/shapes.go", line: 6, rule: "unprefixed"}},
		},
		{
			name: "bare Errorf ident with two w verbs",
			src:  "package p\n\nfunc Errorf(f string) error { return nil }\n\nfunc f() {\n\t_ = Errorf(\"%w %w\")\n}\n",
			want: []vkey{
				{path: "pkg/shapes.go", line: 6, rule: "double-%w"},
				{path: "pkg/shapes.go", line: 6, rule: "unprefixed"},
			},
		},
		{
			name: "single w verb is clean",
			src:  "package p\n\nimport (\n\t\"errors\"\n\t\"fmt\"\n)\n\nvar sentinel = errors.New(\"p: sentinel\")\n\nfunc f() error {\n\treturn fmt.Errorf(\"p: %w\", sentinel)\n}\n",
			want: nil,
		},
		{
			name: "no arguments is skipped",
			src:  "package p\n\nimport \"errors\"\n\nfunc f() {\n\t_ = errors.New()\n}\n",
			want: nil,
		},
		{
			name: "non-string literal is skipped",
			src:  "package p\n\nimport \"errors\"\n\nfunc f() {\n\t_ = errors.New(42)\n}\n",
			want: nil,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := vkeys(checkSource(t, "pkg/shapes.go", tc.src))
			if len(got) == 0 {
				got = nil
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("shapes = %#v, want %#v", got, tc.want)
			}
		})
	}
}

// TestCheckFileAllowMarkerInStringNotComment pins that a marker inside a
// string literal does not suppress anything.
func TestCheckFileAllowMarkerInStringNotComment(t *testing.T) {
	t.Parallel()

	src := "package p\n\nimport \"errors\"\n\nvar _ = \"errscan:allow\"\n\nfunc f() {\n\t_ = errors.New(\"x\")\n}\n"
	got := vkeys(checkSource(t, "pkg/s.go", src))
	want := []vkey{{path: "pkg/s.go", line: 8, rule: "unprefixed"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("marker-in-string = %#v, want %#v", got, want)
	}
}

// TestCheckFileStandaloneAllowMidFile pins that a standalone allow comment
// anywhere in the file suppresses the whole file.
func TestCheckFileStandaloneAllowMidFile(t *testing.T) {
	t.Parallel()

	src := "package p\n\nimport \"errors\"\n\n// errscan:allow\n\nfunc f() {\n\t_ = errors.New(\"x\")\n}\n"
	if vs := checkSource(t, "pkg/s.go", src); len(vs) != 0 {
		t.Errorf("mid-file allow = %#v, want none", vs)
	}
}
