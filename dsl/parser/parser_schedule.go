package parser

import (
	"github.com/zenta-dev/zever/dsl/ast"
	"github.com/zenta-dev/zever/dsl/token"
)

// scheduleOptionStart is the schedule-option-body sync predicate: an IDENT
// whose literal text is one of the two recognized option labels.
func scheduleOptionStart(tok token.Token) bool {
	if tok.Kind != token.IDENT {
		return false
	}

	switch tok.Lit {
	case "cron", "dispatch":
		return true
	default:
		return false
	}
}

// parseScheduleDecl parses `"schedule" ident "{" { ScheduleOption } "}"`.
// The caller has confirmed cur.Kind == token.SCHEDULE but not consumed it.
func (p *Parser) parseScheduleDecl() *ast.ScheduleDecl {
	pos := p.cur.Pos
	doc := p.docCommentFor(pos)
	p.advance() // consume 'schedule'

	name, namePos, ok := p.expectIdentText()
	if !ok {
		p.syncTopLevel()
		return nil
	}

	if !p.expect(token.LBRACE) {
		p.syncTopLevel()
		return nil
	}

	decl := &ast.ScheduleDecl{Pos: pos, NamePos: namePos, Name: name, DocComment: doc}

	for p.cur.Kind != token.RBRACE && p.cur.Kind != token.EOF {
		p.parseScheduleOption(decl)
	}

	if !p.expect(token.RBRACE) {
		p.syncTopLevel()
		return nil
	}

	return decl
}

// parseScheduleOption parses one ScheduleOption (CronOption |
// DispatchOption), dispatching on cur.Lit, and stores it on decl. It is the
// schedule-option-body loop item, so it owns the single recovery call for
// any failure inside it.
func (p *Parser) parseScheduleOption(decl *ast.ScheduleDecl) {
	if p.cur.Kind != token.IDENT {
		p.errorf(p.cur.Pos, "expected cron or dispatch option, got %s %q", p.cur.Kind, p.cur.Lit)
		p.syncBlock(scheduleOptionStart)

		return
	}

	switch p.cur.Lit {
	case "cron":
		p.advance() // consume 'cron'

		if !p.expect(token.COLON) {
			break
		}

		if p.cur.Kind != token.STRING {
			p.errorf(p.cur.Pos, "expected string cron spec, got %s %q", p.cur.Kind, p.cur.Lit)
			break
		}

		decl.Cron = p.cur.Lit
		decl.CronPos = p.cur.Pos
		p.advance()

		return
	case "dispatch":
		p.advance() // consume 'dispatch'

		if v := p.parseColonValue(); v != nil {
			decl.Dispatch = v
			return
		}
	default:
		p.errorf(p.cur.Pos, "expected cron or dispatch option, got IDENT %q", p.cur.Lit)
	}

	p.syncBlock(scheduleOptionStart)
}
