package main

import (
	"fmt"
	"strings"
	"testing"

	"go.lsp.dev/protocol"

	"github.com/zenta-dev/zever/dsl/ast"
	"github.com/zenta-dev/zever/dsl/compile"
	"github.com/zenta-dev/zever/dsl/parser"
)

const hoverSrc = "entity User {\n" +
	"\tid: uuid @primary\n" +
	"\temail: string @unique @validate(format: \"email\")\n" +
	"\tcreated_at: timestamp @default(now())\n" +
	"}\n" +
	"\n" +
	"entity Task {\n" +
	"\tid: uuid @primary\n" +
	"\tuser_id: uuid\n" +
	"\tbelongs_to user: User @foreign_key(user_id)\n" +
	"}\n"

// hoverText renders hover at a cursor and returns its Markdown body.
func hoverText(t *testing.T, cursor protocol.Position) string {
	t.Helper()

	result, diags := compile.Compile(map[string]string{"main.zen": hoverSrc})
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	file, _ := parser.New("main.zen", []byte(hoverSrc)).ParseFile()

	h := hoverAt(result.Schema, file, cursor)
	if h == nil {
		return ""
	}

	content, ok := h.Contents.(*protocol.MarkupContent)
	if !ok {
		t.Fatalf("hover contents = %T, want *MarkupContent", h.Contents)
	}

	if content.Kind != protocol.MarkupKindMarkdown {
		t.Errorf("hover kind = %q, want markdown", content.Kind)
	}

	return content.Value
}

func TestHoverOnFieldWithAttributes(t *testing.T) {
	// Line 2 (0-based) is `email: string @unique @validate(format: "email")`.
	got := hoverText(t, cursorOn(t, hoverSrc, 2, "email"))

	wantSubstrings := []string{
		"email: string",
		"entity `User`",
		"`@unique`",
		`@validate(format: "email")`,
	}

	for _, want := range wantSubstrings {
		if !strings.Contains(got, want) {
			t.Errorf("hover markdown missing %q:\n%s", want, got)
		}
	}
}

func TestHoverOnFieldWithCallDefault(t *testing.T) {
	got := hoverText(t, cursorOn(t, hoverSrc, 3, "created_at"))

	for _, want := range []string{"created_at: timestamp", "`@default(now())`"} {
		if !strings.Contains(got, want) {
			t.Errorf("hover markdown missing %q:\n%s", want, got)
		}
	}
}

func TestHoverOnFieldWithoutAttributes(t *testing.T) {
	// Line 8 is `user_id: uuid` in entity Task -- no attributes at all.
	got := hoverText(t, cursorOn(t, hoverSrc, 8, "user_id"))

	if !strings.Contains(got, "user_id: uuid") {
		t.Errorf("hover markdown missing the signature:\n%s", got)
	}

	if strings.Contains(got, "- `@") {
		t.Errorf("hover markdown should list no attributes:\n%s", got)
	}
}

func TestHoverOnRelationTarget(t *testing.T) {
	line := 9
	col := strings.Index(strings.Split(hoverSrc, "\n")[line], ": User") + len(": ")
	cursor := protocol.Position{Line: uint32(line), Character: uint32(col)} //nolint:gosec // test fixture

	got := hoverText(t, cursor)

	wantSubstrings := []string{
		"belongs_to user: User",
		"**entity User**",
		"`id: uuid`",
		"`email: string`",
		"`created_at: timestamp`",
	}

	for _, want := range wantSubstrings {
		if !strings.Contains(got, want) {
			t.Errorf("hover markdown missing %q:\n%s", want, got)
		}
	}
}

func TestHoverOnEntityName(t *testing.T) {
	got := hoverText(t, cursorOn(t, hoverSrc, 6, "Task"))

	for _, want := range []string{"entity Task", "2 field(s), 1 relation(s)."} {
		if !strings.Contains(got, want) {
			t.Errorf("hover markdown missing %q:\n%s", want, got)
		}
	}
}

func TestHoverOnNothingReturnsNil(t *testing.T) {
	result, _ := compile.Compile(map[string]string{"main.zen": hoverSrc})
	file, _ := parser.New("main.zen", []byte(hoverSrc)).ParseFile()

	if h := hoverAt(result.Schema, file, protocol.Position{Line: 5, Character: 0}); h != nil {
		t.Errorf("hover on a blank line = %+v, want nil", h)
	}
}

// TestHoverRelationTargetCapsFieldList proves a very wide entity does not
// produce an unbounded hover card.
func TestHoverRelationTargetCapsFieldList(t *testing.T) {
	var wide strings.Builder

	wide.WriteString("entity Wide {\n\tid: uuid @primary\n")

	// hoverFieldLimit + 5 fields in total, so 5 must be elided.
	for i := range hoverFieldLimit + 4 {
		fmt.Fprintf(&wide, "\tf%d: string\n", i)
	}

	wide.WriteString("}\n\nentity Ref {\n\tid: uuid @primary\n\twide_id: uuid\n" +
		"\tbelongs_to wide: Wide @foreign_key(wide_id)\n}\n")

	src := wide.String()

	result, diags := compile.Compile(map[string]string{"main.zen": src})
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	file, _ := parser.New("main.zen", []byte(src)).ParseFile()

	lines := strings.Split(src, "\n")

	var cursor protocol.Position

	for i, line := range lines {
		if idx := strings.Index(line, ": Wide"); idx >= 0 {
			cursor = protocol.Position{
				Line:      uint32(i),               // test fixture
				Character: uint32(idx + len(": ")), //nolint:gosec // test fixture
			}

			break
		}
	}

	h := hoverAt(result.Schema, file, cursor)
	if h == nil {
		t.Fatal("expected hover on the relation target")
	}

	content, _ := h.Contents.(*protocol.MarkupContent)

	bullets := strings.Count(content.Value, "\n- `")
	if bullets != hoverFieldLimit {
		t.Errorf("rendered %d field bullets, want %d:\n%s", bullets, hoverFieldLimit, content.Value)
	}

	if !strings.Contains(content.Value, "_… 5 more_") {
		t.Errorf("missing the elision marker for the remaining fields:\n%s", content.Value)
	}
}

// docCommentSrc mirrors hoverSrc's shape but with "//" doc comments written
// immediately above the entity declaration and one of its fields, so hover
// tests can assert the comment text is surfaced.
const docCommentSrc = "// The user of the system.\n" +
	"entity User {\n" +
	"\t// Primary key.\n" +
	"\tid: uuid @primary\n" +
	"\temail: string @unique\n" +
	"}\n" +
	"\n" +
	"entity Task {\n" +
	"\tid: uuid @primary\n" +
	"\tuser_id: uuid\n" +
	"\tbelongs_to user: User @foreign_key(user_id)\n" +
	"}\n"

func hoverTextForSrc(t *testing.T, src string, cursor protocol.Position) string {
	t.Helper()

	result, diags := compile.Compile(map[string]string{"main.zen": src})
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	file, _ := parser.New("main.zen", []byte(src)).ParseFile()

	h := hoverAt(result.Schema, file, cursor)
	if h == nil {
		return ""
	}

	content, ok := h.Contents.(*protocol.MarkupContent)
	if !ok {
		t.Fatalf("hover contents = %T, want *MarkupContent", h.Contents)
	}

	return content.Value
}

func TestHoverOnFieldIncludesDocComment(t *testing.T) {
	// Line 3 (0-based) is `\tid: uuid @primary`, preceded by "// Primary key."
	got := hoverTextForSrc(t, docCommentSrc, cursorOn(t, docCommentSrc, 3, "id"))

	if !strings.Contains(got, "Primary key.") {
		t.Errorf("hover markdown missing the field doc comment:\n%s", got)
	}
}

func TestHoverOnFieldWithoutDocCommentOmitsParagraph(t *testing.T) {
	// email has no doc comment of its own.
	got := hoverTextForSrc(t, docCommentSrc, cursorOn(t, docCommentSrc, 4, "email"))

	if strings.Contains(got, "Primary key.") {
		t.Errorf("hover markdown for email leaked User's or id's doc comment:\n%s", got)
	}
}

func TestHoverOnEntityNameIncludesDocComment(t *testing.T) {
	got := hoverTextForSrc(t, docCommentSrc, cursorOn(t, docCommentSrc, 1, "User"))

	if !strings.Contains(got, "The user of the system.") {
		t.Errorf("hover markdown missing the entity doc comment:\n%s", got)
	}
}

func TestHoverOnRelationTargetIncludesDocComment(t *testing.T) {
	line := 10
	col := strings.Index(strings.Split(docCommentSrc, "\n")[line], ": User") + len(": ")
	cursor := protocol.Position{Line: uint32(line), Character: uint32(col)} //nolint:gosec // test fixture

	got := hoverTextForSrc(t, docCommentSrc, cursor)

	if !strings.Contains(got, "The user of the system.") {
		t.Errorf("hover markdown for the relation target missing the resolved entity's doc comment:\n%s", got)
	}
}

const errorsHoverSrc = "entity Task {\n" +
	"\tid: uuid @primary\n" +
	"}\n" +
	"\n" +
	"service TaskService {\n" +
	"\trpc GetTask(id: uuid) -> Task {\n" +
	"\t\tauth: none\n" +
	"\t\terrors: { not_found }\n" +
	"\t}\n" +
	"}\n"

func TestHoverOnRecognizedErrorCase(t *testing.T) {
	got := hoverTextForSrc(t, errorsHoverSrc, cursorOn(t, errorsHoverSrc, 7, "not_found"))

	for _, want := range []string{"`not_found`", "HTTP 404", "gRPC NOT_FOUND"} {
		if !strings.Contains(got, want) {
			t.Errorf("hover markdown missing %q:\n%s", want, got)
		}
	}
}

// errorsHoverSrcUnrecognized swaps in a bogus error code so hover can be
// checked against the parse-only path (compile.Compile would fail on this
// with an "unknown error code" diagnostic, so this uses hoverTextForSrc's
// parse-only sibling instead of routing through a fatal diagnostics check).
const errorsHoverSrcUnrecognized = "entity Task {\n" +
	"\tid: uuid @primary\n" +
	"}\n" +
	"\n" +
	"service TaskService {\n" +
	"\trpc GetTask(id: uuid) -> Task {\n" +
	"\t\tauth: none\n" +
	"\t\terrors: { bogus_code }\n" +
	"\t}\n" +
	"}\n"

func TestHoverOnUnrecognizedErrorCase(t *testing.T) {
	file, _ := parser.New("main.zen", []byte(errorsHoverSrcUnrecognized)).ParseFile()

	h := hoverAt(nil, file, cursorOn(t, errorsHoverSrcUnrecognized, 7, "bogus_code"))
	if h == nil {
		t.Fatal("expected hover on an unrecognized error case, got nil")
	}

	content, ok := h.Contents.(*protocol.MarkupContent)
	if !ok {
		t.Fatalf("hover contents = %T, want *MarkupContent", h.Contents)
	}

	for _, want := range []string{"`bogus_code`", "not a recognized error code"} {
		if !strings.Contains(content.Value, want) {
			t.Errorf("hover markdown missing %q:\n%s", want, content.Value)
		}
	}
}

func TestMarkdownHover_emptyContent_returnsNil(t *testing.T) {
	rng := protocol.Range{Start: protocol.Position{Line: 1, Character: 2}}

	if got := markdownHover("", rng); got != nil {
		t.Errorf("markdownHover(\"\") = %+v, want nil", got)
	}

	got := markdownHover("hello", rng)
	if got == nil {
		t.Fatal("markdownHover(\"hello\") = nil, want a hover")
	}

	content, ok := got.Contents.(*protocol.MarkupContent)
	if !ok {
		t.Fatalf("hover contents = %T, want *MarkupContent", got.Contents)
	}

	if content.Kind != protocol.MarkupKindMarkdown || content.Value != "hello" {
		t.Errorf("hover contents = %+v, want markdown \"hello\"", content)
	}

	if got.Range == nil || *got.Range != rng {
		t.Errorf("hover range = %v, want %v", got.Range, rng)
	}
}

// TestHoverOnUnresolvedRelationTarget proves hover stays informative when
// the relation target names no known entity: the signature still renders,
// followed by the unresolved marker.
func TestHoverOnUnresolvedRelationTarget(t *testing.T) {
	src := "entity Task {\n" +
		"\tid: uuid @primary\n" +
		"\tuser_id: uuid\n" +
		"\tbelongs_to user: Ghost @foreign_key(user_id)\n" +
		"}\n"

	file, _ := parser.New("main.zen", []byte(src)).ParseFile()

	line := 3
	col := strings.Index(strings.Split(src, "\n")[line], ": Ghost") + len(": ")
	cursor := protocol.Position{Line: uint32(line), Character: uint32(col)} //nolint:gosec // test fixture

	h := hoverAt(nil, file, cursor)
	if h == nil {
		t.Fatal("expected hover on the unresolved relation target, got nil")
	}

	content, _ := h.Contents.(*protocol.MarkupContent)

	for _, want := range []string{"belongs_to user: Ghost", "Unresolved entity `Ghost`"} {
		if !strings.Contains(content.Value, want) {
			t.Errorf("hover markdown missing %q:\n%s", want, content.Value)
		}
	}
}

// TestHoverOnRelationTargetWithoutFields proves the hover card for an
// entity that declares no fields renders the _no fields_ marker instead of
// an empty bullet list.
func TestHoverOnRelationTargetWithoutFields(t *testing.T) {
	// Empty declares no fields, so compilation reports a missing-@primary
	// diagnostic -- but the resolver still records the (fieldless) entity,
	// which is exactly the shape the _no fields_ arm must handle.
	src := "entity Empty {\n" +
		"}\n\n" +
		"entity Ref {\n" +
		"\tid: uuid @primary\n" +
		"\tempty_id: uuid\n" +
		"\tbelongs_to empty: Empty @foreign_key(empty_id)\n" +
		"}\n"

	result, _ := compile.Compile(map[string]string{"main.zen": src})

	file, _ := parser.New("main.zen", []byte(src)).ParseFile()

	lines := strings.Split(src, "\n")

	var cursor protocol.Position

	for i, line := range lines {
		if idx := strings.Index(line, ": Empty"); idx >= 0 {
			cursor = protocol.Position{
				Line:      uint32(i),               // test fixture
				Character: uint32(idx + len(": ")), //nolint:gosec // test fixture
			}

			break
		}
	}

	h := hoverAt(result.Schema, file, cursor)
	if h == nil {
		t.Fatal("expected hover on the relation target, got nil")
	}

	content, _ := h.Contents.(*protocol.MarkupContent)

	if !strings.Contains(content.Value, "_no fields_") {
		t.Errorf("hover markdown missing the _no fields_ marker:\n%s", content.Value)
	}
}

func TestRenderTypeExpr(t *testing.T) {
	tests := []struct {
		name string
		in   *ast.TypeExpr
		want string
	}{
		{"bare", &ast.TypeExpr{Name: "string"}, "string"},
		{"with args", &ast.TypeExpr{Name: "enum", Args: []string{"a", "b"}}, "enum(a, b)"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := renderTypeExpr(tc.in); got != tc.want {
				t.Errorf("renderTypeExpr() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestRenderAttributes_skipsNil_rendersArgs(t *testing.T) {
	got := renderAttributes([]*ast.Attribute{
		nil,
		{Name: "unique"},
		{Name: "validate", Args: []*ast.Arg{{Name: "format", Value: &ast.StringLit{Value: "email"}}}},
	})

	if len(got) != 2 {
		t.Fatalf("renderAttributes() = %v, want 2 lines (nil skipped)", got)
	}

	if got[0] != "- `@unique`" {
		t.Errorf("renderAttributes()[0] = %q, want %q", got[0], "- `@unique`")
	}

	if got[1] != "- `@validate(format: \"email\")`" {
		t.Errorf("renderAttributes()[1] = %q, want the validate bullet", got[1])
	}
}

func TestRenderArgs_skipsNil_rendersPositionalAndNamed(t *testing.T) {
	got := renderArgs([]*ast.Arg{
		nil,
		{Value: &ast.IntLit{Value: 3}},
		{Name: "min_len", Value: &ast.IntLit{Value: 3}},
	})

	if got != "(3, min_len: 3)" {
		t.Errorf("renderArgs() = %q, want %q", got, "(3, min_len: 3)")
	}

	if got := renderArgs(nil); got != "" {
		t.Errorf("renderArgs(nil) = %q, want %q", got, "")
	}
}

func TestRenderValue_allArms(t *testing.T) {
	tests := []struct {
		name string
		in   ast.Value
		want string
	}{
		{"nil", nil, ""},
		{"string", &ast.StringLit{Value: "hi"}, `"hi"`},
		{"int", &ast.IntLit{Value: 42}, "42"},
		{"float", &ast.FloatLit{Value: 1.5}, "1.5"},
		{"duration", &ast.DurationLit{Raw: "24h"}, "24h"},
		{"ident", &ast.IdentValue{Name: "admin"}, "admin"},
		{"set", &ast.SetLit{Items: []string{"a", "b"}}, "{a, b}"},
		{"call without args", &ast.CallValue{Name: "now"}, "now()"},
		{
			"call with args",
			&ast.CallValue{Name: "check", Args: []*ast.Arg{{Value: &ast.StringLit{Value: "x"}}}},
			`check("x")`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := renderValue(tc.in); got != tc.want {
				t.Errorf("renderValue() = %q, want %q", got, tc.want)
			}
		})
	}
}

// bogusValue implements ast.Value via embedding: the unexported valuePos
// seals the interface against any other outside implementation, but the
// promoted method still satisfies it, while the dynamic type matches none of
// renderValue's seven concrete cases.
type bogusValue struct{ ast.Value }

// TestRenderValue_unknownType_returnsPlaceholder pins the default arm: a
// future ast.Value implementation outside the seven known cases renders as
// "?".
func TestRenderValue_unknownType_returnsPlaceholder(t *testing.T) {
	if got := renderValue(bogusValue{}); got != "?" {
		t.Errorf("renderValue(bogusValue) = %q, want %q", got, "?")
	}
}

func TestHoverFieldIncludesTypeLine(t *testing.T) {
	got := hoverText(t, cursorOn(t, hoverSrc, 2, "email"))

	if !strings.Contains(got, "Type: string") {
		t.Errorf("hover markdown missing Type line:\n%s", got)
	}
}

func TestHoverFieldIncludesTypeHintForDefault(t *testing.T) {
	got := hoverText(t, cursorOn(t, hoverSrc, 3, "created_at"))

	if !strings.Contains(got, "Type hint") {
		t.Errorf("hover markdown missing Type hint line:\n%s", got)
	}

	if !strings.Contains(got, "default: now()") {
		t.Errorf("hover markdown missing default hint:\n%s", got)
	}
}

func TestHoverFieldIncludesTypeHintForValidate(t *testing.T) {
	got := hoverText(t, cursorOn(t, hoverSrc, 2, "email"))

	if !strings.Contains(got, "Type hint") {
		t.Errorf("hover markdown missing Type hint line:\n%s", got)
	}
}

// hoverVisionDeclSrc exercises the message/enum/saga hover anchors. None of
// them needs a resolved schema, so these tests hover against a bare parse.
const hoverVisionDeclSrc = `// A named value set for state columns.
enum Status {
	active, dormant
}

// A non-persistent payload.
message CartInput {
	// Where to ship.
	email: string
}

// Two-phase capture with a pivot.
saga Checkout {
	step reserve {
		execute: InventoryService.Reserve
		compensate: InventoryService.Release
	}
	step charge {
		execute: PaymentService.Charge
		pivot: true
	}
}
`

// hoverDeclText renders hover from a parse-only fixture, the way mid-edit
// anchors work: none of the new decl anchors consult the resolved schema.
func hoverDeclText(t *testing.T, src string, cursor protocol.Position) string {
	t.Helper()

	file, _ := parser.New("decl.zen", []byte(src)).ParseFile()
	if file == nil {
		t.Fatal("parser returned no file")
	}

	h := hoverAt(nil, file, cursor)
	if h == nil {
		return ""
	}

	content, ok := h.Contents.(*protocol.MarkupContent)
	if !ok {
		t.Fatalf("hover contents = %T, want *MarkupContent", h.Contents)
	}

	return content.Value
}

func TestHoverOnMessageDecl(t *testing.T) {
	got := hoverDeclText(t, hoverVisionDeclSrc, cursorOn(t, hoverVisionDeclSrc, 6, "CartInput"))

	wantSubstrings := []string{
		"```zen\nmessage CartInput\n```",
		"1 field(s).",
		"A non-persistent payload.",
		"- `email: string`",
	}

	for _, want := range wantSubstrings {
		if !strings.Contains(got, want) {
			t.Errorf("hover markdown missing %q:\n%s", want, got)
		}
	}
}

func TestHoverOnEnumDecl(t *testing.T) {
	got := hoverDeclText(t, hoverVisionDeclSrc, cursorOn(t, hoverVisionDeclSrc, 1, "Status"))

	wantSubstrings := []string{
		"```zen\nenum Status\n```",
		"A named value set for state columns.",
		"- `active`",
		"- `dormant`",
	}

	for _, want := range wantSubstrings {
		if !strings.Contains(got, want) {
			t.Errorf("hover markdown missing %q:\n%s", want, got)
		}
	}
}

func TestHoverOnSagaDecl(t *testing.T) {
	got := hoverDeclText(t, hoverVisionDeclSrc, cursorOn(t, hoverVisionDeclSrc, 12, "Checkout"))

	wantSubstrings := []string{
		"```zen\nsaga Checkout\n```",
		"Two-phase capture with a pivot.",
		"- `reserve`: execute `InventoryService.Reserve`, compensate `InventoryService.Release`",
		"- `charge`: execute `PaymentService.Charge` (pivot)",
	}

	for _, want := range wantSubstrings {
		if !strings.Contains(got, want) {
			t.Errorf("hover markdown missing %q:\n%s", want, got)
		}
	}
}

// TestHoverOnEnumDeclCapsValueList proves the value list uses the same
// hoverFieldLimit convention with the "… N more" tail as the field lists.
func TestHoverOnEnumDeclCapsValueList(t *testing.T) {
	var src strings.Builder

	src.WriteString("enum Wide {\n")
	src.WriteString("\tv0")

	for i := 1; i < 13; i++ {
		fmt.Fprintf(&src, ", v%d", i)
	}

	src.WriteString("\n}\n")

	got := hoverDeclText(t, src.String(), cursorOn(t, src.String(), 0, "Wide"))

	if !strings.Contains(got, "`v9`") {
		t.Errorf("hover should show the first %d values:\n%s", hoverFieldLimit, got)
	}

	if strings.Contains(got, "`v10`") {
		t.Errorf("hover should stop at %d values:\n%s", hoverFieldLimit, got)
	}

	if !strings.Contains(got, "… 3 more") {
		t.Errorf("hover should end with the three-more tail:\n%s", got)
	}
}

func TestHoverOnEmptyMessageDecl(t *testing.T) {
	src := "message Empty {\n}\n"

	got := hoverDeclText(t, src, cursorOn(t, src, 0, "Empty"))

	if !strings.Contains(got, "0 field(s).") || !strings.Contains(got, "_none_") {
		t.Errorf("hover markdown missing the empty field marker:\n%s", got)
	}
}

// hoverRefSrc names User from a relation target and an rpc return type in
// the same file, so the entity hover can list both reference sites.
const hoverRefSrc = "entity User {\n" +
	"\tid: uuid @primary\n" +
	"}\n" +
	"\n" +
	"entity Order {\n" +
	"\tid: uuid @primary\n" +
	"\tuser_id: uuid\n" +
	"\tbelongs_to user: User @foreign_key(user_id)\n" +
	"}\n" +
	"\n" +
	"service UserService {\n" +
	"\trpc GetUser(id: uuid) -> User {\n" +
	"\t\tauth: none\n" +
	"\t}\n" +
	"}\n"

func TestHoverOnEntityDeclDefinedInAndReferencedBy(t *testing.T) {
	got := hoverTextForSrc(t, hoverRefSrc, cursorOn(t, hoverRefSrc, 0, "User"))

	for _, want := range []string{
		"`defined in main.zen`",
		"Referenced by:",
		"`Order.user`",
		"`UserService.GetUser`",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("hover markdown missing %q:\n%s", want, got)
		}
	}
}

// TestHoverOnEntityDeclWithoutReferencesOmitsSection proves a declaration
// nothing else names renders no "Referenced by" section rather than an
// empty one: Order is referenced by nothing in hoverRefSrc.
func TestHoverOnEntityDeclWithoutReferencesOmitsSection(t *testing.T) {
	got := hoverTextForSrc(t, hoverRefSrc, cursorOn(t, hoverRefSrc, 4, "Order"))

	if strings.Contains(got, "Referenced by:") {
		t.Errorf("hover markdown should omit the section when unreferenced:\n%s", got)
	}

	if !strings.Contains(got, "`defined in main.zen`") {
		t.Errorf("hover markdown missing the defined-in line:\n%s", got)
	}
}

// hoverMsgRefSrc names Cart from a message field type, an rpc parameter
// type and an rpc return type, the three message reference kinds.
const hoverMsgRefSrc = "message Cart {\n" +
	"\tid: uuid\n" +
	"}\n" +
	"\n" +
	"message Order {\n" +
	"\tcart: Cart\n" +
	"}\n" +
	"\n" +
	"service Shop {\n" +
	"\trpc Checkout(cart: Cart) -> Cart {\n" +
	"\t\tauth: none\n" +
	"\t}\n" +
	"}\n"

func TestHoverOnMessageDeclReferencedBy(t *testing.T) {
	got := hoverDeclText(t, hoverMsgRefSrc, cursorOn(t, hoverMsgRefSrc, 0, "Cart"))

	for _, want := range []string{
		"`defined in decl.zen`",
		"Referenced by:",
		"`Order.cart`",
		"`Shop.Checkout(cart)`",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("hover markdown missing %q:\n%s", want, got)
		}
	}

	// The rpc's return type is not a messageRefsIn site (that walker
	// covers declarations, field types and parameter types only), so no
	// bare `Shop.Checkout` bullet may appear.
	if strings.Contains(got, "- `Shop.Checkout`") {
		t.Errorf("hover lists a return-type site the walker does not cover:\n%s", got)
	}
}

// TestHoverOnEnumDeclReferencedByCapped proves the referenced-by list uses
// the same hoverFieldLimit convention with the "… N more" tail as the
// field lists: 12 fields of enum type, 10 shown, 2 elided.
func TestHoverOnEnumDeclReferencedByCapped(t *testing.T) {
	var src strings.Builder

	src.WriteString("enum Status {\n\tok\n}\n\nentity Task {\n\tid: uuid @primary\n")

	for i := range 12 {
		fmt.Fprintf(&src, "\ts%d: Status\n", i)
	}

	src.WriteString("}\n")

	got := hoverDeclText(t, src.String(), cursorOn(t, src.String(), 0, "Status"))

	if !strings.Contains(got, "`Task.s9`") {
		t.Errorf("hover should show the first %d references:\n%s", hoverFieldLimit, got)
	}

	if strings.Contains(got, "`Task.s10`") {
		t.Errorf("hover should stop at %d references:\n%s", hoverFieldLimit, got)
	}

	if !strings.Contains(got, "… 2 more") {
		t.Errorf("hover should end with the two-more tail:\n%s", got)
	}
}

func TestDefinedInDisplay(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"relative file", "main.zen", "main.zen"},
		{"nested relative file", "schema/billing/x.zen", "schema/billing/x.zen"},
		{"absolute path outside cwd falls back to base", "/definitely/not/cwd/x.zen", "x.zen"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := definedInDisplay(tc.in); got != tc.want {
				t.Errorf("definedInDisplay(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestDefinedInLine_emptyFile_returnsEmpty(t *testing.T) {
	if got := definedInLine(nil); got != "" {
		t.Errorf("definedInLine(nil) = %q, want empty", got)
	}

	if got := definedInLine(&ast.File{}); got != "" {
		t.Errorf("definedInLine(nameless file) = %q, want empty", got)
	}
}

// paramsHoverSrc exercises hover on rpc and job parameter names: a
// two-param rpc with a validated string param, and a two-param job.
const paramsHoverSrc = `entity User {
	id: uuid @primary
}

service UserService {
	rpc Rename(id: uuid, name: string @validate(min_len: 1, max_len: 64)) -> User {
		http: POST "/users/{id}/rename"
		auth: required
	}
}

job SendDigest(target: string, batch: int64) {
	queue: low
}
`

func TestHoverOnRPCParamName(t *testing.T) {
	// Line 5 is the rpc signature; "name: string" pins the cursor to the
	// parameter rather than the "name" inside "Rename".
	got := hoverTextForSrc(t, paramsHoverSrc, cursorOn(t, paramsHoverSrc, 5, "name: string"))

	for _, want := range []string{
		"```zen\nname: string\n```",
		"Param of rpc `UserService.Rename`.",
		"- `@validate(min_len: 1, max_len: 64)`",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("hover markdown missing %q:\n%s", want, got)
		}
	}
}

func TestHoverOnRPCParamNameWithoutValidate(t *testing.T) {
	got := hoverTextForSrc(t, paramsHoverSrc, cursorOn(t, paramsHoverSrc, 5, "id: uuid"))

	for _, want := range []string{
		"```zen\nid: uuid\n```",
		"Param of rpc `UserService.Rename`.",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("hover markdown missing %q:\n%s", want, got)
		}
	}

	if strings.Contains(got, "@validate") {
		t.Errorf("hover markdown should list no attributes:\n%s", got)
	}
}

func TestHoverOnJobParamName(t *testing.T) {
	got := hoverTextForSrc(t, paramsHoverSrc, cursorOn(t, paramsHoverSrc, 11, "target"))

	for _, want := range []string{
		"```zen\ntarget: string\n```",
		"Param of job `SendDigest`.",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("hover markdown missing %q:\n%s", want, got)
		}
	}
}

// TestHoverOnParamTypeNameReturnsNil pins the anchor to the name slot: a
// parameter's type name is a definition reference slot, not a hovered
// parameter, so hovering it stays silent.
func TestHoverOnParamTypeNameReturnsNil(t *testing.T) {
	result, diags := compile.Compile(map[string]string{"main.zen": paramsHoverSrc})
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	file, _ := parser.New("main.zen", []byte(paramsHoverSrc)).ParseFile()

	// Line 5 `id: uuid`: the cursor sits on the type, not the name.
	cursor := cursorOn(t, paramsHoverSrc, 5, "uuid")

	if h := hoverAt(result.Schema, file, cursor); h != nil {
		t.Errorf("hover on a param type name = %+v, want nil", h)
	}
}

// TestHoverOnParamNamePrefersParamOverNothing proves the appended-last
// anchor fires where no earlier anchor matches: the rpc name itself is not
// otherwise hoverable, while a param name right beside it is.
func TestHoverOnParamNameRange(t *testing.T) {
	file, _ := parser.New("main.zen", []byte(paramsHoverSrc)).ParseFile()

	h := hoverAt(nil, file, cursorOn(t, paramsHoverSrc, 5, "name: string"))
	if h == nil {
		t.Fatal("expected hover on the rpc param name, got nil")
	}

	if h.Range == nil {
		t.Fatal("hover range is nil, want the param name span")
	}

	// "name" is four characters wide starting at the cursor.
	if got := h.Range.End.Character - h.Range.Start.Character; got != uint32(len("name")) {
		t.Errorf("hover range width = %d, want %d", got, len("name"))
	}
}
