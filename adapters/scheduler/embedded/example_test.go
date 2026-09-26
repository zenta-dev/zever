package embedded_test

import (
	schedulerembedded "github.com/zenta-dev/zever/adapters/scheduler/embedded"
	"github.com/zenta-dev/zever/core/job"
	"github.com/zenta-dev/zever/core/scheduler"
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
