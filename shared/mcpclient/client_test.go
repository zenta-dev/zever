package mcpclient

import (
	"encoding/json"
	"errors"
	"net"
	"strings"
	"testing"
)

// scriptedServer answers canned responses keyed by method name.
type scriptedServer struct {
	t       *testing.T
	conn    net.Conn
	methods map[string]func(id int64, raw json.RawMessage)
}

func (s *scriptedServer) respond(id int64, result any) {
	s.t.Helper()

	raw, err := json.Marshal(result)
	if err != nil {
		s.t.Fatalf("marshal result: %v", err)
	}

	out, err := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": id, "result": json.RawMessage(raw)})
	if err != nil {
		s.t.Fatalf("marshal response: %v", err)
	}

	out = append(out, '\n')

	if _, err := s.conn.Write(out); err != nil {
		s.t.Logf("server write: %v", err)
	}
}

func (s *scriptedServer) fail(id int64, code int, msg string) {
	s.t.Helper()

	out, err := json.Marshal(map[string]any{
		"jsonrpc": "2.0", "id": id,
		"error": map[string]any{"code": code, "message": msg},
	})
	if err != nil {
		s.t.Fatalf("marshal error: %v", err)
	}

	out = append(out, '\n')

	if _, err := s.conn.Write(out); err != nil {
		s.t.Logf("server write: %v", err)
	}
}

func (s *scriptedServer) serve() {
	defer s.conn.Close()

	dec := json.NewDecoder(s.conn)

	for {
		var req struct {
			ID     int64           `json:"id"`
			Method string          `json:"method"`
			Params json.RawMessage `json:"params"`
		}

		if err := dec.Decode(&req); err != nil {
			return
		}

		h, ok := s.methods[req.Method]
		if !ok {
			s.fail(req.ID, -32601, "method not found")
			continue
		}

		h(req.ID, req.Params)
	}
}

func startScriptedServer(t *testing.T) (*scriptedServer, *Client) {
	t.Helper()

	server, client := net.Pipe()

	s := &scriptedServer{t: t, conn: server, methods: map[string]func(id int64, raw json.RawMessage){}}

	go s.serve()

	t.Cleanup(func() { _ = client.Close() })

	return s, New(client, client)
}

func TestClient_handshakeListCall(t *testing.T) {
	s, c := startScriptedServer(t)

	s.methods["initialize"] = func(id int64, _ json.RawMessage) {
		s.respond(id, map[string]any{"protocolVersion": "2024-11-05"})
	}
	s.methods["tools/list"] = func(id int64, _ json.RawMessage) {
		s.respond(id, map[string]any{"tools": []any{
			map[string]any{"name": "echo", "description": "Echo back.", "inputSchema": map[string]any{"type": "object"}},
		}})
	}
	s.methods["tools/call"] = func(id int64, raw json.RawMessage) {
		var p struct {
			Name      string         `json:"name"`
			Arguments map[string]any `json:"arguments"`
		}
		if err := json.Unmarshal(raw, &p); err != nil {
			s.fail(id, -32602, "bad params")
			return
		}

		s.respond(id, map[string]any{"content": []any{
			map[string]any{"type": "text", "text": "got:" + p.Name},
		}})
	}

	version, err := c.Initialize(t.Context())
	if err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}

	if version != "2024-11-05" {
		t.Fatalf("version = %q", version)
	}

	tools, err := c.ListTools(t.Context())
	if err != nil {
		t.Fatalf("ListTools() error = %v", err)
	}

	if len(tools) != 1 || tools[0].Name != "echo" {
		t.Fatalf("tools = %+v", tools)
	}

	res, err := c.CallTool(t.Context(), "echo", map[string]any{"x": "y"})
	if err != nil {
		t.Fatalf("CallTool() error = %v", err)
	}

	if res.IsError || len(res.Content) != 1 || !strings.HasPrefix(res.Content[0].Text, "got:echo") {
		t.Fatalf("result = %+v", res)
	}
}

func TestClient_rpcError(t *testing.T) {
	s, c := startScriptedServer(t)

	s.methods["tools/list"] = func(id int64, _ json.RawMessage) {
		s.fail(id, -32602, "bad call")
	}

	_, err := c.ListTools(t.Context())
	if err == nil {
		t.Fatal("expected RPC error, got nil")
	}

	var rpcErr RPCError
	if !errors.As(err, &rpcErr) {
		t.Fatalf("error type = %T, want RPCError", err)
	}

	if !errors.Is(err, ErrRPC) {
		t.Fatalf("error = %v, want ErrRPC", err)
	}
}

func TestDialStdio_missingBinary(t *testing.T) {
	t.Parallel()

	if _, _, err := DialStdio(t.Context(), "/nonexistent-zever-test-binary"); err == nil {
		t.Fatal("expected error for missing binary, got nil")
	}
}

func TestClient_unknownMethod(t *testing.T) {
	_, c := startScriptedServer(t)

	// No handlers registered: server answers method-not-found.
	_, err := c.ListTools(t.Context())
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if !errors.Is(err, ErrRPC) {
		t.Fatalf("error = %v, want ErrRPC", err)
	}
}
