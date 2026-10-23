package document

import (
	"context"
)

// Document defines the document-rendering contract for document backends.
// Implementations return nil output on error.
type Document interface {
	// Render renders source into format. It returns nil output on error.
	Render(ctx context.Context, source []byte, format OutputFormat) ([]byte, error)
	// Close releases backend resources.
	Close() error
}

// Factory creates a Document from the given Options.
type Factory func(opts Options) (Document, error)
