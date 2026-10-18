package media

import (
	"errors"
	"net/url"
	"time"
)

const (
	// DefaultMaxDownloadBytes is the default byte limit for downloads.
	DefaultMaxDownloadBytes = 64 << 20
	// DefaultDerivedTTL is the default time-to-live for derived assets.
	DefaultDerivedTTL = 10 * time.Minute
	// DefaultMaxPixels is the default pixel-count limit for decoded images.
	DefaultMaxPixels = 50 * 1024 * 1024
	// DefaultPresignTTL is the default time-to-live for presigned URLs.
	DefaultPresignTTL = time.Hour
	// MaxPresignTTL is the maximum time-to-live for presigned URLs.
	MaxPresignTTL = 7 * 24 * time.Hour
	// DefaultLocalRoot is the default local storage root directory.
	DefaultLocalRoot = "/tmp/media"
	// DefaultLocalBaseURL is the default base URL for locally served assets.
	DefaultLocalBaseURL = "/media"
	// DefaultS3Region is the default S3 region.
	DefaultS3Region = "us-east-1"
	// DefaultMaxDuration is the default maximum media duration. Zero means unlimited.
	DefaultMaxDuration = 0
	// DefaultFFmpeg is the default ffmpeg binary name.
	DefaultFFmpeg = "ffmpeg"
	// DefaultFFProbe is the default ffprobe binary name.
	DefaultFFProbe = "ffprobe"
)

// Options configures media backend selection and limits.
type Options struct {
	// Root is the local storage root directory.
	Root string `json:"root"`
	// BaseURL is the base URL for served assets.
	BaseURL string `json:"base_url"`
	// MaxDownloadBytes bounds a single download in bytes.
	MaxDownloadBytes int64 `json:"max_download_bytes"`
	// MaxPixels bounds the decoded image pixel count.
	MaxPixels int64 `json:"max_pixels"`
	// DerivedTTL is the time-to-live for derived assets.
	DerivedTTL time.Duration `json:"derived_ttl"`
	// PresignTTL is the time-to-live for presigned URLs.
	PresignTTL time.Duration `json:"presign_ttl"`
	// MaxDuration bounds media duration. Zero means unlimited.
	MaxDuration time.Duration `json:"max_duration"`
	// Endpoint holds the optional custom storage endpoint URL.
	Endpoint string `json:"endpoint"`
	// Region is the storage region.
	Region string `json:"region"`
	// Bucket is the storage bucket name.
	Bucket string `json:"bucket"`
	// AccessKeyID holds the storage access key. It is never logged.
	AccessKeyID string `json:"access_key_id" zever:"secret"`
	// SecretAccessKey holds the storage secret key. It is never logged.
	SecretAccessKey string `json:"secret_access_key" zever:"secret"`
	// FFmpeg is the ffmpeg binary path.
	FFmpeg string `json:"ffmpeg"`
	// FFProbe is the ffprobe binary path.
	FFProbe string `json:"ffprobe"`
}

// Validate checks options for consistency, joining all violations.
func (o Options) Validate() error {
	var errs []error

	if o.MaxDownloadBytes < 0 {
		errs = append(errs, InvalidOptionsError{Reason: "max_download_bytes must be >= 0"})
	}

	if o.MaxPixels < 0 {
		errs = append(errs, InvalidOptionsError{Reason: "max_pixels must be >= 0"})
	}

	if o.DerivedTTL < 0 {
		errs = append(errs, InvalidOptionsError{Reason: "derived_ttl must be >= 0"})
	}

	if o.MaxDuration < 0 {
		errs = append(errs, InvalidOptionsError{Reason: "max_duration must be >= 0"})
	}

	if o.PresignTTL < 0 || o.PresignTTL > MaxPresignTTL {
		errs = append(errs, InvalidOptionsError{Reason: "presign_ttl must be >= 0 and <= 7 days"})
	}

	if o.Endpoint != "" {
		u, err := url.Parse(o.Endpoint)
		if err != nil {
			errs = append(errs, InvalidOptionsError{Reason: "endpoint must be a valid url"})
		} else {
			if u.Scheme == "" {
				errs = append(errs, InvalidOptionsError{Reason: "endpoint must include scheme"})
			}

			if u.Host == "" {
				errs = append(errs, InvalidOptionsError{Reason: "endpoint must include host"})
			}
		}
	}

	return errors.Join(errs...)
}
