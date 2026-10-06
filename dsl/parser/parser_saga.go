package parser

import (
	"github.com/zenta-dev/zever/dsl/ast"
	"github.com/zenta-dev/zever/dsl/token"
)

// sagaStepStart is the saga-body sync predicate: an IDENT whose literal
// text is "step". "step" is contextual (not a reserved keyword), so the
// predicate matches on literal text exactly like jobOptionStart matches
// "queue"/"retry".
func sagaStepStart(tok token.Token) bool {
	return tok.Kind == token.IDENT && tok.Lit == "step"
}

// sagaStepOptionStart is the step-body sync predicate: an IDENT whose
// literal text is one of the three recognized step-option labels.
func sagaStepOptionStart(tok token.Token) bool {
	if tok.Kind != token.IDENT {
		return false
	}

	switch tok.Lit {
	case "execute", "compensate", "pivot":
		return true
	default:
		return false
	}
}

// parseSagaDecl parses `"saga" ident "{" { SagaStep } "}"`. The caller has
// confirmed cur is the contextual "saga" ident but has not consumed it.
func (p *Parser) parseSagaDecl() *ast.SagaDecl {
	pos := p.cur.Pos
	doc := p.docCommentFor(pos)
	p.advance() // consume 'saga'

	name, namePos, ok := p.expectIdentText()
	if !ok {
		p.syncTopLevel()
		return nil
	}

	if !p.expect(token.LBRACE) {
		p.syncTopLevel()
		return nil
	}

	decl := &ast.SagaDecl{Pos: pos, NamePos: namePos, Name: name, DocComment: doc}

	for p.cur.Kind != token.RBRACE && p.cur.Kind != token.EOF {
		if step := p.parseSagaStep(); step != nil {
			decl.Steps = append(decl.Steps, step)
		}
	}

	if !p.expect(token.RBRACE) {
		p.syncTopLevel()
		return nil
	}

	return decl
}

// parseSagaStep parses `"step" ident "{" { StepOption } "}"`, returning nil
// (after recovering) when the step is too malformed to salvage. A step that
// parses but lacks its required execute option is still returned; the
// missing option is reported here and re-checked by the resolver.
func (p *Parser) parseSagaStep() *ast.SagaStepDecl {
	if !sagaStepStart(p.cur) {
		p.errorf(p.cur.Pos, "expected step, got %s %q", p.cur.Kind, p.cur.Lit)
		p.syncBlock(sagaStepStart)

		return nil
	}

	pos := p.cur.Pos
	p.advance() // consume 'step'

	name, _, ok := p.expectIdentText()
	if !ok {
		p.syncBlock(sagaStepStart)
		return nil
	}

	if !p.expect(token.LBRACE) {
		p.syncBlock(sagaStepStart)
		return nil
	}

	step := &ast.SagaStepDecl{Pos: pos, Name: name}

	for p.cur.Kind != token.RBRACE && p.cur.Kind != token.EOF {
		p.parseSagaStepOption(step)

		if p.cur.Kind == token.COMMA {
			p.advance()
		}
	}

	if !p.expect(token.RBRACE) {
		p.syncBlock(sagaStepStart)
		return nil
	}

	if step.Execute == nil {
		p.errorf(pos, "saga step %q is missing its required execute option", name)
	}

	return step
}

// parseSagaStepOption parses one StepOption (ExecuteOption |
// CompensateOption | PivotOption), dispatching on cur.Lit, and stores it on
// step. It is the step-option-body loop item, so it owns the single recovery
// call for any failure inside it.
func (p *Parser) parseSagaStepOption(step *ast.SagaStepDecl) {
	if p.cur.Kind != token.IDENT {
		p.errorf(p.cur.Pos, "expected execute, compensate, or pivot option, got %s %q", p.cur.Kind, p.cur.Lit)
		p.syncBlock(sagaStepOptionStart)

		return
	}

	switch p.cur.Lit {
	case "execute":
		p.advance() // consume 'execute'

		if !p.expect(token.COLON) {
			break
		}

		if ref, ok := p.parseRPCRef(); ok {
			step.Execute = ref

			return
		}
	case "compensate":
		p.advance() // consume 'compensate'

		if !p.expect(token.COLON) {
			break
		}

		if ref, ok := p.parseRPCRef(); ok {
			step.Compensate = ref

			return
		}
	case "pivot":
		p.advance() // consume 'pivot'

		if !p.expect(token.COLON) {
			break
		}

		//nolint:exhaustive // dispatch on the 2 boolean-literal kinds; every other Kind falls to default as a parse error.
		switch p.cur.Kind {
		case token.TRUE:
			step.Pivot = true
			p.advance()

			return
		case token.FALSE:
			p.advance()

			return
		default:
			p.errorf(p.cur.Pos, "pivot must be true or false, got %s %q", p.cur.Kind, p.cur.Lit)
		}
	default:
		p.errorf(p.cur.Pos, "expected execute, compensate, or pivot option, got IDENT %q", p.cur.Lit)
	}

	p.syncBlock(sagaStepOptionStart)
}

// parseRPCRef parses `ident "." ident` (a Service.RPC reference). ok is
// false (after one diagnostic and no advance past the service ident) when
// the shape is malformed.
func (p *Parser) parseRPCRef() (*ast.RPCRef, bool) {
	ref := &ast.RPCRef{Pos: p.cur.Pos}

	service, _, ok := p.expectIdentText()
	if !ok {
		return nil, false
	}

	ref.Service = service

	if !p.expectSagaDot() {
		return nil, false
	}

	rpc, _, ok := p.expectIdentText()
	if !ok {
		return nil, false
	}

	ref.RPC = rpc

	return ref, true
}

// expectSagaDot consumes the "." separating the service and RPC identifiers
// of a Service.RPC reference. The DSL lexer deliberately has no punctuation
// token for "." (adding one would change dsl/token and dsl/lexer, which this
// feature avoids to stay non-breaking), so a "." arrives as an ILLEGAL token
// whose literal is exactly ".". Consuming it as valid syntax therefore also
// retracts the "lex" diagnostic the lexer recorded for it -- see
// Parser.sagaDots and ParseFile. It reports false, with one parse diagnostic
// and without advancing, when cur is not such a token.
func (p *Parser) expectSagaDot() bool {
	if p.cur.Kind == token.ILLEGAL && p.cur.Lit == "." {
		p.sagaDots = append(p.sagaDots, p.cur.Pos)
		p.advance()

		return true
	}

	p.errorf(p.cur.Pos, "expected \".\", got %s %q", p.cur.Kind, p.cur.Lit)

	return false
}
