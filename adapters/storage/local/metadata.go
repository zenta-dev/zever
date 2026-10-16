package local

import "strings"

// maxBody is the default per-object upload cap (64 MiB); override per adapter
// with storage.LocalOptions.MaxUploadBytes.
var maxBody = int64(64) << 20

type contentType string

type extension string

var contentTypes = map[extension]contentType{
	".png":  "image/png",
	".jpg":  "image/jpeg",
	".jpeg": "image/jpeg",
	".gif":  "image/gif",
	".webp": "image/webp",
	".pdf":  "application/pdf",
	".txt":  "text/plain",
	".json": "application/json",
}

var scriptableContentTypes = map[contentType]bool{
	"text/html":              true,
	"image/svg+xml":          true,
	"application/xhtml+xml":  true,
	"text/xml":               true,
	"application/xml":        true,
	"text/javascript":        true,
	"application/javascript": true,
}

const metaSuffix = ".meta"

// Staging-file prefixes the adapter creates next to objects; keys using them
// (or the sidecar suffix) are rejected so a client cannot read, overwrite or
// race the adapter's own bookkeeping files.
const (
	bodyTempPrefix = ".tmp-"
	metaTempPrefix = ".mtmp-"
)

// reservedKey reports whether key collides with adapter-internal files.
func reservedKey(key string) bool {
	if strings.HasSuffix(key, metaSuffix) {
		return true
	}

	for _, seg := range strings.Split(key, "/") {
		if strings.HasPrefix(seg, bodyTempPrefix) || strings.HasPrefix(seg, metaTempPrefix) {
			return true
		}
	}

	return false
}

// inlineSafeTypes is the allowlist of stored content types the adapter will
// echo back; anything else is served as application/octet-stream.
var inlineSafeTypes = map[contentType]bool{
	"image/png": true, "image/jpeg": true, "image/gif": true, "image/webp": true,
	"application/pdf": true, "text/plain": true, "application/json": true,
	"application/octet-stream": true, "text/csv": true, "application/zip": true,
	"video/mp4": true, "video/webm": true, "audio/mpeg": true, "audio/ogg": true,
}

func allowedContentType(ct string) bool {
	ct = strings.ToLower(strings.TrimSpace(strings.SplitN(ct, ";", 2)[0]))
	return inlineSafeTypes[contentType(ct)] && !scriptableContentTypes[contentType(ct)]
}

type objectMeta struct {
	ContentType string `json:"contentType"`
}
