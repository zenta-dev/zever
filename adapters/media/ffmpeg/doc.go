// Package ffmpeg compiles fixed ffmpeg/ffprobe argument vectors and runs them without a shell.
//
// Security: paths are untrusted input. BuildArgs always emits `--` before
// the output path so a leading-dash name cannot parse as a flag; Probe and
// RunTranscode reject flag-looking inputs/outputs before exec. Untrusted
// upload names must additionally pass ValidatePath (known media extension
// allowlist via media.KindForExt): RunTranscode cannot allowlist from the
// argv alone because legitimate filter inputs (e.g. lavfi `sine=...`) carry
// no extension. Runs are bounded by DefaultProbeTimeout /
// DefaultTranscodeTimeout when the caller sets no deadline, and successful
// transcode outputs are checked against DefaultMaxOutputBytes;
// operators serving untrusted uploads should also enforce directory quotas
// at the filesystem level.
package ffmpeg
