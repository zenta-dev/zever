package parser

import (
	"github.com/zenta-dev/zever/internal/dsl/ast"
	"github.com/zenta-dev/zever/internal/dsl/token"
)

// entityMemberStart is the entity-body sync predicate: it reports true for
// the reserved keywords that unambiguously start a RelationDecl or
// IndexDecl. A malformed plain field has no reserved lead-in keyword, so it
// can only resync as far as the next relation/index keyword or the closing
// brace — several bad field lines in a row collapse into one reported
// error, a documented, scoped trade-off of this recovery strategy.
func entityMemberStart(tok token.Token) bool {
	//nolint:exhaustive // membership test against the 5 relation/index keywords; every other Kind is "not a member start" via default.
	switch tok.Kind {
	case token.HAS_MANY, token.HAS_ONE, token.BELONGS_TO, token.MANY_TO_MANY, token.INDEX:
		return true
	default:
		return false
	}
}

// parseEntityDecl parses `"entity" ident { Attribute } "{" { EntityMember }
// "}"`. The caller has confirmed cur.Kind == token.ENTITY but not consumed
// it. On any structural failure (missing name, a malformed entity-level
// attribute, a missing brace) it reports one diagnostic, resyncs at the top
// level, and returns nil so the caller drops this entity entirely.
func (p *Parser) parseEntityDecl() *ast.EntityDecl {
	pos := p.cur.Pos
	doc := p.docCommentFor(pos)
	p.advance() // consume 'entity'

	name, namePos, ok := p.expectIdentText()
	if !ok {
		p.syncTopLevel()
		return nil
	}

	attrs, ok := p.parseAttributeList()
	if !ok {
		p.syncTopLevel()
		return nil
	}

	if !p.expect(token.LBRACE) {
		p.syncTopLevel()
		return nil
	}

	decl := &ast.EntityDecl{Pos: pos, Name: name, NamePos: namePos, Attributes: attrs, DocComment: doc}

	for p.cur.Kind != token.RBRACE && p.cur.Kind != token.EOF {
		p.parseEntityMember(decl)
	}

	if !p.expect(token.RBRACE) {
		p.syncTopLevel()
		return nil
	}

	return decl
}

// parseEntityMember parses one EntityMember (FieldDecl | RelationDecl |
// IndexDecl) and appends it to decl. It is the entity-body loop item, so it
// owns the single recovery call for any failure inside it: either its own
// direct detection of an unrecognized member-start token, or a delegate
// (parseFieldDecl/parseRelationDecl/parseIndexDecl) returning nil after
// already recording its own diagnostic.
func (p *Parser) parseEntityMember(decl *ast.EntityDecl) {
	//nolint:exhaustive // dispatch on the Kinds that can start an EntityMember; anything else falls to default as a parse error.
	switch p.cur.Kind {
	case token.HAS_MANY:
		if r := p.parseRelationDecl(ast.HasMany); r != nil {
			decl.Relations = append(decl.Relations, r)
			return
		}
	case token.HAS_ONE:
		if r := p.parseRelationDecl(ast.HasOne); r != nil {
			decl.Relations = append(decl.Relations, r)
			return
		}
	case token.BELONGS_TO:
		if r := p.parseRelationDecl(ast.BelongsTo); r != nil {
			decl.Relations = append(decl.Relations, r)
			return
		}
	case token.MANY_TO_MANY:
		if r := p.parseRelationDecl(ast.ManyToMany); r != nil {
			decl.Relations = append(decl.Relations, r)
			return
		}
	case token.INDEX:
		if idx := p.parseIndexDecl(); idx != nil {
			decl.Indexes = append(decl.Indexes, idx)
			return
		}
	case token.IDENT, token.ENUM, token.MESSAGE:
		if f := p.parseFieldDecl(); f != nil {
			decl.Fields = append(decl.Fields, f)
			return
		}
	default:
		p.errorf(p.cur.Pos, "expected field, relation, or index declaration, got %s %q", p.cur.Kind, p.cur.Lit)
	}

	p.syncBlock(entityMemberStart)
}

// parseFieldDecl parses `ident ":" TypeExpr { Attribute }`. The caller has
// confirmed cur is identifier-like (IDENT, ENUM, or MESSAGE) but not consumed it.
func (p *Parser) parseFieldDecl() *ast.FieldDecl {
	// Proof: parseEntityMember and parseMessageDecl dispatch here only on
	// IDENT/ENUM/MESSAGE, all isIdentLike, so expectIdentText cannot fail.
	name, namePos := p.cur.Lit, p.cur.Pos
	p.advance() // consume field name.

	doc := p.docCommentFor(namePos)

	if !p.expect(token.COLON) {
		return nil
	}

	typ := p.parseTypeExpr()
	if typ == nil {
		return nil
	}

	optional := false
	if p.cur.Kind == token.QUESTION {
		optional = true

		p.advance() // consume '?'
	}

	attrs, ok := p.parseAttributeList()
	if !ok {
		return nil
	}

	return &ast.FieldDecl{Pos: namePos, NamePos: namePos, Name: name, Type: typ, Optional: optional, Attributes: attrs, DocComment: doc}
}

// parseTypeExpr parses `ident [ "(" ident { "," ident } ")" ]`. The type
// name and any arg list are accepted permissively: whether the name is a
// known scalar, and whether an arg list is even allowed for it (only
// "enum" takes one), is a resolver check, not the parser's.
func (p *Parser) parseTypeExpr() *ast.TypeExpr {
	name, namePos, ok := p.expectIdentText()
	if !ok {
		return nil
	}

	typ := &ast.TypeExpr{Pos: namePos, NamePos: namePos, Name: name}

	if p.cur.Kind != token.LPAREN {
		return typ
	}

	p.advance() // consume '('

	for {
		argText, argPos, ok := p.expectIdentText()
		if !ok {
			return nil
		}

		typ.Args = append(typ.Args, argText)
		typ.ArgPos = append(typ.ArgPos, argPos)

		if p.cur.Kind != token.COMMA {
			break
		}

		p.advance() // consume ','
	}

	if !p.expect(token.RPAREN) {
		return nil
	}

	return typ
}

// parseRelationDecl parses `RelationKind ident ":" ident { Attribute }
// [ JoinBlock ]`. The caller has confirmed cur.Kind is the reserved keyword
// corresponding to kind, but has not consumed it.
func (p *Parser) parseRelationDecl(kind ast.RelationKind) *ast.RelationDecl {
	pos := p.cur.Pos
	p.advance() // consume the relation keyword

	fieldName, fieldNamePos, ok := p.expectIdentText()
	if !ok {
		return nil
	}

	if !p.expect(token.COLON) {
		return nil
	}

	target, targetPos, ok := p.expectIdentText()
	if !ok {
		return nil
	}

	attrs, ok := p.parseAttributeList()
	if !ok {
		return nil
	}

	decl := &ast.RelationDecl{
		Pos:          pos,
		Kind:         kind,
		FieldName:    fieldName,
		FieldNamePos: fieldNamePos,
		Target:       target,
		TargetPos:    targetPos,
		Attributes:   attrs,
	}

	if p.cur.Kind == token.LBRACE {
		join := p.parseJoinBlock()
		if join == nil {
			return nil
		}

		decl.Join = join
	}

	return decl
}

// parseJoinBlock parses `"{" "join_table" ":" ident "}"`. The caller has
// confirmed cur.Kind == token.LBRACE but not yet consumed it. "join_table"
// is a contextual label, not a reserved keyword (per token.Keywords), so it
// arrives as a plain IDENT and is matched by its literal text.
func (p *Parser) parseJoinBlock() *ast.JoinBlock {
	pos := p.cur.Pos
	p.advance() // consume '{'

	if p.cur.Kind != token.IDENT || p.cur.Lit != "join_table" {
		p.errorf(p.cur.Pos, "expected \"join_table\", got %s %q", p.cur.Kind, p.cur.Lit)
		return nil
	}

	p.advance() // consume 'join_table'

	if !p.expect(token.COLON) {
		return nil
	}

	table, tablePos, ok := p.expectIdentText()
	if !ok {
		return nil
	}

	if !p.expect(token.RBRACE) {
		return nil
	}

	return &ast.JoinBlock{Pos: pos, Table: table, TablePos: tablePos}
}

// parseIndexDecl parses `"index" "(" ident { "," ident } ")" { Attribute
// }`. The caller has confirmed cur.Kind == token.INDEX but not consumed it.
func (p *Parser) parseIndexDecl() *ast.IndexDecl {
	pos := p.cur.Pos
	p.advance() // consume 'index'

	if !p.expect(token.LPAREN) {
		return nil
	}

	decl := &ast.IndexDecl{Pos: pos}

	for {
		col, colPos, ok := p.expectIdentText()
		if !ok {
			return nil
		}

		decl.Columns = append(decl.Columns, col)
		decl.ColumnPos = append(decl.ColumnPos, colPos)

		if p.cur.Kind != token.COMMA {
			break
		}

		p.advance() // consume ','
	}

	if !p.expect(token.RPAREN) {
		return nil
	}

	attrs, ok := p.parseAttributeList()
	if !ok {
		return nil
	}

	decl.Attributes = attrs

	return decl
}
