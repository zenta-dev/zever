package ffmpeg

import (
	"errors"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/zenta-dev/zever/core/media"
)

// TestConformance proves the ffmpeg toolkit honors the CLI contract the media
// battery suite relies on: BuildArgs argv shape, Probe ffprobe-JSON parsing,
// RunTranscode error mapping, and LookPath resolution. Deterministic; fake
// shell binaries stand in for ffmpeg/ffprobe; no network.
func TestConformance(t *testing.T) {
	t.Parallel()

	t.Run("BuildArgs", conformanceBuildArgs)
	t.Run("Probe", conformanceProbe)
	t.Run("RunTranscode", conformanceRunTranscode)
	t.Run("LookPath", conformanceLookPath)
}

func conformanceBuildArgs(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		in   string
		out  string
		spec Spec
		want []string
	}{
		{
			name: "video mp4 default",
			in:   "in.mov",
			out:  "out.mp4",
			spec: Spec{Kind: media.KindVideo, Format: "mp4"},
			want: []string{"-y", "-i", "in.mov", "-crf", "23", "-c:v", "libx264", "-preset", "veryfast", "-pix_fmt", "yuv420p", "out.mp4"},
		},
		{
			name: "video webm",
			in:   "in.mov",
			out:  "out.webm",
			spec: Spec{Kind: media.KindVideo, Format: "webm"},
			want: []string{"-y", "-i", "in.mov", "-crf", "23", "-c:v", "libvpx-vp9", "-b:v", "0", "out.webm"},
		},
		{
			name: "video scale bitrate crf",
			in:   "in.mov",
			out:  "out.mp4",
			spec: Spec{Kind: media.KindVideo, Format: "mp4", Width: 641, Height: 481, Bitrate: 500, CRF: 18},
			want: []string{"-y", "-i", "in.mov", "-vf", "scale=640:480", "-b:a", "500k", "-crf", "18", "-c:v", "libx264", "-preset", "veryfast", "-pix_fmt", "yuv420p", "out.mp4"},
		},
		{
			name: "audio mp3 bitrate",
			in:   "in.wav",
			out:  "out.mp3",
			spec: Spec{Kind: media.KindAudio, Format: "mp3", Bitrate: 192},
			want: []string{"-y", "-i", "in.wav", "-b:a", "192k", "-c:a", "libmp3lame", "out.mp3"},
		},
		{
			name: "audio wav default",
			in:   "in.wav",
			out:  "out.wav",
			spec: Spec{Kind: media.KindAudio, Format: "wav"},
			want: []string{"-y", "-i", "in.wav", "-c:a", "pcm_s16le", "out.wav"},
		},
		{
			name: "thumbnail offset quality scale",
			in:   "v.mp4",
			out:  "t.jpg",
			spec: Spec{Kind: media.KindVideo, Format: "jpg", Offset: 5 * time.Second, Quality: 90, Width: 321},
			want: []string{"-y", "-ss", "5.000", "-i", "v.mp4", "-frames:v", "1", "-vf", "scale=320:-2", "-q:v", "5", "t.jpg"},
		},
		{
			name: "thumbnail from out ext",
			in:   "v.mp4",
			out:  "t.png",
			spec: Spec{Kind: media.KindVideo},
			want: []string{"-y", "-ss", "0.000", "-i", "v.mp4", "-frames:v", "1", "-q:v", "8", "t.png"},
		},
		{
			name: "unknown format no codec args",
			in:   "in.mov",
			out:  "out.mkv",
			spec: Spec{Kind: media.KindVideo, Format: "mkv"},
			want: []string{"-y", "-i", "in.mov", "-crf", "23", "out.mkv"},
		},
		{
			name: "audio kind image ext passthrough",
			in:   "a.mp3",
			out:  "t.jpg",
			spec: Spec{Kind: media.KindAudio, Format: "jpg"},
			want: []string{"-y", "-i", "a.mp3", "t.jpg"},
		},
		{
			name: "empty paths still valid",
			in:   "",
			out:  "",
			spec: Spec{},
			want: []string{"-y", "-i", "", ""},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := BuildArgs(tc.in, tc.out, tc.spec)

			if !equalArgs(got, tc.want) {
				t.Errorf("BuildArgs(%q, %q, %+v) = %q, want %q", tc.in, tc.out, tc.spec, got, tc.want)
			}
		})
	}

	t.Run("format normalization", func(t *testing.T) {
		t.Parallel()

		want := []string{"-y", "-i", "in.mov", "-crf", "23", "-c:v", "libx264", "-preset", "veryfast", "-pix_fmt", "yuv420p", "out.mp4"}

		for _, format := range []string{"MP4", ".mp4", " Mp4 "} {
			if got := BuildArgs("in.mov", "out.mp4", Spec{Kind: media.KindVideo, Format: format}); !equalArgs(got, want) {
				t.Errorf("BuildArgs(format %q) = %q, want %q", format, got, want)
			}
		}
	})
}

func conformanceProbe(t *testing.T) {
	t.Parallel()

	t.Run("nested format precedence", func(t *testing.T) {
		t.Parallel()

		var res ProbeResult
		raw := `{"format_name":"flat","duration":1.0,"format":{"format_name":"nested","duration":"2.5"}}`

		if err := res.UnmarshalJSON([]byte(raw)); err != nil {
			t.Fatalf("UnmarshalJSON() error = %v", err)
		}

		if res.FormatName != "nested" {
			t.Errorf("FormatName = %q, want nested", res.FormatName)
		}

		if res.Duration != 2.5 {
			t.Errorf("Duration = %v, want 2.5", res.Duration)
		}
	})

	t.Run("flat numeric duration", func(t *testing.T) {
		t.Parallel()

		var res ProbeResult
		raw := `{"format_name":"mp3","duration":7.5,"streams":[{"codec_type":"audio","codec_name":"mp3"}]}`

		if err := res.UnmarshalJSON([]byte(raw)); err != nil {
			t.Fatalf("UnmarshalJSON() error = %v", err)
		}

		if res.FormatName != "mp3" || res.Duration != 7.5 || len(res.Streams) != 1 {
			t.Errorf("got %+v", res)
		}
	})

	t.Run("missing duration zero", func(t *testing.T) {
		t.Parallel()

		var res ProbeResult
		if err := res.UnmarshalJSON([]byte(`{"format_name":"x"}`)); err != nil {
			t.Fatalf("UnmarshalJSON() error = %v", err)
		}

		if res.Duration != 0 {
			t.Errorf("Duration = %v, want 0", res.Duration)
		}
	})

	t.Run("invalid duration", func(t *testing.T) {
		t.Parallel()

		var res ProbeResult
		if err := res.UnmarshalJSON([]byte(`{"format_name":"x","duration":"abc"}`)); err == nil {
			t.Error("UnmarshalJSON() error = nil, want parse error")
		}
	})

	t.Run("malformed json", func(t *testing.T) {
		t.Parallel()

		var res ProbeResult
		if err := res.UnmarshalJSON([]byte(`{oops`)); err == nil {
			t.Error("UnmarshalJSON() error = nil, want syntax error")
		}
	})

	t.Run("tool missing", func(t *testing.T) {
		t.Parallel()

		_, err := Probe(t.Context(), t.TempDir()+"/no-such-ffprobe", "in.mp3")
		if !errors.Is(err, ErrToolMissing) {
			t.Errorf("Probe() err = %v, want ErrToolMissing", err)
		}
	})

	t.Run("exit failure wraps stderr", func(t *testing.T) {
		t.Parallel()

		bin := writeScript(t, "#!/bin/sh\necho 'conformance probe boom' >&2\nexit 1\n")

		_, err := Probe(t.Context(), bin, "in.mp3")
		if !errors.Is(err, ErrProbeFailed) {
			t.Fatalf("Probe() err = %v, want ErrProbeFailed", err)
		}

		if !strings.Contains(err.Error(), "conformance probe boom") {
			t.Errorf("err = %v, want stderr snippet", err)
		}
	})

	t.Run("stdout over cap", func(t *testing.T) {
		t.Parallel()

		bin := writeScript(t, "#!/bin/sh\nhead -c 1200000 /dev/zero | tr '\\0' 'x'\n")

		_, err := Probe(t.Context(), bin, "in.mp3")
		if !errors.Is(err, ErrProbeFailed) {
			t.Errorf("Probe() err = %v, want ErrProbeFailed", err)
		}
	})

	t.Run("stderr snippet capped", func(t *testing.T) {
		t.Parallel()

		bin := writeScript(t, "#!/bin/sh\nhead -c 8192 /dev/zero | tr '\\0' 'e' >&2\nexit 1\n")

		_, err := Probe(t.Context(), bin, "in.mp3")
		if !errors.Is(err, ErrProbeFailed) {
			t.Fatalf("Probe() err = %v, want ErrProbeFailed", err)
		}

		if len(err.Error()) > 8192 {
			t.Errorf("stderr snippet not capped: %d bytes", len(err.Error()))
		}
	})
}

func conformanceRunTranscode(t *testing.T) {
	t.Parallel()

	argv := BuildArgs("in.wav", "out.mp3", Spec{Kind: media.KindAudio, Format: "mp3"})

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		bin := writeScript(t, "#!/bin/sh\nexit 0\n")

		if err := RunTranscode(t.Context(), bin, argv); err != nil {
			t.Errorf("RunTranscode() err = %v, want nil", err)
		}
	})

	t.Run("failure wraps stderr", func(t *testing.T) {
		t.Parallel()

		bin := writeScript(t, "#!/bin/sh\necho 'conformance transcode boom' >&2\nexit 1\n")

		err := RunTranscode(t.Context(), bin, argv)
		if !errors.Is(err, ErrTranscodeFailed) {
			t.Fatalf("RunTranscode() err = %v, want ErrTranscodeFailed", err)
		}

		if !strings.Contains(err.Error(), "conformance transcode boom") {
			t.Errorf("err = %v, want stderr snippet", err)
		}
	})

	t.Run("tool missing", func(t *testing.T) {
		t.Parallel()

		err := RunTranscode(t.Context(), t.TempDir()+"/no-such-ffmpeg", argv)
		if !errors.Is(err, ErrToolMissing) {
			t.Errorf("RunTranscode() err = %v, want ErrToolMissing", err)
		}
	})
}

func conformanceLookPath(t *testing.T) {
	t.Parallel()

	t.Run("found", func(t *testing.T) {
		t.Parallel()

		got, err := LookPath("sh")
		if err != nil {
			t.Fatalf("LookPath() error = %v", err)
		}

		if got == "" {
			t.Error("LookPath() = empty, want non-empty")
		}
	})

	t.Run("missing", func(t *testing.T) {
		t.Parallel()

		_, err := LookPath("zever-no-such-tool-xyz")
		if !errors.Is(err, ErrToolMissing) {
			t.Errorf("LookPath() err = %v, want ErrToolMissing", err)
		}

		if !errors.Is(err, exec.ErrNotFound) {
			t.Errorf("LookPath() err = %v, want exec.ErrNotFound chain", err)
		}
	})
}
