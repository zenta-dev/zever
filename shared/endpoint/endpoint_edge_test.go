package endpoint

import (
	"errors"
	"strings"
	"testing"
)

// TestValidateURL_longPathAccepted verifies large URLs are not truncated or rejected.
func TestValidateURL_longPathAccepted(t *testing.T) {
	t.Parallel()

	raw := "https://example.com/" + strings.Repeat("a", 64*1024)

	got, err := ValidateURL(raw)
	if err != nil {
		t.Fatalf("ValidateURL(long) error = %v", err)
	}

	if got != raw {
		t.Fatal("ValidateURL(long) normalized away content")
	}
}

// TestValidateURL_loopbackUserinfoRejected verifies policy options compose.
func TestValidateURL_loopbackUserinfoRejected(t *testing.T) {
	t.Parallel()

	_, err := ValidateURL("http://user@127.0.0.1:8080", WithAllowLoopbackHTTP(), WithRejectUserinfo())
	if !errors.Is(err, ErrUserinfo) {
		t.Fatalf("err = %v, want ErrUserinfo", err)
	}
}
