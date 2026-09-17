package main

import (
	"sort"

	"go.lsp.dev/protocol"

	"github.com/zenta-dev/zever/internal/dsl/ast"
)

// referencesAt resolves the cursor to a symbol via renameTargetAt, then
// collects every symbolRef naming that symbol across every file in
// parsedFiles (including, if includeDeclaration, the declaration itself),
// returning LSP Locations sorted by URI then position. parsedFiles is
// expected to come from Workspace.ParsedFiles, which parses each file at
// most once per content change rather than once per request. A cursor that
// does not resolve to a renameable symbol yields an empty result, not an
// error: unlike rename, "find references" is allowed to simply find nothing.
func referencesAt(
	file *ast.File,
	cursor protocol.Position,
	parsedFiles map[string]*ast.File,
	includeDeclaration bool,
) protocol.LocationSlice {
	target, ok := renameTargetAt(file, cursor)
	if !ok {
		return nil
	}

	// Proof: renameTargetAt yields only the five kinds refsFnForKind handles,
	// so refsFn is non-nil on every reachable path; the old nil guard was
	// dead (the unknown-kind arm stays pinned by TestRefsFnForKindUnknownKindReturnsNil).
	refsFn := refsFnForKind(target)

	declPath := ""
	if file != nil {
		declPath = file.Name
	}

	var locations protocol.LocationSlice

	for path, f := range parsedFiles {
		if f == nil {
			continue
		}

		for _, ref := range refsFn(f) {
			if ref.Name != target.Name || ref.Pos.Line <= 0 {
				continue
			}

			isDecl := path == declPath && ref.Pos == target.Pos
			if isDecl && !includeDeclaration {
				continue
			}

			locations = append(locations, protocol.Location{
				URI:   pathToURI(path),
				Range: identRange(ref.Pos, ref.Name),
			})
		}
	}

	sortLocations(locations)

	return locations
}

// refsFnForKind returns the *RefsIn walk matching target.Kind, pre-bound to
// target.Owner where the kind is entity- or service-scoped (field, rpc). Nil
// means the kind is not one referencesAt (or renameTargetAt) knows how to
// resolve to a workspace-wide walk.
func refsFnForKind(target renameTarget) func(*ast.File) []symbolRef {
	switch target.Kind {
	case entityKind:
		return entityRefsIn
	case fieldKind:
		return func(f *ast.File) []symbolRef { return fieldRefsIn(f, target.Owner) }
	case jobKind:
		return jobRefsIn
	case serviceKind:
		return serviceRefsIn
	case rpcKind:
		return func(f *ast.File) []symbolRef { return rpcRefsIn(f, target.Owner) }
	default:
		return nil
	}
}

// sortLocations orders locations by URI then position so the result is
// deterministic regardless of the files map's iteration order.
func sortLocations(locations protocol.LocationSlice) {
	sort.Slice(locations, func(i, j int) bool {
		a, b := locations[i], locations[j]
		if a.URI != b.URI {
			return a.URI < b.URI
		}

		if a.Range.Start.Line != b.Range.Start.Line {
			return a.Range.Start.Line < b.Range.Start.Line
		}

		return a.Range.Start.Character < b.Range.Start.Character
	})
}
