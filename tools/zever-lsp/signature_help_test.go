package main

import (
	"strings"
	"testing"

	"go.lsp.dev/protocol"
	"go.lsp.dev/uri"

	"github.com/zenta-dev/zever/dsl/compile"
	"github.com/zenta-dev/zever/dsl/ir"
)

// posAfter returns the LSP position immediately after marker's last
// occurrence in src, assuming src is a single line. It fails the test if
// marker is not found.
func posAfter(t *testing.T, src, marker string) protocol.Position {
	t.Helper()

	idx := strings.LastIndex(src, marker)
	if idx < 0 {
		t.Fatalf("marker %q not found in %q", marker, src)
	}

	return protocol.Position{Character: uint32(idx + len(marker))} // indexes non-negative
}

// paramIndex looks a parameter's declared index up by name, so test
// expectations read as "the roles parameter" rather than a magic number
// that silently goes stale if signatureSpecs is reordered.
func paramIndex(t *testing.T, call, name string) int {
	t.Helper()

	spec, ok := signatureSpecs[call]
	if !ok {
		t.Fatalf("no signature spec for %q", call)
	}

	if i := paramIndexByName(spec.params, name); i >= 0 {
		return i
	}

	t.Fatalf("call %q has no parameter named %q", call, name)

	return -1
}

// activeParamOf extracts the active parameter index from signature help,
// failing the test when none is set.
func activeParamOf(t *testing.T, got *protocol.SignatureHelp) int {
	t.Helper()

	if got == nil {
		t.Fatal("signatureHelpAt returned nil")
	}

	active, ok := got.ActiveParameter.Get()
	if !ok {
		t.Fatal("ActiveParameter is absent, want a parameter index")
	}

	return int(active)
}

func TestSignatureHelpNoneOutsideCall(t *testing.T) {
	for _, src := range []string{
		"entity User {",
		"",
		"id: uuid",
		"foo(bar",
	} {
		got := signatureHelpAt(src, protocol.Position{Character: uint32(len(src))}) // len never negative
		if got != nil {
			t.Errorf("signatureHelpAt(%q) = %+v, want nil", src, got)
		}
	}
}

func TestSignatureHelpValidate(t *testing.T) {
	src := `@validate(min_len: 1, max_len: 128, format: "email")`

	tests := []struct {
		name   string
		marker string
		want   int
	}{
		{"open paren, no args yet", "@validate(", paramIndex(t, "validate", "format")},
		{"typing the name before its colon (positional fallback)", "min_len: 1, ", paramIndex(t, "validate", "min_len")},
		{"named arg fully typed", "max_len: ", paramIndex(t, "validate", "max_len")},
		{"third named arg fully typed", "format: ", paramIndex(t, "validate", "format")},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cursor := posAfter(t, src, tc.marker)

			got := signatureHelpAt(src, cursor)
			if got == nil {
				t.Fatalf("signatureHelpAt returned nil at %q", tc.marker)
			}

			if len(got.Signatures) != 1 {
				t.Fatalf("Signatures = %d, want 1", len(got.Signatures))
			}

			wantLabel := "validate(format, min_len, max_len, gt, gte, lt, lte)"
			if got.Signatures[0].Label != wantLabel {
				t.Errorf("Label = %q, want %q", got.Signatures[0].Label, wantLabel)
			}

			if len(got.Signatures[0].Parameters) != 7 {
				t.Errorf("Parameters = %d, want 7", len(got.Signatures[0].Parameters))
			}

			if active := activeParamOf(t, got); active != tc.want {
				t.Errorf("ActiveParameter = %d, want %d", active, tc.want)
			}
		})
	}
}

func TestSignatureHelpRequiredRoles(t *testing.T) {
	src := `auth: required(roles: {owner, admin})`

	tests := []struct {
		name   string
		marker string
	}{
		{"open paren, no args yet", "required("},
		{"named arg fully typed", "roles: "},
		{"cursor inside the nested set literal", "roles: {owner, "},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cursor := posAfter(t, src, tc.marker)

			got := signatureHelpAt(src, cursor)
			if got == nil {
				t.Fatalf("signatureHelpAt returned nil at %q", tc.marker)
			}

			wantLabel := "required(roles)"
			if got.Signatures[0].Label != wantLabel {
				t.Errorf("Label = %q, want %q", got.Signatures[0].Label, wantLabel)
			}

			// roles is the only parameter, and the nested set literal's comma
			// must not be mistaken for a top-level argument separator.
			if active := activeParamOf(t, got); active != 0 {
				t.Errorf("ActiveParameter = %d, want 0", active)
			}
		})
	}
}

func TestSignatureHelpPermissionCheck(t *testing.T) {
	src := `permission: check("policy-name", resource: User, owner_field: user_id)`

	tests := []struct {
		name   string
		marker string
		want   int
	}{
		{"open paren, no args yet", "check(", paramIndex(t, "check", "policy")},
		{"after positional string, before resource typed", `"policy-name", `, paramIndex(t, "check", "resource")},
		{"resource named arg fully typed", "resource: ", paramIndex(t, "check", "resource")},
		{"owner_field named arg fully typed", "owner_field: ", paramIndex(t, "check", "owner_field")},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cursor := posAfter(t, src, tc.marker)

			got := signatureHelpAt(src, cursor)
			if got == nil {
				t.Fatalf("signatureHelpAt returned nil at %q", tc.marker)
			}

			if active := activeParamOf(t, got); active != tc.want {
				t.Errorf("ActiveParameter = %d, want %d", active, tc.want)
			}
		})
	}
}

func TestSignatureHelpJobRetry(t *testing.T) {
	t.Run("max_attempts", func(t *testing.T) {
		src := `retry: max_attempts(3)`
		cursor := posAfter(t, src, "max_attempts(")

		got := signatureHelpAt(src, cursor)
		if got == nil {
			t.Fatal("signatureHelpAt returned nil")
		}

		if got.Signatures[0].Label != "max_attempts(attempts)" {
			t.Errorf("Label = %q", got.Signatures[0].Label)
		}
	})

	t.Run("backoff positional kind then named base", func(t *testing.T) {
		src := `retry: backoff(exponential, base: 30s)`

		cursor := posAfter(t, src, "backoff(")

		got := signatureHelpAt(src, cursor)
		if got == nil {
			t.Fatal("signatureHelpAt returned nil")
		}

		if active := activeParamOf(t, got); active != 0 {
			t.Fatalf("at open paren: ActiveParameter = %d, want 0", active)
		}

		cursor = posAfter(t, src, "base: ")

		got = signatureHelpAt(src, cursor)
		if got == nil {
			t.Fatal("signatureHelpAt returned nil")
		}

		if active := activeParamOf(t, got); active != paramIndex(t, "backoff", "base") {
			t.Fatalf("at base:, ActiveParameter = %d", active)
		}
	})
}

func TestSignatureHelpFieldAttributes(t *testing.T) {
	t.Run("default", func(t *testing.T) {
		src := `@default(now())`
		cursor := posAfter(t, src, "@default(")

		got := signatureHelpAt(src, cursor)
		if got == nil {
			t.Fatal("signatureHelpAt returned nil")
		}

		if got.Signatures[0].Label != "default(value)" {
			t.Errorf("Label = %q", got.Signatures[0].Label)
		}
	})

	t.Run("renamed_from", func(t *testing.T) {
		src := `@renamed_from("old_col")`
		cursor := posAfter(t, src, "@renamed_from(")

		got := signatureHelpAt(src, cursor)
		if got == nil {
			t.Fatal("signatureHelpAt returned nil")
		}

		if got.Signatures[0].Label != "renamed_from(old_name)" {
			t.Errorf("Label = %q", got.Signatures[0].Label)
		}
	})
}

// TestSignatureHelpParameterLabelsAreNativeArms proves the rendered
// signature uses the protocol's native union arms for parameter labels and
// documentation, not raw strings.
func TestSignatureHelpParameterLabelsAreNativeArms(t *testing.T) {
	src := `@validate(min_len: 1)`
	cursor := posAfter(t, src, "@validate(")

	got := signatureHelpAt(src, cursor)
	if got == nil {
		t.Fatal("signatureHelpAt returned nil")
	}

	for _, param := range got.Signatures[0].Parameters {
		if _, ok := param.Label.(protocol.String); !ok {
			t.Errorf("parameter Label = %T, want protocol.String", param.Label)
		}

		if _, ok := param.Documentation.(protocol.String); !ok {
			t.Errorf("parameter Documentation = %T, want protocol.String", param.Documentation)
		}
	}
}

// TestServerSignatureHelp exercises the textDocument/signatureHelp handler
// end to end through the server's workspace, not just the pure function.
func TestServerSignatureHelp(t *testing.T) {
	s := NewServer()

	rawURI := "file:///sig.zen"
	src := `entity User {
	email: string @validate(min_len: 1, )
}
`
	s.ws.SetDoc(rawURI, src)

	cursor := cursorOn(t, src, 1, ", )")
	cursor.Character += uint32(len(", "))

	got, err := s.signatureHelp(t.Context(), &protocol.SignatureHelpParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: uri.URI(rawURI)},
			Position:     cursor,
		},
	})
	if err != nil {
		t.Fatalf("signatureHelp error: %v", err)
	}

	if got == nil {
		t.Fatal("signatureHelp returned nil")
	}

	if got.Signatures[0].Label != "validate(format, min_len, max_len, gt, gte, lt, lte)" {
		t.Errorf("Label = %q", got.Signatures[0].Label)
	}

	if active := activeParamOf(t, got); active != paramIndex(t, "validate", "min_len") {
		t.Errorf("ActiveParameter = %d, want the position after min_len's comma", active)
	}
}

func TestServerSignatureHelpUnknownDocument(t *testing.T) {
	s := NewServer()

	got, err := s.signatureHelp(t.Context(), &protocol.SignatureHelpParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: uri.URI("file:///missing.zen")},
			Position:     protocol.Position{Line: 0, Character: 0},
		},
	})
	if err != nil {
		t.Fatalf("signatureHelp error: %v", err)
	}

	if got != nil {
		t.Errorf("signatureHelp() = %+v, want nil for an unknown document", got)
	}
}

func TestSignatureHelpOpenParenAlone(t *testing.T) {
	if got := signatureHelpAt("(x", protocol.Position{Character: 2}); got != nil {
		t.Errorf("signatureHelpAt() = %+v, want nil with no call name", got)
	}
}

func TestSignatureHelpNonWordBeforeParen(t *testing.T) {
	if got := signatureHelpAt("@(x", protocol.Position{Character: 3}); got != nil {
		t.Errorf("signatureHelpAt() = %+v, want nil when the paren follows no name", got)
	}
}

func TestSignatureHelpClampsPastLastParameter(t *testing.T) {
	src := `@validate(a, b, c, d, e, f, g, h`
	cursor := posAfter(t, src, "h")

	got := signatureHelpAt(src, cursor)
	if got == nil {
		t.Fatal("signatureHelpAt returned nil")
	}

	// Eight slots typed, seven parameters declared: the cursor pins to the
	// last one rather than running off the end.
	if active := activeParamOf(t, got); active != len(signatureSpecs["validate"].params)-1 {
		t.Errorf("ActiveParameter = %d, want the last parameter", active)
	}
}

func TestSignatureHelpNestedCallParensIgnored(t *testing.T) {
	src := `retry: backoff(warmup(1), base: 30s)`
	cursor := posAfter(t, src, "base: ")

	got := signatureHelpAt(src, cursor)
	if got == nil {
		t.Fatal("signatureHelpAt returned nil")
	}

	// The comma inside warmup(1) belongs to the nested call, so base: is
	// still the second top-level argument.
	if active := activeParamOf(t, got); active != paramIndex(t, "backoff", "base") {
		t.Errorf("ActiveParameter = %d, want the base parameter", active)
	}
}

func TestParamIndexByNameUnknown(t *testing.T) {
	if got := paramIndexByName(signatureSpecs["validate"].params, "bogus"); got != -1 {
		t.Errorf("paramIndexByName() = %d, want -1", got)
	}
}

// dispatchSigSchemaSrc is the workspace schema behind the dispatch
// signature tests: a validated-param job, a message/entity-ref job and a
// parameterless job, plus a schedule whose dispatch passes matching arity
// (resolveDispatch rejects arity mismatches, so the fixture itself must be
// valid to compile cleanly).
const dispatchSigSchemaSrc = `entity User {
	id: uuid @primary
}

job SendDigest(target: string @validate(min_len: 1), batch: int64) {
	queue: low
}

job Invite(u: User) {
	queue: mail
}

job Flush() {
	queue: low
}

schedule Nightly {
	cron: "0 0 * * *"
	dispatch: SendDigest("a", 1)
}
`

// compileDispatchSigSchema compiles dispatchSigSchemaSrc and returns the
// resolved schema, failing the test on diagnostics.
func compileDispatchSigSchema(t *testing.T) *ir.Schema {
	t.Helper()

	result, diags := compile.Compile(map[string]string{"main.zen": dispatchSigSchemaSrc})
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	return result.Schema
}

// dispatchCursor returns the cursor at the end of the line ending in
// marker in a multi-line src, the mid-edit position right after an
// unclosed `(`.
func dispatchCursor(t *testing.T, src, marker string) protocol.Position {
	t.Helper()

	for i, line := range strings.Split(src, "\n") {
		if strings.HasSuffix(line, marker) {
			return protocol.Position{Line: uint32(i), Character: uint32(len(line))} // test fixture
		}
	}

	t.Fatalf("line ending in %q not found in %q", marker, src)

	return protocol.Position{}
}

// dispatchParamDoc extracts the documentation string of one signature
// parameter, failing the test when it is not a native protocol.String arm.
func dispatchParamDoc(t *testing.T, param protocol.ParameterInformation) string {
	t.Helper()

	doc, ok := param.Documentation.(protocol.String)
	if !ok {
		t.Fatalf("parameter Documentation = %T, want protocol.String", param.Documentation)
	}

	return string(doc)
}

func TestDispatchSignatureHelpBasic(t *testing.T) {
	schema := compileDispatchSigSchema(t)
	src := "schedule Nightly {\n\tcron: \"0 0 * * *\"\n\tdispatch: SendDigest("

	got := dispatchSignatureHelp(schema, src, dispatchCursor(t, src, "dispatch: SendDigest("))
	if got == nil {
		t.Fatal("dispatchSignatureHelp returned nil")
	}

	if len(got.Signatures) != 1 {
		t.Fatalf("Signatures = %d, want 1", len(got.Signatures))
	}

	if want := "SendDigest(target: string, batch: int64)"; got.Signatures[0].Label != want {
		t.Errorf("Label = %q, want %q", got.Signatures[0].Label, want)
	}

	if len(got.Signatures[0].Parameters) != 2 {
		t.Fatalf("Parameters = %d, want 2", len(got.Signatures[0].Parameters))
	}

	if active := activeParamOf(t, got); active != 0 {
		t.Errorf("ActiveParameter = %d, want 0", active)
	}
}

func TestDispatchSignatureHelpActiveParameterAdvance(t *testing.T) {
	schema := compileDispatchSigSchema(t)
	src := "schedule Nightly {\n\tcron: \"0 0 * * *\"\n\tdispatch: SendDigest(\"a\", "

	got := dispatchSignatureHelp(schema, src, dispatchCursor(t, src, "dispatch: SendDigest(\"a\", "))
	if got == nil {
		t.Fatal("dispatchSignatureHelp returned nil")
	}

	if active := activeParamOf(t, got); active != 1 {
		t.Errorf("ActiveParameter = %d, want 1", active)
	}
}

func TestDispatchSignatureHelpNamedArgRemap(t *testing.T) {
	schema := compileDispatchSigSchema(t)
	src := "schedule Nightly {\n\tcron: \"0 0 * * *\"\n\tdispatch: SendDigest(batch: "

	got := dispatchSignatureHelp(schema, src, dispatchCursor(t, src, "dispatch: SendDigest(batch: "))
	if got == nil {
		t.Fatal("dispatchSignatureHelp returned nil")
	}

	// batch: is named out of order, so the cursor remaps to its declared
	// slot rather than staying positional.
	if active := activeParamOf(t, got); active != 1 {
		t.Errorf("ActiveParameter = %d, want 1", active)
	}
}

func TestDispatchSignatureHelpValidateDetail(t *testing.T) {
	schema := compileDispatchSigSchema(t)
	src := "schedule Nightly {\n\tcron: \"0 0 * * *\"\n\tdispatch: SendDigest("

	got := dispatchSignatureHelp(schema, src, dispatchCursor(t, src, "dispatch: SendDigest("))
	if got == nil {
		t.Fatal("dispatchSignatureHelp returned nil")
	}

	doc := dispatchParamDoc(t, got.Signatures[0].Parameters[0])

	for _, want := range []string{"string", "@validate(min_len: 1)"} {
		if !strings.Contains(doc, want) {
			t.Errorf("parameter doc = %q, want it to contain %q", doc, want)
		}
	}
}

func TestDispatchSignatureHelpRefParam(t *testing.T) {
	schema := compileDispatchSigSchema(t)
	src := "schedule Nightly {\n\tcron: \"0 0 * * *\"\n\tdispatch: Invite("

	got := dispatchSignatureHelp(schema, src, dispatchCursor(t, src, "dispatch: Invite("))
	if got == nil {
		t.Fatal("dispatchSignatureHelp returned nil")
	}

	if want := "Invite(u: User)"; got.Signatures[0].Label != want {
		t.Errorf("Label = %q, want %q", got.Signatures[0].Label, want)
	}
}

func TestDispatchSignatureHelpZeroParams(t *testing.T) {
	schema := compileDispatchSigSchema(t)
	src := "schedule Nightly {\n\tcron: \"0 0 * * *\"\n\tdispatch: Flush("

	got := dispatchSignatureHelp(schema, src, dispatchCursor(t, src, "dispatch: Flush("))
	if got == nil {
		t.Fatal("dispatchSignatureHelp returned nil")
	}

	if want := "Flush()"; got.Signatures[0].Label != want {
		t.Errorf("Label = %q, want %q", got.Signatures[0].Label, want)
	}

	if len(got.Signatures[0].Parameters) != 0 {
		t.Errorf("Parameters = %d, want 0", len(got.Signatures[0].Parameters))
	}

	if _, ok := got.ActiveParameter.Get(); ok {
		t.Errorf("ActiveParameter is set, want absent for a parameterless job")
	}
}

func TestDispatchSignatureHelpUnknownJob(t *testing.T) {
	schema := compileDispatchSigSchema(t)
	src := "schedule Nightly {\n\tcron: \"0 0 * * *\"\n\tdispatch: Missing("

	if got := dispatchSignatureHelp(schema, src, dispatchCursor(t, src, "dispatch: Missing(")); got != nil {
		t.Errorf("dispatchSignatureHelp() = %+v, want nil for an unknown job", got)
	}
}

func TestDispatchSignatureHelpNilSchema(t *testing.T) {
	src := "schedule Nightly {\n\tcron: \"0 0 * * *\"\n\tdispatch: SendDigest("

	if got := dispatchSignatureHelp(nil, src, dispatchCursor(t, src, "dispatch: SendDigest(")); got != nil {
		t.Errorf("dispatchSignatureHelp() = %+v, want nil for a nil schema", got)
	}
}

func TestDispatchSignatureHelpNotInSchedule(t *testing.T) {
	schema := compileDispatchSigSchema(t)
	src := "job X {\n\tqueue: low\n\tdispatch: SendDigest("

	if got := dispatchSignatureHelp(schema, src, dispatchCursor(t, src, "dispatch: SendDigest(")); got != nil {
		t.Errorf("dispatchSignatureHelp() = %+v, want nil outside a schedule block", got)
	}
}

func TestDispatchSignatureHelpNestedCallIgnored(t *testing.T) {
	schema := compileDispatchSigSchema(t)
	src := "schedule Nightly {\n\tcron: \"0 0 * * *\"\n\tdispatch: SendDigest(warmup("

	if got := dispatchSignatureHelp(schema, src, dispatchCursor(t, src, "dispatch: SendDigest(warmup(")); got != nil {
		t.Errorf("dispatchSignatureHelp() = %+v, want nil inside a nested call", got)
	}
}

// TestDispatchSignatureHelpExecuteTakesNoSignature pins the deliberate
// absence of an `execute:` arm: a saga step's execute option is a bare
// `Service.RPC` reference (a `(` there is a parse error), so there is no
// call site and the cursor gets no signature.
func TestDispatchSignatureHelpExecuteTakesNoSignature(t *testing.T) {
	schema := compileDispatchSigSchema(t)
	src := "saga Checkout {\n\tstep reserve {\n\t\texecute: UserService.CreateUser("

	if got := dispatchSignatureHelp(schema, src, dispatchCursor(t, src, "execute: UserService.CreateUser(")); got != nil {
		t.Errorf("dispatchSignatureHelp() = %+v, want nil for an execute: reference", got)
	}
}

func TestDispatchSignatureHelpClampsPastLastParameter(t *testing.T) {
	schema := compileDispatchSigSchema(t)
	src := "schedule Nightly {\n\tcron: \"0 0 * * *\"\n\tdispatch: SendDigest(\"a\", 1, \"extra\""

	got := dispatchSignatureHelp(schema, src, dispatchCursor(t, src, "dispatch: SendDigest(\"a\", 1, \"extra\""))
	if got == nil {
		t.Fatal("dispatchSignatureHelp returned nil")
	}

	if active := activeParamOf(t, got); active != 1 {
		t.Errorf("ActiveParameter = %d, want the last parameter", active)
	}
}

// TestDispatchSignatureHelpParameterLabelsAreNativeArms proves the dispatch
// signature uses the protocol's native union arms, like the
// static-attribute path does.
func TestDispatchSignatureHelpParameterLabelsAreNativeArms(t *testing.T) {
	schema := compileDispatchSigSchema(t)
	src := "schedule Nightly {\n\tcron: \"0 0 * * *\"\n\tdispatch: SendDigest("

	got := dispatchSignatureHelp(schema, src, dispatchCursor(t, src, "dispatch: SendDigest("))
	if got == nil {
		t.Fatal("dispatchSignatureHelp returned nil")
	}

	for _, param := range got.Signatures[0].Parameters {
		if _, ok := param.Label.(protocol.String); !ok {
			t.Errorf("parameter Label = %T, want protocol.String", param.Label)
		}
	}
}

// TestServerDispatchSignatureHelp exercises the textDocument/signatureHelp
// handler end to end for a dispatch call: the open paren is unclosed, so
// the document carries a parse diagnostic, but the job itself still
// resolves.
func TestServerDispatchSignatureHelp(t *testing.T) {
	s := NewServer()

	rawURI := "file:///dispatch.zen"
	src := "job SendDigest(target: string, batch: int64) {\n" +
		"\tqueue: low\n" +
		"}\n" +
		"\n" +
		"schedule Nightly {\n" +
		"\tcron: \"0 0 * * *\"\n" +
		"\tdispatch: SendDigest(\n" +
		"}\n"
	s.ws.SetDoc(rawURI, src)

	cursor := cursorOn(t, src, 6, "dispatch: SendDigest(")
	cursor.Character += uint32(len("dispatch: SendDigest("))

	got, err := s.signatureHelp(t.Context(), &protocol.SignatureHelpParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: uri.URI(rawURI)},
			Position:     cursor,
		},
	})
	if err != nil {
		t.Fatalf("signatureHelp error: %v", err)
	}

	if got == nil {
		t.Fatal("signatureHelp returned nil")
	}

	if want := "SendDigest(target: string, batch: int64)"; got.Signatures[0].Label != want {
		t.Errorf("Label = %q, want %q", got.Signatures[0].Label, want)
	}
}
