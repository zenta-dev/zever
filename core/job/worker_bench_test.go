package job

import (
	"sync"
	"testing"
	"time"
)

// BenchmarkWorkerEmptyPoll measures the empty-poll wait path that runLoop
// hits on every idle iteration. It exercises the first empty poll in a run
// (attempt 0, wait 0) and reuses one timer across iterations, matching
// runLoop's own timer reuse, so the per-poll timer allocation is what shows
// up in allocs/op rather than benchmark bookkeeping.
func BenchmarkWorkerEmptyPoll(b *testing.B) {
	w := &Worker{}
	var wg sync.WaitGroup

	timer := time.NewTimer(0)
	defer timer.Stop()

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		attempt := 0
		if w.handleEmpty(b.Context(), &wg, &attempt, DefaultMaxPollWait, timer) {
			b.Fatal("handleEmpty returned true for background context")
		}
	}
}
