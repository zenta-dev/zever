package scheduler

import (
	"errors"
	"testing"
	"time"

	"github.com/zenta-dev/zever/core/job"
)

func TestOptionsZeroInvalid(t *testing.T) {
	t.Parallel()

	err := Options{}.Validate()
	if !errors.Is(err, ErrInvalidOptions) {
		t.Fatalf("err=%v want ErrInvalidOptions", err)
	}
}

func TestOptionsValid(t *testing.T) {
	t.Parallel()

	d := &job.Dispatcher{Q: &stubQueue{}}
	opts := Options{Dispatcher: d}

	if err := opts.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
}

func TestOptionsNegativeTimeout(t *testing.T) {
	t.Parallel()

	d := &job.Dispatcher{Q: &stubQueue{}}
	opts := Options{Dispatcher: d, CloseTimeout: -time.Second}

	err := opts.Validate()
	if !errors.Is(err, ErrInvalidOptions) {
		t.Fatalf("err=%v want ErrInvalidOptions", err)
	}
}

func TestOptionsZeroTimeoutValid(t *testing.T) {
	t.Parallel()

	d := &job.Dispatcher{Q: &stubQueue{}}
	opts := Options{Dispatcher: d, CloseTimeout: 0}

	if err := opts.Validate(); err != nil {
		t.Fatalf("Validate zero timeout: %v", err)
	}
}
