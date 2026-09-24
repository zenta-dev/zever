package vectorstore

import (
	"encoding/json"
	"fmt"
)

// Vector is a single stored embedding with its identifier and metadata.
type Vector struct {
	// ID is the unique identifier of the vector.
	ID string
	// Embedding is the dense vector values.
	Embedding []float32
	// Metadata carries caller-defined attributes.
	Metadata map[string]any
}

// Validate reports whether v's metadata can be JSON-encoded.
// Nil metadata is valid.
func (v Vector) Validate() error {
	if v.Metadata == nil {
		return nil
	}

	if _, err := json.Marshal(v.Metadata); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidMetadata, err)
	}

	return nil
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
