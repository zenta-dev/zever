package parser

import (
	"strconv"
	"time"

	"github.com/zenta-dev/zever/internal/dsl/ast"
	"github.com/zenta-dev/zever/internal/dsl/diag"
	"github.com/zenta-dev/zever/internal/dsl/token"
)

// parseAttributeList parses a possibly-empty sequence of "@..." attributes,
// stopping at the first token that isn't AT. It's reused by every
// production with a `{ Attribute }` list: entity-level, field-level,
// relation-level, and index-level.
//
// If any attribute in the sequence fails to parse, the whole list is
// abandoned (nil, false) so the caller drops its enclosing construct (the
// entity, field, relation, or index) entirely and resyncs at whichever
// level that construct belongs to — the diagnostic was already recorded by
// parseAttribute (or one of its own delegates), so this never adds a
// second one for the same failure.
func (p *Parser) parseAttributeList() ([]*ast.Attribute, bool) {
	var attrs []*ast.Attribute

	for p.cur.Kind == token.AT {
		attr := p.parseAttribute()
		if attr == nil {
			return nil, false
		}

		attrs = append(attrs, attr)
	}

	return attrs, true
}

// parseAttribute parses `"@" ident [ "(" [ArgList] ")" ]`. The caller must
// have already confirmed cur.Kind == token.AT.
//
// The attribute name is accepted permissively: an unrecognized name (e.g.
// "@bogus") is not a parse error, only a resolver one — same
// parse-permissively/resolve-strictly split used throughout this grammar.
func (p *Parser) parseAttribute() *ast.Attribute {
	pos := p.cur.Pos
	p.advance() // consume '@'

	name, namePos, ok := p.expectIdentText()
	if !ok {
		return nil
	}

	attr := &ast.Attribute{Pos: pos, NamePos: namePos, Name: name}

	if p.cur.Kind != token.LPAREN {
		return attr
	}

	p.advance() // consume '('

	if p.cur.Kind != token.RPAREN {
		args, ok := p.parseArgList()
		if !ok {
			return nil
		}

		attr.Args = args
	}

	if !p.expect(token.RPAREN) {
		return nil
	}

	return attr
}

// parseArgList parses `Arg { "," Arg }`. The caller ensures the list is
// non-empty (cur is not the closing token) before calling.
func (p *Parser) parseArgList() ([]*ast.Arg, bool) {
	var args []*ast.Arg

	for {
		arg := p.parseArg()
		if arg == nil {
			return nil, false
		}

		args = append(args, arg)

		if p.cur.Kind != token.COMMA {
			break
		}

		p.advance() // consume ','
	}

	return args, true
}

// parseArg parses `[ ident ":" ] Value`. Disambiguating the optional
// "ident:" prefix from a bare Value that itself starts with an identifier
// (CallOrIdent) needs one token of lookahead beyond cur, which is exactly
// what Parser.peek is for.
func (p *Parser) parseArg() *ast.Arg {
	pos := p.cur.Pos

	name := ""

	if isIdentLike(p.cur.Kind) && p.peek.Kind == token.COLON {
		text, _, ok := p.expectIdentText()
		if !ok {
			return nil
		}

		name = text

		if !p.expect(token.COLON) {
			return nil
		}
	}

	val := p.parseValue()
	if val == nil {
		return nil
	}

	return &ast.Arg{Pos: pos, Name: name, Value: val}
}

// parseValueList parses `Value { "," Value }` — the shared shape behind
// RetryOption's comma-separated value list.
func (p *Parser) parseValueList() ([]ast.Value, bool) {
	var values []ast.Value

	for {
		v := p.parseValue()
		if v == nil {
			return nil, false
		}

		values = append(values, v)

		if p.cur.Kind != token.COMMA {
			break
		}

		p.advance() // consume ','
	}

	return values, true
}

// parseColonValue consumes ":" then a Value — the common shape of
// AuthOption/PermissionOption/DispatchOption once their label ident has
// already been consumed by the caller.
func (p *Parser) parseColonValue() ast.Value {
	if !p.expect(token.COLON) {
		return nil
	}

	return p.parseValue()
}

// maxValueDepth bounds recursion through the parseValue -> parseCallOrIdent
// -> parseArgList -> parseArg -> parseValue cycle (nested call-expression
// arguments, e.g. "@validate(nested(nested(...)))"). Each level of nesting
// costs multiple unbounded Go stack frames with nothing else limiting it, so
// adversarial or generated input with deeply nested calls could otherwise
// exhaust the goroutine stack and crash the process outright -- a fatal,
// unrecoverable failure, unlike the recoverable diag.Diagnostic path used
// everywhere else in this parser. 200 is far deeper than any realistic
// schema's attribute nesting while staying nowhere near Go's real stack
// limit.
const maxValueDepth = 200

// parseValue parses `Value = string | Number | Duration | SetLit |
// CallOrIdent`, where `Number = [ "-" ] ( INT | FLOAT )`.
//
// parseValue is the shared choke point of the parseValue/parseCallOrIdent/
// parseArgList/parseArg mutual-recursion cycle, so it's where recursion
// depth is tracked and enforced (see maxValueDepth): every level of nested
// call-expression arguments passes back through here exactly once.
func (p *Parser) parseValue() ast.Value {
	p.valueDepth++
	defer func() { p.valueDepth-- }()

	if p.valueDepth > maxValueDepth {
		p.errorf(p.cur.Pos, "max nesting depth exceeded (%d) while parsing value", maxValueDepth)
		return nil
	}

	//nolint:exhaustive // dispatch on the Kinds that can start a Value; anything else falls to default as a parse error.
	switch p.cur.Kind {
	case token.STRING:
		v := &ast.StringLit{Pos: p.cur.Pos, Value: p.cur.Lit}
		p.advance()

		return v
	case token.DURATION:
		return p.parseDurationLit()
	case token.INT:
		return p.parseIntLitToken(p.cur.Pos, 1)
	case token.FLOAT:
		return p.parseFloatLitToken(p.cur.Pos, 1)
	case token.MINUS:
		return p.parseNegativeNumber()
	case token.LBRACE:
		return p.parseSetLit()
	case token.IDENT, token.ENUM:
		return p.parseCallOrIdent()
	default:
		p.errorf(p.cur.Pos, "expected a value, got %s %q", p.cur.Kind, p.cur.Lit)
		return nil
	}
}

// parseNegativeNumber parses the "-" branch of Number. The caller has
// confirmed cur.Kind == token.MINUS but not consumed it.
func (p *Parser) parseNegativeNumber() ast.Value {
	pos := p.cur.Pos
	p.advance() // consume '-'

	//nolint:exhaustive // Number only permits INT|FLOAT after '-'; every other Kind falls to default as a parse error.
	switch p.cur.Kind {
	case token.INT:
		return p.parseIntLitToken(pos, -1)
	case token.FLOAT:
		return p.parseFloatLitToken(pos, -1)
	default:
		p.errorf(p.cur.Pos, "expected number after '-', got %s %q", p.cur.Kind, p.cur.Lit)
		return nil
	}
}

// parseIntLitToken parses cur (an INT token) into an IntLit at pos, scaled
// by sign (1 or -1). It always advances past the INT token, even when the
// literal overflows int64.
func (p *Parser) parseIntLitToken(pos diag.Position, sign int64) ast.Value {
	n, err := strconv.ParseInt(p.cur.Lit, 10, 64)
	if err != nil {
		p.errorf(p.cur.Pos, "integer literal %q out of range: %v", p.cur.Lit, err)
		p.advance()

		return nil
	}

	p.advance()

	return &ast.IntLit{Pos: pos, Value: sign * n}
}

// parseFloatLitToken parses cur (a FLOAT token) into a FloatLit at pos,
// scaled by sign (1 or -1). It always advances past the FLOAT token, even
// when strconv rejects the literal.
func (p *Parser) parseFloatLitToken(pos diag.Position, sign float64) ast.Value {
	f, err := strconv.ParseFloat(p.cur.Lit, 64)
	if err != nil {
		p.errorf(p.cur.Pos, "float literal %q invalid: %v", p.cur.Lit, err)
		p.advance()

		return nil
	}

	p.advance()

	return &ast.FloatLit{Pos: pos, Value: sign * f}
}

// parseDurationLit parses cur (a DURATION token) into a DurationLit. The
// lexer only ever produces a digit run plus a recognized unit suffix, which
// time.ParseDuration also accepts, so failure here is limited to numeric
// overflow. On failure, Valid is false and a diagnostic is recorded, but
// the node is still returned (not dropped) so the caller can still see
// where in the source the invalid duration was written.
func (p *Parser) parseDurationLit() ast.Value {
	tok := p.cur
	p.advance()

	d, err := time.ParseDuration(tok.Lit)
	if err != nil {
		p.errorf(tok.Pos, "invalid duration %q: %v", tok.Lit, err)
	}

	return &ast.DurationLit{Pos: tok.Pos, Raw: tok.Lit, Value: d, Valid: err == nil}
}

// parseSetLit parses `"{" [ ident { "," ident } ] "}"`. The caller has
// confirmed cur.Kind == token.LBRACE but not yet consumed it.
func (p *Parser) parseSetLit() ast.Value {
	pos := p.cur.Pos
	p.advance() // consume '{'

	set := &ast.SetLit{Pos: pos}

	if p.cur.Kind == token.RBRACE {
		p.advance()
		return set
	}

	for {
		text, itemPos, ok := p.expectIdentText()
		if !ok {
			return nil
		}

		set.Items = append(set.Items, text)
		set.ItemPos = append(set.ItemPos, itemPos)

		if p.cur.Kind != token.COMMA {
			break
		}

		p.advance() // consume ','
	}

	if !p.expect(token.RBRACE) {
		return nil
	}

	return set
}

// parseCallOrIdent parses `ident [ "(" [ArgList] ")" ]`. The identifier's
// text is read via expectIdentText, so "enum" is accepted here too even
// though it lexes as its own reserved Kind.
func (p *Parser) parseCallOrIdent() ast.Value {
	name, pos, ok := p.expectIdentText()
	if !ok {
		return nil
	}

	if p.cur.Kind != token.LPAREN {
		return &ast.IdentValue{Pos: pos, Name: name}
	}

	p.advance() // consume '('

	call := &ast.CallValue{Pos: pos, NamePos: pos, Name: name}

	if p.cur.Kind != token.RPAREN {
		args, ok := p.parseArgList()
		if !ok {
			return nil
		}

		call.Args = args
	}

	if !p.expect(token.RPAREN) {
		return nil
	}

	return call
}
