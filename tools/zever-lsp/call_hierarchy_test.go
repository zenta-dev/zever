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

// chSagaDoc is a saga fixture with both reference kinds: `reserve` carries an
// execute plus a compensate, `extra` a lone execute. Line map (0-based) used
// by cursor positions below:
//
//	13: saga Checkout {
//	14: \tstep reserve {
//	15: \t\texecute: UserService.GetUser
//	16: \t\tcompensate: UserService.RefundUser
//	17: \t}
//	18: \tstep extra {
//	19: \t\texecute: UserService.RefundUser
//	20: \t}
//	21: }
const chSagaDoc = `entity User {
	id: uuid @primary
}

service UserService {
	rpc GetUser(id: uuid) -> User {
		auth: none
	}
	rpc RefundUser(id: uuid) -> User {
		auth: none
	}
}

saga Checkout {
	step reserve {
		execute: UserService.GetUser
		compensate: UserService.RefundUser
	}
	step extra {
		execute: UserService.RefundUser
	}
}
`

// prepareSaga opens doc, prepares on the saga name, and returns the single
// saga item, failing the test on any deviation. The parameters stay even
// though every fixture below names its saga Checkout: they keep the helper
// reusable for the next saga fixture.
//
//nolint:unparam // constant args across current callers, kept for reuse.
func prepareSaga(t *testing.T, s *Server, doc, saga string, line int) protocol.CallHierarchyItem {
	t.Helper()

	openDoc(t, s, serverDocA, doc)

	items, err := s.PrepareCallHierarchy(t.Context(), &protocol.CallHierarchyPrepareParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: uri.URI(serverDocA)},
			Position:     cursorOn(t, doc, line, saga),
		},
	})
	if err != nil {
		t.Fatalf("PrepareCallHierarchy: %v", err)
	}

	if len(items) != 1 {
		t.Fatalf("items = %d, want 1 for saga %s", len(items), saga)
	}

	return items[0]
}

// TestCallHierarchyPrepareSaga pins the saga prepare shape: one Event item
// named for the saga, detailed as its saga, carrying the saga (not an RPC)
// payload.
func TestCallHierarchyPrepareSaga(t *testing.T) {
	t.Parallel()

	s := NewServer()
	item := prepareSaga(t, s, chDoc, "Checkout", 13)

	if item.Name != "Checkout" {
		t.Errorf("Name = %q, want Checkout", item.Name)
	}

	if item.Kind != protocol.SymbolKindEvent {
		t.Errorf("Kind = %v, want Event", item.Kind)
	}

	if item.Detail == nil || *item.Detail != "saga Checkout" {
		t.Errorf("Detail = %v, want \"saga Checkout\"", item.Detail)
	}

	if got := callHierarchySagaFromItem(item); got != "Checkout" {
		t.Errorf("callHierarchySagaFromItem() = %q, want Checkout", got)
	}

	if _, _, ok := callHierarchyDataFromAny(item.Data); ok {
		t.Errorf("callHierarchyDataFromAny(saga item) ok = true, want false (a saga names no RPC)")
	}
}

// TestCallHierarchyOutgoingSaga is the core outgoing case: preparing on the
// saga yields one entry per step reference, each naming the referenced RPC
// with the reference site as its FromRanges.
func TestCallHierarchyOutgoingSaga(t *testing.T) {
	t.Parallel()

	s := NewServer()
	item := prepareSaga(t, s, chDoc, "Checkout", 13)

	outgoing, err := s.OutgoingCalls(t.Context(), &protocol.CallHierarchyOutgoingCallsParams{Item: item})
	if err != nil {
		t.Fatalf("OutgoingCalls: %v", err)
	}

	if len(outgoing) != 1 {
		t.Fatalf("outgoing = %d, want 1 (the single execute reference)", len(outgoing))
	}

	call := outgoing[0]

	if call.To.Name != "GetUser" {
		t.Errorf("To.Name = %q, want GetUser", call.To.Name)
	}

	if call.To.Kind != protocol.SymbolKindMethod {
		t.Errorf("To.Kind = %v, want Method", call.To.Kind)
	}

	if call.To.Detail == nil || *call.To.Detail != "service UserService" {
		t.Errorf("To.Detail = %v, want \"service UserService\"", call.To.Detail)
	}

	if len(call.FromRanges) != 1 {
		t.Fatalf("FromRanges = %d, want 1 (the execute reference site)", len(call.FromRanges))
	}

	// The callee item round-trips: its Data names the RPC, so feeding it back
	// into incomingCalls finds the saga that references it.
	svc, rpc, ok := callHierarchyDataFromAny(call.To.Data)
	if !ok || svc != "UserService" || rpc != "GetUser" {
		t.Fatalf("To.Data = (%q, %q, %v), want (UserService, GetUser, true)", svc, rpc, ok)
	}

	incoming, err := s.IncomingCalls(t.Context(), &protocol.CallHierarchyIncomingCallsParams{Item: call.To})
	if err != nil {
		t.Fatalf("IncomingCalls(To): %v", err)
	}

	if len(incoming) != 1 || incoming[0].From.Name != "Checkout" {
		t.Errorf("incoming(To) = %+v, want the Checkout caller", incoming)
	}
}

// TestCallHierarchyOutgoingSagaSteps covers both reference kinds: execute and
// compensate references each become their own entry, in step order.
func TestCallHierarchyOutgoingSagaSteps(t *testing.T) {
	t.Parallel()

	s := NewServer()
	item := prepareSaga(t, s, chSagaDoc, "Checkout", 13)

	outgoing, err := s.OutgoingCalls(t.Context(), &protocol.CallHierarchyOutgoingCallsParams{Item: item})
	if err != nil {
		t.Fatalf("OutgoingCalls: %v", err)
	}

	want := []string{"GetUser", "RefundUser", "RefundUser"}
	if len(outgoing) != len(want) {
		t.Fatalf("outgoing = %d, want %d (execute, compensate, execute)", len(outgoing), len(want))
	}

	for i, call := range outgoing {
		if call.To.Name != want[i] {
			t.Errorf("outgoing[%d].To.Name = %q, want %q", i, call.To.Name, want[i])
		}

		if len(call.FromRanges) != 1 {
			t.Errorf("outgoing[%d].FromRanges = %d, want 1", i, len(call.FromRanges))
		}
	}
}

// TestCallHierarchySagaItemIncomingIsEmpty pins the directionality: a saga
// item names no RPC, so incoming calls from it is honestly empty rather than
// a lookup against a garbage service derived from its "saga ..." detail.
func TestCallHierarchySagaItemIncomingIsEmpty(t *testing.T) {
	t.Parallel()

	s := NewServer()
	item := prepareSaga(t, s, chDoc, "Checkout", 13)

	incoming, err := s.IncomingCalls(t.Context(), &protocol.CallHierarchyIncomingCallsParams{Item: item})
	if err != nil {
		t.Fatalf("IncomingCalls(saga item): %v", err)
	}

	if incoming == nil {
		t.Fatal("incoming nil, want empty slice")
	}

	if len(incoming) != 0 {
		t.Errorf("incoming = %d, want 0 (a saga names no RPC)", len(incoming))
	}
}

// TestCallHierarchySagaDataCompat guards the JSON wire shape: old two-field
// RPC payloads still decode with no saga, and new saga payloads decode in
// every shape a client or test can produce.
func TestCallHierarchySagaDataCompat(t *testing.T) {
	t.Parallel()

	t.Run("old rpc payload has no saga", func(t *testing.T) {
		t.Parallel()

		raw := protocol.LSPAny([]byte(`{"service":"UserService","rpc":"GetUser"}`))

		svc, rpc, ok := callHierarchyDataFromAny(raw)
		if !ok || svc != "UserService" || rpc != "GetUser" {
			t.Fatalf("callHierarchyDataFromAny(old) = (%q, %q, %v), want (UserService, GetUser, true)", svc, rpc, ok)
		}

		item := protocol.CallHierarchyItem{Name: "GetUser", Data: raw}
		if got := callHierarchySagaFromItem(item); got != "" {
			t.Errorf("callHierarchySagaFromItem(old rpc payload) = %q, want \"\"", got)
		}
	})

	t.Run("saga payload in every shape", func(t *testing.T) {
		t.Parallel()

		saga := "Checkout"
		raw := protocol.LSPAny([]byte(`{"saga":"Checkout"}`))

		// Raw JSON is the only shape that fits the typed Data field; the
		// struct/pointer/map shapes go straight into the payload decoder,
		// which also backs the (service, rpc) recovery.
		item := protocol.CallHierarchyItem{Name: saga, Data: raw}
		if got := callHierarchySagaFromItem(item); got != saga {
			t.Errorf("callHierarchySagaFromItem(raw json) = %q, want %q", got, saga)
		}

		if _, _, ok := callHierarchyDataFromAny(raw); ok {
			t.Errorf("callHierarchyDataFromAny(saga payload) ok = true, want false")
		}

		payloads := map[string]any{
			"struct":  callHierarchyData{Saga: saga},
			"pointer": &callHierarchyData{Saga: saga},
			"map":     map[string]any{"saga": saga},
		}

		for name, data := range payloads {
			t.Run(name, func(t *testing.T) {
				t.Parallel()

				payload, ok := callHierarchyPayloadFromAny(data)
				if !ok || payload.Saga != saga {
					t.Errorf("callHierarchyPayloadFromAny(%s) = (%+v, %v), want saga %q", name, payload, ok, saga)
				}

				if _, _, ok := callHierarchyDataFromAny(data); ok {
					t.Errorf("callHierarchyDataFromAny(%s saga payload) ok = true, want false", name)
				}
			})
		}
	})

	t.Run("detail fallback", func(t *testing.T) {
		t.Parallel()

		detail := "saga Checkout"
		item := protocol.CallHierarchyItem{Name: "Checkout", Detail: &detail}

		if got := callHierarchySagaFromItem(item); got != "Checkout" {
			t.Errorf("callHierarchySagaFromItem(detail fallback) = %q, want Checkout", got)
		}

		if svc, rpc := callHierarchyTargetFromItem(item); svc != "" || rpc != "" {
			t.Errorf("callHierarchyTargetFromItem(saga item) = (%q, %q), want (\"\", \"\")", svc, rpc)
		}
	})
}
