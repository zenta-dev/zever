package job

import (
	"testing"

	"github.com/robfig/cron/v3"
)

func TestEveryWithScheduleReusesPassedSchedule(t *testing.T) {
	t.Parallel()

	s := NewScheduler(&Dispatcher{Q: &stubQueue{}}, NewUniqueLocker(&fakeCache{}))
	spec := "30 * * * *"

	parsed, err := cron.ParseStandard(spec)
	if err != nil {
		t.Fatalf("ParseStandard: %v", err)
	}

	id, err := s.EveryWithSchedule(spec, "some-job", nil, parsed)
	if err != nil {
		t.Fatalf("EveryWithSchedule: %v", err)
	}

	if id == 0 {
		t.Fatal("EveryWithSchedule id=0 want nonzero")
	}

	for _, e := range s.cron.Entries() {
		//nolint:gosec // IDs originate from Every, a small positive cron sequence.
		if EntryID(e.ID) == id && e.Schedule != parsed {
			t.Fatalf("entry Schedule %p != passed %p", e.Schedule, parsed)
		}
	}
}

func TestEveryWithScheduleNilSchedule(t *testing.T) {
	t.Parallel()

	s := NewScheduler(&Dispatcher{Q: &stubQueue{}}, NewUniqueLocker(&fakeCache{}))

	if _, err := s.EveryWithSchedule("0 * * * *", "some-job", nil, nil); err == nil {
		t.Fatal("EveryWithSchedule nil sched want error")
	}
}

func TestEveryWithScheduleNilDispatcher(t *testing.T) {
	t.Parallel()

	s := NewScheduler(nil, NewUniqueLocker(&fakeCache{}))

	parsed, err := cron.ParseStandard("0 * * * *")
	if err != nil {
		t.Fatalf("ParseStandard: %v", err)
	}

	if _, err := s.EveryWithSchedule("0 * * * *", "some-job", nil, parsed); err == nil {
		t.Fatal("EveryWithSchedule nil Dispatcher want error")
	}
}
