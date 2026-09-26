package job

import (
	"context"
	"errors"
	"testing"

	"github.com/zenta-dev/zever/core/queue"
)

func TestSentinelMessages(t *testing.T) {
	cases := map[string][2]string{
		"ErrRegisterNameEmpty": {ErrRegisterNameEmpty.Error(), "job: register name is empty"},
		"ErrRegisterHandleNil": {ErrRegisterHandleNil.Error(), "job: register handle is nil"},
		"ErrUseMiddlewareNil":  {ErrUseMiddlewareNil.Error(), "job: use middleware is nil"},
		"ErrDuplicateJob":      {ErrDuplicateJob.Error(), "job: duplicate registration"},
		"ErrUnknownJob":        {ErrUnknownJob.Error(), "job: unknown job"},
		"ErrMemberLost":        {ErrMemberLost.Error(), "job: batch member lost"},
		"ErrInvalidLockTTL":    {ErrInvalidLockTTL.Error(), "job: unique lock ttl must be > 0"},
		"ErrUniqueLockerNil":   {ErrUniqueLockerNil.Error(), "job: unique locker is nil"},
		"ErrHandlerPanic":      {ErrHandlerPanic.Error(), "job: handler panic"},
	}

	for name, c := range cases {
		if c[0] != c[1] {
			t.Errorf("%s = %q, want %q", name, c[0], c[1])
		}
	}
}

func TestDuplicateJobErrorIsAs(t *testing.T) {
	Reset()

	handler := func(context.Context, string) error { return nil }

	if err := Register("dup", handler); err != nil {
		t.Fatalf("Register(dup) = %v, want nil", err)
	}

	err := Register("dup", handler)
	if err == nil {
		t.Fatal("second Register(dup) = nil, want DuplicateJobError")
	}

	if !errors.Is(err, ErrDuplicateJob) {
		t.Errorf("errors.Is(err, ErrDuplicateJob) = false (err = %T %v)", err, err)
	}

	var dupErr *DuplicateJobError
	if !errors.As(err, &dupErr) {
		t.Fatalf("errors.As(err, DuplicateJobError) = false (err = %T %v)", err, err)
	}

	if dupErr.Name != "dup" {
		t.Errorf("DuplicateJobError.Name = %q, want %q", dupErr.Name, "dup")
	}
}

func TestUnknownJobErrorIsAs(t *testing.T) {
	Reset()

	handler := func(context.Context, string) error { return nil }

	if err := Register("known", handler); err != nil {
		t.Fatalf("Register(known) = %v, want nil", err)
	}

	err := (&Dispatcher{}).Dispatch(t.Context(), "nope", "anything")
	if err == nil {
		t.Fatal("Dispatch(nope) = nil, want UnknownJobError")
	}

	if !errors.Is(err, ErrUnknownJob) {
		t.Errorf("errors.Is(err, ErrUnknownJob) = false (err = %T %v)", err, err)
	}

	var unknownErr *UnknownJobError
	if !errors.As(err, &unknownErr) {
		t.Fatalf("errors.As(err, UnknownJobError) = false (err = %T %v)", err, err)
	}

	if unknownErr.Name != "nope" {
		t.Errorf("UnknownJobError.Name = %q, want %q", unknownErr.Name, "nope")
	}
}

func TestNilUniqueLocker(t *testing.T) {
	Reset()

	handler := func(context.Context, string) error { return nil }

	if err := Register("t", handler); err != nil {
		t.Fatalf("Register(t) = %v, want nil", err)
	}

	err := (&Dispatcher{}).Dispatch(t.Context(), "t", "arg", UniqueBy("k"))
	if !errors.Is(err, ErrUniqueLockerNil) {
		t.Errorf("errors.Is(err, ErrUniqueLockerNil) = false (err = %T %v)", err, err)
	}
}

func TestInvalidLockTTL(t *testing.T) {
	l := NewUniqueLocker(nil)

	ok, err := l.Acquire(t.Context(), "k", 0)
	if ok {
		t.Errorf("Acquire(k, 0) ok = true, want false")
	}

	if !errors.Is(err, ErrInvalidLockTTL) {
		t.Errorf("errors.Is(err, ErrInvalidLockTTL) = false (err = %T %v)", err, err)
	}
}

func TestHandlerPanicSentinel(t *testing.T) {
	w := &Worker{}

	err := w.invokeHandler(t.Context(), func(context.Context, Payload) error { panic("boom") }, nil, "panicky")
	if !errors.Is(err, ErrHandlerPanic) {
		t.Errorf("errors.Is(err, ErrHandlerPanic) = false (err = %T %v)", err, err)
	}
}

type popEmptyQueue struct{ queue.Queue }

func (popEmptyQueue) Pop(context.Context, string) (queue.Message, error) {
	return queue.Message{}, &queue.EmptyError{Topic: "t"}
}

var _ queue.Queue = popEmptyQueue{}

func TestPopEmptyErrorIsEmpty(t *testing.T) {
	w := &Worker{Q: popEmptyQueue{}, Queues: []string{"t"}}

	_, _, err := w.popNextAvailable(t.Context())
	if !errors.Is(err, queue.ErrEmpty) {
		t.Errorf("errors.Is(err, queue.ErrEmpty) = false (err = %T %v)", err, err)
	}
}
