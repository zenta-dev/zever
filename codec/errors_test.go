package codec

import (
	"testing"
)

func TestErrSentinelMessages(t *testing.T) {
	t.Parallel()

	if got := ErrEncode.Error(); got != "codec: encode failed" {
		t.Errorf("ErrEncode.Error() = %q, want %q", got, "codec: encode failed")
	}

	if got := ErrDecode.Error(); got != "codec: decode failed" {
		t.Errorf("ErrDecode.Error() = %q, want %q", got, "codec: decode failed")
	}
}
