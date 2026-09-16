package parser

import (
	"github.com/zenta-dev/zever/internal/dsl/ast"
	"github.com/zenta-dev/zever/internal/dsl/diag"
	"github.com/zenta-dev/zever/internal/dsl/token"
)

// serviceMemberStart is the service-body sync predicate: only "rpc" starts
// a member.
func serviceMemberStart(tok token.Token) bool {
	return tok.Kind == token.RPC
}

// rpcOptionStart is the RPC-option-body sync predicate: an IDENT whose
// literal text is one of the three recognized option labels.
func rpcOptionStart(tok token.Token) bool {
	if tok.Kind != token.IDENT {
		return false
	}

	switch tok.Lit {
	case "http", "auth", "permission", "errors", "paginated":
		return true
	default:
		return false
	}
}

// parseServiceDecl parses `"service" ident "{" { RPCDecl } "}"`. The caller
// has confirmed cur.Kind == token.SERVICE but not consumed it.
func (p *Parser) parseServiceDecl() *ast.ServiceDecl {
	pos := p.cur.Pos
	doc := p.docCommentFor(pos)
	p.advance() // consume 'service'

	name, namePos, ok := p.expectIdentText()
	if !ok {
		p.syncTopLevel()
		return nil
	}

	if !p.expect(token.LBRACE) {
		p.syncTopLevel()
		return nil
	}

	decl := &ast.ServiceDecl{Pos: pos, Name: name, NamePos: namePos, DocComment: doc}

	for p.cur.Kind != token.RBRACE && p.cur.Kind != token.EOF {
		if p.cur.Kind != token.RPC {
			p.errorf(p.cur.Pos, "expected rpc declaration, got %s %q", p.cur.Kind, p.cur.Lit)
			p.syncBlock(serviceMemberStart)

			continue
		}

		rpc := p.parseRPCDecl()
		if rpc == nil {
			p.syncBlock(serviceMemberStart)
			continue
		}

		decl.RPCs = append(decl.RPCs, rpc)
	}

	if !p.expect(token.RBRACE) {
		p.syncTopLevel()
		return nil
	}

	return decl
}

// parseRPCDecl parses `"rpc" ident "(" [ParamList] ")" "->" ident "{"
// { RPCOption } "}"`. The caller has confirmed cur.Kind == token.RPC but
// not consumed it. It never calls a sync function itself: it is a service-
// body member, so its caller (parseServiceDecl's loop) owns recovery.
func (p *Parser) parseRPCDecl() *ast.RPCDecl {
	pos := p.cur.Pos
	doc := p.docCommentFor(pos)
	p.advance() // consume 'rpc'

	name, namePos, ok := p.expectIdentText()
	if !ok {
		return nil
	}

	if !p.expect(token.LPAREN) {
		return nil
	}

	var params []*ast.ParamDecl

	if p.cur.Kind != token.RPAREN {
		params, ok = p.parseParamList()
		if !ok {
			return nil
		}
	}

	if !p.expect(token.RPAREN) {
		return nil
	}

	if !p.expect(token.ARROW) {
		return nil
	}

	returns, returnsPos, ok := p.expectIdentText()
	if !ok {
		return nil
	}

	if !p.expect(token.LBRACE) {
		return nil
	}

	decl := &ast.RPCDecl{
		Pos:        pos,
		NamePos:    namePos,
		ReturnsPos: returnsPos,
		Name:       name,
		Returns:    returns,
		Params:     params,
		DocComment: doc,
	}

	for p.cur.Kind != token.RBRACE && p.cur.Kind != token.EOF {
		p.parseRPCOption(decl)
	}

	if !p.expect(token.RBRACE) {
		return nil
	}

	return decl
}

// parseParamList parses `Param { "," Param }`. The caller ensures the list
// is non-empty (cur is not the closing token) before calling.
func (p *Parser) parseParamList() ([]*ast.ParamDecl, bool) {
	var params []*ast.ParamDecl

	for {
		param := p.parseParam()
		if param == nil {
			return nil, false
		}

		params = append(params, param)

		if p.cur.Kind != token.COMMA {
			break
		}

		p.advance() // consume ','
	}

	return params, true
}

// parseParam parses `ident ":" TypeExpr { Attribute }`. Like FieldDecl,
// params carry an attribute list — reusing the identical `@attribute(...)`
// grammar entity fields already use (e.g. `@validate(format: "email")`).
func (p *Parser) parseParam() *ast.ParamDecl {
	name, namePos, ok := p.expectIdentText()
	if !ok {
		return nil
	}

	if !p.expect(token.COLON) {
		return nil
	}

	typ := p.parseTypeExpr()
	if typ == nil {
		return nil
	}

	attrs, ok := p.parseAttributeList()
	if !ok {
		return nil
	}

	return &ast.ParamDecl{Pos: namePos, NamePos: namePos, Name: name, Type: typ, Attributes: attrs}
}

// parseRPCOption parses one RPCOption (HTTPOption | AuthOption |
// PermissionOption), dispatching on cur.Lit, and stores it on decl. It is
// the RPC-option-body loop item, so it owns the single recovery call for
// any failure inside it.
func (p *Parser) parseRPCOption(decl *ast.RPCDecl) {
	if p.cur.Kind != token.IDENT {
		p.errorf(p.cur.Pos, "expected http, auth, permission, errors, or paginated option, got %s %q", p.cur.Kind, p.cur.Lit)
		p.syncBlock(rpcOptionStart)

		return
	}

	switch p.cur.Lit {
	case "http":
		if opt := p.parseHTTPOption(); opt != nil {
			decl.HTTP = opt
			return
		}
	case "auth":
		p.advance() // consume 'auth'

		if v := p.parseColonValue(); v != nil {
			decl.Auth = v
			return
		}
	case "permission":
		p.advance() // consume 'permission'

		if v := p.parseColonValue(); v != nil {
			decl.Permission = v
			return
		}
	case "errors":
		if items, ok := p.parseErrorSet(); ok {
			decl.Errors = items
			return
		}
	case "paginated":
		if value, pos, ok := p.parsePaginatedOption(); ok {
			decl.Paginated = value
			decl.PaginatedPos = pos
			decl.PaginatedSet = true

			return
		}
	default:
		p.errorf(p.cur.Pos, "expected http, auth, permission, errors, or paginated option, got IDENT %q", p.cur.Lit)
	}

	p.syncBlock(rpcOptionStart)
}

// parsePaginatedOption parses `"paginated" ":" ("true" | "false")`. The
// caller has confirmed cur is an IDENT with Lit == "paginated" but not
// consumed it. Deliberately its own small grammar rather than routing
// through the general Value production: Value has no boolean literal (see
// isIdentLike's doc comment), and paginated: is the one rpc-body option
// where a bare true/false keyword is exactly what's wanted, so it is
// recognized directly against token.TRUE/token.FALSE here instead of
// growing the shared Value grammar for one caller.
func (p *Parser) parsePaginatedOption() (value bool, pos diag.Position, ok bool) {
	p.advance() // consume 'paginated'

	if !p.expect(token.COLON) {
		return false, diag.Position{}, false
	}

	pos = p.cur.Pos

	//nolint:exhaustive // dispatch on the 2 boolean-literal kinds; every other Kind falls to default as a parse error.
	switch p.cur.Kind {
	case token.TRUE:
		p.advance()
		return true, pos, true
	case token.FALSE:
		p.advance()
		return false, pos, true
	default:
		p.errorf(p.cur.Pos, "expected true or false, got %s %q", p.cur.Kind, p.cur.Lit)
		return false, diag.Position{}, false
	}
}

// parseErrorSet parses `"errors" ":" "{" [ CallOrIdent { "," CallOrIdent } ]
// "}"`. The caller has confirmed cur is an IDENT with Lit == "errors" but
// not consumed it. Each item is parsed via the existing parseCallOrIdent, so
// it accepts both a bare error code (IdentValue) and a code with an
// optional positional string message (CallValue) — resolveErrors is
// responsible for validating the fixed vocabulary and the call-argument
// shape. An empty errors: {} parses fine, matching parseSetLit's existing
// permissive precedent.
func (p *Parser) parseErrorSet() ([]ast.Value, bool) {
	p.advance() // consume 'errors'

	if !p.expect(token.COLON) {
		return nil, false
	}

	if !p.expect(token.LBRACE) {
		return nil, false
	}

	var items []ast.Value

	if p.cur.Kind == token.RBRACE {
		p.advance()
		return items, true
	}

	for {
		v := p.parseCallOrIdent()
		if v == nil {
			return nil, false
		}

		items = append(items, v)

		if p.cur.Kind != token.COMMA {
			break
		}

		p.advance() // consume ','
	}

	if !p.expect(token.RBRACE) {
		return nil, false
	}

	return items, true
}

// parseHTTPOption parses `"http" ":" ident string`. The caller has
// confirmed cur is an IDENT with Lit == "http" but not consumed it. The
// verb ident is accepted permissively: checking it's one of
// GET/POST/PUT/PATCH/DELETE is a resolver job, not the parser's.
func (p *Parser) parseHTTPOption() *ast.HTTPOption {
	pos := p.cur.Pos
	p.advance() // consume 'http'

	if !p.expect(token.COLON) {
		return nil
	}

	method, methodPos, ok := p.expectIdentText()
	if !ok {
		return nil
	}

	if p.cur.Kind != token.STRING {
		p.errorf(p.cur.Pos, "expected string path, got %s %q", p.cur.Kind, p.cur.Lit)
		return nil
	}

	path, pathPos := p.cur.Lit, p.cur.Pos
	p.advance()

	return &ast.HTTPOption{Pos: pos, MethodPos: methodPos, PathPos: pathPos, Method: method, Path: path}
}
