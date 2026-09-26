package job

import (
	"testing"
)

// TestEveryReusesCachedSchedule proves the single-parse path: the cron entry
// must reuse the cached cron.Schedule instead of re-parsing via AddFunc.
func TestEveryReusesCachedSchedule(t *testing.T) {
	t.Parallel()

	s := NewScheduler(&Dispatcher{Q: &stubQueue{}}, NewUniqueLocker(&fakeCache{}))
	spec := "0 * * * *"

	id, err := s.Every(spec, "some-job", nil)
	if err != nil {
		t.Fatalf("Every: %v", err)
	}

	if id == 0 {
		t.Fatal("Every id=0 want nonzero")
	}

	cached, err := s.cachedSchedule(spec)
	if err != nil {
		t.Fatalf("cachedSchedule: %v", err)
	}

	var entrySched any

	for _, e := range s.cron.Entries() {
		//nolint:gosec // IDs originate from Every, a small positive cron sequence.
		if EntryID(e.ID) == id {
			entrySched = e.Schedule

			break
		}
	}

	if entrySched == nil {
		t.Fatalf("entry %v not found", id)
	}

	if entrySched != cached {
		t.Fatalf("cron entry Schedule %p != cached %p: Every re-parsed instead of reusing cached schedule", entrySched, cached)
	}
}
