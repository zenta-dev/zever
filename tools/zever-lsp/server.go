package main

import (
	"context"
	"log"
	"sync"

	"go.lsp.dev/protocol"

	"github.com/zenta-dev/zever/internal/dsl/ast"
	"github.com/zenta-dev/zever/internal/dsl/ir"
	"github.com/zenta-dev/zever/internal/dsl/parser"
)

// serverName is the log base name and the binary name editors invoke.
const serverName = "zever-lsp"

// serverVersion is reported in the initialize result's serverInfo.
// It tracks the framework release version.
const serverVersion = "0.3.0"

// Compile-time proof that Server satisfies the protocol.Server interface,
// so a renamed or mis-signed handler fails here instead of at runtime.
var _ protocol.Server = (*Server)(nil)

// Server is the zever language server: a workspace plus the LSP request
// handlers that read from it. The embedded UnimplementedServer supplies the
// LSP methods this server does not support; every method below overrides one
// of them with real behavior.
type Server struct {
	protocol.UnimplementedServer

	ws        *Workspace
	publisher *diagnosticPublisher

	mu sync.Mutex
	// client is the peer to publish diagnostics to. It is captured from the
	// request context when present, or installed once by run.go from
	// protocol.NewServer's return value.
	client protocol.Client
	// trace is the per-server trace setting from textDocument/setTrace.
	// No global trace sink exists, so this is stored, not forwarded.
	trace protocol.TraceValue
}

// NewServer returns a server with an empty workspace and no client yet.
func NewServer() *Server {
	return &Server{
		ws:        NewWorkspace(),
		publisher: newDiagnosticPublisher(),
		trace:     protocol.TraceValueOff,
	}
}

// setClient installs the peer client, typically once at startup.
func (s *Server) setClient(c protocol.Client) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.client = c
}

// captureClient records the peer client carried by the request context, if
// any. Handlers call this first so refreshDiagnostics can publish over the
// stored client from contexts (such as the debounce timer) that carry none.
func (s *Server) captureClient(ctx context.Context) {
	c, ok := protocol.ClientFromContext(ctx)
	if !ok || c == nil {
		return
	}

	s.setClient(c)
}

// currentClient returns the captured peer client, or nil if none yet.
func (s *Server) currentClient() protocol.Client {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.client
}

// Initialize answers the initialize request: records the workspace root and
// reports exactly the capabilities this server implements.
func (s *Server) Initialize(ctx context.Context, params *protocol.InitializeParams) (*protocol.InitializeResult, error) {
	s.captureClient(ctx)

	root := ""

	// Root precedence: workspaceFolders > rootUri > rootPath.
	if params != nil {
		if folders, ok := params.WorkspaceFolders.Get(); ok && len(folders) > 0 {
			root = string(folders[0].URI)
		} else if params.RootURI != nil && *params.RootURI != "" { //nolint:staticcheck // honor pre-3.17 clients that still send rootUri
			root = string(*params.RootURI) //nolint:staticcheck // honor pre-3.17 clients that still send rootUri
		} else if rp, ok := params.RootPath.Get(); ok && rp != "" { //nolint:staticcheck // honor pre-3.17 clients that still send rootPath
			root = rp
		}
	}

	if root != "" {
		s.ws.SetRoot(root)
	}

	syncKind := protocol.TextDocumentSyncKindFull
	enabled := true

	return &protocol.InitializeResult{
		Capabilities: protocol.ServerCapabilities{
			// A bare sync-kind value (rather than this options struct)
			// declares open/close/change support only -- no "save" key at
			// all -- yet clients may send textDocument/didSave
			// unconditionally regardless of what's declared here. Declaring
			// Save explicitly stops the dispatcher from answering that
			// notification with a "method not supported" error, and is
			// also just correct per the sync capability this server
			// actually implements (see DidSave).
			TextDocumentSync: &protocol.TextDocumentSyncOptions{
				OpenClose: &enabled,
				Change:    &syncKind,
				Save:      protocol.Boolean(true),
			},
			DocumentSymbolProvider:  protocol.Boolean(true),
			WorkspaceSymbolProvider: protocol.Boolean(true),
			DefinitionProvider:      protocol.Boolean(true),
			HoverProvider:           protocol.Boolean(true),
			// Unlike the providers above, this one is a pointer to an
			// options struct, not a bool: the trigger characters are part
			// of the capability itself.
			CompletionProvider: &protocol.CompletionOptions{
				TriggerCharacters: []string{"@", ":", " "},
				ResolveProvider:   &enabled,
			},
			// Same options-struct shape as CompletionProvider above: the
			// trigger characters are part of the capability itself. '('
			// opens a call's argument list and ',' moves to the next
			// argument, matching every call-like construct signatureHelp
			// understands (@validate(...), required(...), check(...),
			// max_attempts(...), backoff(...), @default(...),
			// @renamed_from(...)).
			SignatureHelpProvider: &protocol.SignatureHelpOptions{
				TriggerCharacters: []string{"(", ","},
			},
			DocumentFormattingProvider:      protocol.Boolean(true),
			DocumentRangeFormattingProvider: protocol.Boolean(true),
			CodeActionProvider:              protocol.Boolean(true),
			// PrepareProvider tells the client to ask prepareRename first,
			// so a cursor on a non-renameable symbol is refused before the
			// user is ever prompted for a new name.
			RenameProvider:            &protocol.RenameOptions{PrepareProvider: &enabled},
			ReferencesProvider:        protocol.Boolean(true),
			FoldingRangeProvider:      protocol.Boolean(true),
			DocumentHighlightProvider: protocol.Boolean(true),
			// Never left to any capability auto-wiring: a legend-less
			// semanticTokensProvider makes every returned token index
			// unresolvable to the client.
			SemanticTokensProvider: &protocol.SemanticTokensOptions{
				Legend: semanticTokenLegend,
				Full:   protocol.Boolean(true),
			},
			InlayHintProvider: protocol.Boolean(true),
		},
		ServerInfo: protocol.ServerInfo{
			Name:    serverName,
			Version: protocol.NewOptional(serverVersion),
		},
	}, nil
}

// Initialized handles the initialized notification by publishing the first
// round of diagnostics.
func (s *Server) Initialized(ctx context.Context, _ *protocol.InitializedParams) error {
	s.captureClient(ctx)
	s.refreshDiagnostics()

	return nil
}

// Shutdown is a no-op: there is no background state to tear down.
func (s *Server) Shutdown(_ context.Context) error {
	return nil
}

// Exit is a no-op: it must not terminate the process itself; run.go owns
// shutdown once the connection drains.
func (s *Server) Exit(_ context.Context) error {
	return nil
}

// SetTrace records the client's trace setting on this server. There is no
// global trace sink to forward it to, so the value is stored, not applied.
func (s *Server) SetTrace(ctx context.Context, params *protocol.SetTraceParams) error {
	s.captureClient(ctx)

	if params != nil {
		s.mu.Lock()
		s.trace = params.Value
		s.mu.Unlock()
	}

	return nil
}

// DidOpen records a newly opened buffer and republishes diagnostics.
func (s *Server) DidOpen(ctx context.Context, params *protocol.DidOpenTextDocumentParams) error {
	s.captureClient(ctx)
	s.ws.SetDoc(string(params.TextDocument.URI), params.TextDocument.Text)
	s.refreshDiagnostics()

	return nil
}

// DidChange ingests full-document updates and debounces recompilation: an
// edit burst collapses into one compile once typing pauses.
func (s *Server) DidChange(ctx context.Context, params *protocol.DidChangeTextDocumentParams) error {
	s.captureClient(ctx)

	// The server advertises full sync, so every change event carries the whole
	// document as a *TextDocumentContentChangeWholeDocument arm.
	for _, change := range params.ContentChanges {
		if whole, ok := change.(*protocol.TextDocumentContentChangeWholeDocument); ok {
			s.ws.SetDoc(string(params.TextDocument.URI), whole.Text)
		}
	}

	// Debounce: an edit burst collapses into one compile once typing pauses.
	s.publisher.schedule(string(params.TextDocument.URI), s.refreshDiagnostics)

	return nil
}

// DidSave handles textDocument/didSave. The server advertises full sync, so
// document content is already current via DidChange by the time a save
// happens -- there is nothing new to ingest here (params.Text is only
// populated when includeText is requested, which this server doesn't ask
// for). Still needs a registered handler: with none, the dispatcher answers
// every save with a "method not supported" error instead of silently
// accepting the notification, which is both spec-incorrect and visibly noisy
// in clients that surface server stderr.
func (s *Server) DidSave(ctx context.Context, _ *protocol.DidSaveTextDocumentParams) error {
	s.captureClient(ctx)

	return nil
}

// DidClose drops a buffer, cancels its pending debounce, and republishes.
func (s *Server) DidClose(ctx context.Context, params *protocol.DidCloseTextDocumentParams) error {
	s.captureClient(ctx)
	s.publisher.cancel(string(params.TextDocument.URI))
	s.ws.CloseDoc(string(params.TextDocument.URI))
	s.refreshDiagnostics()

	return nil
}

// DidChangeWatchedFiles rescans the workspace root from disk and
// republishes, since on-disk files changed outside any open buffer.
func (s *Server) DidChangeWatchedFiles(ctx context.Context, _ *protocol.DidChangeWatchedFilesParams) error {
	s.captureClient(ctx)
	s.ws.Rescan()
	s.refreshDiagnostics()

	return nil
}

// DocumentSymbol returns the outline of one document, preferring the latest
// resolved schema and falling back to a standalone parse of the document so
// the outline stays useful mid-edit.
func (s *Server) DocumentSymbol(ctx context.Context, params *protocol.DocumentSymbolParams) (protocol.DocumentSymbolResult, error) {
	s.captureClient(ctx)

	uriStr := string(params.TextDocument.URI)

	// Unknown documents yield an empty outline, not an error.
	file := s.parseDoc(uriStr)
	if file == nil {
		return protocol.DocumentSymbolSlice{}, nil
	}

	path, _ := uriToPath(uriStr)

	result, diags := s.ws.Recompile()
	if result != nil && result.Schema != nil && !diags.HasErrors() {
		return symbolsFromSchema(result.Schema, path), nil
	}

	// Whole-workspace resolution failed; fall back to this one document so the
	// outline stays useful mid-edit.
	return symbolsFromAST(file), nil
}

// Symbols answers a workspace-wide fuzzy symbol search: every
// entity/field/job/schedule/service/RPC across every file the workspace
// knows about, filtered by params.Query. Like DocumentSymbol it prefers the
// latest resolved schema and falls back to a per-file AST parse when the
// workspace does not currently resolve cleanly.
func (s *Server) Symbols(ctx context.Context, params *protocol.WorkspaceSymbolParams) (protocol.WorkspaceSymbolResult, error) {
	s.captureClient(ctx)

	files := s.ws.CollectAllFiles()

	var schema *ir.Schema

	if result, diags := s.ws.Recompile(); result != nil && result.Schema != nil && !diags.HasErrors() {
		schema = result.Schema
	}

	return workspaceSymbols(schema, files, params.Query), nil
}

// Definition jumps to the declaration of the type named under the cursor. A
// nil result encodes LSP null: the cursor names nothing jumpable.
func (s *Server) Definition(ctx context.Context, params *protocol.DefinitionParams) (protocol.DefinitionResult, error) {
	s.captureClient(ctx)

	schema, file := s.schemaAndFile(string(params.TextDocument.URI))

	return definitionAt(schema, file, params.Position), nil
}

// Hover describes the symbol under the cursor, or nil for no hover.
func (s *Server) Hover(ctx context.Context, params *protocol.HoverParams) (*protocol.Hover, error) {
	s.captureClient(ctx)

	schema, file := s.schemaAndFile(string(params.TextDocument.URI))

	return hoverAt(schema, file, params.Position), nil
}

// Completion lists the candidates valid at the cursor. A nil result encodes
// LSP null: nothing to offer here, or the document is unknown.
func (s *Server) Completion(ctx context.Context, params *protocol.CompletionParams) (protocol.CompletionResult, error) {
	s.captureClient(ctx)

	uriStr := string(params.TextDocument.URI)

	content, ok := s.ws.DocContent(uriStr)
	if !ok {
		return nil, nil //nolint:nilnil // LSP encodes "no completions" as a null result
	}

	schema, file := s.schemaAndFile(uriStr)

	items := completionAt(schema, file, content, params.Position)
	if len(items) == 0 {
		return nil, nil //nolint:nilnil // LSP encodes "no completions" as a null result
	}

	return protocol.CompletionItemSlice(items), nil
}

// CompletionResolve fills in a single completion item's detail. It delegates
// to the lowercase implementation that completion.go owns.
func (s *Server) CompletionResolve(ctx context.Context, params *protocol.CompletionItem) (*protocol.CompletionItem, error) {
	return s.completionResolve(ctx, params)
}

// SignatureHelp describes the call-like construct whose argument list holds
// the cursor. It delegates to the lowercase implementation that
// signature_help.go owns.
func (s *Server) SignatureHelp(ctx context.Context, params *protocol.SignatureHelpParams) (*protocol.SignatureHelp, error) {
	return s.signatureHelp(ctx, params)
}

// Formatting rewrites a whole document to canonical style. Unlike Hover and
// Definition it needs no resolved schema: formatting is purely syntactic,
// driven by the token stream alone.
func (s *Server) Formatting(ctx context.Context, params *protocol.DocumentFormattingParams) ([]protocol.TextEdit, error) {
	s.captureClient(ctx)

	uriStr := string(params.TextDocument.URI)

	content, ok := s.ws.DocContent(uriStr)
	if !ok {
		return nil, nil
	}

	path, _ := uriToPath(uriStr)

	return formatDocument(path, []byte(content)), nil
}

// RangeFormatting mirrors Formatting exactly, but narrows the result to the
// edits overlapping the requested range.
func (s *Server) RangeFormatting(ctx context.Context, params *protocol.DocumentRangeFormattingParams) ([]protocol.TextEdit, error) {
	s.captureClient(ctx)

	uriStr := string(params.TextDocument.URI)

	content, ok := s.ws.DocContent(uriStr)
	if !ok {
		return nil, nil
	}

	path, _ := uriToPath(uriStr)

	edits := formatDocument(path, []byte(content))

	return filterEditsToRange(edits, params.Range), nil
}

// CodeAction returns quick fixes for the diagnostics the client reports as
// present in params.Context.Diagnostics. Like Completion and Hover it
// resolves against the latest whole-workspace schema, falling back to a
// standalone parse of the document when resolution fails. A nil result
// encodes LSP null: no fix applies.
func (s *Server) CodeAction(ctx context.Context, params *protocol.CodeActionParams) ([]protocol.CommandOrCodeAction, error) {
	s.captureClient(ctx)

	schema, file := s.schemaAndFile(string(params.TextDocument.URI))

	actions := codeActionsAt(schema, file, params.TextDocument.URI, params)
	if len(actions) == 0 {
		return nil, nil //nolint:nilnil // LSP encodes "no code actions" as a null result
	}

	return actions, nil
}

// PrepareRename answers whether the symbol under the cursor can be renamed,
// and if so which span the client should pre-fill the prompt from. A nil
// result means "not renameable here", which is how the client learns to
// refuse before prompting.
func (s *Server) PrepareRename(ctx context.Context, params *protocol.PrepareRenameParams) (protocol.PrepareRenameResult, error) {
	s.captureClient(ctx)

	file := s.parseDoc(string(params.TextDocument.URI))

	target, ok := renameTargetAt(file, params.Position)
	if !ok {
		return nil, nil //nolint:nilnil // LSP encodes "not renameable here" as a null result
	}

	rng := identRange(target.Pos, target.Name)

	return &rng, nil
}

// Rename renames the symbol under the cursor across the whole workspace,
// dispatching to the reference-finding walk that matches its kind. A cursor
// on anything else fails loudly rather than returning an empty edit that
// would look like success.
func (s *Server) Rename(ctx context.Context, params *protocol.RenameParams) (*protocol.WorkspaceEdit, error) {
	s.captureClient(ctx)

	file := s.parseDoc(string(params.TextDocument.URI))

	target, ok := renameTargetAt(file, params.Position)
	if !ok {
		return nil, errRenameUnsupported
	}

	parsedFiles := s.ws.ParsedFiles()

	switch target.Kind {
	case entityKind:
		return renameEntityEdits(parsedFiles, target.Name, params.NewName)
	case fieldKind:
		return renameFieldEdits(parsedFiles, target.Owner, target.Name, params.NewName)
	case jobKind:
		return renameJobEdits(parsedFiles, target.Name, params.NewName)
	case serviceKind:
		return renameServiceEdits(parsedFiles, target.Name, params.NewName)
	}
	// Proof: renameTargetAt yields only the five kinds dispatched here and
	// below; rpcKind is the sole remaining reachable kind, so control reaches
	// here only for rpc renames (the live rpc subtest in TestRename covers
	// this return).
	return renameRPCEdits(parsedFiles, target.Owner, target.Name, params.NewName)
}

// References finds every occurrence of the symbol under the cursor across
// the whole workspace. Unlike Rename, a cursor that does not resolve to a
// renameable symbol is not an error -- it simply yields an empty list, per
// LSP's "find references" semantics.
func (s *Server) References(ctx context.Context, params *protocol.ReferenceParams) ([]protocol.Location, error) {
	s.captureClient(ctx)

	file := s.parseDoc(string(params.TextDocument.URI))
	parsedFiles := s.ws.ParsedFiles()

	locations := referencesAt(file, params.Position, parsedFiles, params.Context.IncludeDeclaration)
	if locations == nil {
		return []protocol.Location{}, nil
	}

	return []protocol.Location(locations), nil
}

// FoldingRanges returns one fold per multi-line brace pair in the document.
// Like Formatting it is purely lexical -- no resolved schema is needed, so
// folding stays available even while the document fails to parse or resolve.
func (s *Server) FoldingRanges(ctx context.Context, params *protocol.FoldingRangeParams) ([]protocol.FoldingRange, error) {
	s.captureClient(ctx)

	uriStr := string(params.TextDocument.URI)

	content, ok := s.ws.DocContent(uriStr)
	if !ok {
		return nil, nil
	}

	path, _ := uriToPath(uriStr)

	return foldingRangesFor(path, []byte(content)), nil
}

// DocumentHighlight returns every occurrence, in the current document only,
// of the symbol under the cursor. Unlike Rename/References it never walks
// other files: the LSP spec defines this request as document-scoped.
func (s *Server) DocumentHighlight(ctx context.Context, params *protocol.DocumentHighlightParams) ([]protocol.DocumentHighlight, error) {
	s.captureClient(ctx)

	file := s.parseDoc(string(params.TextDocument.URI))

	return highlightsAt(file, params.Position), nil
}

// SemanticTokensFull classifies a whole document into delta-encoded
// semantic tokens. Like Formatting it needs no resolved schema: every
// category is decided by the slot an identifier occupies in the parsed tree,
// so a document that fails to resolve still highlights.
func (s *Server) SemanticTokensFull(ctx context.Context, params *protocol.SemanticTokensParams) (*protocol.SemanticTokens, error) {
	s.captureClient(ctx)

	uriStr := string(params.TextDocument.URI)

	content, ok := s.ws.DocContent(uriStr)
	if !ok {
		return nil, nil //nolint:nilnil // LSP encodes "no tokens for this document" as a null result
	}

	file := s.parseDoc(uriStr)

	return &protocol.SemanticTokens{
		Data: encodeSemanticTokens(classifyFile(file, content)),
	}, nil
}

// InlayHint returns every inlay hint for one document, computed natively via
// inlayHintsAt. A nil slice encodes LSP null for unknown documents.
func (s *Server) InlayHint(ctx context.Context, params *protocol.InlayHintParams) ([]protocol.InlayHint, error) {
	s.captureClient(ctx)

	schema, file := s.schemaAndFile(string(params.TextDocument.URI))

	return inlayHintsAt(schema, file), nil
}

// InlayHintResolve is best-effort by design: hints already carry their full
// labels, so the item round-trips unchanged.
func (s *Server) InlayHintResolve(ctx context.Context, params *protocol.InlayHint) (*protocol.InlayHint, error) {
	s.captureClient(ctx)

	return params, nil
}

// schemaAndFile returns the latest resolved schema (possibly nil) together
// with the parsed AST of one document. The AST is always available even when
// resolution failed, because the parser recovers from malformed input.
func (s *Server) schemaAndFile(uriStr string) (*ir.Schema, *ast.File) {
	var schema *ir.Schema

	if result, _ := s.ws.Recompile(); result != nil {
		schema = result.Schema
	}

	return schema, s.parseDoc(uriStr)
}

// parseDoc parses a single document standalone, returning nil for documents
// the workspace does not know.
func (s *Server) parseDoc(uriStr string) *ast.File {
	content, ok := s.ws.DocContent(uriStr)
	if !ok {
		return nil
	}

	path, _ := uriToPath(uriStr)

	file, _ := parser.New(path, []byte(content)).ParseFile()

	return file
}

// refreshDiagnostics recompiles and publishes the result to every affected
// document, clearing files that are now clean. It publishes over the
// captured client on a background context, since the debounce timer fires
// without a request context. With no client captured yet it is a no-op.
func (s *Server) refreshDiagnostics() {
	client := s.currentClient()
	if client == nil {
		return
	}

	files := s.ws.CollectAllFiles()

	known := make([]string, 0, len(files))
	for path := range files {
		known = append(known, path)
	}

	_, diags := s.ws.Recompile()

	// Log-only: diagnostics publishing is best-effort, and logging goes to
	// stderr so the stdio JSON-RPC stream stays clean.
	if err := s.publisher.publish(client, known, diags); err != nil {
		log.Printf("zever-lsp: publish diagnostics: %v", err)
	}
}
