package crypto

import "testing"

func TestIsDevCryptoKey(t *testing.T) {
	t.Parallel()

	if !IsDevCryptoKey(DevCryptoKey) {
		t.Error("IsDevCryptoKey(DevCryptoKey) = false, want true")
	}
	if IsDevCryptoKey("") || IsDevCryptoKey("not-the-dev-key") {
		t.Error("IsDevCryptoKey returned true for a non-dev key")
	}
}
