package main

import (
	"encoding/json"
	"net"
	"strings"
	"testing"
)

// TestGenerateApplyRoundTrip drives a full confirm:true generate through
// run() against a scripted client: the server must elicit, the client
// accepts, the stub zever applies, and the tool result carries its output.
func TestGenerateApplyRoundTrip(t *testing.T) {
	stubZever(t, "applied-ok")

	server, client := net.Pipe()
	defer client.Close()

	done := make(chan int, 1)

	go func() {
		done <- run(t.Context(), server, server, tDiscard{})
	}()

	dec := json.NewDecoder(client)

	call, _ := json.Marshal(map[string]any{
		"subcommand": "entity",
		"args":       []any{"shop", "Order"},
		"confirm":    true,
	})
	req, _ := json.Marshal(map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "tools/call",
		"params": map[string]any{"name": "zever_generate", "arguments": json.RawMessage(call)},
	})

	if _, err := client.Write(append(req, '\n')); err != nil {
		t.Fatalf("write call: %v", err)
	}

	// Expect the elicitation request.
	var elicited struct {
		ID     int64  `json:"id"`
		Method string `json:"method"`
	}
	if err := dec.Decode(&elicited); err != nil {
		t.Fatalf("read elicitation: %v", err)
	}

	if elicited.Method != "elicitation/create" {
		t.Fatalf("method = %q, want elicitation/create", elicited.Method)
	}

	// Accept it.
	answer, _ := json.Marshal(map[string]any{
		"jsonrpc": "2.0", "id": elicited.ID,
		"result": map[string]any{"action": "accept"},
	})
	if _, err := client.Write(append(answer, '\n')); err != nil {
		t.Fatalf("write answer: %v", err)
	}

	// Expect the tool result carrying the stub output.
	var result struct {
		Result struct {
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
			IsError bool `json:"isError"`
		} `json:"result"`
	}
	if err := dec.Decode(&result); err != nil {
		t.Fatalf("read result: %v", err)
	}

	if result.Result.IsError {
		t.Fatalf("tool error: %+v", result.Result)
	}

	if !strings.Contains(result.Result.Content[0].Text, "applied-ok") {
		t.Fatalf("text = %q", result.Result.Content[0].Text)
	}

	_ = client.Close()

	if code := <-done; code != 0 {
		t.Fatalf("run() = %d, want 0", code)
	}
}

// tDiscard drops run() diagnostics.
type tDiscard struct{}

func (tDiscard) Write(p []byte) (int, error) { return len(p), nil }
