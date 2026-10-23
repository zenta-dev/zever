package main

import (
	"context"
	"encoding/json"
	"strings"

	"go.lsp.dev/protocol"

	"github.com/zenta-dev/zever/dsl/ast"
)

// callHierarchyData is the payload stored on a CallHierarchyItem's Data field
// so callHierarchy/incomingCalls can recover which service and RPC the item
// names without re-resolving the schema. It round-trips through JSON exactly
// like completionResolveData.
type callHierarchyData struct {
	Service string `json:"service"`
	RPC     string `json:"rpc"`
}

const (
	callHierarchyServiceKey = "service"
	callHierarchyRPCKey     = "rpc"
)

// callHierarchyDataFromAny recovers the (service, rpc) pair a prepare request
// attached to an item's Data field, in any of the shapes a real client or an
// in-process test can produce (raw JSON bytes, the struct itself, a pointer,
// or the map[string]any a JSON decode into `any` yields).
func callHierarchyDataFromAny(data any) (string, string, bool) {
	if raw, ok := data.(protocol.LSPAny); ok {
		var payload callHierarchyData
		if err := json.Unmarshal(raw, &payload); err == nil &&
			(payload.Service != "" || payload.RPC != "") {
			return payload.Service, payload.RPC, true
		}

		var m map[string]any
		if err := json.Unmarshal(raw, &m); err == nil {
			svc, _ := m[callHierarchyServiceKey].(string)
			rpc, _ := m[callHierarchyRPCKey].(string)
			if svc != "" || rpc != "" {
				return svc, rpc, true
			}
		}

		return "", "", false
	}

	switch v := data.(type) {
	case callHierarchyData:
		return v.Service, v.RPC, v.Service != "" || v.RPC != ""
	case *callHierarchyData:
		if v == nil {
			return "", "", false
		}

		return v.Service, v.RPC, v.Service != "" || v.RPC != ""
	case map[string]any:
		svc, _ := v[callHierarchyServiceKey].(string)
		rpc, _ := v[callHierarchyRPCKey].(string)
		if svc != "" || rpc != "" {
			return svc, rpc, true
		}
	}

	return "", "", false
}

// callHierarchyTargetFromItem recovers the (service, rpc) pair an incoming
// request's item refers to, preferring the Data field and falling back to the
// item's Detail ("service S") and Name when Data is absent or undecodable.
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
		svc = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(*item.Detail), "service"))
		svc = strings.TrimSpace(svc)
	}

	return svc, item.Name
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

		if raw, err := json.Marshal(callHierarchyData{Service: target.Owner, RPC: target.Name}); err == nil {
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

			if raw, err := json.Marshal(callHierarchyData{Service: svc.Name, RPC: rpc.Name}); err == nil {
				item.Data = protocol.LSPAny(raw)
			}

			out = append(out, item)
		}

		if len(out) == 0 {
			return nil
		}

		return out

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

// outgoingCalls returns no calls: RPCs in this DSL do not call other RPCs.
func outgoingCalls(_ *protocol.CallHierarchyOutgoingCallsParams) []protocol.CallHierarchyOutgoingCall {
	return []protocol.CallHierarchyOutgoingCall{}
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

	return outgoingCalls(params), nil
}

func (s *Server) CallHierarchyOutgoingCalls(ctx context.Context, params *protocol.CallHierarchyOutgoingCallsParams) ([]protocol.CallHierarchyOutgoingCall, error) {
	return s.OutgoingCalls(ctx, params)
}
