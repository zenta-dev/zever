package agent

import (
	"context"
	"testing"

	"github.com/zenta-dev/zever/core/ai"
)

// stubTextAI always answers without tool calls.
type stubTextAI struct{ content string }

func (s *stubTextAI) Generate(context.Context, string, []ai.Message, ai.GenerateOptions) (ai.Generation, error) {
	return ai.Generation{Content: s.content}, nil
}

func (s *stubTextAI) Stream(context.Context, string, []ai.Message, ai.GenerateOptions) (<-chan ai.StreamChunk, error) {
	ch := make(chan ai.StreamChunk)
	close(ch)
	return ch, nil
}

func (s *stubTextAI) Embed(context.Context, string, []string, ai.EmbedOptions) ([][]float32, error) {
	return nil, nil
}

func (s *stubTextAI) Close() error { return nil }

// stubToolAI emits one tool call then a final answer.
type stubToolAI struct{}

func (stubToolAI) Generate(_ context.Context, _ string, msgs []ai.Message, _ ai.GenerateOptions) (ai.Generation, error) {
	for _, m := range msgs {
		if m.Role == ai.RoleTool {
			return ai.Generation{Content: "done"}, nil
		}
	}
	return ai.Generation{ToolCalls: []ai.ToolCall{{ID: "1", Name: "add", Arguments: `{"a":1}`}}}, nil
}

func (stubToolAI) Stream(context.Context, string, []ai.Message, ai.GenerateOptions) (<-chan ai.StreamChunk, error) {
	ch := make(chan ai.StreamChunk)
	close(ch)
	return ch, nil
}

func (stubToolAI) Embed(context.Context, string, []string, ai.EmbedOptions) ([][]float32, error) {
	return nil, nil
}

func (stubToolAI) Close() error { return nil }

// stubJSONAI always returns valid JSON for GenerateStructured.
type stubJSONAI struct{}

func (stubJSONAI) Generate(context.Context, string, []ai.Message, ai.GenerateOptions) (ai.Generation, error) {
	return ai.Generation{Content: `{"name":"zever"}`}, nil
}

func (stubJSONAI) Stream(context.Context, string, []ai.Message, ai.GenerateOptions) (<-chan ai.StreamChunk, error) {
	ch := make(chan ai.StreamChunk)
	close(ch)
	return ch, nil
}

func (stubJSONAI) Embed(context.Context, string, []string, ai.EmbedOptions) ([][]float32, error) {
	return nil, nil
}

func (stubJSONAI) Close() error { return nil }

func mustBenchLoop(b *testing.B, client ai.AI, opts Options) *Loop {
	b.Helper()

	l, err := New(client, opts)
	if err != nil {
		b.Fatalf("New() error = %v", err)
	}
	return l
}

// BenchmarkNew measures Loop construction with a small tool set.
func BenchmarkNew(b *testing.B) {
	client := &stubTextAI{content: "ok"}
	opts := Options{Model: "m", Tools: []Tool{echoTool("a"), echoTool("b")}}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if _, err := New(client, opts); err != nil {
			b.Fatalf("New() error = %v", err)
		}
	}
}

// BenchmarkRunNoTools measures the single-generation fast path.
func BenchmarkRunNoTools(b *testing.B) {
	l := mustBenchLoop(b, &stubTextAI{content: "ok"}, Options{Model: "m"})
	msgs := []ai.Message{{Role: ai.RoleUser, Content: "hi"}}
	ctx := context.Background()

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if _, err := l.Run(ctx, msgs); err != nil {
			b.Fatalf("Run() error = %v", err)
		}
	}
}

// BenchmarkRunToolLoop measures generate -> dispatch -> generate.
func BenchmarkRunToolLoop(b *testing.B) {
	l := mustBenchLoop(b, stubToolAI{}, Options{Model: "m", Tools: []Tool{echoTool("add")}})
	msgs := []ai.Message{{Role: ai.RoleUser, Content: "add"}}
	ctx := context.Background()

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if _, err := l.Run(ctx, msgs); err != nil {
			b.Fatalf("Run() error = %v", err)
		}
	}
}

// BenchmarkDecodeArgs measures tool-argument JSON decoding.
func BenchmarkDecodeArgs(b *testing.B) {
	raw := `{"a":1,"b":"x","c":[1,2,3],"d":{"e":true}}`

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if _, err := decodeArgs(raw); err != nil {
			b.Fatalf("decodeArgs() error = %v", err)
		}
	}
}

// BenchmarkGenerateStructured measures constrained generation plus decode.
func BenchmarkGenerateStructured(b *testing.B) {
	schema := map[string]any{"type": "object"}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		var out struct {
			Name string `json:"name"`
		}
		if _, err := GenerateStructured(b.Context(), stubJSONAI{}, "m", nil, schema, &out); err != nil {
			b.Fatalf("GenerateStructured() error = %v", err)
		}
	}
}

// BenchmarkTools measures the registration-order snapshot.
func BenchmarkTools(b *testing.B) {
	l := mustBenchLoop(b, &stubTextAI{}, Options{Tools: []Tool{echoTool("a"), echoTool("b"), echoTool("c")}})

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		_ = l.Tools()
	}
}
