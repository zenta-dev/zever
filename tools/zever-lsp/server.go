package main

import (
	"context"
	"log"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"go.lsp.dev/protocol"

	"github.com/zenta-dev/zever/dsl/ast"
	"github.com/zenta-dev/zever/dsl/ir"
	"github.com/zenta-dev/zever/dsl/parser"
)

// serverName is the log base name and the binary name editors invoke.
const serverName = "zever-lsp"

// serverVersion is reported in the initialize result's serverInfo.
// It tracks the framework release version.
const serverVersion = "0.8.0"

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

	// semTokCache caches the last full semantic-token result per document
	// URI so textDocument/semanticTokens/full/delta can answer with edits
	// instead of recomputing from scratch. Guarded by mu.
	semTokCache   map[string]semTokCacheEntry
	semTokCounter uint64

	// docLinkTooltip records whether the initialize negotiation advertised
	// the client's textDocument/didOpen documentLink.tooltipSupport, so
	// documentLink/resolve knows whether to fill Tooltip. Guarded by mu.
	docLinkTooltip bool

	// completionCaps records the completion capabilities the client
	// advertised at Initialize: whether items may carry snippets and which
	// resolve properties the client handles lazily. Guarded by mu.
	completionCaps clientCaps

	// settings is the current editing configuration, swapped whole by
	// workspace/didChangeConfiguration and read by every handler. Atomic
	// (rather than mu-guarded) so the diagnostics publisher's delay hook can
	// read it while holding the publisher's own lock.
	settings atomic.Pointer[Settings]

	// encoding is the position encoding negotiated at initialize. It is ""
	// when UTF-16, the protocol's implicit default, was left in place.
	// Stored information only today: every hand-computed offset in this
	// server is byte-based (which is why utf-8 is negotiated when offered),
	// and semantic tokens are UTF-16-exact regardless. Guarded by mu.
	encoding protocol.PositionEncodingKind
}

// NewServer returns a server with an empty workspace and no client yet. The
// settings snapshot starts at the defaults.
func NewServer() *Server {
	ws := NewWorkspace()
	publisher := newDiagnosticPublisher()
	// Let published diagnostics widen their ranges to the enclosing
	// identifier using each file's live source.
	publisher.srcFor = ws.ContentForPath

	s := &Server{
		ws:          ws,
		publisher:   publisher,
		trace:       protocol.TraceValueOff,
		semTokCache: make(map[string]semTokCacheEntry),
	}

	// The publisher consults the settings atomically on each schedule, so a
	// didChangeConfiguration between edit bursts is honored by the next one
	// without any cross-lock dance.
	publisher.delayHook = func() time.Duration { return s.currentSettings().DebounceDelay }

	return s
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

	// Multi-root workspaces: register every folder beyond the primary so
	// Rescan walks all of them. SetRoot already covers the first.
	if params != nil {
		if folders, ok := params.WorkspaceFolders.Get(); ok && len(folders) > 1 {
			for i := 1; i < len(folders); i++ {
				s.ws.AddRoot(string(folders[i].URI))
			}
		}
	}

	// textDocument/documentLink.tooltipSupport is the one client capability
	// a handler needs to behave differently (resolve fills Tooltip only for
	// a client that asked for tooltips), so it is the only one this server
	// records. Like trace, it is per-server state on the struct, under mu.
	if params != nil && params.Capabilities.TextDocument != nil &&
		params.Capabilities.TextDocument.DocumentLink != nil &&
		params.Capabilities.TextDocument.DocumentLink.TooltipSupport != nil {
		s.mu.Lock()
		s.docLinkTooltip = *params.Capabilities.TextDocument.DocumentLink.TooltipSupport
		s.mu.Unlock()
	}

	// Position encoding negotiation: this server computes all LSP character
	// offsets as byte offsets (semantics tokens aside, which the token
	// encoder builds UTF-16-exact itself), so UTF-8 is picked exactly when
	// the client offers it; otherwise the protocol's implicit UTF-16 default
	// applies by omitting the capability entirely.
	encoding := negotiatePositionEncoding(params)
	if encoding != "" {
		s.mu.Lock()
		s.encoding = encoding
		s.mu.Unlock()
	}

	// The completion capabilities decide whether completion items may
	// carry snippets: recorded the same way, under mu, alongside the
	// tooltip flag above. clientCapsFor tolerates a nil params, so this is
	// unconditional.
	s.mu.Lock()
	s.completionCaps = clientCapsFor(params)
	s.mu.Unlock()

	syncKind := protocol.TextDocumentSyncKindIncremental
	enabled := true

	return &protocol.InitializeResult{
		Capabilities: protocol.ServerCapabilities{
			PositionEncoding: encoding,
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
			DeclarationProvider:     protocol.Boolean(true),
			TypeDefinitionProvider:  protocol.Boolean(true),
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
			DocumentOnTypeFormattingProvider: protocol.DocumentOnTypeFormattingOptions{
				FirstTriggerCharacter: "\n",
				MoreTriggerCharacter:  []string{"}", " "},
			},
			// Like CompletionProvider above, this is a pointer to an options
			// struct rather than a bare bool: ResolveProvider advertises that
			// codeAction/resolve fills an action's edit lazily. Actions still
			// carry their edits eagerly, so clients without resolve support
			// keep working; resolve only fills edits that are absent.
			CodeActionProvider: &protocol.CodeActionOptions{ResolveProvider: &enabled},
			// PrepareProvider tells the client to ask prepareRename first,
			// so a cursor on a non-renameable symbol is refused before the
			// user is ever prompted for a new name.
			RenameProvider:     &protocol.RenameOptions{PrepareProvider: &enabled},
			ReferencesProvider: protocol.Boolean(true),
			// Same options-struct shape as DocumentLinkProvider below: the
			// resolve round-trip exists so a future command-bearing lens can
			// fill in lazily. Today's lenses are info-only (a title, no
			// command to execute), so resolve is a passthrough.
			CodeLensProvider:          &protocol.CodeLensOptions{ResolveProvider: &enabled},
			FoldingRangeProvider:      protocol.Boolean(true),
			DocumentHighlightProvider: protocol.Boolean(true),
			// Never left to any capability auto-wiring: a legend-less
			// semanticTokensProvider makes every returned token index
			// unresolvable to the client.
			SemanticTokensProvider: &protocol.SemanticTokensOptions{
				Legend: semanticTokenLegend,
				Range:  protocol.Boolean(true),
				Full:   &protocol.SemanticTokensFullDelta{Delta: boolPtr(true)},
			},
			InlayHintProvider: protocol.Boolean(true),
			// The selection-range provider is a bare boolean: unlike
			// completion/signature help there are no per-request options,
			// so the bool form both implements and declares the feature.
			SelectionRangeProvider: protocol.Boolean(true),
			// Same options-struct shape as CompletionProvider above, for the
			// same reason: resolveProvider is part of the documentLink
			// capability itself.
			DocumentLinkProvider:  &protocol.DocumentLinkOptions{ResolveProvider: &enabled},
			CallHierarchyProvider: protocol.Boolean(true),
			Workspace: &protocol.WorkspaceOptions{
				WorkspaceFolders: &protocol.WorkspaceFoldersServerCapabilities{
					Supported:           boolPtr(true),
					ChangeNotifications: protocol.String("workspace/didChangeWorkspaceFolders"),
				},
			},
		},
		ServerInfo: protocol.ServerInfo{
			Name:    serverName,
			Version: protocol.NewOptional(serverVersion),
		},
	}, nil
}

// negotiatePositionEncoding picks the position encoding to answer an
// initialize request with, from the client's general.positionEncodings
// announcement. It returns the UTF-8 kind when the client offers it and ""
// otherwise, which is mapped to the protocol's implicit UTF-16 default by
// omitting the positionEncoding capability (the spec's mandatory fallback).
func negotiatePositionEncoding(params *protocol.InitializeParams) protocol.PositionEncodingKind {
	if params == nil || params.Capabilities.General == nil {
		return ""
	}

	for _, kind := range params.Capabilities.General.PositionEncodings {
		if kind == protocol.PositionEncodingKindUTF8 {
			return protocol.PositionEncodingKindUTF8
		}
	}

	return ""
}

// clientCaps records the completion capabilities a client advertised at
// Initialize: whether completion items may carry snippet insert text,
// whether the client understands insert/replace edits, and which item
// properties it can resolve lazily. The zero value -- a client that
// advertised nothing -- renders every item as a plain-text label.
type clientCaps struct {
	snippetSupport       bool
	insertReplaceSupport bool
	resolveProps         []string
}

// clientCapsFor reads the completion capabilities out of an Initialize
// request's params. Every pointer in the chain may be nil -- a client that
// advertises no completion block negotiates nothing -- so each level is
// guarded the same way Initialize guards the tooltip flag.
func clientCapsFor(params *protocol.InitializeParams) clientCaps {
	if params == nil || params.Capabilities.TextDocument == nil ||
		params.Capabilities.TextDocument.Completion == nil ||
		params.Capabilities.TextDocument.Completion.CompletionItem == nil {
		return clientCaps{}
	}

	item := params.Capabilities.TextDocument.Completion.CompletionItem

	var caps clientCaps

	if item.SnippetSupport != nil {
		caps.snippetSupport = *item.SnippetSupport
	}

	if item.InsertReplaceSupport != nil {
		caps.insertReplaceSupport = *item.InsertReplaceSupport
	}

	caps.resolveProps = append([]string(nil), item.ResolveSupport.Properties...)

	return caps
}

// Initialized handles the initialized notification by publishing the first
// round of diagnostics.
func (s *Server) Initialized(ctx context.Context, _ *protocol.InitializedParams) error {
	s.captureClient(ctx)
	s.refreshDiagnostics() //nolint:contextcheck // publish is intentionally background: the stored client serves contexts (e.g. debounce timer) that carry none

	return nil
}

// DidChangeConfiguration stores the settings the client pushes over
// workspace/didChangeConfiguration, under the sole `zever` section (see
// settings.go for the shape). An empty or non-object payload leaves the
// current settings untouched; everything else is a whole replacement.
func (s *Server) DidChangeConfiguration(ctx context.Context, params *protocol.DidChangeConfigurationParams) error {
	s.captureClient(ctx)

	if params == nil {
		return nil
	}

	if parsed, ok := parseSettings(params.Settings); ok {
		s.settings.Store(&parsed)
	}

	return nil
}

// currentSettings returns the settings snapshot in effect. NewServer always
// installs one; the defaults are the answer for a zero-value Server, of the
// kind direct struct literals in tests can produce.
func (s *Server) currentSettings() Settings {
	if st := s.settings.Load(); st != nil {
		return *st
	}

	return defaultSettings()
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
	s.refreshDiagnostics() //nolint:contextcheck // publish is intentionally background: the stored client serves contexts (e.g. debounce timer) that carry none

	return nil
}

// DidChange ingests full-document updates and debounces recompilation: an
// edit burst collapses into one compile once typing pauses.
func (s *Server) DidChange(ctx context.Context, params *protocol.DidChangeTextDocumentParams) error {
	s.captureClient(ctx)

	uriStr := string(params.TextDocument.URI)

	// Handle both whole-document and incremental arms: full-sync clients keep
	// sending whole-document changes, while incremental clients send partial
	// edits that are applied on top of the current buffer.
	for _, change := range params.ContentChanges {
		switch v := change.(type) {
		case *protocol.TextDocumentContentChangeWholeDocument:
			s.ws.SetDoc(uriStr, v.Text)
		case *protocol.TextDocumentContentChangePartial:
			current, _ := s.ws.DocContent(uriStr)
			s.ws.SetDoc(uriStr, applyIncrementalEdit(current, v))
		default:
			// Tolerate the interface/boxed forms some clients/tests use.
			if whole, ok := change.(*protocol.TextDocumentContentChangeWholeDocument); ok {
				s.ws.SetDoc(uriStr, whole.Text)
			} else if partial, ok := change.(*protocol.TextDocumentContentChangePartial); ok {
				current, _ := s.ws.DocContent(uriStr)
				s.ws.SetDoc(uriStr, applyIncrementalEdit(current, partial))
			}
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
	s.refreshDiagnostics() //nolint:contextcheck // publish is intentionally background: the stored client serves contexts (e.g. debounce timer) that carry none

	return nil
}

// DidChangeWatchedFiles rescans the workspace root from disk and
// republishes, since on-disk files changed outside any open buffer.
func (s *Server) DidChangeWatchedFiles(ctx context.Context, _ *protocol.DidChangeWatchedFilesParams) error {
	s.captureClient(ctx)
	s.ws.Rescan()
	s.refreshDiagnostics() //nolint:contextcheck // publish is intentionally background: the stored client serves contexts (e.g. debounce timer) that carry none

	return nil
}

// DidChangeWorkspaceFolders updates the set of workspace roots in response
// to a client-side folder change and rescans.
func (s *Server) DidChangeWorkspaceFolders(ctx context.Context, params *protocol.DidChangeWorkspaceFoldersParams) error {
	s.captureClient(ctx)

	if params != nil {
		for _, folder := range params.Event.Added {
			s.ws.AddRoot(string(folder.URI))
		}

		for _, folder := range params.Event.Removed {
			s.ws.RemoveRoot(string(folder.URI))
		}
	}

	s.ws.Rescan()
	s.refreshDiagnostics() //nolint:contextcheck // publish is intentionally background: the stored client serves contexts (e.g. debounce timer) that carry none

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

// TypeDefinition jumps to the declaration of the named type under the cursor:
// an entity, message, or named enum named by a relation target, an rpc
// Returns type, a param type, or a field type. A nil result encodes LSP null:
// the cursor names a scalar (which has no declaration), an unknown type, or
// nothing jumpable at all.
func (s *Server) TypeDefinition(ctx context.Context, params *protocol.TypeDefinitionParams) (protocol.DefinitionResult, error) {
	s.captureClient(ctx)

	schema, file := s.schemaAndFile(string(params.TextDocument.URI))

	return typeDefinitionAt(schema, file, params.Position), nil
}

// Declaration is an alias for Definition: every .zen declaration is also
// its own declaration site (there is no separate header/implementation
// split), so the declaration of the type named under the cursor is exactly
// where Definition jumps. A nil result encodes LSP null, same as
// Definition.
func (s *Server) Declaration(ctx context.Context, params *protocol.DeclarationParams) (protocol.DeclarationResult, error) {
	s.captureClient(ctx)

	schema, file := s.schemaAndFile(string(params.TextDocument.URI))

	if loc, ok := definitionAt(schema, file, params.Position).(*protocol.Location); ok && loc != nil {
		return loc, nil
	}

	return nil, nil //nolint:nilnil // LSP encodes "nothing jumpable" as a null result, same as Definition
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

	s.mu.Lock()
	caps := s.completionCaps
	s.mu.Unlock()

	items := completionAtWithCaps(schema, file, content, params.Position, caps)
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

// OnTypeFormatting reuses formatDocument and narrows the result to the line
// holding the trigger position. An unknown document yields nil.
func (s *Server) OnTypeFormatting(ctx context.Context, params *protocol.DocumentOnTypeFormattingParams) ([]protocol.TextEdit, error) {
	s.captureClient(ctx)

	uriStr := string(params.TextDocument.URI)

	content, ok := s.ws.DocContent(uriStr)
	if !ok {
		return nil, nil
	}

	path, _ := uriToPath(uriStr)

	edits := formatDocument(path, []byte(content))

	rng := protocol.Range{
		Start: protocol.Position{Line: params.Position.Line, Character: 0},
		End:   protocol.Position{Line: params.Position.Line, Character: 0},
	}

	return filterEditsToRange(edits, rng), nil
}

// TextDocumentOnTypeFormatting is an alias kept for callers that use the
// protocol method's full name.
func (s *Server) TextDocumentOnTypeFormatting(ctx context.Context, params *protocol.DocumentOnTypeFormattingParams) ([]protocol.TextEdit, error) {
	return s.OnTypeFormatting(ctx, params)
}

// CodeAction returns quick fixes for the diagnostics the client reports as
// present in params.Context.Diagnostics, plus the whole-document format
// action when the request's Only filter admits source actions. Like
// Completion and Hover it resolves against the latest whole-workspace
// schema, falling back to a standalone parse of the document when
// resolution fails. A nil result encodes LSP null: no fix applies.
func (s *Server) CodeAction(ctx context.Context, params *protocol.CodeActionParams) ([]protocol.CommandOrCodeAction, error) {
	s.captureClient(ctx)

	schema, file := s.schemaAndFile(string(params.TextDocument.URI))

	var src []byte

	if content, ok := s.ws.DocContent(string(params.TextDocument.URI)); ok {
		src = []byte(content)
	}

	actions := codeActionsAt(schema, file, params.TextDocument.URI, src, params)
	if len(actions) == 0 {
		return nil, nil // LSP encodes "no code actions" as a null result
	}

	return actions, nil
}

// CodeActionResolve fills in a single code action's edit when the action
// arrived without one. It delegates to the lowercase implementation that
// code_action.go owns.
func (s *Server) CodeActionResolve(ctx context.Context, params *protocol.CodeAction) (*protocol.CodeAction, error) {
	s.captureClient(ctx)

	if params == nil {
		return nil, nil //nolint:nilnil // LSP encodes "nothing to resolve" as a null result
	}

	return s.codeActionResolve(params), nil
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
	case rpcKind:
		return renameRPCEdits(parsedFiles, target.Owner, target.Name, params.NewName)
	case messageKind:
		return renameMessageEdits(parsedFiles, target.Name, params.NewName)
	case enumKind:
		return renameEnumEdits(parsedFiles, target.Name, params.NewName)
	case sagaKind:
		return renameSagaEdits(parsedFiles, target.Name, params.NewName)
	}
	// Proof: renameTargetAt yields only the eight kinds dispatched here and
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

// CodeLens returns one reference-count lens per top-level declaration in the
// requested document, computed by codeLensesAt over the whole workspace. An
// unknown document yields nil, not an error, per the "no lenses for this
// document" reading DocumentLink shares; a switched-off
// zever.codelens.references setting yields nil too, so the client renders
// nothing at all.
func (s *Server) CodeLens(ctx context.Context, params *protocol.CodeLensParams) ([]protocol.CodeLens, error) {
	s.captureClient(ctx)

	if !s.currentSettings().CodeLensReferences {
		return nil, nil
	}

	uriStr := string(params.TextDocument.URI)

	schema, _ := s.schemaAndFile(uriStr)

	return codeLensesAt(schema, s.ws.ParsedFiles(), uriStr), nil
}

// CodeLensResolve is a passthrough by design: lenses already carry their
// full titles (there is no command to fill in lazily), so the item
// round-trips unchanged.
func (s *Server) CodeLensResolve(ctx context.Context, params *protocol.CodeLens) (*protocol.CodeLens, error) {
	s.captureClient(ctx)

	return params, nil
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

// SelectionRange resolves each requested position into the chain of ranges
// a client can climb outward through .Parent: token -> declaration head
// line -> enclosing brace block -> whole document. Like FoldingRanges it is
// purely lexical -- no resolved schema is needed, so selection keeps
// working mid-edit. An unknown document yields an empty result per
// position: the spec-honest degenerate range each result must fall back to
// is anchored at the requested position itself, which selectionRangeAt
// builds.
func (s *Server) SelectionRange(ctx context.Context, params *protocol.SelectionRangeParams) ([]protocol.SelectionRange, error) {
	s.captureClient(ctx)

	uriStr := string(params.TextDocument.URI)

	content, ok := s.ws.DocContent(uriStr)
	if !ok {
		return []protocol.SelectionRange{}, nil
	}

	path, _ := uriToPath(uriStr)

	tokens, _ := lexAll(path, []byte(content))

	result := make([]protocol.SelectionRange, 0, len(params.Positions))

	for _, pos := range params.Positions {
		result = append(result, selectionRangeAt(tokens, content, pos))
	}

	return result, nil
}

// DocumentLink turns every URL written in a `//` comment of the document
// into a clickable link. Like FoldingRanges it needs no resolved schema:
// comments exist only in the source, not in any token stream the parser
// records. An unknown document yields an empty list, not an error, per the
// "no links for this document" reading DocumentSymbol and InlayHint share.
func (s *Server) DocumentLink(ctx context.Context, params *protocol.DocumentLinkParams) ([]protocol.DocumentLink, error) {
	s.captureClient(ctx)

	uriStr := string(params.TextDocument.URI)

	content, ok := s.ws.DocContent(uriStr)
	if !ok {
		return []protocol.DocumentLink{}, nil
	}

	return documentLinksFor(content), nil
}

// DocumentLinkResolve fills a link's Tooltip with its target URL, when the
// client negotiated tooltipSupport at Initialize. The scan itself leaves
// Tooltip empty (the resolve round-trip is exactly what the protocol docs
// use it for), so a client that never asked never sees one. A link that
// already carries a Tooltip, or points nowhere, round-trips unchanged.
func (s *Server) DocumentLinkResolve(ctx context.Context, params *protocol.DocumentLink) (*protocol.DocumentLink, error) {
	s.captureClient(ctx)

	s.mu.Lock()
	tooltipWanted := s.docLinkTooltip
	s.mu.Unlock()

	if !tooltipWanted || params == nil || params.Target == nil {
		return params, nil
	}

	tooltip := string(*params.Target)
	params.Tooltip = &tooltip

	return params, nil
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

	data := encodeSemanticTokens(classifyFile(file, content))
	resultID := s.storeSemTokResult(uriStr, data)

	return &protocol.SemanticTokens{
		ResultID: &resultID,
		Data:     data,
	}, nil
}

// SemanticTokensRange classifies the spans overlapping params.Range.
func (s *Server) SemanticTokensRange(ctx context.Context, params *protocol.SemanticTokensRangeParams) (*protocol.SemanticTokens, error) {
	s.captureClient(ctx)

	uriStr := string(params.TextDocument.URI)

	content, ok := s.ws.DocContent(uriStr)
	if !ok {
		return nil, nil //nolint:nilnil // LSP encodes "no tokens for this document" as a null result
	}

	file := s.parseDoc(uriStr)

	spans := filterTokenSpansToRange(classifyFile(file, content), params.Range)

	return &protocol.SemanticTokens{
		Data: encodeSemanticTokens(spans),
	}, nil
}

// SemanticTokensFullDelta answers a full/delta request. When there is no
// cached previous result or the supplied previousResultId does not match,
// it returns a full result; otherwise it returns delta edits transforming
// the cached result into the freshly computed one.
func (s *Server) SemanticTokensFullDelta(ctx context.Context, params *protocol.SemanticTokensDeltaParams) (protocol.SemanticTokensDeltaResult, error) {
	s.captureClient(ctx)

	uriStr := string(params.TextDocument.URI)

	content, ok := s.ws.DocContent(uriStr)
	if !ok {
		return nil, nil //nolint:nilnil // LSP encodes "no tokens for this document" as a null result
	}

	file := s.parseDoc(uriStr)

	data := encodeSemanticTokens(classifyFile(file, content))

	s.mu.Lock()
	if s.semTokCache == nil {
		s.semTokCache = make(map[string]semTokCacheEntry)
	}
	prev, hasPrev := s.semTokCache[uriStr]
	mismatch := params.PreviousResultID == "" || !hasPrev || prev.resultID != params.PreviousResultID
	s.mu.Unlock()

	if mismatch {
		resultID := s.storeSemTokResult(uriStr, data)

		return &protocol.SemanticTokens{
			ResultID: &resultID,
			Data:     data,
		}, nil
	}

	resultID := s.storeSemTokResult(uriStr, data)
	edits := computeSemTokDelta(prev.data, data)

	return &protocol.SemanticTokensDelta{
		ResultID: &resultID,
		Edits:    edits,
	}, nil
}

// InlayHint returns every inlay hint for one document, computed natively via
// inlayHintsFor under the server's current settings. A nil slice encodes LSP
// null for unknown documents.
func (s *Server) InlayHint(ctx context.Context, params *protocol.InlayHintParams) ([]protocol.InlayHint, error) {
	s.captureClient(ctx)

	schema, file := s.schemaAndFile(string(params.TextDocument.URI))

	return inlayHintsFor(schema, file, s.currentSettings()), nil
}

// InlayHintResolve is best-effort by design: hints already carry their full
// labels, so the item round-trips unchanged.
func (s *Server) InlayHintResolve(ctx context.Context, params *protocol.InlayHint) (*protocol.InlayHint, error) {
	s.captureClient(ctx)

	return params, nil
}

// storeSemTokResult records a full result under a fresh result id and
// returns that id.
func (s *Server) storeSemTokResult(uriStr string, data []uint32) string {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.semTokCache == nil {
		s.semTokCache = make(map[string]semTokCacheEntry)
	}

	s.semTokCounter++
	id := strconv.FormatUint(s.semTokCounter, 10)
	s.semTokCache[uriStr] = semTokCacheEntry{resultID: id, data: data}

	return id
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
