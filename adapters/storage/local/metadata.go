package local

var maxBody = int64(2) << 30

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

type objectMeta struct {
	ContentType string `json:"contentType"`
}
