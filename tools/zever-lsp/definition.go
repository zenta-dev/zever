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

// findEnum looks a named enum up by name across every module. Enums are
// globally visible (resolver_enum.go's index is flattened the same way), but
// the scan is per-module like findEntity's because the schema is stored
// per-module.
func findEnum(schema *ir.Schema, name string) *ir.Enum {
	if schema == nil || name == "" {
		return nil
	}

	for _, module := range schema.Modules {
		for _, enum := range module.Enums {
			if enum.Name == name {
				return enum
			}
		}
	}

	return nil
}

// findJob looks a resolved job up by name across every module, same linear
// scan as findEntity. A schedule's dispatch option names jobs, so
// go-to-definition on a dispatch target consults this.
func findJob(schema *ir.Schema, name string) *ir.Job {
	if schema == nil || name == "" {
		return nil
	}

	for _, module := range schema.Modules {
		for _, job := range module.Jobs {
			if job.Name == name {
				return job
			}
		}
	}

	return nil
}

// findTypeDeclPos resolves a bare type name to its declaration's position,
// checking entities before messages before named enums (same precedence the
// resolver itself uses when a bare identifier could name either --
// resolver_service.go's resolveParam, resolver_message.go's
// resolveFieldForMessage, with enums last because a named enum reference
// only falls through to the enum index when no entity/message answers
// first). Returns ("", false) for a scalar type name, which has no
// declaration to jump to.
func findTypeDeclPos(schema *ir.Schema, name string) (diag.Position, bool) {
	if entity := findEntity(schema, name); entity != nil {
		return entity.Pos, true
	}

	if msg := findMessage(schema, name); msg != nil {
		return msg.Pos, true
	}

	if enum := findEnum(schema, name); enum != nil {
		return enum.Pos, true
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
// target is any bare identifier that names an entity, message or named
// enum: a relation's target, an rpc's Returns type, an rpc/job param's
// type, or a message/entity field's type. A scalar field type has no declaration to jump
// to. One exception has no type-name slot at all: a schedule's dispatch
// call, whose target names a job, is resolved directly against the job
// declarations. The resolved schema is workspace-wide (Server.schemaAndFile
// recompiles across every file the workspace knows about), so a target
// declared in a different file resolves exactly the same way as one in the
// current file.
//
// The result is a DefinitionResult union: a *Location singleton when a
// target resolves, literal nil when the cursor names nothing jumpable.
func definitionAt(schema *ir.Schema, file *ast.File, cursor protocol.Position) protocol.DefinitionResult {
	if sched := dispatchTargetAt(file, cursor); sched != nil {
		if call, ok := sched.Dispatch.(*ast.CallValue); ok {
			if job := findJob(schema, call.Name); job != nil {
				return &protocol.Location{
					URI:   pathToURI(job.Pos.File),
					Range: pointRange(job.Pos),
				}
			}
		}
	}

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
		// anchored to the type only -- a field name has no declaration to
		// jump to, and fieldAt matches either position.
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
