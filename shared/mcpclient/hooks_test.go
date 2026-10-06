package mcpclient

import (
	"context"
	"encoding/json"
	"net"
	"testing"
	"time"
)

// fakeIO bundles a scripted server's ends.
type fakeIO struct {
	dec *json.Decoder
	enc *json.Encoder
}

func (f fakeIO) read(t *testing.T, wantMethod string) {
	t.Helper()

	var req struct {
		Method string `json:"method"`
	}
	if err := f.dec.Decode(&req); err != nil {
		t.Errorf("read %s: %v", wantMethod, err)
		return
	}
	if req.Method != wantMethod {
		t.Errorf("method = %q, want %q", req.Method, wantMethod)
	}
}

func (f fakeIO) write(t *testing.T, msg map[string]any) {
	t.Helper()

	if err := f.enc.Encode(msg); err != nil {
		t.Errorf("write: %v", err)
	}
}

func resultMsg(id int64, result map[string]any) map[string]any {
	return map[string]any{"jsonrpc": "2.0", "id": id, "result": result}
}

// driveConversation runs a scripted server that answers initialize, then
// sends one mid-call server request during a tools/list call, verifies the
// client's answer, and completes the list call.
func driveConversation(t *testing.T, midRequest map[string]any, verify func(*testing.T, map[string]any), opts ...Option) {
	t.Helper()

	server, client := net.Pipe()
	defer client.Close()

	go func() {
		defer server.Close()

		f := fakeIO{dec: json.NewDecoder(server), enc: json.NewEncoder(server)}

		f.read(t, "initialize")
		f.write(t, resultMsg(1, map[string]any{"protocolVersion": "2024-11-05"}))
		f.read(t, "tools/list")
		f.write(t, midRequest)

		var answer struct {
			ID     int64          `json:"id"`
			Result map[string]any `json:"result"`
		}
		if err := f.dec.Decode(&answer); err != nil {
			t.Errorf("read answer: %v", err)
			return
		}
		verify(t, answer.Result)

		f.write(t, resultMsg(2, map[string]any{"tools": []any{}}))
	}()

	c := New(client, client, opts...)

	if _, err := c.Initialize(t.Context()); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}

	if _, err := c.ListTools(t.Context()); err != nil {
		t.Fatalf("ListTools() error = %v", err)
	}
}

func TestClientSamplingHook(t *testing.T) {
	var sampled bool

	driveConversation(t, map[string]any{
		"jsonrpc": "2.0", "id": 99, "method": "sampling/create",
		"params": map[string]any{"messages": []any{}},
	}, func(t *testing.T, result map[string]any) {
		t.Helper()

		if result["text"] != "hi" {
			t.Errorf("hook answer = %v", result)
		}
	}, WithSampler(func(context.Context, SampleRequest) (SampleResult, error) {
		sampled = true

		return SampleResult{Content: map[string]any{"type": "text", "text": "hi"}}, nil
	}))

	if !sampled {
		t.Fatal("sampler hook never ran")
	}
}

func TestClientElicitAutoDecline(t *testing.T) {
	driveConversation(t, map[string]any{
		"jsonrpc": "2.0", "id": 7, "method": "elicitation/create",
		"params": map[string]any{"message": "ok?"},
	}, func(t *testing.T, result map[string]any) {
		t.Helper()

		if result["action"] != "decline" {
			t.Errorf("action = %v, want decline", result["action"])
		}
	})
}

func TestClientElicitPolicy(t *testing.T) {
	var saw string

	driveConversation(t, map[string]any{
		"jsonrpc": "2.0", "id": 7, "method": "elicitation/create",
		"params": map[string]any{"message": "ok?"},
	}, func(t *testing.T, result map[string]any) {
		t.Helper()

		if result["action"] != "accept" {
			t.Errorf("action = %v, want accept", result)
		}
	}, WithElicitPolicy(func(_ context.Context, req ElicitRequest) (ElicitAction, map[string]any, error) {
		saw = req.Message
		return ElicitAccept, nil, nil
	}))

	if saw != "ok?" {
		t.Fatalf("policy saw %q", saw)
	}
}

func TestDialStdioRetry_missingBinary(t *testing.T) {
	t.Parallel()

	_, _, err := DialStdioRetry(t.Context(), "/nonexistent-zever-test-binary", DialOptions{
		Attempts:       2,
		InitialBackoff: time.Millisecond,
		MaxBackoff:     time.Millisecond,
	})
	if err == nil {
		t.Fatal("expected error for missing binary, got nil")
	}
}

func TestDialDefaults(t *testing.T) {
	t.Parallel()

	if DefaultDialAttempts <= 0 || DefaultDialInitialBackoff <= 0 || DefaultDialMaxBackoff <= 0 {
		t.Fatal("dial defaults must be positive")
	}
}
