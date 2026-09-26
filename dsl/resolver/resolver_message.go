package resolver

import (
	"github.com/zenta-dev/zever/dsl/ast"
	"github.com/zenta-dev/zever/dsl/diag"
	"github.com/zenta-dev/zever/dsl/ir"
)

// buildMessageIndex flattens every module's already-declared messages into a
// single name -> *ir.Message map. Called after declareMessages, so every
// message already has stable pointer identity (Fields is still nil at that
// point -- resolveMessageFields fills it in afterwards using this same
// index), letting a message field reference another message regardless of
// source declaration order.
func buildMessageIndex(modules []*ir.Module) map[string]*ir.Message {
	idx := make(map[string]*ir.Message)

	for _, m := range modules {
		for _, msg := range m.Messages {
			idx[msg.Name] = msg
		}
	}

	return idx
}

// declareMessages is the message declaration pass: it creates an empty
// *ir.Message{Name, Module, Pos} shell (Fields left nil) for every message
// decl (in Pass 0's source-declaration order) and appends it to its owning
// Module's Messages slice. Splitting declaration from field resolution
// (resolveMessageFields) gives every message stable pointer identity before
// any field is resolved against entityByName/messageByName, so a message
// field can reference a message declared later in source order, or in a
// mutual pair.
func declareMessages(decls []*ast.MessageDecl, declModule map[ast.Decl]*ir.Module) {
	for _, decl := range decls {
		module := declModule[decl]
		if module == nil {
			continue
		}

		msg := &ir.Message{Name: decl.Name, Module: module, Pos: decl.Pos, DocComment: decl.DocComment}
		module.Messages = append(module.Messages, msg)
	}
}

// resolveMessageFields is the message field-resolution pass: for every
// message decl (same order as declareMessages), it resolves the decl's
// fields and fills them into the already-declared *ir.Message shell found
// via messageByName. Runs after entityByName/messageByName both exist, so a
// field can resolve as a Ref to either.
func resolveMessageFields(
	decls []*ast.MessageDecl, declModule map[ast.Decl]*ir.Module, entityByName map[string]*ir.Entity, messageByName map[string]*ir.Message,
	enumByName map[string]*ir.Enum,
) diag.List {
	var diags diag.List

	for _, decl := range decls {
		module := declModule[decl]
		if module == nil {
			continue
		}

		msg := messageByName[decl.Name]
		if msg == nil {
			// Unreachable given declareMessages' invariants; guarded defensively.
			continue
		}

		fields := make([]*ir.Field, 0, len(decl.Fields))

		for _, fd := range decl.Fields {
			field, d := resolveFieldForMessage(fd, entityByName, messageByName, enumByName)
			diags = append(diags, d...)

			if field == nil {
				continue
			}

			fields = append(fields, field)
		}

		msg.Fields = fields
	}

	return diags
}

// resolveFieldForMessage resolves one message field's type and attributes.
// A bare identifier type with no type-arguments that names a known
// entity/message resolves as an ir.Field.Ref (the same TypeRef union
// ir.Param.Ref and Operation.Returns already use) -- checked entity-then-
// message, the same lookup order resolveParam already uses for RPC params
// -- before falling back to resolveFieldType for an actual scalar. Message
// fields are otherwise scalar only (no relations).
func resolveFieldForMessage(
	fd *ast.FieldDecl, entityByName map[string]*ir.Entity, messageByName map[string]*ir.Message, enumByName map[string]*ir.Enum,
) (*ir.Field, diag.List) {
	if fd.Type != nil && len(fd.Type.Args) == 0 {
		if ent, ok := entityByName[fd.Type.Name]; ok {
			return resolveRefFieldForMessage(fd, &ir.TypeRef{Entity: ent})
		}

		if msg, ok := messageByName[fd.Type.Name]; ok {
			return resolveRefFieldForMessage(fd, &ir.TypeRef{Message: msg})
		}
	}

	ft, ftDiag := resolveFieldType(fd.Type, enumByName)
	if ftDiag != nil {
		return nil, diag.List{ftDiag}
	}

	var diags diag.List

	field := &ir.Field{Name: fd.Name, Type: ft, Optional: fd.Optional, Pos: fd.Pos, DocComment: fd.DocComment}

	for _, attr := range fd.Attributes {
		switch attr.Name {
		case "validate":
			vals, d := resolveValidateRules(attr, field.Type.Scalar)
			field.Validate = append(field.Validate, vals...)
			diags = append(diags, d...)
		case "default":
			diags = append(diags, resolveDefaultAttribute(attr, field)...)
		default:
			diags = append(diags, diag.Wrap("resolve", attr.Pos, ErrInvalidAttribute, "unknown message field attribute @%s", attr.Name))
		}
	}

	return field, diags
}

// resolveRefFieldForMessage finishes resolving a message field already
// known to reference an entity/message (ref). @validate/@default on a
// ref-typed field is rejected: per the same "validate-everything-in"
// opinion that governs ref-typed RPC params (resolveRefParam), validation
// lives on the referenced type's own fields, not on the field that merely
// names the type.
func resolveRefFieldForMessage(fd *ast.FieldDecl, ref *ir.TypeRef) (*ir.Field, diag.List) {
	var diags diag.List

	field := &ir.Field{Name: fd.Name, Optional: fd.Optional, Ref: ref, Pos: fd.Pos}

	for _, attr := range fd.Attributes {
		switch attr.Name {
		case "validate":
			diags = append(diags, diag.Wrap("resolve", attr.Pos, ErrInvalidAttribute,
				"cannot declare @validate on message field %q of entity/message type %q; add @validate to that type's own fields instead",
				fd.Name, ref.Name()))
		default:
			diags = append(diags, diag.Wrap("resolve", attr.Pos, ErrInvalidAttribute,
				"unknown message field attribute @%s", attr.Name))
		}
	}

	return field, diags
}
