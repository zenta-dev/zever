package main

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"testing"

	"go.lsp.dev/protocol"
	"go.lsp.dev/uri"

	"github.com/zenta-dev/zever/dsl/compile"
	"github.com/zenta-dev/zever/dsl/diag"
	"github.com/zenta-dev/zever/dsl/parser"
	"github.com/zenta-dev/zever/dsl/resolver"
	"github.com/zenta-dev/zever/dsl/token"
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

// candidateLabels extracts the label of every candidate, the same shape
// completionLabels produces for protocol items.
func candidateLabels(cands []candidate) []string {
	labels := make([]string, 0, len(cands))
	for _, cand := range cands {
		labels = append(labels, cand.label)
	}

	return labels
}

// findCandidate returns the first candidate with the given label.
func findCandidate(cands []candidate, label string) (candidate, bool) {
	for _, cand := range cands {
		if cand.label == label {
			return cand, true
		}
	}

	return candidate{}, false
}

// atLineEnd returns the cursor position at the end of a 0-indexed source
// line, the usual hand-built cursor for lexical-tier completion fixtures.
func atLineEnd(src string, line int) protocol.Position {
	return protocol.Position{
		Line:      uint32(line),                                //nolint:gosec // test fixture line index
		Character: uint32(len(strings.Split(src, "\n")[line])), //nolint:gosec // len never negative
	}
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

// TestCompletionValidateArgNameNarrowsByScalarType proves the @validate arg
// names are narrowed to the kinds that apply to the field's declared scalar
// type, instead of always offering both string- and numeric-only variants.
func TestCompletionValidateArgNameNarrowsByScalarType(t *testing.T) {
	tests := []struct {
		name      string
		src       string
		want      []string
		notWanted []string
	}{
		{
			name:      "string field offers string kinds only",
			src:       "entity User {\n\tname: string @validate(\n}\n",
			want:      []string{"format", "min_len", "max_len"},
			notWanted: []string{"gt", "gte", "lt", "lte"},
		},
		{
			name:      "numeric field offers numeric kinds only",
			src:       "entity User {\n\tage: int64 @validate(\n}\n",
			want:      []string{"gt", "gte", "lt", "lte"},
			notWanted: []string{"format", "min_len", "max_len"},
		},
		{
			name:      "attribute before validate still finds the field type",
			src:       "entity User {\n\tage: int64 @default(0) @validate(\n}\n",
			want:      []string{"gt", "gte", "lt", "lte"},
			notWanted: []string{"format", "min_len", "max_len"},
		},
		{
			name: "scalar with no applicable kind offers none",
			src:  "entity User {\n\tid: uuid @validate(\n}\n",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			file, _ := parser.New("u.zen", []byte(tc.src)).ParseFile()
			cursor := protocol.Position{Line: 1, Character: uint32(len(strings.Split(tc.src, "\n")[1]))} //nolint:gosec // test fixture

			labels := completionLabels(completionAt(nil, file, tc.src, cursor))

			for _, want := range tc.want {
				if !containsLabel(labels, want) {
					t.Errorf("missing %q, got %v", want, labels)
				}
			}

			for _, notWanted := range tc.notWanted {
				if containsLabel(labels, notWanted) {
					t.Errorf("should not offer %q, got %v", notWanted, labels)
				}
			}

			if tc.want == nil && len(labels) != 0 {
				t.Errorf("want no items, got %v", labels)
			}
		})
	}
}

// TestCompletionValidateArgNameFallsBackWhenTypeUnknown proves that when the
// field's type cannot be read off the attribute's line (here the attribute is
// on its own line), completion still offers the full candidate list rather
// than guessing.
func TestCompletionValidateArgNameFallsBackWhenTypeUnknown(t *testing.T) {
	src := "entity User {\n\tname: string\n\t\t@validate(\n}\n"

	file, _ := parser.New("u.zen", []byte(src)).ParseFile()
	cursor := protocol.Position{Line: 2, Character: uint32(len(strings.Split(src, "\n")[2]))} //nolint:gosec // test fixture

	labels := completionLabels(completionAt(nil, file, src, cursor))

	for _, want := range []string{"format", "min_len", "max_len", "gt", "gte", "lt", "lte"} {
		if !containsLabel(labels, want) {
			t.Errorf("fallback missing %q, got %v", want, labels)
		}
	}
}

// TestValidateArgCandidatesFor covers the scalar-type narrowing helper
// directly, including the not-determinable empty scalar.
func TestValidateArgCandidatesFor(t *testing.T) {
	tests := []struct {
		scalar string
		want   int
	}{
		{"", len(validateArgCandidates)},
		{"string", len(stringValidateArgCandidates)},
		{"int32", len(numericValidateArgCandidates)},
		{"int64", len(numericValidateArgCandidates)},
		{"float32", len(numericValidateArgCandidates)},
		{"float64", len(numericValidateArgCandidates)},
		{"uuid", 0},
		{"bool", 0},
		{"timestamp", 0},
		{"User", 0},
	}

	for _, tc := range tests {
		if got := len(validateArgCandidatesFor(tc.scalar)); got != tc.want {
			t.Errorf("validateArgCandidatesFor(%q) returned %d candidates, want %d", tc.scalar, got, tc.want)
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

	if detail, present := item.Detail.Get(); !present || detail != "declare an entity" {
		t.Errorf("initial completion item Detail = %q (present=%v), want %q", detail, present, "declare an entity")
	}

	if len(item.Data) == 0 {
		t.Fatalf("initial completion item Data is empty, want the resolve payload")
	}

	if st, present := item.SortText.Get(); !present || st == "" {
		t.Fatalf("initial completion item SortText is empty, want a sort prefix")
	}

	if ft, present := item.FilterText.Get(); !present || ft != "entity" {
		t.Errorf("initial completion item FilterText = %q (present=%v), want %q", ft, present, "entity")
	}

	if item.LabelDetails == nil || item.LabelDetails.Detail == nil || *item.LabelDetails.Detail != "declare an entity" {
		t.Errorf("initial completion item LabelDetails missing detail, got %+v", item.LabelDetails)
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

func TestCompletionServiceBodyOffersRPC(t *testing.T) {
	src := "service S {\n\t\n}\n"

	file, _ := parser.New("s.zen", []byte(src)).ParseFile()
	cursor := protocol.Position{Line: 1, Character: 1}

	// A brace that is not an errors: set falls through past
	// errorsSetContext to blockContext, which offers the service-body
	// vocabulary (only rpc).
	items := completionAt(nil, file, src, cursor)
	labels := completionLabels(items)

	if !containsLabel(labels, "rpc") {
		t.Errorf("service body missing %q, got %v", "rpc", labels)
	}

	if containsLabel(labels, "entity") {
		t.Errorf("service body should not offer top-level keyword %q, got %v", "entity", labels)
	}
}

func TestCompletionRPCBodyOffersOptions(t *testing.T) {
	src := "service S {\n\trpc M() -> T {\n\t\t\n\t}\n}\n"

	file, _ := parser.New("s.zen", []byte(src)).ParseFile()
	cursor := protocol.Position{Line: 2, Character: 2}

	items := completionAt(nil, file, src, cursor)
	labels := completionLabels(items)

	for _, want := range []string{"http", "auth", "permission", "errors", "paginated"} {
		if !containsLabel(labels, want) {
			t.Errorf("rpc body missing %q, got %v", want, labels)
		}
	}

	if containsLabel(labels, "rpc") {
		t.Errorf("rpc body should not offer service member %q, got %v", "rpc", labels)
	}
}

func TestCompletionJobBodyOffersOptions(t *testing.T) {
	src := "job J() {\n\t\n}\n"

	file, _ := parser.New("j.zen", []byte(src)).ParseFile()
	cursor := protocol.Position{Line: 1, Character: 1}

	items := completionAt(nil, file, src, cursor)
	labels := completionLabels(items)

	for _, want := range []string{"queue", "retry"} {
		if !containsLabel(labels, want) {
			t.Errorf("job body missing %q, got %v", want, labels)
		}
	}

	if containsLabel(labels, "cron") {
		t.Errorf("job body should not offer schedule option %q, got %v", "cron", labels)
	}
}

func TestCompletionScheduleBodyOffersOptions(t *testing.T) {
	src := "schedule S {\n\t\n}\n"

	file, _ := parser.New("s.zen", []byte(src)).ParseFile()
	cursor := protocol.Position{Line: 1, Character: 1}

	items := completionAt(nil, file, src, cursor)
	labels := completionLabels(items)

	for _, want := range []string{"cron", "dispatch"} {
		if !containsLabel(labels, want) {
			t.Errorf("schedule body missing %q, got %v", want, labels)
		}
	}

	if containsLabel(labels, "queue") {
		t.Errorf("schedule body should not offer job option %q, got %v", "queue", labels)
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
			if got := colonContext(tc.toks, tc.toks); got != tc.want {
				t.Errorf("colonContext() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestBlockContext_condition_expected(t *testing.T) {
	entity := []token.Token{testTok(token.ENTITY, "entity", 1, 1), testTok(token.IDENT, "User", 1, 8), testTok(token.LBRACE, "{", 1, 13)}
	service := []token.Token{testTok(token.SERVICE, "service", 1, 1), testTok(token.IDENT, "S", 1, 9), testTok(token.LBRACE, "{", 1, 11)}
	job := []token.Token{testTok(token.JOB, "job", 1, 1), testTok(token.IDENT, "J", 1, 5), testTok(token.LBRACE, "{", 1, 8)}
	schedule := []token.Token{testTok(token.SCHEDULE, "schedule", 1, 1), testTok(token.IDENT, "S", 1, 10), testTok(token.LBRACE, "{", 1, 12)}
	rpc := []token.Token{
		testTok(token.SERVICE, "service", 1, 1),
		testTok(token.IDENT, "S", 1, 9),
		testTok(token.LBRACE, "{", 1, 11),
		testTok(token.RPC, "rpc", 2, 2),
		testTok(token.IDENT, "M", 2, 6),
		testTok(token.LPAREN, "(", 2, 7),
		testTok(token.RPAREN, ")", 2, 8),
		testTok(token.ARROW, "->", 2, 10),
		testTok(token.IDENT, "T", 2, 13),
		testTok(token.LBRACE, "{", 2, 15),
	}
	closed := []token.Token{testTok(token.ENTITY, "entity", 1, 1), testTok(token.LBRACE, "{", 1, 8), testTok(token.RBRACE, "}", 1, 9)}

	tests := []struct {
		name string
		toks []token.Token
		want completionContext
	}{
		{"empty is top level", nil, ctxTopLevelKeyword},
		{"closed block is top level", closed, ctxTopLevelKeyword},
		{"entity body", entity, ctxEntityMember},
		{"service body", service, ctxServiceMember},
		{"rpc body", rpc, ctxRPCOptionName},
		{"job body", job, ctxJobOptionName},
		{"schedule body", schedule, ctxScheduleOptionName},
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

// TestScalarTypeCandidatesDeriveFromResolver pins that the field-type
// candidates are derived from resolver.ScalarTypeNames plus the separate
// `enum` candidate, keep their human descriptions, and stay sorted by
// label -- so neither completion nor the resolver's scalar table can drift
// without failing here (every resolver name, including the datetime alias
// of timestamp, must appear exactly once).
func TestScalarTypeCandidatesDeriveFromResolver(t *testing.T) {
	want := append([]string{}, resolver.ScalarTypeNames()...)
	want = append(want, "enum")
	sort.Strings(want)

	labels := candidateLabels(scalarTypeCandidates)

	if len(labels) != len(want) {
		t.Fatalf("scalarTypeCandidates has %d entries (%v), want %d: %v", len(labels), labels, len(want), want)
	}

	for i, name := range want {
		if labels[i] != name {
			t.Fatalf("scalarTypeCandidates[%d] = %q, want %q (full list: %v)", i, labels[i], name, labels)
		}
	}

	if !sort.StringsAreSorted(labels) {
		t.Errorf("scalarTypeCandidates is not sorted by label: %v", labels)
	}
}

// TestScalarTypeCandidateDetails pins the human descriptions of the scalar
// candidates, including that `datetime` (the resolver's alias of timestamp)
// phrases like timestamp, and that `enum` keeps its separate inline-enum
// phrasing.
func TestScalarTypeCandidateDetails(t *testing.T) {
	tests := []struct {
		label string
		want  string
	}{
		{"datetime", "timestamp scalar"},
		{"timestamp", "timestamp scalar"},
		{"uuid", "UUID scalar"},
		{"bool", "boolean scalar"},
		{"json", "JSON document scalar"},
		{"enum", "enum(value, ...) inline enumeration"},
	}

	for _, tc := range tests {
		t.Run(tc.label, func(t *testing.T) {
			cand, ok := findCandidate(scalarTypeCandidates, tc.label)
			if !ok {
				t.Fatalf("scalarTypeCandidates missing %q entirely", tc.label)
			}

			if cand.detail != tc.want {
				t.Errorf("candidate %q detail = %q, want %q", tc.label, cand.detail, tc.want)
			}
		})
	}
}

// TestCompletionFieldTypePinFullCandidateList pins the exact candidate list
// a `field: <empty>` slot offers -- resolver.ScalarTypeNames plus enum,
// sorted by label -- so adding, dropping or renaming a scalar (e.g. losing
// the datetime alias) fails here at the completion level too.
func TestCompletionFieldTypePinFullCandidateList(t *testing.T) {
	src := "entity User {\n\tid: \n}\n"

	file, _ := parser.New("u.zen", []byte(src)).ParseFile()

	want := []string{"bool", "bytes", "date", "datetime", "enum", "float32", "float64", "int32", "int64", "json", "string", "timestamp", "uuid"}

	items := completionAt(nil, file, src, atLineEnd(src, 1))
	labels := completionLabels(items)

	if len(labels) != len(want) {
		t.Fatalf("field-type completion returned %d items (%v), want exactly %v", len(labels), labels, want)
	}

	for i, name := range want {
		if labels[i] != name {
			t.Errorf("field-type completion label %d = %q, want %q", i, labels[i], name)
		}
	}
}

// TestCompletionDispatchValueOffersJobs proves the schedule dispatch slot
// offers the jobs (from the schema when resolvable, else the file's own job
// declarations) and stays silent once the call argument list has content.
func TestCompletionDispatchValueOffersJobs(t *testing.T) {
	jobName := "CleanupAbandonedCarts"

	tests := []struct {
		name    string
		value   string
		want    []string
		notWant []string
	}{
		{
			name:    "empty value lists the jobs",
			value:   "",
			want:    []string{jobName},
			notWant: []string{"uuid", "int32", "GET"},
		},
		{
			name:    "typed prefix filters the jobs",
			value:   "clea",
			want:    []string{jobName},
			notWant: []string{"uuid"},
		},
		{
			name:    "unclosed call paren still lists the jobs",
			value:   jobName + "(",
			want:    []string{jobName},
			notWant: []string{"uuid"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			src := fmt.Sprintf("job %s() {\n}\n\nschedule Nightly {\n\tdispatch: %s\n}\n", jobName, tc.value)

			file, _ := parser.New("s.zen", []byte(src)).ParseFile()

			cursor := atLineEnd(src, 4)
			cursor.Character -= uint32(len(tc.value)) //nolint:gosec // test fixture arithmetic

			items := completionAt(nil, file, src, cursor)
			labels := completionLabels(items)

			for _, want := range tc.want {
				if !containsLabel(labels, want) {
					t.Errorf("dispatch completion missing %q, got %v", want, labels)
				}
			}

			for _, notWant := range tc.notWant {
				if containsLabel(labels, notWant) {
					t.Errorf("dispatch completion should not offer %q, got %v", notWant, labels)
				}
			}

			item, ok := findCompletionItem(items, jobName)
			if !ok {
				t.Fatalf("missing %q item entirely, got %v", jobName, labels)
			}

			if item.Kind != protocol.CompletionItemKindFunction {
				t.Errorf("dispatch target kind = %v, want Function", item.Kind)
			}
		})
	}
}

// TestCompletionDispatchValueScopedToSchedule proves a field legitimately
// named `dispatch` outside a schedule block still completes as a field type
// slot rather than a job target.
func TestCompletionDispatchValueScopedToSchedule(t *testing.T) {
	src := "job CleanupAbandonedCarts() {\n}\n\nentity Order {\n\tdispatch: \n}\n"

	file, _ := parser.New("o.zen", []byte(src)).ParseFile()

	items := completionAt(nil, file, src, atLineEnd(src, 4))
	labels := completionLabels(items)

	if containsLabel(labels, "CleanupAbandonedCarts") {
		t.Errorf("entity field named dispatch should not offer jobs, got %v", labels)
	}

	if !containsLabel(labels, "uuid") {
		t.Errorf("entity field named dispatch should still offer scalar types, got %v", labels)
	}
}

// TestCompletionHTTPVerbSlot proves the rpc http: option's verb slot offers
// the fixed v1 verb vocabulary in a deterministic order, prefix-filters,
// and stays scoped to rpc blocks.
func TestCompletionHTTPVerbSlot(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		want    []string
		notWant []string
	}{
		{
			name:    "empty value offers every verb in order",
			value:   "",
			want:    []string{"GET", "POST", "PUT", "PATCH", "DELETE"},
			notWant: []string{"uuid"},
		},
		{
			name:    "typed prefix filters the verbs",
			value:   "PO",
			want:    []string{"POST"},
			notWant: []string{"GET", "PUT", "PATCH", "DELETE"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			src := fmt.Sprintf("service S {\n\trpc M() -> T {\n\t\thttp: %s\n\t}\n}\n", tc.value)

			file, _ := parser.New("s.zen", []byte(src)).ParseFile()

			cursor := atLineEnd(src, 2)

			items := completionAt(nil, file, src, cursor)
			labels := completionLabels(items)

			if len(labels) != len(tc.want) {
				t.Fatalf("http verb completion returned %v, want exactly %v", labels, tc.want)
			}

			for i, name := range tc.want {
				if labels[i] != name {
					t.Errorf("http verb completion label %d = %q, want %q", i, labels[i], name)
				}
			}

			for _, notWant := range tc.notWant {
				if containsLabel(labels, notWant) {
					t.Errorf("http verb completion should not offer %q, got %v", notWant, labels)
				}
			}
		})
	}
}

// TestCompletionHTTPVerbScopedToRPC proves a field legitimately named `http`
// outside an rpc block still completes as a field type slot rather than a
// verb slot.
func TestCompletionHTTPVerbScopedToRPC(t *testing.T) {
	src := "entity Order {\n\thttp: \n}\n"

	file, _ := parser.New("o.zen", []byte(src)).ParseFile()

	items := completionAt(nil, file, src, atLineEnd(src, 1))
	labels := completionLabels(items)

	if containsLabel(labels, "GET") {
		t.Errorf("entity field named http should not offer HTTP verbs, got %v", labels)
	}

	if !containsLabel(labels, "uuid") {
		t.Errorf("entity field named http should still offer scalar types, got %v", labels)
	}
}

// TestCompletionDefaultEnumValueOffersEnumValues proves @default( on a
// field whose declared type names an enum in this file enumerates exactly
// that enum's values, prefix-filters, and offers nothing it cannot resolve
// (a scalar field, or a type name missing from both the schema and the
// file) rather than guessing.
func TestCompletionDefaultEnumValueOffersEnumValues(t *testing.T) {
	tests := []struct {
		name    string
		typ     string
		value   string
		want    []string
		notWant []string
	}{
		{
			name:    "named enum values offered",
			typ:     "Status",
			value:   "",
			want:    []string{"pending", "paid"},
			notWant: []string{"pending,", "Mystery"},
		},
		{
			name:    "typed prefix filters the values",
			typ:     "Status",
			value:   "pe",
			want:    []string{"pending"},
			notWant: []string{"paid"},
		},
		{
			name: "unresolvable type name offers nothing",
			typ:  "Mystery",
		},
		{
			name: "scalar type name offers nothing",
			typ:  "uuid",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			src := fmt.Sprintf("enum Status {\n\tpending, paid\n}\n\nentity Order {\n\tstatus: %s @default(%s\n}\n", tc.typ, tc.value)

			file, _ := parser.New("o.zen", []byte(src)).ParseFile()

			cursor := atLineEnd(src, 5)

			items := completionAt(nil, file, src, cursor)
			labels := completionLabels(items)

			for _, want := range tc.want {
				if !containsLabel(labels, want) {
					t.Errorf("default completion missing %q, got %v", want, labels)
				}
			}

			for _, notWant := range tc.notWant {
				if containsLabel(labels, notWant) {
					t.Errorf("default completion should not offer %q, got %v", notWant, labels)
				}
			}

			if tc.want == nil && len(labels) != 0 {
				t.Errorf("default completion returned %d items, want none: %v", len(labels), labels)
			}

			if len(tc.want) > 0 {
				item, ok := findCompletionItem(items, tc.want[0])
				if !ok {
					t.Fatalf("missing %q item entirely, got %v", tc.want[0], labels)
				}

				if item.Kind != protocol.CompletionItemKindEnumMember {
					t.Errorf("default value kind = %v, want EnumMember", item.Kind)
				}

				if detail, present := item.Detail.Get(); !present || detail != fmt.Sprintf("enum %s value", tc.typ) {
					t.Errorf("default value detail = %q (present=%v), want %q", detail, present, fmt.Sprintf("enum %s value", tc.typ))
				}
			}
		})
	}
}

// TestCompletionDefaultEnumValueNeedsSchemaAcrossFiles proves the default
// slot enumerates a named enum declared in a different file of the same
// workspace schema, and that without the schema (or with the enum absent
// from both sources) it offers nothing instead of guessing.
func TestCompletionDefaultEnumValueNeedsSchemaAcrossFiles(t *testing.T) {
	enumSrc := "enum Status {\n\tpending\n}\n"
	orderSrc := "entity Order {\n\tstatus: Status @default(\n}\n"

	result, _ := compile.Compile(map[string]string{
		"e.zen": enumSrc, // the enum lives in another file of the workspace
		"o.zen": orderSrc,
	})

	file, _ := parser.New("o.zen", []byte(orderSrc)).ParseFile()
	cursor := atLineEnd(orderSrc, 1)

	labels := completionLabels(completionAt(result.Schema, file, orderSrc, cursor))
	if !containsLabel(labels, "pending") {
		t.Errorf("schema-backed default completion missing %q, got %v", "pending", labels)
	}

	if containsLabel(labels, "paid") {
		t.Errorf("schema-backed default completion offered a value of a different enum, got %v", labels)
	}

	// Fall back to the file only: the enum is not declared here, so the slot
	// must stay silent rather than guess.
	labels = completionLabels(completionAt(nil, file, orderSrc, cursor))
	if len(labels) != 0 {
		t.Errorf("schema-less default completion offered %v, want none", labels)
	}
}

// TestCompletionTypeSlotsOfferEntitiesAndMessages proves the type slots
// that take bare type references -- a message field's type, an rpc's return
// type, an rpc/job param's type -- offer entities and messages merged (plus
// the scalar names they also accept), while the relation-target slot stays
// entity-only.
func TestCompletionTypeSlotsOfferEntitiesAndMessages(t *testing.T) {
	src := "entity User {\n\tid: uuid @primary\n}\n\nmessage UserCreated {\n\tactor: User\n}\n\nmessage SyncReport {\n\tnote: \n}\n\nservice S {\n\trpc Get(u: User) -> UserCreated {\n\t}\n}\n\njob Sync(u: User) {\n}\n"

	result, _ := compile.Compile(map[string]string{"t.zen": src})
	if result == nil {
		t.Fatal("compile.Compile() = nil schema")
	}

	file, _ := parser.New("t.zen", []byte(src)).ParseFile()

	// Lines: 0 entity User, 1 id, 2 }, 3 "", 4 message UserCreated,
	// 5 actor, 6 }, 7 "", 8 message SyncReport, 9 note, 10 }, 11 "",
	// 12 service, 13 rpc Get(u: User) -> UserCreated, 14 }, 15 }, 16 "",
	// 17 job Sync(u: User), 18 }, 19 "".
	tests := []struct {
		name    string
		line    int
		needle  string
		want    []string
		notWant []string
	}{
		{
			name:    "message field type",
			line:    5,
			needle:  "User",
			want:    []string{"User", "UserCreated"},
			notWant: []string{"has_many"},
		},
		{
			name:    "message field type (empty slot)",
			line:    9,
			needle:  "",
			want:    []string{"User", "UserCreated", "uuid", "bool", "string"},
			notWant: []string{"has_many"},
		},
		{
			name:   "rpc param type",
			line:   13,
			needle: "User)",
			want:   []string{"User", "UserCreated"},
		},
		{
			name:   "rpc return type",
			line:   13,
			needle: "UserCreated",
			want:   []string{"UserCreated"},
		},
		{
			name:   "job param type",
			line:   17,
			needle: "User)",
			want:   []string{"User", "UserCreated"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var cursor protocol.Position
			if tc.needle == "" {
				cursor = atLineEnd(src, tc.line)
			} else {
				cursor = cursorOn(t, src, tc.line, tc.needle)
			}

			items := completionAt(result.Schema, file, src, cursor)
			labels := completionLabels(items)

			for _, want := range tc.want {
				if !containsLabel(labels, want) {
					t.Errorf("%s completion missing %q, got %v", tc.name, want, labels)
				}
			}

			for _, notWant := range tc.notWant {
				if containsLabel(labels, notWant) {
					t.Errorf("%s completion should not offer %q, got %v", tc.name, notWant, labels)
				}
			}
		})
	}
}

// snippetCaps is the negotiated capability set of a client that accepts
// snippet insert text, used by the snippet completion tests.
func snippetCaps() clientCaps {
	return clientCaps{snippetSupport: true}
}

// TestCompletionSnippetTopLevelSkeletons proves a snippet-capable client is
// offered skeleton snippets for every block-opening top-level keyword,
// while keywords without a skeleton still insert their label as plain text.
func TestCompletionSnippetTopLevelSkeletons(t *testing.T) {
	src := ""

	file, _ := parser.New("empty.zen", []byte(src)).ParseFile()
	cursor := protocol.Position{Line: 0, Character: 0}

	items := completionAtWithCaps(nil, file, src, cursor, snippetCaps())

	tests := []struct {
		label   string
		snippet string
	}{
		{"entity", "entity ${1:Name} {\n\t$0\n}"},
		{"message", "message ${1:Name} {\n\t$0\n}"},
		{"service", "service ${1:Name} {\n\t$0\n}"},
		{"job", "job ${1:Name}(${2}) {\n\t$0\n}"},
		{"schedule", "schedule ${1:Name} {\n\t$0\n}"},
		{"enum", "enum ${1:Name} {\n\t$0\n}"},
		{"saga", "saga ${1:Name} {\n\t$0\n}"},
	}

	for _, tc := range tests {
		t.Run(tc.label, func(t *testing.T) {
			item, ok := findCompletionItem(items, tc.label)
			if !ok {
				t.Fatalf("completion items missing %q, got %v", tc.label, completionLabels(items))
			}

			if got, _ := item.InsertText.Get(); got != tc.snippet {
				t.Errorf("%s InsertText = %q, want snippet %q", tc.label, got, tc.snippet)
			}

			if item.InsertTextFormat != protocol.InsertTextFormatSnippet {
				t.Errorf("%s InsertTextFormat = %v, want Snippet", tc.label, item.InsertTextFormat)
			}

			if item.TextEdit != nil {
				t.Errorf("%s TextEdit is present with no prefix typed, want nil", tc.label)
			}
		})
	}

	for _, label := range []string{"rpc", "index", "has_many"} {
		item, ok := findCompletionItem(items, label)
		if !ok {
			t.Fatalf("completion items missing %q, got %v", label, completionLabels(items))
		}

		if got, _ := item.InsertText.Get(); got != label {
			t.Errorf("%s InsertText = %q, want plain label", label, got)
		}

		if item.InsertTextFormat != protocol.InsertTextFormatPlainText {
			t.Errorf("%s InsertTextFormat = %v, want PlainText", label, item.InsertTextFormat)
		}
	}
}

// TestCompletionSnippetFallbackWithoutCaps proves a client that negotiated
// no snippet support gets plain-text labels everywhere -- the pre-snippet
// behavior -- even for candidates that carry a snippet skeleton.
func TestCompletionSnippetFallbackWithoutCaps(t *testing.T) {
	src := ""

	file, _ := parser.New("empty.zen", []byte(src)).ParseFile()
	cursor := protocol.Position{Line: 0, Character: 0}

	items := completionAtWithCaps(nil, file, src, cursor, clientCaps{})

	if len(items) == 0 {
		t.Fatal("completion returned no items")
	}

	for _, item := range items {
		if item.InsertTextFormat != protocol.InsertTextFormatPlainText {
			t.Errorf("%s InsertTextFormat = %v, want PlainText", item.Label, item.InsertTextFormat)
		}

		if got, _ := item.InsertText.Get(); got != item.Label {
			t.Errorf("%s InsertText = %q, want the plain label", item.Label, got)
		}
	}
}

// TestCompletionSnippetBraceMagic proves the brace-context keywords -- the
// service-body rpc and the entity-body index -- complete to snippets for
// snippet-capable clients and to the plain label otherwise.
func TestCompletionSnippetBraceMagic(t *testing.T) {
	tests := []struct {
		name    string
		src     string
		line    int
		char    int
		label   string
		snippet string
	}{
		{
			name:    "service-body rpc signature",
			src:     "service S {\n\t\n}\n",
			line:    1,
			char:    1,
			label:   "rpc",
			snippet: "rpc ${1:name}($2) -> ${3:Type} {\n\t$0\n}",
		},
		{
			name:    "entity-body index columns",
			src:     "entity User {\n\tid: uuid @primary\n\n}\n",
			line:    2,
			char:    0,
			label:   "index",
			snippet: "index ${1:cols}",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			file, _ := parser.New("m.zen", []byte(tc.src)).ParseFile()
			cursor := protocol.Position{Line: uint32(tc.line), Character: uint32(tc.char)} //nolint:gosec // test fixture indices

			item, ok := findCompletionItem(completionAtWithCaps(nil, file, tc.src, cursor, snippetCaps()), tc.label)
			if !ok {
				t.Fatalf("%s completion missing %q", tc.name, tc.label)
			}

			if got, _ := item.InsertText.Get(); got != tc.snippet {
				t.Errorf("%s InsertText = %q, want snippet %q", tc.label, got, tc.snippet)
			}

			if item.InsertTextFormat != protocol.InsertTextFormatSnippet {
				t.Errorf("%s InsertTextFormat = %v, want Snippet", tc.label, item.InsertTextFormat)
			}

			item, ok = findCompletionItem(completionAt(nil, file, tc.src, cursor), tc.label)
			if !ok {
				t.Fatalf("%s completion missing %q", tc.name, tc.label)
			}

			if got, _ := item.InsertText.Get(); got != tc.label {
				t.Errorf("%s InsertText without caps = %q, want the plain label", tc.label, got)
			}

			if item.InsertTextFormat != protocol.InsertTextFormatPlainText {
				t.Errorf("%s InsertTextFormat without caps = %v, want PlainText", tc.label, item.InsertTextFormat)
			}
		})
	}
}

// TestCompletionTextEditReplacesPrefix proves an item offered while a prefix
// is typed carries a TextEdit covering exactly that prefix on the cursor
// line, with the edit text matching the effective insert text -- the plain
// label here, the snippet for a snippet candidate.
func TestCompletionTextEditReplacesPrefix(t *testing.T) {
	src := "service S {\n\trpc M() -> T {\n\t\terrors: { not_ \n\t}\n}\n"

	file, _ := parser.New("s.zen", []byte(src)).ParseFile()
	cursor := cursorOn(t, src, 2, "not_")
	cursor.Character += 4 // land just after "not_"

	item, ok := findCompletionItem(completionAt(nil, file, src, cursor), "not_found")
	if !ok {
		t.Fatal("errors-set completion missing not_found")
	}

	edit, ok := item.TextEdit.(*protocol.TextEdit)
	if !ok {
		t.Fatalf("not_found TextEdit = %T, want *protocol.TextEdit", item.TextEdit)
	}

	wantRange := protocol.Range{
		Start: protocol.Position{Line: 2, Character: cursor.Character - 4},
		End:   cursor,
	}

	if edit.Range != wantRange {
		t.Errorf("not_found TextEdit range = %+v, want %+v", edit.Range, wantRange)
	}

	if edit.NewText != "not_found" {
		t.Errorf("not_found TextEdit NewText = %q, want the plain label", edit.NewText)
	}

	topSrc := "ent"

	topFile, _ := parser.New("t.zen", []byte(topSrc)).ParseFile()
	topCursor := protocol.Position{Line: 0, Character: 3}

	item, ok = findCompletionItem(completionAtWithCaps(nil, topFile, topSrc, topCursor, snippetCaps()), "entity")
	if !ok {
		t.Fatal("top-level completion missing entity")
	}

	edit, ok = item.TextEdit.(*protocol.TextEdit)
	if !ok {
		t.Fatalf("entity TextEdit = %T, want *protocol.TextEdit", item.TextEdit)
	}

	wantTopRange := protocol.Range{
		Start: protocol.Position{Line: 0, Character: 0},
		End:   topCursor,
	}

	if edit.Range != wantTopRange {
		t.Errorf("entity TextEdit range = %+v, want %+v", edit.Range, wantTopRange)
	}

	const snippet = "entity ${1:Name} {\n\t$0\n}"

	if edit.NewText != snippet {
		t.Errorf("entity TextEdit NewText = %q, want the snippet %q", edit.NewText, snippet)
	}
}

// TestCompletionNoTextEditWithoutPrefix proves items offered with nothing
// typed carry no TextEdit: there is no partial word to replace.
func TestCompletionNoTextEditWithoutPrefix(t *testing.T) {
	src := ""

	file, _ := parser.New("empty.zen", []byte(src)).ParseFile()

	for _, caps := range []clientCaps{{}, snippetCaps()} {
		for _, item := range completionAtWithCaps(nil, file, src, protocol.Position{Line: 0, Character: 0}, caps) {
			if item.TextEdit != nil {
				t.Errorf("%s TextEdit is present with no prefix typed, want nil (caps %+v)", item.Label, caps)
			}
		}
	}
}

// TestPrefixRange pins the prefix-covering range computation, including the
// clamp when the prefix outruns the cursor column.
func TestPrefixRange(t *testing.T) {
	tests := []struct {
		name   string
		cursor protocol.Position
		prefix string
		want   protocol.Range
	}{
		{
			name:   "covers the typed word",
			cursor: protocol.Position{Line: 2, Character: 16},
			prefix: "not_",
			want: protocol.Range{
				Start: protocol.Position{Line: 2, Character: 12},
				End:   protocol.Position{Line: 2, Character: 16},
			},
		},
		{
			name:   "empty prefix is a caret range",
			cursor: protocol.Position{Line: 0, Character: 0},
			prefix: "",
			want: protocol.Range{
				Start: protocol.Position{Line: 0, Character: 0},
				End:   protocol.Position{Line: 0, Character: 0},
			},
		},
		{
			name:   "overlong prefix clamps to the line start",
			cursor: protocol.Position{Line: 1, Character: 1},
			prefix: "toolong",
			want: protocol.Range{
				Start: protocol.Position{Line: 1, Character: 0},
				End:   protocol.Position{Line: 1, Character: 1},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := prefixRange(tc.cursor, tc.prefix); got != tc.want {
				t.Errorf("prefixRange(%+v, %q) = %+v, want %+v", tc.cursor, tc.prefix, got, tc.want)
			}
		})
	}
}

// TestCompletionHonorsNegotiatedSnippetSupport is the end-to-end contract:
// a client that negotiates snippetSupport at Initialize gets snippet items
// (with prefix-replacing edits) out of textDocument/completion, while a
// client that negotiates nothing gets plain-text labels.
func TestCompletionHonorsNegotiatedSnippetSupport(t *testing.T) {
	const docURI = "file:///tmp/zever-lsp-snippet.zen"

	newServerWithSnippet := func(t *testing.T, snippet *bool) *Server {
		t.Helper()

		s := NewServer()

		var item *protocol.ClientCompletionItemOptions
		if snippet != nil {
			item = &protocol.ClientCompletionItemOptions{SnippetSupport: snippet}
		}

		params := &protocol.InitializeParams{
			Capabilities: protocol.ClientCapabilities{
				TextDocument: &protocol.TextDocumentClientCapabilities{
					Completion: &protocol.CompletionClientCapabilities{
						CompletionItem: item,
					},
				},
			},
		}

		if _, err := s.Initialize(t.Context(), params); err != nil {
			t.Fatalf("Initialize: %v", err)
		}

		return s
	}

	completeEntity := func(t *testing.T, s *Server) protocol.CompletionItem {
		t.Helper()

		openDoc(t, s, docURI, "ent")

		res, err := s.Completion(t.Context(), &protocol.CompletionParams{
			TextDocumentPositionParams: protocol.TextDocumentPositionParams{
				TextDocument: protocol.TextDocumentIdentifier{URI: uri.URI(docURI)},
				Position:     protocol.Position{Line: 0, Character: 3},
			},
		})
		if err != nil {
			t.Fatalf("Completion: %v", err)
		}

		slice, ok := res.(protocol.CompletionItemSlice)
		if !ok {
			t.Fatalf("Completion result = %T, want CompletionItemSlice", res)
		}

		item, ok := findCompletionItem(slice, "entity")
		if !ok {
			t.Fatalf("completion missing entity, got %v", completionLabels(slice))
		}

		return item
	}

	t.Run("snippet client gets the skeleton", func(t *testing.T) {
		snippet := true

		item := completeEntity(t, newServerWithSnippet(t, &snippet))

		const want = "entity ${1:Name} {\n\t$0\n}"

		if got, _ := item.InsertText.Get(); got != want {
			t.Errorf("entity InsertText = %q, want snippet %q", got, want)
		}

		if item.InsertTextFormat != protocol.InsertTextFormatSnippet {
			t.Errorf("entity InsertTextFormat = %v, want Snippet", item.InsertTextFormat)
		}

		edit, ok := item.TextEdit.(*protocol.TextEdit)
		if !ok {
			t.Fatalf("entity TextEdit = %T, want *protocol.TextEdit", item.TextEdit)
		}

		wantRange := protocol.Range{
			Start: protocol.Position{Line: 0, Character: 0},
			End:   protocol.Position{Line: 0, Character: 3},
		}

		if edit.Range != wantRange {
			t.Errorf("entity TextEdit range = %+v, want %+v", edit.Range, wantRange)
		}
	})

	t.Run("plain client gets the label", func(t *testing.T) {
		item := completeEntity(t, newServerWithSnippet(t, nil))

		if got, _ := item.InsertText.Get(); got != "entity" {
			t.Errorf("entity InsertText = %q, want the plain label", got)
		}

		if item.InsertTextFormat != protocol.InsertTextFormatPlainText {
			t.Errorf("entity InsertTextFormat = %v, want PlainText", item.InsertTextFormat)
		}
	})
}
