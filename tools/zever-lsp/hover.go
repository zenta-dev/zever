package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"go.lsp.dev/protocol"

	"github.com/zenta-dev/zever/dsl/ast"
	"github.com/zenta-dev/zever/dsl/ir"
	"github.com/zenta-dev/zever/dsl/resolver"
)

// hoverFieldLimit caps how many fields a hovered entity lists, so hovering a
// wide table does not produce an unreadable wall of text.
const hoverFieldLimit = 10

// hoverAt produces hover Markdown for a cursor in one file, or nil when the
// cursor is not over something worth describing.
func hoverAt(schema *ir.Schema, file *ast.File, cursor protocol.Position) *protocol.Hover {
	if rel := relationTargetAt(file, cursor); rel != nil {
		return markdownHover(relationHoverMarkdown(schema, rel), identRange(rel.TargetPos, rel.Target))
	}

	if entity, field := fieldAt(file, cursor); field != nil {
		return markdownHover(fieldHoverMarkdown(entity, field), identRange(field.NamePos, field.Name))
	}

	if entity := entityNameAt(file, cursor); entity != nil {
		return markdownHover(entityDeclHoverMarkdown(file, entity), identRange(entity.NamePos, entity.Name))
	}

	if name, pos, ok := errorCaseAt(file, cursor); ok {
		return markdownHover(errorCaseHoverMarkdown(name), identRange(pos, name))
	}

	if msg := messageNameAt(file, cursor); msg != nil {
		return markdownHover(messageDeclHoverMarkdown(file, msg), identRange(msg.NamePos, msg.Name))
	}

	if enum := enumNameAt(file, cursor); enum != nil {
		return markdownHover(enumDeclHoverMarkdown(file, enum), identRange(enum.NamePos, enum.Name))
	}

	if saga := sagaNameAt(file, cursor); saga != nil {
		return markdownHover(sagaDeclHoverMarkdown(saga), identRange(saga.NamePos, saga.Name))
	}

	return nil
}

// errorCaseHoverMarkdown renders what one errors: {...} case name resolves
// to: its HTTP status and gRPC name (resolver.ValidErrorCodes, the same
// fixed vocabulary the resolver itself validates against), or a plain
// warning when the resolver would reject it as unrecognized -- hover stays
// honest about what the compiler will flag as a diagnostic rather than
// going silent on an invalid name.
func errorCaseHoverMarkdown(name string) string {
	code, ok := resolver.ValidErrorCodes[name]
	if !ok {
		return fmt.Sprintf("`%s` — not a recognized error code", name)
	}

	return fmt.Sprintf("`%s` — HTTP %d, gRPC %s", name, code.HTTPStatus(), code.GRPCName())
}

// markdownHover wraps content as Markdown hover text anchored at rng, or nil when content is empty.
func markdownHover(content string, rng protocol.Range) *protocol.Hover {
	if content == "" {
		return nil
	}

	r := rng

	return &protocol.Hover{
		Contents: &protocol.MarkupContent{
			Kind:  protocol.MarkupKindMarkdown,
			Value: content,
		},
		Range: &r,
	}
}

// fieldHoverMarkdown renders a field's signature plus one line per attribute.
func fieldHoverMarkdown(entity *ast.EntityDecl, field *ast.FieldDecl) string {
	typeName := "?"
	if field.Type != nil {
		typeName = renderTypeExpr(field.Type)
	}

	var sb strings.Builder

	fmt.Fprintf(&sb, "```zen\n%s: %s\n```", field.Name, typeName)

	if entity != nil {
		fmt.Fprintf(&sb, "\n\nField of entity `%s`.", entity.Name)
	}

	if field != nil {
		hint := fieldTypeHint(field)
		if hint == "" && field.Type != nil {
			hint = renderTypeExpr(field.Type)
		}
		if hint != "" {
			fmt.Fprintf(&sb, "\n\nType: %s", hint)
		}
	}

	sb.WriteString(docCommentParagraph(field.DocComment))

	if lines := renderAttributes(field.Attributes); len(lines) > 0 {
		sb.WriteString("\n\n")
		sb.WriteString(strings.Join(lines, "\n"))
	}

	if hasDefaultOrValidate(field) {
		fmt.Fprintf(&sb, "\n\nType hint: %s", fieldTypeHint(field))
		if vh := validateHint(field); vh != "" {
			fmt.Fprintf(&sb, " (validate: %s)", vh)
		}
	}

	return sb.String()
}

// hasDefaultOrValidate reports whether field declares a @default or @validate
// attribute -- the only cases where a Type hint line adds information beyond
// the attribute bullets already rendered above.
func hasDefaultOrValidate(field *ast.FieldDecl) bool {
	if field == nil {
		return false
	}

	for _, attr := range field.Attributes {
		if attr == nil {
			continue
		}

		if attr.Name == "default" || attr.Name == "validate" {
			return true
		}
	}

	return false
}

// validateHint renders the arguments of a field's @validate attribute, if any,
// so the Type hint line can surface the constraint alongside the type.
func validateHint(field *ast.FieldDecl) string {
	if field == nil {
		return ""
	}

	for _, attr := range field.Attributes {
		if attr == nil || attr.Name != "validate" {
			continue
		}

		args := renderArgs(attr.Args)
		args = strings.TrimPrefix(args, "(")
		args = strings.TrimSuffix(args, ")")
		return strings.TrimSpace(args)
	}

	return ""
}

// entityDeclHoverMarkdown summarises an entity declaration under the cursor.
func entityDeclHoverMarkdown(file *ast.File, entity *ast.EntityDecl) string {
	base := fmt.Sprintf("```zen\nentity %s\n```\n\n%d field(s), %d relation(s).",
		entity.Name, len(entity.Fields), len(entity.Relations))

	return base + docCommentParagraph(entity.DocComment) +
		definedInLine(file) +
		hoverReferencedBy(file, renameTarget{Kind: entityKind, Name: entity.Name, Pos: entity.NamePos})
}

// messageDeclHoverMarkdown summarises a message declaration under the
// cursor: its field count and doc comment, then one bullet per field in the
// same rendered shape a relation target's hover lists an entity's fields.
func messageDeclHoverMarkdown(file *ast.File, message *ast.MessageDecl) string {
	var sb strings.Builder

	fmt.Fprintf(&sb, "```zen\nmessage %s\n```\n\n%d field(s).",
		message.Name, len(message.Fields))

	sb.WriteString(docCommentParagraph(message.DocComment))

	sb.WriteString(cappedNamedList(
		renderFieldBullets(message.Fields), len(message.Fields)))

	sb.WriteString(definedInLine(file))

	sb.WriteString(hoverReferencedBy(file, renameTarget{Kind: messageKind, Name: message.Name, Pos: message.NamePos}))

	return sb.String()
}

// enumDeclHoverMarkdown summarises a named enum declaration under the
// cursor: its value list capped at hoverFieldLimit, same as a field listing.
func enumDeclHoverMarkdown(file *ast.File, enum *ast.EnumDecl) string {
	var sb strings.Builder

	fmt.Fprintf(&sb, "```zen\nenum %s\n```", enum.Name)

	sb.WriteString(docCommentParagraph(enum.DocComment))

	bullets := make([]string, 0, len(enum.Values))

	for _, val := range enum.Values {
		bullets = append(bullets, "- `"+val+"`")
	}

	sb.WriteString(cappedNamedList(bullets, len(enum.Values)))

	sb.WriteString(definedInLine(file))

	sb.WriteString(hoverReferencedBy(file, renameTarget{Kind: enumKind, Name: enum.Name, Pos: enum.NamePos}))

	return sb.String()
}

// sagaDeclHoverMarkdown summarises a saga declaration under the cursor with
// one line per step: the step name, its forward and optional compensating
// Service.RPC references, and the pivot marker.
func sagaDeclHoverMarkdown(saga *ast.SagaDecl) string {
	var sb strings.Builder

	fmt.Fprintf(&sb, "```zen\nsaga %s\n```", saga.Name)

	sb.WriteString(docCommentParagraph(saga.DocComment))

	bullets := make([]string, 0, len(saga.Steps))

	for _, step := range saga.Steps {
		if step == nil {
			continue
		}

		var execute, compensate string
		if step.Execute != nil {
			execute = step.Execute.Service + "." + step.Execute.RPC
		}

		if step.Compensate != nil {
			compensate = step.Compensate.Service + "." + step.Compensate.RPC
		}

		line := fmt.Sprintf("- `%s`: execute `%s`", step.Name, execute)
		if compensate != "" {
			line += ", compensate `" + compensate + "`"
		}

		if step.Pivot {
			line += " (pivot)"
		}

		bullets = append(bullets, line)
	}

	sb.WriteString(cappedNamedList(bullets, len(saga.Steps)))

	return sb.String()
}

// renderFieldBullets renders one Markdown bullet per field in the shared
// field-list shape (`name: type`). Attributes are deliberately left out:
// the list mirrors the relation-target hover, which shows bare signatures
// only, and a field's own hover covers the attributes one by one.
func renderFieldBullets(fields []*ast.FieldDecl) []string {
	bullets := make([]string, 0, len(fields))

	for _, field := range fields {
		if field == nil {
			continue
		}

		typeName := "?"
		if field.Type != nil {
			typeName = renderTypeExpr(field.Type)
		}

		bullets = append(bullets, fmt.Sprintf("- `%s: %s`", field.Name, typeName))
	}

	return bullets
}

// cappedNamedList renders bullets under hoverFieldLimit and appends the
// "… N more" tail when the source list was longer, the same convention
// relationHoverMarkdown applies to an entity's field list. count is the
// uncapped total, and nil bullets render as the explicit empty-list marker.
func cappedNamedList(bullets []string, count int) string {
	if count == 0 {
		return "\n\n_none_"
	}

	shown := bullets
	if len(shown) > hoverFieldLimit {
		shown = shown[:hoverFieldLimit]
	}

	var sb strings.Builder

	sb.WriteString("\n")

	for _, bullet := range shown {
		sb.WriteString("\n")
		sb.WriteString(bullet)
	}

	if remaining := count - len(shown); remaining > 0 {
		fmt.Fprintf(&sb, "\n- _… %d more_", remaining)
	}

	return sb.String()
}

// definedInLine renders the "`defined in <relpath>`" hover line naming the
// file that declares the hovered symbol, or "" when the file carries no
// name. The path is relative to the process working directory (which, for a
// workspace opened at its root, reads as the workspace-relative path, e.g.
// `defined in schema/billing/x.zen`) with a base-name fallback, so hover
// needs no workspace-root threading.
func definedInLine(file *ast.File) string {
	if file == nil || file.Name == "" {
		return ""
	}

	return fmt.Sprintf("\n\n`defined in %s`", definedInDisplay(file.Name))
}

// definedInDisplay renders name relative to the working directory, falling
// back to the base name when the path is absolute, outside the working
// directory, or otherwise not relatable: an absolute or escaping path would
// otherwise leak machine-specific directories into the hover card.
func definedInDisplay(name string) string {
	if !filepath.IsAbs(name) {
		return filepath.ToSlash(filepath.Clean(name))
	}

	if cwd, err := os.Getwd(); err == nil {
		if rel, err := filepath.Rel(cwd, name); err == nil && rel != "" && rel != "." &&
			rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return filepath.ToSlash(rel)
		}
	}

	return filepath.ToSlash(filepath.Base(name))
}

// hoverReferencedBy builds the "Referenced by" hover section for an
// entity/message/enum declaration: one bullet per same-file reference site
// naming it, capped at hoverFieldLimit with the same "… N more" tail the
// field lists use, or "" when nothing else in the file names it. It reuses
// refsFnForKind (the same walker referencesAt resolves through), so the
// listed sites stay consistent with what rename would touch; the reference
// walk is single-file because hoverAt only sees the current file. Each
// bullet names the enclosing site (e.g. `Order.user_id` for a field type,
// `dispatch: SyncOrders` for a schedule dispatch) via describeRefSite.
func hoverReferencedBy(file *ast.File, target renameTarget) string {
	refsFn := refsFnForKind(target)
	if refsFn == nil || file == nil {
		return ""
	}

	var bullets []string

	for _, ref := range refsFn(file) {
		if ref.Name != target.Name || ref.Pos.Line <= 0 {
			continue
		}

		if ref.Pos == target.Pos {
			continue
		}

		bullets = append(bullets, "- `"+describeRefSite(file, ref)+"`")
	}

	if len(bullets) == 0 {
		return ""
	}

	return "\n\nReferenced by:" + cappedNamedList(bullets, len(bullets))
}

// describeRefSite names the declaration site holding ref in
// Owner.member shape: a field type in entity Order naming User reads
// `Order.user_id`, a schedule dispatch naming job SyncOrders reads
// `dispatch: SyncOrders`. Anything the scans below do not recognize (a
// position from a newer walker arm) falls back to a line number rather
// than an empty bullet.
func describeRefSite(file *ast.File, ref symbolRef) string {
	for _, decl := range flattenDecls(file) {
		switch d := decl.(type) {
		case *ast.EntityDecl:
			if desc, ok := describeEntityRefSite(d, ref); ok {
				return desc
			}
		case *ast.MessageDecl:
			for _, field := range d.Fields {
				if field == nil || field.Type == nil {
					continue
				}

				if ref.Pos == field.Type.NamePos {
					return d.Name + "." + field.Name
				}
			}
		case *ast.ServiceDecl:
			if desc, ok := describeServiceRefSite(d, ref); ok {
				return desc
			}
		case *ast.JobDecl:
			for _, param := range d.Params {
				if param == nil || param.Type == nil {
					continue
				}

				if ref.Pos == param.Type.NamePos {
					return d.Name + "(" + param.Name + ")"
				}
			}
		case *ast.ScheduleDecl:
			if call, ok := d.Dispatch.(*ast.CallValue); ok && ref.Pos == call.NamePos {
				return "dispatch: " + call.Name
			}
		}
	}

	return fmt.Sprintf("line %d", ref.Pos.Line)
}

// describeEntityRefSite names an entity-held reference site: a field's own
// name or type position reads `Order.user_id`, a relation target reads
// `Order.user`, and an index column reads `Order.user_id`.
func describeEntityRefSite(entity *ast.EntityDecl, ref symbolRef) (string, bool) {
	for _, field := range entity.Fields {
		if field == nil {
			continue
		}

		if ref.Pos == field.NamePos || (field.Type != nil && ref.Pos == field.Type.NamePos) {
			return entity.Name + "." + field.Name, true
		}
	}

	for _, rel := range entity.Relations {
		if rel != nil && ref.Pos == rel.TargetPos {
			return entity.Name + "." + rel.FieldName, true
		}
	}

	for _, idx := range entity.Indexes {
		if idx == nil {
			continue
		}

		for i, col := range idx.Columns {
			if i < len(idx.ColumnPos) && ref.Pos == idx.ColumnPos[i] {
				return entity.Name + "." + col, true
			}
		}
	}

	return "", false
}

// describeServiceRefSite names a service-held reference site: an rpc return
// type reads `TaskService.GetTask`, a permission resource reads
// `TaskService.GetTask permission`, and a parameter type reads
// `TaskService.GetTask(id)`.
func describeServiceRefSite(service *ast.ServiceDecl, ref symbolRef) (string, bool) {
	for _, rpc := range service.RPCs {
		if rpc == nil {
			continue
		}

		if ref.Pos == rpc.ReturnsPos {
			return service.Name + "." + rpc.Name, true
		}

		if resource, ok := permissionResourceRef(rpc); ok && ref.Pos == resource.Pos {
			return service.Name + "." + rpc.Name + " permission", true
		}

		for _, param := range rpc.Params {
			if param == nil || param.Type == nil {
				continue
			}

			if ref.Pos == param.Type.NamePos {
				return service.Name + "." + rpc.Name + "(" + param.Name + ")", true
			}
		}
	}

	return "", false
}

// docCommentParagraph renders doc as its own trailing Markdown paragraph
// (blank line, then the comment text verbatim), or "" when there is no doc
// comment. A multi-line DocComment (one joined by "\n" per
// parser.docCommentFor) renders as a single paragraph with hard line breaks,
// via Markdown's two-trailing-spaces convention.
func docCommentParagraph(doc string) string {
	if doc == "" {
		return ""
	}

	return "\n\n" + strings.ReplaceAll(doc, "\n", "  \n")
}

// relationHoverMarkdown renders the target entity of a relation: its name and
// a capped bullet list of its fields.
func relationHoverMarkdown(schema *ir.Schema, rel *ast.RelationDecl) string {
	var sb strings.Builder

	fmt.Fprintf(&sb, "```zen\n%s %s: %s\n```", astRelationKindName(rel.Kind), rel.FieldName, rel.Target)

	target := findEntity(schema, rel.Target)
	if target == nil {
		fmt.Fprintf(&sb, "\n\nUnresolved entity `%s`.", rel.Target)

		return sb.String()
	}

	fmt.Fprintf(&sb, "\n\n**entity %s**", target.Name)
	sb.WriteString(docCommentParagraph(target.DocComment))

	if len(target.Fields) == 0 {
		sb.WriteString("\n\n_no fields_")

		return sb.String()
	}

	sb.WriteString("\n")

	shown := target.Fields
	if len(shown) > hoverFieldLimit {
		shown = shown[:hoverFieldLimit]
	}

	for _, f := range shown {
		fmt.Fprintf(&sb, "\n- `%s: %s`", f.Name, fieldTypeName(f.Type))
	}

	if remaining := len(target.Fields) - len(shown); remaining > 0 {
		fmt.Fprintf(&sb, "\n- _… %d more_", remaining)
	}

	return sb.String()
}

// renderTypeExpr renders a parsed type expression, e.g. `enum(a, b)`.
func renderTypeExpr(t *ast.TypeExpr) string {
	if len(t.Args) == 0 {
		return t.Name
	}

	return t.Name + "(" + strings.Join(t.Args, ", ") + ")"
}

// renderAttributes renders each attribute as a Markdown bullet, reproducing
// its arguments so `@validate(min_len: 3)` stays informative on hover.
func renderAttributes(attrs []*ast.Attribute) []string {
	lines := make([]string, 0, len(attrs))

	for _, attr := range attrs {
		if attr == nil {
			continue
		}

		lines = append(lines, "- `@"+attr.Name+renderArgs(attr.Args)+"`")
	}

	return lines
}

// renderArgs renders call arguments as a parenthesized list, prefixing named arguments with "name: ".
func renderArgs(args []*ast.Arg) string {
	if len(args) == 0 {
		return ""
	}

	parts := make([]string, 0, len(args))

	for _, arg := range args {
		if arg == nil {
			continue
		}

		text := renderValue(arg.Value)
		if arg.Name != "" {
			text = arg.Name + ": " + text
		}

		parts = append(parts, text)
	}

	return "(" + strings.Join(parts, ", ") + ")"
}

// renderValue renders one attribute or call value expression in zen source form.
func renderValue(v ast.Value) string {
	switch val := v.(type) {
	case nil:
		return ""
	case *ast.StringLit:
		return fmt.Sprintf("%q", val.Value)
	case *ast.IntLit:
		return strconv.FormatInt(val.Value, 10)
	case *ast.FloatLit:
		return fmt.Sprintf("%g", val.Value)
	case *ast.DurationLit:
		return val.Raw
	case *ast.IdentValue:
		return val.Name
	case *ast.SetLit:
		return "{" + strings.Join(val.Items, ", ") + "}"
	case *ast.CallValue:
		// Always parenthesise: a zero-arg call like now() must not render as
		// the bare identifier `now`, which means something else in the DSL.
		if len(val.Args) == 0 {
			return val.Name + "()"
		}

		return val.Name + renderArgs(val.Args)
	default:
		return "?"
	}
}
