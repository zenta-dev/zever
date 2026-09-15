package ffmpeg

import "errors"

// ErrProbeFailed is returned when media probing fails.
var ErrProbeFailed = errors.New("ffmpeg: probe failed")

// ErrTranscodeFailed is returned when media transcoding fails.
var ErrTranscodeFailed = errors.New("ffmpeg: transcode failed")

// ErrToolMissing is returned when a required external tool is missing.
var ErrToolMissing = errors.New("ffmpeg: tool not found")
