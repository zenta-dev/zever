package ai

import (
	"context"
)

// AI defines the LLM facade contract for generation, streaming and embeddings.
type AI interface {
	// Generate produces a completion for model from messages.
	Generate(ctx context.Context, model string, messages []Message, opts GenerateOptions) (Generation, error)
	// Stream streams completion chunks for model from messages.
	Stream(ctx context.Context, model string, messages []Message, opts GenerateOptions) (<-chan StreamChunk, error)
	// Embed returns embeddings for inputs using model.
	Embed(ctx context.Context, model string, inputs []string, opts EmbedOptions) ([][]float32, error)
	// Close releases backend resources held by the adapter.
	Close() error
}

// Factory creates an AI from the given Options.
type Factory func(opts Options) (AI, error)
