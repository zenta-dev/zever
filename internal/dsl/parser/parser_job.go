package parser

import (
	"github.com/zenta-dev/zever/internal/dsl/ast"
	"github.com/zenta-dev/zever/internal/dsl/token"
)

// jobOptionStart is the job-option-body sync predicate: an IDENT whose
// literal text is one of the two recognized option labels.
func jobOptionStart(tok token.Token) bool {
	if tok.Kind != token.IDENT {
		return false
	}

	switch tok.Lit {
	case "queue", "retry":
		return true
	default:
		return false
	}
}

// parseJobDecl parses `"job" ident "(" [ParamList] ")" "{" { JobOption }
// "}"`. The caller has confirmed cur.Kind == token.JOB but not consumed it.
func (p *Parser) parseJobDecl() *ast.JobDecl {
	pos := p.cur.Pos
	doc := p.docCommentFor(pos)
	p.advance() // consume 'job'

	name, namePos, ok := p.expectIdentText()
	if !ok {
		p.syncTopLevel()
		return nil
	}

	if !p.expect(token.LPAREN) {
		p.syncTopLevel()
		return nil
	}

	var params []*ast.ParamDecl

	if p.cur.Kind != token.RPAREN {
		params, ok = p.parseParamList()
		if !ok {
			p.syncTopLevel()
			return nil
		}
	}

	if !p.expect(token.RPAREN) {
		p.syncTopLevel()
		return nil
	}

	if !p.expect(token.LBRACE) {
		p.syncTopLevel()
		return nil
	}

	decl := &ast.JobDecl{Pos: pos, NamePos: namePos, Name: name, Params: params, DocComment: doc}

	for p.cur.Kind != token.RBRACE && p.cur.Kind != token.EOF {
		p.parseJobOption(decl)
	}

	if !p.expect(token.RBRACE) {
		p.syncTopLevel()
		return nil
	}

	return decl
}

// parseJobOption parses one JobOption (QueueOption | RetryOption),
// dispatching on cur.Lit, and stores it on decl. It is the job-option-body
// loop item, so it owns the single recovery call for any failure inside
// it.
func (p *Parser) parseJobOption(decl *ast.JobDecl) {
	if p.cur.Kind != token.IDENT {
		p.errorf(p.cur.Pos, "expected queue or retry option, got %s %q", p.cur.Kind, p.cur.Lit)
		p.syncBlock(jobOptionStart)

		return
	}

	switch p.cur.Lit {
	case "queue":
		p.advance() // consume 'queue'

		if !p.expect(token.COLON) {
			break
		}

		queue, queuePos, ok := p.expectIdentText()
		if ok {
			decl.Queue = queue
			decl.QueuePos = queuePos

			return
		}
	case "retry":
		p.advance() // consume 'retry'

		if !p.expect(token.COLON) {
			break
		}

		if values, ok := p.parseValueList(); ok {
			decl.Retry = values
			return
		}
	default:
		p.errorf(p.cur.Pos, "expected queue or retry option, got IDENT %q", p.cur.Lit)
	}

	p.syncBlock(jobOptionStart)
}
