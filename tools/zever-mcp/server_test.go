package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

const testSchema = `entity Post {
  id: uuid @primary
  title: string
}

service PostService {
  rpc GetPost(id: uuid) -> Post {
    http: GET "/v1/posts/{id}"
    auth: required
  }
}`

func testFiles() map[string]string {
	return map[string]string{"blog/blog.zen": testSchema}
}

func callToolReq(t *testing.T, s *Server, name string, args map[string]any) rpcResponse {
	t.Helper()

	raw, err := json.Marshal(callParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("marshal params: %v", err)
	}

	return s.handle(t.Context(), rpcRequest{
		JSONRPC: "2.0",
		ID:      json.RawMessage("1"),
		Method:  "tools/call",
		Params:  raw,
	})
}

func resultText(t *testing.T, resp rpcResponse) string {
	t.Helper()

	res := toolResult(t, resp)
	if len(res.Content) == 0 {
		t.Fatal("no content blocks")
	}
	return res.Content[0].Text
}

func toolResult(t *testing.T, resp rpcResponse) callToolResult {
	t.Helper()

	res, ok := resp.Result.(callToolResult)
	if !ok {
		t.Fatalf("result type = %T, want callToolResult", resp.Result)
	}
	return res
}

func TestInitialize(t *testing.T) {
	t.Parallel()

	s := newServer()
	resp := s.handle(t.Context(), rpcRequest{JSONRPC: "2.0", ID: json.RawMessage("1"), Method: "initialize", Params: json.RawMessage(`{"protocolVersion":"2025-06-18"}`)})

	m, ok := resp.Result.(map[string]any)
	if !ok {
		t.Fatalf("result type = %T", resp.Result)
	}
	if m["protocolVersion"] != "2025-06-18" {
		t.Errorf("protocolVersion = %v, want echo 2025-06-18", m["protocolVersion"])
	}
}

func TestToolsList(t *testing.T) {
	t.Parallel()

	s := newServer()
	resp := s.handle(t.Context(), rpcRequest{JSONRPC: "2.0", ID: json.RawMessage("1"), Method: "tools/list"})

	m, ok := resp.Result.(map[string]any)
	if !ok {
		t.Fatalf("result type = %T", resp.Result)
	}
	tools, ok := m["tools"].([]map[string]any)
	if !ok {
		t.Fatalf("tools type = %T", m["tools"])
	}
	if len(tools) != 5 {
		t.Fatalf("tools = %d, want 5", len(tools))
	}
}

func TestUnknownMethod(t *testing.T) {
	t.Parallel()

	s := newServer()
	resp := s.handle(t.Context(), rpcRequest{JSONRPC: "2.0", ID: json.RawMessage("1"), Method: "nope"})
	if resp.Error == nil || resp.Error.Code != codeMethodNotFound {
		t.Fatalf("error = %+v, want method not found", resp.Error)
	}
}

func TestUnknownTool(t *testing.T) {
	t.Parallel()

	s := newServer()
	resp := callToolReq(t, s, "nope", nil)
	if resp.Error == nil || resp.Error.Code != codeInvalidParams {
		t.Fatalf("error = %+v, want invalid params", resp.Error)
	}
}

func TestCompileTool(t *testing.T) {
	t.Parallel()

	s := newServer()
	resp := callToolReq(t, s, "zever_compile", map[string]any{"files": testFiles()})
	if resp.Error != nil {
		t.Fatalf("unexpected rpc error: %+v", resp.Error)
	}

	text := resultText(t, resp)
	if !strings.Contains(text, "openapi") {
		t.Fatalf("compile output missing openapi: %s", text)
	}
}

func TestCompileTool_badSchemaIsToolError(t *testing.T) {
	t.Parallel()

	s := newServer()
	resp := callToolReq(t, s, "zever_compile", map[string]any{
		"files": map[string]string{"blog/blog.zen": "entity {"},
	})
	if resp.Error != nil {
		t.Fatalf("bad schema should be a tool error, not rpc error: %+v", resp.Error)
	}

	res := toolResult(t, resp)
	if !res.IsError {
		t.Fatal("expected isError for a schema with diagnostics")
	}
}

func TestCompileTool_missingSource(t *testing.T) {
	t.Parallel()

	s := newServer()
	resp := callToolReq(t, s, "zever_compile", map[string]any{})
	res := toolResult(t, resp)
	if !res.IsError {
		t.Fatal("expected isError when neither dir nor files is given")
	}
}

func TestSchemaTool(t *testing.T) {
	t.Parallel()

	s := newServer()
	resp := callToolReq(t, s, "zever_schema", map[string]any{"files": testFiles()})
	if resp.Error != nil {
		t.Fatalf("unexpected rpc error: %+v", resp.Error)
	}

	var sum schemaSummary
	if err := json.Unmarshal([]byte(resultText(t, resp)), &sum); err != nil {
		t.Fatalf("summary is not JSON: %v", err)
	}
	if len(sum.Modules) != 1 {
		t.Fatalf("modules = %d, want 1", len(sum.Modules))
	}
	if len(sum.Modules[0].Entities) != 1 || sum.Modules[0].Entities[0] != "Post" {
		t.Fatalf("entities = %v, want [Post]", sum.Modules[0].Entities)
	}
}

func TestExplainTool(t *testing.T) {
	t.Parallel()

	s := newServer()

	// Discover the module name from the schema summary, then explain an entity.
	sumResp := callToolReq(t, s, "zever_schema", map[string]any{"files": testFiles()})
	var sum schemaSummary
	if err := json.Unmarshal([]byte(resultText(t, sumResp)), &sum); err != nil {
		t.Fatalf("summary is not JSON: %v", err)
	}
	mod := sum.Modules[0].Name

	resp := callToolReq(t, s, "zever_explain", map[string]any{"files": testFiles(), "path": mod + ".Post"})
	if resp.Error != nil {
		t.Fatalf("unexpected rpc error: %+v", resp.Error)
	}

	text := resultText(t, resp)
	if !strings.Contains(text, "entity Post") || !strings.Contains(text, "title") {
		t.Fatalf("explain output = %q, want entity Post with title", text)
	}
}

func TestExplainTool_notFound(t *testing.T) {
	t.Parallel()

	s := newServer()
	resp := callToolReq(t, s, "zever_explain", map[string]any{"files": testFiles(), "path": "nope.Thing"})
	res := toolResult(t, resp)
	if !res.IsError {
		t.Fatal("expected isError for an unknown module")
	}
}

func TestRun_stdioLoop(t *testing.T) {
	t.Parallel()

	input := strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`,
		`{"jsonrpc":"2.0","method":"notifications/initialized"}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/list"}`,
	}, "\n") + "\n"

	var out, errb bytes.Buffer
	if code := run(t.Context(), strings.NewReader(input), &out, &errb); code != 0 {
		t.Fatalf("run() = %d, stderr = %s", code, errb.String())
	}

	dec := json.NewDecoder(&out)
	count := 0
	for dec.More() {
		var resp rpcResponse
		if err := dec.Decode(&resp); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		count++
	}
	if count != 2 {
		t.Fatalf("responses = %d, want 2 (notification must not respond)", count)
	}
}
