package parser

import (
	"github.com/zenta-dev/zever/internal/dsl/ast"
	"github.com/zenta-dev/zever/internal/dsl/token"
)

// messageMemberStart reports whether tok starts a message member. Messages contain only fields.
func messageMemberStart(tok token.Token) bool {
	return tok.Kind == token.IDENT || tok.Kind == token.ENUM || tok.Kind == token.MESSAGE
}

// parseMessageDecl parses `"message" ident "{" { FieldDecl } "}"`. The caller has confirmed cur.Kind == token.MESSAGE but not consumed it.
func (p *Parser) parseMessageDecl() *ast.MessageDecl {
	pos := p.cur.Pos
	doc := p.docCommentFor(pos)
	p.advance() // consume 'message'

	name, namePos, ok := p.expectIdentText()
	if !ok {
		p.syncTopLevel()
		return nil
	}

	if !p.expect(token.LBRACE) {
		p.syncTopLevel()
		return nil
	}

	decl := &ast.MessageDecl{Pos: pos, NamePos: namePos, Name: name, DocComment: doc}

	for p.cur.Kind != token.RBRACE && p.cur.Kind != token.EOF {
		//nolint:exhaustive // messages contain only fields; every other token kind falls to default (a syntax error)
		switch p.cur.Kind {
		case token.IDENT, token.ENUM, token.MESSAGE:
			if f := p.parseFieldDecl(); f != nil {
				decl.Fields = append(decl.Fields, f)
				continue
			}
		default:
			p.errorf(p.cur.Pos, "expected field declaration, got %s %q", p.cur.Kind, p.cur.Lit)
		}

		p.syncBlock(messageMemberStart)
	}

	if !p.expect(token.RBRACE) {
		p.syncTopLevel()
		return nil
	}

	return decl
}
