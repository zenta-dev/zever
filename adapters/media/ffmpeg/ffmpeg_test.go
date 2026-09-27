package ffmpeg

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/zenta-dev/zever/core/media"
)

func TestIsVideoFormat(t *testing.T) {
	t.Parallel()

	cases := []struct {
		in   string
		want bool
	}{
		{"mp4", true},
		{"webm", true},
		{"MP4", true},
		{".mp4", true},
		{" mp4 ", true},
		{"mp3", false},
		{"jpg", false},
		{"", false},
		{"mkv", false},
	}

	for _, tc := range cases {
		if got := IsVideoFormat(tc.in); got != tc.want {
			t.Errorf("IsVideoFormat(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

func TestIsAudioFormat(t *testing.T) {
	t.Parallel()

	cases := []struct {
		in   string
		want bool
	}{
		{"mp3", true},
		{"aac", true},
		{"m4a", true},
		{"ogg", true},
		{"opus", true},
		{"flac", true},
		{"wav", true},
		{"weba", true},
		{"MP3", true},
		{".wav", true},
		{" wav ", true},
		{"mp4", false},
		{"jpg", false},
		{"", false},
	}

	for _, tc := range cases {
		if got := IsAudioFormat(tc.in); got != tc.want {
			t.Errorf("IsAudioFormat(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

func TestFormatSeconds(t *testing.T) {
	t.Parallel()

	cases := map[time.Duration]string{
		0:                         "0.000",
		1500 * time.Millisecond:   "1.500",
		5 * time.Second:           "5.000",
		90 * time.Second:          "90.000",
		123456 * time.Millisecond: "123.456",
		-time.Second:              "-1.000",
	}

	for in, want := range cases {
		if got := formatSeconds(in); got != want {
			t.Errorf("formatSeconds(%v) = %q, want %q", in, got, want)
		}
	}
}

func TestJpegQ(t *testing.T) {
	t.Parallel()

	cases := map[int]string{
		0:   "8",
		80:  "8",
		100: "2",
		1:   "31",
		50:  "17",
		-10: "8",
		200: "2",
	}

	for in, want := range cases {
		if got := jpegQ(in); got != want {
			t.Errorf("jpegQ(%d) = %q, want %q", in, got, want)
		}
	}
}

func TestEvenDim(t *testing.T) {
	t.Parallel()

	cases := map[int]int{
		0:   -2,
		-3:  -2,
		2:   2,
		3:   2,
		640: 640,
		641: 640,
		481: 480,
		1:   2,
	}

	for in, want := range cases {
		if got := evenDim(in); got != want {
			t.Errorf("evenDim(%d) = %d, want %d", in, got, want)
		}
	}
}

func equalArgs(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}

	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}

	return true
}

func TestBuildArgsMP4Default(t *testing.T) {
	t.Parallel()

	got := BuildArgs("in.mov", "out.mp4", Spec{Kind: media.KindVideo, Format: "mp4"})
	want := []string{"-y", "-i", "in.mov", "-crf", "23", "-c:v", "libx264", "-preset", "veryfast", "-pix_fmt", "yuv420p", "out.mp4"}

	if !equalArgs(got, want) {
		t.Errorf("BuildArgs = %q, want %q", got, want)
	}
}

func TestBuildArgsFormatNormalization(t *testing.T) {
	t.Parallel()

	for _, format := range []string{"MP4", ".mp4", " Mp4 "} {
		got := BuildArgs("in.mov", "out.mp4", Spec{Kind: media.KindVideo, Format: format})
		want := []string{"-y", "-i", "in.mov", "-crf", "23", "-c:v", "libx264", "-preset", "veryfast", "-pix_fmt", "yuv420p", "out.mp4"}

		if !equalArgs(got, want) {
			t.Errorf("BuildArgs(%q) = %q, want %q", format, got, want)
		}
	}
}

func TestBuildArgsScaleBitrateCRF(t *testing.T) {
	t.Parallel()

	got := BuildArgs("in.mov", "out.mp4", Spec{Kind: media.KindVideo, Format: "mp4", Width: 641, Height: 481, Bitrate: 500, CRF: 18})
	want := []string{"-y", "-i", "in.mov", "-vf", "scale=640:480", "-b:a", "500k", "-crf", "18", "-c:v", "libx264", "-preset", "veryfast", "-pix_fmt", "yuv420p", "out.mp4"}

	if !equalArgs(got, want) {
		t.Errorf("BuildArgs = %q, want %q", got, want)
	}
}

func TestBuildArgsScaleAspect(t *testing.T) {
	t.Parallel()

	got := BuildArgs("in.mov", "out.mp4", Spec{Kind: media.KindVideo, Format: "mp4", Width: 320})
	want := []string{"-y", "-i", "in.mov", "-vf", "scale=320:-2", "-crf", "23", "-c:v", "libx264", "-preset", "veryfast", "-pix_fmt", "yuv420p", "out.mp4"}

	if !equalArgs(got, want) {
		t.Errorf("BuildArgs = %q, want %q", got, want)
	}

	got = BuildArgs("in.mov", "out.mp4", Spec{Kind: media.KindVideo, Format: "mp4", Height: 240})
	want = []string{"-y", "-i", "in.mov", "-vf", "scale=-2:240", "-crf", "23", "-c:v", "libx264", "-preset", "veryfast", "-pix_fmt", "yuv420p", "out.mp4"}

	if !equalArgs(got, want) {
		t.Errorf("BuildArgs = %q, want %q", got, want)
	}
}

func TestBuildArgsWebM(t *testing.T) {
	t.Parallel()

	got := BuildArgs("in.mov", "out.webm", Spec{Kind: media.KindVideo, Format: "webm"})
	want := []string{"-y", "-i", "in.mov", "-crf", "23", "-c:v", "libvpx-vp9", "-b:v", "0", "out.webm"}

	if !equalArgs(got, want) {
		t.Errorf("BuildArgs = %q, want %q", got, want)
	}
}

func TestBuildArgsMP3(t *testing.T) {
	t.Parallel()

	got := BuildArgs("in.wav", "out.mp3", Spec{Kind: media.KindAudio, Format: "mp3", Bitrate: 192})
	want := []string{"-y", "-i", "in.wav", "-b:a", "192k", "-c:a", "libmp3lame", "out.mp3"}

	if !equalArgs(got, want) {
		t.Errorf("BuildArgs = %q, want %q", got, want)
	}
}

func TestBuildArgsWAV(t *testing.T) {
	t.Parallel()

	got := BuildArgs("in.wav", "out.wav", Spec{Kind: media.KindAudio, Format: "wav"})
	want := []string{"-y", "-i", "in.wav", "-c:a", "pcm_s16le", "out.wav"}

	if !equalArgs(got, want) {
		t.Errorf("BuildArgs = %q, want %q", got, want)
	}
}

func TestBuildArgsUnknownFormat(t *testing.T) {
	t.Parallel()

	got := BuildArgs("in.mov", "out.mkv", Spec{Kind: media.KindVideo, Format: "mkv"})
	want := []string{"-y", "-i", "in.mov", "-crf", "23", "out.mkv"}

	if !equalArgs(got, want) {
		t.Errorf("BuildArgs = %q, want %q", got, want)
	}

	got = BuildArgs("in.wav", "out.wav", Spec{Kind: media.KindAudio, Format: ""})
	want = []string{"-y", "-i", "in.wav", "-c:a", "pcm_s16le", "out.wav"}

	if !equalArgs(got, want) {
		t.Errorf("BuildArgs empty format fallback = %q, want %q", got, want)
	}
}

func TestBuildArgsThumbnail(t *testing.T) {
	t.Parallel()

	got := BuildArgs("v.mp4", "t.jpg", Spec{Kind: media.KindVideo, Format: "jpg", Offset: 5 * time.Second, Quality: 90})
	want := []string{"-y", "-ss", "5.000", "-i", "v.mp4", "-frames:v", "1", "-q:v", "5", "t.jpg"}

	if !equalArgs(got, want) {
		t.Errorf("BuildArgs = %q, want %q", got, want)
	}
}

func TestBuildArgsThumbnailDefaults(t *testing.T) {
	t.Parallel()

	got := BuildArgs("v.mp4", "t.jpg", Spec{Kind: media.KindVideo, Format: "jpg"})
	want := []string{"-y", "-ss", "0.000", "-i", "v.mp4", "-frames:v", "1", "-q:v", "8", "t.jpg"}

	if !equalArgs(got, want) {
		t.Errorf("BuildArgs = %q, want %q", got, want)
	}
}

func TestBuildArgsThumbnailScaleClamp(t *testing.T) {
	t.Parallel()

	got := BuildArgs("v.mp4", "t.png", Spec{Kind: media.KindVideo, Format: "png", Width: 321, Offset: 1500 * time.Millisecond, Quality: 200})
	want := []string{"-y", "-ss", "1.500", "-i", "v.mp4", "-frames:v", "1", "-vf", "scale=320:-2", "-q:v", "2", "t.png"}

	if !equalArgs(got, want) {
		t.Errorf("BuildArgs = %q, want %q", got, want)
	}
}

func TestBuildArgsThumbnailFromOutExt(t *testing.T) {
	t.Parallel()

	got := BuildArgs("v.mp4", "t.jpg", Spec{Kind: media.KindVideo})
	want := []string{"-y", "-ss", "0.000", "-i", "v.mp4", "-frames:v", "1", "-q:v", "8", "t.jpg"}

	if !equalArgs(got, want) {
		t.Errorf("BuildArgs = %q, want %q", got, want)
	}
}

func TestBuildArgsAudioKindImageFormatNotThumbnail(t *testing.T) {
	t.Parallel()

	got := BuildArgs("a.mp3", "t.jpg", Spec{Kind: media.KindAudio, Format: "jpg"})
	want := []string{"-y", "-i", "a.mp3", "t.jpg"}

	if !equalArgs(got, want) {
		t.Errorf("BuildArgs = %q, want %q", got, want)
	}
}

func TestProbeJSONNested(t *testing.T) {
	t.Parallel()

	var res ProbeResult
	raw := `{"format":{"format_name":"mov,mp4,m4a,3gp,3g2,mj2","duration":"12.340"},"streams":[{"codec_type":"video","codec_name":"h264","width":640,"height":480},{"codec_type":"audio","codec_name":"aac"}]}`

	if err := res.UnmarshalJSON([]byte(raw)); err != nil {
		t.Fatalf("UnmarshalJSON failed: %v", err)
	}

	if res.FormatName != "mov,mp4,m4a,3gp,3g2,mj2" {
		t.Errorf("FormatName = %q", res.FormatName)
	}

	if res.Duration != 12.34 {
		t.Errorf("Duration = %v, want 12.34", res.Duration)
	}

	if len(res.Streams) != 2 {
		t.Fatalf("Streams len = %d, want 2", len(res.Streams))
	}

	if res.Streams[0].CodecType != "video" || res.Streams[0].CodecName != "h264" || res.Streams[0].Width != 640 || res.Streams[0].Height != 480 {
		t.Errorf("stream 0 = %+v", res.Streams[0])
	}
}

func TestProbeJSONFlatNumeric(t *testing.T) {
	t.Parallel()

	var res ProbeResult
	raw := `{"format_name":"mp3","duration":7.5,"streams":[{"codec_type":"audio","codec_name":"mp3"}]}`

	if err := res.UnmarshalJSON([]byte(raw)); err != nil {
		t.Fatalf("UnmarshalJSON failed: %v", err)
	}

	if res.FormatName != "mp3" || res.Duration != 7.5 || len(res.Streams) != 1 {
		t.Errorf("got %+v", res)
	}
}

func TestProbeJSONMissing(t *testing.T) {
	t.Parallel()

	var res ProbeResult
	if err := res.UnmarshalJSON([]byte(`{}`)); err != nil {
		t.Fatalf("UnmarshalJSON failed: %v", err)
	}

	if res.FormatName != "" || res.Duration != 0 || len(res.Streams) != 0 {
		t.Errorf("got %+v", res)
	}
}

func TestProbeJSONBad(t *testing.T) {
	t.Parallel()

	var res ProbeResult
	if err := res.UnmarshalJSON([]byte(`{oops`)); err == nil {
		t.Error("expected error for bad JSON")
	}
}

func TestProbeJSONInvalidDurations(t *testing.T) {
	t.Parallel()

	for _, raw := range []string{
		`{"format_name":"x","duration":"abc"}`,
		`{"format_name":"x","duration":true}`,
		`{"format_name":"x","duration":{}}`,
	} {
		var res ProbeResult
		if err := res.UnmarshalJSON([]byte(raw)); err == nil {
			t.Errorf("expected error for %s", raw)
		}
	}
}

func TestProbeJSONEmptyDuration(t *testing.T) {
	t.Parallel()

	var res ProbeResult
	if err := res.UnmarshalJSON([]byte(`{"format_name":"x","duration":""}`)); err != nil {
		t.Fatalf("UnmarshalJSON failed: %v", err)
	}

	if res.Duration != 0 {
		t.Errorf("Duration = %v, want 0", res.Duration)
	}
}

func TestProbeStartDenied(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "noexec")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}

	_, err := Probe(t.Context(), path, "in.mp3")
	if !errors.Is(err, ErrProbeFailed) {
		t.Fatalf("expected ErrProbeFailed, got %v", err)
	}
}

func TestToProbeVideoAudio(t *testing.T) {
	t.Parallel()

	res := ProbeResult{
		FormatName: "mov,mp4",
		Duration:   12.34,
		Streams: []ProbeStream{
			{CodecType: "video", CodecName: "h264", Width: 640, Height: 480},
			{CodecType: "audio", CodecName: "aac"},
		},
	}
	got := res.ToProbe()

	if got.Kind != media.KindVideo || got.Format != "mov,mp4" || got.Width != 640 || got.Height != 480 {
		t.Errorf("got %+v", got)
	}

	if got.VideoCodec != "h264" || got.AudioCodec != "aac" {
		t.Errorf("got %+v", got)
	}

	if got.Duration != time.Duration(12.34*float64(time.Second)) {
		t.Errorf("Duration = %v", got.Duration)
	}
}

func TestToProbeAudioOnly(t *testing.T) {
	t.Parallel()

	res := ProbeResult{
		FormatName: "mp3",
		Duration:   7.5,
		Streams:    []ProbeStream{{CodecType: "audio", CodecName: "mp3"}},
	}
	got := res.ToProbe()

	if got.Kind != media.KindAudio || got.AudioCodec != "mp3" || got.VideoCodec != "" {
		t.Errorf("got %+v", got)
	}

	if got.Width != 0 || got.Height != 0 {
		t.Errorf("got %+v", got)
	}
}

func TestToProbeEmpty(t *testing.T) {
	t.Parallel()

	got := ProbeResult{}.ToProbe()
	if got.Kind != "" || got.Format != "" || got.Duration != 0 {
		t.Errorf("got %+v", got)
	}

	if got.VideoCodec != "" || got.AudioCodec != "" {
		t.Errorf("got %+v", got)
	}
}

func TestToProbeVideoOnly(t *testing.T) {
	t.Parallel()

	res := ProbeResult{
		FormatName: "png",
		Streams:    []ProbeStream{{CodecType: "video", CodecName: "png", Width: 100, Height: 50}},
	}
	got := res.ToProbe()

	if got.Kind != media.KindVideo || got.VideoCodec != "png" || got.AudioCodec != "" {
		t.Errorf("got %+v", got)
	}
}

func TestLookPathFound(t *testing.T) {
	t.Parallel()

	got, err := LookPath("sh")
	if err != nil {
		t.Fatalf("LookPath failed: %v", err)
	}

	if got == "" {
		t.Error("empty path")
	}
}

func TestLookPathMissing(t *testing.T) {
	t.Parallel()

	_, err := LookPath("zever-no-such-tool-xyz")
	if !errors.Is(err, ErrToolMissing) {
		t.Errorf("expected ErrToolMissing, got %v", err)
	}

	if !errors.Is(err, exec.ErrNotFound) {
		t.Errorf("expected exec.ErrNotFound chain, got %v", err)
	}
}

func writeScript(t *testing.T, body string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "fakebin")

	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o755)
	if err != nil {
		t.Fatalf("create script: %v", err)
	}

	if _, err := file.WriteString(body); err != nil {
		t.Fatalf("write script: %v", err)
	}

	if err := file.Sync(); err != nil {
		t.Fatalf("sync script: %v", err)
	}

	if err := file.Close(); err != nil {
		t.Fatalf("close script: %v", err)
	}

	if err := os.Chmod(path, 0o755); err != nil {
		t.Fatalf("chmod script: %v", err)
	}

	return path
}

func TestProbeFakeSuccess(t *testing.T) {
	t.Parallel()

	bin := writeScript(t, "#!/bin/sh\ncat <<'EOF'\n{\"format\":{\"format_name\":\"mp3\",\"duration\":\"7.5\"},\"streams\":[{\"codec_type\":\"audio\",\"codec_name\":\"mp3\"}]}\nEOF\n")
	res, err := Probe(t.Context(), bin, "in.mp3")

	if err != nil {
		t.Fatalf("Probe failed: %v", err)
	}

	if res.FormatName != "mp3" || res.Duration != 7.5 {
		t.Errorf("got %+v", res)
	}

	if len(res.Streams) != 1 || res.Streams[0].CodecName != "mp3" {
		t.Errorf("got %+v", res)
	}
}

func TestProbeFakeExitFailure(t *testing.T) {
	t.Parallel()

	bin := writeScript(t, "#!/bin/sh\necho 'fake ffprobe boom' >&2\nexit 1\n")
	_, err := Probe(t.Context(), bin, "in.mp3")

	if !errors.Is(err, ErrProbeFailed) {
		t.Fatalf("expected ErrProbeFailed, got %v", err)
	}

	if !strings.Contains(err.Error(), "fake ffprobe boom") {
		t.Errorf("missing stderr snippet: %v", err)
	}
}

func TestProbeFakeBadJSON(t *testing.T) {
	t.Parallel()

	bin := writeScript(t, "#!/bin/sh\necho 'not json'\n")
	_, err := Probe(t.Context(), bin, "in.mp3")

	if !errors.Is(err, ErrProbeFailed) {
		t.Fatalf("expected ErrProbeFailed, got %v", err)
	}
}

func TestProbeFakeOverCap(t *testing.T) {
	t.Parallel()

	bin := writeScript(t, "#!/bin/sh\nhead -c 1200000 /dev/zero | tr '\\0' 'x'\n")
	_, err := Probe(t.Context(), bin, "in.mp3")

	if !errors.Is(err, ErrProbeFailed) {
		t.Fatalf("expected ErrProbeFailed, got %v", err)
	}
}

func TestProbeFakeBigStderr(t *testing.T) {
	t.Parallel()

	bin := writeScript(t, "#!/bin/sh\nhead -c 8192 /dev/zero | tr '\\0' 'e' >&2\nexit 1\n")
	_, err := Probe(t.Context(), bin, "in.mp3")

	if !errors.Is(err, ErrProbeFailed) {
		t.Fatalf("expected ErrProbeFailed, got %v", err)
	}

	if len(err.Error()) > 8192 {
		t.Errorf("stderr snippet not capped: %d bytes", len(err.Error()))
	}
}

func TestProbeMissingBinary(t *testing.T) {
	t.Parallel()

	_, err := Probe(t.Context(), filepath.Join(t.TempDir(), "no-such-ffprobe"), "in.mp3")
	if !errors.Is(err, ErrToolMissing) {
		t.Errorf("expected ErrToolMissing, got %v", err)
	}
}

func TestRunTranscodeFakeSuccess(t *testing.T) {
	t.Parallel()

	bin := writeScript(t, "#!/bin/sh\nexit 0\n")
	argv := BuildArgs("in.wav", "out.mp3", Spec{Kind: media.KindAudio, Format: "mp3"})

	if err := RunTranscode(t.Context(), bin, argv); err != nil {
		t.Fatalf("RunTranscode failed: %v", err)
	}
}

func TestRunTranscodeFakeFailure(t *testing.T) {
	t.Parallel()

	bin := writeScript(t, "#!/bin/sh\necho 'fake ffmpeg boom' >&2\nexit 1\n")
	argv := BuildArgs("in.wav", "out.mp3", Spec{Kind: media.KindAudio, Format: "mp3"})
	err := RunTranscode(t.Context(), bin, argv)

	if !errors.Is(err, ErrTranscodeFailed) {
		t.Fatalf("expected ErrTranscodeFailed, got %v", err)
	}

	if !strings.Contains(err.Error(), "fake ffmpeg boom") {
		t.Errorf("missing stderr snippet: %v", err)
	}
}

func TestRunTranscodeFakeBigStderr(t *testing.T) {
	t.Parallel()

	bin := writeScript(t, "#!/bin/sh\nhead -c 8192 /dev/zero | tr '\\0' 'e' >&2\nexit 1\n")
	argv := BuildArgs("in.wav", "out.mp3", Spec{Kind: media.KindAudio, Format: "mp3"})
	err := RunTranscode(t.Context(), bin, argv)

	if !errors.Is(err, ErrTranscodeFailed) {
		t.Fatalf("expected ErrTranscodeFailed, got %v", err)
	}

	if len(err.Error()) > 8192 {
		t.Errorf("stderr snippet not capped: %d bytes", len(err.Error()))
	}
}

func TestRunTranscodeMissingBinary(t *testing.T) {
	t.Parallel()

	argv := BuildArgs("in.wav", "out.mp3", Spec{Kind: media.KindAudio, Format: "mp3"})
	err := RunTranscode(t.Context(), filepath.Join(t.TempDir(), "no-such-ffmpeg"), argv)

	if !errors.Is(err, ErrToolMissing) {
		t.Errorf("expected ErrToolMissing, got %v", err)
	}
}

func TestE2ESineToMP3(t *testing.T) {
	ffmpegBin, err := LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg not found")
	}

	ffprobeBin, err := LookPath("ffprobe")
	if err != nil {
		t.Skip("ffprobe not found")
	}

	ctx := t.Context()
	dir := t.TempDir()
	src := filepath.Join(dir, "sine.wav")
	out := filepath.Join(dir, "sine.mp3")

	gen := []string{"-y", "-f", "lavfi", "-i", "sine=frequency=440:duration=1", "-c:a", "pcm_s16le", src}
	if err = RunTranscode(ctx, ffmpegBin, gen); err != nil {
		t.Fatalf("generate sine wav: %v", err)
	}

	argv := BuildArgs(src, out, Spec{Kind: media.KindAudio, Format: "mp3", Bitrate: 128})
	if err = RunTranscode(ctx, ffmpegBin, argv); err != nil {
		t.Fatalf("RunTranscode: %v", err)
	}

	res, err := Probe(ctx, ffprobeBin, out)
	if err != nil {
		t.Fatalf("Probe: %v", err)
	}

	got := res.ToProbe()
	if got.Kind != media.KindAudio {
		t.Errorf("Kind = %q, want audio", got.Kind)
	}

	if got.AudioCodec != "mp3" {
		t.Errorf("AudioCodec = %q, want mp3", got.AudioCodec)
	}

	if !strings.Contains(res.FormatName, "mp3") {
		t.Errorf("FormatName = %q, want mp3 container", res.FormatName)
	}

	if got.Duration <= 0 {
		t.Errorf("Duration = %v, want positive", got.Duration)
	}
}
