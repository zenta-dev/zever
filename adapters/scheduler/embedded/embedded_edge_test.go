package embedded

import (
	"sync"
	"testing"
)

func TestStartStopRestart(t *testing.T) {
	t.Parallel()

	s := newTestScheduler(t)

	for i := 0; i < 2; i++ {
		if err := s.Start(); err != nil {
			t.Fatalf("Start #%d: %v", i, err)
		}
		if err := s.Stop(); err != nil {
			t.Fatalf("Stop #%d: %v", i, err)
		}
	}
}

func TestSchedule_concurrentSafe(t *testing.T) {
	registerJobOnce(t, "emb-test-concurrent")

	s := newTestScheduler(t)
	ctx := t.Context()

	const workers = 32

	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := s.Schedule(ctx, "0 * * * *", "emb-test-concurrent", nil); err != nil {
				t.Errorf("Schedule: %v", err)
			}
		}()
	}
	wg.Wait()

	if got := len(s.Entries()); got != workers {
		t.Errorf("Entries() len = %d, want %d", got, workers)
	}
}

func TestEntries_snapshotIsolated(t *testing.T) {
	registerJobOnce(t, "emb-test-snapshot")

	s := newTestScheduler(t)
	ctx := t.Context()

	first, err := s.Schedule(ctx, "0 * * * *", "emb-test-snapshot", nil)
	if err != nil {
		t.Fatalf("Schedule: %v", err)
	}
	if _, err := s.Schedule(ctx, "0 * * * *", "emb-test-snapshot", nil); err != nil {
		t.Fatalf("Schedule: %v", err)
	}

	snapshot := s.Entries()
	if len(snapshot) != 2 {
		t.Fatalf("Entries() len = %d, want 2", len(snapshot))
	}

	if err := s.Remove(first); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if len(snapshot) != 2 {
		t.Errorf("snapshot len changed to %d after Remove, want 2", len(snapshot))
	}
	if got := s.Entries(); len(got) != 1 {
		t.Errorf("Entries() after Remove = %v, want 1", got)
	}
}
