package main

import (
	"testing"

	"go.lsp.dev/protocol"

	"github.com/zenta-dev/zever/dsl/compile"
	"github.com/zenta-dev/zever/dsl/diag"
	"github.com/zenta-dev/zever/dsl/ir"
	"github.com/zenta-dev/zever/dsl/parser"
)

// inlayLabel extracts a hint's label text, asserting the native union arm.
func inlayLabel(t *testing.T, h protocol.InlayHint) string {
	t.Helper()

	s, ok := h.Label.(protocol.String)
	if !ok {
		t.Fatalf("inlay hint label = %T, want protocol.String", h.Label)
	}

	return string(s)
}

// TestInlayHintImplicitModule builds a two-module schema (one public file
// directly under schema/, one file under schema/billing/) and checks that
// only the declaration resolving to the implicit public module gets the
// "(module: public)" hint; the one resolving to the named "billing" module
// gets none, per the task's "confirm no hint appears where none should"
// case.
func TestInlayHintImplicitModule(t *testing.T) {
	publicSrc := "entity User {\n" +
		"\tid: uuid @primary\n" +
		"}\n" +
		"\n" +
		"message Ping {\n" +
		"\tid: uuid\n" +
		"}\n"

	billingSrc := "entity Invoice {\n" +
		"\tid: uuid @primary\n" +
		"}\n"

	result, diags := compile.Compile(map[string]string{
		"schema/app.zen":             publicSrc,
		"schema/billing/invoice.zen": billingSrc,
	})
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	publicFile, _ := parser.New("schema/app.zen", []byte(publicSrc)).ParseFile()

	hints := inlayHintsAt(result.Schema, publicFile)
	if len(hints) != 2 {
		t.Fatalf("public file: got %d hints, want 2 (entity + message): %+v", len(hints), hints)
	}

	for _, h := range hints {
		if inlayLabel(t, h) != " (module: public)" {
			t.Errorf("hint label = %q, want %q", inlayLabel(t, h), " (module: public)")
		}
	}

	// User's name is "User" (4 chars) starting at line 0 col 8 ("entity ").
	if hints[0].Position.Line != 0 {
		t.Errorf("User hint line = %d, want 0", hints[0].Position.Line)
	}

	billingFile, _ := parser.New("schema/billing/invoice.zen", []byte(billingSrc)).ParseFile()

	billingHints := inlayHintsAt(result.Schema, billingFile)
	if len(billingHints) != 0 {
		t.Errorf("billing file (named module): got %d hints, want 0: %+v", len(billingHints), billingHints)
	}
}

// TestInlayHintNoHintInAllPublicSchema confirms the noise-avoidance rule: a
// schema with only the implicit public module (the common, flat-schema
// case -- every example schema in this repo is laid out this way) gets no
// hints at all, since "module: public" on every single declaration would be
// noise, not information.
func TestInlayHintNoHintInAllPublicSchema(t *testing.T) {
	src := "entity User {\n" +
		"\tid: uuid @primary\n" +
		"}\n"

	result, diags := compile.Compile(map[string]string{"schema/app.zen": src})
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	file, _ := parser.New("schema/app.zen", []byte(src)).ParseFile()

	hints := inlayHintsAt(result.Schema, file)
	if len(hints) != 0 {
		t.Errorf("all-public schema: got %d hints, want 0: %+v", len(hints), hints)
	}
}

// TestInlayHintService checks the service case alongside entity/message.
func TestInlayHintService(t *testing.T) {
	publicSrc := "service Accounts {\n" +
		"\trpc Ping() -> Pong {\n" +
		"\t\tauth: none\n" +
		"\t}\n" +
		"}\n" +
		"\n" +
		"message Pong {\n" +
		"\tid: uuid\n" +
		"}\n"

	billingSrc := "entity Invoice {\n" +
		"\tid: uuid @primary\n" +
		"}\n"

	result, diags := compile.Compile(map[string]string{
		"schema/app.zen":             publicSrc,
		"schema/billing/invoice.zen": billingSrc,
	})
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	publicFile, _ := parser.New("schema/app.zen", []byte(publicSrc)).ParseFile()

	hints := inlayHintsAt(result.Schema, publicFile)

	found := false

	for _, h := range hints {
		if inlayLabel(t, h) == " (module: public)" {
			found = true
		}
	}

	if !found {
		t.Errorf("expected a public-module hint among %+v", hints)
	}

	if len(hints) != 2 {
		t.Errorf("got %d hints, want 2 (service + message)", len(hints))
	}
}

func TestInlayHintNilSchema(t *testing.T) {
	src := "entity User {\n\tid: uuid @primary\n}\n"
	file, _ := parser.New("schema/app.zen", []byte(src)).ParseFile()

	if hints := inlayHintsAt(nil, file); hints != nil {
		t.Errorf("nil schema: got %+v, want nil", hints)
	}

	if hints := inlayHintsAt(multiModuleSchema(t), nil); hints != nil {
		t.Errorf("nil file: got %+v, want nil", hints)
	}
}

// multiModuleSchema compiles a trivial multi-module schema, for
// TestInlayHintNilSchema's nil-file case where any non-nil schema with a
// named module (so the noise-avoidance guard doesn't itself short-circuit
// the nil-file check) will do.
func multiModuleSchema(t *testing.T) *ir.Schema {
	t.Helper()

	r, diags := compile.Compile(map[string]string{
		"schema/app.zen":             "entity User {\n\tid: uuid @primary\n}\n",
		"schema/billing/invoice.zen": "entity Invoice {\n\tid: uuid @primary\n}\n",
	})
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	return r.Schema
}

// errorCaseInlaySrc declares an rpc with two error cases (one recognized,
// one not) in a flat, single-(implicit-public)-module schema -- exactly the
// case that inlayHintsAt's old "bail out unless a named module exists"
// check wrongly suppressed before it was split into an independent,
// unconditional pass.
const errorCaseInlaySrc = "entity Task {\n" +
	"\tid: uuid @primary\n" +
	"}\n" +
	"\n" +
	"service TaskService {\n" +
	"\trpc GetTask(id: uuid) -> Task {\n" +
	"\t\tauth: none\n" +
	"\t\terrors: { not_found, bogus_code }\n" +
	"\t}\n" +
	"}\n"

// TestInlayHintErrorCaseInFlatSchema proves an error-case hint appears even
// when schemaHasNamedModule is false (a single, implicit-public-module
// schema) -- the module-name hint pass is gated on that, error-case hints
// must not be.
func TestInlayHintErrorCaseInFlatSchema(t *testing.T) {
	src := "entity Task {\n" +
		"\tid: uuid @primary\n" +
		"}\n" +
		"\n" +
		"service TaskService {\n" +
		"\trpc GetTask(id: uuid) -> Task {\n" +
		"\t\tauth: none\n" +
		"\t\terrors: { not_found }\n" +
		"\t}\n" +
		"}\n"

	result, diags := compile.Compile(map[string]string{"schema/app.zen": src})
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	if schemaHasNamedModule(result.Schema) {
		t.Fatal("test fixture must resolve to only the implicit public module")
	}

	file, _ := parser.New("schema/app.zen", []byte(src)).ParseFile()

	hints := inlayHintsAt(result.Schema, file)

	var found *protocol.InlayHint

	for i := range hints {
		if inlayLabel(t, hints[i]) == " (404 NOT_FOUND)" {
			found = &hints[i]
		}
	}

	if found == nil {
		t.Fatalf("expected a %q hint among %+v", " (404 NOT_FOUND)", hints)
	}
}

// TestInlayHintSkipsUnrecognizedErrorCase proves a name the resolver would
// reject produces no hint (no crash, just skipped) -- unlike
// TestInlayHintErrorCaseInFlatSchema this uses a standalone parse (no
// compile.Compile) since an unknown error code is a diagnostics error.
func TestInlayHintSkipsUnrecognizedErrorCase(t *testing.T) {
	file, _ := parser.New("schema/app.zen", []byte(errorCaseInlaySrc)).ParseFile()

	hints := errorCaseHints(file)

	if len(hints) != 1 {
		t.Fatalf("got %d error-case hints, want exactly 1 (bogus_code skipped): %+v", len(hints), hints)
	}

	if inlayLabel(t, hints[0]) != " (404 NOT_FOUND)" {
		t.Errorf("hint label = %q, want %q", inlayLabel(t, hints[0]), " (404 NOT_FOUND)")
	}
}

// TestInlayHintPositionsFollowIdentifiers proves every hint renders
// immediately after the identifier it annotates, in 0-based LSP
// coordinates -- the contract the textDocument/inlayHint handler relies on.
func TestInlayHintPositionsFollowIdentifiers(t *testing.T) {
	file, _ := parser.New("schema/app.zen", []byte(errorCaseInlaySrc)).ParseFile()

	hints := errorCaseHints(file)
	if len(hints) != 1 {
		t.Fatalf("got %d error-case hints, want 1: %+v", len(hints), hints)
	}

	// `not_found` sits on line 7 (0-based) starting at column 12
	// ("\t\terrors: { " is 2 tabs + 10 chars); the hint follows the
	// 9-character name.
	if hints[0].Position.Line != 7 {
		t.Errorf("hint line = %d, want 7", hints[0].Position.Line)
	}

	if hints[0].Position.Character != 12+9 {
		t.Errorf("hint character = %d, want %d (just after not_found)", hints[0].Position.Character, 12+9)
	}
}

// TestModuleLookups_condition_expected proves the module lookups report
// absent (never crash) for a nil schema, an empty name, and a name no
// module owns.
func TestModuleLookups_condition_expected(t *testing.T) {
	schema := multiModuleSchema(t)

	tests := []struct {
		name string
		got  *ir.Module
	}{
		{"entity nil schema", moduleOfEntity(nil, "User")},
		{"entity empty name", moduleOfEntity(schema, "")},
		{"entity unknown name", moduleOfEntity(schema, "Missing")},
		{"message nil schema", moduleOfMessage(nil, "Ping")},
		{"message empty name", moduleOfMessage(schema, "")},
		{"message unknown name", moduleOfMessage(schema, "Missing")},
		{"service nil schema", moduleOfService(nil, "Accounts")},
		{"service empty name", moduleOfService(schema, "")},
		{"service unknown name", moduleOfService(schema, "Missing")},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.got != nil {
				t.Errorf("lookup returned %+v, want nil", tc.got)
			}
		})
	}
}

// TestImplicitModuleHint_condition_expected pins both gates: a nil module
// and a named module produce nothing, the public sentinel produces the hint.
func TestImplicitModuleHint_condition_expected(t *testing.T) {
	schema := multiModuleSchema(t)

	if _, ok := implicitModuleHint(nil, diag.Position{Line: 1, Col: 8}, "User"); ok {
		t.Errorf("nil module produced a hint, want none")
	}

	var named *ir.Module

	for _, module := range schema.Modules {
		if module.Name != "" {
			named = module
		}
	}

	if named == nil {
		t.Fatal("fixture has no named module")
	}

	if _, ok := implicitModuleHint(named, diag.Position{Line: 1, Col: 8}, "Invoice"); ok {
		t.Errorf("named module produced a hint, want none")
	}

	var public *ir.Module

	for _, module := range schema.Modules {
		if module.Name == "" {
			public = module
		}
	}

	if public == nil {
		t.Fatal("fixture has no public module")
	}

	hint, ok := implicitModuleHint(public, diag.Position{Line: 1, Col: 8}, "User")
	if !ok {
		t.Fatal("public module produced no hint, want one")
	}

	if inlayLabel(t, hint) != " (module: public)" {
		t.Errorf("hint label = %q, want %q", inlayLabel(t, hint), " (module: public)")
	}
}
