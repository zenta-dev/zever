package openai

import (
	"testing"

	"github.com/openai/openai-go/v3"

	"github.com/zenta-dev/zever/core/ai"
)

// benchMessages returns a representative conversation covering system, user,
// assistant-with-tool-call, and tool-result turns.
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

// BenchmarkBuildMessages measures the pure ai.Message -> OpenAI message
// mapping performed on every Generate/Stream call.
func BenchmarkBuildMessages(b *testing.B) {
	messages := benchMessages()

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = buildMessages(messages)
	}
}

// BenchmarkBuildTools measures tool-definition mapping.
func BenchmarkBuildTools(b *testing.B) {
	tools := []ai.Tool{
		{
			Name:        "get_weather",
			Description: "gets the weather",
			Parameters: map[string]any{
				"type":       "object",
				"properties": map[string]any{"city": map[string]any{"type": "string"}},
			},
		},
		{Name: "get_time"},
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = buildTools(tools)
	}
}

// BenchmarkBuildToolChoice measures tool-choice string mapping.
func BenchmarkBuildToolChoice(b *testing.B) {
	choices := []string{
		string(ai.ToolChoiceAuto),
		string(ai.ToolChoiceRequired),
		string(ai.ToolChoiceNone),
		"tool:get_weather",
		"unknown",
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = buildToolChoice(choices[i%len(choices)])
	}
}

// BenchmarkApplyResponseFormat measures structured-output mapping.
func BenchmarkApplyResponseFormat(b *testing.B) {
	rf := &ai.ResponseFormat{JSONSchema: map[string]any{
		"name":   "response",
		"strict": true,
		"schema": map[string]any{
			"type":       "object",
			"properties": map[string]any{"answer": map[string]any{"type": "string"}},
		},
	}}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		var params openai.ChatCompletionNewParams
		applyResponseFormat(&params, rf)
	}
}

// BenchmarkRegister measures the exported registry-wiring entrypoint.
func BenchmarkRegister(b *testing.B) {
	b.ReportAllocs()

	for b.Loop() {
		Register()
	}
}
