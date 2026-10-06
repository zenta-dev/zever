package rag

import "github.com/zenta-dev/zever/core/ai"

// Document is a single source item to ingest.
type Document struct {
	// ID is the unique document identifier. Required.
	ID string
	// Content is the text to chunk and embed.
	Content string
	// Metadata carries caller-defined attributes copied onto every chunk.
	Metadata map[string]any
}

// Validate reports whether d has an ID. Empty content is allowed and ingests
// nothing.
func (d Document) Validate() error {
	if d.ID == "" {
		return InvalidDocumentError{Reason: "id is required"}
	}
	return nil
}

// Source is a retrieved context item.
type Source struct {
	// ID identifies the stored chunk.
	ID string
	// Score is the retrieval similarity; higher means more relevant.
	Score float32
	// Content is the chunk text stored at ingestion.
	Content string
	// Metadata carries the stored attributes of the chunk.
	Metadata map[string]any
}

// Answer is the outcome of grounded generation.
type Answer struct {
	// Text is the generated answer.
	Text string
	// Sources lists the retrieved context in ranked order.
	Sources []Source
	// Usage reports generation token consumption.
	Usage ai.Usage
}
