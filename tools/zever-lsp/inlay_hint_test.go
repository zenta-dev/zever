package main

import (
	"strings"
	"testing"

	"go.lsp.dev/protocol"

	"github.com/zenta-dev/zever/dsl/ast"
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

// paramNameSrc declares a two-parameter job and a schedule dispatching it
// with two positional arguments -- the exact shape paramNameHints annotates.
const paramNameSrc = "job Send(to: string, subject: string) {\n" +
	"\tqueue: default\n" +
	"}\n" +
	"\n" +
	"schedule Nightly {\n" +
	"\tcron: \"0 0 * * *\"\n" +
	"\tdispatch: Send(\"a\", \"b\")\n" +
	"}\n"

// paramNameFixture compiles paramNameSrc and returns the schema plus the
// parsed schedule file the hints are computed from.
func paramNameFixture(t *testing.T) (*ir.Schema, *ast.File) {
	t.Helper()

	result, diags := compile.Compile(map[string]string{"schema/app.zen": paramNameSrc})
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	file, _ := parser.New("schema/app.zen", []byte(paramNameSrc)).ParseFile()

	return result.Schema, file
}

// TestParamNameHintsAppear proves a positional dispatch call is annotated
// with one `name:` hint per argument, in parameter order, anchored at each
// argument's start and typed as a parameter hint.
func TestParamNameHintsAppear(t *testing.T) {
	schema, file := paramNameFixture(t)

	hints := paramNameHints(schema, file)
	if len(hints) != 2 {
		t.Fatalf("got %d param-name hints, want 2: %+v", len(hints), hints)
	}

	for i, want := range []string{"to:", "subject:"} {
		if got := inlayLabel(t, hints[i]); got != want {
			t.Errorf("hint %d label = %q, want %q", i, got, want)
		}

		if hints[i].Kind != protocol.InlayHintKindParameter {
			t.Errorf("hint %d kind = %d, want parameter (%d)", i, hints[i].Kind, protocol.InlayHintKindParameter)
		}
	}

	// Both hints sit on the dispatch line, at the start of each argument:
	// \tdispatch: Send("a", "b").
	lines := strings.Split(paramNameSrc, "\n")

	var dispatchLine int

	for i, line := range lines {
		if strings.Contains(line, "dispatch: Send(") {
			dispatchLine = i
		}
	}

	for i, arg := range []string{`"a"`, `"b"`} {
		wantChar := strings.Index(lines[dispatchLine], arg)

		//nolint:gosec // test fixture offsets are non-negative
		if hints[i].Position.Line != uint32(dispatchLine) ||
			hints[i].Position.Character != uint32(wantChar) {
			t.Errorf("hint %d position = %+v, want line %d char %d (start of %s)",
				i, hints[i].Position, dispatchLine, wantChar, arg)
		}
	}
}

// TestParamNameHintsSkipNamed proves an argument that already carries a
// `name:` label is never annotated.
func TestParamNameHintsSkipNamed(t *testing.T) {
	src := "job Send(to: string, subject: string) {\n" +
		"\tqueue: default\n" +
		"}\n" +
		"\n" +
		"schedule Nightly {\n" +
		"\tcron: \"0 0 * * *\"\n" +
		"\tdispatch: Send(to: \"a\", subject: \"b\")\n" +
		"}\n"

	result, diags := compile.Compile(map[string]string{"schema/app.zen": src})
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	file, _ := parser.New("schema/app.zen", []byte(src)).ParseFile()

	if hints := paramNameHints(result.Schema, file); len(hints) != 0 {
		t.Errorf("named arguments produced %d hints, want 0: %+v", len(hints), hints)
	}
}

// TestParamNameHintsSkipSingleIdentMatch proves the same-name heuristic: a
// single-argument call whose value identifier already names the parameter --
// case-insensitively -- gets no hint, since the name is already visible.
func TestParamNameHintsSkipSingleIdentMatch(t *testing.T) {
	tests := []struct {
		name     string
		dispatch string
	}{
		{"exact match", "Send(user)"},
		{"case-insensitive match", "Send(USER)"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			src := "job Send(user: string) {\n" +
				"\tqueue: default\n" +
				"}\n" +
				"\n" +
				"schedule Nightly {\n" +
				"\tcron: \"0 0 * * *\"\n" +
				"\tdispatch: " + tc.dispatch + "\n" +
				"}\n"

			result, diags := compile.Compile(map[string]string{"schema/app.zen": src})
			if diags.HasErrors() {
				t.Fatalf("unexpected diagnostics: %v", diags)
			}

			file, _ := parser.New("schema/app.zen", []byte(src)).ParseFile()

			if hints := paramNameHints(result.Schema, file); len(hints) != 0 {
				t.Errorf("same-name call produced %d hints, want 0: %+v", len(hints), hints)
			}
		})
	}
}

// TestParamNameHintsSingleIdentMismatch proves the heuristic only fires on
// a match: a differently-named identifier still gets its hint.
func TestParamNameHintsSingleIdentMismatch(t *testing.T) {
	src := "job Send(user: string) {\n" +
		"\tqueue: default\n" +
		"}\n" +
		"\n" +
		"schedule Nightly {\n" +
		"\tcron: \"0 0 * * *\"\n" +
		"\tdispatch: Send(admin)\n" +
		"}\n"

	result, diags := compile.Compile(map[string]string{"schema/app.zen": src})
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	file, _ := parser.New("schema/app.zen", []byte(src)).ParseFile()

	hints := paramNameHints(result.Schema, file)
	if len(hints) != 1 {
		t.Fatalf("got %d hints, want 1: %+v", len(hints), hints)
	}

	if got := inlayLabel(t, hints[0]); got != "user:" {
		t.Errorf("hint label = %q, want %q", got, "user:")
	}
}

// TestParamNameHintsSettingOff proves the family is gated: default settings
// (paramNames off) yield no param-name hints on a call that would otherwise
// get them, while an explicit opt-in does.
func TestParamNameHintsSettingOff(t *testing.T) {
	schema, file := paramNameFixture(t)

	if hints := inlayHintsFor(schema, file, defaultSettings()); len(hints) != 0 {
		t.Errorf("default settings produced %d hints, want 0 (paramNames off): %+v", len(hints), hints)
	}

	on := defaultSettings()
	on.ParamNames = true

	hints := inlayHintsFor(schema, file, on)
	if len(hints) != 2 {
		t.Errorf("paramNames on produced %d hints, want 2: %+v", len(hints), hints)
	}
}

// TestParamNameHintsNilSchemaOrFile proves a missing schema (unresolved
// workspace) or file yields no hints rather than guesses.
func TestParamNameHintsNilSchemaOrFile(t *testing.T) {
	_, file := paramNameFixture(t)

	if hints := paramNameHints(nil, file); hints != nil {
		t.Errorf("nil schema produced %+v, want nil", hints)
	}

	schema, _ := paramNameFixture(t)

	if hints := paramNameHints(schema, nil); hints != nil {
		t.Errorf("nil file produced %+v, want nil", hints)
	}
}

// TestParamNameHintsUnknownJob proves a dispatch naming no declared job is
// skipped: there are no resolved parameter names to show.
func TestParamNameHintsUnknownJob(t *testing.T) {
	schema, _ := paramNameFixture(t)

	file, _ := parser.New("schema/app.zen", []byte(
		"schedule Nightly {\n\tcron: \"0 0 * * *\"\n\tdispatch: Missing(\"a\")\n}\n")).ParseFile()

	if hints := paramNameHints(schema, file); len(hints) != 0 {
		t.Errorf("unknown job produced %d hints, want 0: %+v", len(hints), hints)
	}
}

// TestWalkDispatchCallsSkipsNonCall proves a dispatch that is not a call
// (mid-edit) contributes no call to annotate.
func TestWalkDispatchCallsSkipsNonCall(t *testing.T) {
	file, _ := parser.New("schema/app.zen", []byte(
		"schedule Nightly {\n\tcron: \"0 0 * * *\"\n\tdispatch: Missing\n}\n")).ParseFile()

	calls := 0

	walkDispatchCalls(file, func(_ *ast.CallValue) { calls++ })

	if calls != 0 {
		t.Errorf("non-call dispatch produced %d calls, want 0", calls)
	}

	calls = 0

	walkDispatchCalls(nil, func(_ *ast.CallValue) { calls++ })

	if calls != 0 {
		t.Errorf("nil file produced %d calls, want 0", calls)
	}
}

// TestParamNameHintsDefaultOff pins the default: parameter-name hints are
// noise unless explicitly asked for.
func TestParamNameHintsDefaultOff(t *testing.T) {
	if defaultSettings().ParamNames {
		t.Errorf("defaultSettings().ParamNames = true, want false")
	}
}
