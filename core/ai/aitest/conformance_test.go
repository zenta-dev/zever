package aitest_test

import (
	"context"
	"testing"

	"github.com/zenta-dev/zever/core/ai"
	"github.com/zenta-dev/zever/core/ai/aitest"
)

// fakeAI is a deterministic in-memory ai.AI: no network, no credentials.
type fakeAI struct{ closed bool }

func (f *fakeAI) Generate(ctx context.Context, _ string, _ []ai.Message, _ ai.GenerateOptions) (ai.Generation, error) {
	if err := ctx.Err(); err != nil {
		return ai.Generation{}, err
	}

	return ai.Generation{Content: "kit"}, nil
}

func (f *fakeAI) Stream(ctx context.Context, _ string, _ []ai.Message, _ ai.GenerateOptions) (<-chan ai.StreamChunk, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	ch := make(chan ai.StreamChunk, 1)
	ch <- ai.StreamChunk{Delta: "kit", Done: true}
	close(ch)

	return ch, nil
}

func (f *fakeAI) Embed(ctx context.Context, _ string, inputs []string, _ ai.EmbedOptions) ([][]float32, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	vecs := make([][]float32, len(inputs))
	for i := range vecs {
		vecs[i] = []float32{0.1, 0.2}
	}

	return vecs, nil
}

func (f *fakeAI) Close() error {
	f.closed = true

	return nil
}

// TestConformanceFake proves the kit passes against the in-memory fake.
func TestConformanceFake(t *testing.T) {
	t.Parallel()

	aitest.Conformance(t, func(t *testing.T) ai.AI {
		t.Helper()

		return &fakeAI{}
	})
}
