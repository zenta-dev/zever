package s3opts

import (
	"fmt"
	"net/url"
	"strings"
)

// StaticURL returns the public unsigned URL for bucket and key.
func (c *Core) StaticURL(bucket, key string) (string, error) {
	if c.URLBase != "" {
		return strings.TrimRight(c.URLBase, "/") + "/" + url.PathEscape(bucket) + "/" + EscapeKey(key), nil
	}

	return fmt.Sprintf("https://%s.s3.%s.amazonaws.com/%s", bucket, c.Region, EscapeKey(key)), nil
}

// EscapeKey escapes key segment-wise so slashes survive as separators.
func EscapeKey(key string) string {
	segs := strings.Split(key, "/")
	for i := range segs {
		segs[i] = url.PathEscape(segs[i])
	}

	return strings.Join(segs, "/")
}
