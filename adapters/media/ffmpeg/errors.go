package ffmpeg

import "errors"

// ErrProbeFailed is returned when media probing fails.
var ErrProbeFailed = errors.New("ffmpeg: probe failed")

// ErrTranscodeFailed is returned when media transcoding fails.
var ErrTranscodeFailed = errors.New("ffmpeg: transcode failed")

// ErrToolMissing is returned when a required external tool is missing.
var ErrToolMissing = errors.New("ffmpeg: tool not found")

// ErrInvalidPath is returned when an input or output path is unsafe for
// argv use (empty, starting with `-`, or carrying an unknown extension).
var ErrInvalidPath = errors.New("ffmpeg: invalid path")

// ErrOutputTooLarge is returned when a transcode output exceeds
// DefaultMaxOutputBytes.
var ErrOutputTooLarge = errors.New("ffmpeg: output exceeds size limit")
