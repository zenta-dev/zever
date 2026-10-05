package main

import (
	"bytes"
	"strings"
	"testing"

	"go.lsp.dev/protocol"

	"github.com/zenta-dev/zever/dsl/parser"
)

// TestStdioRW pins the trivial adapter contract: Read/Write pass through to
// the wrapped stream and Close is a no-op.
func TestStdioRW(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	rw := stdioRW{stdin: strings.NewReader("hello"), stdout: &out}

	buf := make([]byte, 5)
	if n, err := rw.Read(buf); err != nil || n != 5 || string(buf) != "hello" {
		t.Fatalf("Read = (%d, %v) %q, want (5, nil) hello", n, err, buf)
	}
	if _, err := rw.Write([]byte("world")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if out.String() != "world" {
		t.Errorf("Write output = %q, want world", out.String())
	}
	if err := rw.Close(); err != nil {
		t.Errorf("Close = %v, want nil", err)
	}
}

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

// TestValidateScalarAt pins the scalar-type recovery behind @validate arg
// completion: the declared type is found, and every non-validate or ambiguous
// position yields "".
func TestValidateScalarAt(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		src  string
		line uint32
		col  uint32
		want string
	}{
		{"field scalar is recovered", "entity User {\n id: string @validate(\n}\n", 1, 22, "string"},
		{"other attribute is not validate", "entity User {\n id: string @default(\n}\n", 1, 22, ""},
		{"brace before any colon", "x { @validate(\n", 0, 14, ""},
		{"no colon on the line", "entity User {\n @validate(\n}\n", 1, 11, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := validateScalarAt(tc.src, protocol.Position{Line: tc.line, Character: tc.col})
			if got != tc.want {
				t.Errorf("validateScalarAt = %q, want %q", got, tc.want)
			}
		})
	}
}
