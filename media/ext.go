package media

import (
	"strings"
)

// ExtForContentType maps a MIME content type to a file extension.
// The input is lowercased and any ";..." parameters are stripped.
// Unknown types map to ".bin".
func ExtForContentType(contentType string) string {
	ct := strings.ToLower(strings.TrimSpace(contentType))
	if i := strings.Index(ct, ";"); i >= 0 {
		ct = strings.TrimSpace(ct[:i])
	}

	switch ct {
	case "image/jpeg":
		return ".jpg"
	case "image/png":
		return ".png"
	case "image/gif":
		return ".gif"
	case "image/webp":
		return ".webp"
	case "video/mp4":
		return ".mp4"
	case "video/webm":
		return ".webm"
	case "video/quicktime":
		return ".mov"
	case "video/x-msvideo":
		return ".avi"
	case "video/x-matroska":
		return ".mkv"
	case "video/mp2t":
		return ".ts"
	case "audio/mpeg":
		return ".mp3"
	case "audio/wav":
		return ".wav"
	case "audio/ogg":
		return ".ogg"
	case "audio/flac":
		return ".flac"
	case "audio/mp4":
		return ".m4a"
	case "audio/aac":
		return ".aac"
	case "audio/webm":
		return ".weba"
	case "audio/opus":
		return ".opus"
	default:
		return ".bin"
	}
}

// ContentTypeForExt maps a file extension to a MIME content type.
// The input is lowercased and accepted with or without a leading dot.
// Unknown extensions map to "application/octet-stream".
func ContentTypeForExt(ext string) string {
	switch strings.TrimPrefix(strings.ToLower(strings.TrimSpace(ext)), ".") {
	case "jpg", "jpeg":
		return "image/jpeg"
	case "png":
		return "image/png"
	case "gif":
		return "image/gif"
	case "webp":
		return "image/webp"
	case "mp4":
		return "video/mp4"
	case "webm":
		return "video/webm"
	case "mov":
		return "video/quicktime"
	case "avi":
		return "video/x-msvideo"
	case "mkv":
		return "video/x-matroska"
	case "ts":
		return "video/mp2t"
	case "mp3":
		return "audio/mpeg"
	case "wav":
		return "audio/wav"
	case "ogg":
		return "audio/ogg"
	case "flac":
		return "audio/flac"
	case "m4a":
		return "audio/mp4"
	case "aac":
		return "audio/aac"
	case "weba":
		return "audio/webm"
	case "opus":
		return "audio/opus"
	default:
		return "application/octet-stream"
	}
}

// KindForExt maps a file extension to a MediaKind.
// The input is lowercased and accepted with or without a leading dot.
// Unknown extensions return "", false.
func KindForExt(ext string) (MediaKind, bool) {
	switch strings.TrimPrefix(strings.ToLower(strings.TrimSpace(ext)), ".") {
	case "jpg", "jpeg", "png", "gif", "webp":
		return KindImage, true
	case "mp3", "wav", "ogg", "flac", "m4a", "aac", "weba", "opus":
		return KindAudio, true
	case "mp4", "webm", "mov", "avi", "mkv", "ts":
		return KindVideo, true
	default:
		return "", false
	}
}
