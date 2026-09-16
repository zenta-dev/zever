package parser

import (
	"github.com/zenta-dev/zever/internal/dsl/ast"
	"github.com/zenta-dev/zever/internal/dsl/token"
)

// parseEnumDecl parses `"enum" ident "{" ident { "," ident } [","] "}"`. The
// caller has confirmed cur.Kind == token.ENUM but not consumed it.
func (p *Parser) parseEnumDecl() *ast.EnumDecl {
	pos := p.cur.Pos
	doc := p.docCommentFor(pos)
	p.advance() // consume 'enum'

	name, namePos, ok := p.expectIdentText()
	if !ok {
		p.syncTopLevel()
		return nil
	}

	if !p.expect(token.LBRACE) {
		p.syncTopLevel()
		return nil
	}

	decl := &ast.EnumDecl{Pos: pos, NamePos: namePos, Name: name, DocComment: doc}

	for p.cur.Kind != token.RBRACE && p.cur.Kind != token.EOF {
		val, valPos, ok := p.expectIdentText()
		if !ok {
			p.errorf(p.cur.Pos, "expected enum value, got %s %q", p.cur.Kind, p.cur.Lit)
			p.syncTopLevel()

			return nil
		}

		decl.Values = append(decl.Values, val)
		decl.ValuePos = append(decl.ValuePos, valPos)

		if p.cur.Kind != token.COMMA {
			break
		}

		p.advance() // consume ','
	}

	if !p.expect(token.RBRACE) {
		p.syncTopLevel()
		return nil
	}

	return decl
}
