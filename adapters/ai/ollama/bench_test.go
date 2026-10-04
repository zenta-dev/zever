package ollama

import (
	"testing"

	"github.com/zenta-dev/zever/core/ai"
)

// benchMessages returns a conversation with roles that need normalizing and
// a tool turn.
func benchMessages() []ai.Message {
	return []ai.Message{
		{Role: ai.RoleSystem, Content: "be helpful"},
		{Role: ai.RoleUser, Content: "what is the weather?"},
		{Role: ai.RoleAssistant, Content: "calling tool"},
		{Role: ai.RoleTool, Content: `{"temp":20}`},
		{Role: ai.RoleUser, Content: "thanks"},
	}
}

// BenchmarkNormalizeMessages measures the role-normalization pass performed on
// every Generate/Stream call.
func BenchmarkNormalizeMessages(b *testing.B) {
	messages := benchMessages()

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = normalizeMessages(messages)
	}
}

// BenchmarkChatOpts measures sampling-option mapping.
func BenchmarkChatOpts(b *testing.B) {
	temp := float32(0.7)
	topP := float32(0.9)
	opts := ai.GenerateOptions{Temperature: &temp, TopP: &topP, MaxTokens: 512}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = chatOpts(opts)
	}
}

// BenchmarkOllamaTools measures tool-definition mapping.
func BenchmarkOllamaTools(b *testing.B) {
	tools := []ai.Tool{
		{
			Name:        "get_weather",
			Description: "gets the weather",
			Parameters:  map[string]any{"type": "object"},
		},
		{Name: "get_time", Description: "gets the time"},
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = ollamaTools(tools)
	}
}

// BenchmarkToolCalls measures tool-call argument encoding and deterministic
// call_N id assignment on the response path.
func BenchmarkToolCalls(b *testing.B) {
	var tc ollamaToolCall
	tc.Function.Name = "get_weather"
	tc.Function.Arguments = map[string]any{"city": "Paris", "units": "metric"}
	calls := []ollamaToolCall{tc, tc}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		if _, err := toolCalls(calls); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkResponseFormat measures structured-output format mapping.
func BenchmarkResponseFormat(b *testing.B) {
	rf := &ai.ResponseFormat{JSONSchema: map[string]any{
		"name":   "response",
		"schema": map[string]any{"type": "object"},
	}}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = responseFormat(rf)
	}
}

// BenchmarkEncode measures request-body JSON encoding.
func BenchmarkEncode(b *testing.B) {
	req := chatRequest{Model: "llama3", Messages: normalizeMessages(benchMessages()), Stream: false}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		if _, err := encode(req); err != nil {
			b.Fatal(err)
		}
	}
}
