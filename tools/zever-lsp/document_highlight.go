package main

import (
	"go.lsp.dev/protocol"

	"github.com/zenta-dev/zever/dsl/ast"
)

// highlightsAt resolves the cursor to a symbol via renameTargetAt, then
// returns every occurrence of that symbol within this SAME file only. This
// is the single-file-scoped special case of the workspace-wide reference
// walk: it calls the same *RefsIn function references.go and rename.go use,
// but only against file itself, never CollectAllFiles -- documentHighlight
// is defined by the LSP spec as "highlight all occurrences in the current
// document", not the whole workspace.
//
// Occurrences carry a read/write kind now that the kinds exist in the
// protocol: the occurrence starting exactly at the resolved target's own
// Pos -- the declaration's namePos when the cursor sits on the declaration,
// the resolved reference's position otherwise -- is the Write occurrence
// and every other occurrence is Read, so editors can stand out the
// declaration (or, for a highlight issued from a bare reference, the
// occurrence under the cursor) without the collapse the old flat
// DocumentHighlightKindText tagging produced.
func highlightsAt(file *ast.File, cursor protocol.Position) []protocol.DocumentHighlight {
	target, ok := renameTargetAt(file, cursor)
	if !ok {
		return nil
	}

	var refs []symbolRef

	switch target.Kind {
	case entityKind:
		refs = entityRefsIn(file)
	case fieldKind:
		refs = fieldRefsIn(file, target.Owner)
	case jobKind:
		refs = jobRefsIn(file)
	case serviceKind:
		refs = serviceRefsIn(file)
	case rpcKind:
		refs = rpcRefsIn(file, target.Owner)
	case messageKind:
		refs = messageRefsIn(file)
	case enumKind:
		refs = enumRefsIn(file)
	case sagaKind:
		refs = sagaRefsIn(file)
	}
	// Proof: renameTargetAt yields only the eight kinds dispatched above, so
	// refs is always assigned on reachable paths; an (impossible) unknown
	// kind leaves refs nil and the shared loop below yields nil, exactly as
	// the old default arm did.

	var highlights []protocol.DocumentHighlight

	for _, ref := range refs {
		if ref.Name != target.Name || ref.Pos.Line <= 0 {
			continue
		}

		kind := protocol.DocumentHighlightKindRead
		if ref.Pos == target.Pos {
			kind = protocol.DocumentHighlightKindWrite
		}

		highlights = append(highlights, protocol.DocumentHighlight{
			Range: identRange(ref.Pos, ref.Name),
			Kind:  kind,
		})
	}

	return highlights
}
