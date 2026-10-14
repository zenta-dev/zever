package payment

import (
	"context"
	"errors"
	"testing"

	"github.com/zenta-dev/zever/core/idempotency"
)

type fakeStore struct {
	idempotency.Store
	replay      bool
	completeErr error
	forgotten   int
}

func (s *fakeStore) Begin(context.Context, string, idempotency.BeginOptions) (idempotency.Outcome, error) {
	return idempotency.Outcome{Replay: s.replay}, nil
}

func (s *fakeStore) Complete(context.Context, string, []byte, []byte) error { return s.completeErr }

func (s *fakeStore) Forget(context.Context, string) error {
	s.forgotten++

	return nil
}

func TestProcessWebhookOnce(t *testing.T) {
	t.Parallel()

	ok := func(context.Context, Event) error { return nil }
	ev := Event{ID: "evt_1"}

	if err := ProcessWebhookOnce(t.Context(), nil, "k", Event{}, nil, ok); err != nil {
		t.Fatalf("nil store err = %v, want nil", err)
	}

	if err := ProcessWebhookOnce(t.Context(), &fakeStore{}, "k", ev, nil, nil); err == nil {
		t.Fatal("nil handler err = nil, want error")
	}

	if err := ProcessWebhookOnce(t.Context(), &fakeStore{}, "k", Event{}, nil, ok); !errors.Is(err, ErrMissingEventID) {
		t.Fatalf("empty id err = %v, want ErrMissingEventID", err)
	}

	if err := ProcessWebhookOnce(t.Context(), &fakeStore{replay: true}, "k", ev, nil, ok); !errors.Is(err, ErrDuplicateEvent) {
		t.Fatalf("replay err = %v, want ErrDuplicateEvent", err)
	}

	boom := errors.New("complete failed")
	s := &fakeStore{completeErr: boom}

	if err := ProcessWebhookOnce(t.Context(), s, "k", ev, nil, ok); !errors.Is(err, boom) || s.forgotten != 1 {
		t.Fatalf("complete failure err = %v forgotten = %d, want boom/1", err, s.forgotten)
	}

	s = &fakeStore{}
	if err := ProcessWebhookOnce(t.Context(), s, "k", ev, nil, ok); err != nil || s.forgotten != 0 {
		t.Fatalf("success err = %v forgotten = %d, want nil/0", err, s.forgotten)
	}
}
