// Package lexer scans .zen DSL source into a stream of tokens.
//
// The lexer never panics and never stops: any illegal input produces an
// ILLEGAL token plus a recorded diagnostic, and scanning always continues
// to the next token. This is load-bearing, not a style preference -- the
// parser's error-recovery strategy assumes Next keeps producing tokens
// until EOF no matter how malformed the source is.
//
// "//" line comments never produce a token: Next's output is exactly the
// same token stream it always has been, so every existing consumer (the
// parser, formatters, LSP semantic tokenizers) is unaffected.
// Comments are, however, no longer discarded outright: each one is
// recorded as a CommentToken in a side channel (see Comments), retaining
// its position and text and whether it stands alone on its line. This
// exists so the parser can attach a comment immediately preceding a
// declaration to that declaration as a doc comment (see
// parser.docCommentFor) -- nothing else in the lexer's contract changes.
package lexer

import (
	"strings"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/zenta-dev/zever/internal/dsl/diag"
	"github.com/zenta-dev/zever/internal/dsl/token"
)

// durationUnits lists recognized duration suffixes, longest match first so
// that e.g. "ms" is tried (and wins) before the single-letter "m".
var durationUnits = []string{"ns", "us", "µs", "ms", "s", "m", "h"}

// Lexer scans DSL source bytes into a stream of tokens.
type Lexer struct {
	file string
	src  []byte
	pos  int // byte offset of the next unread rune
	line int // 1-based line of the next unread rune
	col  int // 1-based column (in UTF-16 code units) of the next unread rune
	errs diag.List

	// lastTokenLine is the line of the most recently returned real token
	// (from Next), or 0 before any token has been returned. It exists only
	// to classify a "//" comment as standalone vs. trailing: see
	// CommentToken and skipLineComment.
	lastTokenLine int
	comments      []CommentToken
}

// CommentToken records one "//" line comment encountered while scanning,
// independently of the main token stream (Next never emits a token for a
// comment -- that part of the lexer's behavior is unchanged). Pos is the
// position of the leading "//". Text is the comment's content with the
// "//" marker and surrounding whitespace trimmed.
//
// Standalone reports whether the comment is the only thing on its source
// line -- i.e. no other real token was returned by Next on that same line
// before this comment was scanned. A comment trailing other code on the
// same line (`} // like this`) has Standalone == false. Only a contiguous
// run of standalone comments immediately above a declaration is eligible
// to become that declaration's doc comment; see parser.docCommentFor.
type CommentToken struct {
	Pos        diag.Position
	Text       string
	Standalone bool
}

// New creates a Lexer over src, reporting positions against file.
func New(file string, src []byte) *Lexer {
	return &Lexer{
		file: file,
		src:  src,
		pos:  0,
		line: 1,
		col:  1,
	}
}

// Errors returns every diagnostic recorded while scanning so far.
func (l *Lexer) Errors() diag.List {
	return l.errs
}

// Comments returns every "//" comment scanned so far, in source order. It
// is a side channel: comments never appear in the main token stream (Next
// still returns exactly the same tokens it always did), so callers that
// don't ask for Comments see no behavior change at all. A caller pulling
// tokens one at a time via Next (as parser.Parser does, with a one-token
// lookahead) sees this slice grow as scanning progresses; by the time Next
// has returned a token at line N, every comment at a line < N has already
// been recorded here.
func (l *Lexer) Comments() []CommentToken {
	return l.comments
}

// Next scans and returns the next token. It never panics and never
// returns an error: illegal input becomes an ILLEGAL token plus a recorded
// diagnostic (Phase "lex"), and scanning continues normally on the next
// call. Once the source is exhausted, Next returns EOF forever.
func (l *Lexer) Next() token.Token {
	tok := l.next()
	l.lastTokenLine = l.line

	return tok
}

// next does the actual scanning for Next, before lastTokenLine bookkeeping.
// Split out purely so Next can update lastTokenLine once, right before
// returning, regardless of which branch below produced the token.
func (l *Lexer) next() token.Token {
	l.skipWhitespaceAndComments()

	pos := l.position()

	if l.atEOF() {
		return token.Token{Kind: token.EOF, Lit: "", Pos: pos}
	}

	r, _ := l.peek()

	switch {
	case isIdentStart(r):
		return l.scanIdent(pos)
	case isDigit(r):
		return l.scanNumber(pos)
	case r == '"':
		return l.scanString(pos)
	default:
		return l.scanPunctOrIllegal(pos, r)
	}
}

// --- low-level cursor helpers ---

func (l *Lexer) atEOF() bool {
	return l.pos >= len(l.src)
}

func (l *Lexer) position() diag.Position {
	return diag.Position{File: l.file, Line: l.line, Col: l.col}
}

// peek returns the rune at the current position and its byte width, or
// (utf8.RuneError, 0) at EOF. An invalid UTF-8 byte decodes as
// (utf8.RuneError, 1) per the stdlib contract, which keeps advance making
// forward progress one byte at a time instead of getting stuck.
func (l *Lexer) peek() (rune, int) {
	return l.peekAt(l.pos)
}

func (l *Lexer) peekAt(pos int) (rune, int) {
	if pos >= len(l.src) {
		return utf8.RuneError, 0
	}

	r, size := utf8.DecodeRune(l.src[pos:])

	return r, size
}

// advance consumes exactly one rune, updating line/col bookkeeping. It is
// a no-op at EOF.
//
// Col counts UTF-16 code units (not runes) so a Position can be reported
// verbatim as an LSP Position, which is UTF-16 based. Astral-plane runes
// (outside BMP, e.g. emoji "😀" U+1F600) are 2 units, BMP runes are 1.
// See TestPositionTracksUTF16ColumnNotRuneColumn.
func (l *Lexer) advance() {
	r, size := l.peek()
	if size == 0 {
		return
	}

	l.pos += size

	if r == '\n' {
		l.line++
		l.col = 1
	} else {
		l.col += utf16.RuneLen(r)
	}
}

// advanceBytes consumes n bytes' worth of runes, one rune at a time, so
// that line/col stay correct even when n spans a multi-byte rune (e.g. the
// "µs" duration unit).
func (l *Lexer) advanceBytes(n int) {
	end := l.pos + n
	for l.pos < end {
		l.advance()
	}
}

func (l *Lexer) errorf(pos diag.Position, format string, args ...any) {
	l.errs = append(l.errs, diag.New("lex", pos, format, args...))
}

// --- whitespace and comments ---

func (l *Lexer) skipWhitespaceAndComments() {
	for {
		r, _ := l.peek()

		switch r {
		case ' ', '\t', '\r', '\n':
			l.advance()
			continue
		}

		if r == '/' {
			if r2, _ := l.peekAt(l.pos + 1); r2 == '/' {
				l.skipLineComment()
				continue
			}
		}

		return
	}
}

// skipLineComment consumes a "//" comment up to (not including) the
// terminating newline, or up to EOF. A comment with no trailing newline
// before EOF terminates cleanly without hanging.
//
// It also records the comment as a CommentToken (see Comments): the main
// token stream is unaffected -- this is purely additional bookkeeping on
// the side.
func (l *Lexer) skipLineComment() {
	pos := l.position()
	standalone := l.line != l.lastTokenLine

	l.advance() // consume the first '/'
	l.advance() // consume the second '/'

	start := l.pos

	for {
		r, size := l.peek()
		if size == 0 || r == '\n' {
			break
		}

		l.advance()
	}

	text := strings.TrimSpace(string(l.src[start:l.pos]))
	l.comments = append(l.comments, CommentToken{Pos: pos, Text: text, Standalone: standalone})
}

// --- identifiers and keywords ---

func isIdentStart(r rune) bool {
	return r == '_' || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')
}

func isIdentContinue(r rune) bool {
	return isIdentStart(r) || isDigit(r)
}

func isDigit(r rune) bool {
	return r >= '0' && r <= '9'
}

func (l *Lexer) scanIdent(pos diag.Position) token.Token {
	start := l.pos

	for {
		r, size := l.peek()
		if size == 0 || !isIdentContinue(r) {
			break
		}

		l.advance()
	}

	lit := string(l.src[start:l.pos])

	kind, ok := token.Keywords[lit]
	if !ok {
		kind = token.IDENT
	}

	return token.Token{Kind: kind, Lit: lit, Pos: pos}
}

// --- numbers: INT, FLOAT, DURATION ---

// numberIsGlued reports whether the rune immediately preceding byte offset
// start is an identifier-start letter, i.e. this number is glued directly
// (no separating whitespace) onto the tail of a previously scanned token.
//
// This only ever fires right after a DURATION token's single-letter unit,
// because every other token kind that can end in a letter (IDENT, and
// keywords, which are just idents) already swallows any immediately
// following digits as part of its own maximal-munch scan -- a digit run
// can only start a *fresh* Next() call, with a letter as its immediate
// predecessor, when that letter was the trailing unit of a duration and
// the scanner deliberately stopped after consuming exactly one unit.
//
// Design decision (documented per the task brief): when this is the case,
// duration-suffix matching is skipped for the glued number, so a compound
// expression like "1h30m" lexes as DURATION("1h"), INT("30"), IDENT("m")
// instead of two independently-valid but silently concatenated DURATION
// tokens ("1h" and "30m") with no separator between them. Standalone
// durations (e.g. "5m" preceded by whitespace, punctuation, or start of
// input) are completely unaffected.
func numberIsGlued(src []byte, start int) bool {
	if start <= 0 || start > len(src) {
		return false
	}

	r, _ := utf8.DecodeLastRune(src[:start])

	return isIdentStart(r)
}

func (l *Lexer) scanNumber(pos diag.Position) token.Token {
	start := l.pos
	glued := numberIsGlued(l.src, start)

	l.consumeDigits()

	if lit, ok := l.tryFloatSuffix(start); ok {
		return token.Token{Kind: token.FLOAT, Lit: lit, Pos: pos}
	}

	if !glued {
		if unit := l.matchDurationUnit(); unit != "" {
			return token.Token{Kind: token.DURATION, Lit: string(l.src[start:l.pos]), Pos: pos}
		}
	}

	return token.Token{Kind: token.INT, Lit: string(l.src[start:l.pos]), Pos: pos}
}

func (l *Lexer) consumeDigits() {
	for {
		r, size := l.peek()
		if size == 0 || !isDigit(r) {
			return
		}

		l.advance()
	}
}

// tryFloatSuffix continues an already-scanned INT digit run into a FLOAT
// when a '.' is immediately followed by a digit. It requires a digit
// before AND after the dot: if the rune after '.' is not a digit, no dot
// is consumed here, leaving it for the next Next() call (which will emit
// it as ILLEGAL, since '.' has no punctuation token of its own).
func (l *Lexer) tryFloatSuffix(start int) (string, bool) {
	if l.pos >= len(l.src) || l.src[l.pos] != '.' {
		return "", false
	}

	next, size := l.peekAt(l.pos + 1)
	if size == 0 || !isDigit(next) {
		return "", false
	}

	l.advance() // consume '.'
	l.consumeDigits()

	return string(l.src[start:l.pos]), true
}

// matchDurationUnit tries each candidate unit at the current position,
// longest match first, and consumes + returns the first one that matches.
// It returns "" and consumes nothing if no unit matches here.
func (l *Lexer) matchDurationUnit() string {
	for _, unit := range durationUnits {
		if l.hasPrefixAt(l.pos, unit) {
			l.advanceBytes(len(unit))
			return unit
		}
	}

	return ""
}

// hasPrefixAt reports whether l.src[pos:] has prefix s without allocating.
//
// Perf (R26): hot path — called for every durationUnits candidate (7×) on each
// numeric token via matchDurationUnit, which is on Next()'s hot loop. Must be
// zero-alloc: do NOT use string(l.src[pos:pos+len(s)]) == s (allocates) nor
// bytes.HasPrefix with []byte(s) (string→[]byte allocates). Manual byte loop
// is zero-alloc and inlined; bytes.HasPrefix would be equivalent only with
// unsafe string→[]byte, so keep explicit loop.
func (l *Lexer) hasPrefixAt(pos int, s string) bool {
	if pos+len(s) > len(l.src) {
		return false
	}

	for i := 0; i < len(s); i++ {
		if l.src[pos+i] != s[i] {
			return false
		}
	}

	return true
}

// --- strings ---

// scanString scans a "..." string literal with escapes \" \\ \n \t only.
// An unrecognized escape sequence keeps the backslash and following rune
// literally and continues (documented judgment call: the lexer must never
// stop, and silently dropping user-written characters would be worse than
// preserving them verbatim for a later phase or human to notice).
//
// An unterminated string (EOF or a raw newline reached before the closing
// quote) records a diagnostic and returns a STRING token with whatever was
// scanned so far; it never fails and never consumes the newline that ended
// it, so the next call resumes cleanly right after that newline.
func (l *Lexer) scanString(pos diag.Position) token.Token {
	l.advance() // consume opening '"'

	var sb strings.Builder

	for {
		r, size := l.peek()

		switch {
		case size == 0:
			l.errorf(pos, "unterminated string literal (reached EOF)")
			return token.Token{Kind: token.STRING, Lit: sb.String(), Pos: pos}
		case r == '\n':
			l.errorf(pos, "unterminated string literal (raw newline before closing quote)")
			return token.Token{Kind: token.STRING, Lit: sb.String(), Pos: pos}
		case r == '"':
			l.advance()
			return token.Token{Kind: token.STRING, Lit: sb.String(), Pos: pos}
		case r == '\\':
			l.scanStringEscape(&sb)
		default:
			_, _ = sb.WriteRune(r)

			l.advance()
		}
	}
}

func (l *Lexer) scanStringEscape(sb *strings.Builder) {
	l.advance() // consume backslash

	esc, size := l.peek()
	if size == 0 {
		return // caller's next loop iteration reports unterminated-at-EOF
	}

	switch esc {
	case '"':
		l.consumeEscaped(sb, '"')
	case '\\':
		l.consumeEscaped(sb, '\\')
	case 'n':
		l.consumeEscaped(sb, '\n')
	case 't':
		l.consumeEscaped(sb, '\t')
	default:
		// Unrecognized escape: keep the backslash and the following rune
		// literally, and keep scanning.
		l.keepEscapeLiterally(sb, esc)
	}
}

// consumeEscaped writes the decoded escape rune to sb and advances past
// the source rune that produced it (e.g. the 'n' in "\n").
func (l *Lexer) consumeEscaped(sb *strings.Builder, decoded rune) {
	_, _ = sb.WriteRune(decoded)

	l.advance()
}

// keepEscapeLiterally handles an unrecognized escape sequence by keeping
// the backslash and following rune as-is in the string's value, then
// advancing past that rune.
func (l *Lexer) keepEscapeLiterally(sb *strings.Builder, esc rune) {
	_ = sb.WriteByte('\\')

	l.consumeEscaped(sb, esc)
}

// --- punctuation and illegal characters ---

func (l *Lexer) scanPunctOrIllegal(pos diag.Position, r rune) token.Token {
	switch r {
	case '{':
		l.advance()
		return token.Token{Kind: token.LBRACE, Lit: "{", Pos: pos}
	case '}':
		l.advance()
		return token.Token{Kind: token.RBRACE, Lit: "}", Pos: pos}
	case '(':
		l.advance()
		return token.Token{Kind: token.LPAREN, Lit: "(", Pos: pos}
	case ')':
		l.advance()
		return token.Token{Kind: token.RPAREN, Lit: ")", Pos: pos}
	case '[':
		l.advance()
		return token.Token{Kind: token.LBRACK, Lit: "[", Pos: pos}
	case ']':
		l.advance()
		return token.Token{Kind: token.RBRACK, Lit: "]", Pos: pos}
	case ':':
		l.advance()
		return token.Token{Kind: token.COLON, Lit: ":", Pos: pos}
	case ',':
		l.advance()
		return token.Token{Kind: token.COMMA, Lit: ",", Pos: pos}
	case '@':
		l.advance()
		return token.Token{Kind: token.AT, Lit: "@", Pos: pos}
	case '-':
		return l.scanMinusOrArrow(pos)
	case '?':
		l.advance()
		return token.Token{Kind: token.QUESTION, Lit: "?", Pos: pos}
	default:
		return l.scanIllegal(pos, r)
	}
}

func (l *Lexer) scanMinusOrArrow(pos diag.Position) token.Token {
	l.advance() // consume '-'

	if r, size := l.peek(); size != 0 && r == '>' {
		l.advance()
		return token.Token{Kind: token.ARROW, Lit: "->", Pos: pos}
	}

	return token.Token{Kind: token.MINUS, Lit: "-", Pos: pos}
}

// scanIllegal records a diagnostic for one illegal rune, emits an ILLEGAL
// token for it, advances exactly one rune, and lets scanning continue
// normally on the next call. The lexer never stops on illegal input.
func (l *Lexer) scanIllegal(pos diag.Position, r rune) token.Token {
	lit := string(r)
	l.errorf(pos, "illegal character %q", lit)
	l.advance()

	return token.Token{Kind: token.ILLEGAL, Lit: lit, Pos: pos}
}
