package adapters

import (
	"github.com/zenta-dev/zever/ai"
	"github.com/zenta-dev/zever/ai/anthropic"
	"github.com/zenta-dev/zever/ai/gemini"
	"github.com/zenta-dev/zever/ai/openai"
)

// RegisterAI registers the SDK-backed AI adapters (anthropic, openai,
// gemini) that the core container no longer imports. The local ollama
// adapter stays wired by the container itself. Registration only fills the
// ai factory map; it performs no I/O. Duplicate registrations are ignored,
// so calling RegisterAI more than once (or alongside RegisterAll) is safe.
func RegisterAI() {
	_ = ai.Register(ai.Anthropic, anthropic.New)
	_ = ai.Register(ai.OpenAI, openai.New)
	_ = ai.Register(ai.Gemini, gemini.New)
}
