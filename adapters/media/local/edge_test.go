package local

import (
	"errors"
	"testing"

	"github.com/zenta-dev/zever/core/media"
)

// edgeMedia opens a local backend over a per-test temp root.
func edgeMedia(t *testing.T) media.Media {
	t.Helper()

	m, err := New(media.Options{Root: t.TempDir()})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	t.Cleanup(func() { _ = m.Close() })

	return m
}

// TestEdgeUpload_emptyData proves a zero-byte upload succeeds and reports a
// zero size.
func TestEdgeUpload_emptyData(t *testing.T) {
	t.Parallel()

	m := edgeMedia(t)

	asset, err := m.Upload(t.Context(), "empty.png", nil, media.UploadOptions{})
	if err != nil {
		t.Fatalf("Upload: %v", err)
	}

	if asset.Size != 0 {
		t.Fatalf("Size = %d, want 0", asset.Size)
	}
}

// TestEdgeDownload_missing proves a well-formed but absent id fails closed.
func TestEdgeDownload_missing(t *testing.T) {
	t.Parallel()

	m := edgeMedia(t)

	if _, err := m.Download(t.Context(), "0123456789abcdef0123456789abcdef"); !errors.Is(err, media.ErrNotFound) {
		t.Fatalf("Download(missing) err = %v, want ErrNotFound", err)
	}
}

// TestEdgeDelete_missingNil proves deleting an absent id is a no-op.
func TestEdgeDelete_missingNil(t *testing.T) {
	t.Parallel()

	m := edgeMedia(t)

	if err := m.Delete(t.Context(), "0123456789abcdef0123456789abcdef"); err != nil {
		t.Fatalf("Delete(missing) err = %v, want nil", err)
	}
}

// TestEdgeDownloadRange_invalid proves negative offset and length fail
// closed.
func TestEdgeDownloadRange_invalid(t *testing.T) {
	t.Parallel()

	m := edgeMedia(t)
	id := "0123456789abcdef0123456789abcdef"

	if _, err := m.DownloadRange(t.Context(), id, -1, 1); !errors.Is(err, media.ErrInvalidRange) {
		t.Fatalf("DownloadRange(negative offset) err = %v, want ErrInvalidRange", err)
	}

	if _, err := m.DownloadRange(t.Context(), id, 0, -1); !errors.Is(err, media.ErrInvalidRange) {
		t.Fatalf("DownloadRange(negative length) err = %v, want ErrInvalidRange", err)
	}
}
