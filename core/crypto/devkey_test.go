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

func TestIsDevEnv(t *testing.T) {
	t.Parallel()

	cases := []struct {
		in   string
		want bool
	}{
		{" Dev ", true},
		{"DEVELOPMENT", true},
		{"test\n", true},
		{"dev", true},
		{"prod", false},
		{"", false},
		{"   ", false},
		{"production", false},
	}
	for _, c := range cases {
		in, want := c.in, c.want
		if got := IsDevEnv(in); got != want {
			t.Errorf("IsDevEnv(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestDevKeyAllowed_trimsEnv(t *testing.T) {
	t.Setenv("ZEVER_ENV", " Dev\n")
	if !DevKeyAllowed(Options{Key: DevCryptoKey}) {
		t.Error("DevKeyAllowed with padded ZEVER_ENV = false, want true")
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
