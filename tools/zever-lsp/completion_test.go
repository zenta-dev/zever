package main

import (
	"encoding/json"
	"strings"
	"testing"

	"go.lsp.dev/protocol"

	"github.com/zenta-dev/zever/internal/dsl/compile"
	"github.com/zenta-dev/zever/internal/dsl/diag"
	"github.com/zenta-dev/zever/internal/dsl/parser"
	"github.com/zenta-dev/zever/internal/dsl/token"
)

// completionLabels extracts the label of every item, for order-insensitive
// membership assertions.
func completionLabels(items []protocol.CompletionItem) []string {
	labels := make([]string, 0, len(items))
	for _, item := range items {
		labels = append(labels, item.Label)
	}

	return labels
}

func containsLabel(labels []string, want string) bool {
	for _, l := range labels {
		if l == want {
			return true
		}
	}

	return false
}

func TestCompletionTopLevelKeywordsOnEmptyFile(t *testing.T) {
	src := ""

	file, _ := parser.New("empty.zen", []byte(src)).ParseFile()

	items := completionAt(nil, file, src, protocol.Position{Line: 0, Character: 0})
	labels := completionLabels(items)

	for _, want := range []string{"entity", "message", "job", "service"} {
		if !containsLabel(labels, want) {
			t.Errorf("top-level completion missing %q, got %v", want, labels)
		}
	}

	if containsLabel(labels, "primary") {
		t.Errorf("top-level completion should not offer field attribute %q", "primary")
	}
}

func TestCompletionFieldTypeAfterColon(t *testing.T) {
	src := "entity User {\n\tid: \n}\n"

	file, _ := parser.New("u.zen", []byte(src)).ParseFile()
	cursor := protocol.Position{Line: 1, Character: 5}

	items := completionAt(nil, file, src, cursor)
	labels := completionLabels(items)

	for _, want := range []string{"uuid", "string", "int64", "bool", "timestamp", "enum"} {
		if !containsLabel(labels, want) {
			t.Errorf("field-type completion missing %q, got %v", want, labels)
		}
	}

	if containsLabel(labels, "has_many") {
		t.Errorf("field-type completion should not offer relation keyword %q", "has_many")
	}
}

func TestCompletionRelationTargetCrossFile(t *testing.T) {
	taskSrc := "entity Task {\n\tuser_id: uuid\n\tbelongs_to user: \n}\n"

	// This schema is deliberately mid-edit (an incomplete `belongs_to`
	// line), so compile.Compile is expected to report diagnostics here;
	// completion must still work off whatever schema/AST it can recover.
	result, _ := compile.Compile(map[string]string{
		"a.zen": defASrc, // "entity User { id: uuid @primary }"
		"c.zen": taskSrc,
	})

	file, _ := parser.New("c.zen", []byte(taskSrc)).ParseFile()
	cursor := protocol.Position{Line: 2, Character: uint32(len(strings.Split(taskSrc, "\n")[2]))} //nolint:gosec // len never negative

	items := completionAt(result.Schema, file, taskSrc, cursor)
	labels := completionLabels(items)

	if !containsLabel(labels, "User") {
		t.Errorf("relation-target completion missing %q, got %v", "User", labels)
	}
}

func TestCompletionMidIdentifierPrefixFilters(t *testing.T) {
	orderSrc := "entity Order {\n\tid: uuid @primary\n}\n"
	taskSrc := "entity Task {\n\tuser_id: uuid\n\tbelongs_to user: Us\n}\n"

	result, diags := compile.Compile(map[string]string{
		"a.zen": defASrc, // entity User
		"o.zen": orderSrc,
	})
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	file, _ := parser.New("t.zen", []byte(taskSrc)).ParseFile()
	cursor := cursorOn(t, taskSrc, 2, "Us")
	cursor.Character += 2 // land after "Us", not before it

	items := completionAt(result.Schema, file, taskSrc, cursor)
	labels := completionLabels(items)

	if !containsLabel(labels, "User") {
		t.Errorf("prefix %q should match %q, got %v", "Us", "User", labels)
	}

	if containsLabel(labels, "Order") {
		t.Errorf("prefix %q should not match %q, got %v", "Us", "Order", labels)
	}
}

func TestCompletionFieldAttributeAfterAt(t *testing.T) {
	src := "entity User {\n\tid: uuid @\n}\n"

	file, _ := parser.New("u.zen", []byte(src)).ParseFile()
	cursor := cursorOn(t, src, 1, "@")
	cursor.Character++

	items := completionAt(nil, file, src, cursor)
	labels := completionLabels(items)

	for _, want := range []string{"primary", "unique", "validate", "default"} {
		if !containsLabel(labels, want) {
			t.Errorf("field-attribute completion missing %q, got %v", want, labels)
		}
	}

	if containsLabel(labels, "foreign_key") {
		t.Errorf("field-attribute completion should not offer relation attribute %q", "foreign_key")
	}
}

func TestCompletionValidateArgName(t *testing.T) {
	src := "entity User {\n\tname: string @validate(\n}\n"

	file, _ := parser.New("u.zen", []byte(src)).ParseFile()
	cursor := protocol.Position{Line: 1, Character: uint32(len(strings.Split(src, "\n")[1]))} //nolint:gosec // len never negative

	items := completionAt(nil, file, src, cursor)
	labels := completionLabels(items)

	for _, want := range []string{"min_len", "max_len", "format"} {
		if !containsLabel(labels, want) {
			t.Errorf("validate-arg completion missing %q, got %v", want, labels)
		}
	}
}

func TestCompletionOnDeleteValue(t *testing.T) {
	src := "entity Task {\n\tuser_id: uuid\n\tbelongs_to user: User @on_delete(\n}\n"

	file, _ := parser.New("t.zen", []byte(src)).ParseFile()
	cursor := protocol.Position{Line: 2, Character: uint32(len(strings.Split(src, "\n")[2]))} //nolint:gosec // len never negative

	items := completionAt(nil, file, src, cursor)
	labels := completionLabels(items)

	for _, want := range []string{"restrict", "cascade", "set_null"} {
		if !containsLabel(labels, want) {
			t.Errorf("on_delete-value completion missing %q, got %v", want, labels)
		}
	}
}

func TestCompletionRelationAttrName(t *testing.T) {
	src := "entity Task {\n\tuser_id: uuid\n\tbelongs_to user: User @\n}\n"

	file, _ := parser.New("t.zen", []byte(src)).ParseFile()
	cursor := cursorOn(t, src, 2, "@")
	cursor.Character++

	items := completionAt(nil, file, src, cursor)
	labels := completionLabels(items)

	for _, want := range []string{"foreign_key", "on_delete"} {
		if !containsLabel(labels, want) {
			t.Errorf("relation-attribute completion missing %q, got %v", want, labels)
		}
	}

	if containsLabel(labels, "primary") {
		t.Errorf("relation-attribute completion should not offer field attribute %q", "primary")
	}
}

func TestCompletionErrorsSetValue(t *testing.T) {
	src := "service S {\n\trpc M() -> T {\n\t\terrors: { \n\t}\n}\n"

	file, _ := parser.New("s.zen", []byte(src)).ParseFile()
	cursor := cursorOn(t, src, 2, "{ ")
	cursor.Character += 2 // land just after "{ "

	items := completionAt(nil, file, src, cursor)
	labels := completionLabels(items)

	for _, want := range []string{"not_found", "permission_denied", "invalid_argument", "internal"} {
		if !containsLabel(labels, want) {
			t.Errorf("errors-set completion missing %q, got %v", want, labels)
		}
	}

	if len(labels) != len(errorCandidates) {
		t.Errorf("errors-set completion returned %d items, want all %d error codes", len(labels), len(errorCandidates))
	}

	item, ok := findCompletionItem(items, "not_found")
	if !ok {
		t.Fatalf("missing %q item entirely", "not_found")
	}

	detail, ok := completionResolveDetail(item.Data)
	if !ok || detail != "404 NOT_FOUND" {
		t.Errorf("not_found detail = %q, ok=%v, want %q", detail, ok, "404 NOT_FOUND")
	}
}

func TestCompletionErrorsSetValuePrefixFilters(t *testing.T) {
	src := "service S {\n\trpc M() -> T {\n\t\terrors: { not_ \n\t}\n}\n"

	file, _ := parser.New("s.zen", []byte(src)).ParseFile()
	cursor := cursorOn(t, src, 2, "not_")
	cursor.Character += 4 // land just after "not_"

	items := completionAt(nil, file, src, cursor)
	labels := completionLabels(items)

	if !containsLabel(labels, "not_found") {
		t.Errorf("prefix %q should match %q, got %v", "not_", "not_found", labels)
	}

	if containsLabel(labels, "internal") {
		t.Errorf("prefix %q should not match %q, got %v", "not_", "internal", labels)
	}
}

// TestCompletionErrorsSetValueMultiLine proves the brace-based approach
// handles a multi-line errors: {...} set, unlike a line-only check --
// mirrors how attrArgContext already handles a multi-line @validate(...).
func TestCompletionErrorsSetValueMultiLine(t *testing.T) {
	src := "service S {\n\trpc M() -> T {\n\t\terrors: {\n\t\t\tnot_found,\n\t\t\t\n\t\t}\n\t}\n}\n"

	file, _ := parser.New("s.zen", []byte(src)).ParseFile()
	cursor := protocol.Position{Line: 4, Character: 3} // end of the blank line inside errors: {}

	items := completionAt(nil, file, src, cursor)
	labels := completionLabels(items)

	if !containsLabel(labels, "internal") {
		t.Errorf("multi-line errors-set completion missing %q, got %v", "internal", labels)
	}
}

func TestHasPrefix(t *testing.T) {
	tests := []struct {
		candidate, prefix string
		want              bool
	}{
		{"User", "", true},
		{"User", "Us", true},
		{"User", "us", false},
		{"Order", "Us", false},
	}

	for _, tc := range tests {
		if got := hasPrefix(tc.candidate, tc.prefix); got != tc.want {
			t.Errorf("hasPrefix(%q, %q) = %v, want %v", tc.candidate, tc.prefix, got, tc.want)
		}
	}
}

// TestCompletionItemsCarryNoEagerDetail proves the initial list no longer
// attaches Detail eagerly -- that is now completionResolve's job -- while
// still carrying the label/kind exactly as before.
func TestCompletionItemsCarryNoEagerDetail(t *testing.T) {
	src := ""

	file, _ := parser.New("empty.zen", []byte(src)).ParseFile()

	items := completionAt(nil, file, src, protocol.Position{Line: 0, Character: 0})

	item, ok := findCompletionItem(items, "entity")
	if !ok {
		t.Fatalf("completion items missing %q, got %v", "entity", completionLabels(items))
	}

	if detail, present := item.Detail.Get(); present {
		t.Errorf("initial completion item Detail = %q, want absent (deferred to resolve)", detail)
	}

	if len(item.Data) == 0 {
		t.Fatalf("initial completion item Data is empty, want the resolve payload")
	}
}

// TestCompletionResolveFillsDetailFromRoundTrippedData is the
// completionItem/resolve contract test: it takes one item from the initial
// list, round-trips it through JSON exactly as a real client would (encode
// on the way out, decode on the way back in via resolve), and confirms
// completionResolve fills in the same Detail text the initial (eager)
// implementation used to carry directly on the item.
func TestCompletionResolveFillsDetailFromRoundTrippedData(t *testing.T) {
	src := ""

	file, _ := parser.New("empty.zen", []byte(src)).ParseFile()

	items := completionAt(nil, file, src, protocol.Position{Line: 0, Character: 0})

	item, ok := findCompletionItem(items, "entity")
	if !ok {
		t.Fatalf("completion items missing %q, got %v", "entity", completionLabels(items))
	}

	wantDetail := "declare an entity"
	for _, cand := range topLevelKeywordCandidates {
		if cand.label == "entity" {
			wantDetail = cand.detail
		}
	}

	raw, err := json.Marshal(item)
	if err != nil {
		t.Fatalf("json.Marshal(item) error = %v", err)
	}

	var roundTripped protocol.CompletionItem
	if unmarshalErr := json.Unmarshal(raw, &roundTripped); unmarshalErr != nil {
		t.Fatalf("json.Unmarshal(item) error = %v", unmarshalErr)
	}

	server := NewServer()

	resolved, err := server.completionResolve(t.Context(), &roundTripped)
	if err != nil {
		t.Fatalf("completionResolve() error = %v, want nil", err)
	}

	gotDetail, present := resolved.Detail.Get()
	if !present {
		t.Fatalf("completionResolve() Detail is absent, want %q", wantDetail)
	}

	if gotDetail != wantDetail {
		t.Errorf("completionResolve() Detail = %q, want %q", gotDetail, wantDetail)
	}

	doc, ok := resolved.Documentation.(protocol.String)
	if !ok {
		t.Fatalf("completionResolve() Documentation = %T, want protocol.String", resolved.Documentation)
	}

	if string(doc) != wantDetail {
		t.Errorf("completionResolve() Documentation = %q, want %q", string(doc), wantDetail)
	}
}

// TestCompletionResolveDetailShapes covers every Data shape
// completionResolveDetail accepts: the raw-JSON LSPAny the server attaches,
// the in-process struct and pointer forms, a JSON-decoded map, and the
// unknown/empty shapes that must report absent rather than crash.
func TestCompletionResolveDetailShapes(t *testing.T) {
	raw, err := json.Marshal(completionResolveData{Detail: "declare an entity"})
	if err != nil {
		t.Fatalf("json.Marshal error = %v", err)
	}

	tests := []struct {
		name   string
		data   any
		want   string
		wantOK bool
	}{
		{"lspAny payload", protocol.LSPAny(raw), "declare an entity", true},
		{"in-process value", completionResolveData{Detail: "declare an entity"}, "declare an entity", true},
		{"in-process pointer", &completionResolveData{Detail: "declare an entity"}, "declare an entity", true},
		{"nil pointer", (*completionResolveData)(nil), "", false},
		{"decoded map", map[string]any{"detail": "declare an entity"}, "declare an entity", true},
		{"decoded map missing key", map[string]any{"other": "x"}, "", false},
		{"lspAny corrupt bytes", protocol.LSPAny([]byte("{oops")), "", false},
		{"lspAny wrong detail type", protocol.LSPAny([]byte(`{"detail": 42}`)), "", false},
		{"unknown shape", 42, "", false},
		{"nil", nil, "", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := completionResolveDetail(tc.data)
			if got != tc.want || ok != tc.wantOK {
				t.Errorf("completionResolveDetail() = (%q, %v), want (%q, %v)", got, ok, tc.want, tc.wantOK)
			}
		})
	}
}

// TestCompletionResolvePassthroughWithoutData proves an item with no detail
// payload comes back unchanged rather than as an error, since resolve is
// best-effort by design.
func TestCompletionResolvePassthroughWithoutData(t *testing.T) {
	server := NewServer()

	item := &protocol.CompletionItem{Label: "entity", Kind: protocol.CompletionItemKindKeyword}

	resolved, err := server.completionResolve(t.Context(), item)
	if err != nil {
		t.Fatalf("completionResolve() error = %v, want nil", err)
	}

	if resolved != item {
		t.Errorf("completionResolve() returned a different item, want the input unchanged")
	}

	if _, present := resolved.Detail.Get(); present {
		t.Errorf("completionResolve() Detail is present, want absent for an item with no payload")
	}
}

// findCompletionItem returns the first item with the given label.
func findCompletionItem(items []protocol.CompletionItem, label string) (protocol.CompletionItem, bool) {
	for _, item := range items {
		if item.Label == label {
			return item, true
		}
	}

	return protocol.CompletionItem{}, false
}

func TestAllEntityNames(t *testing.T) {
	schema, _ := compileDefFixture(t)

	entities := allEntityNames(schema)
	if len(entities) != 2 {
		t.Fatalf("allEntityNames() = %d entities, want 2: %+v", len(entities), entities)
	}

	if got := allEntityNames(nil); got != nil {
		t.Errorf("allEntityNames(nil) = %v, want nil", got)
	}
}

func TestCompletionEntityMemberOnEmptyLine(t *testing.T) {
	src := "entity User {\n\tid: uuid @primary\n\n}\n"

	file, _ := parser.New("u.zen", []byte(src)).ParseFile()
	cursor := protocol.Position{Line: 2, Character: 0}

	items := completionAt(nil, file, src, cursor)
	labels := completionLabels(items)

	for _, want := range []string{"has_many", "belongs_to", "index"} {
		if !containsLabel(labels, want) {
			t.Errorf("entity-member completion missing %q, got %v", want, labels)
		}
	}

	if containsLabel(labels, "uuid") {
		t.Errorf("entity-member completion should not offer scalar type %q", "uuid")
	}
}

func TestCompletionLoneAttributeTopLevel(t *testing.T) {
	src := "@"

	file, _ := parser.New("a.zen", []byte(src)).ParseFile()
	cursor := protocol.Position{Line: 0, Character: 1}

	items := completionAt(nil, file, src, cursor)
	labels := completionLabels(items)

	if !containsLabel(labels, "schema") {
		t.Errorf("top-level attribute completion missing %q, got %v", "schema", labels)
	}
}

func TestCompletionLoneAttributeInEntity(t *testing.T) {
	src := "entity User {\n\t@\n}\n"

	file, _ := parser.New("u.zen", []byte(src)).ParseFile()
	cursor := cursorOn(t, src, 1, "@")
	cursor.Character++

	items := completionAt(nil, file, src, cursor)
	labels := completionLabels(items)

	if !containsLabel(labels, "primary") {
		t.Errorf("in-entity attribute completion missing %q, got %v", "primary", labels)
	}
}

func TestCompletionEntityLeadAttribute(t *testing.T) {
	src := "entity @\n"

	file, _ := parser.New("e.zen", []byte(src)).ParseFile()
	cursor := cursorOn(t, src, 0, "@")
	cursor.Character++

	items := completionAt(nil, file, src, cursor)
	labels := completionLabels(items)

	if !containsLabel(labels, "schema") {
		t.Errorf("entity-lead attribute completion missing %q, got %v", "schema", labels)
	}
}

func TestCompletionIndexAttribute(t *testing.T) {
	src := "entity U {\n\tindex @\n}\n"

	file, _ := parser.New("u.zen", []byte(src)).ParseFile()
	cursor := cursorOn(t, src, 1, "@")
	cursor.Character++

	items := completionAt(nil, file, src, cursor)
	labels := completionLabels(items)

	if !containsLabel(labels, "unique") {
		t.Errorf("index-attribute completion missing %q, got %v", "unique", labels)
	}
}

func TestCompletionValidateFormatValue(t *testing.T) {
	src := "entity User {\n\tname: string @validate(format: \n}\n"

	file, _ := parser.New("u.zen", []byte(src)).ParseFile()
	cursor := protocol.Position{Line: 1, Character: uint32(len("name: string @validate(format: ") + 1)}

	items := completionAt(nil, file, src, cursor)
	labels := completionLabels(items)

	for _, want := range []string{"email", "url", "uuid"} {
		if !containsLabel(labels, want) {
			t.Errorf("validate-format completion missing %q, got %v", want, labels)
		}
	}

	if containsLabel(labels, "min_len") {
		t.Errorf("validate-format completion should not offer arg name %q", "min_len")
	}
}

func TestCompletionCompleteValidateArgOffersNothing(t *testing.T) {
	src := "entity User {\n\tname: string @validate(min_len: 1\n}\n"

	file, _ := parser.New("u.zen", []byte(src)).ParseFile()
	cursor := protocol.Position{Line: 1, Character: uint32(len("\tname: string @validate(min_len: 1"))}

	if items := completionAt(nil, file, src, cursor); len(items) != 0 {
		t.Errorf("completed validate arg produced %d items, want none: %v", len(items), completionLabels(items))
	}
}

func TestCompletionOnDeleteStartedValueOffersNothing(t *testing.T) {
	src := "entity Task {\n\tuser_id: uuid\n\tbelongs_to user: User @on_delete(cascade \n}\n"

	file, _ := parser.New("t.zen", []byte(src)).ParseFile()
	cursor := protocol.Position{Line: 2, Character: uint32(len("\tbelongs_to user: User @on_delete(cascade "))}

	if items := completionAt(nil, file, src, cursor); len(items) != 0 {
		t.Errorf("started on_delete value produced %d items, want none: %v", len(items), completionLabels(items))
	}
}

func TestCompletionDefaultTakesFreeFormValue(t *testing.T) {
	src := "entity User {\n\tid: uuid @default(\n}\n"

	file, _ := parser.New("u.zen", []byte(src)).ParseFile()
	cursor := protocol.Position{Line: 1, Character: uint32(len("\tid: uuid @default("))}

	if items := completionAt(nil, file, src, cursor); len(items) != 0 {
		t.Errorf("free-form default value produced %d items, want none: %v", len(items), completionLabels(items))
	}
}

func TestCompletionEnumParenOffersNothing(t *testing.T) {
	src := "entity Task {\n\tstatus: enum(\n}\n"

	file, _ := parser.New("t.zen", []byte(src)).ParseFile()
	cursor := protocol.Position{Line: 1, Character: uint32(len("\tstatus: enum("))}

	if items := completionAt(nil, file, src, cursor); len(items) != 0 {
		t.Errorf("enum value paren produced %d items, want none: %v", len(items), completionLabels(items))
	}
}

func TestCompletionMidDeclarationOffersNothing(t *testing.T) {
	src := "entity User {\n\tid: uuid @validate(min_len: 1)\n}\n"

	file, _ := parser.New("u.zen", []byte(src)).ParseFile()
	cursor := protocol.Position{Line: 1, Character: uint32(len("\tid: uuid @validate(min_len: 1)"))}

	if items := completionAt(nil, file, src, cursor); len(items) != 0 {
		t.Errorf("mid-declaration cursor produced %d items, want none: %v", len(items), completionLabels(items))
	}
}

func TestCompletionLoneColonOffersNothing(t *testing.T) {
	src := ":"

	file, _ := parser.New("c.zen", []byte(src)).ParseFile()
	cursor := protocol.Position{Line: 0, Character: 1}

	if items := completionAt(nil, file, src, cursor); len(items) != 0 {
		t.Errorf("lone colon produced %d items, want none: %v", len(items), completionLabels(items))
	}
}

func TestCompletionServiceBodyOffersNothing(t *testing.T) {
	src := "service S {\n\t\n}\n"

	file, _ := parser.New("s.zen", []byte(src)).ParseFile()
	cursor := protocol.Position{Line: 1, Character: 1}

	// A brace that is not an errors: set falls through past
	// errorsSetContext to blockContext, which has no vocabulary for
	// service bodies.
	if items := completionAt(nil, file, src, cursor); len(items) != 0 {
		t.Errorf("service body produced %d items, want none: %v", len(items), completionLabels(items))
	}
}

func TestCompletionNonErrorsBraceFallsThrough(t *testing.T) {
	src := "service S {\n\trpc M() -> T {\n\t\tfoo: {\n\t\t\n\t}\n\t}\n}\n"

	file, _ := parser.New("s.zen", []byte(src)).ParseFile()
	cursor := protocol.Position{Line: 3, Character: 0}

	// `foo: {` has the colon-brace shape of an errors: set but the wrong
	// name, so errorsSetContext declines and completion stays silent.
	if items := completionAt(nil, file, src, cursor); len(items) != 0 {
		t.Errorf("non-errors brace produced %d items, want none: %v", len(items), completionLabels(items))
	}
}

func TestCompletionASTRelationTargetPrefix(t *testing.T) {
	result, diags := compile.Compile(map[string]string{
		"a.zen": defASrc, // entity User
		"b.zen": defBSrc, // belongs_to user: User
	})
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	file, _ := parser.New("b.zen", []byte(defBSrc)).ParseFile()
	cursor := cursorOn(t, defBSrc, 3, "User")

	items := completionAt(result.Schema, file, defBSrc, cursor)
	labels := completionLabels(items)

	if !containsLabel(labels, "User") {
		t.Errorf("AST relation-target completion missing %q, got %v", "User", labels)
	}

	if containsLabel(labels, "Task") {
		t.Errorf("AST relation-target prefix %q should not match %q, got %v", "User", "Task", labels)
	}
}

func TestCompletionASTFieldTypePrefixFilters(t *testing.T) {
	file, _ := parser.New("a.zen", []byte(defASrc)).ParseFile()
	cursor := cursorOn(t, defASrc, 1, "uuid")

	items := completionAt(nil, file, defASrc, cursor)
	labels := completionLabels(items)

	if len(labels) != 1 || labels[0] != "uuid" {
		t.Errorf("AST field-type completion = %v, want exactly [uuid]", labels)
	}
}

func TestCompletionCursorPastEndOfFile(t *testing.T) {
	src := "entity User {\n}\n"

	file, _ := parser.New("u.zen", []byte(src)).ParseFile()
	cursor := protocol.Position{Line: 50, Character: 0}

	labels := completionLabels(completionAt(nil, file, src, cursor))

	if !containsLabel(labels, "entity") {
		t.Errorf("past-end cursor missing top-level %q, got %v", "entity", labels)
	}
}

func TestCompletionTokenOnEarlierLineThanCursor(t *testing.T) {
	src := "entity User {\n\tid: uuid\n"

	file, _ := parser.New("u.zen", []byte(src)).ParseFile()
	cursor := protocol.Position{Line: 2, Character: 0}

	labels := completionLabels(completionAt(nil, file, src, cursor))

	if !containsLabel(labels, "has_many") {
		t.Errorf("expected entity-member completion, got %v", labels)
	}
}

// testTok builds a minimal token for white-box classifier tests.
func testTok(kind token.Kind, lit string, line, col int) token.Token {
	return token.Token{Kind: kind, Lit: lit, Pos: diag.Position{Line: line, Col: col}}
}

func TestAttrNameContext_condition_expected(t *testing.T) {
	ident := testTok(token.IDENT, "id", 1, 1)

	tests := []struct {
		name   string
		before []token.Token
		depth  int
		want   completionContext
	}{
		{"lone attribute top level", nil, 0, ctxEntityAttrName},
		{"lone attribute in block", nil, 1, ctxFieldAttrName},
		{"entity lead", []token.Token{testTok(token.ENTITY, "entity", 1, 1)}, 0, ctxEntityAttrName},
		{"index lead", []token.Token{testTok(token.INDEX, "index", 2, 2)}, 1, ctxIndexAttrName},
		{"has_many lead", []token.Token{testTok(token.HAS_MANY, "has_many", 2, 2)}, 1, ctxRelationAttrName},
		{"has_one lead", []token.Token{testTok(token.HAS_ONE, "has_one", 2, 2)}, 1, ctxRelationAttrName},
		{"belongs_to lead", []token.Token{testTok(token.BELONGS_TO, "belongs_to", 2, 2)}, 1, ctxRelationAttrName},
		{"many_to_many lead", []token.Token{testTok(token.MANY_TO_MANY, "many_to_many", 2, 2)}, 1, ctxRelationAttrName},
		{"plain field lead", []token.Token{ident}, 1, ctxFieldAttrName},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := attrNameContext(tc.before, tc.depth); got != tc.want {
				t.Errorf("attrNameContext() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestColonContext_condition_expected(t *testing.T) {
	colon := testTok(token.COLON, ":", 1, 4)

	tests := []struct {
		name string
		toks []token.Token
		want completionContext
	}{
		{"lone colon", []token.Token{colon}, ctxNone},
		{"field type", []token.Token{testTok(token.IDENT, "id", 1, 1), colon}, ctxFieldType},
		{"relation target", []token.Token{testTok(token.HAS_MANY, "has_many", 1, 1), testTok(token.IDENT, "tasks", 1, 10), colon}, ctxRelationTarget},
		{"too many tokens", []token.Token{testTok(token.IDENT, "a", 1, 1), testTok(token.IDENT, "b", 1, 3), colon}, ctxNone},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := colonContext(tc.toks); got != tc.want {
				t.Errorf("colonContext() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestBlockContext_condition_expected(t *testing.T) {
	entity := []token.Token{testTok(token.ENTITY, "entity", 1, 1), testTok(token.IDENT, "User", 1, 8), testTok(token.LBRACE, "{", 1, 13)}
	service := []token.Token{testTok(token.SERVICE, "service", 1, 1), testTok(token.IDENT, "S", 1, 9), testTok(token.LBRACE, "{", 1, 11)}
	closed := []token.Token{testTok(token.ENTITY, "entity", 1, 1), testTok(token.LBRACE, "{", 1, 8), testTok(token.RBRACE, "}", 1, 9)}

	tests := []struct {
		name string
		toks []token.Token
		want completionContext
	}{
		{"empty is top level", nil, ctxTopLevelKeyword},
		{"closed block is top level", closed, ctxTopLevelKeyword},
		{"entity body", entity, ctxEntityMember},
		{"service body has no vocabulary", service, ctxNone},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := blockContext(tc.toks); got != tc.want {
				t.Errorf("blockContext() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestEnclosingDeclKind_condition_expected(t *testing.T) {
	entity := testTok(token.ENTITY, "entity", 1, 1)
	lbrace := testTok(token.LBRACE, "{", 1, 8)
	rbrace := testTok(token.RBRACE, "}", 2, 1)

	tests := []struct {
		name string
		toks []token.Token
		open int
		want token.Kind
	}{
		{"entity owns brace", []token.Token{entity, lbrace}, 1, token.ENTITY},
		{"sibling brace stops scan", []token.Token{lbrace, entity, rbrace, lbrace}, 3, token.ILLEGAL},
		{"nothing before brace", []token.Token{lbrace}, 0, token.ILLEGAL},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := enclosingDeclKind(tc.toks, tc.open); got != tc.want {
				t.Errorf("enclosingDeclKind() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestOpenBraceIndex_condition_expected(t *testing.T) {
	lbrace := testTok(token.LBRACE, "{", 1, 1)
	rbrace := testTok(token.RBRACE, "}", 1, 2)

	tests := []struct {
		name string
		toks []token.Token
		want int
	}{
		{"none open", nil, -1},
		{"one open", []token.Token{lbrace}, 0},
		{"balanced is none open", []token.Token{lbrace, rbrace}, -1},
		{"close without open", []token.Token{rbrace}, -1},
		{"innermost wins", []token.Token{lbrace, lbrace, rbrace}, 0},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := openBraceIndex(tc.toks); got != tc.want {
				t.Errorf("openBraceIndex() = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestBraceDepth_condition_expected(t *testing.T) {
	lbrace := testTok(token.LBRACE, "{", 1, 1)
	rbrace := testTok(token.RBRACE, "}", 1, 2)

	tests := []struct {
		name string
		toks []token.Token
		want int
	}{
		{"empty", nil, 0},
		{"one open", []token.Token{lbrace}, 1},
		{"balanced", []token.Token{lbrace, rbrace}, 0},
		{"close without open stays zero", []token.Token{rbrace}, 0},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := braceDepth(tc.toks); got != tc.want {
				t.Errorf("braceDepth() = %d, want %d", got, tc.want)
			}
		})
	}
}
