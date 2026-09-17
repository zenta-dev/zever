package main

import (
	"go.lsp.dev/protocol"

	"github.com/zenta-dev/zever/internal/dsl/ast"
	"github.com/zenta-dev/zever/internal/dsl/diag"
)

// lspPosition converts a 1-based compiler position to a 0-based LSP position.
func lspPosition(p diag.Position) protocol.Position {
	line := p.Line - 1
	if line < 0 {
		line = 0
	}

	col := p.Col - 1
	if col < 0 {
		col = 0
	}

	return protocol.Position{
		Line:      uint32(line), //nolint:gosec // clamped non-negative above
		Character: uint32(col),  //nolint:gosec // clamped non-negative above
	}
}

// pointRange is a one-character-wide range at a compiler position. The
// compiler records points, not spans, so this is the best available anchor.
func pointRange(p diag.Position) protocol.Range {
	start := lspPosition(p)
	end := start
	end.Character++

	return protocol.Range{Start: start, End: end}
}

// identRange spans an identifier starting at p, assuming it is single-line.
func identRange(p diag.Position, ident string) protocol.Range {
	start := lspPosition(p)
	end := start
	end.Character += uint32(len(ident)) //nolint:gosec // len is non-negative

	return protocol.Range{Start: start, End: end}
}

// coversIdent reports whether an LSP cursor position falls inside the
// identifier ident that begins at the compiler position p. The end is
// inclusive so a cursor placed just past the last character still hits,
// matching how editors report a click at a word's trailing edge.
func coversIdent(p diag.Position, ident string, cursor protocol.Position) bool {
	if p.Line <= 0 || ident == "" {
		return false
	}

	line := int(cursor.Line) + 1
	col := int(cursor.Character) + 1

	return p.Line == line && col >= p.Col && col <= p.Col+len(ident)
}

// flattenDecls returns every top-level declaration in the file. Retained as a
// thin wrapper (rather than inlining file.Decls at call sites) since module
// blocks previously required expansion here; now dir-derived modules mean
// file.Decls is already flat.
func flattenDecls(file *ast.File) []ast.Decl {
	if file == nil {
		return nil
	}

	return file.Decls
}

// relationTargetAt returns the relation declaration whose target type name is
// under the cursor, or nil if the cursor is elsewhere.
func relationTargetAt(file *ast.File, cursor protocol.Position) *ast.RelationDecl {
	for _, decl := range flattenDecls(file) {
		entity, ok := decl.(*ast.EntityDecl)
		if !ok {
			continue
		}

		for _, rel := range entity.Relations {
			if coversIdent(rel.TargetPos, rel.Target, cursor) {
				return rel
			}
		}
	}

	return nil
}

// fieldAt returns the field declaration whose name or type is under the
// cursor, along with the entity that owns it.
func fieldAt(file *ast.File, cursor protocol.Position) (*ast.EntityDecl, *ast.FieldDecl) {
	for _, decl := range flattenDecls(file) {
		entity, ok := decl.(*ast.EntityDecl)
		if !ok {
			continue
		}

		for _, field := range entity.Fields {
			if coversIdent(field.NamePos, field.Name, cursor) {
				return entity, field
			}

			if field.Type != nil && coversIdent(field.Type.NamePos, field.Type.Name, cursor) {
				return entity, field
			}
		}
	}

	return nil, nil
}

// messageFieldAt returns the field declaration whose name or type is under
// the cursor, along with the message that owns it -- the message-decl
// counterpart of fieldAt, since a message's fields (unlike an entity's) can
// themselves reference another message/entity by name (Field.Ref in the
// resolved IR) and that reference needs the same jump-to-definition support
// a relation target already gets.
func messageFieldAt(file *ast.File, cursor protocol.Position) (*ast.MessageDecl, *ast.FieldDecl) {
	for _, decl := range flattenDecls(file) {
		msg, ok := decl.(*ast.MessageDecl)
		if !ok {
			continue
		}

		for _, field := range msg.Fields {
			if coversIdent(field.NamePos, field.Name, cursor) {
				return msg, field
			}

			if field.Type != nil && coversIdent(field.Type.NamePos, field.Type.Name, cursor) {
				return msg, field
			}
		}
	}

	return nil, nil
}

// rpcReturnsAt returns the rpc declaration whose Returns type name is under
// the cursor.
func rpcReturnsAt(file *ast.File, cursor protocol.Position) *ast.RPCDecl {
	for _, decl := range flattenDecls(file) {
		service, ok := decl.(*ast.ServiceDecl)
		if !ok {
			continue
		}

		for _, rpc := range service.RPCs {
			if rpc == nil {
				continue
			}

			if coversIdent(rpc.ReturnsPos, rpc.Returns, cursor) {
				return rpc
			}
		}
	}

	return nil
}

// paramTypeAt returns the param declaration whose type name is under the
// cursor, scanning both rpc and job params (both share ast.ParamDecl).
func paramTypeAt(file *ast.File, cursor protocol.Position) *ast.ParamDecl {
	for _, decl := range flattenDecls(file) {
		switch d := decl.(type) {
		case *ast.ServiceDecl:
			for _, rpc := range d.RPCs {
				if rpc == nil {
					continue
				}

				if p := paramTypeAtCursor(rpc.Params, cursor); p != nil {
					return p
				}
			}
		case *ast.JobDecl:
			if p := paramTypeAtCursor(d.Params, cursor); p != nil {
				return p
			}
		}
	}

	return nil
}

// paramTypeAtCursor returns the first param whose type name covers the cursor.
func paramTypeAtCursor(params []*ast.ParamDecl, cursor protocol.Position) *ast.ParamDecl {
	for _, param := range params {
		if param == nil || param.Type == nil {
			continue
		}

		if coversIdent(param.Type.NamePos, param.Type.Name, cursor) {
			return param
		}
	}

	return nil
}

// entityNameAt returns the entity whose declared name is under the cursor.
func entityNameAt(file *ast.File, cursor protocol.Position) *ast.EntityDecl {
	for _, decl := range flattenDecls(file) {
		entity, ok := decl.(*ast.EntityDecl)
		if !ok {
			continue
		}

		if coversIdent(entity.NamePos, entity.Name, cursor) {
			return entity
		}
	}

	return nil
}

// jobNameAt returns the job whose declared name is under the cursor.
func jobNameAt(file *ast.File, cursor protocol.Position) *ast.JobDecl {
	for _, decl := range flattenDecls(file) {
		job, ok := decl.(*ast.JobDecl)
		if !ok {
			continue
		}

		if coversIdent(job.NamePos, job.Name, cursor) {
			return job
		}
	}

	return nil
}

// serviceNameAt returns the service whose declared name is under the
// cursor.
func serviceNameAt(file *ast.File, cursor protocol.Position) *ast.ServiceDecl {
	for _, decl := range flattenDecls(file) {
		service, ok := decl.(*ast.ServiceDecl)
		if !ok {
			continue
		}

		if coversIdent(service.NamePos, service.Name, cursor) {
			return service
		}
	}

	return nil
}

// rpcNameAt returns the rpc whose declared name is under the cursor, along
// with the service that declares it.
func rpcNameAt(file *ast.File, cursor protocol.Position) (*ast.ServiceDecl, *ast.RPCDecl) {
	for _, decl := range flattenDecls(file) {
		service, ok := decl.(*ast.ServiceDecl)
		if !ok {
			continue
		}

		for _, rpc := range service.RPCs {
			if rpc == nil {
				continue
			}

			if coversIdent(rpc.NamePos, rpc.Name, cursor) {
				return service, rpc
			}
		}
	}

	return nil, nil
}

// dispatchTargetAt returns the schedule whose dispatch call names a job
// under the cursor.
func dispatchTargetAt(file *ast.File, cursor protocol.Position) *ast.ScheduleDecl {
	for _, decl := range flattenDecls(file) {
		sched, ok := decl.(*ast.ScheduleDecl)
		if !ok {
			continue
		}

		call, ok := sched.Dispatch.(*ast.CallValue)
		if !ok {
			continue
		}

		if coversIdent(call.NamePos, call.Name, cursor) {
			return sched
		}
	}

	return nil
}
