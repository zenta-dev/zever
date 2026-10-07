package noop

import (
	"context"

	"github.com/zenta-dev/zever/core/cdn"
)

// noopAdapter implements cdn.CDN by discarding every purge.
type noopAdapter struct{}

var _ cdn.CDN = (*noopAdapter)(nil)

// New returns a no-op CDN. It accepts any options, including zero values.
func New(_ cdn.Options) (cdn.CDN, error) {
	return &noopAdapter{}, nil
}

// Purge discards the purge request and always succeeds.
func (n *noopAdapter) Purge(_ context.Context, _ cdn.PurgeRequest) error {
	return nil
}

// Close releases nothing and always succeeds.
func (n *noopAdapter) Close(_ context.Context) error {
	return nil
}

// Name returns the canonical adapter name.
func (n *noopAdapter) Name() string {
	return "noop"
}
