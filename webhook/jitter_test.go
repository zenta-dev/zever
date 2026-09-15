package webhook

import (
	"testing"
	"time"
)

func TestJitterRand_nonNil(t *testing.T) {
	t.Parallel()
	if r := jitterRand(); r == nil {
		t.Fatal("jitterRand() = nil")
	}
}

func TestJitterRand_Int63n_inRange(t *testing.T) {
	t.Parallel()
	r := jitterRand()
	for range 100 {
		if got := r.Int63n(10); got < 0 || got >= 10 {
			t.Fatalf("Int63n(10) = %d, want in [0, 10)", got)
		}
	}
}

func TestJitter_rangeAndZero(t *testing.T) {
	t.Parallel()
	r := jitterRand()
	if got := jitter(r, 0); got != 0 {
		t.Errorf("jitter(0) = %v want 0", got)
	}
	if got := jitter(r, -time.Second); got != 0 {
		t.Errorf("jitter(-1s) = %v want 0", got)
	}
	base := time.Second
	for range 100 {
		got := jitter(r, base)
		if got < base || got > base+base/4 {
			t.Fatalf("jitter(1s) = %v, want in [1s, 1.25s]", got)
		}
	}
}
