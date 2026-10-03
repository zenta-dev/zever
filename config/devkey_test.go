package config

import "testing"

func TestIsDevCryptoKey(t *testing.T) {
	t.Parallel()

	if !IsDevCryptoKey(devCryptoKey) {
		t.Error("IsDevCryptoKey(devCryptoKey) = false, want true")
	}
	if IsDevCryptoKey("") || IsDevCryptoKey("not-the-dev-key") {
		t.Error("IsDevCryptoKey returned true for a non-dev key")
	}
}
