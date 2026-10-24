package main

import (
	"sort"
	"unicode/utf16"
	"unicode/utf8"

	"go.lsp.dev/protocol"

	"github.com/zenta-dev/zever/dsl/ast"
	"github.com/zenta-dev/zever/dsl/diag"
	"github.com/zenta-dev/zever/dsl/token"
)

// Semantic tokens exist to separate the identifier categories the grammar
// deliberately does not reserve. token.Keywords holds only the 14 words that
// unambiguously start a production; scalar type names (uuid, string,
// int64, ...), contextual block labels (http, queue, cron, join_table, ...)
// and HTTP verbs all lex as a plain IDENT. A TextMate/vim grammar cannot
// tell an entity name from a field name from an attribute name from a
// relation target, because they are the same lexeme class. The resolved
// syntax tree can, so that is what this file publishes.
//
// The classifier is two-tiered, for the same reason completion.go is:
//
//   - The AST tier walks every declaration and tags identifiers by the slot
//     they occupy (entity name -> class, field name -> property, attribute
//     name -> macro, ...). Only the tree knows this.
//   - The token tier re-lexes the document and tags reserved keywords and
//     literals, whose category is already settled lexically and whose exact
//     source extent only the token stream records.
//
// The two tiers cannot collide: a reserved keyword is never token.IDENT, and
// a literal is never an identifier. The one deliberate exception is `enum`,
// which is a reserved keyword that also appears as a field's type name; the
// AST tier claims that span first and the token tier skips positions already
// taken, so it is reported as a type rather than a bare keyword.
//
// Comments are permanently absent from this output. The lexer's
// skipLineComment consumes "//" runs without emitting a token and without
// recording their position anywhere (see format.go, which is built entirely
// around that fact), so no comment span data survives into the token stream
// or the AST. This is not a gap to fill later: there is nothing to recover.
// TestClassifyFileHasNoCommentTokens asserts the absence so the limitation
// stays a tested contract rather than an undocumented surprise.
//
// Contextual block labels (http, auth, permission, queue, retry, cron,
// dispatch, join_table) are likewise left unclassified: the parser records a
// position for only one of them (HTTPOption.Pos), and highlighting one label
// while leaving its siblings plain would read as a bug. The shipped grammars
// already colour them uniformly.

// semanticTokenLegend is the legend advertised in initialize and the index
// space every tokenSpan.TokenType refers to. It is the standard LSP token
// type list in its canonical order; this server uses a subset, but publishes
// the whole list so a client theme keyed to the standard names resolves
// every index the same way it would for any other server. One modifier,
// "definition", marks declaration identifiers (decl names and field names
// at their declaration); references to those names carry none, so a theme
// can style definitions distinctly. Growing the modifier list (rather than
// renumbering the type list) is the spec-tolerated direction for old
// clients: tokenTypes/tokenModifiers are extensible enumerations, and a
// client that does not know "definition" ignores the bit.
var semanticTokenLegend = protocol.SemanticTokensLegend{
	TokenTypes: []string{
		"namespace", "type", "class", "enum", "interface", "struct",
		"typeParameter", "parameter", "variable", "property", "enumMember",
		"event", "function", "method", "macro", "keyword", "modifier",
		"comment", "string", "number", "regexp", "operator",
	},
	TokenModifiers: []string{"definition"},
}

// semTokDefinitionBit is the modifier bit for the legend's "definition"
// modifier: bit N of a token's modifier mask refers to the Nth entry of
// TokenModifiers, and "definition" is entry 0.
const semTokDefinitionBit = 1 << 0

// The token type indices this server actually emits. They are derived from
// semanticTokenLegend at init time rather than written as literals, so
// reordering the legend can never silently desynchronize the encoding from
// what initialize advertised.
var (
	semTokNamespace  = semanticTokenIndex("namespace")
	semTokType       = semanticTokenIndex("type")
	semTokClass      = semanticTokenIndex("class")
	semTokInterface  = semanticTokenIndex("interface")
	semTokParameter  = semanticTokenIndex("parameter")
	semTokProperty   = semanticTokenIndex("property")
	semTokEnumMember = semanticTokenIndex("enumMember")
	semTokEvent      = semanticTokenIndex("event")
	semTokFunction   = semanticTokenIndex("function")
	semTokMethod     = semanticTokenIndex("method")
	semTokMacro      = semanticTokenIndex("macro")
	semTokKeyword    = semanticTokenIndex("keyword")
	semTokString     = semanticTokenIndex("string")
	semTokNumber     = semanticTokenIndex("number")
)

// semanticTokenIndex returns name's position in the legend, or -1 if the
// legend does not carry it. A -1 span is dropped during encoding rather than
// wrapping around into a nonsense index.
func semanticTokenIndex(name string) int {
	for i, candidate := range semanticTokenLegend.TokenTypes {
		if candidate == name {
			return i
		}
	}

	return -1
}

// tokenSpan is one classified identifier or literal span, before delta
// encoding. Line and StartChar are 0-based LSP coordinates; Length is in
// UTF-16 code units, as the protocol requires.
//
// StartChar follows the same convention as the rest of this server
// (lspPosition, identRange, formatDocument): the compiler's 1-based rune
// column minus one. That is exact for every line this DSL can produce
// identifiers on, since the lexer's isIdentStart admits only '_' and ASCII
// letters -- a non-ASCII rune can appear solely inside a string literal or a
// comment, so only a line already carrying one can drift, and only for spans
// to the right of it.
type tokenSpan struct {
	Line      int
	StartChar int
	Length    int
	TokenType int // index into semanticTokenLegend.TokenTypes
	// Modifiers is the token's modifier bitmask: bit N refers to the Nth
	// entry of semanticTokenLegend.TokenModifiers. Only
	// semTokDefinitionBit is ever set, on declaration identifiers; every
	// reference span carries 0.
	Modifiers uint32
}

// classifyFile returns every classified span in one document, sorted by
// position.
//
// No resolved *ir.Schema is required: like formatting, classification is
// purely syntactic. Every category this server distinguishes is decided by
// the slot an identifier occupies in the tree, not by what it resolves to,
// so an unresolvable document still highlights correctly -- which is exactly
// when highlighting matters most.
func classifyFile(file *ast.File, src string) []tokenSpan {
	collector := &spanCollector{taken: make(map[[2]int]bool)}

	collector.declList(fileDecls(file))
	collector.literalsAndKeywords(src)

	sortTokenSpans(collector.spans)

	return collector.spans
}

// fileDecls returns a file's top-level declarations.
func fileDecls(file *ast.File) []ast.Decl {
	if file == nil {
		return nil
	}

	return file.Decls
}

// sortTokenSpans orders spans by position, the order the delta encoding
// requires and the order tests assert against.
func sortTokenSpans(spans []tokenSpan) {
	sort.Slice(spans, func(i, j int) bool {
		if spans[i].Line != spans[j].Line {
			return spans[i].Line < spans[j].Line
		}

		return spans[i].StartChar < spans[j].StartChar
	})
}

// spanCollector accumulates spans while guaranteeing one classification per
// source position: the first tier to claim a position keeps it. The AST tier
// always runs first, so a semantic classification always beats the lexical
// fallback for the same span.
type spanCollector struct {
	spans []tokenSpan
	taken map[[2]int]bool
}

// add records the span covering ident, which begins at the compiler position
// pos. Empty text, an unpositioned node (the parser leaves a zero Position
// on nodes it recovered past) and an unknown token type are all dropped.
func (c *spanCollector) add(pos diag.Position, ident string, tokenType int) {
	c.addSpan(pos, utf16Len(ident), tokenType)
}

// addSpan records a span of an explicitly measured length, for text whose
// source extent is not simply the identifier itself.
func (c *spanCollector) addSpan(pos diag.Position, length, tokenType int) {
	c.addSpanWithModifiers(pos, length, tokenType, 0)
}

// addDefinition records a declaration-identifier span: a decl name
// (entity/message/service/rpc/job/schedule/saga/enum) or a field name at
// its declaration. The span carries the legend's "definition" modifier
// bit; every reference span added via add/addSpan carries none.
func (c *spanCollector) addDefinition(pos diag.Position, ident string, tokenType int) {
	c.addSpanWithModifiers(pos, utf16Len(ident), tokenType, semTokDefinitionBit)
}

// addSpanWithModifiers records a span of an explicitly measured length
// carrying modifier bits, for declaration identifiers (via addDefinition)
// and every other span (with a zero mask).
func (c *spanCollector) addSpanWithModifiers(pos diag.Position, length, tokenType int, modifiers uint32) {
	if pos.Line <= 0 || pos.Col <= 0 || length <= 0 || tokenType < 0 {
		return
	}

	key := [2]int{pos.Line - 1, pos.Col - 1}
	if c.taken[key] {
		return
	}

	c.taken[key] = true

	c.spans = append(c.spans, tokenSpan{
		Line:      key[0],
		StartChar: key[1],
		Length:    length,
		TokenType: tokenType,
		Modifiers: modifiers,
	})
}

// declList classifies a run of top-level declarations.
func (c *spanCollector) declList(decls []ast.Decl) {
	for _, decl := range decls {
		switch d := decl.(type) {
		case *ast.EntityDecl:
			c.entity(d)
		case *ast.MessageDecl:
			c.message(d)
		case *ast.ServiceDecl:
			c.service(d)
		case *ast.JobDecl:
			c.job(d)
		case *ast.ScheduleDecl:
			c.schedule(d)
		case *ast.EnumDecl:
			c.enumDecl(d)
		case *ast.SagaDecl:
			c.saga(d)
		}
	}
}

// entity classifies an entity declaration: the entity name is a class, and
// everything it declares (fields, relations, indexes) names properties of
// that class.
func (c *spanCollector) entity(decl *ast.EntityDecl) {
	if decl == nil {
		return
	}

	c.addDefinition(decl.NamePos, decl.Name, semTokClass)
	c.attributes(decl.Attributes)

	for _, field := range decl.Fields {
		if field == nil {
			continue
		}

		c.addDefinition(field.NamePos, field.Name, semTokProperty)
		c.typeExpr(field.Type)
		c.attributes(field.Attributes)
	}

	for _, rel := range decl.Relations {
		c.relation(rel)
	}

	for _, index := range decl.Indexes {
		c.index(index)
	}
}

// message classifies a message declaration: the message name is a class,
// and each field is a property of that class, same as an entity's fields --
// a message just never has relations or indexes.
func (c *spanCollector) message(decl *ast.MessageDecl) {
	if decl == nil {
		return
	}

	c.addDefinition(decl.NamePos, decl.Name, semTokClass)

	for _, field := range decl.Fields {
		if field == nil {
			continue
		}

		c.addDefinition(field.NamePos, field.Name, semTokProperty)
		c.typeExpr(field.Type)
		c.attributes(field.Attributes)
	}
}

// enumDecl classifies a named enum declaration: the enum's name takes the
// class category (the same slot another named type's declaration occupies,
// since a named enum is a type the field-type grammar references) and each
// of its values an enum member, exactly like the inline `enum(a, b)` form's
// arguments in typeExpr. The `enum` keyword itself is reserved, so the token
// tier already labels it; the value identifiers are the first tier's to
// claim.
func (c *spanCollector) enumDecl(decl *ast.EnumDecl) {
	if decl == nil {
		return
	}

	c.addDefinition(decl.NamePos, decl.Name, semTokClass)

	for i, val := range decl.Values {
		if i >= len(decl.ValuePos) {
			break
		}

		c.add(decl.ValuePos[i], val, semTokEnumMember)
	}
}

// saga classifies a saga declaration: the saga name is an invocable unit
// (function, the same category a job takes), and each step's
// execute/compensate options name a Service.RPC pair -- the service
// identifier reads as the callable being invoked, the rpc identifier as the
// method chosen on it. A step carries no separate recorded position for its
// own name (the parser accepts the ident without storing it), so steps add
// nothing of their own; only the references they carry are classified.
func (c *spanCollector) saga(decl *ast.SagaDecl) {
	if decl == nil {
		return
	}

	c.addDefinition(decl.NamePos, decl.Name, semTokFunction)

	for _, step := range decl.Steps {
		for _, ref := range []*ast.RPCRef{step.Execute, step.Compensate} {
			if ref == nil {
				continue
			}

			c.add(ref.Pos, ref.Service, semTokFunction)

			if ref.RPC != "" {
				c.add(rpcRefPos(ref), ref.RPC, semTokMethod)
			}
		}
	}
}

// relation classifies a relation: the relation's own name is a property of
// the declaring entity, its target names another entity, and a many-to-many
// join block names the backing table.
func (c *spanCollector) relation(decl *ast.RelationDecl) {
	if decl == nil {
		return
	}

	c.addDefinition(decl.FieldNamePos, decl.FieldName, semTokProperty)
	c.add(decl.TargetPos, decl.Target, semTokClass)
	c.attributes(decl.Attributes)

	if decl.Join != nil {
		c.add(decl.Join.TablePos, decl.Join.Table, semTokClass)
	}
}

// index classifies an index: every column names a field of the entity, so
// the columns carry the same category the field declarations do.
func (c *spanCollector) index(decl *ast.IndexDecl) {
	if decl == nil {
		return
	}

	for i, col := range decl.Columns {
		if i >= len(decl.ColumnPos) {
			break
		}

		c.add(decl.ColumnPos[i], col, semTokProperty)
	}

	c.attributes(decl.Attributes)
}

// typeExpr classifies a type reference. Scalar names, `enum` and any
// unrecognized (typo'd) name are all types: what the identifier resolves to
// is the resolver's business, and a misspelling should still highlight as
// the type slot it occupies. An inline enum's values are enum members.
func (c *spanCollector) typeExpr(expr *ast.TypeExpr) {
	if expr == nil {
		return
	}

	c.add(expr.NamePos, expr.Name, semTokType)

	for i, arg := range expr.Args {
		if i >= len(expr.ArgPos) {
			break
		}

		c.add(expr.ArgPos[i], arg, semTokEnumMember)
	}
}

// service classifies a service and its rpcs. A service is an interface, an
// rpc a method on it.
func (c *spanCollector) service(decl *ast.ServiceDecl) {
	if decl == nil {
		return
	}

	c.addDefinition(decl.NamePos, decl.Name, semTokInterface)

	for _, rpc := range decl.RPCs {
		c.rpc(rpc)
	}
}

// rpc classifies one rpc declaration, including the entity references its
// return type and permission block carry.
func (c *spanCollector) rpc(decl *ast.RPCDecl) {
	if decl == nil {
		return
	}

	c.addDefinition(decl.NamePos, decl.Name, semTokMethod)
	c.add(decl.ReturnsPos, decl.Returns, semTokClass)

	for _, param := range decl.Params {
		c.param(param)
	}

	if decl.HTTP != nil {
		// The verb is one of a fixed set, so it reads as an enum member.
		// The path is a string literal and is left to the token tier.
		c.add(decl.HTTP.MethodPos, decl.HTTP.Method, semTokEnumMember)
	}

	// The permission block's `resource:` argument names an entity. Claim it
	// before the generic value walk, which would otherwise see a bare
	// identifier and call it an enum member.
	if ref, ok := permissionResourceRef(decl); ok {
		c.add(ref.Pos, ref.Name, semTokClass)
	}

	c.value(decl.Auth)
	c.value(decl.Permission)
}

// param classifies an rpc or job parameter.
func (c *spanCollector) param(decl *ast.ParamDecl) {
	if decl == nil {
		return
	}

	c.add(decl.NamePos, decl.Name, semTokParameter)
	c.typeExpr(decl.Type)
}

// job classifies a background job declaration. A job is an invocable unit,
// so it takes the function category; its queue name is a fixed-vocabulary
// identifier.
func (c *spanCollector) job(decl *ast.JobDecl) {
	if decl == nil {
		return
	}

	c.addDefinition(decl.NamePos, decl.Name, semTokFunction)
	c.add(decl.QueuePos, decl.Queue, semTokEnumMember)

	for _, param := range decl.Params {
		c.param(param)
	}

	for _, val := range decl.Retry {
		c.value(val)
	}
}

// schedule classifies a scheduled task. A schedule fires rather than being
// called, so it takes the event category; its cron spec is a string literal
// left to the token tier, and its dispatch target is a call naming a job.
func (c *spanCollector) schedule(decl *ast.ScheduleDecl) {
	if decl == nil {
		return
	}

	c.addDefinition(decl.NamePos, decl.Name, semTokEvent)
	c.value(decl.Dispatch)
}

// attributes classifies a run of `@name(...)` annotations. The name is a
// macro: it is user-written syntax that expands into behaviour, which is the
// closest standard category the legend offers. The leading '@' is left
// unclassified rather than assumed adjacent -- the lexer accepts `@ primary`
// even though the formatter never writes it.
func (c *spanCollector) attributes(attrs []*ast.Attribute) {
	for _, attr := range attrs {
		if attr == nil {
			continue
		}

		c.add(attr.NamePos, attr.Name, semTokMacro)

		c.args(attr.Args)
	}
}

// args classifies attribute/call arguments. A named argument's label is a
// parameter; Arg.Pos points at that label whenever Name is non-empty.
func (c *spanCollector) args(args []*ast.Arg) {
	for _, arg := range args {
		if arg == nil {
			continue
		}

		if arg.Name != "" {
			c.add(arg.Pos, arg.Name, semTokParameter)
		}

		c.value(arg.Value)
	}
}

// value classifies the identifier-shaped parts of a value expression.
// Literals are deliberately skipped here: only the token stream records
// their true source extent (an IntLit keeps a parsed int64, a StringLit an
// unescaped string), so the token tier owns them.
func (c *spanCollector) value(val ast.Value) {
	switch v := val.(type) {
	case *ast.IdentValue:
		c.add(v.Pos, v.Name, semTokEnumMember)
	case *ast.CallValue:
		c.add(v.NamePos, v.Name, semTokFunction)
		c.args(v.Args)
	case *ast.SetLit:
		for i, item := range v.Items {
			if i >= len(v.ItemPos) {
				break
			}

			c.add(v.ItemPos[i], item, semTokEnumMember)
		}
	}
}

// literalsAndKeywords is the token tier: it re-lexes the document and
// classifies the spans whose category the lexer already settled. Lexer
// errors are ignored on purpose -- a mid-edit document should still
// highlight the parts that did scan, matching how every other feature here
// degrades.
func (c *spanCollector) literalsAndKeywords(src string) {
	raw := []byte(src)

	tokens, _ := lexAll("semantic-tokens.zen", raw)
	lines := splitLines(raw)

	for _, tok := range tokens {
		switch tok.Kind { //nolint:exhaustive // the default arm covers every remaining kind
		case token.STRING:
			c.addSpan(tok.Pos, stringSpanLength(raw, lines, tok.Pos), semTokString)
		case token.INT, token.FLOAT, token.DURATION:
			// Lit is the raw source text for these kinds, so its width is
			// the span's width.
			c.addSpan(tok.Pos, utf16Len(tok.Lit), semTokNumber)
		default:
			if _, isKeyword := token.Keywords[tok.Lit]; isKeyword {
				c.addSpan(tok.Pos, utf16Len(tok.Lit), semTokKeyword)
			}
		}
	}
}

// stringSpanLength measures a string literal's true source width, quotes
// included. It cannot be derived from the token: Token.Lit holds the
// unescaped value, so `"a\nb"` (6 source characters) arrives as 3. Re-read
// the source instead, following the lexer's own escape rules, so an escaped
// or non-ASCII literal is highlighted over its real extent.
func stringSpanLength(src []byte, lines []sourceLine, pos diag.Position) int {
	index := pos.Line - 1
	if index < 0 || index >= len(lines) {
		return 0
	}

	start := offsetOf(src, lines[index], pos.Col)
	if start >= len(src) || src[start] != '"' {
		return 0
	}

	offset := start + 1

	for offset < len(src) {
		r, size := utf8.DecodeRune(src[offset:])
		if size == 0 || r == '\n' {
			break // unterminated: the lexer already reported it
		}

		offset += size

		if r == '"' {
			break
		}

		if r == '\\' && offset < len(src) {
			// Every escape the lexer recognizes is backslash plus exactly
			// one rune, and an unrecognized one is kept verbatim as the same
			// two runes, so skipping one rune is right in both cases.
			_, escSize := utf8.DecodeRune(src[offset:])
			offset += escSize
		}
	}

	return utf16Len(string(src[start:offset]))
}

// utf16Len returns s's length in UTF-16 code units, which is the unit the
// LSP measures token lengths in. Every identifier and keyword in this DSL is
// ASCII (lexer.isIdentStart admits only '_' and ASCII letters), so this
// agrees with len(s) for all of them; it matters only for string literals,
// the one place a non-ASCII rune can appear.
func utf16Len(s string) int {
	for i := range len(s) {
		if s[i] >= utf8.RuneSelf {
			return len(utf16.Encode([]rune(s)))
		}
	}

	return len(s)
}

// encodeSemanticTokens turns classified spans into the protocol's
// delta-encoded Data array: five integers per token, as
// (deltaLine, deltaStartChar, length, tokenType, tokenModifiers).
//
// deltaLine is always relative to the previous token's line, starting from
// line 0 for the first token. deltaStartChar is relative to the previous
// token's start character ONLY when deltaLine == 0, i.e. when both tokens
// sit on the same line; the moment a token starts a new line the character
// delta resets to the absolute column. Forgetting that reset is the classic
// semantic-token off-by-one -- every token after the first line boundary
// drifts by the previous line's last column -- so
// TestEncodeSemanticTokensDeltaEncodingAcrossLineBoundary pins it directly.
//
// Tracking prevChar from 0 makes the first token fall out of the same rule
// rather than needing a special case: on line 0 its delta is its own column
// minus zero.
//
// Spans are sorted here rather than trusted to arrive sorted, since the
// encoding is meaningless (and can produce negative deltas) in any other
// order. The input slice is copied first so callers keep their own ordering.
//
// The textDocument/semanticTokens/full handler wraps the result as
// protocol.SemanticTokens{Data: encodeSemanticTokens(spans)}; the legend
// capability is advertised as a *protocol.SemanticTokensOptions pointer with
// Full set to protocol.Boolean(true).
func encodeSemanticTokens(spans []tokenSpan) []uint32 {
	sorted := make([]tokenSpan, len(spans))
	copy(sorted, spans)
	sortTokenSpans(sorted)

	data := make([]uint32, 0, len(sorted)*5)
	prevLine, prevChar := 0, 0

	for _, span := range sorted {
		if span.Line < 0 || span.StartChar < 0 || span.Length <= 0 || span.TokenType < 0 {
			continue
		}

		deltaLine := span.Line - prevLine

		deltaChar := span.StartChar
		if deltaLine == 0 {
			deltaChar -= prevChar
		}

		data = append(data,
			uint32(deltaLine),      // spans sorted ascending: deltas never negative
			uint32(deltaChar),      // spans sorted ascending: deltas never negative
			uint32(span.Length),    // token lengths never negative
			uint32(span.TokenType), // token types are non-negative kind indexes
			span.Modifiers,         // declaration identifiers carry the definition bit, references none
		)

		prevLine, prevChar = span.Line, span.StartChar
	}

	return data
}

// semTokCacheEntry stores one full semantic-token result for delta requests.
type semTokCacheEntry struct {
	resultID string
	data     []uint32
}

// boolPtr returns a pointer to v, for capability fields that take *bool.
func boolPtr(v bool) *bool { return &v }

// filterTokenSpansToRange keeps only the spans that intersect rng (half-open
// LSP positions). Spans are single-line, so a span is dropped when its whole
// extent lies strictly before the range start or strictly after the range end.
func filterTokenSpansToRange(spans []tokenSpan, rng protocol.Range) []tokenSpan {
	out := make([]tokenSpan, 0, len(spans))

	for _, sp := range spans {
		if spanIntersectsRange(sp, rng) {
			out = append(out, sp)
		}
	}

	return out
}

// spanIntersectsRange reports whether the single-line span sp overlaps rng.
func spanIntersectsRange(sp tokenSpan, rng protocol.Range) bool {
	line := uint32(sp.Line) // non-negative

	if line < rng.Start.Line {
		return false
	}

	if line > rng.End.Line {
		return false
	}

	startChar := uint32(sp.StartChar)        // non-negative
	endChar := startChar + uint32(sp.Length) // non-negative

	if line == rng.Start.Line && endChar <= rng.Start.Character {
		return false
	}

	if line == rng.End.Line && startChar >= rng.End.Character {
		return false
	}

	return true
}

// computeSemTokDelta produces the edits transforming old into next.
func computeSemTokDelta(old, next []uint32) []protocol.SemanticTokensEdit {
	prefix := 0
	minLen := len(old)
	if len(next) < minLen {
		minLen = len(next)
	}

	for prefix < minLen && old[prefix] == next[prefix] {
		prefix++
	}

	suffix := 0
	for suffix < minLen-prefix && old[len(old)-1-suffix] == next[len(next)-1-suffix] {
		suffix++
	}

	deleteCount := len(old) - prefix - suffix
	insertData := append([]uint32(nil), next[prefix:len(next)-suffix]...)

	if deleteCount == 0 && len(insertData) == 0 {
		return []protocol.SemanticTokensEdit{}
	}

	return []protocol.SemanticTokensEdit{{
		Start:       uint32(prefix),
		DeleteCount: uint32(deleteCount), // deleteCount is a non-negative slice length difference
		Data:        insertData,
	}}
}
