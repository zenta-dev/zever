package ai

import (
	"context"
	"sync/atomic"
)

var testSeq atomic.Int64

type stubAI struct {
	genErr    error
	streamErr error
	embedErr  error
	closed    bool
}

func (s *stubAI) Generate(_ context.Context, _ string, _ []Message, _ GenerateOptions) (Generation, error) {
	if s.genErr != nil {
		return Generation{}, s.genErr
	}
	return Generation{Content: "ok", Usage: Usage{PromptTokens: 1, CompletionTokens: 2}, FinishReason: "stop"}, nil
}

func (s *stubAI) Stream(_ context.Context, _ string, _ []Message, _ GenerateOptions) (<-chan StreamChunk, error) {
	if s.streamErr != nil {
		return nil, s.streamErr
	}
	ch := make(chan StreamChunk, 1)
	ch <- StreamChunk{Delta: "hi", Done: true}
	close(ch)
	return ch, nil
}

func (s *stubAI) Embed(_ context.Context, _ string, inputs []string, _ EmbedOptions) ([][]float32, error) {
	if s.embedErr != nil {
		return nil, s.embedErr
	}
	out := make([][]float32, len(inputs))
	for i := range inputs {
		out[i] = []float32{0.1, 0.2}
	}
	return out, nil
}

func (s *stubAI) Close() error {
	s.closed = true
	return nil
}

