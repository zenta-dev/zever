package main

import (
	"context"
	"strings"

	"go.lsp.dev/protocol"

	"github.com/zenta-dev/zever/dsl/token"
)

// sigParam is one parameter of a call-like construct's signature: its name
// as the resolver actually accepts it, plus a one-line description mined
// from the resolver source (never invented) so hover-style help stays
// truthful to what the compiler will accept.
type sigParam struct {
	name string
	doc  string
}

// sigSpec is the full signature of one call-like construct.
type sigSpec struct {
	params []sigParam
}

// signatureSpecs maps a call-like construct's name to its signature. Every
// entry mirrors one resolver function:
//
//   - "validate": resolver_validate.go's resolveValidateRules -- the fixed
//     set of @validate kinds, each a named argument (format/min_len/max_len
//     apply to string fields only, gt/gte/lt/lte to numeric fields only).
//   - "required": resolver_service.go's resolveAuthCall -- required(roles:
//     {...}) inside an auth: option.
//   - "check": resolver_service.go's resolvePermission -- permission:
//     check(<string>, resource: <entity>, owner_field: <field>).
//   - "max_attempts", "backoff": resolver_job.go's resolveRetryPolicy,
//     decodeMaxAttempts and decodeBackoff -- a job's retry: list entries.
//   - "default": resolver_entity.go's resolveDefaultAttribute -- a field's
//     @default(...) value.
//   - "renamed_from": resolver_entity.go's resolveRenamedFromAttribute --
//     a field's @renamed_from("old_name").
var signatureSpecs = map[string]sigSpec{
	"validate": {params: []sigParam{
		{"format", `string format constraint: "email", "url" or "uuid" (string fields only)`},
		{"min_len", "minimum string length (string fields only)"},
		{"max_len", "maximum string length (string fields only)"},
		{"gt", "numeric greater-than bound (numeric fields only)"},
		{"gte", "numeric greater-or-equal bound (numeric fields only)"},
		{"lt", "numeric less-than bound (numeric fields only)"},
		{"lte", "numeric less-or-equal bound (numeric fields only)"},
	}},
	"required": {params: []sigParam{
		{"roles", "set literal of role names required to call this rpc, e.g. {owner, admin}"},
	}},
	"check": {params: []sigParam{
		{"policy", "positional string naming the permission check to evaluate"},
		{"resource", "entity this permission check applies to"},
		{"owner_field", "field on the resource entity holding the record's owner reference"},
	}},
	"max_attempts": {params: []sigParam{
		{"attempts", "maximum number of attempts, an integer greater than zero"},
	}},
	"backoff": {params: []sigParam{
		{"kind", "backoff kind; only exponential is supported in v1"},
		{"base", "base delay duration for exponential backoff, e.g. 30s"},
	}},
	"default": {params: []sigParam{
		{"value", "the default value: a literal matching the field's scalar type, true/false for bool fields, an enum member for enum fields, or now() for timestamp fields"},
	}},
	"renamed_from": {params: []sigParam{
		{"old_name", "the field's previous column name, so `zever db migrate` emits a rename instead of a drop+add"},
	}},
}

// signatureHelp answers textDocument/signatureHelp.
func (s *Server) signatureHelp(
	ctx context.Context,
	params *protocol.SignatureHelpParams,
) (*protocol.SignatureHelp, error) {
	s.captureClient(ctx)

	content, ok := s.ws.DocContent(string(params.TextDocument.URI))
	if !ok {
		return nil, nil //nolint:nilnil // LSP encodes "no signature help" as a null result
	}

	return signatureHelpAt(content, params.Position), nil
}

// signatureHelpAt classifies a cursor as sitting inside one of this DSL's
// call-like argument lists and, if so, returns the full signature for that
// call with ActiveParameter set to the cursor's current argument slot.
//
// Detection is purely lexical, following the same re-lex-up-to-cursor
// approach completion.go's lexicalContextAt already uses (and reuses its
// helpers -- lexPrefix, sourceBefore, openParenIndex, isWordToken -- rather
// than re-implementing them): the parser's error recovery can drop a whole
// declaration when the cursor sits right after an unclosed '(', so there is
// often no AST node left to inspect exactly where signature help matters
// most.
func signatureHelpAt(src string, cursor protocol.Position) *protocol.SignatureHelp {
	toks := lexPrefix(sourceBefore(src, cursor))

	open := openParenIndex(toks)
	if open <= 0 {
		return nil
	}

	nameTok := toks[open-1]
	if !isWordToken(nameTok) {
		return nil
	}

	spec, ok := signatureSpecs[nameTok.Lit]
	if !ok {
		return nil
	}

	index, name := activeArgument(toks[open+1:])

	if name != "" {
		if i := paramIndexByName(spec.params, name); i >= 0 {
			index = i
		}
	}

	if index >= len(spec.params) {
		index = len(spec.params) - 1
	}

	// Proof: index comes from activeArgument (starts at 0, only incremented)
	// and is only reassigned from paramIndexByName when the result is >= 0,
	// so index < 0 is impossible and the old clamp was dead.
	active := protocol.NewNullable(uint32(index)) //nolint:gosec // proven non-negative above

	return &protocol.SignatureHelp{
		Signatures:      []protocol.SignatureInformation{signatureInformation(nameTok.Lit, spec)},
		ActiveParameter: active,
	}
}

// activeArgument scans the tokens already typed inside a call's parens (up
// to the cursor) and returns the zero-based index of the argument the
// cursor currently sits in (a count of top-level commas), plus the argument
// name already typed there, if any ("name: " fully typed, not mid-typing).
// Commas inside a nested set literal ({a, b}) or nested call ((...)) do not
// count, since those belong to a value, not the outer argument list.
func activeArgument(inner []token.Token) (index int, name string) {
	depth := 0
	segStart := 0

	for i, tok := range inner {
		switch tok.Kind { //nolint:exhaustive // only nesting/comma tokens affect segmentation
		case token.LBRACE, token.LPAREN:
			depth++
		case token.RBRACE, token.RPAREN:
			if depth > 0 {
				depth--
			}
		case token.COMMA:
			if depth == 0 {
				index++
				segStart = i + 1
			}
		default:
		}
	}

	return index, segmentName(inner[segStart:])
}

// segmentName reports the argument name already typed in one "name: value"
// segment, or "" when the segment has no colon yet (the user is still
// typing the name, or the argument is positional).
func segmentName(seg []token.Token) string {
	if len(seg) >= 2 && isWordToken(seg[0]) && seg[1].Kind == token.COLON {
		return seg[0].Lit
	}

	return ""
}

// paramIndexByName returns the index of the parameter named name, or -1.
func paramIndexByName(params []sigParam, name string) int {
	for i, p := range params {
		if p.name == name {
			return i
		}
	}

	return -1
}

// signatureInformation renders one call's full parameter list.
func signatureInformation(name string, spec sigSpec) protocol.SignatureInformation {
	names := make([]string, len(spec.params))
	params := make([]protocol.ParameterInformation, len(spec.params))

	for i, p := range spec.params {
		names[i] = p.name
		params[i] = protocol.ParameterInformation{Label: protocol.String(p.name), Documentation: protocol.String(p.doc)}
	}

	return protocol.SignatureInformation{
		Label:      name + "(" + strings.Join(names, ", ") + ")",
		Parameters: params,
	}
}
