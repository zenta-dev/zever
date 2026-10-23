package local_test

import (
	"context"
	"os"

	medialocal "github.com/zenta-dev/zever/adapters/media/local"
	"github.com/zenta-dev/zever/core/media"
)

// Example uploads and stats one asset through the local adapter.
func Example() {
	root, err := os.MkdirTemp("", "media-example-")
	if err != nil {
		return
	}
	defer func() { _ = os.RemoveAll(root) }()

	m, err := medialocal.New(media.Options{Root: root, BaseURL: "/media"})
	if err != nil {
		return
	}
	defer func() { _ = m.Close() }()

	ctx := context.Background()

	asset, err := m.Upload(ctx, "hello.txt", []byte("hello"), media.UploadOptions{ContentType: "text/plain"})
	if err != nil {
		return
	}

	_, _ = m.Stat(ctx, asset.ID)
	_ = m.Delete(ctx, asset.ID)
}
