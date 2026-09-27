package main

import (
	"errors"
	"sort"

	"go.lsp.dev/protocol"
	"go.lsp.dev/uri"

	"github.com/zenta-dev/zever/dsl/ast"
	"github.com/zenta-dev/zever/dsl/diag"
)

// ErrRenameEmptyName rejects a blank new name before any edit is built.
var ErrRenameEmptyName = errors.New("zever-lsp: rename requires a non-empty new name")

// errRenameEmptyName aliases ErrRenameEmptyName for existing same-package callers.
var errRenameEmptyName = ErrRenameEmptyName

// ErrRenameInvalidIdent rejects a new name that is not a legal zen
// identifier before any edit is built.
var ErrRenameInvalidIdent = errors.New("zever-lsp: rename requires a valid identifier for the new name")

// errRenameInvalidIdent aliases ErrRenameInvalidIdent for existing same-package callers.
var errRenameInvalidIdent = ErrRenameInvalidIdent

// isValidZenIdent reports whether name is a legal zen DSL identifier,
// mirroring dsl/lexer's exact isIdentStart/isIdentContinue rule:
// the first character must be '_' or an ASCII letter, and every subsequent
// character must be '_', an ASCII letter, or an ASCII digit.
func isValidZenIdent(name string) bool {
	if name == "" {
		return false
	}

	for i, r := range name {
		if r > 127 {
			return false
		}

		isStart := r == '_' || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')
		if i == 0 {
			if !isStart {
				return false
			}

			continue
		}

		isDigit := r >= '0' && r <= '9'
		if !isStart && !isDigit {
			return false
		}
	}

	return true
}

// symbolKind classifies what a renameTarget refers to.
type symbolKind string

const (
	entityKind  symbolKind = "entity"
	fieldKind   symbolKind = "field"
	jobKind     symbolKind = "job"
	serviceKind symbolKind = "service"
	rpcKind     symbolKind = "rpc"
)

// symbolRef is one textual occurrence of a symbol name in a position that
// means "this names the symbol": a declaration, or a reference to one.
type symbolRef struct {
	Name string
	Pos  diag.Position
}

// renameTarget is what a cursor resolves to for both prepareRename (which
// only needs Name/Pos, to pre-fill the client's rename prompt) and rename
// (which also needs Kind and, for entity- or service-scoped kinds, Owner,
// to pick the right reference-finding walk).
type renameTarget struct {
	Kind symbolKind
	Name string
	Pos  diag.Position
	// Owner is the owning entity name for fieldKind, the owning service
	// name for rpcKind, and empty for every other kind: entity, job and
	// service names are workspace- (or file-) unique on their own, but a
	// field or rpc name is only unique within the entity or service that
	// declares it.
	Owner string
}

// refsAt returns the first ref in refs whose span covers cursor, the shared
// cursor-matcher every per-kind reference walk's single-cursor resolution
// funnels through.
func refsAt(refs []symbolRef, cursor protocol.Position) (symbolRef, bool) {
	for _, ref := range refs {
		if coversIdent(ref.Pos, ref.Name, cursor) {
			return ref, true
		}
	}

	return symbolRef{}, false
}

// permissionResourceRef extracts the entity reference from an rpc's
// permission option, which the resolver requires to have the shape
// check(<string>, resource: <entity>, owner_field: <field>). Anything that
// does not match that shape yields ok == false: the resolver reports the
// malformed option, this walk simply has nothing to rename.
func permissionResourceRef(rpc *ast.RPCDecl) (symbolRef, bool) {
	return permissionCallArgRef(rpc, "resource")
}

// ownerFieldRef extracts the field reference from an rpc's permission
// option's owner_field argument, mirroring permissionResourceRef's exact
// pattern for the sibling resource argument in the same call.
func ownerFieldRef(rpc *ast.RPCDecl) (symbolRef, bool) {
	return permissionCallArgRef(rpc, "owner_field")
}

// permissionCallArgRef extracts the identifier value of the named argument
// from an rpc's permission option call, shared by permissionResourceRef and
// ownerFieldRef since both arguments live in the same check(...) call shape.
func permissionCallArgRef(rpc *ast.RPCDecl, argName string) (symbolRef, bool) {
	if rpc == nil || rpc.Permission == nil {
		return symbolRef{}, false
	}

	call, ok := rpc.Permission.(*ast.CallValue)
	if !ok || call.Name != "check" {
		return symbolRef{}, false
	}

	for _, arg := range call.Args {
		if arg == nil || arg.Name != argName {
			continue
		}

		// Arg.Pos points at the argument's label, so the identifier's own
		// Pos is the only span that covers the referenced name itself.
		ident, ok := arg.Value.(*ast.IdentValue)
		if !ok {
			continue
		}

		return symbolRef{Name: ident.Name, Pos: ident.Pos}, true
	}

	return symbolRef{}, false
}

// serviceEntityRefs returns every entity reference carried by a service
// declaration: each rpc's return type plus its permission resource. These
// are exactly the positions position.go's cursor helpers do not cover
// (relationTargetAt handles relation targets, entityNameAt the entity's own
// declaration), so the single-cursor and find-all paths share this one walk.
func serviceEntityRefs(file *ast.File) []symbolRef {
	var refs []symbolRef

	for _, decl := range flattenDecls(file) {
		service, ok := decl.(*ast.ServiceDecl)
		if !ok {
			continue
		}

		for _, rpc := range service.RPCs {
			if rpc == nil {
				continue
			}

			if rpc.Returns != "" {
				refs = append(refs, symbolRef{Name: rpc.Returns, Pos: rpc.ReturnsPos})
			}

			if ref, ok := permissionResourceRef(rpc); ok {
				refs = append(refs, ref)
			}
		}
	}

	return refs
}

// entityRefsIn returns every entity-naming span in one parsed file, across
// all four reference kinds. There is no reference index anywhere in ir/ast
// (only forward pointers: an Entity does not know who references it), so a
// workspace rename has to re-walk each file's AST like this.
func entityRefsIn(file *ast.File) []symbolRef {
	var refs []symbolRef

	for _, decl := range flattenDecls(file) {
		entity, ok := decl.(*ast.EntityDecl)
		if !ok {
			continue
		}

		if entity.Name != "" {
			refs = append(refs, symbolRef{Name: entity.Name, Pos: entity.NamePos})
		}

		for _, rel := range entity.Relations {
			if rel != nil && rel.Target != "" {
				refs = append(refs, symbolRef{Name: rel.Target, Pos: rel.TargetPos})
			}
		}
	}

	return append(refs, serviceEntityRefs(file)...)
}

// fieldRefsIn returns every field-naming span belonging to the entity named
// entityName in one parsed file: every field's own declaration, every index
// column, and every permission owner_field reference whose sibling resource
// argument names this same entity. Field names are not workspace-unique
// (two entities may each declare a field called "email"), so this is scoped
// per-entity rather than global like entityRefsIn -- the caller still
// filters by the specific old field name afterwards, exactly like
// entityRefsIn's callers filter by old entity name.
func fieldRefsIn(file *ast.File, entityName string) []symbolRef {
	var refs []symbolRef

	for _, decl := range flattenDecls(file) {
		if entity, ok := decl.(*ast.EntityDecl); ok && entity.Name == entityName {
			for _, field := range entity.Fields {
				if field != nil && field.Name != "" {
					refs = append(refs, symbolRef{Name: field.Name, Pos: field.NamePos})
				}
			}

			for _, idx := range entity.Indexes {
				if idx == nil {
					continue
				}

				for i, col := range idx.Columns {
					var pos diag.Position
					if i < len(idx.ColumnPos) {
						pos = idx.ColumnPos[i]
					}

					refs = append(refs, symbolRef{Name: col, Pos: pos})
				}
			}
		}

		service, ok := decl.(*ast.ServiceDecl)
		if !ok {
			continue
		}

		for _, rpc := range service.RPCs {
			if rpc == nil {
				continue
			}

			resource, ok := permissionResourceRef(rpc)
			if !ok || resource.Name != entityName {
				continue
			}

			if ref, ok := ownerFieldRef(rpc); ok {
				refs = append(refs, ref)
			}
		}
	}

	return refs
}

// jobRefsIn returns every job-naming span in one parsed file: every job's
// own declaration plus every schedule dispatch naming it. Like
// entityRefsIn, job names are workspace-unique (Pass 0 of the resolver
// enforces global uniqueness across every declaration kind), so this is not
// scoped to a single job -- the caller filters by old job name afterwards.
func jobRefsIn(file *ast.File) []symbolRef {
	var refs []symbolRef

	for _, decl := range flattenDecls(file) {
		switch d := decl.(type) {
		case *ast.JobDecl:
			if d.Name != "" {
				refs = append(refs, symbolRef{Name: d.Name, Pos: d.NamePos})
			}
		case *ast.ScheduleDecl:
			if call, ok := d.Dispatch.(*ast.CallValue); ok && call.Name != "" {
				refs = append(refs, symbolRef{Name: call.Name, Pos: call.NamePos})
			}
		}
	}

	return refs
}

// serviceRefsIn returns every service's own declaration in one parsed file.
// Grepping the resolver turns up no code path that looks up a service by
// name from anywhere else, so this is declaration-only -- implemented for
// API symmetry with the other *RefsIn functions even though no cross-decl
// walk is needed.
func serviceRefsIn(file *ast.File) []symbolRef {
	var refs []symbolRef

	for _, decl := range flattenDecls(file) {
		service, ok := decl.(*ast.ServiceDecl)
		if !ok {
			continue
		}

		if service.Name != "" {
			refs = append(refs, symbolRef{Name: service.Name, Pos: service.NamePos})
		}
	}

	return refs
}

// rpcRefsIn returns every rpc's own declaration belonging to the service
// named serviceName in one parsed file. Rpc names are only unique within
// their own service (two services may each declare a "Get" rpc), so this is
// scoped per-service, mirroring fieldRefsIn's per-entity scoping.
func rpcRefsIn(file *ast.File, serviceName string) []symbolRef {
	var refs []symbolRef

	for _, decl := range flattenDecls(file) {
		service, ok := decl.(*ast.ServiceDecl)
		if !ok || service.Name != serviceName {
			continue
		}

		for _, rpc := range service.RPCs {
			if rpc != nil && rpc.Name != "" {
				refs = append(refs, symbolRef{Name: rpc.Name, Pos: rpc.NamePos})
			}
		}
	}

	return refs
}

// entityReferenceAt finds an entity-name identifier under the cursor in a
// position position.go's existing helpers do not cover: an rpc's return
// type, or a permission block's resource reference.
func entityReferenceAt(file *ast.File, cursor protocol.Position) (symbolRef, bool) {
	return refsAt(serviceEntityRefs(file), cursor)
}

// indexColumnAt finds an index column identifier under the cursor, along
// with the entity that declares the index.
func indexColumnAt(file *ast.File, cursor protocol.Position) (*ast.EntityDecl, symbolRef, bool) {
	for _, decl := range flattenDecls(file) {
		entity, ok := decl.(*ast.EntityDecl)
		if !ok {
			continue
		}

		for _, idx := range entity.Indexes {
			if idx == nil {
				continue
			}

			for i, col := range idx.Columns {
				var pos diag.Position
				if i < len(idx.ColumnPos) {
					pos = idx.ColumnPos[i]
				}

				if coversIdent(pos, col, cursor) {
					return entity, symbolRef{Name: col, Pos: pos}, true
				}
			}
		}
	}

	return nil, symbolRef{}, false
}

// ownerFieldAt finds an owner_field identifier under the cursor, along with
// the name of the entity its sibling resource argument names (empty if the
// permission call has no resource argument, e.g. mid-edit).
func ownerFieldAt(file *ast.File, cursor protocol.Position) (string, symbolRef, bool) {
	for _, decl := range flattenDecls(file) {
		service, ok := decl.(*ast.ServiceDecl)
		if !ok {
			continue
		}

		for _, rpc := range service.RPCs {
			if rpc == nil {
				continue
			}

			ref, ok := ownerFieldRef(rpc)
			if !ok || !coversIdent(ref.Pos, ref.Name, cursor) {
				continue
			}

			entityName := ""
			if resource, ok := permissionResourceRef(rpc); ok {
				entityName = resource.Name
			}

			return entityName, ref, true
		}
	}

	return "", symbolRef{}, false
}

// renameTargetAt resolves the cursor to the symbol it refers to, trying
// each kind in turn: entity name, relation target, entity reference
// (rpc-return/permission-resource), field name or type, index column,
// owner_field reference, job name, dispatch target, service name, then rpc
// name. First match wins. Anything else (a relation's own field name, a
// type-argument value, an rpc parameter name) is not renameable.
func renameTargetAt(file *ast.File, cursor protocol.Position) (renameTarget, bool) {
	if entity := entityNameAt(file, cursor); entity != nil {
		return renameTarget{Kind: entityKind, Name: entity.Name, Pos: entity.NamePos}, true
	}

	if rel := relationTargetAt(file, cursor); rel != nil {
		return renameTarget{Kind: entityKind, Name: rel.Target, Pos: rel.TargetPos}, true
	}

	if ref, ok := entityReferenceAt(file, cursor); ok {
		return renameTarget{Kind: entityKind, Name: ref.Name, Pos: ref.Pos}, true
	}

	if entity, field := fieldAt(file, cursor); field != nil {
		return renameTarget{Kind: fieldKind, Name: field.Name, Pos: field.NamePos, Owner: entity.Name}, true
	}

	if entity, ref, ok := indexColumnAt(file, cursor); ok {
		return renameTarget{Kind: fieldKind, Name: ref.Name, Pos: ref.Pos, Owner: entity.Name}, true
	}

	if entityName, ref, ok := ownerFieldAt(file, cursor); ok {
		return renameTarget{Kind: fieldKind, Name: ref.Name, Pos: ref.Pos, Owner: entityName}, true
	}

	if job := jobNameAt(file, cursor); job != nil {
		return renameTarget{Kind: jobKind, Name: job.Name, Pos: job.NamePos}, true
	}

	if sched := dispatchTargetAt(file, cursor); sched != nil {
		if call, ok := sched.Dispatch.(*ast.CallValue); ok {
			return renameTarget{Kind: jobKind, Name: call.Name, Pos: call.NamePos}, true
		}
	}

	if service := serviceNameAt(file, cursor); service != nil {
		return renameTarget{Kind: serviceKind, Name: service.Name, Pos: service.NamePos}, true
	}

	if service, rpc := rpcNameAt(file, cursor); rpc != nil {
		return renameTarget{Kind: rpcKind, Name: rpc.Name, Pos: rpc.NamePos, Owner: service.Name}, true
	}

	return renameTarget{}, false
}

// renameSymbolEdits is the shared shape behind every rename*Edits builder:
// walk every already-parsed file in parsedFiles, collect the refsFn matches
// equal to oldName, and build one sorted WorkspaceEdit. refsFn already
// carries whatever entity/service scoping the symbol kind needs (see
// fieldRefsIn/rpcRefsIn), so this helper itself stays kind-agnostic.
// parsedFiles is expected to come from Workspace.ParsedFiles, which parses
// each file at most once per content change rather than once per request;
// each *ast.File may still be nil (the parser recovers from malformed input,
// but a hopelessly broken file can still fail to yield one), so a broken
// sibling file is simply skipped rather than dropping the whole rename.
func renameSymbolEdits(
	parsedFiles map[string]*ast.File,
	oldName, newName string,
	refsFn func(*ast.File) []symbolRef,
) (*protocol.WorkspaceEdit, error) {
	if oldName == "" {
		return nil, errRenameUnsupported
	}

	if newName == "" {
		return nil, ErrRenameEmptyName
	}

	if !isValidZenIdent(newName) {
		return nil, ErrRenameInvalidIdent
	}

	changes := make(map[uri.URI][]protocol.TextEdit)

	for path, file := range parsedFiles {
		if file == nil {
			continue
		}

		var edits []protocol.TextEdit

		for _, ref := range refsFn(file) {
			if ref.Name != oldName || ref.Pos.Line <= 0 {
				continue
			}

			edits = append(edits, protocol.TextEdit{
				Range:   identRange(ref.Pos, oldName),
				NewText: newName,
			})
		}

		if len(edits) == 0 {
			continue
		}

		sortTextEdits(edits)

		changes[pathToURI(path)] = edits
	}

	return &protocol.WorkspaceEdit{Changes: changes}, nil
}

// renameEntityEdits finds every textual reference to the entity named
// oldName across every file in parsedFiles and returns the WorkspaceEdit
// that renames every occurrence to newName. Scope: the entity's own
// declaration, every relation target naming it, every rpc return-type
// reference, and every permission-block resource reference naming it.
func renameEntityEdits(parsedFiles map[string]*ast.File, oldName, newName string) (*protocol.WorkspaceEdit, error) {
	return renameSymbolEdits(parsedFiles, oldName, newName, entityRefsIn)
}

// renameFieldEdits finds every textual reference to the field named oldName
// on the entity named entityName across every file in parsedFiles and
// returns the WorkspaceEdit that renames every occurrence to newName. Scope:
// the field's own declaration, every index column naming it, and every
// permission owner_field reference naming it -- all scoped to entityName,
// so a same-named field on a different entity is left untouched.
func renameFieldEdits(
	parsedFiles map[string]*ast.File, entityName, oldName, newName string,
) (*protocol.WorkspaceEdit, error) {
	return renameSymbolEdits(parsedFiles, oldName, newName, func(file *ast.File) []symbolRef {
		return fieldRefsIn(file, entityName)
	})
}

// renameJobEdits finds every textual reference to the job named oldName
// across every file in parsedFiles and returns the WorkspaceEdit that
// renames every occurrence to newName. Scope: the job's own declaration and
// every schedule dispatch naming it.
func renameJobEdits(parsedFiles map[string]*ast.File, oldName, newName string) (*protocol.WorkspaceEdit, error) {
	return renameSymbolEdits(parsedFiles, oldName, newName, jobRefsIn)
}

// renameServiceEdits finds every textual reference to the service named
// oldName across every file in parsedFiles and returns the WorkspaceEdit
// that renames every occurrence to newName. Scope: the service's own
// declaration only -- nothing else in the resolver looks up a service by
// name.
func renameServiceEdits(parsedFiles map[string]*ast.File, oldName, newName string) (*protocol.WorkspaceEdit, error) {
	return renameSymbolEdits(parsedFiles, oldName, newName, serviceRefsIn)
}

// renameRPCEdits finds every textual reference to the rpc named oldName on
// the service named serviceName across every file in parsedFiles and
// returns the WorkspaceEdit that renames every occurrence to newName. Scope:
// the rpc's own declaration only, scoped to serviceName so a same-named rpc
// on a different service is left untouched.
func renameRPCEdits(
	parsedFiles map[string]*ast.File, serviceName, oldName, newName string,
) (*protocol.WorkspaceEdit, error) {
	return renameSymbolEdits(parsedFiles, oldName, newName, func(file *ast.File) []symbolRef {
		return rpcRefsIn(file, serviceName)
	})
}

// sortTextEdits orders edits by position so the result is deterministic
// regardless of the map iteration order the walk happened to see.
func sortTextEdits(edits []protocol.TextEdit) {
	sort.Slice(edits, func(i, j int) bool {
		a, b := edits[i].Range.Start, edits[j].Range.Start
		if a.Line != b.Line {
			return a.Line < b.Line
		}

		return a.Character < b.Character
	})
}
