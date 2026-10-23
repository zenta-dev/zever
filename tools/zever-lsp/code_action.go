package main

import (
	"encoding/json"
	"math"
	"strings"

	"go.lsp.dev/protocol"
	"go.lsp.dev/uri"

	"github.com/zenta-dev/zever/dsl/ast"
	"github.com/zenta-dev/zever/dsl/ir"
)

// quickFixKind is shared by every CodeAction this server produces: both
// fixes below are quick fixes for a diagnostic, never a refactoring.
var quickFixKind = protocol.CodeActionKindQuickFix

// sourceFormatAllKind is the kind of the whole-document format action. The
// v1.0.1 protocol constants stop at source.fixAll, so the formatAll kind is
// spelled from the source base the same way terraform-ls spells its own
// source.formatAll action.
var sourceFormatAllKind = protocol.CodeActionKind("source.formatAll")

// CodeAction resolve kinds carried in an action's Data field. They name the
// fix the action offers so codeAction/resolve can rebuild the edit lazily.
const (
	codeActionResolveKindScalar   = "scalar"
	codeActionResolveKindRelation = "relation"
	codeActionResolveKindFormat   = "format"
)

// codeActionResolveData is the payload stored on a CodeAction's Data field
// so codeAction/resolve can rebuild the edit without the original request.
// For the two quick fixes it records which fix applies, the document, and
// the diagnostic's position and message text; for the format action only
// the kind and document, since the edit is recomputed from the live
// buffer. It round-trips through JSON exactly like completionResolveData.
type codeActionResolveData struct {
	Kind      string `json:"kind"`
	URI       string `json:"uri"`
	Line      uint32 `json:"line"`
	Character uint32 `json:"character"`
	Message   string `json:"message"`
}

// truePtr is the sole *bool this package ever needs (IsPreferred), kept as
// a named helper so each fix doesn't have to spell out its own local.
func truePtr() *bool {
	v := true
	return &v
}

// diagnosticMessage extracts the plain text of a diagnostic's message union,
// which in go.lsp.dev/protocol is an InlayHintTooltip holding either a
// protocol.String or a *protocol.MarkupContent. Anything else (including a
// nil message) yields the empty string, which matches no known fix pattern
// and therefore produces no action.
func diagnosticMessage(d protocol.Diagnostic) string {
	switch m := d.Message.(type) {
	case protocol.String:
		return string(m)
	case *protocol.MarkupContent:
		if m == nil {
			return ""
		}

		return m.Value
	default:
		return ""
	}
}

// attachCodeActionData stores the resolve payload on an action's Data
// field, marshaled to raw JSON the way completion items carry theirs. A
// marshal failure leaves Data zero rather than failing the request: the
// payload is a static struct, so this never happens, and resolve treats a
// missing payload as "nothing to fill".
func attachCodeActionData(action *protocol.CodeAction, data codeActionResolveData) {
	raw, err := json.Marshal(data)
	if err != nil {
		return
	}

	action.Data = protocol.LSPAny(raw)
}

// codeActionResolveDataFromAny recovers the resolve payload an action
// carries, whichever shape it comes back in: the raw-JSON LSPAny this
// server attached (a real client round-trips Data through JSON, so it
// arrives as encoded bytes), the struct value attached in-process, or the
// map[string]any a JSON decode into an `any` produces. Anything else, or a
// payload without a kind, reports false.
func codeActionResolveDataFromAny(data any) (codeActionResolveData, bool) {
	if raw, ok := data.(protocol.LSPAny); ok {
		var payload codeActionResolveData
		if err := json.Unmarshal(raw, &payload); err == nil && payload.Kind != "" {
			return payload, true
		}

		var m map[string]any
		if err := json.Unmarshal(raw, &m); err == nil {
			return codeActionResolveDataFromMap(m)
		}

		return codeActionResolveData{}, false
	}

	switch v := data.(type) {
	case codeActionResolveData:
		return v, v.Kind != ""
	case *codeActionResolveData:
		if v == nil {
			return codeActionResolveData{}, false
		}

		return *v, v.Kind != ""
	case map[string]any:
		return codeActionResolveDataFromMap(v)
	default:
		return codeActionResolveData{}, false
	}
}

// codeActionResolveDataFromMap reads the payload from the map[string]any a
// JSON decode into an `any` produces, where numbers arrive as float64. A
// missing kind means this was never a resolve payload and reports false;
// every other field degrades to its zero value, which the fix constructors
// treat as "no match" rather than a wrong edit.
func codeActionResolveDataFromMap(m map[string]any) (codeActionResolveData, bool) {
	kind, _ := m["kind"].(string)
	if kind == "" {
		return codeActionResolveData{}, false
	}

	uriStr, _ := m["uri"].(string)
	message, _ := m["message"].(string)

	return codeActionResolveData{
		Kind:      kind,
		URI:       uriStr,
		Line:      uint32FromAny(m["line"]),
		Character: uint32FromAny(m["character"]),
		Message:   message,
	}, true
}

// uint32FromAny converts a JSON-decoded number (float64) or a Go int/uint32
// to a uint32, yielding 0 for anything else. Positions are best-effort: a
// garbled coordinate only moves the rebuilt lookup, and the fix
// constructors return nil rather than a wrong edit when nothing matches
// there.
func uint32FromAny(v any) uint32 {
	switch n := v.(type) {
	case float64:
		if n < 0 || n > math.MaxUint32 {
			return 0
		}

		return uint32(n)
	case uint32:
		return n
	case int:
		if n < 0 || n > math.MaxUint32 {
			return 0
		}

		return uint32(n)
	default:
		return 0
	}
}

// levenshtein returns the edit distance between a and b.
func levenshtein(a, b string) int {
	ra, rb := []rune(a), []rune(b)

	prev := make([]int, len(rb)+1)
	for j := range prev {
		prev[j] = j
	}

	curr := make([]int, len(rb)+1)

	for i := 1; i <= len(ra); i++ {
		curr[0] = i

		for j := 1; j <= len(rb); j++ {
			cost := 1
			if ra[i-1] == rb[j-1] {
				cost = 0
			}

			del := prev[j] + 1
			ins := curr[j-1] + 1
			sub := prev[j-1] + cost

			least := del
			if ins < least {
				least = ins
			}

			if sub < least {
				least = sub
			}

			curr[j] = least
		}

		prev, curr = curr, prev
	}

	return prev[len(rb)]
}

// nearestCandidate returns the candidate string closest to name by edit
// distance, or ("", false) if candidates is empty or nothing is close
// enough to be a plausible suggestion. The cutoff (distance <= half the
// candidate's length, minimum 1) avoids suggesting e.g. "bool" for a
// wildly unrelated identifier just because it happened to be nearest.
func nearestCandidate(name string, candidates []string) (string, bool) {
	best := ""
	bestDist := -1

	for _, c := range candidates {
		d := levenshtein(name, c)
		if bestDist == -1 || d < bestDist {
			best = c
			bestDist = d
		}
	}

	if bestDist == -1 {
		return "", false
	}

	threshold := len(best) / 2
	if threshold < 1 {
		threshold = 1
	}

	if bestDist > threshold {
		return "", false
	}

	return best, true
}

// diagnosticCodeOf returns the stable zen-### code carried by d's Code
// field, or "" when Code is absent or not a string. The publisher sets Code
// through diagnosticCode (see diagnostics.go); a codeless diagnostic means
// the class could not be determined, and dispatch falls back to message
// matching.
func diagnosticCodeOf(d protocol.Diagnostic) string {
	if s, ok := d.Code.(protocol.String); ok {
		return string(s)
	}

	return ""
}

// diagMatchesCodeOrMessage reports whether d belongs to the fix class named
// by code. A present Code decides on its own: a code-carrying diagnostic
// needs no message sniffing. A codeless diagnostic falls back to the legacy
// message-substring match, so older diagnostics keep working.
func diagMatchesCodeOrMessage(d protocol.Diagnostic, code, pattern string) bool {
	if c := diagnosticCodeOf(d); c != "" {
		return c == code
	}

	return strings.Contains(diagnosticMessage(d), pattern)
}

// fixUnknownScalarType handles the resolver's "unknown field type %q"
// diagnostic: it locates the offending field at the diagnostic's position
// (rather than parsing the type name out of the human-readable message,
// which is more fragile), finds the nearest valid scalar name, and offers
// to replace the bad type token with it.
func fixUnknownScalarType(file *ast.File, id uri.URI, d protocol.Diagnostic) *protocol.CodeAction {
	if !diagMatchesCodeOrMessage(d, diagCodeUnknownFieldType, "unknown field type") {
		return nil
	}

	_, field := fieldAt(file, d.Range.Start)
	if field == nil || field.Type == nil || field.Type.Name == "" {
		return nil
	}

	names := make([]string, 0, len(scalarTypeCandidates))
	for _, c := range scalarTypeCandidates {
		names = append(names, c.label)
	}

	best, ok := nearestCandidate(field.Type.Name, names)
	if !ok {
		return nil
	}

	edit := protocol.TextEdit{
		Range:   identRange(field.Type.NamePos, field.Type.Name),
		NewText: best,
	}

	action := &protocol.CodeAction{
		Title:       "Change to \"" + best + "\"",
		Kind:        &quickFixKind,
		Diagnostics: []protocol.Diagnostic{d},
		Edit: &protocol.WorkspaceEdit{
			Changes: map[uri.URI][]protocol.TextEdit{id: {edit}},
		},
		IsPreferred: truePtr(),
	}

	attachCodeActionData(action, codeActionResolveData{
		Kind:      codeActionResolveKindScalar,
		URI:       string(id),
		Line:      d.Range.Start.Line,
		Character: d.Range.Start.Character,
		Message:   diagnosticMessage(d),
	})

	return action
}

// fixUnresolvedRelationTarget handles the resolver's "relation %s.%s
// references unknown entity %q" diagnostic: it locates the relation whose
// target is under the diagnostic's position, finds the nearest resolved
// entity name, and offers to replace the bad target with it.
func fixUnresolvedRelationTarget(
	schema *ir.Schema,
	file *ast.File,
	id uri.URI,
	d protocol.Diagnostic,
) *protocol.CodeAction {
	if !diagMatchesCodeOrMessage(d, diagCodeUnknownRelationTarget, "references unknown entity") {
		return nil
	}

	rel := relationTargetAt(file, d.Range.Start)
	if rel == nil || rel.Target == "" {
		return nil
	}

	entities := allEntityNames(schema)
	if len(entities) == 0 {
		return nil
	}

	names := make([]string, 0, len(entities))
	for _, e := range entities {
		names = append(names, e.Name)
	}

	best, ok := nearestCandidate(rel.Target, names)
	if !ok {
		return nil
	}

	edit := protocol.TextEdit{
		Range:   identRange(rel.TargetPos, rel.Target),
		NewText: best,
	}

	action := &protocol.CodeAction{
		Title:       "Change to \"" + best + "\"",
		Kind:        &quickFixKind,
		Diagnostics: []protocol.Diagnostic{d},
		Edit: &protocol.WorkspaceEdit{
			Changes: map[uri.URI][]protocol.TextEdit{id: {edit}},
		},
		IsPreferred: truePtr(),
	}

	attachCodeActionData(action, codeActionResolveData{
		Kind:      codeActionResolveKindRelation,
		URI:       string(id),
		Line:      d.Range.Start.Line,
		Character: d.Range.Start.Character,
		Message:   diagnosticMessage(d),
	})

	return action
}

// wantsSourceAction reports whether a codeAction request's Only filter
// admits source actions: no filter admits everything, and any listed kind
// that is the source base or lives under it (source.formatAll,
// source.organizeImports, ...) admits them. A filter of only narrower
// kinds, such as quickfix alone, does not.
func wantsSourceAction(only []protocol.CodeActionKind) bool {
	if len(only) == 0 {
		return true
	}

	for _, kind := range only {
		if kind == protocol.CodeActionKindSource ||
			strings.HasPrefix(string(kind), string(protocol.CodeActionKindSource)+".") {
			return true
		}
	}

	return false
}

// formatAllAction builds the whole-document "Format document" source action
// for the document id: the formatDocument edits, expressed as a single
// versionless TextDocumentEdit under DocumentChanges, the same canonical
// output the formatting handlers produce. It returns nil when the document
// is already canonical (there is nothing to apply) or the source is empty.
func formatAllAction(id uri.URI, src []byte) *protocol.CodeAction {
	if len(src) == 0 {
		return nil
	}

	path, _ := uriToPath(string(id))

	edits := formatDocument(path, src)
	if len(edits) == 0 {
		return nil
	}

	elements := make([]protocol.TextDocumentEditElement, len(edits))
	for i := range edits {
		elements[i] = &edits[i]
	}

	action := &protocol.CodeAction{
		Title: "Format document",
		Kind:  &sourceFormatAllKind,
		Edit: &protocol.WorkspaceEdit{
			DocumentChanges: []protocol.DocumentChange{
				&protocol.TextDocumentEdit{
					TextDocument: protocol.OptionalVersionedTextDocumentIdentifier{
						TextDocumentIdentifier: protocol.TextDocumentIdentifier{URI: id},
					},
					Edits: elements,
				},
			},
		},
	}

	attachCodeActionData(action, codeActionResolveData{
		Kind: codeActionResolveKindFormat,
		URI:  string(id),
	})

	return action
}

// codeActionsAt returns quick fixes applicable at the diagnostics present
// in params.Context.Diagnostics, plus the whole-document format action when
// the request's Only filter admits source actions. Each diagnostic now
// carries a structured code (diagnosticCode in diagnostics.go), which the
// fixes prefer over message text: a present code decides dispatch on its
// own, and only codeless diagnostics fall back to message-substring
// matching. Mirrored diagnostics are built through the same matcher table,
// so dispatch and publishing stay coupled to one list of classes -- a
// deliberately narrow approach, not a general framework.
//
// src is the document's live buffer, used only for the format action; the
// quick fixes resolve against the parsed file and schema. A nil or empty
// src simply yields no format action.
//
// The result uses the server handler's []CommandOrCodeAction shape with
// *CodeAction arms; a literal nil means no fix applies.
func codeActionsAt(
	schema *ir.Schema,
	file *ast.File,
	id uri.URI,
	src []byte,
	params *protocol.CodeActionParams,
) []protocol.CommandOrCodeAction {
	var actions []protocol.CommandOrCodeAction

	for _, d := range params.Context.Diagnostics {
		if a := fixUnknownScalarType(file, id, d); a != nil {
			actions = append(actions, a)
		}

		if a := fixUnresolvedRelationTarget(schema, file, id, d); a != nil {
			actions = append(actions, a)
		}
	}

	if wantsSourceAction(params.Context.Only) {
		if a := formatAllAction(id, src); a != nil {
			actions = append(actions, a)
		}
	}

	return actions
}

// hasCodeActionEdit reports whether an action already carries an edit to
// apply, through either WorkspaceEdit arm this server produces. Resolve
// uses it to stay idempotent: an action that already has edits round-trips
// unchanged.
func hasCodeActionEdit(action *protocol.CodeAction) bool {
	if action == nil || action.Edit == nil {
		return false
	}

	return len(action.Edit.Changes) > 0 || len(action.Edit.DocumentChanges) > 0
}

// codeActionResolve fills in a code action's edit when the action arrived
// without one. Every action the server produces carries a resolve payload
// in Data, so the edit is rebuilt from the live workspace: the two quick
// fixes re-run their constructors against the current schema and buffer,
// and the format action recomputes formatDocument. An action that already
// has an edit, or whose payload is missing or no longer resolves (the user
// fixed the typo before resolve ran), round-trips unchanged: resolve is
// best-effort by design, never an error.
func (s *Server) codeActionResolve(action *protocol.CodeAction) *protocol.CodeAction {
	if hasCodeActionEdit(action) {
		return action
	}

	data, ok := codeActionResolveDataFromAny(action.Data)
	if !ok {
		return action
	}

	id := uri.URI(data.URI)

	switch data.Kind {
	case codeActionResolveKindFormat:
		content, ok := s.ws.DocContent(data.URI)
		if !ok {
			return action
		}

		if fresh := formatAllAction(id, []byte(content)); fresh != nil {
			action.Edit = fresh.Edit
		}

		return action
	case codeActionResolveKindScalar, codeActionResolveKindRelation:
		// Only the diagnostic's start position feeds the fix
		// constructors, so the payload stores just that; the end is
		// echoed back for shape. The message text round-trips as a plain
		// string (a markup diagnostic flattens to its value), which the
		// constructors' Contains match reads identically.
		pos := protocol.Position{Line: data.Line, Character: data.Character}
		diag := protocol.Diagnostic{
			Range:   protocol.Range{Start: pos, End: pos},
			Message: protocol.String(data.Message),
		}

		schema, file := s.schemaAndFile(data.URI)

		var fresh *protocol.CodeAction
		if data.Kind == codeActionResolveKindScalar {
			fresh = fixUnknownScalarType(file, id, diag)
		} else {
			fresh = fixUnresolvedRelationTarget(schema, file, id, diag)
		}

		if fresh != nil && fresh.Edit != nil {
			action.Edit = fresh.Edit
		}

		return action
	default:
		return action
	}
}
