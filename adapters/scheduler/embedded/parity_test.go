package embedded

import (
	"reflect"
	"testing"

	"github.com/zenta-dev/zever/core/job"
	"github.com/zenta-dev/zever/core/scheduler"
)

func TestFacadeParityEntryIDAlias(t *testing.T) {
	t.Parallel()

	if reflect.TypeOf(scheduler.EntryID(0)) != reflect.TypeOf(job.EntryID(0)) {
		t.Fatalf("scheduler.EntryID %v is not job.EntryID %v: facade must alias job type",
			reflect.TypeOf(scheduler.EntryID(0)), reflect.TypeOf(job.EntryID(0)))
	}
}

func TestFacadeParityEntryLifecycle(t *testing.T) {
	registerJobOnce(t, "parity-job")

	dispatcher := &job.Dispatcher{Q: &stubQueue{}}
	direct := job.NewScheduler(dispatcher, nil)

	facade, err := New(scheduler.Options{Dispatcher: dispatcher})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx := t.Context()
	spec := "0 * * * *"

	jobID, err := direct.Every(spec, "parity-job", nil)
	if err != nil {
		t.Fatalf("Every: %v", err)
	}

	if jobID == 0 {
		t.Fatal("Every id=0 want nonzero")
	}

	facadeID, err := facade.Schedule(ctx, spec, "parity-job", nil)
	if err != nil {
		t.Fatalf("Schedule: %v", err)
	}

	if facadeID == 0 {
		t.Fatal("Schedule id=0 want nonzero")
	}

	foundDirect := false

	for _, id := range direct.Entries() {
		if id == jobID {
			foundDirect = true
			break
		}
	}

	if !foundDirect {
		t.Fatalf("direct Entries %v missing id %v", direct.Entries(), jobID)
	}

	foundFacade := false

	for _, id := range facade.Entries() {
		if id == facadeID {
			foundFacade = true
			break
		}
	}

	if !foundFacade {
		t.Fatalf("facade Entries %v missing id %v", facade.Entries(), facadeID)
	}

	// Unknown IDs are a no-op on both APIs.
	direct.Remove(job.EntryID(999999))

	if err := facade.Remove(scheduler.EntryID(999999)); err != nil {
		t.Fatalf("Remove unknown: %v", err)
	}

	direct.Remove(jobID)

	if err := facade.Remove(facadeID); err != nil {
		t.Fatalf("Remove: %v", err)
	}

	for _, id := range direct.Entries() {
		if id == jobID {
			t.Fatalf("direct Entries %v still contains %v", direct.Entries(), jobID)
		}
	}

	for _, id := range facade.Entries() {
		if id == facadeID {
			t.Fatalf("facade Entries %v still contains %v", facade.Entries(), facadeID)
		}
	}

	// Start/Stop idempotency through the facade only.
	if err := facade.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}

	if err := facade.Start(); err != nil {
		t.Fatalf("second Start: %v", err)
	}

	if err := facade.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}

	if err := facade.Stop(); err != nil {
		t.Fatalf("second Stop: %v", err)
	}
}
