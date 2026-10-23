package main

import (
	"go.lsp.dev/protocol"

	"github.com/zenta-dev/zever/dsl/ast"
	"github.com/zenta-dev/zever/dsl/ir"
)

// typeDefinitionAt resolves go-to-type-definition for a cursor in one file. A
// jump target is any bare identifier that names a type: a relation's target,
// an rpc's Returns type, an rpc/job param's type, or a message/entity field's
// type. Every named type's declaration IS its type definition in this DSL --
// there is no separate interface/implementation split -- so the target is
// resolved through findTypeDeclPos (entities before messages before named
// enums, the same precedence the resolver itself uses). A scalar field type
// has no declaration to jump to, so it yields nil.
//
// One deliberate difference from definitionAt: a schedule's dispatch call
// names a job, which is a value rather than a type, so it is not a
// type-definition target and yields nil here while definitionAt jumps to the
// job. The resolved schema is workspace-wide (Server.schemaAndFile
// recompiles across every file the workspace knows about), so a target
// declared in a different file resolves exactly the same way as one in the
// current file.
//
// The result is a DefinitionResult union: a *Location singleton when a type
// resolves, literal nil when the cursor names nothing jumpable.
func typeDefinitionAt(schema *ir.Schema, file *ast.File, cursor protocol.Position) protocol.DefinitionResult {
	name := ""

	if rel := relationTargetAt(file, cursor); rel != nil {
		name = rel.Target
	} else if rpc := rpcReturnsAt(file, cursor); rpc != nil {
		name = rpc.Returns
	} else if param := paramTypeAt(file, cursor); param != nil {
		name = param.Type.Name
	} else if _, field := messageFieldAt(file, cursor); field != nil && field.Type != nil {
		name = field.Type.Name
	} else if _, field := fieldAt(file, cursor); field != nil && field.Type != nil &&
		!coversIdent(field.NamePos, field.Name, cursor) {
		// An entity field's own type name: the enum/scalar reference slot
		// fieldAt's name arm does not cover. The name check keeps the jump
		// anchored to the type only -- a field name has no type definition
		// to jump to, and fieldAt matches either position.
		name = field.Type.Name
	}

	if name == "" {
		return nil
	}

	pos, ok := findTypeDeclPos(schema, name)
	if !ok || pos.File == "" {
		return nil
	}

	// Pos points at the `entity`/`message`/`enum` keyword, so a point range there
	// is the honest anchor: it lands the cursor on the declaration line
	// without claiming a span the compiler never recorded.
	return &protocol.Location{
		URI:   pathToURI(pos.File),
		Range: pointRange(pos),
	}
}
