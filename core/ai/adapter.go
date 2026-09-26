package ai

// Adapter identifies the LLM backend implementation.

type Adapter string

const (
	// Anthropic selects the Anthropic backend.
	Anthropic Adapter = "anthropic"
	// OpenAI selects the OpenAI backend.
	OpenAI Adapter = "openai"
	// Gemini selects the Gemini backend.
	Gemini Adapter = "gemini"
	// Ollama selects the Ollama backend.
	Ollama Adapter = "ollama"
)

// String returns the canonical name of Adapter.
func (a Adapter) String() string {
	if a == "" {
		return "unknown"
	}
	return string(a)
}

// ParseAdapter parses adapter name into an Adapter.
// Any non-empty name is accepted to allow custom adapters; empty fails.
func ParseAdapter(s string) (Adapter, error) {
	if s == "" {
		return Adapter(""), &InvalidAdapterError{Adapter: s}
	}
	return Adapter(s), nil
}
