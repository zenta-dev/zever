package main

import (
	"context"
	"fmt"
	"sort"
	"strconv"

	"go.lsp.dev/protocol"
	"go.lsp.dev/uri"

	"github.com/zenta-dev/zever/dsl/ir"

	"github.com/zenta-dev/zever/shared/codec"
)

// CommandValidate is the workspace/executeCommand identifier that forces a
// recompile and republishes diagnostics for one document.
const CommandValidate = "zever.validate"

// CommandFixAll is the workspace/executeCommand identifier that collects
// every quickfix for one document into a single WorkspaceEdit.
const CommandFixAll = "zever.fixAll"

// ExecuteCommand answers workspace/executeCommand: zever.validate forces a
// recompile and republishes diagnostics for the [{uri}] document and returns
// null, while zever.fixAll returns one WorkspaceEdit holding every quickfix
// for it. Any other command, or arguments the handler cannot use, fail with
// ErrExecuteUnknownCommand rather than a guess or a panic.
func (s *Server) ExecuteCommand(ctx context.Context, params *protocol.ExecuteCommandParams) (protocol.LSPAny, error) {
	s.captureClient(ctx)

	if params == nil {
		return nil, errExecuteUnknownCommand
	}

	switch params.Command {
	case CommandValidate:
		//nolint:contextcheck // validate publishes over the stored client: refreshDiagnostics owns its background context by design (see server.go)
		if err := s.executeValidate(params.Arguments); err != nil {
			return nil, err
		}

		return nil, nil
	case CommandFixAll:
		return s.executeFixAll(params.Arguments)
	default:
		return nil, errExecuteUnknownCommand
	}
}

// executeValidate forces a recompile and republishes diagnostics, covering
// the document named by args. It answers nil on success: the LSP null
// result a validate command has nothing else to report.
func (s *Server) executeValidate(args []protocol.LSPAny) error {
	uriStr, ok := executeCommandURI(args)
	if !ok {
		return fmt.Errorf("zever-lsp: %s requires one document URI argument: %w", CommandValidate, errExecuteUnknownCommand)
	}

	if _, ok := uriToPath(uriStr); !ok {
		return fmt.Errorf("zever-lsp: %s argument %q is not a document URI: %w", CommandValidate, uriStr, errExecuteUnknownCommand)
	}

	// The results feed publishDiagnostics through refreshDiagnostics below;
	// only the side effect matters here.
	_, _ = s.ws.Recompile()
	s.refreshDiagnostics()

	return nil
}

// executeFixAll collects every quickfix for the document named by args into
// a single WorkspaceEdit. It recomputes the workspace diagnostics, filters
// them to the file, and runs the result through the same codeActionsAt
// dispatch textDocument/codeAction uses (scoped to quickfix, so the format
// action never leaks in), merging each action's Changes into one edit. A
// document with nothing to fix, or one the workspace does not know, yields
// an empty edit rather than an error.
func (s *Server) executeFixAll(args []protocol.LSPAny) (protocol.LSPAny, error) {
	uriStr, ok := executeCommandURI(args)
	if !ok {
		return nil, fmt.Errorf("zever-lsp: %s requires one document URI argument: %w", CommandFixAll, errExecuteUnknownCommand)
	}

	path, ok := uriToPath(uriStr)
	if !ok {
		return nil, fmt.Errorf("zever-lsp: %s argument %q is not a document URI: %w", CommandFixAll, uriStr, errExecuteUnknownCommand)
	}

	id := uri.URI(uriStr)

	content, known := s.ws.DocContent(uriStr)
	if !known {
		return emptyWorkspaceEdit(), nil
	}

	var schema *ir.Schema

	result, diags := s.ws.Recompile()
	if result != nil {
		schema = result.Schema
	}

	file := s.parseDoc(uriStr)

	grouped := groupDiagnostics(diags, s.publisher.srcFor, s.markupMessage.Load())

	callParams := &protocol.CodeActionParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: id},
		Context: protocol.CodeActionContext{
			Diagnostics: grouped[path],
			Only:        []protocol.CodeActionKind{protocol.CodeActionKindQuickFix},
		},
	}

	merged := map[uri.URI][]protocol.TextEdit{}

	for _, item := range codeActionsAt(schema, file, id, []byte(content), callParams) {
		action, ok := item.(*protocol.CodeAction)
		if !ok || action == nil || action.Edit == nil {
			continue
		}

		for docID, edits := range action.Edit.Changes {
			merged[docID] = append(merged[docID], edits...)
		}
	}

	// Only fixAll uses change annotations: codeAction/resolve keeps
	// rebuilding plain edits (see hasCodeActionEdit), so a resolved action
	// never invents a confirmation the client did not negotiate.
	edit := buildFixAllEdit(uriStr, merged, s.changeAnnotation.Load())

	raw, err := codec.Marshal(edit)
	if err != nil {
		return nil, fmt.Errorf("zever-lsp: %s cannot encode the fix edit: %w", CommandFixAll, err)
	}

	return protocol.LSPAny(raw), nil
}

// buildFixAllEdit renders collected per-file quickfix edits as a
// WorkspaceEdit. Without annotation support -- or when every fix lands in
// the requested document -- it is the legacy plain Changes edit, so
// same-file responses stay byte-identical to what older clients received.
// When the client advertised workspace.workspaceEdit.changeAnnotationSupport
// and fixes span other files, edits in the requested document stay silent
// plain TextEdits inside DocumentChanges while each foreign file's edits
// become AnnotatedTextEdits sharing one needsConfirmation ChangeAnnotation
// for that file, so the editor confirms before touching files the user did
// not ask to fix.
func buildFixAllEdit(requested string, merged map[uri.URI][]protocol.TextEdit, annotate bool) *protocol.WorkspaceEdit {
	if !annotate {
		return &protocol.WorkspaceEdit{Changes: merged}
	}

	var foreign []uri.URI

	for docID, edits := range merged {
		if len(edits) == 0 {
			continue
		}

		if string(docID) != requested {
			foreign = append(foreign, docID)
		}
	}

	// Same-file-only: no annotations to carry, so keep the plain shape.
	if len(foreign) == 0 {
		return &protocol.WorkspaceEdit{Changes: merged}
	}

	sort.Slice(foreign, func(i, j int) bool { return foreign[i] < foreign[j] })

	out := &protocol.WorkspaceEdit{
		ChangeAnnotations: make(map[protocol.ChangeAnnotationIdentifier]protocol.ChangeAnnotation, len(foreign)),
	}

	if edits := merged[uri.URI(requested)]; len(edits) > 0 {
		elements := make([]protocol.TextDocumentEditElement, len(edits))
		for i := range edits {
			elements[i] = &edits[i]
		}

		out.DocumentChanges = append(out.DocumentChanges, &protocol.TextDocumentEdit{
			TextDocument: protocol.OptionalVersionedTextDocumentIdentifier{
				TextDocumentIdentifier: protocol.TextDocumentIdentifier{URI: uri.URI(requested)},
			},
			Edits: elements,
		})
	}

	for i, docID := range foreign {
		edits := merged[docID]
		annotationID := protocol.ChangeAnnotationIdentifier("zever-fixAll-" + strconv.Itoa(i+1))

		out.ChangeAnnotations[annotationID] = protocol.ChangeAnnotation{
			Label:             "Apply zever fixes in " + string(docID),
			NeedsConfirmation: boolPtr(true),
		}

		elements := make([]protocol.TextDocumentEditElement, len(edits))

		for j := range edits {
			elements[j] = &protocol.AnnotatedTextEdit{
				TextEdit:     edits[j],
				AnnotationID: annotationID,
			}
		}

		out.DocumentChanges = append(out.DocumentChanges, &protocol.TextDocumentEdit{
			TextDocument: protocol.OptionalVersionedTextDocumentIdentifier{
				TextDocumentIdentifier: protocol.TextDocumentIdentifier{URI: docID},
			},
			Edits: elements,
		})
	}

	return out
}

// emptyWorkspaceEdit encodes an empty WorkspaceEdit as LSPAny: the fixAll
// answer for a document with nothing to fix.
func emptyWorkspaceEdit() protocol.LSPAny {
	raw, err := codec.Marshal(&protocol.WorkspaceEdit{})
	if err != nil {
		return nil
	}

	return protocol.LSPAny(raw)
}

// executeCommandURI reads the document URI out of an executeCommand argument
// list, which carries [{uri}]: either a bare URI string, an object holding
// it under "uri", or a textDocument wrapper holding it under
// textDocument.uri. Anything else reports false, and the caller answers
// ErrExecuteUnknownCommand.
func executeCommandURI(args []protocol.LSPAny) (string, bool) {
	if len(args) == 0 {
		return "", false
	}

	raw := []byte(args[0])

	var direct string

	if err := codec.Unmarshal(raw, &direct); err == nil && direct != "" {
		return direct, true
	}

	var obj map[string]any

	if err := codec.Unmarshal(raw, &obj); err != nil {
		return "", false
	}

	if u, _ := obj["uri"].(string); u != "" {
		return u, true
	}

	if wrapped, _ := obj["textDocument"].(map[string]any); wrapped != nil {
		if u, _ := wrapped["uri"].(string); u != "" {
			return u, true
		}
	}

	return "", false
}
