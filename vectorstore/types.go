package vectorstore

// Vector is a single stored embedding with its identifier and metadata.
type Vector struct {
	// ID is the unique identifier of the vector.
	ID string
	// Embedding is the dense vector values.
	Embedding []float32
	// Metadata carries caller-defined attributes.
	Metadata map[string]any
}

// ScoreMatch is a single query result pairing an identifier with its score.
type ScoreMatch struct {
	// ID is the identifier of the matched vector.
	ID string
	// Score is the cosine similarity of the match; higher means more similar.
	Score float32
	// Metadata carries the stored attributes of the matched vector.
	Metadata map[string]any
}
