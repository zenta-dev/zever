package main

import (
	"testing"

	"go.lsp.dev/protocol"
	"go.lsp.dev/uri"
)

// lensUserSrc declares an entity nothing else names on its own.
const lensUserSrc = "entity User {\n" +
	"\tid: uuid @primary\n" +
	"}\n"

// lensTaskSrc references User once, via a relation target.
const lensTaskSrc = "entity Task {\n" +
	"\tid: uuid @primary\n" +
	"\tuser_id: uuid\n" +
	"\tbelongs_to user: User @foreign_key(user_id)\n" +
	"}\n"

// lensGreeterSrc references User a second time, via an rpc return type.
const lensGreeterSrc = "service Greeter {\n" +
	"\trpc Hello(req: Greeting) -> User {\n" +
	"\t\tauth: none\n" +
	"\t}\n" +
	"}\n"

// lensAllSrc exercises one declaration of every lens-bearing kind in a
// single document. Reference counts (declaration itself excluded):
// User 1 (the rpc return type), Greeting 1 (the rpc param type),
// CleanupJob 1 (the schedule dispatch), everything else 0 -- the saga
// step's execute identifiers deliberately belong to no walk, the service,
// rpc, schedule, saga, and enum walks are declaration-only, and Role is
// never spelled as a type.
const lensAllSrc = "entity User {\n" +
	"\tid: uuid @primary\n" +
	"}\n" +
	"\n" +
	"message Greeting {\n" +
	"\ttext: string\n" +
	"}\n" +
	"\n" +
	"service Greeter {\n" +
	"\trpc Hello(req: Greeting) -> User {\n" +
	"\t\tauth: none\n" +
	"\t}\n" +
	"}\n" +
	"\n" +
	"job CleanupJob(id: uuid) {\n" +
	"\tqueue: \"default\"\n" +
	"}\n" +
	"\n" +
	"schedule Nightly {\n" +
	"\tcron: \"0 0 * * *\"\n" +
	"\tdispatch: CleanupJob()\n" +
	"}\n" +
	"\n" +
	"saga Checkout {\n" +
	"\tstep reserve {\n" +
	"\t\texecute: Greeter.Hello\n" +
	"\t}\n" +
	"}\n" +
	"\n" +
	"enum Role {\n" +
	"\tadmin,\n" +
	"\tmember\n" +
	"}\n"

// lensTitles indexes lenses by title for order-independent assertions.
func lensTitles(t *testing.T, lenses []protocol.CodeLens) map[string]int {
	t.Helper()

	titles := make(map[string]int, len(lenses))

	for _, lens := range lenses {
		titles[lens.Command.Title]++
	}

	return titles
}

// TestCodeLensesAtEntityCountsWorkspaceReferences proves the entity lens
// counts every workspace occurrence of the name except the declaration
// itself: a relation target plus an rpc return type read "2 reference(s)".
func TestCodeLensesAtEntityCountsWorkspaceReferences(t *testing.T) {
	t.Parallel()

	parsed := parseRefsFiles(t, map[string]string{
		"/tmp/lens-user.zen":    lensUserSrc,
		"/tmp/lens-task.zen":    lensTaskSrc,
		"/tmp/lens-greeter.zen": lensGreeterSrc,
	})

	lenses := codeLensesAt(nil, parsed, "file:///tmp/lens-user.zen")

	if len(lenses) != 1 {
		t.Fatalf("codeLensesAt = %d lenses, want 1 (the User declaration): %+v", len(lenses), lenses)
	}

	if got := lenses[0].Command.Title; got != "2 reference(s)" {
		t.Errorf("lens title = %q, want %q", got, "2 reference(s)")
	}

	if lenses[0].Command.Command != "" {
		t.Errorf("lens command = %q, want empty (info-only, nothing to execute)", lenses[0].Command.Command)
	}
}

// TestCodeLensesAtCoversEveryTopLevelKind proves every declaration kind
// earns exactly one lens (services earn one more per rpc) with the expected
// reference counts.
func TestCodeLensesAtCoversEveryTopLevelKind(t *testing.T) {
	t.Parallel()

	parsed := parseRefsFiles(t, map[string]string{"/tmp/lens-all.zen": lensAllSrc})

	lenses := codeLensesAt(nil, parsed, "file:///tmp/lens-all.zen")

	if len(lenses) != 8 {
		t.Fatalf("codeLensesAt = %d lenses, want 8 (7 decls plus 1 rpc): %+v", len(lenses), lenses)
	}

	titles := lensTitles(t, lenses)

	for title, want := range map[string]int{
		"1 reference(s)": 3, // User, Greeting, CleanupJob
		"0 reference(s)": 5, // Greeter, Hello, Nightly, Checkout, Role
	} {
		if got := titles[title]; got != want {
			t.Errorf("lenses titled %q = %d, want %d (all: %+v)", title, got, want, lenses)
		}
	}
}

// TestCodeLensesAtAnchorsDeclNameRanges proves each lens range spans exactly
// the declaration name it counts for.
func TestCodeLensesAtAnchorsDeclNameRanges(t *testing.T) {
	t.Parallel()

	parsed := parseRefsFiles(t, map[string]string{"/tmp/lens-all.zen": lensAllSrc})

	lenses := codeLensesAt(nil, parsed, "file:///tmp/lens-all.zen")

	for _, tc := range []struct {
		line  uint32
		start uint32
		end   uint32
	}{
		{line: 0, start: 7, end: 11},  // entity User
		{line: 4, start: 8, end: 16},  // message Greeting
		{line: 8, start: 8, end: 15},  // service Greeter
		{line: 9, start: 5, end: 10},  // rpc Hello
		{line: 14, start: 4, end: 14}, // job CleanupJob
		{line: 18, start: 9, end: 16}, // schedule Nightly
		{line: 23, start: 5, end: 13}, // saga Checkout
		{line: 29, start: 5, end: 9},  // enum Role
	} {
		found := false

		for _, lens := range lenses {
			if lens.Range.Start.Line == tc.line && lens.Range.Start.Character == tc.start &&
				lens.Range.End.Character == tc.end {
				found = true

				break
			}
		}

		if !found {
			t.Errorf("no lens on line %d chars %d-%d (all: %+v)", tc.line, tc.start, tc.end, lenses)
		}
	}
}

// TestCodeLensesAtUnknownDocNil proves an unknown document yields nil, not
// an error-shaped empty list.
func TestCodeLensesAtUnknownDocNil(t *testing.T) {
	t.Parallel()

	parsed := parseRefsFiles(t, map[string]string{"/tmp/lens-all.zen": lensAllSrc})

	if got := codeLensesAt(nil, parsed, "file:///tmp/lens-missing.zen"); got != nil {
		t.Errorf("codeLensesAt(unknown) = %+v, want nil", got)
	}
}

// TestCodeLensHandlerServesKnownDocs proves the handler wires settings,
// workspace parsing, and codeLensesAt together end to end.
func TestCodeLensHandlerServesKnownDocs(t *testing.T) {
	t.Parallel()

	s := NewServer()
	openDoc(t, s, serverDocA, lensAllSrc)

	lenses, err := s.CodeLens(t.Context(), &protocol.CodeLensParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: uri.URI(serverDocA)},
	})
	if err != nil {
		t.Fatalf("CodeLens: %v", err)
	}

	if len(lenses) != 8 {
		t.Fatalf("CodeLens = %d lenses, want 8: %+v", len(lenses), lenses)
	}

	titles := lensTitles(t, lenses)

	if got := titles["1 reference(s)"]; got != 3 {
		t.Errorf("lenses titled %q = %d, want 3", "1 reference(s)", got)
	}
}

// TestCodeLensHandlerUnknownDocNil proves the handler answers nil for a
// document the workspace does not know.
func TestCodeLensHandlerUnknownDocNil(t *testing.T) {
	t.Parallel()

	s := NewServer()

	got, err := s.CodeLens(t.Context(), &protocol.CodeLensParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: uri.URI(serverUnknown)},
	})
	if err != nil {
		t.Fatalf("CodeLens(unknown): %v", err)
	}

	if got != nil {
		t.Errorf("CodeLens(unknown) = %+v, want nil", got)
	}
}

// TestCodeLensResolvePassthrough proves resolve returns the item unchanged:
// lenses already carry their full titles, so there is nothing to fill in.
func TestCodeLensResolvePassthrough(t *testing.T) {
	t.Parallel()

	s := NewServer()

	lens := &protocol.CodeLens{
		Range:   protocol.Range{Start: protocol.Position{Line: 0, Character: 7}},
		Command: protocol.Command{Title: "2 reference(s)"},
	}

	got, err := s.CodeLensResolve(t.Context(), lens)
	if err != nil {
		t.Fatalf("CodeLensResolve: %v", err)
	}

	if got != lens {
		t.Errorf("CodeLensResolve = %+v, want the same item back", got)
	}
}

// TestCodeLensSettingsGateOff proves zever.codelens.references=false
// suppresses every lens.
func TestCodeLensSettingsGateOff(t *testing.T) {
	t.Parallel()

	s := NewServer()
	openDoc(t, s, serverDocA, lensAllSrc)

	if err := s.DidChangeConfiguration(t.Context(), &protocol.DidChangeConfigurationParams{
		Settings: protocol.LSPAny(`{"zever":{"codelens":{"references":false}}}`),
	}); err != nil {
		t.Fatalf("DidChangeConfiguration: %v", err)
	}

	got, err := s.CodeLens(t.Context(), &protocol.CodeLensParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: uri.URI(serverDocA)},
	})
	if err != nil {
		t.Fatalf("CodeLens: %v", err)
	}

	if len(got) != 0 {
		t.Errorf("CodeLens with references=false = %+v, want empty", got)
	}
}

// TestCodeLensCapability proves Initialize advertises a code lens provider
// with resolve support, matching the DocumentLinkProvider shape.
func TestCodeLensCapability(t *testing.T) {
	t.Parallel()

	s := NewServer()

	res, err := s.Initialize(t.Context(), &protocol.InitializeParams{})
	if err != nil {
		t.Fatalf("Initialize: %v", err)
	}

	provider := res.Capabilities.CodeLensProvider
	if provider == nil {
		t.Fatal("CodeLensProvider is nil")
	}

	if provider.ResolveProvider == nil || !*provider.ResolveProvider {
		t.Error("CodeLensProvider.ResolveProvider not enabled")
	}
}

// TestParseSettingsCodeLensReferences pins the settings key contract:
// zever.codelens.references defaults to true, flips off on false, and
// ignores values of the wrong type.
func TestParseSettingsCodeLensReferences(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		payload string
		want    bool
	}{
		{"absent section keeps the default on", `{}`, true},
		{"explicit true stays on", `{"zever":{"codelens":{"references":true}}}`, true},
		{"explicit false switches off", `{"zever":{"codelens":{"references":false}}}`, false},
		{"wrong type is ignored, keeping the default", `{"zever":{"codelens":{"references":"no"}}}`, true},
		{"non-object codelens is ignored", `{"zever":{"codelens":true}}`, true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, ok := parseSettings(protocol.LSPAny(tc.payload))
			if !ok {
				t.Fatalf("parseSettings ok = false, want true")
			}

			if got.CodeLensReferences != tc.want {
				t.Errorf("CodeLensReferences = %v, want %v", got.CodeLensReferences, tc.want)
			}
		})
	}
}
