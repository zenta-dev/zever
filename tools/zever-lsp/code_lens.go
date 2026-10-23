package main

import (
	"strconv"

	"go.lsp.dev/protocol"

	"github.com/zenta-dev/zever/dsl/ast"
	"github.com/zenta-dev/zever/dsl/diag"
	"github.com/zenta-dev/zever/dsl/ir"
)

// codeLensesAt returns one reference-count lens per top-level declaration
// (entity, message, service, RPC, job, schedule, saga, enum) in the document
// uriStr names. Each lens carries its count as an info-only title -- "N
// reference(s)" -- with no executable command behind it: there is no
// executeCommand round-trip to run, only text for the client to render above
// the declaration line. files is expected to come from Workspace.ParsedFiles,
// the same per-request cache referencesAt and the rename builders read, so
// counts reflect the whole workspace rather than the single document.
//
// An unknown document yields nil, not an error, matching DocumentSymbol and
// InlayHint: there is simply no declaration to annotate.
func codeLensesAt(schema *ir.Schema, files map[string]*ast.File, uriStr string) []protocol.CodeLens {
	// Accepted for symmetry with the other schema-aware feature entries
	// (hoverAt, inlayHintsFor): today's count is purely syntactic, reusing
	// the rename/reference AST walks, so the resolved schema is not needed.
	_ = schema

	path, ok := uriToPath(uriStr)
	if !ok {
		return nil
	}

	file := files[path]
	if file == nil {
		return nil
	}

	var lenses []protocol.CodeLens

	for _, decl := range flattenDecls(file) {
		switch d := decl.(type) {
		case *ast.EntityDecl:
			lenses = append(lenses, codeLensFor(files, path, d.Name, d.NamePos, entityRefsIn))
		case *ast.MessageDecl:
			lenses = append(lenses, codeLensFor(files, path, d.Name, d.NamePos, messageRefsIn))
		case *ast.ServiceDecl:
			lenses = append(lenses, codeLensFor(files, path, d.Name, d.NamePos, serviceRefsIn))

			for _, rpc := range d.RPCs {
				if rpc == nil {
					continue
				}

				serviceName := d.Name

				lenses = append(lenses, codeLensFor(files, path, rpc.Name, rpc.NamePos,
					func(f *ast.File) []symbolRef { return rpcRefsIn(f, serviceName) }))
			}
		case *ast.JobDecl:
			lenses = append(lenses, codeLensFor(files, path, d.Name, d.NamePos, jobRefsIn))
		case *ast.ScheduleDecl:
			lenses = append(lenses, codeLensFor(files, path, d.Name, d.NamePos, scheduleRefsIn))
		case *ast.SagaDecl:
			lenses = append(lenses, codeLensFor(files, path, d.Name, d.NamePos, sagaRefsIn))
		case *ast.EnumDecl:
			lenses = append(lenses, codeLensFor(files, path, d.Name, d.NamePos, enumRefsIn))
		}
	}

	return lenses
}

// codeLensFor builds the reference-count lens for one declaration: it counts
// every refsFn span naming name across the workspace, anchors the lens on
// the declaration-name range, and carries the count as the title with no
// command identifier behind it -- pure info, nothing to execute.
func codeLensFor(
	files map[string]*ast.File,
	declPath string,
	name string,
	pos diag.Position,
	refsFn func(*ast.File) []symbolRef,
) protocol.CodeLens {
	return protocol.CodeLens{
		Range:   identRange(pos, name),
		Command: protocol.Command{Title: strconv.Itoa(countLensRefs(files, declPath, name, pos, refsFn)) + " reference(s)"},
	}
}

// countLensRefs counts every refsFn span naming name across every file in
// files, mirroring referencesAt's filters: spans with malformed positions
// are skipped, and the declaration span itself (same path and position) is
// not a reference to itself, so an unreferenced declaration reads "0
// reference(s)".
func countLensRefs(
	files map[string]*ast.File,
	declPath string,
	name string,
	pos diag.Position,
	refsFn func(*ast.File) []symbolRef,
) int {
	count := 0

	for path, file := range files {
		if file == nil {
			continue
		}

		for _, ref := range refsFn(file) {
			if ref.Name != name || ref.Pos.Line <= 0 {
				continue
			}

			if path == declPath && ref.Pos == pos {
				continue
			}

			count++
		}
	}

	return count
}

// scheduleRefsIn returns every schedule's own declaration in one parsed
// file. Nothing in the resolver looks a schedule up by name (a schedule
// dispatches a job; nothing dispatches a schedule), so this is
// declaration-only, mirroring serviceRefsIn's and sagaRefsIn's rationale --
// schedule lenses therefore read "0 reference(s)" until something names a
// schedule.
func scheduleRefsIn(file *ast.File) []symbolRef {
	var refs []symbolRef

	for _, decl := range flattenDecls(file) {
		sched, ok := decl.(*ast.ScheduleDecl)
		if !ok || sched.Name == "" {
			continue
		}

		refs = append(refs, symbolRef{Name: sched.Name, Pos: sched.NamePos})
	}

	return refs
}
