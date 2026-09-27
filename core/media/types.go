package media

import "time"

// UploadOptions configures a media upload.
type UploadOptions struct {
	// ContentType is the MIME type of the uploaded bytes.
	ContentType string
	// Public marks the asset as publicly readable.
	Public bool
}

// Asset describes a stored media object.
type Asset struct {
	// ID is the unique asset identifier.
	ID string
	// URL is the access URL of the asset.
	URL string
	// ContentType is the MIME type of the asset.
	ContentType string
	// Size is the asset size in bytes.
	Size int64
}

// Info describes a stored asset without its access URL.
type Info struct {
	// ID is the asset identifier.
	ID string
	// Size is the asset size in bytes.
	Size int64
	// ContentType is the MIME type of the asset.
	ContentType string
}

// MediaKind classifies a media asset for adapter routing.
//
//nolint:revive // exported name kept stable for API compatibility
type MediaKind string

const (
	// KindImage routes image assets to the image pipeline.
	KindImage MediaKind = "image"
	// KindAudio routes audio assets to the audio pipeline.
	KindAudio MediaKind = "audio"
	// KindVideo routes video assets to the video pipeline.
	KindVideo MediaKind = "video"
)

// TransformOps describes a media transformation.
// The zero value is the identity transform.
type TransformOps struct {
	// Width is the target width. Zero keeps the source width.
	Width string
	// Height is the target height. Zero keeps the source height.
	Height string
	// Format is the target container or image format. Empty keeps the source format.
	Format string
	// Quality is the image quality for JPEG output, 1-100. Zero keeps the default.
	Quality int
	// Bitrate is the audio/video target bitrate in kbps. Zero keeps the default.
	Bitrate int
	// CRF is the video constant-rate-factor, 0-51. Zero selects the default of 23.
	// Lossless CRF 0 is not expressible; zero always means default.
	CRF int
	// Offset is the seek position for video thumbnail extraction. Zero starts at the beginning.
	Offset time.Duration
}

// Probe describes detected media properties.
type Probe struct {
	// Kind classifies the media.
	Kind MediaKind
	// Format is the container or image format name.
	Format string
	// Width is the frame width in pixels.
	Width int
	// Height is the frame height in pixels.
	Height int
	// AudioCodec is the audio codec name. Empty when the media has no audio stream.
	AudioCodec string
	// VideoCodec is the video codec name. Empty when the media has no video stream.
	VideoCodec string
	// Duration is the media duration. Zero for still images.
	Duration time.Duration
}
