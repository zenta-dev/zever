package media

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestOptions_Validate_zero_valid(t *testing.T) {
	t.Parallel()
	if err := (Options{}).Validate(); err != nil {
		t.Fatalf("Validate zero err = %v, want nil", err)
	}
}

func TestOptions_Validate_violations_table(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		opts    Options
		reasons []string
	}{
		{"negative download bytes", Options{MaxDownloadBytes: -1}, []string{"max_download_bytes"}},
		{"negative pixels", Options{MaxPixels: -1}, []string{"max_pixels"}},
		{"negative derived_ttl", Options{DerivedTTL: -time.Second}, []string{"derived_ttl"}},
		{"negative max_duration", Options{MaxDuration: -time.Second}, []string{"max_duration"}},
		{"negative presign_ttl", Options{PresignTTL: -time.Second}, []string{"presign_ttl"}},
		{"presign_ttl 8d", Options{PresignTTL: 8 * 24 * time.Hour}, []string{"presign_ttl"}},
		{"no scheme", Options{Endpoint: "example.com/x"}, []string{"scheme"}},
		{"no host", Options{Endpoint: "https:///path"}, []string{"host"}},
		{"garbage", Options{Endpoint: "http://[::1"}, []string{"valid url"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := tc.opts.Validate()
			if !errors.Is(err, ErrInvalidOptions) {
				t.Fatalf("Validate err = %v, want ErrInvalidOptions", err)
			}

			for _, r := range tc.reasons {
				if !strings.Contains(err.Error(), r) {
					t.Errorf("Validate err %q missing %q", err.Error(), r)
				}
			}

			var ioe *InvalidOptionsError
			if !errors.As(err, &ioe) {
				t.Errorf("err %T is not *InvalidOptionsError", err)
			}
		})
	}
}

func TestOptions_Validate_multiple_joined(t *testing.T) {
	t.Parallel()

	err := Options{MaxDownloadBytes: -1, MaxPixels: -1, Endpoint: "example.com/x"}.Validate()
	if !errors.Is(err, ErrInvalidOptions) {
		t.Fatalf("Validate err = %v, want ErrInvalidOptions", err)
	}

	if !strings.Contains(err.Error(), "max_download_bytes") {
		t.Errorf("err %q missing download bytes reason", err.Error())
	}

	if !strings.Contains(err.Error(), "max_pixels") {
		t.Errorf("err %q missing max_pixels reason", err.Error())
	}

	if !strings.Contains(err.Error(), "scheme") {
		t.Errorf("err %q missing scheme reason", err.Error())
	}
}

func TestOptions_Validate_endpoints_table(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		endpoint string
		wantErr  bool
	}{
		{"https", "https://example.com/x", false},
		{"http localhost", "http://localhost:8080", false},
		{"empty", "", false},
		{"no scheme", "example.com/x", true},
		{"no host", "https:///path", true},
		{"garbage", "http://[::1", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := Options{Endpoint: tc.endpoint}.Validate()
			if tc.wantErr && err == nil {
				t.Errorf("Validate(%q) = nil, want error", tc.endpoint)
			}

			if !tc.wantErr && err != nil {
				t.Errorf("Validate(%q) = %v, want nil", tc.endpoint, err)
			}
		})
	}
}

func TestOptions_Validate_presignTTL_boundary(t *testing.T) {
	t.Parallel()

	if err := (Options{PresignTTL: MaxPresignTTL}).Validate(); err != nil {
		t.Errorf("Validate max PresignTTL err = %v, want nil", err)
	}

	if err := (Options{PresignTTL: MaxPresignTTL + time.Second}).Validate(); !errors.Is(err, ErrInvalidOptions) {
		t.Errorf("Validate over-max PresignTTL err = %v, want ErrInvalidOptions", err)
	}
}

func TestOptions_defaults_values(t *testing.T) {
	t.Parallel()

	if DefaultMaxDownloadBytes != 64<<20 {
		t.Errorf("DefaultMaxDownloadBytes = %d, want %d", DefaultMaxDownloadBytes, 64<<20)
	}

	if DefaultDerivedTTL != 10*time.Minute {
		t.Errorf("DefaultDerivedTTL = %v, want 10m", DefaultDerivedTTL)
	}

	if DefaultMaxPixels != 50*1024*1024 {
		t.Errorf("DefaultMaxPixels = %d, want %d", DefaultMaxPixels, 50*1024*1024)
	}

	if DefaultPresignTTL != time.Hour {
		t.Errorf("DefaultPresignTTL = %v, want 1h", DefaultPresignTTL)
	}

	if MaxPresignTTL != 7*24*time.Hour {
		t.Errorf("MaxPresignTTL = %v, want 168h", MaxPresignTTL)
	}

	if DefaultLocalRoot != "/tmp/media" {
		t.Errorf("DefaultLocalRoot = %q, want /tmp/media", DefaultLocalRoot)
	}

	if DefaultLocalBaseURL != "/media" {
		t.Errorf("DefaultLocalBaseURL = %q, want /media", DefaultLocalBaseURL)
	}

	if DefaultS3Region != "us-east-1" {
		t.Errorf("DefaultS3Region = %q, want us-east-1", DefaultS3Region)
	}

	if DefaultMaxDuration != 0 {
		t.Errorf("DefaultMaxDuration = %v, want 0", DefaultMaxDuration)
	}

	if DefaultFFmpeg != "ffmpeg" {
		t.Errorf("DefaultFFmpeg = %q, want ffmpeg", DefaultFFmpeg)
	}

	if DefaultFFProbe != "ffprobe" {
		t.Errorf("DefaultFFProbe = %q, want ffprobe", DefaultFFProbe)
	}
}
