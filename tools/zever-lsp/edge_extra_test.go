package main

import (
	"testing"

	"go.lsp.dev/protocol"

	"github.com/zenta-dev/zever/dsl/parser"
)

// TestCompletionAtEmptyFile pins that a completion request on an empty file
// returns top-level keywords instead of panicking.
func TestCompletionAtEmptyFile(t *testing.T) {
	t.Parallel()

	const src = ""
	file, _ := parser.New("empty.zen", []byte(src)).ParseFile()

	items := completionAt(nil, file, src, protocol.Position{Line: 0, Character: 0})
	if !containsLabel(completionLabels(items), "entity") {
		t.Fatal("empty-file completion missing entity keyword")
	}
}

// TestHighlightsAtEmptyFile pins that document highlights on an empty file
// return nothing.
func TestHighlightsAtEmptyFile(t *testing.T) {
	t.Parallel()

	const src = ""
	file, _ := parser.New("empty.zen", []byte(src)).ParseFile()

	if got := highlightsAt(file, protocol.Position{Line: 0, Character: 0}); len(got) != 0 {
		t.Fatalf("highlights on empty file = %v, want none", got)
	}
}

// TestHoverAtOutOfRangePosition pins that an out-of-range cursor yields no
// hover rather than an error or panic.
func TestHoverAtOutOfRangePosition(t *testing.T) {
	t.Parallel()

	const src = "entity User {\n\tid: uuid @primary\n}\n"
	file, _ := parser.New("main.zen", []byte(src)).ParseFile()

	if h := hoverAt(nil, file, protocol.Position{Line: 99, Character: 99}); h != nil {
		t.Fatalf("hover at out-of-range = %v, want nil", h)
	}
}
