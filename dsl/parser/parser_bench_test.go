package parser

import (
	"os"
	"strings"
	"testing"
)

// BenchmarkParseFileAppFixture parses the canonical compile/testdata/app.zen
// fixture: two entities with a bidirectional relation, two services with
// four RPCs total, exercising every top-level production at realistic size.
func BenchmarkParseFileAppFixture(b *testing.B) {
	src, err := os.ReadFile("../compile/testdata/app.zen")
	if err != nil {
		b.Fatalf("read fixture: %v", err)
	}

	b.ReportAllocs()
	b.SetBytes(int64(len(src)))
	for b.Loop() {
		p := New("app.zen", src)
		if _, diags := p.ParseFile(); diags.HasErrors() {
			b.Fatalf("parse: %v", diags)
		}
	}
}

// BenchmarkParseFileMinimal is the boundary case: the smallest meaningful
// declaration (one entity, one field).
func BenchmarkParseFileMinimal(b *testing.B) {
	src := []byte("entity User {\n  id: uuid @primary\n}\n")

	b.ReportAllocs()
	b.SetBytes(int64(len(src)))
	for b.Loop() {
		p := New("min.zen", src)
		if _, diags := p.ParseFile(); diags.HasErrors() {
			b.Fatalf("parse: %v", diags)
		}
	}
}

// BenchmarkParseFileMalformed measures the error-recovery path: a broken
// declaration followed by a valid one forces syncTopLevel to resync before
// the good declaration parses.
func BenchmarkParseFileMalformed(b *testing.B) {
	src := []byte("entity Broken { !!! } entity Good {\n  id: uuid @primary\n}\n")

	b.ReportAllocs()
	b.SetBytes(int64(len(src)))
	for b.Loop() {
		p := New("mixed.zen", src)
		_, _ = p.ParseFile()
	}
}

// BenchmarkParseFileDeepNesting drives the value-recursion depth guard
// (maxValueDepth = 200): nested call expressions one level past the limit
// must be rejected with a diagnostic, not a stack overflow.
func BenchmarkParseFileDeepNesting(b *testing.B) {
	src := []byte("entity Deep { id: uuid @validate(x(" + strings.Repeat("y(", 250) + "1" + strings.Repeat(")", 250) + ")) }")

	b.ReportAllocs()
	b.SetBytes(int64(len(src)))
	for b.Loop() {
		p := New("deep.zen", src)
		_, _ = p.ParseFile()
	}
}

// BenchmarkParseFileManyDecls measures throughput over a wide file: 500
// small entities, stressing the top-level parse loop and sync paths.
func BenchmarkParseFileManyDecls(b *testing.B) {
	var sb strings.Builder
	for i := 0; i < 500; i++ {
		sb.WriteString("entity E")
		sb.WriteString(strings.Repeat("x", i%7))
		sb.WriteString(" {\n  id: uuid @primary\n}\n")
	}

	src := []byte(sb.String())

	b.ReportAllocs()
	b.SetBytes(int64(len(src)))
	for b.Loop() {
		p := New("wide.zen", src)
		if _, diags := p.ParseFile(); diags.HasErrors() {
			b.Fatalf("parse: %v", diags)
		}
	}
}
