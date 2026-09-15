package media

import (
	"testing"
)

func TestExtForContentType_forward_table(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		ct   string
		want string
	}{
		{"jpeg", "image/jpeg", ".jpg"},
		{"png", "image/png", ".png"},
		{"gif", "image/gif", ".gif"},
		{"webp", "image/webp", ".webp"},
		{"mp4", "video/mp4", ".mp4"},
		{"webm video", "video/webm", ".webm"},
		{"quicktime", "video/quicktime", ".mov"},
		{"msvideo", "video/x-msvideo", ".avi"},
		{"matroska", "video/x-matroska", ".mkv"},
		{"mp2t", "video/mp2t", ".ts"},
		{"mpeg", "audio/mpeg", ".mp3"},
		{"wav", "audio/wav", ".wav"},
		{"ogg", "audio/ogg", ".ogg"},
		{"flac", "audio/flac", ".flac"},
		{"mp4 audio", "audio/mp4", ".m4a"},
		{"aac", "audio/aac", ".aac"},
		{"webm audio", "audio/webm", ".weba"},
		{"opus", "audio/opus", ".opus"},
		{"upper", "IMAGE/JPEG", ".jpg"},
		{"params", "image/jpeg; charset=binary", ".jpg"},
		{"params spaces", "  video/mp4 ; foo=bar ", ".mp4"},
		{"empty", "", ".bin"},
		{"octet", "application/octet-stream", ".bin"},
		{"text", "text/plain", ".bin"},
		{"tiff", "image/tiff", ".bin"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := ExtForContentType(tc.ct); got != tc.want {
				t.Errorf("ExtForContentType(%q) = %q, want %q", tc.ct, got, tc.want)
			}
		})
	}
}

func TestContentTypeForExt_reverse_table(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		ext  string
		want string
	}{
		{"jpg", ".jpg", "image/jpeg"},
		{"jpeg", ".jpeg", "image/jpeg"},
		{"png", ".png", "image/png"},
		{"gif", ".gif", "image/gif"},
		{"webp", ".webp", "image/webp"},
		{"mp4", ".mp4", "video/mp4"},
		{"webm", ".webm", "video/webm"},
		{"mov", ".mov", "video/quicktime"},
		{"avi", ".avi", "video/x-msvideo"},
		{"mkv", ".mkv", "video/x-matroska"},
		{"ts", ".ts", "video/mp2t"},
		{"mp3", ".mp3", "audio/mpeg"},
		{"wav", ".wav", "audio/wav"},
		{"ogg", ".ogg", "audio/ogg"},
		{"flac", ".flac", "audio/flac"},
		{"m4a", ".m4a", "audio/mp4"},
		{"aac", ".aac", "audio/aac"},
		{"weba", ".weba", "audio/webm"},
		{"opus", ".opus", "audio/opus"},
		{"no dot", "jpg", "image/jpeg"},
		{"upper", ".JPG", "image/jpeg"},
		{"upper no dot", "MP4", "video/mp4"},
		{"unknown", ".xyz", "application/octet-stream"},
		{"unknown no dot", "xyz", "application/octet-stream"},
		{"empty", "", "application/octet-stream"},
		{"dot only", ".", "application/octet-stream"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := ContentTypeForExt(tc.ext); got != tc.want {
				t.Errorf("ContentTypeForExt(%q) = %q, want %q", tc.ext, got, tc.want)
			}
		})
	}
}

func TestKindForExt_table(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		ext   string
		kind  MediaKind
		found bool
	}{
		{"jpg", ".jpg", KindImage, true},
		{"jpeg", ".jpeg", KindImage, true},
		{"png", ".png", KindImage, true},
		{"gif", ".gif", KindImage, true},
		{"webp", ".webp", KindImage, true},
		{"mp3", ".mp3", KindAudio, true},
		{"wav", ".wav", KindAudio, true},
		{"ogg", ".ogg", KindAudio, true},
		{"flac", ".flac", KindAudio, true},
		{"m4a", ".m4a", KindAudio, true},
		{"aac", ".aac", KindAudio, true},
		{"weba", ".weba", KindAudio, true},
		{"opus", ".opus", KindAudio, true},
		{"mp4", ".mp4", KindVideo, true},
		{"webm", ".webm", KindVideo, true},
		{"mov", ".mov", KindVideo, true},
		{"avi", ".avi", KindVideo, true},
		{"mkv", ".mkv", KindVideo, true},
		{"ts", ".ts", KindVideo, true},
		{"no dot", "mp3", KindAudio, true},
		{"upper", ".MP4", KindVideo, true},
		{"unknown", ".xyz", "", false},
		{"unknown no dot", "xyz", "", false},
		{"empty", "", "", false},
		{"bin", ".bin", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, ok := KindForExt(tc.ext)
			if ok != tc.found {
				t.Fatalf("KindForExt(%q) found = %v, want %v", tc.ext, ok, tc.found)
			}

			if got != tc.kind {
				t.Errorf("KindForExt(%q) = %q, want %q", tc.ext, got, tc.kind)
			}
		})
	}
}
