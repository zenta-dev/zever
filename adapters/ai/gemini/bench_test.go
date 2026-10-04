package gemini

import (
	"errors"
	"testing"

	"google.golang.org/genai"

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
		{Role: ai.RoleTool, ToolCallID: "get_weather", Content: `{"temp":20}`},
		{Role: ai.RoleUser, Content: "thanks"},
	}
}

// benchGenerateOptions returns options exercising sampling, tools, and
// tool-choice mapping.
func benchGenerateOptions() ai.GenerateOptions {
	temp := float32(0.7)
	topP := float32(0.9)

	return ai.GenerateOptions{
		Temperature: &temp,
		TopP:        &topP,
		MaxTokens:   1024,
		Tools: []ai.Tool{
			{
				Name:        "get_weather",
				Description: "gets the weather",
				Parameters: map[string]any{
					"type":       "object",
					"properties": map[string]any{"city": map[string]any{"type": "string"}},
					"required":   []any{"city"},
				},
			},
		},
		ToolChoice: ai.ToolChoiceAuto,
	}
}

// BenchmarkToGenaiContents measures the pure ai.Message -> genai.Content
// mapping performed on every Generate/Stream call.
func BenchmarkToGenaiContents(b *testing.B) {
	messages := benchMessages()

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, _ = toGenaiContents(messages)
	}
}

// BenchmarkConvertMapToSchema measures tool parameter JSON-schema conversion.
func BenchmarkConvertMapToSchema(b *testing.B) {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"city":    map[string]any{"type": "string", "description": "city name"},
			"country": map[string]any{"type": "string"},
			"tags":    map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
		},
		"required": []any{"city"},
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = convertMapToSchema(schema)
	}
}

// BenchmarkBuildGenerateConfig measures generation-option mapping including
// tool declarations and tool-choice.
func BenchmarkBuildGenerateConfig(b *testing.B) {
	opts := benchGenerateOptions()
	system := &genai.Content{Parts: []*genai.Part{genai.NewPartFromText("be helpful")}}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = buildGenerateConfig(opts, system)
	}
}

// BenchmarkMapAndRedact measures API-error mapping plus credential redaction
// on the error path.
func BenchmarkMapAndRedact(b *testing.B) {
	err := errors.New("boom: x-goog-api-key: secret")

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = mapAndRedact(err, "secret")
	}
}
