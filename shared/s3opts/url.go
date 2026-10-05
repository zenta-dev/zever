package s3opts

import (
	"net/url"
	"strings"
)

// StaticURL returns the public unsigned URL for bucket and key.
func (c *Core) StaticURL(bucket, key string) (string, error) {
	if c.URLBase != "" {
		return strings.TrimRight(c.URLBase, "/") + "/" + url.PathEscape(bucket) + "/" + EscapeKey(key), nil
	}

	return "https://" + bucket + ".s3." + c.Region + ".amazonaws.com/" + EscapeKey(key), nil
}

// EscapeKey escapes key segment-wise so slashes survive as separators.
func EscapeKey(key string) string {
	if !strings.Contains(key, "/") {
		return url.PathEscape(key)
	}

	buf := make([]byte, 0, len(key)+8)
	start := 0
	for i := 0; i < len(key); i++ {
		if key[i] == '/' {
			buf = append(buf, url.PathEscape(key[start:i])...)
			buf = append(buf, '/')
			start = i + 1
		}
	}

	buf = append(buf, url.PathEscape(key[start:])...)

	return string(buf)
}
