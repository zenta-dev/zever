// Package format implements canonical formatting for the zen schema DSL.
//
// It is deliberately not part of a parse/pretty-print pipeline. The lexer
// discards "//" comments entirely: skipLineComment consumes them without
// emitting a token and without recording their position anywhere, so they
// are absent from the AST too. A "parse, pretty-print the tree, replace the
// document" formatter would therefore delete every comment in the file --
// including the ones in this repo's own .zen schemas.
//
// Because the DSL has only single-line comments, any comment between two
// tokens necessarily pushes the following token onto a later line. So two
// tokens the lexer reports on the SAME line are provably comment-free, and
// the text between them is pure whitespace. The formatter exploits that: it
// only ever rewrites (a) a line's leading indentation and (b) the gap
// between two tokens on one line. Multi-line gaps -- the only place a
// comment or a blank line can live -- are never regenerated, so comments
// survive structurally rather than by special-casing.
//
// Scope limits for this version: blank-line counts between declarations are
// left alone, comment text is never reflowed, and a file the lexer reports
// any error for is left completely untouched (like gofmt refusing to
// format broken source).
//
// This package holds the whole-document, protocol-agnostic core: given
// source bytes, compute canonical edits (Edits) or the fully formatted
// output (Format). tools/zen-lsp wraps Edits to build LSP protocol.TextEdit
// values and to support partial-range formatting -- concerns specific to
// the LSP protocol that have no place in this package.
package format

import (
	"errors"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/zenta-dev/zever/internal/dsl/lexer"
	"github.com/zenta-dev/zever/internal/dsl/token"
)

// ErrDirty is returned by Format when src has lexical errors and is
// therefore left unformatted, mirroring gofmt's refusal to format broken
// source.
var ErrDirty = errors.New("[format] source has lexical errors, refusing to format")

// indentUnit is one level of indentation. The DSL's own schemas use two
// spaces; this is a fixed-style formatter, not a configurable one.
const indentUnit = "  "

// Line records one source line's byte span, excluding its line terminator.
type Line struct {
	start int
	end   int
}

// lineState is what the token stream tells us about one source line: the
// brace depth in effect when the line begins, and the first token on it
// (if any). Lines holding only a comment or only whitespace have no token
// of their own and simply carry the surrounding depth, which is what makes
// them reindent correctly with no special case.
type lineState struct {
	depth     int
	firstKind token.Kind
	hasToken  bool
}

// Edit describes one rewrite: replace the half-open column range
// [StartCol, EndCol) on Line (0-based) with NewText. Columns are byte
// offsets from the start of the line -- valid because every rewritten span
// (leading indentation, an inter-token gap on one line) is pure ASCII
// whitespace by construction; see the package doc.
type Edit struct {
	Line     int
	StartCol int
	EndCol   int
	NewText  string
}

// Edits computes the edits that rewrite src into canonical zen style. It
// returns ok=false when the lexer reports any error, leaving a mid-edit or
// genuinely broken document untouched.
func Edits(path string, src []byte) (edits []Edit, ok bool) {
	tokens, clean := LexAll(path, src)
	if !clean {
		return nil, false
	}

	lines := SplitLines(src)
	states := lineStates(len(lines), tokens)

	edits = indentEdits(src, lines, states)
	edits = append(edits, blankLineEdits(src, lines, states)...)
	edits = append(edits, spacingEdits(src, lines, tokens)...)

	sort.Slice(edits, func(i, j int) bool {
		if edits[i].Line != edits[j].Line {
			return edits[i].Line < edits[j].Line
		}

		return edits[i].StartCol < edits[j].StartCol
	})

	return edits, true
}

// Format returns src rewritten into canonical zen style. It returns
// ErrDirty when src has lexical errors instead of silently returning it
// unchanged, so callers can distinguish "already formatted" from "can't be
// formatted".
func Format(src []byte) ([]byte, error) {
	edits, ok := Edits("", src)
	if !ok {
		return nil, ErrDirty
	}

	return Apply(src, edits), nil
}

// Apply rewrites src by applying edits, which are expected sorted by
// (Line, StartCol) ascending -- the order Edits returns them in. Overlapping
// edits are defensively skipped rather than applied out of order.
func Apply(src []byte, edits []Edit) []byte {
	if len(edits) == 0 {
		out := make([]byte, len(src))
		copy(out, src)

		return out
	}

	lines := SplitLines(src)

	out := make([]byte, 0, len(src))
	prevEnd := 0

	for _, e := range edits {
		if e.Line < 0 || e.Line >= len(lines) {
			continue
		}

		line := lines[e.Line]
		start := clampOffset(line, e.StartCol)
		end := clampOffset(line, e.EndCol)

		if start < prevEnd {
			continue // overlapping edit: skip rather than corrupt output
		}

		out = append(out, src[prevEnd:start]...)
		out = append(out, e.NewText...)
		prevEnd = end
	}

	out = append(out, src[prevEnd:]...)

	return out
}

func clampOffset(line Line, col int) int {
	off := line.start + col
	if off < line.start {
		return line.start
	}

	if off > line.end {
		return line.end
	}

	return off
}

// LexAll drains the lexer into a slice, dropping the terminating EOF. The
// bool reports whether scanning was error-free.
func LexAll(path string, src []byte) ([]token.Token, bool) {
	lex := lexer.New(path, src)

	var tokens []token.Token

	for {
		tok := lex.Next()
		if tok.Kind == token.EOF {
			break
		}

		tokens = append(tokens, tok)
	}

	return tokens, len(lex.Errors()) == 0
}

// SplitLines returns the byte span of every line in src, excluding the
// newline itself. A trailing newline does not produce an extra empty line.
func SplitLines(src []byte) []Line {
	lines := make([]Line, 0, 16)
	start := 0

	for i, b := range src {
		if b != '\n' {
			continue
		}

		lines = append(lines, Line{start: start, end: i})
		start = i + 1
	}

	if start < len(src) {
		lines = append(lines, Line{start: start, end: len(src)})
	}

	return lines
}

// OffsetOf converts a 1-based rune column on line into a byte offset.
func OffsetOf(src []byte, line Line, col int) int {
	offset := line.start

	for n := 1; n < col && offset < line.end; n++ {
		// Dead-guard removed: DecodeRune returns size 0 only on empty input, and offset < line.end keeps src[offset:line.end] non-empty.
		_, size := utf8.DecodeRune(src[offset:line.end])
		offset += size
	}

	return offset
}

// lineStates walks the token stream once, recording the brace depth in
// effect at the start of each line plus that line's first token.
func lineStates(count int, tokens []token.Token) []lineState {
	states := make([]lineState, count)
	depth := 0
	next := 0

	for i := range states {
		states[i].depth = depth
		lineNo := i + 1

		for next < len(tokens) && tokens[next].Pos.Line <= lineNo {
			tok := tokens[next]
			next++

			if tok.Pos.Line < lineNo {
				continue
			}

			if !states[i].hasToken {
				states[i].hasToken = true
				states[i].firstKind = tok.Kind
			}

			depth = adjustDepth(depth, tok.Kind)
		}
	}

	return states
}

func adjustDepth(depth int, kind token.Kind) int {
	if kind == token.LBRACE {
		return depth + 1
	}

	if kind == token.RBRACE && depth > 0 {
		return depth - 1
	}

	return depth
}

// indentEdits rewrites the leading whitespace of every line that carries
// content: a line of code, or a line holding only a comment. A closing
// brace at the start of a line outdents one level.
func indentEdits(src []byte, lines []Line, states []lineState) []Edit {
	var edits []Edit

	for i, line := range lines {
		text := string(src[line.start:line.end])
		body := strings.TrimLeft(text, " \t")

		if !states[i].hasToken && !strings.HasPrefix(body, "//") {
			continue // whitespace-only: blankLineEdits owns it
		}

		depth := states[i].depth
		if states[i].hasToken && states[i].firstKind == token.RBRACE {
			depth--
		}

		if depth < 0 {
			depth = 0
		}

		want := strings.Repeat(indentUnit, depth)

		have := text[:len(text)-len(body)]
		if have == want {
			continue
		}

		// Leading whitespace is ASCII, so byte length is column count.
		edits = append(edits, Edit{
			Line:     i,
			StartCol: 0,
			EndCol:   len(have),
			NewText:  want,
		})
	}

	return edits
}

// blankLineEdits collapses whitespace-only lines to zero length. It never
// touches a line holding a comment, so no comment text is ever rewritten.
func blankLineEdits(src []byte, lines []Line, states []lineState) []Edit {
	var edits []Edit

	for i, line := range lines {
		if states[i].hasToken {
			continue
		}

		text := string(src[line.start:line.end])
		if text == "" || strings.TrimLeft(text, " \t") != "" {
			continue
		}

		edits = append(edits, Edit{
			Line:     i,
			StartCol: 0,
			EndCol:   len(text),
			NewText:  "",
		})
	}

	return edits
}

// spacingEdits normalizes the whitespace between adjacent tokens that the
// lexer reports on the same line. Pairs spanning a line break are skipped
// unconditionally -- that gap may hold a comment or a blank line, and
// regenerating it is exactly the mistake this formatter exists to avoid.
func spacingEdits(src []byte, lines []Line, tokens []token.Token) []Edit {
	var edits []Edit

	for i := 1; i < len(tokens); i++ {
		prev, next := tokens[i-1], tokens[i]
		if prev.Pos.Line != next.Pos.Line {
			continue
		}

		sep, ok := CanonicalGap(prev, next)
		if !ok {
			continue
		}

		index := next.Pos.Line - 1
		if index < 0 || index >= len(lines) {
			continue
		}

		line := lines[index]

		end := OffsetOf(src, line, next.Pos.Col)

		start := end
		for start > line.start && (src[start-1] == ' ' || src[start-1] == '\t') {
			start--
		}

		if string(src[start:end]) == sep {
			continue
		}

		// The gap is ASCII whitespace, so its byte width is its column width.
		edits = append(edits, Edit{
			Line:     index,
			StartCol: next.Pos.Col - 1 - (end - start),
			EndCol:   next.Pos.Col - 1,
			NewText:  sep,
		})
	}

	return edits
}

// CanonicalGap returns the whitespace that belongs between two tokens on
// one line. Every rule is derived from the schemas this repo ships
// (examples/todo/schema/todo.zen, internal/dsl/compile/testdata/app.zen):
//
//	id: uuid @primary                              -> ":" hugs left, "@" hugs right
//	@validate(min_len: 1, max_len: 200)            -> "," hugs left, no padding in "()"
//	index(user_id, due_at)                         -> no space before "("
//	rpc ListTasks(user_id: uuid) -> Task {         -> "->" spaced, "{" spaced
//
// ok is false for pairs the formatter refuses to reason about, whose gap is
// then left exactly as written.
func CanonicalGap(prev, next token.Token) (string, bool) {
	if prev.Kind == token.ILLEGAL || next.Kind == token.ILLEGAL {
		return "", false
	}

	if prev.Kind == token.EOF || next.Kind == token.EOF {
		return "", false
	}

	switch {
	case next.Kind == token.LPAREN,
		next.Kind == token.RPAREN,
		next.Kind == token.COLON,
		next.Kind == token.COMMA,
		next.Kind == token.QUESTION:
		return "", true
	case prev.Kind == token.LPAREN,
		prev.Kind == token.AT,
		prev.Kind == token.MINUS:
		return "", true
	default:
		return " ", true
	}
}
