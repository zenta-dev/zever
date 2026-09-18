package job_test

import (
	"context"

	"github.com/zenta-dev/zever/job"
	"github.com/zenta-dev/zever/queue"
	queuememory "github.com/zenta-dev/zever/queue/memory"
)

type welcomeArgs struct {
	Name string
}

// ExampleRegister registers a typed job and dispatches it once.
func ExampleRegister() {
	job.Reset()
	defer job.Reset()

	_ = job.Register("example-welcome", func(_ context.Context, args welcomeArgs) error {
		_ = args.Name
		return nil
	})

	q, err := queuememory.New(queue.Options{})
	if err != nil {
		return
	}

	d := &job.Dispatcher{Q: q}

	_ = d.Dispatch(context.Background(), "example-welcome", welcomeArgs{Name: "gopher"})
}
