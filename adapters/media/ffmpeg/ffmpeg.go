package ffmpeg

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/zenta-dev/zever/core/media"
	"github.com/zenta-dev/zever/shared/codec"
)

// probeCodec decodes ffprobe JSON output into ProbeResult. ProbeResult's
// custom UnmarshalJSON is invoked identically under encoding/json/v2 (used
// by codec.JSONCodec) and encoding/json v1, verified by decoding sample
// ffprobe output through both and comparing the results.
var probeCodec = codec.JSONCodec[ProbeResult]{}

const (
	// maxProbeOutput caps ffprobe stdout at 1 MiB.
	maxProbeOutput = 1 << 20
	// maxErrSnippet caps stderr text attached to errors at 4 KiB.
	maxErrSnippet = 4096
	// maxExecAttempts bounds retries of transient text-file-busy exec failures.
	maxExecAttempts = 5
	// execRetryBackoff is the delay between text-file-busy retries, giving
	// the kernel time to release the exec-in-progress lock under CI load
	// instead of re-attempting immediately into the same busy window.
	execRetryBackoff = 5 * time.Millisecond
	// DefaultProbeTimeout bounds ffprobe runs when the caller's context
	// carries no deadline. Callers with their own deadline keep it.
	DefaultProbeTimeout = 30 * time.Second
	// DefaultTranscodeTimeout bounds ffmpeg runs when the caller's context
	// carries no deadline. A hostile input must not run forever on the
	// operator's default path; pass an explicit deadline for longer jobs.
	DefaultTranscodeTimeout = 10 * time.Minute
	// DefaultMaxOutputBytes caps the transcode output file size checked
	// after a successful run. Operators serving untrusted uploads should
	// additionally enforce directory quotas at the filesystem level.
	DefaultMaxOutputBytes = 1 << 30
)

// Spec describes an ffmpeg transcode or thumbnail extraction.
type Spec struct {
	// Kind selects the media pipeline.
	Kind media.MediaKind
	// Format is the target container or image format, lowercased without a dot.
	// Empty derives the format from the output path extension.
	Format string
	// Width is the target width in pixels. Zero keeps the source width.
	Width int
	// Height is the target height in pixels. Zero keeps the source height.
	Height int
	// Bitrate is the audio target bitrate in kbps. Zero keeps the default.
	Bitrate int
	// CRF is the video constant-rate-factor. Zero selects 23.
	CRF int
	// Quality is the thumbnail JPEG quality, 1-100. Zero selects 80.
	Quality int
	// Offset is the seek position for video thumbnail extraction.
	// It applies to video thumbnails only.
	Offset time.Duration
}

// VideoFormats maps a container format to its fixed video codec arguments.
var VideoFormats = map[string][]string{
	"mp4":  {"-c:v", "libx264", "-preset", "veryfast", "-pix_fmt", "yuv420p"},
	"webm": {"-c:v", "libvpx-vp9", "-b:v", "0"},
}

// AudioFormats maps a container format to its fixed audio codec arguments.
var AudioFormats = map[string][]string{
	"mp3":  {"-c:a", "libmp3lame"},
	"aac":  {"-c:a", "aac"},
	"m4a":  {"-c:a", "aac"},
	"ogg":  {"-c:a", "libopus"},
	"opus": {"-c:a", "libopus"},
	"flac": {"-c:a", "flac"},
	"wav":  {"-c:a", "pcm_s16le"},
	"weba": {"-c:a", "libopus"},
}

// IsVideoFormat reports whether f names a supported video container.
func IsVideoFormat(f string) bool {
	_, ok := VideoFormats[normalizeFormat(f)]

	return ok
}

// IsAudioFormat reports whether f names a supported audio container.
func IsAudioFormat(f string) bool {
	_, ok := AudioFormats[normalizeFormat(f)]

	return ok
}

// BuildArgs compiles the fixed ffmpeg argument vector for in to out per s.
// It performs no I/O. Video output in an image format extracts a thumbnail.
func BuildArgs(in, out string, s Spec) []string {
	format := normalizeFormat(s.Format)
	if format == "" {
		format = normalizeFormat(filepath.Ext(out))
	}

	if s.Kind == media.KindVideo && isImageFormat(format) {
		return buildThumbnailArgs(in, out, s)
	}

	return buildAVArgs(in, out, s, format)
}

func buildThumbnailArgs(in, out string, s Spec) []string {
	argv := []string{"-y", "-ss", formatSeconds(s.Offset), "-i", in, "-frames:v", "1"}
	if s.Width > 0 || s.Height > 0 {
		argv = append(argv, "-vf", scaleFilter(s.Width, s.Height))
	}

	return append(argv, "-q:v", jpegQ(s.Quality), "--", out)
}

func buildAVArgs(in, out string, s Spec, format string) []string {
	argv := []string{"-y", "-i", in}
	if s.Width > 0 || s.Height > 0 {
		argv = append(argv, "-vf", scaleFilter(s.Width, s.Height))
	}

	if s.Bitrate > 0 {
		argv = append(argv, "-b:a", strconv.Itoa(s.Bitrate)+"k")
	}

	if s.Kind == media.KindVideo {
		crf := s.CRF
		if crf == 0 {
			crf = 23
		}

		argv = append(argv, "-crf", strconv.Itoa(crf))
	}

	// The output is always preceded by `--` so a leading-dash name cannot
	// parse as a flag. The input follows `-i` literally and needs no
	// separator; leading-dash inputs are rejected by ValidatePath at the
	// Probe/RunTranscode entry points instead.
	argv = append(argv, codecArgs(format)...)
	return append(argv, "--", out)
}

func codecArgs(format string) []string {
	if args, ok := VideoFormats[format]; ok {
		return args
	}

	if args, ok := AudioFormats[format]; ok {
		return args
	}

	return nil
}

func isImageFormat(format string) bool {
	kind, ok := media.KindForExt(format)

	return ok && kind == media.KindImage
}

func scaleFilter(w, h int) string {
	return fmt.Sprintf("scale=%d:%d", evenDim(w), evenDim(h))
}

// evenDim snaps a dimension to even, mapping unset to -2 for aspect scaling.
func evenDim(d int) int {
	if d <= 0 {
		return -2
	}

	v := d &^ 1
	if v <= 0 {
		return 2
	}

	return v
}

// formatSeconds renders d as seconds with millisecond precision.
func formatSeconds(d time.Duration) string {
	return strconv.FormatFloat(d.Seconds(), 'f', 3, 64)
}

// jpegQ maps a 1-100 quality to the ffmpeg -q:v scale of 2-31.
// Zero selects the default quality of 80.
func jpegQ(quality int) string {
	q := quality
	if q <= 0 {
		q = 80
	}

	v := 32 - q*30/100
	if v < 2 {
		v = 2
	}

	if v > 31 {
		v = 31
	}

	return strconv.Itoa(v)
}

func normalizeFormat(f string) string {
	return strings.TrimPrefix(strings.ToLower(strings.TrimSpace(f)), ".")
}

// ValidatePath checks p for safe use as an ffmpeg/ffprobe path: it must be
// non-empty, must not start with `-` (which would parse as a flag), and
// must carry a known media extension (see media.KindForExt). Pass untrusted
// upload names through this before Probe or a transcode argv; RunTranscode
// cannot allowlist inputs itself because legitimate filter inputs (e.g.
// lavfi `sine=...`) carry no extension.
func ValidatePath(p string) error {
	if strings.TrimSpace(p) == "" {
		return fmt.Errorf("%w: path is required", ErrInvalidPath)
	}
	if strings.HasPrefix(p, "-") {
		return fmt.Errorf("%w: path must not start with '-'", ErrInvalidPath)
	}
	if _, ok := media.KindForExt(normalizeFormat(filepath.Ext(p))); !ok {
		return fmt.Errorf("%w: unknown extension in %q", ErrInvalidPath, p)
	}
	return nil
}

// withDefaultTimeout wraps ctx with d when the caller set no deadline, so a
// runaway binary cannot outlive the operator's intent on the default path.
// Callers with their own deadline or timeout keep it untouched.
func withDefaultTimeout(ctx context.Context, d time.Duration) (context.Context, context.CancelFunc) {
	if _, ok := ctx.Deadline(); ok {
		return ctx, func() {}
	}
	return context.WithTimeout(ctx, d)
}

// validateTranscodeArgs fails closed on flag-looking inputs/outputs without
// spawning the binary. The input is the element after the first `-i` when
// present; the output is the final element. Extension allowlisting stays
// with the caller (see ValidatePath): filter inputs carry no extension.
func validateTranscodeArgs(argv []string) error {
	if len(argv) == 0 {
		return fmt.Errorf("%w: empty argv", ErrTranscodeFailed)
	}
	for i, a := range argv {
		if a == "-i" && i+1 < len(argv) && strings.HasPrefix(argv[i+1], "-") && argv[i+1] != "--" {
			return fmt.Errorf("%w: input must not start with '-'", ErrInvalidPath)
		}
	}
	if out := argv[len(argv)-1]; strings.HasPrefix(out, "-") {
		return fmt.Errorf("%w: output must not start with '-'", ErrInvalidPath)
	}
	return nil
}

// withOutputSeparator returns an argv copy with `--` inserted before the
// final (output) element, unless already present. Vectors built by BuildArgs
// carry the separator; hand-built vectors gain it here.
func withOutputSeparator(argv []string) []string {
	if len(argv) >= 2 && argv[len(argv)-2] == "--" {
		return argv
	}
	out := make([]string, 0, len(argv)+1)
	out = append(out, argv[:len(argv)-1]...)
	return append(out, "--", argv[len(argv)-1])
}

// checkOutputSize enforces limit on the transcode output file (the final
// argv element). Missing files are skipped: stub binaries and non-file
// outputs create none. Oversized outputs fail with ErrOutputTooLarge.
func checkOutputSize(argv []string, limit int64) error {
	if len(argv) == 0 {
		return nil
	}
	out := argv[len(argv)-1]
	if out == "" || out == "--" {
		return nil
	}
	st, err := os.Stat(out)
	if err == nil && st.Size() > limit {
		return fmt.Errorf("%w: %q exceeds %d bytes", ErrOutputTooLarge, out, limit)
	}
	return nil
}

// ProbeStream describes one ffprobe stream.
type ProbeStream struct {
	// CodecType is the stream type, e.g. video or audio.
	CodecType string `json:"codec_type"`
	// CodecName is the codec name, e.g. h264 or aac.
	CodecName string `json:"codec_name"`
	// Width is the frame width in pixels.
	Width int `json:"width"`
	// Height is the frame height in pixels.
	Height int `json:"height"`
}

// ProbeResult is the parsed ffprobe JSON output.
type ProbeResult struct {
	// FormatName is the container format name.
	FormatName string `json:"format_name"`
	// Duration is the media duration in seconds.
	Duration float64 `json:"duration"`
	// Streams holds the probed streams.
	Streams []ProbeStream `json:"streams"`
}

// UnmarshalJSON parses flat probe JSON and nested ffprobe output.
// It accepts durations as numbers or strings.
func (r *ProbeResult) UnmarshalJSON(data []byte) error {
	var raw struct {
		Format *struct {
			FormatName string          `json:"format_name"`
			Duration   json.RawMessage `json:"duration"`
		} `json:"format"`
		FormatName string          `json:"format_name"`
		Duration   json.RawMessage `json:"duration"`
		Streams    []ProbeStream   `json:"streams"`
	}

	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	r.Streams = raw.Streams
	r.FormatName = raw.FormatName

	dur := raw.Duration
	if raw.Format != nil {
		r.FormatName = raw.Format.FormatName
		dur = raw.Format.Duration
	}

	if len(dur) == 0 {
		return nil
	}

	f, err := parseDuration(dur)
	if err != nil {
		return err
	}

	r.Duration = f

	return nil
}

func parseDuration(data []byte) (float64, error) {
	var f float64
	if err := json.Unmarshal(data, &f); err == nil {
		return f, nil
	}

	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return 0, err
	}

	s = strings.TrimSpace(s)
	if s == "" {
		return 0, nil
	}

	return strconv.ParseFloat(s, 64)
}

// ToProbe converts r to a media.Probe.
func (r ProbeResult) ToProbe() media.Probe {
	p := media.Probe{
		Format:   r.FormatName,
		Duration: time.Duration(r.Duration * float64(time.Second)),
	}

	for _, s := range r.Streams {
		switch s.CodecType {
		case "video":
			if p.VideoCodec == "" {
				p.VideoCodec = s.CodecName
				p.Width = s.Width
				p.Height = s.Height
			}
		case "audio":
			if p.AudioCodec == "" {
				p.AudioCodec = s.CodecName
			}
		}
	}

	switch {
	case p.VideoCodec != "":
		p.Kind = media.KindVideo
	case p.AudioCodec != "":
		p.Kind = media.KindAudio
	}

	return p
}

// LookPath resolves name via the process PATH.
func LookPath(name string) (string, error) {
	path, err := exec.LookPath(name)
	if err != nil {
		return "", fmt.Errorf("ffmpeg: tool %q: %w", name, errors.Join(ErrToolMissing, err))
	}

	return path, nil
}

// Probe runs ffprobe and parses its JSON output. Stdout is capped at 1 MiB.
// Transient text-file-busy start failures are retried. The path is
// validated (see ValidatePath) and passed after `--`; when the caller's
// context carries no deadline, DefaultProbeTimeout applies.
func Probe(ctx context.Context, ffprobe, path string) (ProbeResult, error) {
	var res ProbeResult

	if err := ValidatePath(path); err != nil {
		return res, err
	}

	ctx, cancel := withDefaultTimeout(ctx, DefaultProbeTimeout)
	defer cancel()

	var stderr bytes.Buffer

	var cmd *exec.Cmd

	var stdout io.ReadCloser

	var err error

	for range maxExecAttempts {
		cmd = exec.CommandContext(ctx, ffprobe, "-v", "error", "-show_format", "-show_streams", "-of", "json", "--", path) // binary resolved via LookPath, argv fixed.
		cmd.Stderr = &stderr
		stdout, _ = cmd.StdoutPipe()
		err = cmd.Start()

		if err == nil || !isTextBusy(err) {
			break
		}

		if sleepErr := sleepCtx(ctx, execRetryBackoff); sleepErr != nil {
			err = sleepErr

			break
		}
	}

	if err != nil {
		if isToolMissing(err) {
			return res, fmt.Errorf("ffmpeg: tool %q: %w", ffprobe, errors.Join(ErrToolMissing, err))
		}

		return res, fmt.Errorf("%w: %q", errors.Join(ErrProbeFailed, err), errSnippet(&stderr))
	}

	data, _ := io.ReadAll(io.LimitReader(stdout, maxProbeOutput+1))

	if len(data) > maxProbeOutput {
		_ = cmd.Process.Kill()
		_ = stdout.Close()
		_ = cmd.Wait()

		return res, fmt.Errorf("%w: output exceeds %d bytes: %q", ErrProbeFailed, maxProbeOutput, errSnippet(&stderr))
	}

	if waitErr := cmd.Wait(); waitErr != nil {
		return res, fmt.Errorf("%w: %q", errors.Join(ErrProbeFailed, waitErr), errSnippet(&stderr))
	}

	res, err = probeCodec.Decode(data)
	if err != nil {
		return res, fmt.Errorf("%w: %q", errors.Join(ErrProbeFailed, err), errSnippet(&stderr))
	}

	return res, nil
}

// RunTranscode runs ffmpeg with the argv vector compiled by BuildArgs.
// Transient text-file-busy start failures are retried. Flag-looking
// inputs/outputs are rejected before exec, `--` is ensured before the
// output, a successful run is checked against DefaultMaxOutputBytes, and
// DefaultTranscodeTimeout applies when the caller's context carries no
// deadline. Callers must allowlist untrusted inputs via ValidatePath:
// filter inputs (e.g. lavfi) carry no extension, so RunTranscode cannot do
// it from the argv alone.
func RunTranscode(ctx context.Context, ffmpegBin string, argv []string) error {
	if err := validateTranscodeArgs(argv); err != nil {
		return err
	}
	argv = withOutputSeparator(argv)

	ctx, cancel := withDefaultTimeout(ctx, DefaultTranscodeTimeout)
	defer cancel()

	var stderr bytes.Buffer

	var err error

	for range maxExecAttempts {
		cmd := exec.CommandContext(ctx, ffmpegBin, argv...) // binary resolved via LookPath, argv fixed.
		cmd.Stderr = &stderr
		err = cmd.Run()

		if err == nil || !isTextBusy(err) {
			break
		}

		if sleepErr := sleepCtx(ctx, execRetryBackoff); sleepErr != nil {
			err = sleepErr

			break
		}
	}

	if err != nil {
		if isToolMissing(err) {
			return fmt.Errorf("ffmpeg: tool %q: %w", ffmpegBin, errors.Join(ErrToolMissing, err))
		}

		return fmt.Errorf("%w: %q", errors.Join(ErrTranscodeFailed, err), errSnippet(&stderr))
	}

	if err := checkOutputSize(argv, DefaultMaxOutputBytes); err != nil {
		return err
	}

	return nil
}

// isTextBusy reports whether err is a transient text-file-busy exec failure.
func isTextBusy(err error) bool {
	return errors.Is(err, syscall.ETXTBSY)
}

// sleepCtx waits d or until ctx is done, whichever comes first, so a
// text-file-busy retry backoff can be cancelled mid-wait instead of
// blocking past the caller's deadline.
func sleepCtx(ctx context.Context, d time.Duration) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(d):
		return nil
	}
}

// isToolMissing reports whether err means the binary was not found.
// Explicit paths fail with ENOENT while bare names fail with exec.ErrNotFound.
func isToolMissing(err error) bool {
	return errors.Is(err, exec.ErrNotFound) || errors.Is(err, os.ErrNotExist)
}

func errSnippet(stderr *bytes.Buffer) string {
	s := stderr.String()
	if len(s) > maxErrSnippet {
		s = s[:maxErrSnippet]
	}

	return s
}
