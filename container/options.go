package container

import (
	"github.com/zenta-dev/zever/core/agent"
	"github.com/zenta-dev/zever/core/rag"
)

// AgentOption customizes the agent loop built by Agent. Options apply only
// on first resolution; later calls return the cached singleton.
type AgentOption func(*agent.Options)

// WithMaxParallel caps concurrent tool dispatches per loop step.
func WithMaxParallel(n int) AgentOption {
	return func(o *agent.Options) { o.MaxParallel = n }
}

// WithAgentObserver receives agent execution events. It must be
// goroutine-safe when MaxParallel exceeds 1.
func WithAgentObserver(o agent.Observer) AgentOption {
	return func(opts *agent.Options) { opts.Observe = o }
}

// RAGOption customizes the engine built by RAG. Options apply only on first
// resolution; later calls return the cached singleton.
type RAGOption func(*ragConfig)

// ragConfig carries RAG options plus the hybrid flag, which has no home in
// rag.Options (the search backend is resolved, not configured).
type ragConfig struct {
	options rag.Options
	hybrid  bool
}

// WithTopK caps retrieved sources per call.
func WithTopK(n int) RAGOption {
	return func(c *ragConfig) { c.options.TopK = n }
}

// WithSystemPrompt overrides the grounding prompt.
func WithSystemPrompt(s string) RAGOption {
	return func(c *ragConfig) { c.options.SystemPrompt = s }
}

// WithSearchIndex names the keyword index hybrid engines read and write.
func WithSearchIndex(index string) RAGOption {
	return func(c *ragConfig) { c.options.SearchIndex = index }
}

// WithRAGObserver receives engine events. It must be goroutine-safe.
func WithRAGObserver(o rag.Observer) RAGOption {
	return func(c *ragConfig) { c.options.Observe = o }
}

// WithHybridSearch builds the engine over the resolved Search backend via
// rag.NewHybrid, enabling hybrid retrieval. Without it, RAG builds a
// vector-only engine.
func WithHybridSearch() RAGOption {
	return func(c *ragConfig) { c.hybrid = true }
}
