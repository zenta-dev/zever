package rag

// Default bounds applied when the matching option is unset.
const (
	// DefaultTopK is the default number of retrieved sources.
	DefaultTopK = 5
	// DefaultChunkSize is the default ingestion chunk size in runes.
	DefaultChunkSize = 512
	// DefaultChunkOverlap is the default rune overlap between chunks.
	DefaultChunkOverlap = 64
)

// Options configures an Engine.
type Options struct {
	// EmbedModel is the model used for embeddings; falls back to Model.
	EmbedModel string
	// Model is the model used for grounded generation.
	Model string
	// TopK caps retrieved sources; <= 0 selects DefaultTopK.
	TopK int
	// ChunkSize splits documents during ingestion; 0 selects DefaultChunkSize
	// and a negative value is rejected by New.
	ChunkSize int
	// ChunkOverlap is the rune overlap between chunks; < 0 selects DefaultChunkOverlap.
	ChunkOverlap int
	// SystemPrompt, when non-empty, overrides the default grounding prompt.
	SystemPrompt string
	// SearchIndex names the keyword index hybrid engines read and write;
	// empty selects DefaultSearchIndex.
	SearchIndex string
	// Observe receives engine events; nil disables observation. It must be
	// goroutine-safe: methods on one Engine may run concurrently.
	Observe Observer
}

// withDefaults returns a copy of o with unset bounds resolved.
func (o Options) withDefaults() Options {
	if o.TopK <= 0 {
		o.TopK = DefaultTopK
	}
	if o.ChunkSize <= 0 {
		o.ChunkSize = DefaultChunkSize
	}
	if o.ChunkOverlap < 0 {
		o.ChunkOverlap = DefaultChunkOverlap
	}
	return o
}

// embedModel returns the embedding model, falling back to the generation model.
func (o Options) embedModel() string {
	if o.EmbedModel != "" {
		return o.EmbedModel
	}
	return o.Model
}
