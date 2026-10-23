package main

import (
	"go.lsp.dev/protocol"

	"github.com/zenta-dev/zever/dsl/diag"
	"github.com/zenta-dev/zever/dsl/token"
)

// selectionRangesAt resolves ONE cursor position into the range chain a
// textDocument/selectionRange result carries for it, innermost first. The
// LSP contract the result must satisfy is: result[i].range contains
// params.positions[i], and each range's parent range contains its child,
// so a client can walk .Parent outward to grow the selection.
//
// The chain is built purely from the token stream (lexAll, the shared
// lexical pass folding_range.go uses) in these layers:
//
//  1. The token covering the cursor -- an identifier, keyword or literal --
//     spanned exactly the way semantic_tokens.go spans its lexical tier.
//     Strings need the source for their width: token.Lit stores the
//     unescaped value, not the raw source extent.
//  2. The enclosing declaration head, at line granularity: the whole source
//     line the token sits on. This DSL writes every declaration (a keyword,
//     or a field name when it has no keyword, plus its argument list) on a
//     single line, and an attribute written after the first argument would
//     fall outside any narrower keyword..first-arg span, so the token's
//     whole line is the line-granular declaration head.
//  3. The innermost brace block containing the position: the line of the
//     matching open brace through the line of its close brace, braces
//     paired with the same single-stack pass folding_range.go pairs them
//     with. Whitespace between tokens counts too -- the position, not the
//     token index, anchors the block layer; an unclosed open brace and a
//     stray close brace contribute no layer, degrading exactly like
//     folding does on unpaired input.
//  4. The whole document.
//
// Every accepted layer must contain the cursor, and every outer layer the
// chain already holds, so both containment rules the spec places on
// positions[i] and on .Parent hold by construction; a candidate layer that
// would break either is skipped, and a candidate equal to the chain's top
// is deduplicated (inside a fully single-line block the head line and the
// block range coincide, and inside a top-level block the block range and
// the document range can). A position no candidate contains -- beyond a
// line's right edge with no enclosing block, or past the end of the
// document -- yields an empty chain, and the caller answers with the
// degenerate range the spec anchors at the position itself.
func selectionRangesAt(tokens []token.Token, src string, cursor protocol.Position) []protocol.Range {
	// Candidates are appended innermost-first; each candidate is strictly
	// larger than the last or a duplicate of it.
	candidates := make([]protocol.Range, 0, 4)

	if i := tokenIndexAt(tokens, src, cursor); i >= 0 {
		if rng := tokenRangeAt(tokens[i], src); rng != nil {
			candidates = append(candidates, *rng)
		}
	}

	if rng := lineRangeAt(src, int(cursor.Line)); rng != nil {
		candidates = append(candidates, *rng)
	}

	if rng := braceBlockRangeAt(tokens, src, cursor); rng != nil {
		candidates = append(candidates, *rng)
	}

	if rng := documentRangeAt(src); rng != nil {
		candidates = append(candidates, *rng)
	}

	var chain []protocol.Range

	for _, rng := range candidates {
		if len(chain) == 0 {
			if rangeContainsPos(rng, cursor) {
				chain = append(chain, rng)
			}

			continue
		}

		top := chain[len(chain)-1]
		if rangeEqual(top, rng) || !rangeContains(rng, top) {
			continue
		}

		chain = append(chain, rng)
	}

	return chain
}

// selectionRangeAt resolves one position into a single parent-linked
// SelectionRange: the innermost range from selectionRangesAt's chain, each
// enclosing layer chained through .Parent outward. An empty chain decays to
// the single degenerate range the spec anchors at the position itself,
// which keeps the positions[i] must be contained in result[i].range rule
// satisfied for positions that cover nothing (whitespace past a line's
// text, off-end positions).
func selectionRangeAt(tokens []token.Token, src string, cursor protocol.Position) protocol.SelectionRange {
	chain := selectionRangesAt(tokens, src, cursor)
	if len(chain) == 0 {
		return protocol.SelectionRange{Range: protocol.Range{Start: cursor, End: cursor}}
	}

	var node *protocol.SelectionRange

	for i := len(chain) - 1; i >= 0; i-- {
		node = &protocol.SelectionRange{Range: chain[i], Parent: node}
	}

	return *node
}

// documentRangeAt spans the whole document: from the start of its first
// line through the last character of its last line's text (not through the
// trailing terminator, so the empty line after a final newline is
// off-document, the way editors treat it).
func documentRangeAt(src string) *protocol.Range {
	lines := splitSourceLines(src)
	if len(lines) == 0 {
		return nil
	}

	return &protocol.Range{
		Start: protocol.Position{Line: 0, Character: 0},
		End: protocol.Position{
			Line:      uint32(len(lines) - 1),                 //nolint:gosec // non-negative line count
			Character: uint32(lineWidth(lines[len(lines)-1])), //nolint:gosec // non-negative width
		},
	}
}

// tokenRangeAt spans one token exactly the way semantic_tokens.go spans its
// lexical tier: literal-wide for identifiers, keywords, numbers and
// durations, and source-measured for strings, whose escaped form token.Lit
// never describes. An unpositioned token (a marker the parse recovered
// past) yields nil rather than an empty range on the chain.
func tokenRangeAt(tok token.Token, src string) *protocol.Range {
	width := tokenWidthAt(tok, src)
	if width <= 0 {
		return nil
	}

	start := protocol.Position{
		Line:      uint32(tok.Pos.Line - 1), //nolint:gosec // lexer positions non-negative
		Character: uint32(tok.Pos.Col - 1),  //nolint:gosec // lexer positions non-negative
	}

	return &protocol.Range{
		Start: start,
		End:   protocol.Position{Line: start.Line, Character: start.Character + uint32(width)}, //nolint:gosec // non-negative
	}
}

// tokenWidthAt measures a token's width in UTF-16 code units, the unit all
// LSP ranges measure lengths in. Identifiers, keywords, numbers and
// durations are ASCII-only (the lexer admits nothing else into them), so
// their width is the byte length of Lit; a string literal is measured off
// the source because Lit stores its unescaped value.
func tokenWidthAt(tok token.Token, src string) int {
	if tok.Kind == token.STRING {
		return stringTokenWidthAt(src, tok)
	}

	return lineWidth(tok.Lit)
}

// stringTokenWidthAt measures the raw source span of the string literal
// starting at tok.Pos, quotes and escapes included, so an escaped or
// multi-byte literal is reported over its true extent exactly like the
// semantic-token tier does. An unterminated string measures to nothing: the
// lexer has already flagged it, and the chain degrades to the next layer.
func stringTokenWidthAt(src string, tok token.Token) int {
	lines := splitSourceLines(src)
	line := tok.Pos.Line - 1

	if line < 0 || line >= len(lines) {
		return 0
	}

	start := tok.Pos.Col - 1
	text := lines[line]

	if start < 0 || start >= len(text) || text[start] != '"' {
		return 0
	}

	// Every escape the lexer recognizes is backslash plus exactly one byte,
	// and unrecognized ones round-trip byte-for-byte, so skipping one byte
	// per backslash is right in both cases.
	for end := start + 1; end < len(text); end++ {
		switch text[end] {
		case '"':
			return lineWidth(text[start : end+1])
		case '\\':
			end++
		}
	}

	return 0
}

// lineRangeAt spans the 0-based LSP line from its first character through
// the last, or nil when the line number is out of the document's range.
func lineRangeAt(src string, line int) *protocol.Range {
	lines := splitSourceLines(src)
	if line < 0 || line >= len(lines) {
		return nil
	}

	return &protocol.Range{
		Start: protocol.Position{Line: uint32(line), Character: 0},                              //nolint:gosec // non-negative
		End:   protocol.Position{Line: uint32(line), Character: uint32(lineWidth(lines[line]))}, //nolint:gosec // non-negative
	}
}

// braceBlockRangeAt spans the innermost brace pair surrounding the cursor:
// the line of the matching open brace through the last character of its
// close brace's line, braces paired with the same single-stack pass
// folding_range.go pairs them with. The position, not a token index, picks
// the pair, so whitespace between tokens inside a block still yields its
// enclosing block. A cursor inside no pair (top-level text, a line before
// any brace, an unpaired brace) yields nil.
func braceBlockRangeAt(tokens []token.Token, src string, cursor protocol.Position) *protocol.Range {
	// Pair every brace with its match in one stack pass: match[i] is the
	// token index of the brace paired with token i, or -1 when unpaired.
	match := make([]int, len(tokens))
	for i := range match {
		match[i] = -1
	}

	stack := make([]int, 0, len(tokens))

	for i, tok := range tokens {
		switch tok.Kind { //nolint:exhaustive // only braces affect the pairing stack
		case token.LBRACE:
			stack = append(stack, i)
		case token.RBRACE:
			if len(stack) == 0 {
				continue // stray close brace: no matching open, no layer
			}

			open := stack[len(stack)-1]
			stack = stack[:len(stack)-1]

			match[open], match[i] = i, open
		}
	}

	best := -1 // index of the innermost containing pair's open brace

	for i, tok := range tokens {
		if tok.Kind != token.LBRACE || match[i] < 0 {
			continue
		}

		if insideBracePair(cursor, tok.Pos, tokens[match[i]].Pos) {
			// Pairs containing the cursor nest, so the pair whose open brace
			// comes last is the innermost one.
			if best < 0 || i > best {
				best = i
			}
		}
	}

	if best < 0 {
		return nil
	}

	open, closeBrace := tokens[best], tokens[match[best]]

	return &protocol.Range{
		Start: protocol.Position{Line: uint32(open.Pos.Line - 1), Character: 0}, //nolint:gosec // lexer positions non-negative
		End: protocol.Position{
			Line:      uint32(closeBrace.Pos.Line - 1),                           //nolint:gosec // lexer positions non-negative
			Character: uint32(lineWidth(lineSourceAt(src, closeBrace.Pos.Line))), //nolint:gosec // non-negative
		},
	}
}

// insideBracePair reports whether the 0-based LSP cursor lies within the
// inclusive span between the 1-based open and close brace positions,
// comparing lexicographically by line then column.
func insideBracePair(cursor protocol.Position, open, closePos diag.Position) bool {
	line := int(cursor.Line) + 1
	col := int(cursor.Character) + 1

	if line < open.Line || (line == open.Line && col < open.Col) {
		return false
	}

	if line > closePos.Line || (line == closePos.Line && col > closePos.Col) {
		return false
	}

	return true
}

// lineSourceAt returns the text of the 1-based source line, without its
// terminator, or empty for a line the document does not have.
func lineSourceAt(src string, oneBased int) string {
	if oneBased < 1 {
		return ""
	}

	lines := splitSourceLines(src)
	if oneBased > len(lines) {
		return ""
	}

	return lines[oneBased-1]
}

// tokenIndexAt returns the index of the token covering the cursor, or -1
// when the position falls between tokens, past every token, or at the EOF
// token, which spans nothing.
func tokenIndexAt(tokens []token.Token, src string, cursor protocol.Position) int {
	for i, tok := range tokens {
		if tok.Kind == token.EOF {
			continue
		}

		if tokenCovers(tok, src, cursor) {
			return i
		}
	}

	return -1
}

// tokenCovers reports whether the cursor falls inside the token's span,
// converted from the lexer's 1-based row/column to 0-based LSP positions,
// with the same inclusive trailing edge coversIdent grants identifiers so a
// click just past a word's last character still resolves to it.
func tokenCovers(tok token.Token, src string, cursor protocol.Position) bool {
	if tok.Pos.Line <= 0 || tok.Pos.Col <= 0 {
		return false
	}

	if int(cursor.Line) != tok.Pos.Line-1 {
		return false
	}

	col := int(cursor.Character) + 1

	width := tokenWidthAt(tok, src)

	return col >= tok.Pos.Col && col <= tok.Pos.Col+width
}

// lineWidth returns text's length in UTF-16 code units, the unit every LSP
// range measures lengths in. Identifiers, keywords and numbers in this DSL
// are ASCII, so for them this agrees with the byte length; the code points
// needing surrogate pairs can only occur inside string literals and
// comments, where a range is otherwise measured off the source.
func lineWidth(text string) int {
	width := 0

	for _, r := range text {
		if r >= 0x10000 {
			width += 2 // surrogate pair

			continue
		}

		width++
	}

	return width
}

// splitSourceLines splits src at newlines, keeping empty lines, so every
// index maps 1:1 onto an LSP 0-based row.
func splitSourceLines(src string) []string {
	if src == "" {
		return nil
	}

	lines := []string{}
	start := 0

	for i := 0; i < len(src); i++ {
		if src[i] == '\n' {
			lines = append(lines, trimCR(src[start:i]))
			start = i + 1
		}
	}

	return append(lines, trimCR(src[start:]))
}

// trimCR drops a trailing carriage return so CRLF documents measure line
// widths to the last text character exactly like LF documents.
func trimCR(text string) string {
	if len(text) > 0 && text[len(text)-1] == '\r' {
		return text[:len(text)-1]
	}

	return text
}

// rangeEqual reports whether a and b span identical LSP ranges.
func rangeEqual(a, b protocol.Range) bool {
	return a.Start == b.Start && a.End == b.End
}

// rangeContains reports whether outer fully contains inner, both endpoint
// checks inclusive, the rule the LSP spec requires of every
// SelectionRange's parent.
func rangeContains(outer, inner protocol.Range) bool {
	return rangeContainsPos(outer, inner.Start) && rangeWithinEnd(outer, inner.End)
}

// rangeContainsPos reports whether rng covers p, both edges inclusive.
func rangeContainsPos(rng protocol.Range, p protocol.Position) bool {
	if p.Line < rng.Start.Line || p.Line > rng.End.Line {
		return false
	}

	if p.Line == rng.Start.Line && p.Character < rng.Start.Character {
		return false
	}

	if p.Line == rng.End.Line && p.Character > rng.End.Character {
		return false
	}

	return true
}

// rangeWithinEnd reports whether p lies at or before rng's end.
func rangeWithinEnd(rng protocol.Range, p protocol.Position) bool {
	if p.Line > rng.End.Line {
		return false
	}

	return p.Line < rng.End.Line || p.Character <= rng.End.Character
}
