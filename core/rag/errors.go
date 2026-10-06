package rag

import (
	"errors"
	"fmt"
)

var (
	// ErrNilClient is returned when New is called without an ai.AI backend.
	ErrNilClient = errors.New("rag: nil client")
	// ErrNilStore is returned when New is called without a vectorstore.
	ErrNilStore = errors.New("rag: nil vectorstore")
	// ErrInvalidOptions is returned for invalid engine options.
	ErrInvalidOptions = errors.New("rag: invalid options")
	// ErrInvalidDocument is returned for a document missing an ID.
	ErrInvalidDocument = errors.New("rag: invalid document")
	// ErrEmptyQuery is returned when a query string is empty.
	ErrEmptyQuery = errors.New("rag: empty query")
	// ErrEmbeddingCount is returned when an embedder returns the wrong count.
	ErrEmbeddingCount = errors.New("rag: embedding count mismatch")
)

// InvalidOptionsError reports an options validation failure.
type InvalidOptionsError struct {
	// Reason describes the validation failure.
	Reason string
}

// Error returns a human-readable invalid-options message.
func (e InvalidOptionsError) Error() string {
	return fmt.Sprintf("%s: %s", ErrInvalidOptions, e.Reason)
}

// Unwrap returns ErrInvalidOptions.
func (e InvalidOptionsError) Unwrap() error { return ErrInvalidOptions }

// InvalidDocumentError reports a malformed document.
type InvalidDocumentError struct {
	// Reason describes the failure.
	Reason string
}

// Error returns a human-readable invalid-document message.
func (e InvalidDocumentError) Error() string {
	return fmt.Sprintf("%s: %s", ErrInvalidDocument, e.Reason)
}

// Unwrap returns ErrInvalidDocument.
func (e InvalidDocumentError) Unwrap() error { return ErrInvalidDocument }

// EmbeddingCountError reports a mismatch between requested inputs and returned
// embeddings.
type EmbeddingCountError struct {
	// Got is the number of embeddings returned.
	Got int
	// Want is the number of inputs requested.
	Want int
}

// Error returns a human-readable embedding-count message.
func (e EmbeddingCountError) Error() string {
	return fmt.Sprintf("%s: got %d for %d inputs", ErrEmbeddingCount, e.Got, e.Want)
}

// Unwrap returns ErrEmbeddingCount.
func (e EmbeddingCountError) Unwrap() error { return ErrEmbeddingCount }
