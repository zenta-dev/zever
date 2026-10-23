package main

import (
	"strings"

	"go.lsp.dev/protocol"
)

// applyIncrementalEdit applies one incremental (partial) content change to
// content and returns the resulting document. A whole-document change arm is
// also accepted and replaces content outright. Malformed or out-of-range
// edits leave content unchanged.
//
// The LSP Range is interpreted byte-ish (line/character as byte offsets),
// matching the rest of this server; UTF-16 conversion is intentionally not
// performed.
func applyIncrementalEdit(content string, change any) string {
	partial, whole := classifyChange(change)

	if whole != nil {
		return whole.Text
	}

	if partial == nil {
		return content
	}

	startOff, ok := positionToOffset(content, partial.Range.Start)
	if !ok {
		return content
	}

	endOff, ok := positionToOffset(content, partial.Range.End)
	if !ok {
		return content
	}

	if startOff > endOff || startOff < 0 || endOff > len(content) {
		return content
	}

	return content[:startOff] + partial.Text + content[endOff:]
}

// classifyChange extracts a partial/whole arm from the various concrete and
// interface forms a client or test may pass.
func classifyChange(change any) (*protocol.TextDocumentContentChangePartial, *protocol.TextDocumentContentChangeWholeDocument) {
	switch v := change.(type) {
	case *protocol.TextDocumentContentChangePartial:
		return v, nil
	case *protocol.TextDocumentContentChangeWholeDocument:
		return nil, v
	case protocol.TextDocumentContentChangeEvent:
		if p, ok := v.(*protocol.TextDocumentContentChangePartial); ok {
			return p, nil
		}

		if w, ok := v.(*protocol.TextDocumentContentChangeWholeDocument); ok {
			return nil, w
		}
	case *protocol.TextDocumentContentChangeEvent:
		if v == nil {
			return nil, nil
		}

		if p, ok := (*v).(*protocol.TextDocumentContentChangePartial); ok {
			return p, nil
		}

		if w, ok := (*v).(*protocol.TextDocumentContentChangeWholeDocument); ok {
			return nil, w
		}
	}

	return nil, nil
}

// positionToOffset converts a 0-based LSP position (line/character) into a
// byte offset into content. It returns ok=false for out-of-range or malformed
// positions.
func positionToOffset(content string, pos protocol.Position) (int, bool) {
	line := int(pos.Line)
	char := int(pos.Character)
	if line < 0 || char < 0 {
		return 0, false
	}

	newlines := strings.Count(content, "\n")
	if line > newlines {
		return 0, false
	}

	// Walk to the start of the requested line.
	i := 0
	currentLine := 0
	for currentLine < line {
		idx := strings.IndexByte(content[i:], '\n')
		if idx < 0 {
			return 0, false
		}

		i += idx + 1
		currentLine++
	}

	lineStart := i
	lineEnd := i
	for lineEnd < len(content) && content[lineEnd] != '\n' {
		lineEnd++
	}

	if char > lineEnd-lineStart {
		return 0, false
	}

	return lineStart + char, true
}
