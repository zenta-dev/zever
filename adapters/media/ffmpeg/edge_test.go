package ffmpeg

import (
	"strings"
	"testing"
	"time"

	"github.com/zenta-dev/zever/core/media"
)

// TestEdgeEvenDim_boundaries proves dimension snapping for unset, odd, one,
// and negative values.
func TestEdgeEvenDim_boundaries(t *testing.T) {
	t.Parallel()

	cases := map[int]int{
		0: -2, -1: -2, -100: -2,
		1: 2, 2: 2, 3: 2, 4: 4,
		100: 100, 101: 100,
	}

	for in, want := range cases {
		if got := evenDim(in); got != want {
			t.Errorf("evenDim(%d) = %d, want %d", in, got, want)
		}
	}
}

// TestEdgeJpegQ_boundaries proves quality maps onto and clamps to the
// ffmpeg 2-31 scale.
func TestEdgeJpegQ_boundaries(t *testing.T) {
	t.Parallel()

	cases := map[int]string{
		0: "8", -5: "8", 1: "31", 100: "2", 101: "2",
	}

	for in, want := range cases {
		if got := jpegQ(in); got != want {
			t.Errorf("jpegQ(%d) = %q, want %q", in, got, want)
		}
	}
}

// TestEdgeFormatSeconds_boundaries proves sub-second, negative, and zero
// durations format deterministically.
func TestEdgeFormatSeconds_boundaries(t *testing.T) {
	t.Parallel()

	cases := map[time.Duration]string{
		0:                       "0.000",
		time.Millisecond:        "0.001",
		time.Second:             "1.000",
		1500 * time.Millisecond: "1.500",
		-time.Second:            "-1.000",
	}

	for d, want := range cases {
		if got := formatSeconds(d); got != want {
			t.Errorf("formatSeconds(%v) = %q, want %q", d, got, want)
		}
	}
}

// TestEdgeBuildArgs_zeroDims proves zero dimensions omit the scale filter
// and preserve the default CRF.
func TestEdgeBuildArgs_zeroDims(t *testing.T) {
	t.Parallel()

	argv := BuildArgs("in.mp4", "out.mp4", Spec{Kind: media.KindVideo})
	joined := strings.Join(argv, " ")

	if strings.Contains(joined, "scale=") {
		t.Fatalf("zero dims emitted scale filter: %v", argv)
	}

	if !strings.Contains(joined, "-crf 23") {
		t.Fatalf("default CRF missing: %v", argv)
	}
}
