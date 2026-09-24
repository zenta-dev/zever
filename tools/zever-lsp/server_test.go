package main

import (
	"errors"
	"strings"
	"testing"

	"go.lsp.dev/protocol"
	"go.lsp.dev/uri"
)

// serverValidDoc is a schema that compiles cleanly: two entities linked by a
// relation. Line map (0-based) used by cursor positions below:
//
//	0: entity User {
//	1: \tid: uuid @primary
//	2: \temail: string @unique
//	3: }
//	4: (blank)
//	5: entity Task {
//	6: \tid: uuid @primary
//	7: \tuser_id: uuid
//	8: \ttitle: string
//	9: (blank)
//	10: \tbelongs_to user: User @foreign_key(user_id)
//	11: }
const serverValidDoc = `entity User {
	id: uuid @primary
	email: string @unique
}

entity Task {
	id: uuid @primary
	user_id: uuid
	title: string

	belongs_to user: User @foreign_key(user_id)
}
`

// serverBrokenDoc parses standalone but never resolves: the field type is
// unknown, so whole-workspace Recompile reports errors and handlers must
// fall back to per-file behavior.
const serverBrokenDoc = `entity User {
	id: strnig
}
`

const (
	serverDocA     = "file:///tmp/zever-lsp-server-a.zen"
	serverDocB     = "file:///tmp/zever-lsp-server-b.zen"
	serverUnknown  = "file:///tmp/zever-lsp-server-unknown.zen"
	serverEntityAt = 8 // character offset of "User" on line 0 of serverValidDoc
)

// diagURI returns the client-facing URI string diagnostics are published
// under for a test document URI.
func diagURI(t *testing.T, docURI string) string {
	t.Helper()

	path, ok := uriToPath(docURI)
	if !ok {
		t.Fatalf("uriToPath(%s) failed", docURI)
	}

	return string(pathToURI(path))
}

// openDoc registers content for uri on s, failing the test on error.
func openDoc(t *testing.T, s *Server, docURI string, content string) {
	t.Helper()

	u := uri.URI(docURI)

	if err := s.DidOpen(t.Context(), &protocol.DidOpenTextDocumentParams{
		TextDocument: protocol.TextDocumentItem{URI: u, Text: content},
	}); err != nil {
		t.Fatalf("DidOpen(%s): %v", docURI, err)
	}
}

func TestInitialize_capabilities(t *testing.T) {
	s := NewServer()

	res, err := s.Initialize(t.Context(), &protocol.InitializeParams{})
	if err != nil {
		t.Fatalf("Initialize: %v", err)
	}

	if res.ServerInfo.Name != serverName {
		t.Errorf("ServerInfo.Name = %q, want %q", res.ServerInfo.Name, serverName)
	}

	version, ok := res.ServerInfo.Version.Get()
	if !ok || version != serverVersion {
		t.Errorf("ServerInfo.Version = (%q, %v), want (%q, true)", version, ok, serverVersion)
	}

	caps := res.Capabilities

	syncOpts, syncOK := caps.TextDocumentSync.(*protocol.TextDocumentSyncOptions)
	if !syncOK {
		t.Fatalf("TextDocumentSync = %T, want *TextDocumentSyncOptions", caps.TextDocumentSync)
	}

	if syncOpts.OpenClose == nil || !*syncOpts.OpenClose {
		t.Error("TextDocumentSync.OpenClose not enabled")
	}

	if syncOpts.Change == nil || *syncOpts.Change != protocol.TextDocumentSyncKindFull {
		t.Errorf("TextDocumentSync.Change = %+v, want Full", syncOpts.Change)
	}

	save, saveOK := syncOpts.Save.(protocol.Boolean)
	if !saveOK || !bool(save) {
		t.Errorf("TextDocumentSync.Save = %#v, want Boolean(true)", syncOpts.Save)
	}

	for _, tc := range []struct {
		name string
		got  any
	}{
		{"DocumentSymbolProvider", caps.DocumentSymbolProvider},
		{"WorkspaceSymbolProvider", caps.WorkspaceSymbolProvider},
		{"DefinitionProvider", caps.DefinitionProvider},
		{"HoverProvider", caps.HoverProvider},
		{"DocumentFormattingProvider", caps.DocumentFormattingProvider},
		{"DocumentRangeFormattingProvider", caps.DocumentRangeFormattingProvider},
		{"CodeActionProvider", caps.CodeActionProvider},
		{"ReferencesProvider", caps.ReferencesProvider},
		{"FoldingRangeProvider", caps.FoldingRangeProvider},
		{"DocumentHighlightProvider", caps.DocumentHighlightProvider},
		{"InlayHintProvider", caps.InlayHintProvider},
	} {
		b, capOK := tc.got.(protocol.Boolean)
		if !capOK || !bool(b) {
			t.Errorf("%s = %#v, want Boolean(true)", tc.name, tc.got)
		}
	}

	if caps.CompletionProvider == nil {
		t.Fatal("CompletionProvider is nil")
	} else {
		want := []string{"@", ":", " "}
		got := caps.CompletionProvider.TriggerCharacters

		if len(got) != len(want) {
			t.Errorf("CompletionProvider.TriggerCharacters = %v, want %v", got, want)
		} else {
			for i := range want {
				if got[i] != want[i] {
					t.Errorf("CompletionProvider.TriggerCharacters = %v, want %v", got, want)

					break
				}
			}
		}

		if caps.CompletionProvider.ResolveProvider == nil || !*caps.CompletionProvider.ResolveProvider {
			t.Error("CompletionProvider.ResolveProvider not enabled")
		}
	}

	if caps.SignatureHelpProvider == nil {
		t.Fatal("SignatureHelpProvider is nil")
	} else if len(caps.SignatureHelpProvider.TriggerCharacters) != 2 {
		t.Errorf("SignatureHelpProvider.TriggerCharacters = %v, want [( ,]",
			caps.SignatureHelpProvider.TriggerCharacters)
	}

	rename, ok := caps.RenameProvider.(*protocol.RenameOptions)
	if !ok {
		t.Fatalf("RenameProvider = %T, want *RenameOptions", caps.RenameProvider)
	}

	if rename.PrepareProvider == nil || !*rename.PrepareProvider {
		t.Error("RenameProvider.PrepareProvider not enabled")
	}

	sem, ok := caps.SemanticTokensProvider.(*protocol.SemanticTokensOptions)
	if !ok {
		t.Fatalf("SemanticTokensProvider = %T, want *SemanticTokensOptions", caps.SemanticTokensProvider)
	}

	if len(sem.Legend.TokenTypes) == 0 {
		t.Error("SemanticTokensProvider.Legend has no token types")
	}

	if full, ok := sem.Full.(protocol.Boolean); !ok || !bool(full) {
		t.Errorf("SemanticTokensProvider.Full = %#v, want Boolean(true)", sem.Full)
	}
}

func TestInitialize_rootPrecedence(t *testing.T) {
	foldersURI := uri.File("/ws/folders")
	rootURI := uri.File("/ws/uri")

	tests := []struct {
		name   string
		params *protocol.InitializeParams
		want   string
	}{
		{
			name:   "no root",
			params: &protocol.InitializeParams{},
			want:   "",
		},
		{
			name:   "nil params",
			params: nil,
			want:   "",
		},
		{
			name: "rootPath only",
			params: &protocol.InitializeParams{
				RootPath: protocol.NewNullable("/ws/path"), //nolint:staticcheck // exercises pre-3.17 rootPath fallback
			},
			want: "/ws/path",
		},
		{
			name: "rootUri beats rootPath",
			params: &protocol.InitializeParams{
				RootPath: protocol.NewNullable("/ws/path"), //nolint:staticcheck // exercises pre-3.17 rootPath fallback
				RootURI:  &rootURI,                         //nolint:staticcheck // exercises pre-3.17 rootUri fallback
			},
			want: "/ws/uri",
		},
		{
			name: "workspaceFolders beats rootUri and rootPath",
			params: &protocol.InitializeParams{
				RootPath:         protocol.NewNullable("/ws/path"), //nolint:staticcheck // exercises pre-3.17 rootPath fallback
				RootURI:          &rootURI,                         //nolint:staticcheck // exercises pre-3.17 rootUri fallback
				WorkspaceFolders: protocol.NewNullable([]protocol.WorkspaceFolder{{URI: foldersURI, Name: "f"}}),
			},
			want: "/ws/folders",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := NewServer()

			if _, err := s.Initialize(t.Context(), tt.params); err != nil {
				t.Fatalf("Initialize: %v", err)
			}

			if s.ws.rootPath != tt.want {
				t.Errorf("rootPath = %q, want %q", s.ws.rootPath, tt.want)
			}
		})
	}
}

func TestLifecycle_initializedShutdownExitSetTrace(t *testing.T) {
	s := NewServer()

	if err := s.Initialized(t.Context(), &protocol.InitializedParams{}); err != nil {
		t.Fatalf("Initialized: %v", err)
	}

	if err := s.Shutdown(t.Context()); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}

	if err := s.Exit(t.Context()); err != nil {
		t.Fatalf("Exit: %v", err)
	}

	if err := s.SetTrace(t.Context(), &protocol.SetTraceParams{Value: protocol.TraceValueVerbose}); err != nil {
		t.Fatalf("SetTrace: %v", err)
	}

	s.mu.Lock()
	got := s.trace
	s.mu.Unlock()

	if got != protocol.TraceValueVerbose {
		t.Errorf("trace = %q, want verbose", got)
	}

	if err := s.SetTrace(t.Context(), nil); err != nil {
		t.Fatalf("SetTrace(nil): %v", err)
	}
}

func TestDidOpenChangeSaveClose(t *testing.T) {
	s := NewServer()
	ctx := t.Context()
	u := uri.URI(serverDocA)

	if err := s.DidOpen(ctx, &protocol.DidOpenTextDocumentParams{
		TextDocument: protocol.TextDocumentItem{URI: u, Text: serverValidDoc},
	}); err != nil {
		t.Fatalf("DidOpen: %v", err)
	}

	if got, ok := s.ws.DocContent(string(u)); !ok || got != serverValidDoc {
		t.Fatalf("DocContent after DidOpen = (%q, %v), want content", got, ok)
	}

	whole := protocol.TextDocumentContentChangeEvent(
		&protocol.TextDocumentContentChangeWholeDocument{Text: "entity Changed {}\n"},
	)

	if err := s.DidChange(ctx, &protocol.DidChangeTextDocumentParams{
		TextDocument:   protocol.VersionedTextDocumentIdentifier{TextDocumentIdentifier: protocol.TextDocumentIdentifier{URI: u}},
		ContentChanges: []protocol.TextDocumentContentChangeEvent{whole},
	}); err != nil {
		t.Fatalf("DidChange: %v", err)
	}

	if got, _ := s.ws.DocContent(string(u)); got != "entity Changed {}\n" {
		t.Errorf("DocContent after DidChange = %q, want updated text", got)
	}

	partial := protocol.TextDocumentContentChangeEvent(
		&protocol.TextDocumentContentChangePartial{Text: "ignored"},
	)

	if err := s.DidChange(ctx, &protocol.DidChangeTextDocumentParams{
		TextDocument:   protocol.VersionedTextDocumentIdentifier{TextDocumentIdentifier: protocol.TextDocumentIdentifier{URI: u}},
		ContentChanges: []protocol.TextDocumentContentChangeEvent{partial},
	}); err != nil {
		t.Fatalf("DidChange partial: %v", err)
	}

	if got, _ := s.ws.DocContent(string(u)); got != "entity Changed {}\n" {
		t.Errorf("DocContent after partial DidChange = %q, want unchanged text", got)
	}

	if err := s.DidChange(ctx, &protocol.DidChangeTextDocumentParams{
		TextDocument: protocol.VersionedTextDocumentIdentifier{TextDocumentIdentifier: protocol.TextDocumentIdentifier{URI: u}},
	}); err != nil {
		t.Fatalf("DidChange empty: %v", err)
	}

	if err := s.DidSave(ctx, &protocol.DidSaveTextDocumentParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: u},
	}); err != nil {
		t.Fatalf("DidSave: %v", err)
	}

	if err := s.DidClose(ctx, &protocol.DidCloseTextDocumentParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: u},
	}); err != nil {
		t.Fatalf("DidClose: %v", err)
	}

	if _, ok := s.ws.DocContent(string(u)); ok {
		t.Error("DocContent after DidClose still present, want dropped")
	}
}

func TestDidChangeWatchedFiles(t *testing.T) {
	s := NewServer()

	if err := s.DidChangeWatchedFiles(t.Context(), &protocol.DidChangeWatchedFilesParams{}); err != nil {
		t.Fatalf("DidChangeWatchedFiles: %v", err)
	}
}

func TestDocumentSymbol(t *testing.T) {
	t.Run("schema path", func(t *testing.T) {
		s := NewServer()
		openDoc(t, s, serverDocA, serverValidDoc)

		res, err := s.DocumentSymbol(t.Context(), &protocol.DocumentSymbolParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: uri.URI(serverDocA)},
		})
		if err != nil {
			t.Fatalf("DocumentSymbol: %v", err)
		}

		syms, ok := res.(protocol.DocumentSymbolSlice)
		if !ok {
			t.Fatalf("DocumentSymbol result = %T, want DocumentSymbolSlice", res)
		}

		names := map[string]bool{}
		for _, sym := range syms {
			names[sym.Name] = true
		}

		for _, want := range []string{"User", "Task"} {
			if !names[want] {
				t.Errorf("symbols missing %q: %+v", want, syms)
			}
		}
	})

	t.Run("AST fallback when workspace has errors", func(t *testing.T) {
		s := NewServer()
		openDoc(t, s, serverDocA, serverBrokenDoc)

		res, err := s.DocumentSymbol(t.Context(), &protocol.DocumentSymbolParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: uri.URI(serverDocA)},
		})
		if err != nil {
			t.Fatalf("DocumentSymbol: %v", err)
		}

		syms, ok := res.(protocol.DocumentSymbolSlice)
		if !ok {
			t.Fatalf("DocumentSymbol result = %T, want DocumentSymbolSlice", res)
		}

		if len(syms) == 0 {
			t.Error("AST fallback returned no symbols, want outline to stay useful mid-edit")
		}
	})

	t.Run("unknown document returns empty", func(t *testing.T) {
		s := NewServer()

		res, err := s.DocumentSymbol(t.Context(), &protocol.DocumentSymbolParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: uri.URI(serverUnknown)},
		})
		if err != nil {
			t.Fatalf("DocumentSymbol: %v", err)
		}

		syms, ok := res.(protocol.DocumentSymbolSlice)
		if !ok {
			t.Fatalf("DocumentSymbol result = %T, want DocumentSymbolSlice", res)
		}

		if syms == nil || len(syms) != 0 {
			t.Errorf("DocumentSymbol unknown doc = %+v, want empty non-nil", syms)
		}
	})
}

func TestSymbols(t *testing.T) {
	s := NewServer()
	openDoc(t, s, serverDocA, serverValidDoc)

	t.Run("query filters", func(t *testing.T) {
		res, err := s.Symbols(t.Context(), &protocol.WorkspaceSymbolParams{Query: "User"})
		if err != nil {
			t.Fatalf("Symbols: %v", err)
		}

		infos, ok := res.(protocol.SymbolInformationSlice)
		if !ok {
			t.Fatalf("Symbols result = %T, want SymbolInformationSlice", res)
		}

		found := false

		for _, info := range infos {
			if info.Name == "User" {
				found = true
			}

			if !strings.Contains(strings.ToLower(info.Name), "user") {
				t.Errorf("Symbols(Query=User) returned unrelated %q", info.Name)
			}
		}

		if !found {
			t.Errorf("Symbols(Query=User) missing User: %+v", infos)
		}
	})

	t.Run("empty query returns all", func(t *testing.T) {
		res, err := s.Symbols(t.Context(), &protocol.WorkspaceSymbolParams{})
		if err != nil {
			t.Fatalf("Symbols: %v", err)
		}

		if infos, ok := res.(protocol.SymbolInformationSlice); !ok || len(infos) == 0 {
			t.Errorf("Symbols(empty query) = %+v, want all symbols", res)
		}
	})

	t.Run("broken workspace falls back without error", func(t *testing.T) {
		broken := NewServer()
		openDoc(t, broken, serverDocA, serverBrokenDoc)

		if _, err := broken.Symbols(t.Context(), &protocol.WorkspaceSymbolParams{Query: "User"}); err != nil {
			t.Fatalf("Symbols on broken workspace: %v", err)
		}
	})
}

func TestDefinition(t *testing.T) {
	t.Run("relation target resolves", func(t *testing.T) {
		s := NewServer()
		openDoc(t, s, serverDocA, serverValidDoc)

		res, err := s.Definition(t.Context(), &protocol.DefinitionParams{
			TextDocumentPositionParams: protocol.TextDocumentPositionParams{
				TextDocument: protocol.TextDocumentIdentifier{URI: uri.URI(serverDocA)},
				Position:     protocol.Position{Line: 10, Character: 19},
			},
		})
		if err != nil {
			t.Fatalf("Definition: %v", err)
		}

		loc, ok := res.(*protocol.Location)
		if !ok || loc == nil {
			t.Fatalf("Definition result = %#v, want *Location", res)
		}

		if loc.Range.Start.Line != 0 {
			t.Errorf("Definition lands on line %d, want 0 (entity User declaration)", loc.Range.Start.Line)
		}
	})

	t.Run("nothing jumpable is null", func(t *testing.T) {
		s := NewServer()
		openDoc(t, s, serverDocA, serverValidDoc)

		res, err := s.Definition(t.Context(), &protocol.DefinitionParams{
			TextDocumentPositionParams: protocol.TextDocumentPositionParams{
				TextDocument: protocol.TextDocumentIdentifier{URI: uri.URI(serverDocA)},
				Position:     protocol.Position{Line: 4, Character: 0},
			},
		})
		if err != nil {
			t.Fatalf("Definition: %v", err)
		}

		if res != nil {
			t.Errorf("Definition on blank line = %#v, want null", res)
		}
	})

	t.Run("unknown document is null", func(t *testing.T) {
		s := NewServer()

		res, err := s.Definition(t.Context(), &protocol.DefinitionParams{
			TextDocumentPositionParams: protocol.TextDocumentPositionParams{
				TextDocument: protocol.TextDocumentIdentifier{URI: uri.URI(serverUnknown)},
				Position:     protocol.Position{Line: 0, Character: 0},
			},
		})
		if err != nil {
			t.Fatalf("Definition: %v", err)
		}

		if res != nil {
			t.Errorf("Definition unknown doc = %#v, want null", res)
		}
	})
}

func TestHover(t *testing.T) {
	t.Run("entity name hovers", func(t *testing.T) {
		s := NewServer()
		openDoc(t, s, serverDocA, serverValidDoc)

		res, err := s.Hover(t.Context(), &protocol.HoverParams{
			TextDocumentPositionParams: protocol.TextDocumentPositionParams{
				TextDocument: protocol.TextDocumentIdentifier{URI: uri.URI(serverDocA)},
				Position:     protocol.Position{Line: 0, Character: serverEntityAt},
			},
		})
		if err != nil {
			t.Fatalf("Hover: %v", err)
		}

		if res == nil {
			t.Fatal("Hover on entity name is null, want hover")
		}
	})

	t.Run("blank line has no hover", func(t *testing.T) {
		s := NewServer()
		openDoc(t, s, serverDocA, serverValidDoc)

		res, err := s.Hover(t.Context(), &protocol.HoverParams{
			TextDocumentPositionParams: protocol.TextDocumentPositionParams{
				TextDocument: protocol.TextDocumentIdentifier{URI: uri.URI(serverDocA)},
				Position:     protocol.Position{Line: 4, Character: 0},
			},
		})
		if err != nil {
			t.Fatalf("Hover: %v", err)
		}

		if res != nil {
			t.Errorf("Hover on blank line = %+v, want null", res)
		}
	})

	t.Run("unknown document is null", func(t *testing.T) {
		s := NewServer()

		res, err := s.Hover(t.Context(), &protocol.HoverParams{
			TextDocumentPositionParams: protocol.TextDocumentPositionParams{
				TextDocument: protocol.TextDocumentIdentifier{URI: uri.URI(serverUnknown)},
				Position:     protocol.Position{Line: 0, Character: 0},
			},
		})
		if err != nil {
			t.Fatalf("Hover: %v", err)
		}

		if res != nil {
			t.Errorf("Hover unknown doc = %+v, want null", res)
		}
	})
}

func TestCompletion(t *testing.T) {
	t.Run("field type slot completes scalars", func(t *testing.T) {
		s := NewServer()
		openDoc(t, s, serverDocA, "entity User {\n\tid: ")

		res, err := s.Completion(t.Context(), &protocol.CompletionParams{
			TextDocumentPositionParams: protocol.TextDocumentPositionParams{
				TextDocument: protocol.TextDocumentIdentifier{URI: uri.URI(serverDocA)},
				Position:     protocol.Position{Line: 1, Character: 5},
			},
		})
		if err != nil {
			t.Fatalf("Completion: %v", err)
		}

		items, ok := res.(protocol.CompletionItemSlice)
		if !ok || len(items) == 0 {
			t.Fatalf("Completion result = %#v, want non-empty CompletionItemSlice", res)
		}
	})

	t.Run("unknown document is null", func(t *testing.T) {
		s := NewServer()

		res, err := s.Completion(t.Context(), &protocol.CompletionParams{
			TextDocumentPositionParams: protocol.TextDocumentPositionParams{
				TextDocument: protocol.TextDocumentIdentifier{URI: uri.URI(serverUnknown)},
				Position:     protocol.Position{Line: 0, Character: 0},
			},
		})
		if err != nil {
			t.Fatalf("Completion: %v", err)
		}

		if res != nil {
			t.Errorf("Completion unknown doc = %#v, want null", res)
		}
	})

	t.Run("no vocabulary slot is null", func(t *testing.T) {
		s := NewServer()
		openDoc(t, s, serverDocA, "service S {\n}\n")

		res, err := s.Completion(t.Context(), &protocol.CompletionParams{
			TextDocumentPositionParams: protocol.TextDocumentPositionParams{
				TextDocument: protocol.TextDocumentIdentifier{URI: uri.URI(serverDocA)},
				Position:     protocol.Position{Line: 0, Character: 9},
			},
		})
		if err != nil {
			t.Fatalf("Completion: %v", err)
		}

		if res != nil {
			t.Errorf("Completion with no vocabulary = %#v, want null", res)
		}
	})
}

func TestCompletionResolve(t *testing.T) {
	s := NewServer()

	item := &protocol.CompletionItem{Label: "entity"}

	res, err := s.CompletionResolve(t.Context(), item)
	if err != nil {
		t.Fatalf("CompletionResolve: %v", err)
	}

	if res == nil {
		t.Fatal("CompletionResolve returned nil")
	}
}

func TestSignatureHelp(t *testing.T) {
	t.Run("validate call completes signature", func(t *testing.T) {
		s := NewServer()
		content := "entity User {\n\tid: string @validate("
		openDoc(t, s, serverDocA, content)

		res, err := s.SignatureHelp(t.Context(), &protocol.SignatureHelpParams{
			TextDocumentPositionParams: protocol.TextDocumentPositionParams{
				TextDocument: protocol.TextDocumentIdentifier{URI: uri.URI(serverDocA)},
				Position:     protocol.Position{Line: 1, Character: 22},
			},
		})
		if err != nil {
			t.Fatalf("SignatureHelp: %v", err)
		}

		if res == nil || len(res.Signatures) != 1 {
			t.Fatalf("SignatureHelp = %+v, want one signature", res)
		}
	})

	t.Run("outside a call is null", func(t *testing.T) {
		s := NewServer()
		openDoc(t, s, serverDocA, serverValidDoc)

		res, err := s.SignatureHelp(t.Context(), &protocol.SignatureHelpParams{
			TextDocumentPositionParams: protocol.TextDocumentPositionParams{
				TextDocument: protocol.TextDocumentIdentifier{URI: uri.URI(serverDocA)},
				Position:     protocol.Position{Line: 0, Character: 0},
			},
		})
		if err != nil {
			t.Fatalf("SignatureHelp: %v", err)
		}

		if res != nil {
			t.Errorf("SignatureHelp outside call = %+v, want null", res)
		}
	})

	t.Run("unknown document is null", func(t *testing.T) {
		s := NewServer()

		res, err := s.SignatureHelp(t.Context(), &protocol.SignatureHelpParams{
			TextDocumentPositionParams: protocol.TextDocumentPositionParams{
				TextDocument: protocol.TextDocumentIdentifier{URI: uri.URI(serverUnknown)},
				Position:     protocol.Position{Line: 0, Character: 0},
			},
		})
		if err != nil {
			t.Fatalf("SignatureHelp: %v", err)
		}

		if res != nil {
			t.Errorf("SignatureHelp unknown doc = %+v, want null", res)
		}
	})
}

func TestFormatting(t *testing.T) {
	const unformatted = "entity User {\nid: uuid @primary\n}\n"

	t.Run("rewrites unformatted", func(t *testing.T) {
		s := NewServer()
		openDoc(t, s, serverDocA, unformatted)

		edits, err := s.Formatting(t.Context(), &protocol.DocumentFormattingParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: uri.URI(serverDocA)},
		})
		if err != nil {
			t.Fatalf("Formatting: %v", err)
		}

		if len(edits) == 0 {
			t.Error("Formatting unformatted doc returned no edits")
		}
	})

	t.Run("unknown document is null", func(t *testing.T) {
		s := NewServer()

		edits, err := s.Formatting(t.Context(), &protocol.DocumentFormattingParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: uri.URI(serverUnknown)},
		})
		if err != nil {
			t.Fatalf("Formatting: %v", err)
		}

		if edits != nil {
			t.Errorf("Formatting unknown doc = %+v, want null", edits)
		}
	})
}

func TestRangeFormatting(t *testing.T) {
	const unformatted = "entity User {\nid: uuid @primary\n}\n"

	t.Run("narrows to range", func(t *testing.T) {
		s := NewServer()
		openDoc(t, s, serverDocA, unformatted)

		full, err := s.Formatting(t.Context(), &protocol.DocumentFormattingParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: uri.URI(serverDocA)},
		})
		if err != nil {
			t.Fatalf("Formatting: %v", err)
		}

		narrow, err := s.RangeFormatting(t.Context(), &protocol.DocumentRangeFormattingParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: uri.URI(serverDocA)},
			Range: protocol.Range{
				Start: protocol.Position{Line: 1, Character: 0},
				End:   protocol.Position{Line: 1, Character: 20},
			},
		})
		if err != nil {
			t.Fatalf("RangeFormatting: %v", err)
		}

		if len(narrow) == 0 {
			t.Error("RangeFormatting returned no edits for a line needing indent")
		}

		if len(narrow) > len(full) {
			t.Errorf("RangeFormatting returned %d edits, more than full %d", len(narrow), len(full))
		}
	})

	t.Run("unknown document is null", func(t *testing.T) {
		s := NewServer()

		edits, err := s.RangeFormatting(t.Context(), &protocol.DocumentRangeFormattingParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: uri.URI(serverDocA)},
			Range: protocol.Range{
				Start: protocol.Position{Line: 0, Character: 0},
				End:   protocol.Position{Line: 0, Character: 1},
			},
		})
		if err != nil {
			t.Fatalf("RangeFormatting: %v", err)
		}

		if edits != nil {
			t.Errorf("RangeFormatting unknown doc = %+v, want null", edits)
		}
	})
}

func TestCodeAction(t *testing.T) {
	diagRange := protocol.Range{
		Start: protocol.Position{Line: 1, Character: 5},
		End:   protocol.Position{Line: 1, Character: 11},
	}

	t.Run("unknown scalar offers fix", func(t *testing.T) {
		s := NewServer()
		openDoc(t, s, serverDocA, serverBrokenDoc)

		actions, err := s.CodeAction(t.Context(), &protocol.CodeActionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: uri.URI(serverDocA)},
			Range:        diagRange,
			Context: protocol.CodeActionContext{
				Diagnostics: []protocol.Diagnostic{{
					Range:   diagRange,
					Message: protocol.String(`unknown field type "strnig"`),
				}},
			},
		})
		if err != nil {
			t.Fatalf("CodeAction: %v", err)
		}

		if len(actions) == 0 {
			t.Fatal("CodeAction returned no fixes, want typo fix")
		}
	})

	t.Run("no diagnostics means null", func(t *testing.T) {
		s := NewServer()
		openDoc(t, s, serverDocA, serverValidDoc)

		actions, err := s.CodeAction(t.Context(), &protocol.CodeActionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: uri.URI(serverDocA)},
			Range:        diagRange,
			Context:      protocol.CodeActionContext{},
		})
		if err != nil {
			t.Fatalf("CodeAction: %v", err)
		}

		if actions != nil {
			t.Errorf("CodeAction with no diagnostics = %+v, want null", actions)
		}
	})

	t.Run("unrelated diagnostic means null", func(t *testing.T) {
		s := NewServer()
		openDoc(t, s, serverDocA, serverValidDoc)

		actions, err := s.CodeAction(t.Context(), &protocol.CodeActionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: uri.URI(serverDocA)},
			Range:        diagRange,
			Context: protocol.CodeActionContext{
				Diagnostics: []protocol.Diagnostic{{
					Range:   diagRange,
					Message: protocol.String("something else entirely"),
				}},
			},
		})
		if err != nil {
			t.Fatalf("CodeAction: %v", err)
		}

		if actions != nil {
			t.Errorf("CodeAction with unrelated diagnostic = %+v, want null", actions)
		}
	})
}

func TestPrepareRename(t *testing.T) {
	t.Run("entity name is renameable", func(t *testing.T) {
		s := NewServer()
		openDoc(t, s, serverDocA, serverValidDoc)

		res, err := s.PrepareRename(t.Context(), &protocol.PrepareRenameParams{
			TextDocumentPositionParams: protocol.TextDocumentPositionParams{
				TextDocument: protocol.TextDocumentIdentifier{URI: uri.URI(serverDocA)},
				Position:     protocol.Position{Line: 0, Character: serverEntityAt},
			},
		})
		if err != nil {
			t.Fatalf("PrepareRename: %v", err)
		}

		rng, ok := res.(*protocol.Range)
		if !ok || rng == nil {
			t.Fatalf("PrepareRename result = %#v, want *Range", res)
		}

		if rng.Start.Line != 0 || rng.Start.Character != 7 || rng.End.Character != 11 {
			t.Errorf("PrepareRename range = %+v, want line 0 chars 7-11", rng)
		}
	})

	t.Run("blank line refuses", func(t *testing.T) {
		s := NewServer()
		openDoc(t, s, serverDocA, serverValidDoc)

		res, err := s.PrepareRename(t.Context(), &protocol.PrepareRenameParams{
			TextDocumentPositionParams: protocol.TextDocumentPositionParams{
				TextDocument: protocol.TextDocumentIdentifier{URI: uri.URI(serverDocA)},
				Position:     protocol.Position{Line: 4, Character: 0},
			},
		})
		if err != nil {
			t.Fatalf("PrepareRename: %v", err)
		}

		if res != nil {
			t.Errorf("PrepareRename on blank line = %#v, want null", res)
		}
	})

	t.Run("unknown document is null", func(t *testing.T) {
		s := NewServer()

		res, err := s.PrepareRename(t.Context(), &protocol.PrepareRenameParams{
			TextDocumentPositionParams: protocol.TextDocumentPositionParams{
				TextDocument: protocol.TextDocumentIdentifier{URI: uri.URI(serverUnknown)},
				Position:     protocol.Position{Line: 0, Character: 0},
			},
		})
		if err != nil {
			t.Fatalf("PrepareRename: %v", err)
		}

		if res != nil {
			t.Errorf("PrepareRename unknown doc = %#v, want null", res)
		}
	})
}

func TestRename(t *testing.T) {
	t.Run("entity renames across workspace", func(t *testing.T) {
		s := NewServer()
		openDoc(t, s, serverDocA, serverValidDoc)
		openDoc(t, s, serverDocB, "entity Audit {\n\tid: uuid @primary\n\n\tbelongs_to owner: User @foreign_key(owner_id)\n}\n")

		edit, err := s.Rename(t.Context(), &protocol.RenameParams{
			TextDocumentPositionParams: protocol.TextDocumentPositionParams{
				TextDocument: protocol.TextDocumentIdentifier{URI: uri.URI(serverDocA)},
				Position:     protocol.Position{Line: 0, Character: serverEntityAt},
			},
			NewName: "Customer",
		})
		if err != nil {
			t.Fatalf("Rename: %v", err)
		}

		if edit == nil || len(edit.Changes) == 0 {
			t.Fatalf("Rename returned no edits: %+v", edit)
		}

		seen := false

		for _, edits := range edit.Changes {
			for _, e := range edits {
				if e.NewText == "Customer" {
					seen = true
				}
			}
		}

		if !seen {
			t.Errorf("Rename edits lack NewText Customer: %+v", edit.Changes)
		}
	})

	t.Run("non-symbol fails loudly", func(t *testing.T) {
		s := NewServer()
		openDoc(t, s, serverDocA, serverValidDoc)

		_, err := s.Rename(t.Context(), &protocol.RenameParams{
			TextDocumentPositionParams: protocol.TextDocumentPositionParams{
				TextDocument: protocol.TextDocumentIdentifier{URI: uri.URI(serverDocA)},
				Position:     protocol.Position{Line: 4, Character: 0},
			},
			NewName: "Customer",
		})
		if !errors.Is(err, errRenameUnsupported) {
			t.Errorf("Rename on blank line error = %v, want errRenameUnsupported", err)
		}
	})

	t.Run("empty new name rejected", func(t *testing.T) {
		s := NewServer()
		openDoc(t, s, serverDocA, serverValidDoc)

		_, err := s.Rename(t.Context(), &protocol.RenameParams{
			TextDocumentPositionParams: protocol.TextDocumentPositionParams{
				TextDocument: protocol.TextDocumentIdentifier{URI: uri.URI(serverDocA)},
				Position:     protocol.Position{Line: 0, Character: serverEntityAt},
			},
		})
		if !errors.Is(err, errRenameEmptyName) {
			t.Errorf("Rename empty name error = %v, want errRenameEmptyName", err)
		}
	})

	t.Run("invalid identifier rejected", func(t *testing.T) {
		s := NewServer()
		openDoc(t, s, serverDocA, serverValidDoc)

		_, err := s.Rename(t.Context(), &protocol.RenameParams{
			TextDocumentPositionParams: protocol.TextDocumentPositionParams{
				TextDocument: protocol.TextDocumentIdentifier{URI: uri.URI(serverDocA)},
				Position:     protocol.Position{Line: 0, Character: serverEntityAt},
			},
			NewName: "9bad",
		})
		if !errors.Is(err, errRenameInvalidIdent) {
			t.Errorf("Rename invalid ident error = %v, want errRenameInvalidIdent", err)
		}
	})

	t.Run("every symbol kind dispatches", func(t *testing.T) {
		const serviceDoc = `service UserService {
	rpc GetUser(id: uuid) -> User {
		http: GET "/users/{id}"
		auth: required
	}
}
`
		const jobDoc = `job CleanupJob(id: uuid) {
	queue: "default"
}

schedule Nightly {
	cron: "0 0 * * *"
	dispatch: CleanupJob(id: "x")
}
`

		tests := []struct {
			name     string
			docURI   string
			content  string
			position protocol.Position
			newName  string
		}{
			{"field", serverDocA, serverValidDoc, protocol.Position{Line: 2, Character: 2}, "email_address"},
			{"job", serverDocB, jobDoc, protocol.Position{Line: 0, Character: 5}, "PurgeJob"},
			{"service", serverDocA, serviceDoc, protocol.Position{Line: 0, Character: 9}, "AccountService"},
			{"rpc", serverDocA, serviceDoc, protocol.Position{Line: 1, Character: 6}, "FetchUser"},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				s := NewServer()
				openDoc(t, s, tt.docURI, tt.content)

				edit, err := s.Rename(t.Context(), &protocol.RenameParams{
					TextDocumentPositionParams: protocol.TextDocumentPositionParams{
						TextDocument: protocol.TextDocumentIdentifier{URI: uri.URI(tt.docURI)},
						Position:     tt.position,
					},
					NewName: tt.newName,
				})
				if err != nil {
					t.Fatalf("Rename(%s): %v", tt.name, err)
				}

				if edit == nil || len(edit.Changes) == 0 {
					t.Fatalf("Rename(%s) returned no edits", tt.name)
				}

				seen := false

				for _, edits := range edit.Changes {
					for _, e := range edits {
						if e.NewText == tt.newName {
							seen = true
						}
					}
				}

				if !seen {
					t.Errorf("Rename(%s) edits lack NewText %q: %+v", tt.name, tt.newName, edit.Changes)
				}
			})
		}
	})
}

func TestReferences(t *testing.T) {
	t.Run("entity finds declaration and uses", func(t *testing.T) {
		s := NewServer()
		openDoc(t, s, serverDocA, serverValidDoc)

		locs, err := s.References(t.Context(), &protocol.ReferenceParams{
			TextDocumentPositionParams: protocol.TextDocumentPositionParams{
				TextDocument: protocol.TextDocumentIdentifier{URI: uri.URI(serverDocA)},
				Position:     protocol.Position{Line: 0, Character: serverEntityAt},
			},
			Context: protocol.ReferenceContext{IncludeDeclaration: true},
		})
		if err != nil {
			t.Fatalf("References: %v", err)
		}

		if len(locs) < 2 {
			t.Errorf("References found %d locations, want declaration plus relation use", len(locs))
		}
	})

	t.Run("non-symbol is empty not error", func(t *testing.T) {
		s := NewServer()
		openDoc(t, s, serverDocA, serverValidDoc)

		locs, err := s.References(t.Context(), &protocol.ReferenceParams{
			TextDocumentPositionParams: protocol.TextDocumentPositionParams{
				TextDocument: protocol.TextDocumentIdentifier{URI: uri.URI(serverDocA)},
				Position:     protocol.Position{Line: 4, Character: 0},
			},
			Context: protocol.ReferenceContext{},
		})
		if err != nil {
			t.Fatalf("References: %v", err)
		}

		if locs == nil || len(locs) != 0 {
			t.Errorf("References on blank line = %+v, want empty non-nil", locs)
		}
	})

	t.Run("unknown document is empty", func(t *testing.T) {
		s := NewServer()

		locs, err := s.References(t.Context(), &protocol.ReferenceParams{
			TextDocumentPositionParams: protocol.TextDocumentPositionParams{
				TextDocument: protocol.TextDocumentIdentifier{URI: uri.URI(serverUnknown)},
				Position:     protocol.Position{Line: 0, Character: 0},
			},
			Context: protocol.ReferenceContext{},
		})
		if err != nil {
			t.Fatalf("References: %v", err)
		}

		if locs == nil || len(locs) != 0 {
			t.Errorf("References unknown doc = %+v, want empty non-nil", locs)
		}
	})
}

func TestFoldingRanges(t *testing.T) {
	t.Run("brace pairs fold", func(t *testing.T) {
		s := NewServer()
		openDoc(t, s, serverDocA, serverValidDoc)

		ranges, err := s.FoldingRanges(t.Context(), &protocol.FoldingRangeParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: uri.URI(serverDocA)},
		})
		if err != nil {
			t.Fatalf("FoldingRanges: %v", err)
		}

		if len(ranges) == 0 {
			t.Error("FoldingRanges returned no folds for a braced document")
		}
	})

	t.Run("unknown document is null", func(t *testing.T) {
		s := NewServer()

		ranges, err := s.FoldingRanges(t.Context(), &protocol.FoldingRangeParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: uri.URI(serverUnknown)},
		})
		if err != nil {
			t.Fatalf("FoldingRanges: %v", err)
		}

		if ranges != nil {
			t.Errorf("FoldingRanges unknown doc = %+v, want null", ranges)
		}
	})
}

func TestDocumentHighlight(t *testing.T) {
	t.Run("entity name highlights", func(t *testing.T) {
		s := NewServer()
		openDoc(t, s, serverDocA, serverValidDoc)

		highlights, err := s.DocumentHighlight(t.Context(), &protocol.DocumentHighlightParams{
			TextDocumentPositionParams: protocol.TextDocumentPositionParams{
				TextDocument: protocol.TextDocumentIdentifier{URI: uri.URI(serverDocA)},
				Position:     protocol.Position{Line: 0, Character: serverEntityAt},
			},
		})
		if err != nil {
			t.Fatalf("DocumentHighlight: %v", err)
		}

		if len(highlights) == 0 {
			t.Error("DocumentHighlight on entity name found nothing")
		}
	})

	t.Run("blank line finds nothing", func(t *testing.T) {
		s := NewServer()
		openDoc(t, s, serverDocA, serverValidDoc)

		_, err := s.DocumentHighlight(t.Context(), &protocol.DocumentHighlightParams{
			TextDocumentPositionParams: protocol.TextDocumentPositionParams{
				TextDocument: protocol.TextDocumentIdentifier{URI: uri.URI(serverDocA)},
				Position:     protocol.Position{Line: 4, Character: 0},
			},
		})
		if err != nil {
			t.Fatalf("DocumentHighlight: %v", err)
		}
	})

	t.Run("unknown document is safe", func(t *testing.T) {
		s := NewServer()

		_, err := s.DocumentHighlight(t.Context(), &protocol.DocumentHighlightParams{
			TextDocumentPositionParams: protocol.TextDocumentPositionParams{
				TextDocument: protocol.TextDocumentIdentifier{URI: uri.URI(serverUnknown)},
				Position:     protocol.Position{Line: 0, Character: 0},
			},
		})
		if err != nil {
			t.Fatalf("DocumentHighlight: %v", err)
		}
	})
}

func TestSemanticTokensFull(t *testing.T) {
	t.Run("document classifies", func(t *testing.T) {
		s := NewServer()
		openDoc(t, s, serverDocA, serverValidDoc)

		tokens, err := s.SemanticTokensFull(t.Context(), &protocol.SemanticTokensParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: uri.URI(serverDocA)},
		})
		if err != nil {
			t.Fatalf("SemanticTokensFull: %v", err)
		}

		if tokens == nil || len(tokens.Data) == 0 {
			t.Errorf("SemanticTokensFull returned no token data: %+v", tokens)
		}
	})

	t.Run("unknown document is null", func(t *testing.T) {
		s := NewServer()

		tokens, err := s.SemanticTokensFull(t.Context(), &protocol.SemanticTokensParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: uri.URI(serverUnknown)},
		})
		if err != nil {
			t.Fatalf("SemanticTokensFull: %v", err)
		}

		if tokens != nil {
			t.Errorf("SemanticTokensFull unknown doc = %+v, want null", tokens)
		}
	})
}

const serverInlayDoc = `entity Task {
	id: uuid @primary
}

service TaskService {
	rpc GetTask(id: uuid) -> Task {
		auth: none
		errors: { not_found }
	}
}
`

func TestInlayHint(t *testing.T) {
	t.Run("error cases hint", func(t *testing.T) {
		s := NewServer()
		openDoc(t, s, serverDocA, serverInlayDoc)

		hints, err := s.InlayHint(t.Context(), &protocol.InlayHintParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: uri.URI(serverDocA)},
			Range: protocol.Range{
				Start: protocol.Position{Line: 0, Character: 0},
				End:   protocol.Position{Line: 9, Character: 0},
			},
		})
		if err != nil {
			t.Fatalf("InlayHint: %v", err)
		}

		found := false

		for _, h := range hints {
			if label, ok := h.Label.(protocol.String); ok && strings.Contains(string(label), "404") {
				found = true
			}
		}

		if !found {
			t.Errorf("InlayHint missing 404 error-case hint: %+v", hints)
		}
	})

	t.Run("unknown document is null", func(t *testing.T) {
		s := NewServer()

		hints, err := s.InlayHint(t.Context(), &protocol.InlayHintParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: uri.URI(serverUnknown)},
			Range: protocol.Range{
				Start: protocol.Position{Line: 0, Character: 0},
				End:   protocol.Position{Line: 1, Character: 0},
			},
		})
		if err != nil {
			t.Fatalf("InlayHint: %v", err)
		}

		if hints != nil {
			t.Errorf("InlayHint unknown doc = %+v, want null", hints)
		}
	})
}

func TestInlayHintResolve(t *testing.T) {
	s := NewServer()

	hint := &protocol.InlayHint{
		Position: protocol.Position{Line: 0, Character: 1},
		Label:    protocol.String(" (404 NOT_FOUND)"),
	}

	res, err := s.InlayHintResolve(t.Context(), hint)
	if err != nil {
		t.Fatalf("InlayHintResolve: %v", err)
	}

	if res != hint {
		t.Errorf("InlayHintResolve changed the hint: %+v", res)
	}
}

func TestClientCapture(t *testing.T) {
	s := NewServer()

	if got := s.currentClient(); got != nil {
		t.Fatalf("currentClient before any request = %+v, want nil", got)
	}

	fake := &fakeClient{}
	ctx := protocol.WithClient(t.Context(), fake)

	if err := s.DidSave(ctx, &protocol.DidSaveTextDocumentParams{}); err != nil {
		t.Fatalf("DidSave: %v", err)
	}

	if got := s.currentClient(); got == nil {
		t.Error("currentClient after request with client in context is nil, want captured")
	}

	plain := NewServer()

	if err := plain.DidSave(t.Context(), &protocol.DidSaveTextDocumentParams{}); err != nil {
		t.Fatalf("DidSave: %v", err)
	}

	if got := plain.currentClient(); got != nil {
		t.Errorf("currentClient after plain context = %+v, want nil", got)
	}
}

func TestSchemaAndFile(t *testing.T) {
	s := NewServer()
	openDoc(t, s, serverDocA, serverValidDoc)

	schema, file := s.schemaAndFile(serverDocA)
	if file == nil {
		t.Fatal("schemaAndFile known doc returned nil file")
	}

	if schema == nil {
		t.Error("schemaAndFile valid doc returned nil schema, want resolved schema")
	}

	_, missing := s.schemaAndFile(serverUnknown)
	if missing != nil {
		t.Errorf("schemaAndFile unknown doc file = %+v, want nil", missing)
	}
}

func TestParseDoc_unknown(t *testing.T) {
	s := NewServer()

	if got := s.parseDoc(serverUnknown); got != nil {
		t.Errorf("parseDoc unknown doc = %+v, want nil", got)
	}
}

func TestRefreshDiagnostics(t *testing.T) {
	t.Run("no client is a no-op", func(t *testing.T) {
		s := NewServer()
		openDoc(t, s, serverDocA, serverBrokenDoc)

		s.refreshDiagnostics()
	})

	t.Run("broken doc publishes errors then clears", func(t *testing.T) {
		s := NewServer()
		fake := &fakeClient{}
		s.setClient(fake)
		openDoc(t, s, serverDocA, serverBrokenDoc)

		// DidOpen already refreshed once; force another round for determinism.
		s.refreshDiagnostics()

		got, ok := fake.published(diagURI(t, serverDocA))
		if !ok {
			t.Fatal("refreshDiagnostics published nothing for a broken doc")
		}

		if len(got) == 0 {
			t.Errorf("published diagnostics for broken doc are empty")
		}

		s.ws.SetDoc(serverDocA, serverValidDoc)
		s.refreshDiagnostics()

		cleared, ok := fake.published(diagURI(t, serverDocA))
		if !ok {
			t.Fatal("fixing the doc published nothing, want clearing empty array")
		}

		if len(cleared) != 0 {
			t.Errorf("clearing publish has %d diagnostics, want 0", len(cleared))
		}
	})

	t.Run("publish failure is swallowed", func(t *testing.T) {
		s := NewServer()
		fake := &fakeClient{err: errors.New("boom")}
		s.setClient(fake)
		openDoc(t, s, serverDocA, serverBrokenDoc)

		s.refreshDiagnostics()
	})
}
