package search

import (
	"encoding/json"
	"fmt"
)

// DefaultLimit is the default search limit applied by all adapters for Limit<=0.
const DefaultLimit = 10

// Document is a single searchable item stored in a search index.
type Document struct {
	// ID is the unique document identifier.
	ID string
	// Index is the index or collection holding the document.
	Index string
	// Content is the full-text content of the document.
	Content string
	// Metadata carries backend-specific document attributes.
	Metadata map[string]any
}

// Validate reports whether d's metadata can be JSON-encoded.
// Nil metadata is valid.
func (d Document) Validate() error {
	if d.Metadata == nil {
		return nil
	}

	if _, err := json.Marshal(d.Metadata); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidMetadata, err)
	}

	return nil
}

// QueryOptions configures a search query.
type QueryOptions struct {
	// Limit caps returned hits; Limit<=0 selects DefaultLimit.
	Limit int
	// Offset skips the first Offset hits.
	Offset int
	// Filters narrows results; only "index" filter honored.
	Filters map[string]string
}

// Result is a paged search response.
type Result struct {
	// Hits holds ranked documents, higher scores first.
	Hits []Hit
	// Total counts all matches ignoring Limit and Offset.
	Total int64
}

// Hit is a single ranked search match.
type Hit struct {
	// ID identifies the matched document.
	ID string
	// Score ranks the hit in backend-defined units, higher ranks first.
	Score float64
	// Metadata carries backend-specific hit attributes.
	Metadata map[string]any
}
