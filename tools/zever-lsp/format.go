package main

import (
	"go.lsp.dev/protocol"

	"github.com/zenta-dev/zever/dsl/format"
	"github.com/zenta-dev/zever/dsl/token"
)

// The whole-document, protocol-agnostic formatting core (token-stream-based
// indentation and inter-token spacing) lives in dsl/format, shared
// with the zever fmt CLI command. What stays here is genuinely
// LSP-protocol-specific: converting the shared package's plain-int Edit
// values into protocol.TextEdit, and range-scoped formatting
// (filterEditsToRange), which has no meaning outside the LSP protocol.

// sourceLine, splitLines and offsetOf are thin aliases over the shared
// package, kept under these names because format_test.go exercises them
// directly (as package-private helpers) to build its own edit-application
// assertions.
type sourceLine = format.Line

func splitLines(src []byte) []sourceLine { return format.SplitLines(src) }

func offsetOf(src []byte, line sourceLine, col int) int { return format.OffsetOf(src, line, col) }

// lexAll and canonicalGap likewise alias the shared package; format_test.go
// calls them directly to assert lexer cleanliness and gap rules.
func lexAll(path string, src []byte) ([]token.Token, bool) { return format.LexAll(path, src) }

func canonicalGap(prev, next token.Token) (string, bool) { return format.CanonicalGap(prev, next) }

// formatDocument returns the edits that rewrite src into canonical zen
// style, as LSP protocol.TextEdit values. It returns nil when the lexer
// reports any error, so a document that is mid-edit or genuinely broken is
// never mangled.
func formatDocument(path string, src []byte) []protocol.TextEdit {
	edits, ok := format.Edits(path, src)
	if !ok {
		return nil
	}

	out := make([]protocol.TextEdit, len(edits))
	for i, e := range edits {
		out[i] = protocol.TextEdit{
			Range:   lineRange(e.Line, e.StartCol, e.EndCol),
			NewText: e.NewText,
		}
	}

	return out
}

// filterEditsToRange keeps only edits overlapping rng, using a line-overlap
// test rather than strict containment: an edit whose span straddles the
// requested range's boundary is kept whole rather than dropped or
// truncated, since indentation/spacing edits are scoped to single lines or
// single token gaps and are not safely splittable.
func filterEditsToRange(edits []protocol.TextEdit, rng protocol.Range) []protocol.TextEdit {
	var kept []protocol.TextEdit

	for _, edit := range edits {
		if edit.Range.Start.Line <= rng.End.Line && edit.Range.End.Line >= rng.Start.Line {
			kept = append(kept, edit)
		}
	}

	return kept
}

// lineRange builds a single-line LSP range from 0-based columns.
func lineRange(line, startCol, endCol int) protocol.Range {
	if startCol < 0 {
		startCol = 0
	}

	if endCol < startCol {
		endCol = startCol
	}

	return protocol.Range{
		Start: protocol.Position{
			Line:      uint32(line), //nolint:gosec // line is a 0-based lexer counter, never negative
			Character: uint32(startCol),
		},
		End: protocol.Position{
			Line:      uint32(line), //nolint:gosec // line is a 0-based lexer counter, never negative
			Character: uint32(endCol),
		},
	}
}
