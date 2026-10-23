package main

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"

	"go.lsp.dev/protocol"

	"github.com/zenta-dev/zever/dsl/ast"
	"github.com/zenta-dev/zever/dsl/ir"
	"github.com/zenta-dev/zever/dsl/lexer"
	"github.com/zenta-dev/zever/dsl/resolver"
	"github.com/zenta-dev/zever/dsl/token"

	"github.com/zenta-dev/zever/shared/codec"
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
	ctxServiceMember
	ctxRPCOptionName
	ctxJobOptionName
	ctxScheduleOptionName
	ctxDispatchJob
	ctxHTTPVerb
	ctxDefaultEnumValue
	ctxMessageType
)

// candidate is one completion suggestion before it becomes a protocol item:
// a label, an optional one-line explanation, and an optional snippet body.
// An empty snippet means the item inserts its label verbatim; a non-empty
// snippet is offered as a snippet-format insert only to clients that
// negotiated snippetSupport, and degrades to the plain label otherwise.
type candidate struct {
	label   string
	detail  string
	snippet string
}

// scalarTypeDescriptions carries the completion detail for each scalar type
// name resolver.scalarTypes can yield. `datetime` is the resolver's alias
// of `timestamp` (both resolve to the same ir type), so the two descriptions
// deliberately match.
var scalarTypeDescriptions = map[string]string{
	"uuid":      "UUID scalar",
	"string":    "string scalar",
	"int32":     "32-bit signed integer",
	"int64":     "64-bit signed integer",
	"float32":   "32-bit float",
	"float64":   "64-bit float",
	"bool":      "boolean scalar",
	"timestamp": "timestamp scalar",
	"datetime":  "timestamp scalar",
	"date":      "date scalar",
	"bytes":     "byte string scalar",
	"json":      "JSON document scalar",
}

// scalarTypeCandidates is derived from resolver.ScalarTypeNames() -- the
// same table resolveFieldType resolves field types from -- so the candidate
// list can never advertise a scalar the resolver rejects or omit one it
// accepts (including the `datetime` alias, which a hand-maintained list
// would drift away from). Built once from that source of truth, with
// "enum" appended on top of it -- the resolver handles enum through a
// separate arg-list path rather than the scalarTypes table -- and sorted
// by label, since map iteration order is random and this feeds directly
// into completion output.
var scalarTypeCandidates = buildScalarTypeCandidates()

// buildScalarTypeCandidates builds the field-type completion candidates
// from resolver.ScalarTypeNames plus the separate `enum` candidate, sorted
// by label.
func buildScalarTypeCandidates() []candidate {
	names := resolver.ScalarTypeNames()

	cands := make([]candidate, 0, len(names)+1)

	for _, name := range names {
		detail, ok := scalarTypeDescriptions[name]
		if !ok {
			// A scalar resolver learns about without a description here
			// still completes, phrased like the built-in ones.
			detail = name + " scalar"
		}

		cands = append(cands, candidate{name, detail, ""})
	}

	cands = append(cands, candidate{"enum", "enum(value, ...) inline enumeration", ""})

	sort.Slice(cands, func(i, j int) bool { return cands[i].label < cands[j].label })

	return cands
}

// topLevelKeywordCandidates mirrors token.Keywords minus true/false -- the
// words that unambiguously start a production -- plus the contextual "saga"
// keyword, which the parser accepts as a top-level declaration despite not
// reserving it. Keywords that open a block carry a snippet skeleton,
// offered as a snippet only to clients that negotiated snippetSupport;
// every other keyword inserts its label verbatim.
var topLevelKeywordCandidates = []candidate{
	{"entity", "declare an entity", "entity ${1:Name} {\n\t$0\n}"},
	{"message", "declare a non-persisted request/response message", "message ${1:Name} {\n\t$0\n}"},
	{"service", "declare a service", "service ${1:Name} {\n\t$0\n}"},
	{"job", "declare a background job", "job ${1:Name}(${2}) {\n\t$0\n}"},
	{"schedule", "declare a scheduled task", "schedule ${1:Name} {\n\t$0\n}"},
	{"rpc", "declare an rpc method", ""},
	{"index", "declare an index", ""},
	{"enum", "inline enumeration type", "enum ${1:Name} {\n\t$0\n}"},
	{"saga", "declare a saga", "saga ${1:Name} {\n\t$0\n}"},
	{"has_many", "one-to-many relation", ""},
	{"has_one", "one-to-one relation", ""},
	{"belongs_to", "inverse relation holding the foreign key", ""},
	{"many_to_many", "many-to-many relation via a join table", ""},
}

// entityMemberCandidates are the reserved words that can start an entity
// body member; a plain field starts with a bare identifier instead. The
// index keyword carries a snippet for its column list, offered only to
// clients that negotiated snippetSupport.
var entityMemberCandidates = []candidate{
	{"has_many", "one-to-many relation", ""},
	{"has_one", "one-to-one relation", ""},
	{"belongs_to", "inverse relation holding the foreign key", ""},
	{"many_to_many", "many-to-many relation via a join table", ""},
	{"index", "index(column, ...) declaration", "index ${1:cols}"},
}

// fieldAttrCandidates mirrors resolver.resolveFieldAttribute.
var fieldAttrCandidates = []candidate{
	{"primary", "mark the field as the primary key", ""},
	{"unique", "add a unique constraint", ""},
	{"validate", "validate(kind: value, ...) constraints", ""},
	{"default", "default(value) for new rows", ""},
	{"renamed_from", "renamed_from(\"old_name\") — record a column rename for zever db migrate", ""},
}

// relationAttrCandidates mirrors resolver.resolveRelationAttributes.
var relationAttrCandidates = []candidate{
	{"foreign_key", "foreign_key(column) backing this relation", ""},
	{"on_delete", "on_delete(restrict | cascade | set_null)", ""},
}

// entityAttrCandidates mirrors resolver.resolveEntitySchema.
var entityAttrCandidates = []candidate{
	{"schema", "schema(name) overriding the module schema", ""},
}

// indexAttrCandidates mirrors resolver.resolveIndex.
var indexAttrCandidates = []candidate{
	{"unique", "make the index unique", ""},
}

// stringValidateArgCandidates are the @validate kinds that apply only to
// string fields, mirroring resolver.resolveValidateRules.
var stringValidateArgCandidates = []candidate{
	{"format", "string format: email, url or uuid", ""},
	{"min_len", "minimum string length", ""},
	{"max_len", "maximum string length", ""},
}

// numericValidateArgCandidates are the @validate kinds that apply only to
// numeric fields, mirroring resolver.resolveValidateRules.
var numericValidateArgCandidates = []candidate{
	{"gt", "numeric greater-than bound", ""},
	{"gte", "numeric greater-or-equal bound", ""},
	{"lt", "numeric less-than bound", ""},
	{"lte", "numeric less-or-equal bound", ""},
}

// validateArgCandidates is the union of both kinds, offered when the field's
// declared scalar type cannot be determined (e.g. the declaration is
// mid-edit and the parser dropped it).
var validateArgCandidates = append(
	append([]candidate{}, stringValidateArgCandidates...),
	numericValidateArgCandidates...,
)

// serviceMemberCandidates mirrors parser.serviceMemberStart: only "rpc"
// starts a service-body member. The rpc keyword carries a snippet for its
// signature, offered only to clients that negotiated snippetSupport.
var serviceMemberCandidates = []candidate{
	{"rpc", "declare an rpc method", "rpc ${1:name}($2) -> ${3:Type} {\n\t$0\n}"},
}

// rpcOptionCandidates mirrors parser.rpcOptionStart.
var rpcOptionCandidates = []candidate{
	{"http", "http: METHOD \"path\" route binding", ""},
	{"auth", "auth: none | required | required(roles: {...})", ""},
	{"permission", "permission: check(resource: ...)", ""},
	{"errors", "errors: { code, ... } response error set", ""},
	{"paginated", "paginated: true | false", ""},
}

// jobOptionCandidates mirrors parser.jobOptionStart.
var jobOptionCandidates = []candidate{
	{"queue", "queue: name of the queue this job drains", ""},
	{"retry", "retry: max_attempts(n), backoff(...)", ""},
}

// scheduleOptionCandidates mirrors parser.scheduleOptionStart.
var scheduleOptionCandidates = []candidate{
	{"cron", "cron: \"0 0 * * *\" schedule spec", ""},
	{"dispatch", "dispatch: JobName() to run", ""},
}

// onDeleteCandidates mirrors resolver.resolveOnDeleteAttribute.
var onDeleteCandidates = []candidate{
	{"restrict", "refuse to delete while children exist", ""},
	{"cascade", "delete children along with the parent", ""},
	{"set_null", "null the foreign key on delete", ""},
}

// validateFormatCandidates mirrors resolver.decodeValidateValue.
var validateFormatCandidates = []candidate{
	{"email", "an email address", ""},
	{"url", "an absolute URL", ""},
	{"uuid", "a UUID string", ""},
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
		cands = append(cands, candidate{name, fmt.Sprintf("%d %s", code.HTTPStatus(), code.GRPCName()), ""})
	}

	sort.Slice(cands, func(i, j int) bool { return cands[i].label < cands[j].label })

	return cands
}

// httpVerbCandidates mirrors resolver_service.go's validHTTPMethods -- the
// fixed v1 set of HTTP verbs an rpc's `http:` option accepts, in a fixed
// conventional order. The resolver's set is private (and map-ordered), so
// the vocabulary is pinned by test rather than derived; a verb the resolver
// gains but this list omits fails the pin instead of drifting.
var httpVerbCandidates = []candidate{
	{"GET", "HTTP verb for the route binding", ""},
	{"POST", "HTTP verb for the route binding", ""},
	{"PUT", "HTTP verb for the route binding", ""},
	{"PATCH", "HTTP verb for the route binding", ""},
	{"DELETE", "HTTP verb for the route binding", ""},
}

// jobCandidates enumerates the jobs a schedule's dispatch slot can target:
// every resolved job in the workspace schema, plus any job declared in the
// current file the schema has not recovered (e.g. while the schedule above
// it is mid-edit). Merged, deduped by name and sorted by label, since the
// two sources can name the same job and schema module order is meaningless
// to a user picking a target.
func jobCandidates(schema *ir.Schema, file *ast.File) []candidate {
	seen := make(map[string]bool)

	out := make([]candidate, 0)

	add := func(name string) {
		if name == "" || seen[name] {
			return
		}

		seen[name] = true
		out = append(out, candidate{name, "job", ""})
	}

	if schema != nil {
		for _, module := range schema.Modules {
			for _, job := range module.Jobs {
				add(job.Name)
			}
		}
	}

	for _, decl := range flattenDecls(file) {
		job, ok := decl.(*ast.JobDecl)
		if ok {
			add(job.Name)
		}
	}

	sort.Slice(out, func(i, j int) bool { return out[i].label < out[j].label })

	return out
}

// typeCandidates is the candidate vocabulary of the type slots that accept
// entities and messages (a message field's type, an rpc's return type or a
// param's type): the scalar and inline-enum names an entity field's slot
// also takes, plus every entity and message in the workspace schema --
// these slots can reference either. Merged, deduped by label and sorted,
// since the entity and message vocabularies share one naming space.
func typeCandidates(schema *ir.Schema) []candidate {
	out := append([]candidate{}, scalarTypeCandidates...)

	for _, module := range allModules(schema) {
		for _, entity := range module.Entities {
			out = append(out, candidate{entity.Name, fmt.Sprintf("entity, %d field(s)", len(entity.Fields)), ""})
		}

		for _, msg := range module.Messages {
			out = append(out, candidate{msg.Name, fmt.Sprintf("message, %d field(s)", len(msg.Fields)), ""})
		}
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].label == out[j].label {
			return out[i].detail < out[j].detail
		}

		return out[i].label < out[j].label
	})

	return dedupeCandidates(out)
}

// allModules returns the schema's modules, tolerating a nil schema (completion
// runs off whatever the workspace could recover).
func allModules(schema *ir.Schema) []*ir.Module {
	if schema == nil {
		return nil
	}

	return schema.Modules
}

// dedupeCandidates drops adjacent duplicate labels. Callers sort first, so
// equal labels end up neighboring.
func dedupeCandidates(cands []candidate) []candidate {
	out := cands[:0]

	for i, cand := range cands {
		if i > 0 && cands[i-1].label == cand.label {
			continue
		}

		out = append(out, cand)
	}

	return out
}

// fileEnumAt returns the named enum declaration in file, or nil.
func fileEnumAt(file *ast.File, name string) *ast.EnumDecl {
	for _, decl := range flattenDecls(file) {
		enum, ok := decl.(*ast.EnumDecl)
		if ok && enum.Name == name {
			return enum
		}
	}

	return nil
}

// enumValueCandidatesFor enumerates the values @default( accepts when the
// enclosing field's declared type names a resolvable enum: the values of
// the named enum -- from the workspace schema when it resolves, else from
// any enum declaration in the current file. When the field's type is not a
// resolvable named enum it returns nothing: a free-form default is a guess
// completion must not make.
func enumValueCandidatesFor(schema *ir.Schema, file *ast.File, src string, cursor protocol.Position) []candidate {
	name := declaredFieldTypeAt(src, cursor)
	if name == "" {
		return nil
	}

	var values []string

	if enum := findEnum(schema, name); enum != nil {
		values = enum.Values
	} else if decl := fileEnumAt(file, name); decl != nil {
		values = decl.Values
	} else {
		return nil
	}

	out := make([]candidate, 0, len(values))
	for _, value := range values {
		out = append(out, candidate{value, fmt.Sprintf("enum %s value", name), ""})
	}

	return out
}

// completionAt returns the completion items for a cursor in one document.
// It never returns nil items for a known context; an unknown context yields
// an empty slice so callers can treat "no completions" uniformly. It
// assumes a client with no negotiated capabilities, so every item inserts
// its label as plain text; completionAtWithCaps is the capability-aware
// entry point.
//
// The textDocument/completion handler wraps a non-empty result in a
// CompletionList via completionListFor and returns a literal nil when there
// is nothing to offer.
func completionAt(
	schema *ir.Schema,
	file *ast.File,
	src string,
	cursor protocol.Position,
) []protocol.CompletionItem {
	return completionAtWithCaps(schema, file, src, cursor, clientCaps{})
}

// completionAtWithCaps is completionAt for a client whose capabilities were
// negotiated at Initialize: when caps reports snippetSupport, candidates
// carrying a snippet skeleton insert it as a snippet; otherwise every item
// inserts its label as plain text, exactly like completionAt.
func completionAtWithCaps(
	schema *ir.Schema,
	file *ast.File,
	src string,
	cursor protocol.Position,
	caps clientCaps,
) []protocol.CompletionItem {
	ctx, prefix := completionContextAt(file, src, cursor)

	switch ctx {
	case ctxTopLevelKeyword:
		return items(topLevelKeywordCandidates, protocol.CompletionItemKindKeyword, prefix, cursor, caps)
	case ctxEntityMember:
		return items(entityMemberCandidates, protocol.CompletionItemKindKeyword, prefix, cursor, caps)
	case ctxFieldType:
		return items(scalarTypeCandidates, protocol.CompletionItemKindClass, prefix, cursor, caps)
	case ctxRelationTarget:
		return items(entityCandidates(schema), protocol.CompletionItemKindClass, prefix, cursor, caps)
	case ctxFieldAttrName:
		return items(fieldAttrCandidates, protocol.CompletionItemKindProperty, prefix, cursor, caps)
	case ctxRelationAttrName:
		return items(relationAttrCandidates, protocol.CompletionItemKindProperty, prefix, cursor, caps)
	case ctxEntityAttrName:
		return items(entityAttrCandidates, protocol.CompletionItemKindProperty, prefix, cursor, caps)
	case ctxIndexAttrName:
		return items(indexAttrCandidates, protocol.CompletionItemKindProperty, prefix, cursor, caps)
	case ctxValidateArgName:
		return items(validateArgCandidatesFor(validateScalarAt(src, cursor)), protocol.CompletionItemKindProperty, prefix, cursor, caps)
	case ctxOnDeleteValue:
		return items(onDeleteCandidates, protocol.CompletionItemKindEnumMember, prefix, cursor, caps)
	case ctxValidateFormatValue:
		return items(validateFormatCandidates, protocol.CompletionItemKindEnumMember, prefix, cursor, caps)
	case ctxErrorsValue:
		return items(errorCandidates, protocol.CompletionItemKindEnumMember, prefix, cursor, caps)
	case ctxServiceMember:
		return items(serviceMemberCandidates, protocol.CompletionItemKindKeyword, prefix, cursor, caps)
	case ctxRPCOptionName:
		return items(rpcOptionCandidates, protocol.CompletionItemKindProperty, prefix, cursor, caps)
	case ctxJobOptionName:
		return items(jobOptionCandidates, protocol.CompletionItemKindProperty, prefix, cursor, caps)
	case ctxScheduleOptionName:
		return items(scheduleOptionCandidates, protocol.CompletionItemKindProperty, prefix, cursor, caps)
	case ctxDispatchJob:
		return items(jobCandidates(schema, file), protocol.CompletionItemKindFunction, prefix, cursor, caps)
	case ctxHTTPVerb:
		return items(httpVerbCandidates, protocol.CompletionItemKindEnumMember, prefix, cursor, caps)
	case ctxDefaultEnumValue:
		return items(enumValueCandidatesFor(schema, file, src, cursor), protocol.CompletionItemKindEnumMember, prefix, cursor, caps)
	case ctxMessageType:
		return items(typeCandidates(schema), protocol.CompletionItemKindClass, prefix, cursor, caps)
	case ctxNone:
		return nil
	}
	// Proof: completionContextAt yields only the twenty contexts dispatched
	// above plus ctxNone, and both ctxNone and any (impossible) unknown
	// context mean "nothing to enumerate", so they share this single nil
	// return, which the existing ctxNone tests cover.
	return nil
}

// validateArgCandidatesFor narrows the @validate arg-name candidates to the
// kinds that apply to scalar (a field's declared type name). An empty scalar
// means "not determinable" and keeps the full list; a known scalar with no
// applicable kind (uuid, bool, timestamp, date, bytes, json, enum) offers
// none, matching resolver.resolveValidateRules.
func validateArgCandidatesFor(scalar string) []candidate {
	switch scalar {
	case "":
		return validateArgCandidates
	case "string":
		return stringValidateArgCandidates
	case "int32", "int64", "float32", "float64":
		return numericValidateArgCandidates
	default:
		return nil
	}
}

// validateScalarAt returns the declared scalar type name of the field or
// param whose @validate(...) the cursor sits inside, or "" when it cannot be
// determined. The parser drops a field whose attribute is left unclosed, so
// this reads the re-lexed tokens (which still hold the `name: type @validate(`
// prefix) rather than the AST; attrTargetTypeAfter does the token back-track.
// Attributes of other names (e.g. a `@default(0) @validate(` predecessor) are
// rejected up front so the caller cannot attribute another attribute's paren
// to validate.
func validateScalarAt(src string, cursor protocol.Position) string {
	return attrScalarAt(src, cursor, "validate")
}

// declaredFieldTypeAt returns the declared type name of the field whose
// `@default(` the cursor sits inside, the same way validateScalarAt reads
// it back for @validate. The name may be a scalar ("uuid") or a named type
// ("Status"): only a name that resolves to a named enum makes the default
// slot enumerable, which enumValueCandidatesFor decides.
func declaredFieldTypeAt(src string, cursor protocol.Position) string {
	return attrScalarAt(src, cursor, "default")
}

// attrScalarAt reads the declared field type name governing the unclosed
// argument list of `@name(` at the cursor, or "" when it cannot be
// determined. The parser drops a field whose attribute is left unclosed, so
// this reads the re-lexed tokens (which still hold the `name: type @name(`
// prefix) rather than the AST. It scans back for the nearest colon outside
// any preceding attribute's parentheses -- the field's own `name: type`
// colon -- and returns the token after it, so `@default(0) @validate(` and
// an earlier param's colon do not confuse it.
func attrScalarAt(src string, cursor protocol.Position, attr string) string {
	toks := lexPrefix(sourceBefore(src, cursor))

	open := openParenIndex(toks)
	if open < 2 || toks[open-2].Kind != token.AT || toks[open-1].Lit != attr {
		return ""
	}

	atLine := toks[open-2].Pos.Line
	depth := 0

	for i := open - 3; i >= 0; i-- {
		if toks[i].Pos.Line != atLine {
			return ""
		}

		switch toks[i].Kind { //nolint:exhaustive // only parens, the type colon, and declaration braces matter here
		case token.RPAREN:
			depth++
		case token.LPAREN:
			if depth > 0 {
				depth--
			}
		case token.COLON:
			if depth == 0 && i+1 < len(toks) {
				return toks[i+1].Lit
			}
		case token.LBRACE, token.RBRACE:
			return ""
		default:
		}
	}

	return ""
}

// entityCandidates turns every resolved entity into a completion candidate,
// labelled with its field count the way hover phrases it.
func entityCandidates(schema *ir.Schema) []candidate {
	entities := allEntityNames(schema)

	out := make([]candidate, 0, len(entities))
	for _, entity := range entities {
		out = append(out, candidate{entity.Name, fmt.Sprintf("entity, %d field(s)", len(entity.Fields)), ""})
	}

	return out
}

// items prefix-filters candidates and converts the survivors to protocol
// items of one kind. It is the single item constructor: every context goes
// through it so the shapes stay identical.
//
// A candidate carrying a snippet inserts it as a snippet when the client
// negotiated snippetSupport, and degrades to its plain label otherwise
// (including when caps is the zero value, which is exactly the
// pre-snippet behavior). When the user already typed a prefix, each item
// also carries a TextEdit covering that prefix on the cursor line, so
// accepting the item replaces the partial word instead of duplicating it;
// the edit's text is the same effective insert text the item carries.
//
// Detail is deliberately not attached here: the LSP spec's intended pattern
// for rich completion is a fast, lightweight initial list followed by a
// lazy completionItem/resolve for whichever single item the user highlights.
// Instead, each item's Data field carries just enough to reconstruct the
// detail (and any richer documentation) on resolve -- see
// completionResolveData and completionResolve. Data is LSPAny (raw JSON),
// so the payload is marshaled; an item with no detail keeps Data zero.
func items(cands []candidate, kind protocol.CompletionItemKind, prefix string, cursor protocol.Position, caps clientCaps) []protocol.CompletionItem {
	out := make([]protocol.CompletionItem, 0, len(cands))

	for _, cand := range cands {
		if !hasPrefix(cand.label, prefix) {
			continue
		}

		group := completionGroup(kind, cand)
		sortPrefix := fmt.Sprintf("%d_", group)

		insert := cand.label
		format := protocol.InsertTextFormatPlainText

		if cand.snippet != "" && caps.snippetSupport {
			insert = cand.snippet
			format = protocol.InsertTextFormatSnippet
		}

		item := protocol.CompletionItem{
			Label:            cand.label,
			Kind:             kind,
			SortText:         protocol.NewOptional(sortPrefix + cand.label),
			FilterText:       protocol.NewOptional(cand.label),
			InsertText:       protocol.NewOptional(insert),
			InsertTextFormat: format,
		}

		if prefix != "" {
			item.TextEdit = &protocol.TextEdit{
				Range:   prefixRange(cursor, prefix),
				NewText: insert,
			}
		}

		if cand.detail != "" {
			item.Detail.Set(cand.detail)
			detail := cand.detail
			item.LabelDetails = &protocol.CompletionItemLabelDetails{Detail: &detail}

			if raw, err := codec.Marshal(completionResolveData{Detail: cand.detail}); err == nil {
				item.Data = protocol.LSPAny(raw)
			}
		}

		out = append(out, item)
	}

	return out
}

// completionListItemDefaultFormat is the CompletionList.itemDefaults
// property name (LSP 3.18 `completionList.itemDefaults: string[]`) for the
// insert-text format default this server can emit.
const completionListItemDefaultFormat = "insertTextFormat"

// completionListFor wraps items in a CompletionList, attaching ItemDefaults
// for the fields every item repeats verbatim -- but only for fields the
// client advertised in `completionList.itemDefaults`.
//
// Of everything items() sets uniformly, only InsertTextFormat is even
// eligible for defaults: SortText embeds a per-item group prefix plus the
// label, FilterText is the label itself, and InsertText carries either the
// label or a per-candidate snippet. Kind is constant per response but has
// no defaults-counterpart in CompletionItemDefaults, so it stays item-only.
// InsertTextFormat is constant across a response exactly when no item
// carries a snippet -- the snippetSupport-off shape, where every item is
// PlainText -- or when every item happens to be a snippet; a mixed response
// (snippet-capable client on a vocabulary with skeletons) emits no default.
//
// Item values stay explicit even when a default is emitted, and applyKind is
// never set: without applyKind the spec's default Replace combination means
// an explicit item value always wins over the default, so the defaults are a
// pure bandwidth hint and the rendered behavior is byte-for-byte identical
// to the bare-slice shape. The bandwidth win is partial (one shared enum
// instead of N repeated ones) in exchange for zero behavioral risk.
func completionListFor(items []protocol.CompletionItem, caps clientCaps) *protocol.CompletionList {
	list := &protocol.CompletionList{Items: items}

	if len(items) == 0 || !advertisesCompletionDefault(caps, completionListItemDefaultFormat) {
		return list
	}

	format := items[0].InsertTextFormat

	for _, item := range items[1:] {
		if item.InsertTextFormat != format {
			return list
		}
	}

	list.ItemDefaults = &protocol.CompletionItemDefaults{InsertTextFormat: format}

	return list
}

// advertisesCompletionDefault reports whether the client opted into the
// named CompletionList.itemDefaults property at Initialize (LSP 3.18
// `completionList.itemDefaults: string[]`). Matching is exact: the spec's
// property names are camelCase JSON keys like "insertTextFormat".
func advertisesCompletionDefault(caps clientCaps, field string) bool {
	for _, advertised := range caps.completionListDefaults {
		if advertised == field {
			return true
		}
	}

	return false
}

// prefixRange returns the single-line range covering the already-typed
// prefix ending at the cursor, so a completion TextEdit replaces the
// partial word instead of duplicating it. The start clamps to the line
// start, since a prefix longer than the cursor column can never match.
func prefixRange(cursor protocol.Position, prefix string) protocol.Range {
	start := int(cursor.Character) - len(prefix)
	if start < 0 {
		start = 0
	}

	return protocol.Range{
		Start: protocol.Position{Line: cursor.Line, Character: uint32(start)},
		End:   cursor,
	}
}

// completionGroup assigns a context-priority bucket used to build SortText:
// keywords first (0), then scalars (1), then entities (2). Everything else
// defaults to the scalar bucket so it lands between keywords and entities.
func completionGroup(kind protocol.CompletionItemKind, cand candidate) int {
	switch kind { //nolint:exhaustive // only keyword/class buckets matter; everything else defaults
	case protocol.CompletionItemKindKeyword:
		return 0
	case protocol.CompletionItemKindClass:
		if strings.HasPrefix(cand.detail, "entity") {
			return 2
		}

		return 1
	default:
		return 1
	}
}

// completionResolveDataKey is the map key completionResolveData round-trips
// under once a CompletionItem's Data field has gone through JSON (either an
// actual client round trip, or a Go map[string]any produced by
// codec.Unmarshal into an `any`).
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
		if err := codec.Unmarshal(raw, &payload); err == nil {
			return payload.Detail, payload.Detail != ""
		}

		var m map[string]any
		if err := codec.Unmarshal(raw, &m); err == nil {
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

	// Message fields, rpc returns and param types take bare type names that
	// can reference an entity or a message (not an entity the way relation
	// targets must), so their one slot offers the merged vocabulary.
	if _, field := messageFieldAt(file, cursor); field != nil && field.Type != nil &&
		coversIdent(field.Type.NamePos, field.Type.Name, cursor) {
		return ctxMessageType, field.Type.Name, true
	}

	if rpc := rpcReturnsAt(file, cursor); rpc != nil {
		return ctxMessageType, rpc.Returns, true
	}

	if param := paramTypeAt(file, cursor); param != nil {
		return ctxMessageType, param.Type.Name, true
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

	if ctx, ok := dispatchValueContext(toks); ok {
		return ctx, prefix
	}

	if ctx, ok := httpVerbContext(toks); ok {
		return ctx, prefix
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

// dispatchValueContext handles a cursor in the value slot of a schedule's
// `dispatch: JobName()` option: before the job name is typed, or inside its
// still-empty argument list. The slot needs its own tier because a dispatch
// value is a plain call value, not an @attr argument -- without it the
// unclosed `(` is dismissed as non-enumerable by attrArgContext and the
// bare `dispatch:` colon is misclassified as a field type slot by
// colonContext. It only fires inside a schedule's block, so a field
// legitimately named `dispatch` elsewhere still completes as a field. A
// call whose argument list already has tokens is past the job-name slot
// and reports ctxNone with ok=true, the same "stop trying other tiers"
// answer attrArgContext gives for a paren it recognizes.
func dispatchValueContext(toks []token.Token) (completionContext, bool) {
	if n := len(toks); n >= 2 &&
		toks[n-1].Kind == token.COLON &&
		toks[n-2].Kind == token.IDENT && toks[n-2].Lit == "dispatch" &&
		toks[n-1].Pos.Line == toks[n-2].Pos.Line &&
		enclosingBlockKind(toks) == token.SCHEDULE {
		return ctxDispatchJob, true
	}

	open := openParenIndex(toks)
	if open < 3 {
		return ctxNone, false
	}

	if toks[open-1].Kind != token.IDENT ||
		toks[open-2].Kind != token.COLON ||
		toks[open-3].Kind != token.IDENT || toks[open-3].Lit != "dispatch" ||
		enclosingBlockKind(toks) != token.SCHEDULE {
		return ctxNone, false
	}

	if len(toks) > open+1 {
		// Something already typed inside the call: not the job-name slot.
		return ctxNone, true
	}

	return ctxDispatchJob, true
}

// httpVerbContext handles a cursor in the value slot of an rpc's
// `http: METHOD "path"` option before the verb is typed. It fires only
// inside an rpc's block, so a field legitimately named `http` elsewhere
// still completes as a field. The value itself is a bare verb followed by
// a string path, so no unclosed-paren shape arises: anything after a
// completed verb belongs to the path, which has nothing to enumerate.
func httpVerbContext(toks []token.Token) (completionContext, bool) {
	n := len(toks)
	if n < 2 {
		return ctxNone, false
	}

	if toks[n-1].Kind != token.COLON ||
		toks[n-2].Kind != token.IDENT || toks[n-2].Lit != "http" ||
		toks[n-1].Pos.Line != toks[n-2].Pos.Line ||
		enclosingBlockKind(toks) != token.RPC {
		return ctxNone, false
	}

	return ctxHTTPVerb, true
}

// enclosingBlockKind reports the declaration keyword whose block is still
// open at the end of toks -- the owner of the innermost unclosed brace --
// or token.ILLEGAL when no block is open. blockContext and the value-slot
// contexts share it so they all answer "what declaration am I inside?" the
// same way.
func enclosingBlockKind(toks []token.Token) token.Kind {
	open := openBraceIndex(toks)
	if open < 0 {
		return token.ILLEGAL
	}

	return enclosingDeclKind(toks, open)
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
	case "default":
		if len(inner) == 0 {
			// ctxDefaultEnumValue: the slot may be a named enum's value
			// list; completionAt decides from the field's declared type
			// whether anything is enumerable, so a scalar field's
			// free-form default still offers nothing.
			return ctxDefaultEnumValue, true
		}

		return ctxNone, true
	default:
		// foreign_key and schema take free-form values: there is
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
		return colonContext(toks, lineToks), true
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
func colonContext(toks, lineToks []token.Token) completionContext {
	if len(lineToks) < 2 {
		return ctxNone
	}

	if isRelationKeyword(lineToks[0].Kind) {
		return ctxRelationTarget
	}

	if len(lineToks) == 2 && lineToks[0].Kind == token.IDENT {
		// Message fields, rpc params and job params (as well as the
		// entities they can reference) take the merged entity+message+
		// scalar vocabulary; entity fields stay scalar-only.
		switch enclosingBlockKind(toks) { //nolint:exhaustive // only decl kinds that host type slots matter
		case token.MESSAGE, token.SERVICE, token.RPC, token.JOB:
			return ctxMessageType
		}

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

	switch enclosingDeclKind(toks, open) { //nolint:exhaustive // only declarations with an enumerable body vocabulary are listed
	case token.ENTITY:
		return ctxEntityMember
	case token.SERVICE:
		return ctxServiceMember
	case token.RPC:
		return ctxRPCOptionName
	case token.JOB:
		return ctxJobOptionName
	case token.SCHEDULE:
		return ctxScheduleOptionName
	default:
		// message and any other block have no body vocabulary.
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
		case token.ENTITY, token.SERVICE, token.JOB, token.SCHEDULE, token.RPC, token.MESSAGE:
			return toks[i].Kind
		default:
		}
	}

	return token.ILLEGAL
}
