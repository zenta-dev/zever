package webhook

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

var freshSeq atomic.Int64

func freshAdapter() Adapter {
	return Adapter(1000 + int(freshSeq.Add(1)))
}

type stubWebhook struct {
	event   string
	target  string
	secret  string
	payload []byte
}

func (s *stubWebhook) Register(_ context.Context, event, target, secret string) error {
	s.event = event
	s.target = target
	s.secret = secret
	return nil
}

func (s *stubWebhook) Unregister(_ context.Context, event, target string) error {
	s.event = event
	s.target = target
	return nil
}

func (s *stubWebhook) Deliver(_ context.Context, event string, payload []byte) error {
	s.event = event
	s.payload = payload
	return nil
}

func (s *stubWebhook) Close() error { return nil }

func TestRegister_nilFactory_returnsErrNilFactory(t *testing.T) {
	a := freshAdapter()
	if err := Register(a, nil); !errors.Is(err, ErrNilFactory) {
		t.Fatalf("Register nil err = %v, want ErrNilFactory", err)
	}
}

func TestRegister_duplicate_returnsDuplicateAdapterError(t *testing.T) {
	a := freshAdapter()
	ok := func(Options) (Webhook, error) { return &stubWebhook{}, nil }
	if err := Register(a, ok); err != nil {
		t.Fatalf("first Register err = %v", err)
	}
	if err := Register(a, ok); !errors.Is(err, ErrDuplicateAdapter) {
		t.Fatalf("second Register err = %v, want ErrDuplicateAdapter", err)
	}
	var de *DuplicateAdapterError
	if err := Register(a, ok); !errors.As(err, &de) {
		t.Fatalf("dup err %T is not *DuplicateAdapterError", err)
	} else if de.Adapter != a {
		t.Fatalf("dup carried adapter = %v want %v", de.Adapter, a)
	}
}

func TestOpen_unknownAdapter_returnsUnknownError(t *testing.T) {
	a := Adapter(9999)
	w, err := Open(a, Options{})
	if !errors.Is(err, ErrUnknownAdapter) {
		t.Fatalf("Open unknown err = %v, want ErrUnknownAdapter", err)
	}
	if w != nil {
		t.Fatalf("Open unknown webhook = %v, want nil", w)
	}
	var ue *UnknownAdapterError
	if !errors.As(err, &ue) {
		t.Fatalf("err %T is not *UnknownAdapterError", err)
	}
	if ue.Adapter != a {
		t.Fatalf("carried adapter = %v want %v", ue.Adapter, a)
	}
}

func TestOpen_invalidOptions_validatedFirst(t *testing.T) {
	a := freshAdapter()
	w, err := Open(a, Options{Timeout: -time.Second})
	if !errors.Is(err, ErrInvalidOptions) {
		t.Fatalf("Open invalid-opts err = %v, want ErrInvalidOptions", err)
	}
	if w != nil {
		t.Fatalf("Open invalid-opts webhook = %v, want nil", w)
	}
	if !strings.Contains(err.Error(), "webhook: open") {
		t.Fatalf("Open err %q missing %q", err.Error(), "webhook: open")
	}
}

func TestOpen_factoryError_wrappedWithAdapter(t *testing.T) {
	a := freshAdapter()
	sentinel := errors.New("boom")
	if err := Register(a, func(Options) (Webhook, error) { return nil, sentinel }); err != nil {
		t.Fatalf("Register err = %v", err)
	}
	w, err := Open(a, Options{})
	if !errors.Is(err, sentinel) {
		t.Fatalf("Open err = %v, want wrap of sentinel", err)
	}
	if w != nil {
		t.Fatalf("Open factory-error webhook = %v, want nil", w)
	}
	if !strings.Contains(err.Error(), "webhook: open") {
		t.Fatalf("Open err %q missing %q", err.Error(), "webhook: open")
	}
}

func TestWebhook_stub_smoke(t *testing.T) {
	a := freshAdapter()
	stub := &stubWebhook{}
	if err := Register(a, func(Options) (Webhook, error) { return stub, nil }); err != nil {
		t.Fatalf("Register err = %v", err)
	}
	w, err := Open(a, Options{})
	if err != nil {
		t.Fatalf("Open err = %v", err)
	}
	ctx := context.Background()
	if err := w.Register(ctx, "order.created", "https://example.com/hook", "s3cr3t"); err != nil {
		t.Fatalf("Register err = %v", err)
	}
	if stub.event != "order.created" || stub.target != "https://example.com/hook" || stub.secret != "s3cr3t" {
		t.Fatalf("stub recorded %+v, want event/target/secret", stub)
	}
	if err := w.Deliver(ctx, "order.created", []byte(`{}`)); err != nil {
		t.Fatalf("Deliver err = %v", err)
	}
	if string(stub.payload) != `{}` {
		t.Fatalf("stub payload = %q want %q", stub.payload, `{}`)
	}
	if err := w.Unregister(ctx, "order.created", "https://example.com/hook"); err != nil {
		t.Fatalf("Unregister err = %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close err = %v", err)
	}
}
