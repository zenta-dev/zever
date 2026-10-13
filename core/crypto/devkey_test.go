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

func TestDevKeyAllowed(t *testing.T) {
	if !DevKeyAllowed(Options{Key: DevCryptoKey, AllowDevKey: true}) {
		t.Error("DevKeyAllowed(AllowDevKey) = false, want true")
	}

	for _, env := range []string{"dev", "test", "development", "DEV", "Test"} {
		t.Run("env="+env, func(t *testing.T) {
			t.Setenv("ZEVER_ENV", env)
			if !DevKeyAllowed(Options{Key: DevCryptoKey}) {
				t.Errorf("DevKeyAllowed with ZEVER_ENV=%s = false, want true", env)
			}
		})
	}

	for _, env := range []string{"", "production", "prod", "staging", "local"} {
		t.Run("env="+env, func(t *testing.T) {
			t.Setenv("ZEVER_ENV", env)
			if DevKeyAllowed(Options{Key: DevCryptoKey}) {
				t.Errorf("DevKeyAllowed with ZEVER_ENV=%q = true, want false", env)
			}
		})
	}
}
