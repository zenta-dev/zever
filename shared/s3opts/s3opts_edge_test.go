package s3opts

import "testing"

// TestEscapeKey_empty verifies an empty key stays empty.
func TestEscapeKey_empty(t *testing.T) {
	t.Parallel()

	if got := EscapeKey(""); got != "" {
		t.Fatalf("EscapeKey(\"\") = %q, want empty", got)
	}
}

// TestStaticURL_emptyKey verifies an empty key produces a trailing slash
// without error.
func TestStaticURL_emptyKey(t *testing.T) {
	t.Parallel()

	c := &Core{Region: DefaultRegion}

	got, err := c.StaticURL("bucket", "")
	if err != nil {
		t.Fatalf("StaticURL() error = %v", err)
	}

	want := "https://bucket.s3.us-east-1.amazonaws.com/"
	if got != want {
		t.Fatalf("StaticURL() = %q, want %q", got, want)
	}
}
