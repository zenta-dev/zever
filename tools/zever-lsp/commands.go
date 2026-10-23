package main

import (
	"context"
	"encoding/json"
	"fmt"

	"go.lsp.dev/protocol"
	"go.lsp.dev/uri"

	"github.com/zenta-dev/zever/dsl/ir"
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

	grouped := groupDiagnostics(diags, s.publisher.srcFor)

	callParams := &protocol.CodeActionParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: id},
		Context: protocol.CodeActionContext{
			Diagnostics: grouped[path],
			Only:        []protocol.CodeActionKind{protocol.CodeActionKindQuickFix},
		},
	}

	merged := &protocol.WorkspaceEdit{Changes: map[uri.URI][]protocol.TextEdit{}}

	for _, item := range codeActionsAt(schema, file, id, []byte(content), callParams) {
		action, ok := item.(*protocol.CodeAction)
		if !ok || action == nil || action.Edit == nil {
			continue
		}

		for docID, edits := range action.Edit.Changes {
			merged.Changes[docID] = append(merged.Changes[docID], edits...)
		}
	}

	raw, err := json.Marshal(merged)
	if err != nil {
		return nil, fmt.Errorf("zever-lsp: %s cannot encode the fix edit: %w", CommandFixAll, err)
	}

	return protocol.LSPAny(raw), nil
}

// emptyWorkspaceEdit encodes an empty WorkspaceEdit as LSPAny: the fixAll
// answer for a document with nothing to fix.
func emptyWorkspaceEdit() protocol.LSPAny {
	raw, err := json.Marshal(&protocol.WorkspaceEdit{})
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

	if err := json.Unmarshal(raw, &direct); err == nil && direct != "" {
		return direct, true
	}

	var obj map[string]any

	if err := json.Unmarshal(raw, &obj); err != nil {
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
