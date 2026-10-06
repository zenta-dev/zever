package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

const (
	serverName             = "zever-mcp"
	serverVersion          = "0.5.3"
	defaultProtocolVersion = "2024-11-05"
)

// toolHandler executes a tool and returns its textual result. An error is
// reported to the client as an MCP tool execution error (isError).
type toolHandler func(ctx context.Context, args map[string]any) (string, error)

// toolDef describes one MCP tool and its handler.
type toolDef struct {
	Name        string
	Description string
	InputSchema map[string]any
	Handler     toolHandler
}

// Server is the zever MCP server: a method dispatcher over a tool set.
type Server struct {
	tools map[string]toolDef
	order []string
}

// newServer builds a Server with every zever tool registered.
func newServer() *Server {
	s := &Server{tools: map[string]toolDef{}}
	s.register(compileTool())
	s.register(schemaTool())
	s.register(explainTool())
	return s
}

// register adds t to the server, preserving registration order.
func (s *Server) register(t toolDef) {
	s.tools[t.Name] = t
	s.order = append(s.order, t.Name)
}

// run reads newline-delimited JSON-RPC requests from in and writes responses to
// out until in reaches EOF. Diagnostics go to errw.
func run(ctx context.Context, in io.Reader, out io.Writer, errw io.Writer) int {
	s := newServer()
	dec := json.NewDecoder(in)
	enc := json.NewEncoder(out)

	for {
		var req rpcRequest
		if err := dec.Decode(&req); err != nil {
			if errors.Is(err, io.EOF) {
				return 0
			}
			_ = enc.Encode(errorResponse(json.RawMessage("null"), codeParseError, "zever-mcp: parse error"))
			fmt.Fprintln(errw, "zever-mcp: decode:", err)
			return 1
		}

		if req.isNotification() {
			continue
		}

		if err := enc.Encode(s.handle(ctx, req)); err != nil {
			fmt.Fprintln(errw, "zever-mcp: encode:", err)
			return 1
		}
	}
}

// handle dispatches one request to a method handler.
func (s *Server) handle(ctx context.Context, req rpcRequest) rpcResponse {
	if req.JSONRPC != "2.0" {
		return errorResponse(req.ID, codeInvalidRequest, "zever-mcp: invalid jsonrpc version")
	}

	switch req.Method {
	case "initialize":
		return s.initializeResponse(req)
	case "ping":
		return rpcResponse{JSONRPC: "2.0", ID: req.ID, Result: map[string]any{}}
	case "tools/list":
		return rpcResponse{JSONRPC: "2.0", ID: req.ID, Result: map[string]any{"tools": s.toolList()}}
	case "tools/call":
		return s.callTool(ctx, req)
	default:
		return errorResponse(req.ID, codeMethodNotFound, "zever-mcp: method not found: "+req.Method)
	}
}

// initializeParams is the subset of the MCP initialize request this server uses.
type initializeParams struct {
	ProtocolVersion string `json:"protocolVersion"`
}

// supportedProtocolVersions is the set of MCP versions this server implements.
var supportedProtocolVersions = map[string]bool{
	"2024-11-05": true,
	"2025-06-18": true,
}

// initializeResponse negotiates the protocol version and advertises
// capabilities, falling back to the default when the client requests a version
// this server does not implement.
func (s *Server) initializeResponse(req rpcRequest) rpcResponse {
	var p initializeParams
	if len(req.Params) > 0 {
		if err := json.Unmarshal(req.Params, &p); err != nil {
			return errorResponse(req.ID, codeInvalidParams, "zever-mcp: invalid initialize params")
		}
	}

	version := defaultProtocolVersion
	if supportedProtocolVersions[p.ProtocolVersion] {
		version = p.ProtocolVersion
	}

	return rpcResponse{JSONRPC: "2.0", ID: req.ID, Result: map[string]any{
		"protocolVersion": version,
		"capabilities":    map[string]any{"tools": map[string]any{}},
		"serverInfo":      map[string]any{"name": serverName, "version": serverVersion},
	}}
}

// toolList returns the MCP tools/list payload in registration order.
func (s *Server) toolList() []map[string]any {
	out := make([]map[string]any, 0, len(s.order))
	for _, name := range s.order {
		t := s.tools[name]
		out = append(out, map[string]any{
			"name":        t.Name,
			"description": t.Description,
			"inputSchema": t.InputSchema,
		})
	}
	return out
}

// callParams is the MCP tools/call request payload.
type callParams struct {
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments"`
}

// callTool resolves and runs a tool, mapping handler errors to MCP tool
// execution errors so the model can self-correct.
func (s *Server) callTool(ctx context.Context, req rpcRequest) rpcResponse {
	var p callParams
	if err := json.Unmarshal(req.Params, &p); err != nil {
		return errorResponse(req.ID, codeInvalidParams, "zever-mcp: invalid tools/call params")
	}

	t, ok := s.tools[p.Name]
	if !ok {
		return errorResponse(req.ID, codeInvalidParams, "zever-mcp: unknown tool: "+p.Name)
	}

	text, err := t.Handler(ctx, p.Arguments)
	if err != nil {
		return rpcResponse{JSONRPC: "2.0", ID: req.ID, Result: callToolResult{
			Content: []textContent{{Type: "text", Text: err.Error()}},
			IsError: true,
		}}
	}

	return rpcResponse{JSONRPC: "2.0", ID: req.ID, Result: callToolResult{
		Content: []textContent{{Type: "text", Text: text}},
	}}
}

// errorResponse builds a JSON-RPC error response.
func errorResponse(id json.RawMessage, code int, msg string) rpcResponse {
	return rpcResponse{JSONRPC: "2.0", ID: id, Error: &rpcError{Code: code, Message: msg}}
}
