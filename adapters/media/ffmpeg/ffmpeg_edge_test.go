package ffmpeg

import (
	"errors"
	"testing"
)

func TestBuildArgs_emptyPaths(t *testing.T) {
	t.Parallel()

	argv := BuildArgs("", "", Spec{})
	if len(argv) == 0 {
		t.Fatal("BuildArgs returned empty argv")
	}
	if argv[0] != "-y" || argv[1] != "-i" {
		t.Errorf("argv prefix = %v, want [-y -i]", argv[:2])
	}
}

func TestBuildArgs_zeroSpecNoCodecArgs(t *testing.T) {
	t.Parallel()

	argv := BuildArgs("in.dat", "out.dat", Spec{})
	for _, a := range argv {
		if a == "out.dat" {
			return
		}
	}
	t.Errorf("output path missing from argv %v", argv)
}

func TestProbeJSON_nestedPrecedence(t *testing.T) {
	t.Parallel()

	var res ProbeResult
	raw := `{"format_name":"flat","duration":1.0,"format":{"format_name":"nested","duration":"2.5"}}`
	if err := res.UnmarshalJSON([]byte(raw)); err != nil {
		t.Fatalf("UnmarshalJSON err = %v", err)
	}
	if res.FormatName != "nested" {
		t.Errorf("FormatName = %q, want nested", res.FormatName)
	}
	if res.Duration != 2.5 {
		t.Errorf("Duration = %v, want 2.5", res.Duration)
	}
}

func TestProbeJSON_nullDuration(t *testing.T) {
	t.Parallel()

	var res ProbeResult
	if err := res.UnmarshalJSON([]byte(`{"format_name":"x","duration":null}`)); err != nil {
		t.Fatalf("UnmarshalJSON err = %v", err)
	}
	if res.Duration != 0 {
		t.Errorf("Duration = %v, want 0", res.Duration)
	}
}

func TestProbeJSON_whitespaceStringDuration(t *testing.T) {
	t.Parallel()

	var res ProbeResult
	if err := res.UnmarshalJSON([]byte(`{"format_name":"x","duration":" 12.5 "}`)); err != nil {
		t.Fatalf("UnmarshalJSON err = %v", err)
	}
	if res.Duration != 12.5 {
		t.Errorf("Duration = %v, want 12.5", res.Duration)
	}
}

func TestProbeJSON_durationArray(t *testing.T) {
	t.Parallel()

	var res ProbeResult
	if err := res.UnmarshalJSON([]byte(`{"format_name":"x","duration":[1]}`)); err == nil {
		t.Error("expected error for array duration, got nil")
	}
}

func TestProbeJSON_emptyStreamsArray(t *testing.T) {
	t.Parallel()

	var res ProbeResult
	if err := res.UnmarshalJSON([]byte(`{"format_name":"x","streams":[]}`)); err != nil {
		t.Fatalf("UnmarshalJSON err = %v", err)
	}
	if res.Streams == nil {
		t.Error("Streams = nil, want empty non-nil slice")
	}
	if len(res.Streams) != 0 {
		t.Errorf("Streams len = %d, want 0", len(res.Streams))
	}
}

func TestToProbe_unknownStreamTypeIgnored(t *testing.T) {
	t.Parallel()

	res := ProbeResult{
		FormatName: "mkv",
		Streams: []ProbeStream{
			{CodecType: "data", CodecName: "bin_data"},
			{CodecType: "attachment", CodecName: "ttf"},
		},
	}
	got := res.ToProbe()
	if got.Kind != "" {
		t.Errorf("Kind = %q, want empty", got.Kind)
	}
	if got.VideoCodec != "" || got.AudioCodec != "" {
		t.Errorf("codecs = %+v, want none", got)
	}
}

func TestToProbe_firstVideoStreamWins(t *testing.T) {
	t.Parallel()

	res := ProbeResult{
		Streams: []ProbeStream{
			{CodecType: "video", CodecName: "h264", Width: 640, Height: 480},
			{CodecType: "video", CodecName: "hevc", Width: 1920, Height: 1080},
		},
	}
	got := res.ToProbe()
	if got.VideoCodec != "h264" || got.Width != 640 || got.Height != 480 {
		t.Errorf("got %+v, want first video stream", got)
	}
}

func TestLookPath_emptyName(t *testing.T) {
	t.Parallel()

	_, err := LookPath("")
	if !errors.Is(err, ErrToolMissing) {
		t.Errorf("LookPath(\"\") err = %v, want ErrToolMissing", err)
	}
}

func TestScaleFilter_unsetDims(t *testing.T) {
	t.Parallel()

	if got := scaleFilter(0, 0); got != "scale=-2:-2" {
		t.Errorf("scaleFilter(0, 0) = %q, want scale=-2:-2", got)
	}
	if got := scaleFilter(641, 1); got != "scale=640:2" {
		t.Errorf("scaleFilter(641, 1) = %q, want scale=640:2", got)
	}
}

func TestNormalizeFormat_edgeInputs(t *testing.T) {
	t.Parallel()

	cases := map[string]string{
		".MP4":  "mp4",
		"  ":    "",
		"webm ": "webm",
	}
	for in, want := range cases {
		if got := normalizeFormat(in); got != want {
			t.Errorf("normalizeFormat(%q) = %q, want %q", in, got, want)
		}
	}
}
