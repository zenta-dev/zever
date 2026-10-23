package media

import (
	"context"
)

// Media defines the media-asset contract for media backends.
// Implementations return the zero value on error.
type Media interface {
	// Upload stores data at path. It returns the zero Asset on error.
	Upload(ctx context.Context, path string, data []byte, opts UploadOptions) (Asset, error)
	// Download fetches asset id. It returns nil on error.
	Download(ctx context.Context, id string) ([]byte, error)
	// DownloadRange fetches the byte range [offset, offset+length) of asset id.
	// A length <= 0 fetches to the end of the asset. Ranges are still capped
	// at the configured download limit. It returns nil on error.
	DownloadRange(ctx context.Context, id string, offset, length int64) ([]byte, error)
	// Delete removes asset id. Deleting a missing asset is a no-op returning nil.
	Delete(ctx context.Context, id string) error
	// Stat describes asset id. It returns the zero Info on error.
	Stat(ctx context.Context, id string) (Info, error)
	// Probe detects the media properties of asset id. It returns the zero Probe on error.
	Probe(ctx context.Context, id string) (Probe, error)
	// Transform derives a variant of asset id per ops and returns its URL.
	// It returns the empty string on error.
	Transform(ctx context.Context, id string, ops TransformOps) (string, error)
	// Close releases backend resources.
	Close() error
}

// Factory creates a Media from the given Options.
type Factory func(opts Options) (Media, error)
