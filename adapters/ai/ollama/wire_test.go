package ollama

import (
	"testing"

	"github.com/zenta-dev/zever/shared/codec"
)

// The tests in this file pin the Ollama wire format: the exact JSON key
// names encoding/json emits for the adapter's request and response structs.
// Key names are part of the Ollama API contract — a stripped or renamed tag
// breaks every Ollama server exchange while still compiling and passing
// unit tests that only check round-trips through the same struct. Each
// golden string is compared byte-for-byte against the marshaled struct, and
// the golden string is also decoded back through the package codec to pin
// the read direction.

func TestChatRequestWireKeys(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		req    chatRequest
		golden string
	}{
		{
			name: "minimal",
			req: chatRequest{
				Model:    "llama3.2",
				Messages: []chatMessage{{Role: "user", Content: "hi"}},
				Stream:   false,
			},
			golden: `{"model":"llama3.2","messages":[{"role":"user","content":"hi"}],"stream":false}`,
		},
		{
			name: "options and tools",
			req: chatRequest{
				Model:    "llama3.2",
				Messages: []chatMessage{{Role: "system", Content: "be nice"}, {Role: "user", Content: "hi"}},
				Stream:   true,
				Options:  &chatOptions{Temperature: float32ptr(0.5), NumPredict: 10, TopP: float32ptr(0.9)},
				Tools: []ollamaTool{{
					Type: "function",
					Function: ollamaFunction{
						Name:        "get_time",
						Description: "current time",
						Parameters:  map[string]any{"type": "object"},
					},
				}},
			},
			golden: `{"model":"llama3.2","messages":[{"role":"system","content":"be nice"},{"role":"user","content":"hi"}],"stream":true,"options":{"temperature":0.5,"num_predict":10,"top_p":0.9},"tools":[{"type":"function","function":{"name":"get_time","description":"current time","parameters":{"type":"object"}}}]}`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			body, err := encode(tc.req)
			if err != nil {
				t.Fatalf("encode: %v", err)
			}

			if string(body) != tc.golden {
				t.Errorf("marshal chatRequest:\n got %s\nwant %s", body, tc.golden)
			}
		})
	}
}

func TestChatResponseWireKeys(t *testing.T) {
	t.Parallel()

	golden := `{"message":{"content":"hello"},"eval_count":5,"prompt_eval_count":3}`

	resp, err := chatResponseCodec.Decode([]byte(golden))
	if err != nil {
		t.Fatalf("decode golden chatResponse: %v", err)
	}

	if resp.Message.Content != "hello" {
		t.Errorf("Message.Content = %q, want %q", resp.Message.Content, "hello")
	}

	if resp.EvalCount != 5 || resp.PromptEvalCount != 3 {
		t.Errorf("counts = %d/%d, want 5/3", resp.EvalCount, resp.PromptEvalCount)
	}

	// Re-encoding the decoded value must reproduce the golden bytes exactly.
	out, err := codec.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal decoded chatResponse: %v", err)
	}

	if string(out) != golden {
		t.Errorf("re-marshal chatResponse:\n got %s\nwant %s", out, golden)
	}
}

func TestChatResponseToolCallsWireKeys(t *testing.T) {
	t.Parallel()

	golden := `{"message":{"content":"","tool_calls":[{"function":{"name":"get_time","arguments":{"unix":1}}}]},"eval_count":1,"prompt_eval_count":2}`

	resp, err := chatResponseCodec.Decode([]byte(golden))
	if err != nil {
		t.Fatalf("decode golden chatResponse with tool calls: %v", err)
	}

	if len(resp.Message.ToolCalls) != 1 {
		t.Fatalf("len(ToolCalls) = %d, want 1", len(resp.Message.ToolCalls))
	}

	if resp.Message.ToolCalls[0].Function.Name != "get_time" {
		t.Errorf("tool call name = %q, want %q", resp.Message.ToolCalls[0].Function.Name, "get_time")
	}

	out, err := codec.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal decoded chatResponse: %v", err)
	}

	if string(out) != golden {
		t.Errorf("re-marshal chatResponse with tool calls:\n got %s\nwant %s", out, golden)
	}
}

func TestStreamResponseWireKeys(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		resp   streamResponse
		golden string
	}{
		{
			name:   "delta chunk",
			resp:   streamResponse{Message: streamMessageContent("partial")},
			golden: `{"message":{"content":"partial"},"done":false,"eval_count":0,"prompt_eval_count":0}`,
		},
		{
			name:   "done with usage",
			resp:   streamResponse{Message: streamMessageContent(""), Done: true, EvalCount: 7, PromptEvalCount: 4},
			golden: `{"message":{"content":""},"done":true,"eval_count":7,"prompt_eval_count":4}`,
		},
		{
			name:   "error line",
			resp:   streamResponse{Error: "boom"},
			golden: `{"message":{"content":""},"done":false,"eval_count":0,"prompt_eval_count":0,"error":"boom"}`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			out, err := codec.Marshal(tc.resp)
			if err != nil {
				t.Fatalf("marshal streamResponse: %v", err)
			}

			if string(out) != tc.golden {
				t.Errorf("marshal streamResponse:\n got %s\nwant %s", out, tc.golden)
			}

			if _, err := streamResponseCodec.Decode(out); err != nil {
				t.Fatalf("decode marshaled streamResponse: %v", err)
			}
		})
	}
}

func TestEmbedWireKeys(t *testing.T) {
	t.Parallel()

	reqBody, err := encode(embedRequest{Model: "nomic-embed", Input: []string{"a", "b"}})
	if err != nil {
		t.Fatalf("encode embedRequest: %v", err)
	}

	if got, want := string(reqBody), `{"model":"nomic-embed","input":["a","b"]}`; got != want {
		t.Errorf("embedRequest:\n got %s\nwant %s", got, want)
	}

	golden := `{"embeddings":[[0.1,0.2]]}`
	resp, err := embedResponseCodec.Decode([]byte(golden))
	if err != nil {
		t.Fatalf("decode golden embedResponse: %v", err)
	}

	out, err := codec.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal decoded embedResponse: %v", err)
	}

	if string(out) != golden {
		t.Errorf("re-marshal embedResponse:\n got %s\nwant %s", out, golden)
	}
}

// streamMessageContent builds the anonymous Message struct carried by
// streamResponse (and chatResponse), keeping the golden literals above
// readable.
func streamMessageContent(content string) struct {
	Content   string           `json:"content"`
	ToolCalls []ollamaToolCall `json:"tool_calls,omitempty"`
} {
	return struct {
		Content   string           `json:"content"`
		ToolCalls []ollamaToolCall `json:"tool_calls,omitempty"`
	}{Content: content}
}

// float32ptr returns a pointer to v for chatOptions fields.
func float32ptr(v float32) *float32 {
	return &v
}
