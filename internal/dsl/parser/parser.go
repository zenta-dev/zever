// Package parser turns a lexer.Lexer token stream into a best-effort
// *ast.File.
//
// The parser never panics: any malformed input is reported as one or more
// diag.Diagnostic values (Phase "parse"), and ParseFile always finishes and
// returns a non-nil *ast.File, no matter how badly the source is malformed.
// This is load-bearing for the resolver (later tasks) — one bad declaration
// must never prevent every other declaration in the file from parsing
// correctly. See syncTopLevel and syncBlock for the recovery strategy.
package parser

import (
	"strings"

	"github.com/zenta-dev/zever/internal/dsl/ast"
	"github.com/zenta-dev/zever/internal/dsl/diag"
	"github.com/zenta-dev/zever/internal/dsl/lexer"
	"github.com/zenta-dev/zever/internal/dsl/token"
)

// Parser holds parsing state: a two-token lookahead window (cur, peek) over
// the underlying lexer, plus the diagnostics recorded so far.
type Parser struct {
	lex       *lexer.Lexer
	file      string
	cur, peek token.Token
	errs      diag.List

	// valueDepth counts live recursions through parseValue, which sits at
	// the center of the parseValue -> parseCallOrIdent -> parseArgList ->
	// parseArg -> parseValue mutual-recursion cycle used for nested call
	// expressions (e.g. "@validate(nested(nested(...)))"). It guards
	// against unbounded recursion on adversarial or generated input: see
	// maxValueDepth in parser_attribute.go.
	valueDepth int
}

// New creates a Parser over src, reporting positions against file.
func New(file string, src []byte) *Parser {
	p := &Parser{lex: lexer.New(file, src), file: file}
	p.cur = p.lex.Next()
	p.peek = p.lex.Next()

	return p
}

// ParseFile parses the whole token stream into an *ast.File by looping over
// top-level declarations until EOF. It never returns early and never
// panics: a declaration that fails to parse is simply absent from the
// result (see parseTopLevelDecl and the sync* helpers), but every other
// declaration still parses normally.
//
// The returned diag.List concatenates every lexical diagnostic (recorded by
// the underlying lexer while tokens were pulled) followed by every
// diagnostic recorded during parsing itself, so callers get one complete
// list of everything wrong with the file. This is NOT true source-position
// order — all lexer diagnostics precede all parser diagnostics regardless
// of where each occurred in the file. Callers that need true positional
// ordering should call the returned diag.List's Sorted method.
func (p *Parser) ParseFile() (*ast.File, diag.List) {
	file := &ast.File{Name: p.file}

	for p.cur.Kind != token.EOF {
		if decl := p.parseTopLevelDecl(); decl != nil {
			file.Decls = append(file.Decls, decl)
		}
	}

	errs := make(diag.List, 0, len(p.errs)+len(p.lex.Errors()))
	errs = append(errs, p.lex.Errors()...)
	errs = append(errs, p.errs...)

	return file, errs
}

// parseTopLevelDecl dispatches on cur.Kind to the matching top-level
// production. An unrecognized token is a parse error: it's reported once,
// then the parser resyncs to the next token that unambiguously starts a
// top-level declaration (or EOF) before the caller's loop continues.
func (p *Parser) parseTopLevelDecl() ast.Decl {
	//nolint:exhaustive // dispatch on the 7 top-level-starting kinds; every other Kind falls to default as a parse error.
	switch p.cur.Kind {
	case token.ENTITY:
		if d := p.parseEntityDecl(); d != nil {
			return d
		}
	case token.SERVICE:
		if d := p.parseServiceDecl(); d != nil {
			return d
		}
	case token.JOB:
		if d := p.parseJobDecl(); d != nil {
			return d
		}
	case token.SCHEDULE:
		if d := p.parseScheduleDecl(); d != nil {
			return d
		}
	case token.MESSAGE:
		if d := p.parseMessageDecl(); d != nil {
			return d
		}
	case token.ENUM:
		if d := p.parseEnumDecl(); d != nil {
			return d
		}
	default:
		p.errorf(p.cur.Pos, "expected entity, service, job, schedule, message, or enum declaration, got %s %q", p.cur.Kind, p.cur.Lit)
		p.syncTopLevel()
	}

	return nil
}

// advance moves the token window forward by one, pulling the next token
// from the lexer. The lexer never panics and never stops producing tokens
// (it emits EOF forever once exhausted), so advance always makes progress
// and never blocks.
func (p *Parser) advance() {
	p.cur = p.peek
	p.peek = p.lex.Next()
}

// expect consumes cur if it has the given kind and reports whether it did.
// Otherwise it records one diagnostic and returns false, without advancing.
// (An earlier revision returned the consumed Token; no caller ever used it,
// so the signature is boolean-only.)
func (p *Parser) expect(kind token.Kind) bool {
	if p.cur.Kind != kind {
		p.errorf(p.cur.Pos, "expected %s, got %s %q", kind, p.cur.Kind, p.cur.Lit)
		return false
	}

	p.advance()

	return true
}

// isIdentLike reports whether kind behaves as an identifier in content
// position. This is IDENT itself, plus ENUM and MESSAGE: the grammar reserves
// "enum" and "message" as keywords only because they unambiguously start a
// TypeExpr's arg list and a top-level message decl respectively, but the words
// themselves are still just ordinary idents wherever the grammar calls for a
// bare `ident` (e.g. as a field name like `message: string`).
// TRUE/FALSE are deliberately excluded: the Grammar Reference's Value
// production (string | Number | Duration | SetLit | CallOrIdent) has no
// boolean literal, so a bare "true"/"false" where a value is expected is a
// genuine parse error, not a permissive case to special-case here.
func isIdentLike(kind token.Kind) bool {
	//nolint:exhaustive // deliberate 3-of-31 membership test (IDENT, ENUM, MESSAGE); every other Kind is "not ident-like" via default.
	switch kind {
	case token.IDENT, token.ENUM, token.MESSAGE:
		return true
	default:
		return false
	}
}

// expectIdentText consumes cur and returns its literal text if cur is
// identifier-like (see isIdentLike). Otherwise it records one diagnostic
// and returns "", cur's position, false, without advancing.
func (p *Parser) expectIdentText() (string, diag.Position, bool) {
	if !isIdentLike(p.cur.Kind) {
		p.errorf(p.cur.Pos, "expected identifier, got %s %q", p.cur.Kind, p.cur.Lit)
		return "", p.cur.Pos, false
	}

	lit, pos := p.cur.Lit, p.cur.Pos
	p.advance()

	return lit, pos, true
}

// docCommentFor returns the doc comment attached to a declaration or member
// whose own position is pos, per this rule: walk backward from the line
// immediately above pos.Line through a contiguous run of standalone "//"
// comments (see lexer.CommentToken.Standalone) -- one comment per line, no
// gaps -- and join their text with "\n" in source order. The walk stops
// (returning "" if it never started) as soon as:
//
//   - the line immediately above pos.Line has no comment at all (this
//     covers both a blank line and a line holding only other code), or
//   - the comment on that line is not standalone (a trailing comment after
//     other content never becomes the next declaration's doc comment).
//
// It relies on p.lex.Comments() already holding every comment up to pos:
// true by construction, since the parser only ever calls this once cur has
// been advanced to pos, and the lexer's one-token lookahead means any
// comment before pos was necessarily scanned already to produce that
// token.
func (p *Parser) docCommentFor(pos diag.Position) string {
	comments := p.lex.Comments()

	end := -1

	for i := len(comments) - 1; i >= 0; i-- {
		if comments[i].Pos.Line < pos.Line {
			end = i
			break
		}
	}

	if end == -1 || !comments[end].Standalone || comments[end].Pos.Line != pos.Line-1 {
		return ""
	}

	start := end
	expectedLine := pos.Line - 1

	for start > 0 && comments[start-1].Standalone && comments[start-1].Pos.Line == expectedLine-1 {
		start--
		expectedLine--
	}

	lines := make([]string, 0, end-start+1)
	for _, c := range comments[start : end+1] {
		lines = append(lines, c.Text)
	}

	return strings.Join(lines, "\n")
}

// errorf records one parse-phase diagnostic at pos.
func (p *Parser) errorf(pos diag.Position, format string, args ...any) {
	p.errs = append(p.errs, diag.New("parse", pos, format, args...))
}

// syncTopLevel is the top-level recovery sync: it skips tokens, tracking
// brace, paren, and bracket depth, until EOF or one of
// ENTITY/SERVICE/JOB/SCHEDULE/MESSAGE/ENUM at depth 0. This guarantees one bad
// top-level declaration never swallows the rest of the file: "entity Bad { !!! }"
// is skipped as one balanced span, then the next declaration parses normally.
// All three bracket kinds are tracked so an unbalanced span like
// "entity Bad { [ ( !!! } entity Good { id: uuid }" still recovers to Good
// without requiring callers to know which delimiters the bad span used.
// Strings and comments are already handled by the lexer: a keyword inside a
// STRING token or a "//" comment never appears as an ENTITY/etc. token, so
// syncTopLevel cannot mis-sync on "entity" that occurs inside a literal or
// comment.
func (p *Parser) syncTopLevel() {
	depthBraces := 0
	depthParens := 0
	depthBrackets := 0

	for {
		//nolint:exhaustive // only EOF/brackets/the 7 trigger keywords affect sync state; everything else is skipped unchanged.
		switch p.cur.Kind {
		case token.EOF:
			return
		case token.LBRACE:
			depthBraces++
		case token.RBRACE:
			if depthBraces > 0 {
				depthBraces--
			}
		case token.LPAREN:
			depthParens++
		case token.RPAREN:
			if depthParens > 0 {
				depthParens--
			}
		case token.LBRACK:
			depthBrackets++
		case token.RBRACK:
			if depthBrackets > 0 {
				depthBrackets--
			}
		case token.ENTITY, token.SERVICE, token.JOB, token.SCHEDULE, token.MESSAGE, token.ENUM:
			if depthBraces == 0 && depthParens == 0 && depthBrackets == 0 {
				return
			}
		}

		p.advance()
	}
}

// syncBlock is the per-block recovery sync: it skips tokens until RBRACE,
// EOF, or isMemberStart reports true for cur, whichever comes first. Each
// block-body loop supplies its own predicate for "does this token start
// the next member" so a malformed member doesn't swallow the rest of the
// block.
func (p *Parser) syncBlock(isMemberStart func(token.Token) bool) {
	for p.cur.Kind != token.RBRACE && p.cur.Kind != token.EOF {
		if isMemberStart(p.cur) {
			return
		}

		p.advance()
	}
}
