package main

import (
	"fmt"
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
		return markdownHover(entityDeclHoverMarkdown(entity), identRange(entity.NamePos, entity.Name))
	}

	if name, pos, ok := errorCaseAt(file, cursor); ok {
		return markdownHover(errorCaseHoverMarkdown(name), identRange(pos, name))
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

	sb.WriteString(docCommentParagraph(field.DocComment))

	if lines := renderAttributes(field.Attributes); len(lines) > 0 {
		sb.WriteString("\n\n")
		sb.WriteString(strings.Join(lines, "\n"))
	}

	return sb.String()
}

// entityDeclHoverMarkdown summarises an entity declaration under the cursor.
func entityDeclHoverMarkdown(entity *ast.EntityDecl) string {
	base := fmt.Sprintf("```zen\nentity %s\n```\n\n%d field(s), %d relation(s).",
		entity.Name, len(entity.Fields), len(entity.Relations))

	return base + docCommentParagraph(entity.DocComment)
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
