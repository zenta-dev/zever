package ai_test

import (
	"context"

	"github.com/zenta-dev/zever/core/ai"
)

type stubAI struct{}

func (stubAI) Generate(_ context.Context, _ string, _ []ai.Message, _ ai.GenerateOptions) (ai.Generation, error) {
	return ai.Generation{Content: "hello from stub", FinishReason: "stop"}, nil
}

func (stubAI) Stream(_ context.Context, _ string, _ []ai.Message, _ ai.GenerateOptions) (<-chan ai.StreamChunk, error) {
	ch := make(chan ai.StreamChunk, 1)
	ch <- ai.StreamChunk{Delta: "hello from stub", Done: true}
	close(ch)
	return ch, nil
}

func (stubAI) Embed(_ context.Context, _ string, inputs []string, _ ai.EmbedOptions) ([][]float32, error) {
	out := make([][]float32, len(inputs))
	for i := range inputs {
		out[i] = []float32{0.1, 0.2}
	}
	return out, nil
}

func (stubAI) Close() error { return nil }

// ExampleOpen opens a registered backend and runs a generation.
