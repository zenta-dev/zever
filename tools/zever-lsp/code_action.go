package main

import (
	"strings"

	"go.lsp.dev/protocol"
	"go.lsp.dev/uri"

	"github.com/zenta-dev/zever/internal/dsl/ast"
	"github.com/zenta-dev/zever/internal/dsl/ir"
)

// quickFixKind is shared by every CodeAction this server produces: both
// fixes below are quick fixes for a diagnostic, never a refactoring.
var quickFixKind = protocol.CodeActionKindQuickFix

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

// fixUnknownScalarType handles the resolver's "unknown field type %q"
// diagnostic: it locates the offending field at the diagnostic's position
// (rather than parsing the type name out of the human-readable message,
// which is more fragile), finds the nearest valid scalar name, and offers
// to replace the bad type token with it.
func fixUnknownScalarType(file *ast.File, id uri.URI, d protocol.Diagnostic) *protocol.CodeAction {
	if !strings.Contains(diagnosticMessage(d), "unknown field type") {
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

	return &protocol.CodeAction{
		Title:       "Change to \"" + best + "\"",
		Kind:        &quickFixKind,
		Diagnostics: []protocol.Diagnostic{d},
		Edit: &protocol.WorkspaceEdit{
			Changes: map[uri.URI][]protocol.TextEdit{id: {edit}},
		},
		IsPreferred: truePtr(),
	}
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
	if !strings.Contains(diagnosticMessage(d), "references unknown entity") {
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

	return &protocol.CodeAction{
		Title:       "Change to \"" + best + "\"",
		Kind:        &quickFixKind,
		Diagnostics: []protocol.Diagnostic{d},
		Edit: &protocol.WorkspaceEdit{
			Changes: map[uri.URI][]protocol.TextEdit{id: {edit}},
		},
		IsPreferred: truePtr(),
	}
}

// codeActionsAt returns quick fixes applicable at the diagnostics present
// in params.Context.Diagnostics -- there is no structured diagnostic
// code in this codebase, so dispatch is by matching known message
// patterns, a deliberately narrow approach, not a general framework.
//
// The result uses the server handler's []CommandOrCodeAction shape with
// *CodeAction arms; a literal nil means no fix applies.
func codeActionsAt(
	schema *ir.Schema,
	file *ast.File,
	id uri.URI,
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

	return actions
}
