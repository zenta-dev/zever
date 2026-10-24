package wire

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"testing"
)

// BenchmarkClassify measures the struct-tag classification hot path.
func BenchmarkClassify(b *testing.B) {
	tags := []string{
		"",
		`zever:"secret"`,
		`json:"id"`,
		`json:"id,omitempty"`,
		`json:""`,
		`json:",omitempty"`,
		`json:"-"`,
	}
	i := 0

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		_ = Classify(tags[i%len(tags)])
		i++
	}
}

// BenchmarkFieldState measures the Field.State hot path.
func BenchmarkFieldState(b *testing.B) {
	fields := []Field{
		{Name: "ID", Tag: `json:"id"`},
		{Name: "ID", Tag: `json:"id,omitempty"`},
		{Name: "ID", Tag: `json:""`},
		{Name: "ID"},
		{Name: "ID", Tag: `json:"-"`},
		{Name: "ID", Tag: `zever:"secret"`},
	}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		for _, f := range fields {
			_ = f.State()
		}
	}
}

// BenchmarkFieldKey measures the Field.Key hot path.
func BenchmarkFieldKey(b *testing.B) {
	fields := []Field{
		{Name: "ID", Tag: `json:"id"`},
		{Name: "ID", Tag: `json:"id,omitempty"`},
		{Name: "ID", Tag: `json:""`},
		{Name: "ID"},
		{Name: "ID", Tag: `json:"-"`},
		{Name: "ID", Tag: `zever:"secret"`},
	}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		for _, f := range fields {
			_, _ = f.Key()
		}
	}
}

// BenchmarkFields measures struct-field extraction from a parsed file.
func BenchmarkFields(b *testing.B) {
	const source = "package pkg\n\n" +
		"type wire struct {\n" +
		"\tName   string `json:\"name\"`\n" +
		"\tCount  int    `json:\"count,omitempty\"`\n" +
		"\tHidden string `json:\"-\"`\n" +
		"\tPlain  string\n" +
		"\tsize   int\n" +
		"\tInner\n" +
		"}\n"

	fset := token.NewFileSet()

	f, err := parser.ParseFile(fset, "wire.go", source, parser.SkipObjectResolution)
	if err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		_, _ = Fields(f, "wire")
	}
}

// BenchmarkSkipDir measures the directory-skip hot path.
func BenchmarkSkipDir(b *testing.B) {
	dirs := []string{
		".git", ".worktrees", ".claude", "vendor", "node_modules",
		"generated", "config", "container", "orm", "wire",
	}
	i := 0

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		_ = SkipDir(dirs[i%len(dirs)])
		i++
	}
}

// BenchmarkGenerated measures the generated-file marker check.
func BenchmarkGenerated(b *testing.B) {
	const source = "package pkg\n\ntype wire struct {\n\tName string `json:\"name\"`\n}\n"

	fset := token.NewFileSet()

	f, err := parser.ParseFile(fset, "wire.go", source, parser.ParseComments)
	if err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		_ = Generated(fset, f)
	}
}

// BenchmarkWalkStructs measures the file-walk struct collection path.
func BenchmarkWalkStructs(b *testing.B) {
	root := b.TempDir()

	const source = "package pkg\n\n" +
		"type alpha struct {\n\tName string `json:\"name\"`\n}\n\n" +
		"type beta struct {\n\tID int `json:\"id\"`\n}\n"

	for _, name := range []string{"a.go", "sub/b.go"} {
		path := filepath.Join(root, name)

		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			b.Fatal(err)
		}

		if err := os.WriteFile(path, []byte(source), 0o600); err != nil {
			b.Fatal(err)
		}
	}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		_ = WalkStructs(root, func(_, _ string, _ []Field) error { return nil })
	}
}
