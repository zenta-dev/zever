package main

import (
	"strings"
	"testing"

	"go.lsp.dev/protocol"

	"github.com/zenta-dev/zever/dsl/ast"
	"github.com/zenta-dev/zever/dsl/compile"
	"github.com/zenta-dev/zever/dsl/ir"
	"github.com/zenta-dev/zever/dsl/parser"
)

// benchCursor returns the LSP position of the first character of needle on
// the given 0-based line of src.
func benchCursor(line int, needle string) protocol.Position {
	col := strings.Index(strings.Split(hoverSrc, "\n")[line], needle)
	if col < 0 {
		panic("needle not found: " + needle)
	}
	return protocol.Position{Line: uint32(line), Character: uint32(col)} //nolint:gosec // test fixture offsets
}

// benchSchema compiles the hover fixture once for the benchmarks.
func benchSchema(b *testing.B) (*ir.Schema, *ast.File) {
	b.Helper()

	result, diags := compile.Compile(map[string]string{"main.zen": hoverSrc})
	if diags.HasErrors() {
		b.Fatalf("compile: %v", diags)
	}
	file, _ := parser.New("main.zen", []byte(hoverSrc)).ParseFile()
	return result.Schema, file
}

// BenchmarkHoverAt measures a hover request over a parsed schema.
func BenchmarkHoverAt(b *testing.B) {
	schema, file := benchSchema(b)
	cursor := benchCursor(2, "email")

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if h := hoverAt(schema, file, cursor); h == nil {
			b.Fatal("nil hover")
		}
	}
}

// BenchmarkCompletionAt measures a field-type completion request.
func BenchmarkCompletionAt(b *testing.B) {
	schema, file := benchSchema(b)
	cursor := benchCursor(8, "uuid")

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if items := completionAt(schema, file, hoverSrc, cursor); len(items) == 0 {
			b.Fatal("no completion items")
		}
	}
}

// BenchmarkDefinitionAt measures a cross-entity definition request.
func BenchmarkDefinitionAt(b *testing.B) {
	schema, file := benchSchema(b)
	cursor := benchCursor(9, "User")

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = definitionAt(schema, file, cursor)
	}
}

// BenchmarkHighlightsAt measures document-highlight computation.
func BenchmarkHighlightsAt(b *testing.B) {
	_, file := benchSchema(b)
	cursor := benchCursor(0, "User")

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = highlightsAt(file, cursor)
	}
}
