package s3

import (
	"errors"
	"net/url"
	"path"
	"strings"
	"testing"

	"github.com/zenta-dev/zever/core/storage"
	"github.com/zenta-dev/zever/shared/s3opts"
)

// FuzzKeyValidation proves object-key validation never panics and stays
// self-consistent: ValidKey and ValidateBucketKey must agree, accepted keys
// must satisfy every documented safety property (no traversal, no
// backslashes, no control characters, canonical Clean form), and rejections
// must carry the matching typed sentinel (ErrInvalidKey / ErrInvalidBucket).
func FuzzKeyValidation(f *testing.F) {
	seeds := []struct {
		bucket, key string
	}{
		{"bucket", "normal.txt"},
		{"bucket", "a/b/c.txt"},
		{"bucket", "../etc/passwd"},
		{"bucket", "..\\windows\\system32"},
		{"bucket", "/absolute"},
		{"bucket", ""},
		{"bucket", "."},
		{"bucket", ".."},
		{"bucket", "a/./b"},
		{"bucket", "a//b"},
		{"bucket", "trailing/"},
		{"bucket", "with space.txt"},
		{"bucket", "line\nbreak"},
		{"bucket", "nul\x00byte"},
		{"bucket", "del\x7fchar"},
		{"bucket", "\x00\xff\xfe"},
		{"", "normal.txt"},
		{"..", "normal.txt"},
		{"UPPER.valid-bucket.123", "normal.txt"},
		{"has space", "normal.txt"},
	}

	for _, seed := range seeds {
		f.Add(seed.bucket, seed.key)
	}

	f.Fuzz(func(t *testing.T, bucket, key string) {
		t.Parallel()

		valid := storage.ValidKey(key)
		err := storage.ValidateBucketKey(bucket, key)

		// Validator and gate must agree.
		if wantErr := !storage.ValidBucket(bucket) || !valid; (err != nil) != wantErr {
			t.Fatalf("ValidateBucketKey(%q, %q) err = %v, want error = %v", bucket, key, err, wantErr)
		}

		if err == nil {
			// Accepted keys must satisfy every documented property, so a
			// regression in any single check breaks this target instead
			// of silently widening what S3 will store.
			trimmed := strings.TrimSpace(key)

			if clean := path.Clean(trimmed); clean != trimmed {
				t.Fatalf("ValidKey(%q): path.Clean = %q, want canonical form", key, clean)
			}

			if trimmed == "." || trimmed == ".." ||
				strings.HasPrefix(trimmed, "/") || strings.HasPrefix(trimmed, "../") {
				t.Fatalf("ValidKey(%q): traversal form accepted", key)
			}

			for i := 0; i < len(trimmed); i++ {
				if c := trimmed[i]; c < 0x20 || c == 0x7f || c == '\\' {
					t.Fatalf("ValidKey(%q): byte 0x%02x accepted", key, c)
				}
			}

			return
		}

		// Rejections must carry the matching typed sentinel.
		if !storage.ValidBucket(bucket) {
			if !errors.Is(err, storage.ErrInvalidBucket) {
				t.Fatalf("ValidateBucketKey(%q, %q) = %v, want ErrInvalidBucket", bucket, key, err)
			}
		} else if !errors.Is(err, storage.ErrInvalidKey) {
			t.Fatalf("ValidateBucketKey(%q, %q) = %v, want ErrInvalidKey", bucket, key, err)
		}
	})
}

// FuzzEscapeKey proves URL escaping for S3 object keys never panics,
// preserves slashes as separators (never %2F), and round-trips segment-wise:
// unescaping every segment of the output must reproduce the input, so no
// key is silently mangled on the way to a presigned or static URL.
func FuzzEscapeKey(f *testing.F) {
	seeds := []string{
		"",
		"normal.txt",
		"a/b/c",
		"a b/c+d",
		"100%.txt",
		"héllo/世界",
		"a%2Fb",
		"\x00\xff\xfe",
		"a//b",
		"/leading",
		"trailing/",
		"?&=#",
	}

	for _, seed := range seeds {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, key string) {
		t.Parallel()

		got := s3opts.EscapeKey(key)

		if strings.Contains(got, "%2F") || strings.Contains(got, "%2f") {
			t.Fatalf("EscapeKey(%q) = %q, slash escaped as separator", key, got)
		}

		segs := strings.Split(got, "/")
		decoded := make([]string, len(segs))
		for i, seg := range segs {
			un, err := url.PathUnescape(seg)
			if err != nil {
				t.Fatalf("EscapeKey(%q) = %q, segment %q does not unescape: %v", key, got, seg, err)
			}
			decoded[i] = un
		}

		if want := strings.Join(decoded, "/"); want != key {
			t.Fatalf("EscapeKey(%q) = %q, segment round-trip = %q", key, got, want)
		}
	})
}
