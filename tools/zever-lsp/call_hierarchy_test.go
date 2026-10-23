package main

import (
	"testing"

	"go.lsp.dev/protocol"
	"go.lsp.dev/uri"
)

const chDoc = `entity User {
	id: uuid @primary
}

service UserService {
	rpc GetUser(id: uuid) -> User {
		auth: none
	}
	rpc ListUsers() -> User {
		auth: none
	}
}

saga Checkout {
	step reserve {
		execute: UserService.GetUser
	}
}
`

func TestCallHierarchyPrepareRPC(t *testing.T) {
	t.Parallel()

	s := NewServer()
	openDoc(t, s, serverDocA, chDoc)

	items, err := s.PrepareCallHierarchy(t.Context(), &protocol.CallHierarchyPrepareParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: uri.URI(serverDocA)},
			Position:     cursorOn(t, chDoc, 5, "GetUser"),
		},
	})
	if err != nil {
		t.Fatalf("PrepareCallHierarchy: %v", err)
	}

	if len(items) != 1 {
		t.Fatalf("items = %d, want 1", len(items))
	}

	if items[0].Name != "GetUser" {
		t.Errorf("Name = %q, want GetUser", items[0].Name)
	}

	if items[0].Kind != protocol.SymbolKindMethod {
		t.Errorf("Kind = %v, want Method", items[0].Kind)
	}

	if items[0].Detail == nil || *items[0].Detail != "service UserService" {
		t.Errorf("Detail = %v, want \"service UserService\"", items[0].Detail)
	}
}

func TestCallHierarchyPrepareService(t *testing.T) {
	t.Parallel()

	s := NewServer()
	openDoc(t, s, serverDocA, chDoc)

	items, err := s.PrepareCallHierarchy(t.Context(), &protocol.CallHierarchyPrepareParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: uri.URI(serverDocA)},
			Position:     cursorOn(t, chDoc, 4, "UserService"),
		},
	})
	if err != nil {
		t.Fatalf("PrepareCallHierarchy: %v", err)
	}

	if len(items) != 2 {
		t.Fatalf("items = %d, want 2", len(items))
	}
}

func TestCallHierarchyPrepareUnknownDoc(t *testing.T) {
	t.Parallel()

	s := NewServer()

	items, err := s.PrepareCallHierarchy(t.Context(), &protocol.CallHierarchyPrepareParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: uri.URI(serverUnknown)},
			Position:     protocol.Position{},
		},
	})
	if err != nil {
		t.Fatalf("PrepareCallHierarchy: %v", err)
	}

	if items != nil {
		t.Errorf("items = %v, want nil", items)
	}
}

func TestCallHierarchyIncomingCalled(t *testing.T) {
	t.Parallel()

	s := NewServer()
	openDoc(t, s, serverDocA, chDoc)

	items, err := s.PrepareCallHierarchy(t.Context(), &protocol.CallHierarchyPrepareParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: uri.URI(serverDocA)},
			Position:     cursorOn(t, chDoc, 5, "GetUser"),
		},
	})
	if err != nil || len(items) != 1 {
		t.Fatalf("prepare failed: %v, %d", err, len(items))
	}

	incoming, err := s.IncomingCalls(t.Context(), &protocol.CallHierarchyIncomingCallsParams{Item: items[0]})
	if err != nil {
		t.Fatalf("IncomingCalls: %v", err)
	}

	if len(incoming) != 1 {
		t.Fatalf("incoming = %d, want 1", len(incoming))
	}

	if incoming[0].From.Name != "Checkout" {
		t.Errorf("From.Name = %q, want Checkout", incoming[0].From.Name)
	}

	if incoming[0].From.Kind != protocol.SymbolKindEvent {
		t.Errorf("From.Kind = %v, want Event", incoming[0].From.Kind)
	}

	if len(incoming[0].FromRanges) == 0 {
		t.Errorf("FromRanges empty, want at least one")
	}
}

//nolint:dupl
func TestCallHierarchyIncomingUncalled(t *testing.T) {
	t.Parallel()

	s := NewServer()
	openDoc(t, s, serverDocA, chDoc)

	items, err := s.PrepareCallHierarchy(t.Context(), &protocol.CallHierarchyPrepareParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: uri.URI(serverDocA)},
			Position:     cursorOn(t, chDoc, 8, "ListUsers"),
		},
	})
	if err != nil || len(items) != 1 {
		t.Fatalf("prepare failed: %v, %d", err, len(items))
	}

	incoming, err := s.IncomingCalls(t.Context(), &protocol.CallHierarchyIncomingCallsParams{Item: items[0]})
	if err != nil {
		t.Fatalf("IncomingCalls: %v", err)
	}

	if incoming == nil {
		t.Fatalf("incoming nil, want empty slice")
	}

	if len(incoming) != 0 {
		t.Errorf("incoming = %d, want 0", len(incoming))
	}
}

//nolint:dupl
func TestCallHierarchyOutgoing(t *testing.T) {
	t.Parallel()

	s := NewServer()
	openDoc(t, s, serverDocA, chDoc)

	items, err := s.PrepareCallHierarchy(t.Context(), &protocol.CallHierarchyPrepareParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: uri.URI(serverDocA)},
			Position:     cursorOn(t, chDoc, 5, "GetUser"),
		},
	})
	if err != nil || len(items) != 1 {
		t.Fatalf("prepare failed: %v, %d", err, len(items))
	}

	outgoing, err := s.OutgoingCalls(t.Context(), &protocol.CallHierarchyOutgoingCallsParams{Item: items[0]})
	if err != nil {
		t.Fatalf("OutgoingCalls: %v", err)
	}

	if outgoing == nil {
		t.Fatalf("outgoing nil, want empty slice")
	}

	if len(outgoing) != 0 {
		t.Errorf("outgoing = %d, want 0", len(outgoing))
	}
}

func TestCallHierarchyAliases(t *testing.T) {
	t.Parallel()

	s := NewServer()
	openDoc(t, s, serverDocA, chDoc)

	items, err := s.TextDocumentPrepareCallHierarchy(t.Context(), &protocol.CallHierarchyPrepareParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: uri.URI(serverDocA)},
			Position:     cursorOn(t, chDoc, 5, "GetUser"),
		},
	})
	if err != nil || len(items) != 1 {
		t.Fatalf("alias prepare failed: %v, %d", err, len(items))
	}

	incoming, err := s.CallHierarchyIncomingCalls(t.Context(), &protocol.CallHierarchyIncomingCallsParams{Item: items[0]})
	if err != nil {
		t.Fatalf("alias incoming failed: %v", err)
	}

	if len(incoming) != 1 {
		t.Errorf("alias incoming = %d, want 1", len(incoming))
	}

	outgoing, err := s.CallHierarchyOutgoingCalls(t.Context(), &protocol.CallHierarchyOutgoingCallsParams{Item: items[0]})
	if err != nil {
		t.Fatalf("alias outgoing failed: %v", err)
	}

	if len(outgoing) != 0 {
		t.Errorf("alias outgoing = %d, want 0", len(outgoing))
	}
}
