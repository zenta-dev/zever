package main

import (
	"context"
	"strings"

	"go.lsp.dev/protocol"

	"github.com/zenta-dev/zever/dsl/ast"
	"github.com/zenta-dev/zever/dsl/diag"

	"github.com/zenta-dev/zever/shared/codec"
)

// callHierarchyData is the payload stored on a CallHierarchyItem's Data field
// so callHierarchy/incomingCalls can recover which service and RPC the item
// names without re-resolving the schema. Saga prepare items carry Saga
// instead (a saga has no owning service); the field is omitempty so RPC
// payloads written before sagas existed decode unchanged, and old two-field
// JSON decodes with Saga empty. It round-trips through JSON exactly like
// completionResolveData.
type callHierarchyData struct {
	Service string `json:"service"`
	RPC     string `json:"rpc"`
	// Saga names the saga a prepare item refers to. Empty for RPC items.
	Saga string `json:"saga,omitempty"`
}

const (
	callHierarchyServiceKey = "service"
	callHierarchyRPCKey     = "rpc"
	callHierarchySagaKey    = "saga"
)

// callHierarchyDataFromAny recovers the (service, rpc) pair a prepare request
// attached to an item's Data field, in any of the shapes a real client or an
// in-process test can produce (raw JSON bytes, the struct itself, a pointer,
// or the map[string]any a JSON decode into `any` yields). A saga-only
// payload names no RPC, so it yields ok == false here; saga recovery lives in
// callHierarchySagaFromItem.
func callHierarchyDataFromAny(data any) (string, string, bool) {
	payload, ok := callHierarchyPayloadFromAny(data)
	if !ok || (payload.Service == "" && payload.RPC == "") {
		return "", "", false
	}

	return payload.Service, payload.RPC, true
}

// callHierarchyTargetFromItem recovers the (service, rpc) pair an incoming
// request's item refers to, preferring the Data field and falling back to the
// item's Detail ("service S") and Name when Data is absent or undecodable. A
// saga item ("saga S" detail) names no RPC, so it yields empty strings and
// RPC-oriented callers answer empty rather than chasing a garbage service.
func callHierarchyTargetFromItem(item protocol.CallHierarchyItem) (string, string) {
	if svc, rpc, ok := callHierarchyDataFromAny(item.Data); ok {
		if svc == "" && item.Detail != nil {
			svc = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(*item.Detail), "service"))
		}

		if rpc == "" {
			rpc = item.Name
		}

		return svc, rpc
	}

	svc := ""

	if item.Detail != nil {
		detail := strings.TrimSpace(*item.Detail)
		if strings.HasPrefix(detail, "saga") {
			return "", ""
		}

		svc = strings.TrimSpace(strings.TrimPrefix(detail, "service"))
		svc = strings.TrimSpace(svc)
	}

	return svc, item.Name
}

// callHierarchySagaFromItem recovers the saga name an outgoing request's item
// refers to, preferring the Data field and falling back to the item's Detail
// ("saga S") and Name when Data is absent or undecodable. RPC items yield "".
func callHierarchySagaFromItem(item protocol.CallHierarchyItem) string {
	if payload, ok := callHierarchyPayloadFromAny(item.Data); ok && payload.Saga != "" {
		return payload.Saga
	}

	if item.Detail != nil {
		detail := strings.TrimSpace(*item.Detail)
		if strings.HasPrefix(detail, "saga") && strings.TrimSpace(strings.TrimPrefix(detail, "saga")) != "" {
			return item.Name
		}
	}

	return ""
}

// callHierarchyPayloadFromAny decodes a Data field in any of the shapes a
// real client or an in-process test can produce (raw JSON bytes, the struct
// itself, a pointer, or the map[string]any a JSON decode into `any` yields).
// It underpins both the (service, rpc) recovery above and the saga recovery:
// one decoder so the shape matrix lives in exactly one place.
func callHierarchyPayloadFromAny(data any) (callHierarchyData, bool) {
	if raw, ok := data.(protocol.LSPAny); ok {
		var payload callHierarchyData
		if err := codec.Unmarshal(raw, &payload); err == nil &&
			(payload.Service != "" || payload.RPC != "" || payload.Saga != "") {
			return payload, true
		}

		var m map[string]any
		if err := codec.Unmarshal(raw, &m); err == nil {
			payload = callHierarchyData{}
			payload.Service, _ = m[callHierarchyServiceKey].(string)
			payload.RPC, _ = m[callHierarchyRPCKey].(string)
			payload.Saga, _ = m[callHierarchySagaKey].(string)

			if payload.Service != "" || payload.RPC != "" || payload.Saga != "" {
				return payload, true
			}
		}

		return callHierarchyData{}, false
	}

	switch v := data.(type) {
	case callHierarchyData:
		return v, v.Service != "" || v.RPC != "" || v.Saga != ""
	case *callHierarchyData:
		if v == nil {
			return callHierarchyData{}, false
		}

		return *v, v.Service != "" || v.RPC != "" || v.Saga != ""
	case map[string]any:
		payload := callHierarchyData{}
		payload.Service, _ = v[callHierarchyServiceKey].(string)
		payload.RPC, _ = v[callHierarchyRPCKey].(string)
		payload.Saga, _ = v[callHierarchySagaKey].(string)

		return payload, payload.Service != "" || payload.RPC != "" || payload.Saga != ""
	}

	return callHierarchyData{}, false
}

// prepareCallHierarchyItems builds the prepare response for one document.
func prepareCallHierarchyItems(s *Server, params *protocol.CallHierarchyPrepareParams) []protocol.CallHierarchyItem {
	if params == nil {
		return nil
	}

	file := s.parseDoc(string(params.TextDocument.URI))
	if file == nil {
		return nil
	}

	target, ok := renameTargetAt(file, params.Position)
	if !ok {
		return nil
	}

	switch target.Kind { //nolint:exhaustive // only rpc and service kinds produce items
	case rpcKind:
		detail := "service " + target.Owner
		item := protocol.CallHierarchyItem{
			Name:           target.Name,
			Kind:           protocol.SymbolKindMethod,
			Detail:         &detail,
			URI:            params.TextDocument.URI,
			Range:          identRange(target.Pos, target.Name),
			SelectionRange: identRange(target.Pos, target.Name),
		}

		if raw, err := codec.Marshal(callHierarchyData{Service: target.Owner, RPC: target.Name}); err == nil {
			item.Data = protocol.LSPAny(raw)
		}

		return []protocol.CallHierarchyItem{item}

	case serviceKind:
		var svc *ast.ServiceDecl
		for _, decl := range flattenDecls(file) {
			if candidate, ok := decl.(*ast.ServiceDecl); ok && candidate.Name == target.Name {
				svc = candidate
				break
			}
		}

		if svc == nil {
			return nil
		}

		out := make([]protocol.CallHierarchyItem, 0, len(svc.RPCs))
		for _, rpc := range svc.RPCs {
			if rpc == nil || rpc.Name == "" {
				continue
			}

			detail := "service " + svc.Name
			item := protocol.CallHierarchyItem{
				Name:           rpc.Name,
				Kind:           protocol.SymbolKindMethod,
				Detail:         &detail,
				URI:            params.TextDocument.URI,
				Range:          identRange(rpc.NamePos, rpc.Name),
				SelectionRange: identRange(rpc.NamePos, rpc.Name),
			}

			if raw, err := codec.Marshal(callHierarchyData{Service: svc.Name, RPC: rpc.Name}); err == nil {
				item.Data = protocol.LSPAny(raw)
			}

			out = append(out, item)
		}

		if len(out) == 0 {
			return nil
		}

		return out

	case sagaKind:
		var saga *ast.SagaDecl
		for _, decl := range flattenDecls(file) {
			if candidate, ok := decl.(*ast.SagaDecl); ok && candidate.Name == target.Name {
				saga = candidate
				break
			}
		}

		if saga == nil {
			return nil
		}

		detail := "saga " + saga.Name
		item := protocol.CallHierarchyItem{
			Name:           saga.Name,
			Kind:           protocol.SymbolKindEvent,
			Detail:         &detail,
			URI:            params.TextDocument.URI,
			Range:          identRange(saga.NamePos, saga.Name),
			SelectionRange: identRange(saga.NamePos, saga.Name),
		}

		if raw, err := codec.Marshal(callHierarchyData{Saga: saga.Name}); err == nil {
			item.Data = protocol.LSPAny(raw)
		}

		return []protocol.CallHierarchyItem{item}

	default:
		return nil
	}
}

// incomingCalls builds the incoming-call list for one RPC item.
func incomingCalls(s *Server, params *protocol.CallHierarchyIncomingCallsParams) []protocol.CallHierarchyIncomingCall {
	out := []protocol.CallHierarchyIncomingCall{}
	if params == nil {
		return out
	}

	service, rpc := callHierarchyTargetFromItem(params.Item)
	if service == "" || rpc == "" {
		return out
	}

	parsedFiles := s.ws.ParsedFiles()
	for path, file := range parsedFiles {
		if file == nil {
			continue
		}

		for _, decl := range flattenDecls(file) {
			saga, ok := decl.(*ast.SagaDecl)
			if !ok {
				continue
			}

			for _, step := range saga.Steps {
				if step == nil {
					continue
				}

				ranges := []protocol.Range{}
				if step.Execute != nil &&
					step.Execute.Service == service && step.Execute.RPC == rpc {
					ranges = append(ranges, identRange(rpcRefPos(step.Execute), step.Execute.RPC))
				}

				if step.Compensate != nil &&
					step.Compensate.Service == service && step.Compensate.RPC == rpc {
					ranges = append(ranges, identRange(rpcRefPos(step.Compensate), step.Compensate.RPC))
				}

				if len(ranges) == 0 {
					continue
				}

				item := protocol.CallHierarchyItem{
					Name:           saga.Name,
					Kind:           protocol.SymbolKindEvent,
					URI:            pathToURI(path),
					Range:          identRange(saga.NamePos, saga.Name),
					SelectionRange: identRange(saga.NamePos, saga.Name),
				}

				out = append(out, protocol.CallHierarchyIncomingCall{
					From:       item,
					FromRanges: ranges,
				})
			}
		}
	}

	return out
}

// outgoingCalls builds the outgoing-call list for one saga item: one entry
// per step reference (each step's execute, plus its compensate when present),
// with FromRanges pointing at the RPC identifier in the saga file. The callee
// item names the referenced RPC and anchors on its declaration when the
// declaring service is visible, falling back to the reference itself
// mid-edit. An RPC item yields no calls: RPCs in this DSL do not call other
// RPCs (only saga steps reference RPCs, which is the incoming direction), so
// outgoing from an RPC is honestly empty rather than guessed.
func outgoingCalls(s *Server, params *protocol.CallHierarchyOutgoingCallsParams) []protocol.CallHierarchyOutgoingCall {
	out := []protocol.CallHierarchyOutgoingCall{}
	if s == nil || params == nil {
		return out
	}

	sagaName := callHierarchySagaFromItem(params.Item)
	if sagaName == "" {
		return out
	}

	parsedFiles := s.ws.ParsedFiles()
	for path, file := range parsedFiles {
		if file == nil {
			continue
		}

		for _, decl := range flattenDecls(file) {
			saga, ok := decl.(*ast.SagaDecl)
			if !ok || saga.Name != sagaName {
				continue
			}

			for _, step := range saga.Steps {
				if step == nil {
					continue
				}

				if step.Execute != nil {
					out = append(out, sagaOutgoingCall(parsedFiles, path, step.Execute))
				}

				if step.Compensate != nil {
					out = append(out, sagaOutgoingCall(parsedFiles, path, step.Compensate))
				}
			}
		}
	}

	return out
}

// sagaOutgoingCall builds one outgoing-call entry for a saga step's RPC
// reference. The To item anchors on the RPC's declaration when its service is
// visible in the workspace, and on the reference itself when the service is
// unknown (mid-edit); either way FromRanges marks the reference site in the
// saga file.
func sagaOutgoingCall(files map[string]*ast.File, sagaPath string, ref *ast.RPCRef) protocol.CallHierarchyOutgoingCall {
	from := identRange(rpcRefPos(ref), ref.RPC)

	toURI := pathToURI(sagaPath)
	toRange := from

	if pos, path, ok := findRPCDecl(files, ref.Service, ref.RPC); ok {
		toURI = pathToURI(path)
		toRange = identRange(pos, ref.RPC)
	}

	detail := "service " + ref.Service
	item := protocol.CallHierarchyItem{
		Name:           ref.RPC,
		Kind:           protocol.SymbolKindMethod,
		Detail:         &detail,
		URI:            toURI,
		Range:          toRange,
		SelectionRange: toRange,
	}

	if raw, err := codec.Marshal(callHierarchyData{Service: ref.Service, RPC: ref.RPC}); err == nil {
		item.Data = protocol.LSPAny(raw)
	}

	return protocol.CallHierarchyOutgoingCall{
		To:         item,
		FromRanges: []protocol.Range{from},
	}
}

// findRPCDecl looks an RPC up by its service and name across every parsed
// file, returning the RPC name's position and the declaring file's path. It
// reports ok == false when the service or RPC is unknown (mid-edit).
func findRPCDecl(files map[string]*ast.File, service, rpc string) (diag.Position, string, bool) {
	for path, file := range files {
		if file == nil {
			continue
		}

		for _, decl := range flattenDecls(file) {
			svc, ok := decl.(*ast.ServiceDecl)
			if !ok || svc.Name != service {
				continue
			}

			for _, candidate := range svc.RPCs {
				if candidate != nil && candidate.Name == rpc {
					return candidate.NamePos, path, true
				}
			}
		}
	}

	return diag.Position{}, "", false
}

// Server method implementations and task-named aliases.

func (s *Server) PrepareCallHierarchy(ctx context.Context, params *protocol.CallHierarchyPrepareParams) ([]protocol.CallHierarchyItem, error) {
	s.captureClient(ctx)

	return prepareCallHierarchyItems(s, params), nil
}

func (s *Server) TextDocumentPrepareCallHierarchy(ctx context.Context, params *protocol.CallHierarchyPrepareParams) ([]protocol.CallHierarchyItem, error) {
	return s.PrepareCallHierarchy(ctx, params)
}

func (s *Server) IncomingCalls(ctx context.Context, params *protocol.CallHierarchyIncomingCallsParams) ([]protocol.CallHierarchyIncomingCall, error) {
	s.captureClient(ctx)

	return incomingCalls(s, params), nil
}

func (s *Server) CallHierarchyIncomingCalls(ctx context.Context, params *protocol.CallHierarchyIncomingCallsParams) ([]protocol.CallHierarchyIncomingCall, error) {
	return s.IncomingCalls(ctx, params)
}

func (s *Server) OutgoingCalls(ctx context.Context, params *protocol.CallHierarchyOutgoingCallsParams) ([]protocol.CallHierarchyOutgoingCall, error) {
	s.captureClient(ctx)

	return outgoingCalls(s, params), nil
}

func (s *Server) CallHierarchyOutgoingCalls(ctx context.Context, params *protocol.CallHierarchyOutgoingCallsParams) ([]protocol.CallHierarchyOutgoingCall, error) {
	return s.OutgoingCalls(ctx, params)
}
