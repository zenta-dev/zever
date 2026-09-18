package scheduler_test

import (
	"github.com/zenta-dev/zever/job"
	"github.com/zenta-dev/zever/scheduler"
	schedulerembedded "github.com/zenta-dev/zever/scheduler/embedded"
)

// ExampleOpen opens the embedded scheduler with an in-process dispatcher.
func ExampleOpen() {
	_ = scheduler.Register(scheduler.Embedded, schedulerembedded.New)

	s, err := scheduler.Open(scheduler.Embedded, scheduler.Options{Dispatcher: &job.Dispatcher{}})
	if err != nil {
		return
	}

	defer func() { _ = s.Stop() }()
}
