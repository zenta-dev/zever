package local_test

import (
	"context"
	"errors"
	"os"

	medialocal "github.com/zenta-dev/zever/adapters/media/local"
	"github.com/zenta-dev/zever/core/media"
)

// ExampleOpen uploads and stats one asset through the local adapter.
func ExampleOpen() {
	if err := media.Register(media.Local, medialocal.New); err != nil {
		var dup *media.DuplicateAdapterError
		if !errors.As(err, &dup) {
			return
		}
	}

	root, err := os.MkdirTemp("", "media-example-")
	if err != nil {
		return
	}
	defer func() { _ = os.RemoveAll(root) }()

	m, err := media.Open(media.Local, media.Options{Root: root, BaseURL: "/media"})
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
