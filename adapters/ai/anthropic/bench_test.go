package anthropic

import (
	"errors"
	"net/url"
	"testing"

	"github.com/zenta-dev/zever/core/ai"
)

// benchMessages returns a representative multi-turn conversation covering
// system, user, assistant-with-tool-call, and tool-result turns.
func benchMessages() []ai.Message {
	return []ai.Message{
		{Role: ai.RoleSystem, Content: "be helpful"},
		{Role: ai.RoleUser, Content: "what is the weather?"},
		{Role: ai.RoleAssistant, Content: "calling tool", ToolCalls: []ai.ToolCall{
			{ID: "t1", Name: "get_weather", Arguments: `{"city":"Paris"}`},
		}},
		{Role: ai.RoleTool, ToolCallID: "t1", Content: `{"temp":20}`},
		{Role: ai.RoleUser, Content: "thanks"},
	}
}

// benchGenerateOptions returns options exercising every mapped knob: sampling,
// tools, tool-choice, and parallel-tool-call disabling.
func benchGenerateOptions() ai.GenerateOptions {
	temp := float32(0.7)
	topP := float32(0.9)
	disable := false

	return ai.GenerateOptions{
		Temperature: &temp,
		TopP:        &topP,
		MaxTokens:   1024,
		Tools: []ai.Tool{
			{
				Name:        "get_weather",
				Description: "gets the weather",
				Parameters: map[string]any{
					"properties": map[string]any{"city": map[string]any{"type": "string"}},
					"required":   []any{"city"},
				},
			},
		},
		ToolChoice:        ai.ToolChoiceRequired,
		ParallelToolCalls: &disable,
	}
}

// BenchmarkBuildMessageParams measures the pure ai.Message -> Anthropic
// MessageNewParams mapping that Generate and Stream both perform per call.
func BenchmarkBuildMessageParams(b *testing.B) {
	messages := benchMessages()
	opts := benchGenerateOptions()

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = buildMessageParams("claude-3-5-sonnet", messages, opts)
	}
}

// BenchmarkBuildToolInputSchema measures tool parameter -> schema conversion.
func BenchmarkBuildToolInputSchema(b *testing.B) {
	params := map[string]any{
		"properties": map[string]any{
			"city":    map[string]any{"type": "string"},
			"country": map[string]any{"type": "string"},
		},
		"required": []any{"city"},
		"extra":    "value",
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = buildToolInputSchema(params)
	}
}

// BenchmarkSafeInt64ToInt measures the usage-token range check on the
// response path.
func BenchmarkSafeInt64ToInt(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		if _, err := safeInt64ToInt(1 << 40); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkRedactURLError measures credential redaction on the error path.
func BenchmarkRedactURLError(b *testing.B) {
	err := &url.Error{
		Op:  "Post",
		URL: "https://api.anthropic.com/v1/messages?x-api-key=secret123&foo=bar",
		Err: errors.New("boom"),
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = redactURLError(err)
	}
}

// BenchmarkRegister measures the exported registry-wiring entrypoint.
func BenchmarkRegister(b *testing.B) {
	b.ReportAllocs()

	for b.Loop() {
		Register()
	}
}
