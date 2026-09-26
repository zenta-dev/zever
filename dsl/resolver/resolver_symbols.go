package resolver

import (
	"github.com/zenta-dev/zever/internal/dsl/ast"
	"github.com/zenta-dev/zever/internal/dsl/diag"
)

// symbolTable is Pass 0's output: for each of the five leaf declaration
// kinds, an ordered slice of the accepted (non-duplicate) declarations in
// source-declaration order. Later passes range over these ordered slices to
// stay deterministic; per-kind name -> decl maps were deliberately not kept
// here (later passes each build their own IR-backed lookup, e.g.
// buildEntityIndex/buildJobIndex, off the resolved IR instead).
type symbolTable struct {
	entityOrder   []*ast.EntityDecl
	messageOrder  []*ast.MessageDecl
	serviceOrder  []*ast.ServiceDecl
	jobOrder      []*ast.JobDecl
	scheduleOrder []*ast.ScheduleDecl
	enumOrder     []*ast.EnumDecl
}

// resolveSymbols is Pass 0: it walks leafDecls (Pass -1's accepted
// declarations, in source-declaration order) and builds one flat namespace
// spanning all four declaration kinds together — an Entity and a Job
// sharing a name collide, because they'd collide as generated identifiers
// downstream. Names are global across modules too, by design (see the
// package-level docs of Pass 0 in the task brief). First occurrence wins;
// later occurrences are reported via ErrDuplicateDecl (naming both
// positions) and dropped from resolution.
func resolveSymbols(leafDecls []ast.Decl) (*symbolTable, diag.List) {
	var diags diag.List

	syms := &symbolTable{}

	seen := map[string]diag.Position{}

	for _, d := range leafDecls {
		name, pos, ok := declNameAndPos(d)
		if !ok {
			continue
		}

		if prevPos, dup := seen[name]; dup {
			diags = append(diags, diag.Wrap("resolve", pos, ErrDuplicateDecl,
				"duplicate top-level declaration %q (first declared at %s)", name, prevPos.String()))

			continue
		}

		seen[name] = pos

		switch v := d.(type) {
		case *ast.EntityDecl:
			syms.entityOrder = append(syms.entityOrder, v)
		case *ast.MessageDecl:
			syms.messageOrder = append(syms.messageOrder, v)
		case *ast.ServiceDecl:
			syms.serviceOrder = append(syms.serviceOrder, v)
		case *ast.JobDecl:
			syms.jobOrder = append(syms.jobOrder, v)
		case *ast.ScheduleDecl:
			syms.scheduleOrder = append(syms.scheduleOrder, v)
		case *ast.EnumDecl:
			syms.enumOrder = append(syms.enumOrder, v)
		}
	}

	return syms, diags
}

// declNameAndPos extracts the declared name and its name position from one
// of the five leaf declaration kinds. ok is false for anything else (not
// expected among Pass -1's leafDecls, but handled defensively rather than
// assumed).
func declNameAndPos(d ast.Decl) (string, diag.Position, bool) {
	switch v := d.(type) {
	case *ast.EntityDecl:
		return v.Name, v.NamePos, true
	case *ast.MessageDecl:
		return v.Name, v.NamePos, true
	case *ast.ServiceDecl:
		return v.Name, v.NamePos, true
	case *ast.JobDecl:
		return v.Name, v.NamePos, true
	case *ast.ScheduleDecl:
		return v.Name, v.NamePos, true
	case *ast.EnumDecl:
		return v.Name, v.NamePos, true
	default:
		return "", diag.Position{}, false
	}
}
