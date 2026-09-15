package webhook

import (
	"math/rand"
	"time"
)

// jitterRand returns a dedicated random source for retry backoff jitter.
// A dedicated source keeps webhook retries from perturbing the shared generator.
func jitterRand() *rand.Rand {
	//nolint:gosec // G404: math/rand suffices for retry jitter, which is not security-sensitive.
	return rand.New(rand.NewSource(time.Now().UnixNano()))
}

// jitter adds up to 25% random delay to d to decorrelate concurrent retries.
// Non-positive durations yield zero.
func jitter(r *rand.Rand, d time.Duration) time.Duration {
	if d <= 0 {
		return 0
	}
	//nolint:gosec // G115: the Int63n bound derives from a non-negative duration and cannot overflow int64.
	extra := r.Int63n(int64(d)/4 + 1)
	return d + time.Duration(extra)
}
