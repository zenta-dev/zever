package main

import (
	"strings"

	"go.lsp.dev/protocol"

	"github.com/zenta-dev/zever/dsl/ast"
	"github.com/zenta-dev/zever/dsl/ir"
)

// inlineSkeletonFor maps a block context to the single ghost-text skeleton
// offered there, reusing the snippet bodies the completion vocabulary pins:
// the most common new declaration at top level, and the sole
// snippet-bearing member keyword inside entity and service bodies. Every
// other context has no single obvious skeleton, so it offers nothing rather
// than guessing.
func inlineSkeletonFor(ctx completionContext) string {
	switch ctx { //nolint:exhaustive // only declaration-skeleton contexts offer ghost text
	case ctxTopLevelKeyword:
		return inlineSnippetFor(topLevelKeywordCandidates, "entity")
	case ctxEntityMember:
		return inlineSnippetFor(entityMemberCandidates, "index")
	case ctxServiceMember:
		return inlineSnippetFor(serviceMemberCandidates, "rpc")
	default:
		return ""
	}
}

// inlineSnippetFor returns the snippet body of the named candidate in table,
// or "" when the table holds no such snippet. Callers treat "" as "offer
// nothing", so a drifted table degrades to silence rather than a wrong
// skeleton.
func inlineSnippetFor(cands []candidate, label string) string {
	for _, cand := range cands {
		if cand.label == label {
			return cand.snippet
		}
	}

	return ""
}

// inlineCompletionAt returns the single ghost-text item for a cursor on an
// otherwise-empty (whitespace-only) line, or nil when there is nothing to
// offer. The block context comes from the same brace analysis completion
// uses (lexPrefix/sourceBefore/blockContext), so the two features agree on
// what declaration encloses the cursor.
//
// The schema, file and caps parameters keep this symmetric with
// completionAtWithCaps. The schema and file are unused because brace nesting
// alone decides the slot; caps is intentionally ignored -- ghost text is
// inherently a tab-stop skeleton, so the insert text is always
// snippet-shaped, and clients that cannot render snippets show the plain
// text instead.
func inlineCompletionAt(
	_ *ir.Schema,
	_ *ast.File,
	content string,
	cursor protocol.Position,
	_ clientCaps,
) *protocol.InlineCompletionItem {
	rng, ok := whitespaceLineRange(content, cursor)
	if !ok {
		return nil
	}

	skeleton := inlineSkeletonFor(blockContext(lexPrefix(sourceBefore(content, cursor))))
	if skeleton == "" {
		return nil
	}

	filter := ""

	return &protocol.InlineCompletionItem{
		InsertText: protocol.String(skeleton),
		FilterText: &filter,
		Range:      &rng,
	}
}

// whitespaceLineRange returns the same-line range covering the whitespace of
// the cursor's line, or false when the line holds anything else: ghost text
// fires only on an otherwise-empty line, so mid-line and mid-declaration
// cursors stay silent. A cursor past the end of the buffer has no line to
// anchor a same-line range to, so it stays silent too.
func whitespaceLineRange(content string, cursor protocol.Position) (protocol.Range, bool) {
	lines := strings.Split(content, "\n")

	line := int(cursor.Line)
	if line >= len(lines) {
		return protocol.Range{}, false
	}

	if strings.TrimSpace(lines[line]) != "" {
		return protocol.Range{}, false
	}

	width := uint32(len(lines[line])) //nolint:gosec // line width is non-negative

	return protocol.Range{
		Start: protocol.Position{Line: cursor.Line, Character: 0},
		End:   protocol.Position{Line: cursor.Line, Character: width},
	}, true
}
