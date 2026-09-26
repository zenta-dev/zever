package ai

// Adapter identifies the LLM backend implementation.
type Adapter int

const (
	// Anthropic selects the Anthropic backend.
	Anthropic Adapter = iota
	// OpenAI selects the OpenAI backend.
	OpenAI
	// Gemini selects the Gemini backend.
	Gemini
	// Ollama selects the Ollama backend.
	Ollama
)

// String returns the canonical name of Adapter.
func (a Adapter) String() string {
	switch a {
	case Anthropic:
		return "anthropic"
	case OpenAI:
		return "openai"
	case Gemini:
		return "gemini"
	case Ollama:
		return "ollama"
	default:
		return "unknown"
	}
}

// ParseAdapter parses adapter name into an Adapter.
// Only exact lowercase names match; anything else fails.
func ParseAdapter(s string) (Adapter, error) {
	switch s {
	case "anthropic":
		return Anthropic, nil
	case "openai":
		return OpenAI, nil
	case "gemini":
		return Gemini, nil
	case "ollama":
		return Ollama, nil
	default:
		return Anthropic, &InvalidAdapterError{Adapter: s}
	}
}
