package mcpclient

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"
)

// BenchmarkNew measures Client construction.
func BenchmarkNew(b *testing.B) {
	var buf bytes.Buffer
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		c := New(&buf, &buf)
		_ = c
	}
}

// BenchmarkEncodeRequest measures the JSON-RPC request encoding hot path.
func BenchmarkEncodeRequest(b *testing.B) {
	var buf bytes.Buffer
	c := New(&buf, &buf)

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		req := request{JSONRPC: "2.0", ID: 1, Method: "test", Params: map[string]any{"k": "v"}}
		_ = c.enc.Encode(req)
		buf.Reset()
	}
}

// BenchmarkDecodeResponse measures the JSON-RPC response decoding hot path.
func BenchmarkDecodeResponse(b *testing.B) {
	// Pre-encode a valid response
	resp := `{"jsonrpc":"2.0","id":1,"result":{"protocolVersion":"2024-11-05"}}`
	var buf bytes.Buffer
	buf.WriteString(resp)

	c := New(&buf, &buf)

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		buf.Reset()
		buf.WriteString(resp)
		var msg inbound
		_ = c.dec.Decode(&msg)
	}
}

// BenchmarkDispatchRequest measures the server-request dispatch hot path
// (sampling/elicitation) without network I/O.
func BenchmarkDispatchRequest(b *testing.B) {
	var buf bytes.Buffer
	c := New(&buf, &buf)
	c.onSampling = func(ctx context.Context, req SampleRequest) (SampleResult, error) {
		return SampleResult{Content: map[string]any{"type": "text", "text": "ok"}}, nil
	}

	reqBytes := []byte(`{"jsonrpc":"2.0","id":1,"method":"sampling/create","params":{"messages":[]}}`)

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		buf.Reset()
		buf.Write(reqBytes)
		var msg inbound
		_ = c.dec.Decode(&msg)
		_ = c.dispatchRequest(b.Context(), msg.ID, msg.Method, msg.Params)
	}
}

// BenchmarkDialStdioRetry_Logic measures the retry backoff calculation
// without actually spawning processes.
func BenchmarkDialStdioRetry_Logic(b *testing.B) {
	opts := DialOptions{
		Attempts:         3,
		InitialBackoff:   200 * 1000000, // 200ms in ns
		MaxBackoff:       5 * 1000000000, // 5s in ns
	}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		attempts := opts.Attempts
		if attempts <= 0 {
			attempts = DefaultDialAttempts
		}

		backoff := opts.InitialBackoff
		if backoff <= 0 {
			backoff = DefaultDialInitialBackoff
		}

		maxBackoff := opts.MaxBackoff
		if maxBackoff <= 0 {
			maxBackoff = DefaultDialMaxBackoff
		}

		for attempt := 1; attempt <= attempts; attempt++ {
			backoff *= 2
			if backoff > maxBackoff {
				backoff = maxBackoff
			}
		}
	}
}

// BenchmarkCallTool_EncodeOnly measures the request encoding portion of
// CallTool without waiting for a response.
func BenchmarkCallTool_EncodeOnly(b *testing.B) {
	var buf bytes.Buffer
	c := New(&buf, &buf)
	args := map[string]any{"key": "value"}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		buf.Reset()
		_ = c.enc.Encode(request{JSONRPC: "2.0", ID: 1, Method: "tools/call", Params: map[string]any{"name": "test-tool", "arguments": args}})
	}
}

// BenchmarkListTools_EncodeOnly measures the request encoding portion of
// ListTools without waiting for a response.
func BenchmarkListTools_EncodeOnly(b *testing.B) {
	var buf bytes.Buffer
	c := New(&buf, &buf)

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		buf.Reset()
		_ = c.enc.Encode(request{JSONRPC: "2.0", ID: 1, Method: "tools/list", Params: nil})
	}
}

// BenchmarkInitialize_EncodeOnly measures the request encoding portion of
// Initialize without waiting for a response.
func BenchmarkInitialize_EncodeOnly(b *testing.B) {
	var buf bytes.Buffer
	c := New(&buf, &buf)

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		buf.Reset()
		_ = c.enc.Encode(request{JSONRPC: "2.0", ID: 1, Method: "initialize", Params: map[string]any{"protocolVersion": "2024-11-05", "capabilities": map[string]any{}, "clientInfo": map[string]any{"name": "test", "version": "1.0"}}})
	}
}

// BenchmarkJSONMarshal measures JSON marshaling of tool arguments.
func BenchmarkJSONMarshal(b *testing.B) {
	args := map[string]any{"key": "value", "nested": map[string]any{"a": 1, "b": "two"}}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		_, _ = json.Marshal(args)
	}
}