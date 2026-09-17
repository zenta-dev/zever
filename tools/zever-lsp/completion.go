package main

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"

	"go.lsp.dev/protocol"

	"github.com/zenta-dev/zever/internal/dsl/ast"
	"github.com/zenta-dev/zever/internal/dsl/ir"
	"github.com/zenta-dev/zever/internal/dsl/lexer"
	"github.com/zenta-dev/zever/internal/dsl/resolver"
	"github.com/zenta-dev/zever/internal/dsl/token"
)

// completionContext names the syntactic slot the cursor sits in. Each one
// maps to exactly one candidate list in completionAt.
type completionContext int

const (
	// ctxNone means the cursor is somewhere with nothing to enumerate.
	ctxNone completionContext = iota
	ctxTopLevelKeyword
	ctxEntityMember
	ctxFieldType
	ctxRelationTarget
	ctxFieldAttrName
	ctxRelationAttrName
	ctxEntityAttrName
	ctxIndexAttrName
	ctxValidateArgName
	ctxOnDeleteValue
	ctxValidateFormatValue
	ctxErrorsValue
)

// candidate is one completion suggestion before it becomes a protocol item:
// a label plus an optional one-line explanation.
type candidate struct {
	label  string
	detail string
}

// scalarTypeCandidates mirrors resolver.scalarTypes, plus `enum`, which the
// resolver handles separately because it takes an argument list.
var scalarTypeCandidates = []candidate{
	{"uuid", "UUID scalar"},
	{"string", "string scalar"},
	{"int32", "32-bit signed integer"},
	{"int64", "64-bit signed integer"},
	{"float32", "32-bit float"},
	{"float64", "64-bit float"},
	{"bool", "boolean scalar"},
	{"timestamp", "timestamp scalar"},
	{"date", "date scalar"},
	{"bytes", "byte string scalar"},
	{"json", "JSON document scalar"},
	{"enum", "enum(value, ...) inline enumeration"},
}

// topLevelKeywordCandidates mirrors token.Keywords minus true/false: the
// words that unambiguously start a production.
var topLevelKeywordCandidates = []candidate{
	{"entity", "declare an entity"},
	{"message", "declare a non-persisted request/response message"},
	{"service", "declare a service"},
	{"job", "declare a background job"},
	{"schedule", "declare a scheduled task"},
	{"rpc", "declare an rpc method"},
	{"index", "declare an index"},
	{"enum", "inline enumeration type"},
	{"has_many", "one-to-many relation"},
	{"has_one", "one-to-one relation"},
	{"belongs_to", "inverse relation holding the foreign key"},
	{"many_to_many", "many-to-many relation via a join table"},
}

// entityMemberCandidates are the reserved words that can start an entity
// body member; a plain field starts with a bare identifier instead.
var entityMemberCandidates = []candidate{
	{"has_many", "one-to-many relation"},
	{"has_one", "one-to-one relation"},
	{"belongs_to", "inverse relation holding the foreign key"},
	{"many_to_many", "many-to-many relation via a join table"},
	{"index", "index(column, ...) declaration"},
}

// fieldAttrCandidates mirrors resolver.resolveFieldAttribute.
var fieldAttrCandidates = []candidate{
	{"primary", "mark the field as the primary key"},
	{"unique", "add a unique constraint"},
	{"validate", "validate(kind: value, ...) constraints"},
	{"default", "default(value) for new rows"},
	{"renamed_from", "renamed_from(\"old_name\") — record a column rename for zengo db migrate"},
}

// relationAttrCandidates mirrors resolver.resolveRelationAttributes.
var relationAttrCandidates = []candidate{
	{"foreign_key", "foreign_key(column) backing this relation"},
	{"on_delete", "on_delete(restrict | cascade | set_null)"},
}

// entityAttrCandidates mirrors resolver.resolveEntitySchema.
var entityAttrCandidates = []candidate{
	{"schema", "schema(name) overriding the module schema"},
}

// indexAttrCandidates mirrors resolver.resolveIndex.
var indexAttrCandidates = []candidate{
	{"unique", "make the index unique"},
}

// validateArgCandidates mirrors resolver.resolveValidateAttribute. Both the
// string-only and numeric-only kinds are offered; narrowing them by the
// field's declared scalar type is a follow-up.
var validateArgCandidates = []candidate{
	{"format", "string format: email, url or uuid"},
	{"min_len", "minimum string length"},
	{"max_len", "maximum string length"},
	{"gt", "numeric greater-than bound"},
	{"gte", "numeric greater-or-equal bound"},
	{"lt", "numeric less-than bound"},
	{"lte", "numeric less-or-equal bound"},
}

// onDeleteCandidates mirrors resolver.resolveOnDeleteAttribute.
var onDeleteCandidates = []candidate{
	{"restrict", "refuse to delete while children exist"},
	{"cascade", "delete children along with the parent"},
	{"set_null", "null the foreign key on delete"},
}

// validateFormatCandidates mirrors resolver.decodeValidateValue.
var validateFormatCandidates = []candidate{
	{"email", "an email address"},
	{"url", "an absolute URL"},
	{"uuid", "a UUID string"},
}

// errorCandidates mirrors resolver.ValidErrorCodes -- the fixed, gRPC-
// canonical vocabulary an rpc's errors: {...} set accepts. Built once from
// that same map (rather than a separately maintained list) so the two
// vocabularies can never drift; sorted by label since map iteration order
// is random and this feeds directly into completion output.
var errorCandidates = buildErrorCandidates()

// buildErrorCandidates builds the errors-set completion candidates from resolver.ValidErrorCodes, sorted by label.
func buildErrorCandidates() []candidate {
	cands := make([]candidate, 0, len(resolver.ValidErrorCodes))

	for name, code := range resolver.ValidErrorCodes {
		cands = append(cands, candidate{name, fmt.Sprintf("%d %s", code.HTTPStatus(), code.GRPCName())})
	}

	sort.Slice(cands, func(i, j int) bool { return cands[i].label < cands[j].label })

	return cands
}

// completionAt returns the completion items for a cursor in one document.
// It never returns nil items for a known context; an unknown context yields
// an empty slice so callers can treat "no completions" uniformly.
//
// The textDocument/completion handler wraps a non-empty result in
// protocol.CompletionItemSlice (the CompletionResult union arm) and returns
// a literal nil when there is nothing to offer.
func completionAt(
	schema *ir.Schema,
	file *ast.File,
	src string,
	cursor protocol.Position,
) []protocol.CompletionItem {
	ctx, prefix := completionContextAt(file, src, cursor)

	switch ctx {
	case ctxTopLevelKeyword:
		return items(topLevelKeywordCandidates, protocol.CompletionItemKindKeyword, prefix)
	case ctxEntityMember:
		return items(entityMemberCandidates, protocol.CompletionItemKindKeyword, prefix)
	case ctxFieldType:
		return items(scalarTypeCandidates, protocol.CompletionItemKindClass, prefix)
	case ctxRelationTarget:
		return items(entityCandidates(schema), protocol.CompletionItemKindClass, prefix)
	case ctxFieldAttrName:
		return items(fieldAttrCandidates, protocol.CompletionItemKindProperty, prefix)
	case ctxRelationAttrName:
		return items(relationAttrCandidates, protocol.CompletionItemKindProperty, prefix)
	case ctxEntityAttrName:
		return items(entityAttrCandidates, protocol.CompletionItemKindProperty, prefix)
	case ctxIndexAttrName:
		return items(indexAttrCandidates, protocol.CompletionItemKindProperty, prefix)
	case ctxValidateArgName:
		return items(validateArgCandidates, protocol.CompletionItemKindProperty, prefix)
	case ctxOnDeleteValue:
		return items(onDeleteCandidates, protocol.CompletionItemKindEnumMember, prefix)
	case ctxValidateFormatValue:
		return items(validateFormatCandidates, protocol.CompletionItemKindEnumMember, prefix)
	case ctxErrorsValue:
		return items(errorCandidates, protocol.CompletionItemKindEnumMember, prefix)
	}
	// Proof: completionContextAt yields only the twelve contexts dispatched
	// above plus ctxNone, and both ctxNone and any (impossible) unknown
	// context mean "nothing to enumerate", so they share this single nil
	// return, which the existing ctxNone tests cover.
	return nil
}

// entityCandidates turns every resolved entity into a completion candidate,
// labelled with its field count the way hover phrases it.
func entityCandidates(schema *ir.Schema) []candidate {
	entities := allEntityNames(schema)

	out := make([]candidate, 0, len(entities))
	for _, entity := range entities {
		out = append(out, candidate{entity.Name, fmt.Sprintf("entity, %d field(s)", len(entity.Fields))})
	}

	return out
}

// items prefix-filters candidates and converts the survivors to protocol
// items of one kind. It is the single item constructor: every context goes
// through it so the shapes stay identical.
//
// Detail is deliberately not attached here: the LSP spec's intended pattern
// for rich completion is a fast, lightweight initial list followed by a
// lazy completionItem/resolve for whichever single item the user highlights.
// Instead, each item's Data field carries just enough to reconstruct the
// detail (and any richer documentation) on resolve -- see
// completionResolveData and completionResolve. Data is LSPAny (raw JSON),
// so the payload is marshaled; an item with no detail keeps Data zero.
func items(cands []candidate, kind protocol.CompletionItemKind, prefix string) []protocol.CompletionItem {
	out := make([]protocol.CompletionItem, 0, len(cands))

	for _, cand := range cands {
		if !hasPrefix(cand.label, prefix) {
			continue
		}

		item := protocol.CompletionItem{Label: cand.label, Kind: kind}

		if cand.detail != "" {
			if raw, err := json.Marshal(completionResolveData{Detail: cand.detail}); err == nil {
				item.Data = protocol.LSPAny(raw)
			}
		}

		out = append(out, item)
	}

	return out
}

// completionResolveDataKey is the map key completionResolveData round-trips
// under once a CompletionItem's Data field has gone through JSON (either an
// actual client round trip, or a Go map[string]any produced by
// json.Unmarshal into an `any`).
const completionResolveDataKey = "detail"

// completionResolveData is what completionAt attaches to a CompletionItem's
// Data field: exactly the detail string needed to fill the item in on
// completionItem/resolve. It is intentionally minimal -- every candidate's
// only documentation today is its one-line detail string, so there is
// nothing richer to carry alongside it.
type completionResolveData struct {
	Detail string `json:"detail"`
}

// completionResolveDetail recovers the detail string a resolve request's
// item.Data carries, whichever shape it comes back in: the raw-JSON LSPAny
// this server itself attached (a real client round-trips Data through JSON,
// so it arrives as encoded bytes), the completionResolveData value attached
// in-process, or the map[string]any a JSON decode into an `any` produces.
func completionResolveDetail(data any) (string, bool) {
	if raw, ok := data.(protocol.LSPAny); ok {
		var payload completionResolveData
		if err := json.Unmarshal(raw, &payload); err == nil {
			return payload.Detail, payload.Detail != ""
		}

		var m map[string]any
		if err := json.Unmarshal(raw, &m); err == nil {
			detail, ok := m[completionResolveDataKey].(string)

			return detail, ok && detail != ""
		}

		return "", false
	}

	switch v := data.(type) {
	case completionResolveData:
		return v.Detail, v.Detail != ""
	case *completionResolveData:
		if v == nil {
			return "", false
		}

		return v.Detail, v.Detail != ""
	case map[string]any:
		detail, ok := v[completionResolveDataKey].(string)

		return detail, ok && detail != ""
	default:
		return "", false
	}
}

// completionResolve fills in the Detail (and mirrors it into Documentation,
// the only richer doc text available for any candidate today) of a single
// completion item the client is asking to resolve, identified by the Data
// field completionAt attached to it on the way out. An item whose Data does
// not decode to a known shape -- e.g. one with no detail to begin with -- is
// returned unchanged rather than as an error, since resolve is best-effort
// by design.
func (s *Server) completionResolve(
	ctx context.Context, params *protocol.CompletionItem,
) (*protocol.CompletionItem, error) {
	s.captureClient(ctx)

	detail, ok := completionResolveDetail(params.Data)
	if !ok {
		return params, nil
	}

	params.Detail.Set(detail)
	params.Documentation = protocol.String(detail)

	return params, nil
}

// hasPrefix reports whether a candidate label matches the partially typed
// text. Matching is case-sensitive and an empty prefix matches everything;
// clients refine further as the user keeps typing.
func hasPrefix(label, prefix string) bool {
	if prefix == "" {
		return true
	}

	return strings.HasPrefix(label, prefix)
}

// completionContextAt classifies the cursor position and returns the partial
// identifier already typed there, if any.
//
// It tries two strategies in order. The AST tier handles a cursor inside an
// identifier the parser managed to keep. The lexical tier handles everything
// else, because the parser's recovery drops a whole declaration when nothing
// follows a trigger character (`id: uuid @` loses the entire field), so
// there is no AST node left to inspect right where completion matters most.
func completionContextAt(file *ast.File, src string, cursor protocol.Position) (completionContext, string) {
	if ctx, prefix, ok := astContextAt(file, cursor); ok {
		return ctx, prefix
	}

	return lexicalContextAt(src, cursor)
}

// astContextAt is the mid-identifier tier: it reuses the position helpers
// that hover and go-to-definition already rely on.
func astContextAt(file *ast.File, cursor protocol.Position) (completionContext, string, bool) {
	if rel := relationTargetAt(file, cursor); rel != nil {
		return ctxRelationTarget, rel.Target, true
	}

	_, field := fieldAt(file, cursor)
	if field != nil && field.Type != nil && coversIdent(field.Type.NamePos, field.Type.Name, cursor) {
		return ctxFieldType, field.Type.Name, true
	}

	return ctxNone, "", false
}

// lexicalContextAt re-lexes the document up to the cursor and pattern-matches
// the trailing tokens. The lexer never panics and always terminates, so this
// is safe on arbitrarily broken input.
func lexicalContextAt(src string, cursor protocol.Position) (completionContext, string) {
	toks := lexPrefix(sourceBefore(src, cursor))

	prefix := ""

	if n := len(toks); n > 0 && isWordToken(toks[n-1]) && tokenEndsAt(toks[n-1], cursor) {
		prefix = toks[n-1].Lit
		toks = toks[:n-1]
	}

	if ctx, ok := attrArgContext(toks); ok {
		return ctx, prefix
	}

	if ctx, ok := errorsSetContext(toks); ok {
		return ctx, prefix
	}

	if ctx, ok := lineContext(toks, int(cursor.Line)+1); ok {
		return ctx, prefix
	}

	return blockContext(toks), prefix
}

// sourceBefore slices the document down to the cursor. The character index is
// treated as a byte offset into the line, matching how the rest of this
// server maps compiler positions to LSP ones.
func sourceBefore(src string, cursor protocol.Position) string {
	lines := strings.SplitAfter(src, "\n")

	line := int(cursor.Line)
	if line >= len(lines) {
		return src
	}

	head := strings.Join(lines[:line], "")

	tail := strings.TrimSuffix(lines[line], "\n")
	if col := int(cursor.Character); col < len(tail) {
		tail = tail[:col]
	}

	return head + tail
}

// lexPrefix scans src into tokens, dropping the terminating EOF.
func lexPrefix(src string) []token.Token {
	lex := lexer.New("completion.zen", []byte(src))

	var toks []token.Token

	for {
		tok := lex.Next()
		if tok.Kind == token.EOF {
			return toks
		}

		toks = append(toks, tok)
	}
}

// isWordToken reports whether a token is an identifier or a keyword, i.e.
// something the user could still be halfway through typing.
func isWordToken(tok token.Token) bool {
	if tok.Kind == token.IDENT {
		return true
	}

	_, isKeyword := token.Keywords[tok.Lit]

	return isKeyword
}

// tokenEndsAt reports whether tok ends exactly at the cursor, which means the
// user is typing that token rather than having finished it.
func tokenEndsAt(tok token.Token, cursor protocol.Position) bool {
	if tok.Pos.Line != int(cursor.Line)+1 {
		return false
	}

	return tok.Pos.Col+utf8.RuneCountInString(tok.Lit) == int(cursor.Character)+1
}

// attrArgContext handles a cursor inside an unclosed `(`. Only attribute
// argument lists have an enumerable vocabulary; every other paren (enum
// values, index columns, rpc signatures) reports ctxNone so the caller stops
// rather than falling through to a line- or block-level guess.
func attrArgContext(toks []token.Token) (completionContext, bool) {
	open := openParenIndex(toks)
	if open < 0 {
		return ctxNone, false
	}

	if open < 2 || toks[open-2].Kind != token.AT {
		return ctxNone, true
	}

	inner := toks[open+1:]

	switch toks[open-1].Lit {
	case "validate":
		return validateArgContext(inner), true
	case "on_delete":
		if len(inner) == 0 {
			return ctxOnDeleteValue, true
		}

		return ctxNone, true
	default:
		// foreign_key, schema and default take free-form values: there is
		// nothing to enumerate, which is an answer, not a gap.
		return ctxNone, true
	}
}

// validateArgContext splits the inside of `@validate(...)` into the arg-name
// slot and the one value slot with a fixed vocabulary.
func validateArgContext(inner []token.Token) completionContext {
	last := len(inner) - 1

	if last >= 1 && inner[last].Kind == token.COLON && inner[last-1].Lit == "format" {
		return ctxValidateFormatValue
	}

	if last < 0 || inner[last].Kind == token.COMMA {
		return ctxValidateArgName
	}

	return ctxNone
}

// openParenIndex returns the index of the innermost unclosed '(' or -1.
func openParenIndex(toks []token.Token) int {
	var stack []int

	for i, tok := range toks {
		switch tok.Kind { //nolint:exhaustive // only parens affect nesting here
		case token.LPAREN:
			stack = append(stack, i)
		case token.RPAREN:
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
		default:
		}
	}

	if len(stack) == 0 {
		return -1
	}

	return stack[len(stack)-1]
}

// errorsSetContext handles a cursor inside an unclosed `{` that belongs to
// an rpc's `errors: {...}` set, mirroring attrArgContext's paren-based shape
// but for the brace before an errors: value list. Brace-based (not line-
// based, unlike lineContext) so a multi-line
//
//	errors: {
//		not_found,
//		<cursor>
//	}
//
// still resolves correctly, the same way attrArgContext already handles a
// multi-line `@validate(...)`.
//
// Unlike attrArgContext's "there was a paren, so stop trying other tiers"
// behavior, a brace that isn't an errors: set reports ok=false so the
// caller keeps falling through to lineContext/blockContext -- blockContext
// also depends on brace nesting (for rpc/service/entity body keywords), and
// must still run for a cursor in some other kind of `{ }`.
func errorsSetContext(toks []token.Token) (completionContext, bool) {
	open := openBraceIndex(toks)
	if open < 2 {
		return ctxNone, false
	}

	if toks[open-1].Kind != token.COLON {
		return ctxNone, false
	}

	if toks[open-2].Kind != token.IDENT || toks[open-2].Lit != "errors" {
		return ctxNone, false
	}

	return ctxErrorsValue, true
}

// lineContext handles the trigger characters that end the current line: '@'
// starts an attribute name, ':' starts a type or relation target. A line that
// already holds other tokens offers nothing, so the caller does not fall
// through to block-level keywords mid-declaration.
func lineContext(toks []token.Token, line int) (completionContext, bool) {
	if len(toks) == 0 {
		return ctxNone, false
	}

	lineToks := tokensOnLine(toks, line)
	if len(lineToks) == 0 {
		return ctxNone, false
	}

	last := lineToks[len(lineToks)-1]

	switch last.Kind { //nolint:exhaustive // only '@' and ':' open a completable slot
	case token.AT:
		return attrNameContext(lineToks[:len(lineToks)-1], braceDepth(toks)), true
	case token.COLON:
		return colonContext(lineToks), true
	default:
		return ctxNone, true
	}
}

// attrNameContext picks the attribute vocabulary from whatever declaration
// the '@' is attached to, using the tokens preceding it on the same line.
func attrNameContext(before []token.Token, depth int) completionContext {
	if len(before) == 0 {
		// An attribute alone on its own line: outside any block it can only
		// belong to a declaration, inside one only to a field.
		if depth == 0 {
			return ctxEntityAttrName
		}

		return ctxFieldAttrName
	}

	switch before[0].Kind { //nolint:exhaustive // every other lead-in token means a plain field
	case token.ENTITY:
		return ctxEntityAttrName
	case token.INDEX:
		return ctxIndexAttrName
	case token.HAS_MANY, token.HAS_ONE, token.BELONGS_TO, token.MANY_TO_MANY:
		return ctxRelationAttrName
	default:
		return ctxFieldAttrName
	}
}

// colonContext distinguishes `field: <type>` from `belongs_to name: <Entity>`.
func colonContext(lineToks []token.Token) completionContext {
	if len(lineToks) < 2 {
		return ctxNone
	}

	if isRelationKeyword(lineToks[0].Kind) {
		return ctxRelationTarget
	}

	if len(lineToks) == 2 && lineToks[0].Kind == token.IDENT {
		return ctxFieldType
	}

	return ctxNone
}

func isRelationKeyword(kind token.Kind) bool {
	switch kind { //nolint:exhaustive // membership test over the four relation keywords
	case token.HAS_MANY, token.HAS_ONE, token.BELONGS_TO, token.MANY_TO_MANY:
		return true
	default:
		return false
	}
}

// tokensOnLine returns the tokens whose position is on the given 1-based line.
func tokensOnLine(toks []token.Token, line int) []token.Token {
	start := len(toks)

	for i := len(toks) - 1; i >= 0; i-- {
		if toks[i].Pos.Line != line {
			break
		}

		start = i
	}

	return toks[start:]
}

// blockContext answers "what can start a statement here" for a cursor on an
// otherwise empty line, using the enclosing brace nesting.
func blockContext(toks []token.Token) completionContext {
	open := openBraceIndex(toks)
	if open < 0 {
		return ctxTopLevelKeyword
	}

	switch enclosingDeclKind(toks, open) { //nolint:exhaustive // other blocks have no vocabulary yet
	case token.ENTITY:
		return ctxEntityMember
	default:
		// service, job, schedule and rpc bodies use different shapes that
		// this pass deliberately leaves alone.
		return ctxNone
	}
}

// openBraceIndex returns the index of the innermost unclosed '{' or -1.
func openBraceIndex(toks []token.Token) int {
	var stack []int

	for i, tok := range toks {
		switch tok.Kind { //nolint:exhaustive // only braces affect nesting here
		case token.LBRACE:
			stack = append(stack, i)
		case token.RBRACE:
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
		default:
		}
	}

	if len(stack) == 0 {
		return -1
	}

	return stack[len(stack)-1]
}

// braceDepth counts how many '{' are still unclosed at the end of toks.
func braceDepth(toks []token.Token) int {
	depth := 0

	for _, tok := range toks {
		switch tok.Kind { //nolint:exhaustive // only braces affect nesting here
		case token.LBRACE:
			depth++
		case token.RBRACE:
			if depth > 0 {
				depth--
			}
		default:
		}
	}

	return depth
}

// enclosingDeclKind scans back from an opening brace for the keyword that
// started the declaration owning it, stopping at any other brace so a
// previous sibling declaration is never mistaken for the enclosing one.
func enclosingDeclKind(toks []token.Token, open int) token.Kind {
	for i := open - 1; i >= 0; i-- {
		switch toks[i].Kind { //nolint:exhaustive // scanning for declaration keywords and brace boundaries
		case token.LBRACE, token.RBRACE:
			return token.ILLEGAL
		case token.ENTITY, token.SERVICE, token.JOB, token.SCHEDULE, token.RPC:
			return toks[i].Kind
		default:
		}
	}

	return token.ILLEGAL
}
