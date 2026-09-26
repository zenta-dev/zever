package main

import (
	"go.lsp.dev/protocol"

	"github.com/zenta-dev/zever/dsl/ast"
	"github.com/zenta-dev/zever/dsl/diag"
	"github.com/zenta-dev/zever/dsl/ir"
)

// findEntity looks a resolved entity up by name across every module. The
// schema is small enough that a linear scan is cheaper than an index.
func findEntity(schema *ir.Schema, name string) *ir.Entity {
	if schema == nil || name == "" {
		return nil
	}

	for _, module := range schema.Modules {
		for _, entity := range module.Entities {
			if entity.Name == name {
				return entity
			}
		}
	}

	return nil
}

// findMessage looks a resolved message up by name across every module, same
// linear scan as findEntity.
func findMessage(schema *ir.Schema, name string) *ir.Message {
	if schema == nil || name == "" {
		return nil
	}

	for _, module := range schema.Modules {
		for _, msg := range module.Messages {
			if msg.Name == name {
				return msg
			}
		}
	}

	return nil
}

// findTypeDeclPos resolves a bare type name to its declaration's position,
// checking entities before messages (same precedence the resolver itself
// uses when a bare identifier could name either -- resolver_service.go's
// resolveParam, resolver_message.go's resolveFieldForMessage). Returns
// ("", false) for a scalar type name, which has no declaration to jump to.
func findTypeDeclPos(schema *ir.Schema, name string) (diag.Position, bool) {
	if entity := findEntity(schema, name); entity != nil {
		return entity.Pos, true
	}

	if msg := findMessage(schema, name); msg != nil {
		return msg.Pos, true
	}

	return diag.Position{}, false
}

// allEntityNames returns every resolved entity across every module, in
// declaration order. Same linear scan as findEntity, kept beside it so all
// resolved-schema entity access lives in one place.
func allEntityNames(schema *ir.Schema) []*ir.Entity {
	if schema == nil {
		return nil
	}

	var out []*ir.Entity

	for _, module := range schema.Modules {
		out = append(out, module.Entities...)
	}

	return out
}

// definitionAt resolves go-to-definition for a cursor in one file. A jump
// target is any bare identifier that names an entity or message: a
// relation's target, an rpc's Returns type, an rpc/job param's type, or a
// message field's type. A scalar field type has no declaration to jump to.
// The resolved schema is workspace-wide (Server.schemaAndFile recompiles
// across every file the workspace knows about), so a target declared in a
// different file resolves exactly the same way as one in the current file.
//
// The result is a DefinitionResult union: a *Location singleton when a
// target resolves, literal nil when the cursor names nothing jumpable.
func definitionAt(schema *ir.Schema, file *ast.File, cursor protocol.Position) protocol.DefinitionResult {
	name := ""

	if rel := relationTargetAt(file, cursor); rel != nil {
		name = rel.Target
	} else if rpc := rpcReturnsAt(file, cursor); rpc != nil {
		name = rpc.Returns
	} else if param := paramTypeAt(file, cursor); param != nil {
		name = param.Type.Name
	} else if _, field := messageFieldAt(file, cursor); field != nil && field.Type != nil {
		name = field.Type.Name
	}

	if name == "" {
		return nil
	}

	pos, ok := findTypeDeclPos(schema, name)
	if !ok || pos.File == "" {
		return nil
	}

	// Pos points at the `entity`/`message` keyword, so a point range there
	// is the honest anchor: it lands the cursor on the declaration line
	// without claiming a span the compiler never recorded.
	return &protocol.Location{
		URI:   pathToURI(pos.File),
		Range: pointRange(pos),
	}
}
