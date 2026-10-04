package s3

import (
	"errors"
	"strings"
	"testing"

	"github.com/zenta-dev/zever/core/media"
)

// TestEdgeTargetSize_invalid proves zero, negative, and non-numeric
// dimensions fail closed.
func TestEdgeTargetSize_invalid(t *testing.T) {
	t.Parallel()

	cases := []struct{ w, h string }{
		{"0", ""}, {"-1", ""}, {"abc", ""},
		{"", "0"}, {"", "-5"}, {"", "x"},
	}

	for _, tc := range cases {
		if _, _, err := targetSize(100, 50, tc.w, tc.h); err == nil {
			t.Errorf("targetSize(100,50,%q,%q) err = nil, want error", tc.w, tc.h)
		}
	}
}

// TestEdgeTargetSize_proportional proves a single side scales the other
// proportionally.
func TestEdgeTargetSize_proportional(t *testing.T) {
	t.Parallel()

	w, h, err := targetSize(100, 50, "50", "")
	if err != nil {
		t.Fatalf("targetSize(width only): %v", err)
	}
	if w != 50 || h != 25 {
		t.Fatalf("targetSize = %d,%d, want 50,25", w, h)
	}

	w, h, err = targetSize(100, 50, "", "25")
	if err != nil {
		t.Fatalf("targetSize(height only): %v", err)
	}
	if w != 50 || h != 25 {
		t.Fatalf("targetSize = %d,%d, want 50,25", w, h)
	}
}

// TestEdgeReadCapped_exactAndOver proves the limit is inclusive and one byte
// over fails with SizeLimitError.
func TestEdgeReadCapped_exactAndOver(t *testing.T) {
	t.Parallel()

	got, err := readCapped(strings.NewReader("abc"), 3)
	if err != nil {
		t.Fatalf("readCapped(exact): %v", err)
	}

	if string(got) != "abc" {
		t.Fatalf("readCapped = %q, want abc", got)
	}

	if _, err := readCapped(strings.NewReader("abcd"), 3); !errors.Is(err, media.ErrTooLarge) {
		t.Fatalf("readCapped(over) err = %v, want ErrTooLarge", err)
	}
}

// TestEdgeProbeImage_empty proves empty data fails image decoding.
func TestEdgeProbeImage_empty(t *testing.T) {
	t.Parallel()

	d := &driver{maxPixels: 1 << 20}

	if _, err := d.probeImage(nil, ".png"); err == nil {
		t.Fatal("probeImage(nil) err = nil, want error")
	}
}
