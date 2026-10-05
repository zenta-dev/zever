package local

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zenta-dev/zever/core/media"
)

func TestUpload_emptyData(t *testing.T) {
	t.Parallel()
	m := openTest(t, media.Options{})
	ctx := t.Context()

	asset, err := m.Upload(ctx, "f.txt", nil, media.UploadOptions{ContentType: "text/plain"})
	if err != nil {
		t.Fatalf("Upload err = %v", err)
	}
	if asset.Size != 0 {
		t.Errorf("Size = %d, want 0", asset.Size)
	}

	data, err := m.Download(ctx, asset.ID)
	if err != nil {
		t.Fatalf("Download err = %v", err)
	}
	if len(data) != 0 {
		t.Errorf("Download len = %d, want 0", len(data))
	}

	info, err := m.Stat(ctx, asset.ID)
	if err != nil {
		t.Fatalf("Stat err = %v", err)
	}
	if info.Size != 0 {
		t.Errorf("Stat Size = %d, want 0", info.Size)
	}
}

func TestUpload_emptyPathFallsBackToBin(t *testing.T) {
	t.Parallel()
	m := openTest(t, media.Options{})

	asset, err := m.Upload(t.Context(), "", []byte("x"), media.UploadOptions{})
	if err != nil {
		t.Fatalf("Upload err = %v", err)
	}
	if !strings.HasSuffix(asset.URL, ".bin") {
		t.Errorf("URL = %q, want .bin suffix", asset.URL)
	}
}

func TestUpload_traversalPathOnlyUsesExt(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	m := openTest(t, media.Options{Root: root})

	asset, err := m.Upload(t.Context(), "../evil.txt", []byte("x"), media.UploadOptions{})
	if err != nil {
		t.Fatalf("Upload err = %v", err)
	}
	if !strings.HasSuffix(asset.URL, ".txt") {
		t.Errorf("URL = %q, want .txt suffix", asset.URL)
	}
	if _, err := os.Stat(filepath.Join(root, "..", "evil.txt")); !os.IsNotExist(err) {
		t.Error("file escaped root via traversal path")
	}
}

func TestDownloadRange_emptyFile(t *testing.T) {
	t.Parallel()
	m := openTest(t, media.Options{})
	ctx := t.Context()

	asset, err := m.Upload(ctx, "f.txt", nil, media.UploadOptions{})
	if err != nil {
		t.Fatalf("Upload err = %v", err)
	}
	if _, err := m.DownloadRange(ctx, asset.ID, 0, 0); !errors.Is(err, media.ErrInvalidRange) {
		t.Errorf("DownloadRange on empty file err = %v, want ErrInvalidRange", err)
	}
}

func TestDownload_directoryAsset(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	m := openTest(t, media.Options{Root: root})
	ctx := t.Context()

	asset, err := m.Upload(ctx, "f.txt", []byte("x"), media.UploadOptions{})
	if err != nil {
		t.Fatalf("Upload err = %v", err)
	}

	dest := destPath(t, root, asset.ID)
	if err := os.Remove(dest); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(dest, 0o750); err != nil {
		t.Fatal(err)
	}

	if _, err := m.Download(ctx, asset.ID); err == nil {
		t.Fatal("Download of directory asset = nil, want error")
	}
}

func TestOpen_negativeOptions(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		mut  func(*media.Options)
	}{
		{name: "derived ttl", mut: func(o *media.Options) { o.DerivedTTL = -1 }},
		{name: "max pixels", mut: func(o *media.Options) { o.MaxPixels = -1 }},
		{name: "presign ttl", mut: func(o *media.Options) { o.PresignTTL = -1 }},
		{name: "max duration", mut: func(o *media.Options) { o.MaxDuration = -1 }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			opts := media.Options{Root: t.TempDir()}
			tc.mut(&opts)
			if _, err := New(opts); !errors.Is(err, media.ErrInvalidOptions) {
				t.Fatalf("New err = %v, want ErrInvalidOptions", err)
			}
		})
	}
}

func TestTransform_qualityBoundaries(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	m := openTest(t, media.Options{Root: root})
	ctx := t.Context()

	asset, err := m.Upload(ctx, "img.png", pngBytes(t, 16, 16), media.UploadOptions{ContentType: "image/png"})
	if err != nil {
		t.Fatalf("Upload err = %v", err)
	}

	for i, q := range []int{1, 100} {
		ops := media.TransformOps{Width: "8", Height: "8", Quality: q}
		url, err := m.Transform(ctx, asset.ID, ops)
		if err != nil {
			t.Fatalf("Transform quality %d err = %v", q, err)
		}
		if !strings.HasSuffix(url, ".png") {
			t.Errorf("Transform quality %d URL = %q, want .png suffix", q, url)
		}
		matches, err := filepath.Glob(filepath.Join(root, "derived", "*.png"))
		if err != nil {
			t.Fatalf("glob derived: %v", err)
		}
		if len(matches) != i+1 {
			t.Errorf("Transform quality %d: derived files = %d, want %d", q, len(matches), i+1)
		}
	}
}

func TestTransform_emptyFormatKeepsSourceExt(t *testing.T) {
	t.Parallel()
	m := openTest(t, media.Options{})
	ctx := t.Context()

	asset, err := m.Upload(ctx, "img.png", pngBytes(t, 16, 16), media.UploadOptions{ContentType: "image/png"})
	if err != nil {
		t.Fatalf("Upload err = %v", err)
	}
	url, err := m.Transform(ctx, asset.ID, media.TransformOps{Width: "8", Height: "8"})
	if err != nil {
		t.Fatalf("Transform err = %v", err)
	}
	if !strings.HasSuffix(url, ".png") {
		t.Errorf("URL = %q, want .png suffix", url)
	}
}

func TestProbe_emptyFile(t *testing.T) {
	t.Parallel()
	m := openTest(t, media.Options{})

	asset, err := m.Upload(t.Context(), "img.png", nil, media.UploadOptions{ContentType: "image/png"})
	if err != nil {
		t.Fatalf("Upload err = %v", err)
	}
	if _, err := m.Probe(t.Context(), asset.ID); !errors.Is(err, media.ErrProbeFailed) {
		t.Errorf("Probe empty file err = %v, want ErrProbeFailed", err)
	}
}
