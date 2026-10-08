package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sync"
	"sync/atomic"
)

const (
	serverName             = "zever-mcp"
	serverVersion          = "0.6.1"
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
// Tools/call handlers run in their own goroutines so elicitation
// round-trips keep flowing while a handler waits. Handler implementations
// must therefore be safe for concurrent use.
type Server struct {
	tools map[string]toolDef
	order []string

	// mu guards sender and pending.
	mu sync.Mutex
	// sender delivers server-originated messages; nil outside run().
	sender senderFunc
	// pending correlates elicitation answers by request ID.
	pending map[int64]chan elicitResult
	// nextServerID issues server-originated request IDs.
	nextServerID atomic.Int64
	// encMu serializes outbound encodes across handler goroutines.
	encMu sync.Mutex
}

// getSender returns the outbound sender, or nil when the server is not running.
func (s *Server) getSender() senderFunc {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.sender
}

// newServer builds a Server with every zever tool registered.
func newServer() *Server {
	s := &Server{tools: map[string]toolDef{}, pending: map[int64]chan elicitResult{}}
	s.register(compileTool())
	s.register(schemaTool())
	s.register(explainTool())
	s.register(doctorTool())
	s.register(s.generateTool())
	return s
}

// register adds t to the server, preserving registration order.
func (s *Server) register(t toolDef) {
	s.tools[t.Name] = t
	s.order = append(s.order, t.Name)
}

// run reads newline-delimited JSON-RPC messages from in and writes responses
// to out until in reaches EOF. Client requests dispatch to handle(); client
// answers to server-initiated elicitations route to the pending waiter.
// Tools/call handlers each run in their own goroutine so an eliciting
// handler never stalls the read loop. Diagnostics go to errw.
func run(ctx context.Context, in io.Reader, out io.Writer, errw io.Writer) int {
	s := newServer()
	dec := json.NewDecoder(in)
	enc := json.NewEncoder(out)

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	s.mu.Lock()
	s.sender = func(v any) error {
		s.encMu.Lock()
		defer s.encMu.Unlock()

		return enc.Encode(v)
	}
	s.mu.Unlock()

	var wg sync.WaitGroup
	failed := false

	respond := func(resp rpcResponse) {
		s.encMu.Lock()
		defer s.encMu.Unlock()

		if err := enc.Encode(resp); err != nil {
			fmt.Fprintln(errw, "zever-mcp: encode:", err)
			failed = true
		}
	}

	for {
		var raw json.RawMessage
		if err := dec.Decode(&raw); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			respond(errorResponse(json.RawMessage("null"), codeParseError, "zever-mcp: parse error"))
			fmt.Fprintln(errw, "zever-mcp: decode:", err)
			cancel()
			wg.Wait()

			return 1
		}

		if s.deliverResponse(raw) {
			continue
		}

		var req rpcRequest
		if err := json.Unmarshal(raw, &req); err != nil {
			respond(errorResponse(json.RawMessage("null"), codeInvalidRequest, "zever-mcp: malformed request"))
			continue
		}

		if req.isNotification() {
			continue
		}

		if req.Method == "tools/call" {
			wg.Add(1)

			go func() {
				defer wg.Done()
				respond(s.handle(ctx, req))
			}()

			continue
		}

		respond(s.handle(ctx, req))
	}

	cancel()
	wg.Wait()

	if failed {
		return 1
	}

	return 0
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
	case "resources/list":
		return rpcResponse{JSONRPC: "2.0", ID: req.ID, Result: map[string]any{"resources": resourceList()}}
	case "resources/read":
		return readResource(req)
	case "prompts/list":
		return rpcResponse{JSONRPC: "2.0", ID: req.ID, Result: map[string]any{"prompts": promptList()}}
	case "prompts/get":
		return getPrompt(req)
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
		"capabilities": map[string]any{
			"tools":     map[string]any{},
			"resources": map[string]any{},
			"prompts":   map[string]any{},
		},
		"serverInfo": map[string]any{"name": serverName, "version": serverVersion},
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
