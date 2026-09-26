package media

import (
	"testing"
	"time"
)

func TestUploadOptions_zero(t *testing.T) {
	t.Parallel()

	var got UploadOptions
	if got.ContentType != "" {
		t.Errorf("ContentType = %q, want empty", got.ContentType)
	}

	if got.Public {
		t.Errorf("Public = true, want false")
	}
}

func TestAsset_zero(t *testing.T) {
	t.Parallel()

	var got Asset
	if got.ID != "" || got.URL != "" || got.ContentType != "" {
		t.Errorf("strings = %+v, want empty", got)
	}

	if got.Size != 0 {
		t.Errorf("Size = %d, want 0", got.Size)
	}
}

func TestInfo_zero(t *testing.T) {
	t.Parallel()

	var got Info
	if got.ID != "" || got.ContentType != "" {
		t.Errorf("strings = %+v, want empty", got)
	}

	if got.Size != 0 {
		t.Errorf("Size = %d, want 0", got.Size)
	}
}

func TestMediaKind_values(t *testing.T) {
	t.Parallel()

	if KindImage != "image" {
		t.Errorf("KindImage = %q, want %q", KindImage, "image")
	}

	if KindAudio != "audio" {
		t.Errorf("KindAudio = %q, want %q", KindAudio, "audio")
	}

	if KindVideo != "video" {
		t.Errorf("KindVideo = %q, want %q", KindVideo, "video")
	}

	if MediaKind("image") != KindImage {
		t.Errorf("MediaKind(image) != KindImage")
	}
}

func TestTransformOps_zero_is_identity(t *testing.T) {
	t.Parallel()

	var got TransformOps
	if got.Width != "" || got.Height != "" || got.Format != "" {
		t.Errorf("strings = %+v, want empty", got)
	}

	if got.Quality != 0 || got.Bitrate != 0 || got.CRF != 0 {
		t.Errorf("ints = %+v, want zero", got)
	}

	if got.Offset != 0 {
		t.Errorf("Offset = %v, want 0", got.Offset)
	}
}

func TestProbe_zero(t *testing.T) {
	t.Parallel()

	var got Probe
	if got.Kind != "" || got.Format != "" || got.AudioCodec != "" || got.VideoCodec != "" {
		t.Errorf("strings = %+v, want empty", got)
	}

	if got.Width != 0 || got.Height != 0 {
		t.Errorf("dims = %dx%d, want 0x0", got.Width, got.Height)
	}

	if got.Duration != 0 {
		t.Errorf("Duration = %v, want 0", got.Duration)
	}
}

func TestProbe_fields_assign(t *testing.T) {
	t.Parallel()

	got := Probe{
		Kind:       KindVideo,
		Format:     "mp4",
		Width:      1920,
		Height:     1080,
		AudioCodec: "aac",
		VideoCodec: "h264",
		Duration:   90 * time.Second,
	}
	if got.Kind != KindVideo || got.Format != "mp4" {
		t.Errorf("probe head = %+v, want video/mp4", got)
	}

	if got.Width != 1920 || got.Height != 1080 {
		t.Errorf("probe dims = %dx%d, want 1920x1080", got.Width, got.Height)
	}

	if got.AudioCodec != "aac" || got.VideoCodec != "h264" {
		t.Errorf("probe codecs = %+v, want aac/h264", got)
	}

	if got.Duration != 90*time.Second {
		t.Errorf("probe duration = %v, want 90s", got.Duration)
	}
}
