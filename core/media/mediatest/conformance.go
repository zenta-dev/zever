// Package mediatest provides the conformance kit third-party media adapters run to prove backend parity.
package mediatest

import (
	"bytes"
	"errors"
	"image"
	"image/png"
	"testing"

	"github.com/zenta-dev/zever/core/media"
)

// absentID is a well-formed 32-hex ID that no subtest ever uploads, so
// not-found assertions stay deterministic on fresh temp roots.
const absentID = "0123456789abcdef0123456789abcdef"

// Conformance verifies factory-built media backends implement the media.Media
// contract over image assets (no ffmpeg needed): Upload/Download round-trip,
// DownloadRange slicing, Delete as missing-tolerant no-op, Stat, image Probe,
// image Transform, invalid-ID / not-found / invalid-range sentinels, and
// Close. Each subtest takes a fresh instance from factory so cases stay
// isolated. Tests are deterministic and touch no network.
func Conformance(t *testing.T, factory func(t *testing.T) media.Media) {
	t.Helper()

	t.Run("UploadDownload", func(t *testing.T) { conformanceUploadDownload(t, factory) })
	t.Run("DownloadRange", func(t *testing.T) { conformanceDownloadRange(t, factory) })
	t.Run("Delete", func(t *testing.T) { conformanceDelete(t, factory) })
	t.Run("InvalidID", func(t *testing.T) { conformanceInvalidID(t, factory) })
	t.Run("NotFound", func(t *testing.T) { conformanceNotFound(t, factory) })
	t.Run("InvalidRange", func(t *testing.T) { conformanceInvalidRange(t, factory) })
	t.Run("Probe", func(t *testing.T) { conformanceProbe(t, factory) })
	t.Run("ProbeUnsupported", func(t *testing.T) { conformanceProbeUnsupported(t, factory) })
	t.Run("Transform", func(t *testing.T) { conformanceTransform(t, factory) })
	t.Run("Close", func(t *testing.T) { conformanceClose(t, factory) })
}

// mustPNG encodes a deterministic w-by-h PNG for upload fixtures.
func mustPNG(t *testing.T, w, h int) []byte {
	t.Helper()

	buf := new(bytes.Buffer)
	if err := png.Encode(buf, image.NewRGBA(image.Rect(0, 0, w, h))); err != nil {
		t.Fatalf("png.Encode() error = %v", err)
	}

	return buf.Bytes()
}

// mustUpload stores data and fails the test on error.
func mustUpload(t *testing.T, m media.Media, path string, data []byte, contentType string) media.Asset {
	t.Helper()

	asset, err := m.Upload(t.Context(), path, data, media.UploadOptions{ContentType: contentType})
	if err != nil {
		t.Fatalf("Upload(%q) error = %v", path, err)
	}

	if asset.ID == "" {
		t.Fatal("Upload() ID = empty, want non-empty")
	}

	return asset
}

func conformanceUploadDownload(t *testing.T, factory func(t *testing.T) media.Media) {
	t.Helper()

	ctx := t.Context()
	m := factory(t)
	data := mustPNG(t, 8, 6)

	asset := mustUpload(t, m, "kit.png", data, "image/png")

	if asset.Size != int64(len(data)) {
		t.Errorf("Asset.Size = %d, want %d", asset.Size, len(data))
	}

	if asset.ContentType != "image/png" {
		t.Errorf("Asset.ContentType = %q, want image/png", asset.ContentType)
	}

	got, err := m.Download(ctx, asset.ID)
	if err != nil {
		t.Fatalf("Download() error = %v", err)
	}

	if !bytes.Equal(got, data) {
		t.Error("Download() bytes differ from upload")
	}

	info, err := m.Stat(ctx, asset.ID)
	if err != nil {
		t.Fatalf("Stat() error = %v", err)
	}

	if info.ID != asset.ID {
		t.Errorf("Info.ID = %q, want %q", info.ID, asset.ID)
	}

	if info.Size != int64(len(data)) {
		t.Errorf("Info.Size = %d, want %d", info.Size, len(data))
	}

	if info.ContentType != "image/png" {
		t.Errorf("Info.ContentType = %q, want image/png", info.ContentType)
	}
}

func conformanceDownloadRange(t *testing.T, factory func(t *testing.T) media.Media) {
	t.Helper()

	ctx := t.Context()
	m := factory(t)
	data := mustPNG(t, 8, 6)

	asset := mustUpload(t, m, "kit-range.png", data, "image/png")

	part, err := m.DownloadRange(ctx, asset.ID, 1, 2)
	if err != nil {
		t.Fatalf("DownloadRange() error = %v", err)
	}

	if !bytes.Equal(part, data[1:3]) {
		t.Error("DownloadRange(1,2) bytes differ from slice")
	}

	full, err := m.DownloadRange(ctx, asset.ID, 0, 0)
	if err != nil {
		t.Fatalf("DownloadRange(0,0) error = %v", err)
	}

	if !bytes.Equal(full, data) {
		t.Error("DownloadRange(0,0) bytes differ from full asset")
	}
}

func conformanceDelete(t *testing.T, factory func(t *testing.T) media.Media) {
	t.Helper()

	ctx := t.Context()
	m := factory(t)

	asset := mustUpload(t, m, "kit-del.png", mustPNG(t, 4, 4), "image/png")

	if err := m.Delete(ctx, asset.ID); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}

	if _, err := m.Download(ctx, asset.ID); !errors.Is(err, media.ErrNotFound) {
		t.Errorf("Download after Delete err = %v, want ErrNotFound", err)
	}

	// Deleting a missing asset is a no-op returning nil.
	if err := m.Delete(ctx, asset.ID); err != nil {
		t.Errorf("Delete(again) error = %v, want nil", err)
	}
}

func conformanceInvalidID(t *testing.T, factory func(t *testing.T) media.Media) {
	t.Helper()

	ctx := t.Context()
	m := factory(t)

	const bad = "not-an-id!"

	if _, err := m.Download(ctx, bad); !errors.Is(err, media.ErrInvalidID) {
		t.Errorf("Download(bad id) err = %v, want ErrInvalidID", err)
	}

	if _, err := m.DownloadRange(ctx, bad, 0, 0); !errors.Is(err, media.ErrInvalidID) {
		t.Errorf("DownloadRange(bad id) err = %v, want ErrInvalidID", err)
	}

	if err := m.Delete(ctx, bad); !errors.Is(err, media.ErrInvalidID) {
		t.Errorf("Delete(bad id) err = %v, want ErrInvalidID", err)
	}

	if _, err := m.Stat(ctx, bad); !errors.Is(err, media.ErrInvalidID) {
		t.Errorf("Stat(bad id) err = %v, want ErrInvalidID", err)
	}

	if _, err := m.Probe(ctx, bad); !errors.Is(err, media.ErrInvalidID) {
		t.Errorf("Probe(bad id) err = %v, want ErrInvalidID", err)
	}

	if _, err := m.Transform(ctx, bad, media.TransformOps{}); !errors.Is(err, media.ErrInvalidID) {
		t.Errorf("Transform(bad id) err = %v, want ErrInvalidID", err)
	}

	var idErr *media.InvalidIDError

	if _, err := m.Download(ctx, bad); !errors.As(err, &idErr) {
		t.Errorf("Download(bad id) err = %T %v, want *InvalidIDError", err, err)
	} else if idErr.ID != bad {
		t.Errorf("InvalidIDError.ID = %q, want %q", idErr.ID, bad)
	}
}

func conformanceNotFound(t *testing.T, factory func(t *testing.T) media.Media) {
	t.Helper()

	ctx := t.Context()
	m := factory(t)

	if _, err := m.Download(ctx, absentID); !errors.Is(err, media.ErrNotFound) {
		t.Errorf("Download(absent) err = %v, want ErrNotFound", err)
	}

	if _, err := m.DownloadRange(ctx, absentID, 0, 0); !errors.Is(err, media.ErrNotFound) {
		t.Errorf("DownloadRange(absent) err = %v, want ErrNotFound", err)
	}

	if _, err := m.Stat(ctx, absentID); !errors.Is(err, media.ErrNotFound) {
		t.Errorf("Stat(absent) err = %v, want ErrNotFound", err)
	}

	if _, err := m.Probe(ctx, absentID); !errors.Is(err, media.ErrNotFound) {
		t.Errorf("Probe(absent) err = %v, want ErrNotFound", err)
	}

	if _, err := m.Transform(ctx, absentID, media.TransformOps{}); !errors.Is(err, media.ErrNotFound) {
		t.Errorf("Transform(absent) err = %v, want ErrNotFound", err)
	}

	var nfErr *media.NotFoundError

	if _, err := m.Download(ctx, absentID); !errors.As(err, &nfErr) {
		t.Errorf("Download(absent) err = %T %v, want *NotFoundError", err, err)
	} else if nfErr.ID != absentID {
		t.Errorf("NotFoundError.ID = %q, want %q", nfErr.ID, absentID)
	}
}

func conformanceInvalidRange(t *testing.T, factory func(t *testing.T) media.Media) {
	t.Helper()

	ctx := t.Context()
	m := factory(t)

	asset := mustUpload(t, m, "kit-range-err.png", mustPNG(t, 4, 4), "image/png")

	if _, err := m.DownloadRange(ctx, asset.ID, -1, 0); !errors.Is(err, media.ErrInvalidRange) {
		t.Errorf("DownloadRange(-1,0) err = %v, want ErrInvalidRange", err)
	}

	if _, err := m.DownloadRange(ctx, asset.ID, 0, -1); !errors.Is(err, media.ErrInvalidRange) {
		t.Errorf("DownloadRange(0,-1) err = %v, want ErrInvalidRange", err)
	}

	if _, err := m.DownloadRange(ctx, asset.ID, int64(len(mustPNG(t, 4, 4)))+100, 0); !errors.Is(err, media.ErrInvalidRange) {
		t.Errorf("DownloadRange(past end) err = %v, want ErrInvalidRange", err)
	}

	var rangeErr *media.InvalidRangeError

	if _, err := m.DownloadRange(ctx, asset.ID, -1, 0); !errors.As(err, &rangeErr) {
		t.Errorf("DownloadRange(-1,0) err = %T %v, want *InvalidRangeError", err, err)
	}
}

func conformanceProbe(t *testing.T, factory func(t *testing.T) media.Media) {
	t.Helper()

	m := factory(t)

	asset := mustUpload(t, m, "kit-probe.png", mustPNG(t, 8, 6), "image/png")

	probe, err := m.Probe(t.Context(), asset.ID)
	if err != nil {
		t.Fatalf("Probe() error = %v", err)
	}

	if probe.Kind != media.KindImage {
		t.Errorf("Probe.Kind = %q, want image", probe.Kind)
	}

	if probe.Width != 8 || probe.Height != 6 {
		t.Errorf("Probe size = %dx%d, want 8x6", probe.Width, probe.Height)
	}
}

func conformanceProbeUnsupported(t *testing.T, factory func(t *testing.T) media.Media) {
	t.Helper()

	m := factory(t)

	asset := mustUpload(t, m, "kit.txt", []byte("plain text, no media stream"), "text/plain")

	_, err := m.Probe(t.Context(), asset.ID)
	if !errors.Is(err, media.ErrUnsupportedFormat) {
		t.Errorf("Probe(txt) err = %v, want ErrUnsupportedFormat", err)
	}

	var ufErr *media.UnsupportedFormatError
	if !errors.As(err, &ufErr) {
		t.Errorf("Probe(txt) err = %T %v, want *UnsupportedFormatError", err, err)
	}
}

func conformanceTransform(t *testing.T, factory func(t *testing.T) media.Media) {
	t.Helper()

	m := factory(t)

	asset := mustUpload(t, m, "kit-tf.png", mustPNG(t, 16, 16), "image/png")

	url, err := m.Transform(t.Context(), asset.ID, media.TransformOps{Width: "8", Height: "8", Format: "png"})
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}

	if url == "" {
		t.Error("Transform() URL = empty, want non-empty")
	}
}

func conformanceClose(t *testing.T, factory func(t *testing.T) media.Media) {
	t.Helper()

	m := factory(t)

	if err := m.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	if err := m.Close(); err != nil {
		t.Errorf("Close() second error = %v, want nil", err)
	}
}
