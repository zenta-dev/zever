package lexer

import (
	"strings"
	"testing"

	"github.com/zenta-dev/zever/dsl/token"
)

// drainAll drains l to EOF and returns the full token slice (EOF included).
func drainAll(t *testing.T, src string) []token.Token {
	t.Helper()

	l := New("edge.zen", []byte(src))

	var toks []token.Token
	for {
		tok := l.Next()
		toks = append(toks, tok)
		if tok.Kind == token.EOF {
			return toks
		}
	}
}

func TestLexEmptySourceYieldsSingleEOF(t *testing.T) {
	t.Parallel()

	l := New("empty.zen", nil)

	if tok := l.Next(); tok.Kind != token.EOF {
		t.Fatalf("first token = %s %q, want EOF", tok.Kind, tok.Lit)
	}

	if errs := l.Errors(); len(errs) != 0 {
		t.Fatalf("errors = %v, want none", errs)
	}

	if comments := l.Comments(); len(comments) != 0 {
		t.Fatalf("comments = %v, want none", comments)
	}
}

func TestLexEOFIsIdempotent(t *testing.T) {
	t.Parallel()

	l := New("empty.zen", nil)

	for i := 0; i < 3; i++ {
		if tok := l.Next(); tok.Kind != token.EOF {
			t.Fatalf("call %d: token = %s, want EOF", i, tok.Kind)
		}
	}
}

func TestLexIllegalCharacterRecordsDiagAndContinues(t *testing.T) {
	t.Parallel()

	l := New("bad.zen", []byte("entity \x01 User"))

	tok := l.Next()
	if tok.Kind != token.ENTITY {
		t.Fatalf("token = %s, want ENTITY", tok.Kind)
	}

	tok = l.Next()
	if tok.Kind != token.ILLEGAL || tok.Lit != "\x01" {
		t.Fatalf("token = %s %q, want ILLEGAL \"\\x01\"", tok.Kind, tok.Lit)
	}

	tok = l.Next()
	if tok.Kind != token.IDENT || tok.Lit != "User" {
		t.Fatalf("token = %s %q, want IDENT \"User\" (scanning must continue past ILLEGAL)", tok.Kind, tok.Lit)
	}

	errs := l.Errors()
	if len(errs) != 1 {
		t.Fatalf("errors = %v, want exactly 1", errs)
	}

	if errs[0].Phase != "lex" {
		t.Fatalf("diag phase = %q, want \"lex\"", errs[0].Phase)
	}
}

func TestLexInvalidUTF8DoesNotHangOrPanic(t *testing.T) {
	t.Parallel()

	l := New("bad.zen", []byte{0xFF, 0xFE, '}', 0x80})

	for i := 0; i < 10; i++ {
		l.Next()
	}

	if tok := l.Next(); tok.Kind != token.EOF {
		t.Fatalf("token after invalid UTF-8 run = %s, want EOF", tok.Kind)
	}
}

func TestLexUnterminatedStringAtEOF(t *testing.T) {
	t.Parallel()

	l := New("bad.zen", []byte("\"never closed"))

	tok := l.Next()
	if tok.Kind != token.STRING {
		t.Fatalf("token = %s %q, want STRING", tok.Kind, tok.Lit)
	}

	if tok.Lit != "never closed" {
		t.Fatalf("string lit = %q, want %q", tok.Lit, "never closed")
	}

	errs := l.Errors()
	if len(errs) != 1 || !strings.Contains(errs[0].Msg, "unterminated string") {
		t.Fatalf("errors = %v, want one unterminated-string diagnostic", errs)
	}
}

func TestLexUnterminatedStringAtRawNewline(t *testing.T) {
	t.Parallel()

	l := New("bad.zen", []byte("\"line one\nb: string"))

	var toks []token.Token
	for {
		tok := l.Next()
		toks = append(toks, tok)
		if tok.Kind == token.EOF {
			break
		}
	}

	if toks[0].Kind != token.STRING {
		t.Fatalf("first token = %s, want STRING", toks[0].Kind)
	}

	if toks[1].Kind != token.IDENT || toks[1].Lit != "b" {
		t.Fatalf("token after newline = %s %q, want IDENT \"b\" (newline must not be consumed)", toks[1].Kind, toks[1].Lit)
	}

	if len(l.Errors()) != 1 {
		t.Fatalf("errors = %v, want exactly 1", l.Errors())
	}
}

func TestLexUnrecognizedEscapeKeptLiterally(t *testing.T) {
	t.Parallel()

	toks := drainAll(t, `"\q"`)

	if toks[0].Kind != token.STRING {
		t.Fatalf("token = %s, want STRING", toks[0].Kind)
	}

	if toks[0].Lit != `\q` {
		t.Fatalf("lit = %q, want %q (backslash kept literally)", toks[0].Lit, `\q`)
	}
}

func TestLexArrowVersusMinus(t *testing.T) {
	t.Parallel()

	toks := drainAll(t, "-> -")

	if toks[0].Kind != token.ARROW {
		t.Fatalf("toks[0] = %s %q, want ARROW", toks[0].Kind, toks[0].Lit)
	}

	if toks[1].Kind != token.MINUS {
		t.Fatalf("toks[1] = %s %q, want MINUS", toks[1].Kind, toks[1].Lit)
	}
}

func TestLexCommentSideChannelStandaloneVersusTrailing(t *testing.T) {
	t.Parallel()

	src := "// standalone one\nentity X { // trailing\nid: uuid }"
	l := New("c.zen", []byte(src))

	last := l.Next()
	for last.Kind != token.EOF {
		last = l.Next()
	}

	comments := l.Comments()
	if len(comments) != 2 {
		t.Fatalf("comments = %v, want 2", comments)
	}

	if !comments[0].Standalone || comments[0].Text != "standalone one" {
		t.Fatalf("comments[0] = %+v, want standalone \"standalone one\"", comments[0])
	}

	if comments[1].Standalone {
		t.Fatalf("comments[1] = %+v, want trailing (Standalone=false)", comments[1])
	}

	if comments[0].Pos.Line != 1 || comments[1].Pos.Line != 2 {
		t.Fatalf("comment lines = %d,%d want 1,2", comments[0].Pos.Line, comments[1].Pos.Line)
	}
}

func TestLexCommentTextTrimmed(t *testing.T) {
	t.Parallel()

	l := New("c.zen", []byte("//   spaced out   \n"))
	last := l.Next()
	for last.Kind != token.EOF {
		last = l.Next()
	}

	comments := l.Comments()
	if len(comments) != 1 || comments[0].Text != "spaced out" {
		t.Fatalf("comments = %v, want one comment with text %q", comments, "spaced out")
	}
}

func TestLexGluedNumberAfterDurationUnit(t *testing.T) {
	t.Parallel()

	toks := drainAll(t, "1h30m")

	if toks[0].Kind != token.DURATION || toks[0].Lit != "1h" {
		t.Fatalf("toks[0] = %s %q, want DURATION \"1h\"", toks[0].Kind, toks[0].Lit)
	}

	if toks[1].Kind != token.INT || toks[1].Lit != "30" {
		t.Fatalf("toks[1] = %s %q, want INT \"30\"", toks[1].Kind, toks[1].Lit)
	}

	if toks[2].Kind != token.IDENT || toks[2].Lit != "m" {
		t.Fatalf("toks[2] = %s %q, want IDENT \"m\"", toks[2].Kind, toks[2].Lit)
	}
}

func TestLexStandaloneDurationsUnaffectedByGluedGuard(t *testing.T) {
	t.Parallel()

	toks := drainAll(t, "1h 30m")

	if toks[0].Kind != token.DURATION || toks[1].Kind != token.DURATION {
		t.Fatalf("kinds = %s,%s want DURATION,DURATION", toks[0].Kind, toks[1].Kind)
	}
}

func TestLexFloatRequiresDigitAfterDot(t *testing.T) {
	t.Parallel()

	toks := drainAll(t, "1. 2.5")

	if toks[0].Kind != token.INT || toks[0].Lit != "1" {
		t.Fatalf("toks[0] = %s %q, want INT \"1\"", toks[0].Kind, toks[0].Lit)
	}

	if toks[1].Kind != token.ILLEGAL || toks[1].Lit != "." {
		t.Fatalf("toks[1] = %s %q, want ILLEGAL \".\"", toks[1].Kind, toks[1].Lit)
	}

	if toks[2].Kind != token.FLOAT || toks[2].Lit != "2.5" {
		t.Fatalf("toks[2] = %s %q, want FLOAT \"2.5\"", toks[2].Kind, toks[2].Lit)
	}
}

func TestLexColumnTracksUTF16Units(t *testing.T) {
	t.Parallel()

	l := New("u.zen", []byte("😀 x"))

	l.Next() // ILLEGAL? no: astral rune is not ident start, not digit, not quote → ILLEGAL

	tok := l.Next()
	if tok.Kind != token.IDENT || tok.Lit != "x" {
		t.Fatalf("token = %s %q, want IDENT \"x\"", tok.Kind, tok.Lit)
	}

	// "😀" is 2 UTF-16 units, so "x" starts at column 4 (1 + 2 + 1 space).
	if tok.Pos.Col != 4 {
		t.Fatalf("col = %d, want 4 (UTF-16 units, not runes)", tok.Pos.Col)
	}
}

func TestLexCRLFAndTabsAreWhitespace(t *testing.T) {
	t.Parallel()

	toks := drainAll(t, "entity\rX\t{")
	if toks[0].Kind != token.ENTITY || toks[1].Kind != token.IDENT || toks[2].Kind != token.LBRACE {
		t.Fatalf("kinds = %s,%s,%s want ENTITY,IDENT,LBRACE", toks[0].Kind, toks[1].Kind, toks[2].Kind)
	}
}

func TestLexVeryLongIdentifier(t *testing.T) {
	t.Parallel()

	src := "entity " + strings.Repeat("a", 100000) + " {}"
	l := New("long.zen", []byte(src))

	l.Next()
	tok := l.Next()

	if tok.Kind != token.IDENT || len(tok.Lit) != 100000 {
		t.Fatalf("token = %s len %d, want IDENT len 100000", tok.Kind, len(tok.Lit))
	}
}

func TestLexDeterministicTokenStream(t *testing.T) {
	t.Parallel()

	src := "entity User { id: uuid @primary } // trailing\n\"str\" 5m 1.5 -> ?"
	first := drainAll(t, src)
	second := drainAll(t, src)

	if len(first) != len(second) {
		t.Fatalf("token count differs: %d vs %d", len(first), len(second))
	}

	for i := range first {
		if first[i] != second[i] {
			t.Fatalf("token %d differs: %+v vs %+v", i, first[i], second[i])
		}
	}
}
