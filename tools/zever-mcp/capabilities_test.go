package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResourcesList(t *testing.T) {
	t.Parallel()

	s := newServer()
	resp := s.handle(t.Context(), rpcRequest{JSONRPC: "2.0", ID: json.RawMessage("1"), Method: "resources/list"})

	if resp.Error != nil {
		t.Fatalf("unexpected error: %+v", resp.Error)
	}

	m, ok := resp.Result.(map[string]any)
	if !ok {
		t.Fatalf("result type = %T", resp.Result)
	}

	list, ok := m["resources"].([]map[string]any)
	if !ok || len(list) == 0 {
		t.Fatalf("resources = %v", m["resources"])
	}
}

func mustResultMap(t *testing.T, resp rpcResponse) map[string]any {
	t.Helper()

	if resp.Error != nil {
		t.Fatalf("unexpected error: %+v", resp.Error)
	}

	m, ok := resp.Result.(map[string]any)
	if !ok {
		t.Fatalf("result type = %T, want map", resp.Result)
	}

	return m
}

func TestResourcesRead(t *testing.T) {
	t.Parallel()

	s := newServer()

	raw, _ := json.Marshal(map[string]any{"uri": "zever://cli-reference"})
	resp := s.handle(t.Context(), rpcRequest{JSONRPC: "2.0", ID: json.RawMessage("1"), Method: "resources/read", Params: raw})

	m := mustResultMap(t, resp)
	contents, ok := m["contents"].([]map[string]any)
	if !ok || len(contents) == 0 {
		t.Fatalf("contents = %v", m["contents"])
	}

	if contents[0]["uri"] != "zever://cli-reference" {
		t.Fatalf("uri = %v", contents[0]["uri"])
	}
}

func TestResourcesReadUnknown(t *testing.T) {
	t.Parallel()

	s := newServer()

	raw, _ := json.Marshal(map[string]any{"uri": "zever://nope"})
	resp := s.handle(t.Context(), rpcRequest{JSONRPC: "2.0", ID: json.RawMessage("1"), Method: "resources/read", Params: raw})

	if resp.Error == nil {
		t.Fatal("expected error for unknown resource, got nil")
	}
}

func TestPromptsList(t *testing.T) {
	t.Parallel()

	s := newServer()
	resp := s.handle(t.Context(), rpcRequest{JSONRPC: "2.0", ID: json.RawMessage("1"), Method: "prompts/list"})

	m := mustResultMap(t, resp)
	list, ok := m["prompts"].([]map[string]any)
	if !ok {
		t.Fatalf("prompts type = %T", m["prompts"])
	}

	if len(list) != 4 {
		t.Fatalf("prompts = %d, want 4", len(list))
	}
}

func TestPromptsGet(t *testing.T) {
	t.Parallel()

	s := newServer()

	raw, _ := json.Marshal(map[string]any{
		"name":      "scaffold-feature",
		"arguments": map[string]string{"description": "a blog"},
	})
	resp := s.handle(t.Context(), rpcRequest{JSONRPC: "2.0", ID: json.RawMessage("1"), Method: "prompts/get", Params: raw})

	m := mustResultMap(t, resp)
	msgs, ok := m["messages"].([]promptMessage)
	if !ok || len(msgs) != 1 {
		t.Fatalf("messages = %v", m["messages"])
	}

	text, ok := msgs[0].Content.(map[string]any)["text"].(string)
	if !ok || !strings.Contains(text, "a blog") {
		t.Fatalf("message text = %v", msgs[0].Content)
	}
}

func TestPromptsGetMissingArg(t *testing.T) {
	t.Parallel()

	s := newServer()

	raw, _ := json.Marshal(map[string]any{"name": "scaffold-feature", "arguments": map[string]string{}})
	resp := s.handle(t.Context(), rpcRequest{JSONRPC: "2.0", ID: json.RawMessage("1"), Method: "prompts/get", Params: raw})

	if resp.Error == nil {
		t.Fatal("expected error for missing argument, got nil")
	}
}

func TestPromptsGetDebugDoctor(t *testing.T) {
	t.Parallel()

	s := newServer()

	raw, _ := json.Marshal(map[string]any{
		"name":      "debug-doctor",
		"arguments": map[string]string{"doctor_json": "auth: FAIL"},
	})
	resp := s.handle(t.Context(), rpcRequest{JSONRPC: "2.0", ID: json.RawMessage("1"), Method: "prompts/get", Params: raw})

	m := mustResultMap(t, resp)
	msgs, ok := m["messages"].([]promptMessage)
	if !ok || len(msgs) != 1 {
		t.Fatalf("messages = %v", m["messages"])
	}

	text, ok := msgs[0].Content.(map[string]any)["text"].(string)
	if !ok || !strings.Contains(text, "auth: FAIL") {
		t.Fatalf("message text = %v", msgs[0].Content)
	}
}

func TestPromptsGetFixDiags(t *testing.T) {
	t.Parallel()

	s := newServer()

	raw, _ := json.Marshal(map[string]any{
		"name":      "fix-diags",
		"arguments": map[string]string{"diags": "app.zen:3:13: bad"},
	})
	resp := s.handle(t.Context(), rpcRequest{JSONRPC: "2.0", ID: json.RawMessage("1"), Method: "prompts/get", Params: raw})

	m := mustResultMap(t, resp)
	msgs, ok := m["messages"].([]promptMessage)
	if !ok || len(msgs) != 1 {
		t.Fatalf("messages = %v", m["messages"])
	}
}

func TestResourcesReadDoctorGuide(t *testing.T) {
	t.Parallel()

	s := newServer()

	raw, _ := json.Marshal(map[string]any{"uri": "zever://doctor-guide"})
	resp := s.handle(t.Context(), rpcRequest{JSONRPC: "2.0", ID: json.RawMessage("1"), Method: "resources/read", Params: raw})

	m := mustResultMap(t, resp)
	contents, ok := m["contents"].([]map[string]any)
	if !ok || len(contents) == 0 {
		t.Fatalf("contents = %v", m["contents"])
	}

	if contents[0]["uri"] != "zever://doctor-guide" {
		t.Fatalf("uri = %v", contents[0]["uri"])
	}
}

func TestPromptsGetUnknown(t *testing.T) {
	t.Parallel()

	s := newServer()

	raw, _ := json.Marshal(map[string]any{"name": "nope"})
	resp := s.handle(t.Context(), rpcRequest{JSONRPC: "2.0", ID: json.RawMessage("1"), Method: "prompts/get", Params: raw})

	if resp.Error == nil {
		t.Fatal("expected error for unknown prompt, got nil")
	}
}

// stubZever installs a fake `zever` binary on PATH that prints canned
// output. Output passes through an env var, never shell interpolation.
func stubZever(t *testing.T, output string) {
	t.Helper()

	dir := t.TempDir()
	path := filepath.Join(dir, "zever")

	script := "#!/bin/sh\nprintf '%s' \"$STUB_OUTPUT\"\n"
	if err := os.WriteFile(path, []byte(script), 0o600); err != nil {
		t.Fatalf("write stub: %v", err)
	}

	if err := os.Chmod(path, 0o755); err != nil {
		t.Fatalf("chmod stub: %v", err)
	}

	t.Setenv("STUB_OUTPUT", output)
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func TestDoctorTool(t *testing.T) {
	stubZever(t, `{"ok":true}`)

	text, err := doctorTool().Handler(t.Context(), map[string]any{})
	if err != nil {
		t.Fatalf("doctor handler error = %v", err)
	}

	if !strings.Contains(text, `"ok":true`) {
		t.Fatalf("output = %q", text)
	}
}

func TestGenerateToolPlan(t *testing.T) {
	stubZever(t, "planned")

	text, err := newServer().tools["zever_generate"].Handler(t.Context(), map[string]any{
		"subcommand": "entity",
		"args":       []any{"shop", "Order"},
	})
	if err != nil {
		t.Fatalf("generate handler error = %v", err)
	}

	if !strings.Contains(text, "planned") || !strings.Contains(text, "confirm:true") {
		t.Fatalf("output = %q", text)
	}
}

func TestGenerateToolUnknownSubcommand(t *testing.T) {
	_, err := newServer().tools["zever_generate"].Handler(t.Context(), map[string]any{
		"subcommand": "rm",
		"args":       []any{},
	})
	if err == nil {
		t.Fatal("expected error for unknown subcommand, got nil")
	}
}

func TestGenerateToolNoBinary(t *testing.T) {
	t.Setenv("PATH", t.TempDir())

	_, err := newServer().tools["zever_generate"].Handler(t.Context(), map[string]any{
		"subcommand": "entity",
		"args":       []any{},
	})
	if err == nil {
		t.Fatal("expected error without zever on PATH, got nil")
	}
}

func TestElicitUnavailable(t *testing.T) {
	s := newServer()

	_, err := s.elicit(t.Context(), "hi?", nil)
	if err == nil {
		t.Fatal("expected elicitation-unavailable error, got nil")
	}
}

func TestElicitCanceled(t *testing.T) {
	s := newServer()
	s.sender = func(any) error { return nil }

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	_, err := s.elicit(ctx, "hi?", nil)
	if err == nil {
		t.Fatal("expected context error, got nil")
	}
}
