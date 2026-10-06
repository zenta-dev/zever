package agent

import (
	"encoding/json"
	"net"
	"strings"
	"testing"

	"github.com/zenta-dev/zever/core/ai"
	"github.com/zenta-dev/zever/shared/mcpclient"
)

// fakeMCPServer answers tools/list and tools/call with canned payloads.
type fakeMCPServer struct {
	t    *testing.T
	conn net.Conn
	fail bool
}

func (s *fakeMCPServer) serve() {
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

		var result any

		switch req.Method {
		case "initialize":
			result = map[string]any{"protocolVersion": "2024-11-05"}
		case "tools/list":
			result = map[string]any{"tools": []any{
				map[string]any{
					"name":        "echo",
					"description": "Echo.",
					"inputSchema": map[string]any{"type": "object"},
				},
			}}
		case "tools/call":
			if s.fail {
				result = nil
			} else {
				result = map[string]any{"content": []any{
					map[string]any{"type": "text", "text": "echo-ok"},
				}}
			}
		}

		out := map[string]any{"jsonrpc": "2.0", "id": req.ID}

		if result == nil {
			out["error"] = map[string]any{"code": -32603, "message": "boom"}
		} else {
			raw, err := json.Marshal(result)
			if err != nil {
				s.t.Errorf("marshal: %v", err)

				return
			}

			out["result"] = json.RawMessage(raw)
		}

		raw, err := json.Marshal(out)
		if err != nil {
			s.t.Errorf("marshal: %v", err)

			return
		}

		if _, err := s.conn.Write(append(raw, '\n')); err != nil {
			return
		}
	}
}

func dialFakeMCP(t *testing.T, fail bool) *mcpclient.Client {
	t.Helper()

	server, client := net.Pipe()

	s := &fakeMCPServer{t: t, conn: server, fail: fail}

	go s.serve()

	t.Cleanup(func() { _ = client.Close() })

	return mcpclient.New(client, client)
}

func TestToolsFromClient(t *testing.T) {
	c := dialFakeMCP(t, false)

	tools, err := ToolsFromClient(t.Context(), c)
	if err != nil {
		t.Fatalf("ToolsFromClient() error = %v", err)
	}

	if len(tools) != 1 || tools[0].Name != "echo" {
		t.Fatalf("tools = %+v", tools)
	}

	out, err := tools[0].Handler(t.Context(), nil)
	if err != nil {
		t.Fatalf("handler error = %v", err)
	}

	if out != "echo-ok" {
		t.Fatalf("output = %q", out)
	}
}

func TestToolsFromClient_failure(t *testing.T) {
	c := dialFakeMCP(t, true)

	tools, err := ToolsFromClient(t.Context(), c)
	if err != nil {
		t.Fatalf("ToolsFromClient() error = %v", err)
	}

	if _, err := tools[0].Handler(t.Context(), nil); err == nil {
		t.Fatal("expected handler error, got nil")
	} else if !strings.Contains(err.Error(), "boom") {
		t.Fatalf("error = %v, want boom", err)
	}
}

func TestToolsFromClient_inLoop(t *testing.T) {
	c := dialFakeMCP(t, false)

	tools, err := ToolsFromClient(t.Context(), c)
	if err != nil {
		t.Fatalf("ToolsFromClient() error = %v", err)
	}

	client := &scriptedAI{responses: []ai.Generation{
		{ToolCalls: []ai.ToolCall{{ID: "1", Name: "echo"}}},
		{Content: "done"},
	}}

	l, err := New(client, Options{Model: "m", Tools: tools})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	res, err := l.Run(t.Context(), nil)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if res.Content != "done" {
		t.Fatalf("content = %q", res.Content)
	}
}

func TestToolsFromClient_listError(t *testing.T) {
	server, client := net.Pipe()

	// No server goroutine: closing the peer makes reads fail fast.
	_ = server.Close()
	defer client.Close()

	c := mcpclient.New(client, client)

	if _, err := ToolsFromClient(t.Context(), c); err == nil {
		t.Fatal("expected list error with closed server, got nil")
	}
}
