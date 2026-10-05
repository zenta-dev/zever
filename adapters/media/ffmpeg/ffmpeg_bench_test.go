package ffmpeg

import (
	"testing"

	"github.com/zenta-dev/zever/core/media"
)

func BenchmarkBuildArgsVideo(b *testing.B) {
	spec := Spec{Kind: media.KindVideo, Format: "mp4", Width: 1280, Height: 720, CRF: 23}
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		argv := BuildArgs("in.mp4", "out.mp4", spec)
		if len(argv) == 0 {
			b.Fatal("BuildArgs returned empty argv")
		}
	}
}

func BenchmarkBuildArgsAudio(b *testing.B) {
	spec := Spec{Kind: media.KindAudio, Format: "mp3", Bitrate: 128}
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		argv := BuildArgs("in.wav", "out.mp3", spec)
		if len(argv) == 0 {
			b.Fatal("BuildArgs returned empty argv")
		}
	}
}

func BenchmarkBuildArgsThumbnail(b *testing.B) {
	spec := Spec{Kind: media.KindVideo, Format: "jpg", Width: 320, Height: 240, Quality: 80, Offset: 5 * 1000000000}
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		argv := BuildArgs("in.mp4", "out.jpg", spec)
		if len(argv) == 0 {
			b.Fatal("BuildArgs returned empty argv")
		}
	}
}

func BenchmarkProbeResultUnmarshalJSON(b *testing.B) {
	raw := []byte(`{"format":{"format_name":"mov,mp4,m4a,3gp,3g2,mj2","duration":"12.340"},"streams":[{"codec_type":"video","codec_name":"h264","width":640,"height":480},{"codec_type":"audio","codec_name":"aac"}]}`)
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		var res ProbeResult
		if err := res.UnmarshalJSON(raw); err != nil {
			b.Fatalf("UnmarshalJSON err = %v", err)
		}
	}
}

func BenchmarkToProbe(b *testing.B) {
	res := ProbeResult{
		FormatName: "mov,mp4",
		Duration:   12.34,
		Streams: []ProbeStream{
			{CodecType: "video", CodecName: "h264", Width: 640, Height: 480},
			{CodecType: "audio", CodecName: "aac"},
		},
	}
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		probe := res.ToProbe()
		if probe.Kind != media.KindVideo {
			b.Fatalf("Kind = %q, want video", probe.Kind)
		}
	}
}

func BenchmarkIsVideoFormat(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if !IsVideoFormat("mp4") {
			b.Fatal("IsVideoFormat(mp4) = false, want true")
		}
	}
}

func BenchmarkIsAudioFormat(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if !IsAudioFormat("mp3") {
			b.Fatal("IsAudioFormat(mp3) = false, want true")
		}
	}
}
