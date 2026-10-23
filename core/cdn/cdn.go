package cdn

import (
	"context"
)

// CDN purges cached content from a CDN provider's edge network.
type CDN interface {
	// Purge invalidates cached content matching req.
	Purge(ctx context.Context, req PurgeRequest) error
	// Close releases any resources held by the CDN client.
	Close(ctx context.Context) error
	// Name returns the canonical adapter name.
	Name() string
}

// PurgeRequest describes what to invalidate. A zero value is a no-op
// and fails validation in Purge.
type PurgeRequest struct {
	// URLs lists full URLs to purge.
	URLs []string
	// Tags lists cache tags to purge.
	Tags []string
	// All purges the entire zone when true.
	All bool
}

// Factory creates a CDN from the given Options.
type Factory func(opts Options) (CDN, error)
