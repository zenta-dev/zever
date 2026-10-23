package main

import (
	"testing"
	"time"

	"go.lsp.dev/protocol"
	"go.lsp.dev/uri"

	"github.com/zenta-dev/zever/dsl/ast"
	"github.com/zenta-dev/zever/dsl/compile"
	"github.com/zenta-dev/zever/dsl/ir"
	"github.com/zenta-dev/zever/dsl/parser"
)

// TestDefaultSettings pins the configuration that preserves today's
// behavior exactly, so a client that never sends didChangeConfiguration
// hears nothing change.
func TestDefaultSettings(t *testing.T) {
	t.Parallel()

	got := defaultSettings()

	if !got.ImplicitModuleHints || !got.ErrorCaseHints {
		t.Errorf("defaultSettings hint gates = %+v, want both enabled", got)
	}

	if got.ParamNames {
		t.Errorf("defaultSettings.ParamNames = true, want false (opt-in noise)")
	}

	if got.DebounceDelay != debounceDelay {
		t.Errorf("defaultSettings.DebounceDelay = %v, want %v", got.DebounceDelay, debounceDelay)
	}
}

// TestParseSettings pins the parsing contract: nested JSON under `zever`,
// whole-replacement with per-key fallback to defaults, malformed input
// rejected whole.
func TestParseSettings(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		payload string
		want    Settings
		wantOK  bool
	}{
		{
			name:    "empty object yields defaults",
			payload: `{}`,
			want:    defaultSettings(),
			wantOK:  true,
		},
		{
			name:    "absent zever section yields defaults",
			payload: `{"otherSub":{"unrelated":true}}`,
			want:    defaultSettings(),
			wantOK:  true,
		},
		{
			name:    "implicitModule off preserves the rest",
			payload: `{"zever":{"inlayHints":{"implicitModule":false}}}`,
			want: Settings{
				ImplicitModuleHints: false,
				ErrorCaseHints:      true,
				CodeLensReferences:  true,
				DebounceDelay:       debounceDelay,
			},
			wantOK: true,
		},
		{
			name:    "errorCases off with a custom debounce",
			payload: `{"zever":{"inlayHints":{"errorCases":false},"diagnostics":{"debounceMs":900}}}`,
			want: Settings{
				ImplicitModuleHints: true,
				ErrorCaseHints:      false,
				CodeLensReferences:  true,
				DebounceDelay:       900 * time.Millisecond,
			},
			wantOK: true,
		},
		{
			name:    "paramNames on enables the call-argument hint family",
			payload: `{"zever":{"inlayHints":{"paramNames":true}}}`,
			want: Settings{
				ImplicitModuleHints: true,
				ErrorCaseHints:      true,
				CodeLensReferences:  true,
				ParamNames:          true,
				DebounceDelay:       debounceDelay,
			},
			wantOK: true,
		},
		{
			name:    "wrong type for a bool is ignored",
			payload: `{"zever":{"inlayHints":{"implicitModule":"yes"}}}`,
			want:    defaultSettings(),
			wantOK:  true,
		},
		{
			name:    "zero debounceMs is ignored, keeping the default",
			payload: `{"zever":{"diagnostics":{"debounceMs":0}}}`,
			want:    defaultSettings(),
			wantOK:  true,
		},
		{
			name:    "negative debounceMs is ignored",
			payload: `{"zever":{"diagnostics":{"debounceMs":-5}}}`,
			want:    defaultSettings(),
			wantOK:  true,
		},
		{
			name:    "non-numeric debounceMs is ignored",
			payload: `{"zever":{"diagnostics":{"debounceMs":"300"}}}`,
			want:    defaultSettings(),
			wantOK:  true,
		},
		{
			name:    "a non-object zever section is ignored",
			payload: `{"zever":"yes"}`,
			want:    defaultSettings(),
			wantOK:  true,
		},
		{
			name:    "malformed JSON is rejected whole",
			payload: `{"zever":`,
			wantOK:  false,
		},
		{
			name:    "a non-object payload is rejected",
			payload: `true`,
			wantOK:  false,
		},
		{
			name:    "an empty payload is rejected",
			payload: ``,
			wantOK:  false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, ok := parseSettings(protocol.LSPAny([]byte(tc.payload)))

			if ok != tc.wantOK {
				t.Fatalf("parseSettings ok = %v, want %v", ok, tc.wantOK)
			}

			if !tc.wantOK {
				return
			}

			if got != tc.want {
				t.Errorf("parseSettings = %+v, want %+v", got, tc.want)
			}
		})
	}
}

// TestDidChangeConfiguration proves the handler stores a well-formed
// payload and leaves the previous settings in place for one it cannot
// parse -- never silently resetting user configuration.
func TestDidChangeConfiguration(t *testing.T) {
	t.Parallel()

	s := NewServer()

	if err := s.DidChangeConfiguration(t.Context(), &protocol.DidChangeConfigurationParams{
		Settings: protocol.LSPAny(`{"zever":{"inlayHints":{"implicitModule":false},"diagnostics":{"debounceMs":40}}}`),
	}); err != nil {
		t.Fatalf("DidChangeConfiguration: %v", err)
	}

	want := Settings{ImplicitModuleHints: false, ErrorCaseHints: true, CodeLensReferences: true, DebounceDelay: 40 * time.Millisecond}
	if got := s.currentSettings(); got != want {
		t.Errorf("settings = %+v, want %+v", got, want)
	}

	// Malformed: the last good settings survive untouched.
	if err := s.DidChangeConfiguration(t.Context(), &protocol.DidChangeConfigurationParams{
		Settings: protocol.LSPAny(`{`),
	}); err != nil {
		t.Fatalf("DidChangeConfiguration (malformed): %v", err)
	}

	if got := s.currentSettings(); got != want {
		t.Errorf("settings after malformed payload = %+v, want %+v unchanged", got, want)
	}

	// A nil params is tolerated.
	if err := s.DidChangeConfiguration(t.Context(), nil); err != nil {
		t.Fatalf("DidChangeConfiguration(nil): %v", err)
	}

	if got := s.currentSettings(); got != want {
		t.Errorf("settings after nil params = %+v, want %+v unchanged", got, want)
	}
}

// TestDidChangeConfigurationDefaultsOnFullReplacement pins the
// whole-replacement reading: a payload without a family flips it back to
// its default, matching how mainstream clients resend the whole settings
// object on every change.
func TestDidChangeConfigurationDefaultsOnFullReplacement(t *testing.T) {
	t.Parallel()

	s := NewServer()

	if err := s.DidChangeConfiguration(t.Context(), &protocol.DidChangeConfigurationParams{
		Settings: protocol.LSPAny(`{"zever":{"diagnostics":{"debounceMs":900}}}`),
	}); err != nil {
		t.Fatalf("DidChangeConfiguration: %v", err)
	}

	if got := s.currentSettings(); got.DebounceDelay != 900*time.Millisecond {
		t.Errorf("settings = %+v, want default gates with a 900ms debounce", got)
	}
}

// refreshTestParams builds Initialize params advertising the given workspace
// refresh support: a nil arm means the client advertised nothing for that
// family, matching a client with no refresh support at all.
func refreshTestParams(t *testing.T, codeLens, inlayHint *bool) *protocol.InitializeParams {
	t.Helper()

	return &protocol.InitializeParams{
		Capabilities: protocol.ClientCapabilities{
			Workspace: &protocol.WorkspaceClientCapabilities{
				CodeLens:  &protocol.CodeLensWorkspaceClientCapabilities{RefreshSupport: codeLens},
				InlayHint: &protocol.InlayHintWorkspaceClientCapabilities{RefreshSupport: inlayHint},
			},
		},
	}
}

// TestRefreshCapsFor pins the Initialize refresh-capability negotiation: a
// nil params or any nil link in the capability chain negotiates no refresh,
// while advertised (and only advertised-true) support is recorded per family.
func TestRefreshCapsFor(t *testing.T) {
	t.Parallel()

	yes := boolPtr(true)
	no := boolPtr(false)

	tests := []struct {
		name   string
		params *protocol.InitializeParams
		wantCL bool
		wantIH bool
	}{
		{"nil params", nil, false, false},
		{"empty params", &protocol.InitializeParams{}, false, false},
		{"workspace without families", &protocol.InitializeParams{
			Capabilities: protocol.ClientCapabilities{Workspace: &protocol.WorkspaceClientCapabilities{}},
		}, false, false},
		{"both advertised", refreshTestParams(t, yes, yes), true, true},
		{"only codelens advertised", refreshTestParams(t, yes, nil), true, false},
		{"only inlay advertised", refreshTestParams(t, nil, yes), false, true},
		{"explicitly denied", refreshTestParams(t, no, no), false, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if gotCL, gotIH := refreshCapsFor(tc.params); gotCL != tc.wantCL || gotIH != tc.wantIH {
				t.Errorf("refreshCapsFor() = (%v, %v), want (%v, %v)", gotCL, gotIH, tc.wantCL, tc.wantIH)
			}
		})
	}
}

// TestDidChangeConfigurationSendsRefreshes proves a settings replacement
// refreshes exactly the families it flips, and only when the client
// advertised that family's refresh support at Initialize: a flipped
// codelens.references pushes workspace/codeLens/refresh, a flipped
// inlay-hint gate pushes workspace/inlayHint/refresh, and anything else --
// an unchanged family, an unsupported family, a debounce-only change --
// stays silent. Semantic tokens are never refreshed: no settings payload
// here touches token behavior, so no case expects one.
func TestDidChangeConfigurationSendsRefreshes(t *testing.T) {
	t.Parallel()

	yes := boolPtr(true)
	no := boolPtr(false)

	tests := []struct {
		name          string
		codeLensCap   *bool
		inlayHintCap  *bool
		payload       string
		wantCodeLens  int
		wantInlayHint int
	}{
		{
			name:         "codelens flip with support refreshes",
			codeLensCap:  yes,
			payload:      `{"zever":{"codelens":{"references":false}}}`,
			wantCodeLens: 1,
		},
		{
			name:    "codelens flip without support stays silent",
			payload: `{"zever":{"codelens":{"references":false}}}`,
		},
		{
			name:        "codelens flip with explicitly denied support stays silent",
			codeLensCap: no,
			payload:     `{"zever":{"codelens":{"references":false}}}`,
		},
		{
			name:          "implicitModule flip with support refreshes",
			inlayHintCap:  yes,
			payload:       `{"zever":{"inlayHints":{"implicitModule":false}}}`,
			wantInlayHint: 1,
		},
		{
			name:          "errorCases flip refreshes",
			inlayHintCap:  yes,
			payload:       `{"zever":{"inlayHints":{"errorCases":false}}}`,
			wantInlayHint: 1,
		},
		{
			name:          "paramNames flip refreshes",
			inlayHintCap:  yes,
			payload:       `{"zever":{"inlayHints":{"paramNames":true}}}`,
			wantInlayHint: 1,
		},
		{
			name:        "inlay flip with only codelens support stays silent",
			codeLensCap: yes,
			payload:     `{"zever":{"inlayHints":{"implicitModule":false}}}`,
		},
		{
			name:         "identical replacement sends nothing",
			codeLensCap:  yes,
			inlayHintCap: yes,
			payload:      `{}`,
		},
		{
			name:         "debounce-only change sends nothing",
			codeLensCap:  yes,
			inlayHintCap: yes,
			payload:      `{"zever":{"diagnostics":{"debounceMs":900}}}`,
		},
		{
			name:          "both families flip with both caps",
			codeLensCap:   yes,
			inlayHintCap:  yes,
			payload:       `{"zever":{"codelens":{"references":false},"inlayHints":{"paramNames":true}}}`,
			wantCodeLens:  1,
			wantInlayHint: 1,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			s := NewServer()

			if _, err := s.Initialize(t.Context(), refreshTestParams(t, tc.codeLensCap, tc.inlayHintCap)); err != nil {
				t.Fatalf("Initialize: %v", err)
			}

			fake := &fakeClient{}
			ctx := protocol.WithClient(t.Context(), fake)

			if err := s.DidChangeConfiguration(ctx, &protocol.DidChangeConfigurationParams{
				Settings: protocol.LSPAny(tc.payload),
			}); err != nil {
				t.Fatalf("DidChangeConfiguration: %v", err)
			}

			if gotCL, gotIH := fake.refreshCounts(); gotCL != tc.wantCodeLens || gotIH != tc.wantInlayHint {
				t.Errorf("refreshes = (codelens %d, inlay %d), want (%d, %d)",
					gotCL, gotIH, tc.wantCodeLens, tc.wantInlayHint)
			}
		})
	}
}

// TestDidChangeConfigurationWithoutClientSendsNothing proves a settings
// flip with no captured client yet is a silent no-op, not a nil dereference.
func TestDidChangeConfigurationWithoutClientSendsNothing(t *testing.T) {
	t.Parallel()

	s := NewServer()

	if err := s.DidChangeConfiguration(t.Context(), &protocol.DidChangeConfigurationParams{
		Settings: protocol.LSPAny(`{"zever":{"codelens":{"references":false}}}`),
	}); err != nil {
		t.Fatalf("DidChangeConfiguration: %v", err)
	}

	if got := s.currentSettings(); got.CodeLensReferences {
		t.Errorf("settings = %+v, want the codelens flip stored despite no client", got)
	}
}

// ServerInlayGateSrc declares an entity (which receives the implicit-public
// module hint in a two-module schema) next to an rpc with a recognized
// error code, so both hint families fire on one file.
const serverInlayGateSrc = "entity Task {\n" +
	"\tid: uuid @primary\n" +
	"}\n" +
	"\n" +
	"service TaskService {\n" +
	"\trpc GetTask(id: uuid) -> Task {\n" +
	"\t\tauth: none\n" +
	"\t\terrors: { not_found }\n" +
	"\t}\n" +
	"}\n"

// inlayGateFile compiles a two-module schema returning the parsed AST of
// the public file, which is where both hint families land.
func inlayGateFile(t *testing.T) (*ir.Schema, *ast.File) {
	t.Helper()

	result, diags := compile.Compile(map[string]string{
		"schema/app.zen":             serverInlayGateSrc,
		"schema/billing/invoice.zen": "entity Invoice {\n\tid: uuid @primary\n}\n",
	})
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	file, _ := parser.New("schema/app.zen", []byte(serverInlayGateSrc)).ParseFile()

	return result.Schema, file
}

// TestSettingsGateInlayHints proves each settings flag removes exactly its
// own hint family, and both flags together suppress all hints.
func TestSettingsGateInlayHints(t *testing.T) {
	t.Parallel()

	schema, file := inlayGateFile(t)

	for _, tc := range []struct {
		name           string
		settings       Settings
		wantModuleHint bool
		wantErrorHint  bool
	}{
		{"defaults, both families on", defaultSettings(), true, true},
		{"implicitModule off", Settings{ErrorCaseHints: true}, false, true},
		{"errorCases off", Settings{ImplicitModuleHints: true}, true, false},
		{"both off", Settings{}, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			hints := inlayHintsFor(schema, file, tc.settings)

			var hasModule, hasError bool

			for _, h := range hints {
				label, _ := h.Label.(protocol.String)
				switch string(label) {
				case " (module: public)":
					hasModule = true
				case " (404 NOT_FOUND)":
					hasError = true
				}
			}

			if hasModule != tc.wantModuleHint {
				t.Errorf("module hint present = %v, want %v (hints: %+v)", hasModule, tc.wantModuleHint, hints)
			}

			if hasError != tc.wantErrorHint {
				t.Errorf("error-case hint present = %v, want %v (hints: %+v)", hasError, tc.wantErrorHint, hints)
			}
		})
	}
}

// TestSettingsDebounceReachesPublisher proves the settings debounce-delay
// reaches the publisher end to end: a didChangeConfiguration, then the next
// edit burst schedules with the configured delay, not the package default.
func TestSettingsDebounceReachesPublisher(t *testing.T) {
	s := NewServer()
	clock := withFakeClock(s.publisher)

	if err := s.DidChangeConfiguration(t.Context(), &protocol.DidChangeConfigurationParams{
		Settings: protocol.LSPAny(`{"zever":{"diagnostics":{"debounceMs":40}}}`),
	}); err != nil {
		t.Fatalf("DidChangeConfiguration: %v", err)
	}

	openDoc(t, s, serverDocA, serverValidDoc)

	whole := protocol.TextDocumentContentChangeEvent(
		&protocol.TextDocumentContentChangeWholeDocument{Text: "entity Changed {}\n"},
	)

	if err := s.DidChange(t.Context(), &protocol.DidChangeTextDocumentParams{
		TextDocument:   protocol.VersionedTextDocumentIdentifier{TextDocumentIdentifier: protocol.TextDocumentIdentifier{URI: uri.URI(serverDocA)}},
		ContentChanges: []protocol.TextDocumentContentChangeEvent{whole},
	}); err != nil {
		t.Fatalf("DidChange: %v", err)
	}

	if got := clock.lastDelay(); got != 40*time.Millisecond {
		t.Errorf("debounce delay = %v, want 40ms from settings", got)
	}
}
