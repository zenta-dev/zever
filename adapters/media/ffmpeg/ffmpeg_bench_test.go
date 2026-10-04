package ffmpeg

import (
	"testing"
	"time"

	"github.com/zenta-dev/zever/core/media"
)

// BenchmarkBuildArgs measures argument-vector compilation for a video transcode.
func BenchmarkBuildArgs(b *testing.B) {
	spec := Spec{Kind: media.KindVideo, Format: "webm", Width: 1280, Height: 720, CRF: 23}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = BuildArgs("in.mp4", "out.webm", spec)
	}
}

// BenchmarkBuildThumbnailArgs measures argument-vector compilation for a
// scaled video thumbnail extraction.
func BenchmarkBuildThumbnailArgs(b *testing.B) {
	spec := Spec{Kind: media.KindVideo, Width: 320, Offset: 5 * time.Second}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = BuildArgs("in.mp4", "out.jpg", spec)
	}
}

// BenchmarkToProbe measures stream inspection and media.Probe conversion.
func BenchmarkToProbe(b *testing.B) {
	r := ProbeResult{
		FormatName: "mp4",
		Duration:   12.5,
		Streams: []ProbeStream{
			{CodecType: "video", CodecName: "h264", Width: 1920, Height: 1080},
			{CodecType: "audio", CodecName: "aac"},
		},
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = r.ToProbe()
	}
}

// BenchmarkProbeDecode measures ffprobe JSON parsing.
func BenchmarkProbeDecode(b *testing.B) {
	data := []byte(`{"format":{"format_name":"mp4","duration":"12.5"},` +
		`"streams":[{"codec_type":"video","codec_name":"h264","width":1920,"height":1080},` +
		`{"codec_type":"audio","codec_name":"aac"}]}`)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		if _, err := probeCodec.Decode(data); err != nil {
			b.Fatalf("Decode: %v", err)
		}
	}
}
