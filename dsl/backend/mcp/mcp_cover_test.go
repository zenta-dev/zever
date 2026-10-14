package mcp

import (
	"strings"
	"testing"

	"github.com/zenta-dev/zever/dsl/ir"
)

// TestBackendName pins the backend identifier.
func TestBackendName(t *testing.T) {
	t.Parallel()

	if got := New().Name(); got != "mcp" {
		t.Fatalf("Name() = %q, want mcp", got)
	}
}

// TestModuleLabelImplicit pins the unnamed-module manifest label.
func TestModuleLabelImplicit(t *testing.T) {
	t.Parallel()

	if got := moduleLabel(&ir.Module{}); got != "default" {
		t.Fatalf("moduleLabel(empty) = %q, want default", got)
	}

	if got := moduleLabel(&ir.Module{Name: "blog"}); got != "blog" {
		t.Fatalf("moduleLabel(blog) = %q, want blog", got)
	}
}

// TestScalarJSONTypeExhaustive pins the scalar-to-JSON-Schema mapping for
// every scalar, including the TJSON fallback (unreached through
// fieldSchema, which short-circuits TJSON first).
func TestScalarJSONTypeExhaustive(t *testing.T) {
	t.Parallel()

	cases := map[ir.ScalarType]string{
		ir.TUUID:           "string",
		ir.TString:         "string",
		ir.TTimestamp:      "string",
		ir.TDate:           "string",
		ir.TBytes:          "string",
		ir.TInt32:          "integer",
		ir.TInt64:          "integer",
		ir.TFloat32:        "number",
		ir.TFloat64:        "number",
		ir.TBool:           "boolean",
		ir.TEnum:           "string",
		ir.TJSON:           "string",
		ir.ScalarType(999): "string",
	}

	for scalar, want := range cases {
		if got := scalarJSONType(scalar); got != want {
			t.Errorf("scalarJSONType(%d) = %q, want %q", int(scalar), got, want)
		}
	}
}

// TestGoIdentSanitizes pins handler-name sanitizing for tool names with
// non-identifier runes (only handler symbols use it; tool names stay raw).
func TestGoIdentSanitizes(t *testing.T) {
	t.Parallel()

	if got := goIdent("blog_UserService_GetUser"); got != "blog_UserService_GetUser" {
		t.Fatalf("goIdent(clean) = %q, want unchanged", got)
	}

	if got := goIdent("a-b.c/d:e"); got != "a_b_c_d_e" {
		t.Fatalf("goIdent(dirty) = %q, want a_b_c_d_e", got)
	}
}

// TestGenerateImplicitModule pins the default/ label and key for a schema
// laid out without a module directory.
func TestGenerateImplicitModule(t *testing.T) {
	t.Parallel()

	schema := mustResolve(t, map[string]string{"schema/app.zen": "entity Item {\n\tid: uuid @primary\n}"})
	out := mustGenerate(t, schema)

	if _, ok := out["default/mcp.json"]; !ok {
		t.Fatalf("missing default/mcp.json, got keys %v", keysOf(out))
	}
}

// coverModule builds a hand-rolled module exercising manifest paths the
// .zen fixtures never reach: message returns, inline enums, TJSON fields,
// ref-typed fields, and refs without an owning module (fallback branch).
func coverModule() *ir.Module {
	m := &ir.Module{Name: "m"}

	msg := &ir.Message{
		Name:   "Msg",
		Module: m,
		Fields: []*ir.Field{
			{Name: "raw", Type: ir.FieldType{Scalar: ir.TJSON}},
			{Name: "kind", Type: ir.FieldType{Scalar: ir.TEnum, EnumValues: []string{"a", "b"}}},
			{Name: "count", Type: ir.FieldType{Scalar: ir.TInt32}},
		},
	}

	ent := &ir.Entity{
		Name:   "Ent",
		Module: m,
		Fields: []*ir.Field{
			{Name: "id", Type: ir.FieldType{Scalar: ir.TUUID}, Primary: true},
			{Name: "maybe", Type: ir.FieldType{Scalar: ir.TString}, Primary: true, Optional: true},
			{Name: "peer", Ref: &ir.TypeRef{Entity: &ir.Entity{Name: "Other"}}},
		},
	}

	svc := &ir.Service{
		Name:   "Svc",
		Module: m,
		Operations: []*ir.Operation{
			{Name: "GetMsg", Returns: &ir.TypeRef{Message: msg}},
			{Name: "GetEnt", Returns: &ir.TypeRef{Entity: ent}},
			{
				Name:   "Echo",
				Params: []*ir.Param{{Name: "in", Ref: &ir.TypeRef{Message: msg}}},
			},
		},
	}
	m.Messages = []*ir.Message{msg}
	m.Entities = []*ir.Entity{ent}
	m.Services = []*ir.Service{svc}

	return m
}

// TestGenerateMessageAndRefPaths covers messageSchema (inline enum, TJSON,
// scalar params), ref-typed entity fields, message params, and the
// ownerModule fallback for refs carrying no module.
func TestGenerateMessageAndRefPaths(t *testing.T) {
	t.Parallel()

	schema := &ir.Schema{Modules: []*ir.Module{coverModule()}}
	out := mustGenerate(t, schema)

	raw, ok := out["m/mcp.json"]
	if !ok {
		t.Fatalf("missing m/mcp.json, got keys %v", keysOf(out))
	}

	doc := decodeManifest(t, raw)
	services, ok := doc["services"].([]any)
	if !ok || len(services) != 1 {
		t.Fatalf("services = %v, want one", doc["services"])
	}

	svc, ok := services[0].(map[string]any)
	if !ok {
		t.Fatalf("service type = %T", services[0])
	}

	defs, ok := svc["definitions"].(map[string]any)
	if !ok {
		t.Fatalf("definitions type = %T", svc["definitions"])
	}

	for _, want := range []string{"m_Msg", "m_Ent", "m_Other"} {
		if _, has := defs[want].(map[string]any); !has {
			t.Errorf("definitions missing %q: %v", want, keysOfAny(defs))
		}
	}

	msgDef, ok := defs["m_Msg"].(map[string]any)
	if !ok {
		t.Fatalf("definitions missing m_Msg: %v", keysOfAny(defs))
	}

	props, ok := msgDef["properties"].(map[string]any)
	if !ok {
		t.Fatalf("m_Msg properties type = %T", msgDef["properties"])
	}

	kind, ok := props["kind"].(map[string]any)
	if !ok {
		t.Fatalf("kind schema type = %T", props["kind"])
	}

	if enum, ok := kind["enum"].([]any); !ok || len(enum) != 2 {
		t.Fatalf("inline enum schema = %v, want two values", props["kind"])
	}
}

// TestGenerateUnknownEnum proves an unresolvable named enum fails closed
// with the invariant error instead of emitting a dangling $ref.
func TestGenerateUnknownEnum(t *testing.T) {
	t.Parallel()

	m := &ir.Module{Name: "m"}
	ent := &ir.Entity{
		Name:   "Ent",
		Module: m,
		Fields: []*ir.Field{
			{Name: "role", Type: ir.FieldType{Scalar: ir.TEnum, EnumName: "Missing"}},
		},
	}
	m.Entities = []*ir.Entity{ent}
	m.Services = []*ir.Service{{
		Name:       "Svc",
		Module:     m,
		Operations: []*ir.Operation{{Name: "Get", Returns: &ir.TypeRef{Entity: ent}}},
	}}

	if _, err := New().Generate(&ir.Schema{Modules: []*ir.Module{m}}); err == nil {
		t.Fatal("Generate with unknown enum: got nil error, want invariant failure")
	} else if !strings.Contains(err.Error(), "unknown enum") {
		t.Fatalf("error = %q, want unknown enum", err)
	}
}

// TestGenerateEmptyTypeRef proves an operation returning neither an entity
// nor a message fails closed.
func TestGenerateEmptyTypeRef(t *testing.T) {
	t.Parallel()

	m := &ir.Module{Name: "m"}
	m.Services = []*ir.Service{{
		Name:       "Svc",
		Module:     m,
		Operations: []*ir.Operation{{Name: "Get", Returns: &ir.TypeRef{}}},
	}}

	if _, err := New().Generate(&ir.Schema{Modules: []*ir.Module{m}}); err == nil {
		t.Fatal("Generate with empty type ref: got nil error, want invariant failure")
	} else if !strings.Contains(err.Error(), "empty type reference") {
		t.Fatalf("error = %q, want empty type reference", err)
	}
}

// TestGenerateNilTypeRefParam proves a nil param ref fails closed.
func TestGenerateNilTypeRefParam(t *testing.T) {
	t.Parallel()

	m := &ir.Module{Name: "m"}
	m.Services = []*ir.Service{{
		Name:   "Svc",
		Module: m,
		Operations: []*ir.Operation{{
			Name:   "Echo",
			Params: []*ir.Param{{Name: "in", Ref: nil, Type: ir.FieldType{Scalar: ir.TJSON}}},
		}},
	}}

	out := mustGenerate(t, &ir.Schema{Modules: []*ir.Module{m}})
	if _, ok := out["m/mcp.json"]; !ok {
		t.Fatalf("missing m/mcp.json, got keys %v", keysOf(out))
	}
}

// TestMarshalServicesRoundTrip pins the embed path: compact JSON without
// HTML escaping, decoding back to the same services.
func TestMarshalServicesRoundTrip(t *testing.T) {
	t.Parallel()

	services := []serviceManifest{{
		Service: "Svc",
		Module:  "m",
		Tools: []toolManifest{{
			Name:        "m_Svc_Op",
			Description: "a <b>bold</b> op",
			InputSchema: map[string]any{"type": "object"},
		}},
	}}

	raw, err := marshalServices(services)
	if err != nil {
		t.Fatalf("marshalServices: %v", err)
	}

	if !strings.Contains(raw, "<b>bold</b>") {
		t.Fatalf("marshalServices escaped HTML: %q", raw)
	}

	rendered, err := renderServerModule(services)
	if err != nil {
		t.Fatalf("renderServerModule: %v", err)
	}

	if !strings.Contains(string(rendered), `"m_Svc_Op":`) {
		t.Fatalf("rendered server lacks the tool dispatch entry")
	}
}

// TestCollectToolNamesSorted pins deterministic handler ordering across
// services.
func TestCollectToolNamesSorted(t *testing.T) {
	t.Parallel()

	services := []serviceManifest{
		{Service: "B", Tools: []toolManifest{{Name: "b"}, {Name: "a"}}},
		{Service: "A", Tools: []toolManifest{{Name: "c"}}},
	}

	got := collectToolNames(services)
	want := []string{"a", "b", "c"}
	if len(got) != len(want) {
		t.Fatalf("collectToolNames = %v, want %v", got, want)
	}

	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("collectToolNames = %v, want %v", got, want)
		}
	}
}
